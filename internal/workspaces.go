package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-orz/orz"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/handler"
	"go.uber.org/zap"
)

type workspace struct {
	app        *orz.App
	components *AppComponents
}

type workspaceRouter struct {
	mu      sync.RWMutex
	parent  *orz.App
	cfg     *config.AppConfig
	root    string
	running bool
	owner   string
	spaces  map[string]*workspace
}

// workspaceID is stable across restarts and safe as a directory/metric label.
// Usernames never become filesystem paths, even if the configuration is edited.
func workspaceID(username string) string {
	sum := sha256.Sum256([]byte(username))
	return hex.EncodeToString(sum[:])
}

func bindLegacyOwner(path, owner string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := file.WriteString(owner)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if !os.IsExist(err) {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(existing) != owner {
		return fmt.Errorf("legacy database belongs to %q, refusing to reassign it to %q", existing, owner)
	}
	return nil
}

func setupWorkspaces(app *orz.App, cfg *config.AppConfig) error {
	router, err := buildWorkspaceRouter(app, cfg)
	if err != nil {
		return err
	}
	if len(router.spaces) > 1 || registrationEnabled(cfg) {
		if err := verifyMetricsAuthentication(cfg.VictoriaMetrics); err != nil {
			return err
		}
	}
	router.running = true
	for _, space := range router.spaces {
		startWorkspaceJobs(app.Context(), space.components, space.app.Logger())
	}
	router.install(app.GetEcho())
	return nil
}

func buildWorkspaceRouter(app *orz.App, cfg *config.AppConfig) (*workspaceRouter, error) {
	if (cfg.OIDC != nil && cfg.OIDC.Enabled) || (cfg.GitHub != nil && cfg.GitHub.Enabled) {
		return nil, fmt.Errorf("isolated workspaces currently require configured password accounts; disable OIDC and GitHub OAuth")
	}
	if app.GetConfig().Database.Type != orz.DatabaseSqlite {
		return nil, fmt.Errorf("isolated workspaces currently require SQLite")
	}
	owner, root := "admin", ""
	if cfg.Workspaces != nil {
		if cfg.Workspaces.LegacyOwner != "" {
			owner = cfg.Workspaces.LegacyOwner
		}
		root = cfg.Workspaces.Dir
	}
	if cfg.Users[owner] == "" {
		return nil, fmt.Errorf("legacy workspace owner %q must exist in App.Users", owner)
	}
	dbPath := app.GetConfig().Database.Sqlite.Path
	if dbPath == "" || strings.Contains(dbPath, "?") || dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return nil, fmt.Errorf("workspaces require an explicit SQLite file path")
	}
	if root == "" {
		root = filepath.Join(filepath.Dir(dbPath), "workspaces")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := bindLegacyOwner(dbPath+".workspace-owner", owner); err != nil {
		return nil, err
	}
	router := &workspaceRouter{owner: owner, spaces: map[string]*workspace{}, parent: app, cfg: cfg, root: root}
	if err := router.initializeAccounts(); err != nil {
		return nil, err
	}
	var accounts []platformAccount
	if err := app.GetDatabase().Order("username").Find(&accounts).Error; err != nil {
		return nil, err
	}
	for _, account := range accounts {
		space, err := router.newWorkspace(account.Username, account.PasswordHash)
		if err != nil {
			return nil, err
		}
		router.spaces[account.Username] = space
	}

	return router, nil
}

func (r *workspaceRouter) newWorkspace(name, hash string) (*workspace, error) {
	child := orz.NewApp()
	child.SetLogger(r.parent.Logger().With(zap.String("workspace", name)))
	child.SetEcho(echo.New())
	child.GetEcho().IPExtractor = r.parent.GetEcho().IPExtractor
	childCfg := *r.cfg
	childCfg.Users = map[string]string{name: hash}
	namespace := ""
	if name == r.owner {
		child.SetDatabase(r.parent.GetDatabase())
	} else {
		namespace = workspaceID(name)
		dir := filepath.Join(r.root, namespace)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		dbCfg := r.parent.GetConfig().Database
		dbCfg.URL = ""
		dbCfg.Sqlite.Path = filepath.Join(dir, "pika.db")
		db, err := orz.ConnectDatabaseWithLogger(dbCfg, child.Logger())
		if err != nil {
			return nil, err
		}
		child.SetDatabase(db)
		childCfg.WorkspaceThemeDir = filepath.Join(dir, "themes")
	}
	components, err := initializeWorkspace(child, &childCfg, namespace)
	if err != nil {
		if name != r.owner {
			if db, dbErr := child.GetDatabase().DB(); dbErr == nil {
				_ = db.Close()
			}
		}
		return nil, fmt.Errorf("initialize workspace %q: %w", name, err)
	}
	return &workspace{app: child, components: components}, nil
}

func (r *workspaceRouter) space(name string) *workspace {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.spaces[name]
}

func (r *workspaceRouter) install(e *echo.Echo) {
	r.installAccountRoutes(e)
	route := func(c *echo.Context) error { return r.route(c) }
	e.Any("/*", route)
	e.Any("/", route)
	e.GET("/ws/agent", r.space(r.owner).components.AgentHandler.RouteWebSocket(func(ctx context.Context, key string) (*handler.AgentHandler, error) {
		space := r.keyWorkspace(ctx, key, "agent")
		if space == nil {
			return nil, fmt.Errorf("invalid agent key")
		}
		return space.components.AgentHandler, nil
	}))
}

func (r *workspaceRouter) keyWorkspace(ctx context.Context, key, kind string) *workspace {
	if key == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found *workspace
	for name, space := range r.spaces {
		if !r.accountActive(name) {
			continue
		}
		// Query within each workspace without logging the secret or caching a revoked key.
		k, err := space.components.ApiKeyService.ApiKeyRepo.FindEnabledByKey(ctx, key)
		if err != nil {
			continue
		}
		typ := k.Type
		if typ == "" {
			typ = "agent"
		}
		if typ != kind {
			continue
		}
		if found != nil {
			return nil
		} // ambiguous keys fail closed
		found = space
	}
	return found
}

func (r *workspaceRouter) route(c *echo.Context) error {
	req := c.Request()
	path := req.URL.Path
	primary := r.space(r.owner)
	var target *workspace
	if path == "/api/agent/install.sh" || strings.HasPrefix(path, "/api/agent/downloads/") {
		key := req.URL.Query().Get("key")
		if path == "/api/agent/install.sh" {
			key = req.URL.Query().Get("token")
		}
		target = r.keyWorkspace(req.Context(), key, "agent")
		if target == nil {
			return c.JSON(401, map[string]string{"message": "无效的探针密钥"})
		}
	} else {
		token := ""
		if auth := req.Header.Get("Authorization"); auth != "" {
			if !strings.HasPrefix(auth, "Bearer ") {
				return c.JSON(401, map[string]string{"message": "无效的认证令牌"})
			}
			token = strings.TrimPrefix(auth, "Bearer ")
			if claims, err := primary.components.AccountHandler.ValidateToken(token); err == nil {
				if r.validAccountClaims(claims) {
					target = r.space(claims.Username)
				}
			} else if strings.HasPrefix(path, "/api/admin/") {
				target = r.keyWorkspace(req.Context(), token, "admin")
			}
			if target == nil {
				return c.JSON(401, map[string]string{"message": "无效的认证令牌"})
			}
		} else if cookie, err := req.Cookie("pika_private_session"); err == nil {
			if claims, err := primary.components.AccountHandler.ValidateToken(cookie.Value); err == nil {
				if r.validAccountClaims(claims) {
					target = r.space(claims.Username)
				}
			}
			if target == nil {
				// An invalid/deleted account must never fall through to the legacy owner's
				// handler: that handler recognizes the shared JWT signing key.
				http.SetCookie(c.Response(), &http.Cookie{Name: "pika_private_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode})
				if path != "/admin/login" && path != "/admin/register" && path != "/api/auth/config" && path != "/api/config" && !strings.HasPrefix(path, "/admin/assets/") {
					if strings.HasPrefix(path, "/api/") {
						return c.JSON(401, map[string]string{"message": "请重新登录"})
					}
					return c.Redirect(303, "/admin/login")
				}
				// Strip the invalid cookie before entering the anonymous login shell.
				req.Header.Del("Cookie")
			}
		}
	}
	if target == nil {
		target = primary
	}
	// Prevent browsers/shared proxies retaining per-workspace configuration/assets.
	c.Response().Header().Set("Cache-Control", "private, no-store")
	c.Response().Header().Add("Vary", "Cookie")
	c.Response().Header().Add("Vary", "Authorization")
	target.app.GetEcho().ServeHTTP(c.Response(), req)
	return nil
}

// Notification webhooks and HTTP checks can reach private network services.
// Require storage authentication so these features cannot bypass metric scoping.
func verifyMetricsAuthentication(cfg *config.VMConfig) error {
	if cfg == nil || !cfg.Enabled || cfg.Username == "" || cfg.Password == "" {
		return fmt.Errorf("multiple workspaces require VictoriaMetrics Username/Password and server-side HTTP authentication")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	endpoint := strings.TrimRight(cfg.URL, "/") + "/api/v1/query?query=1"
	for _, authenticated := range []bool{false, true} {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return err
		}
		if authenticated {
			req.SetBasicAuth(cfg.Username, cfg.Password)
		}
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("cannot verify VictoriaMetrics authentication: %w", err)
		}
		res.Body.Close()
		expected := 401
		if authenticated {
			expected = 200
		}
		if res.StatusCode != expected {
			return fmt.Errorf("VictoriaMetrics authentication check failed (authenticated=%t, status=%d)", authenticated, res.StatusCode)
		}
	}
	return nil
}
