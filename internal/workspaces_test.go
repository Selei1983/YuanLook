package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pika-monitor/pika/internal/protocol"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-orz/orz"
	_ "github.com/go-orz/orz/drivers/sqlite"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/service"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func testWorkspaceRouter(t *testing.T) (*workspaceRouter, *echo.Echo) {
	t.Helper()
	t.Setenv("PIKA_WEB_DIR", "")
	t.Setenv("PIKA_THEME_DIR", filepath.Join(t.TempDir(), "legacy-themes"))
	root := t.TempDir()
	app := orz.NewApp()
	if err := app.LoadConfigFromBytes([]byte(fmt.Sprintf("database:\n  enabled: true\n  type: sqlite\n  sqlite:\n    path: %s\n", filepath.Join(root, "pika.db")))); err != nil {
		t.Fatal(err)
	}
	app.SetLogger(zap.NewNop())
	app.EnableHTTP()
	if err := app.EnableDatabase(); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("workspace-test-password"), bcrypt.MinCost)
	cfg := &config.AppConfig{Users: map[string]string{"admin": string(hash), "alice": string(hash)}, JWT: config.JWTConfig{Secret: "workspace-test-key-at-least-32-characters", ExpiresHours: 1}, VictoriaMetrics: &config.VMConfig{Enabled: true, URL: "http://127.0.0.1:1", QueryTimeout: 1, WriteTimeout: 1}}
	router, err := buildWorkspaceRouter(app, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, space := range router.spaces {
		sql, _ := space.app.GetDatabase().DB()
		t.Cleanup(func() { _ = sql.Close() })
	}
	router.install(app.GetEcho())
	return router, app.GetEcho()
}

func requestWorkspace(t *testing.T, e *echo.Echo, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	return res
}
func loginWorkspace(t *testing.T, e *echo.Echo, name string) (string, *http.Cookie) {
	t.Helper()
	res := requestWorkspace(t, e, "POST", "/api/login", "", fmt.Sprintf(`{"username":%q,"password":"workspace-test-password"}`, name))
	if res.Code != 200 {
		t.Fatalf("login %s: %d %s", name, res.Code, res.Body.String())
	}
	var login service.LoginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Token == "" {
		t.Fatalf("empty token: %s", res.Body.String())
	}
	return login.Token, res.Result().Cookies()[0]
}

func TestWorkspaceIsolationAcrossRealHandlers(t *testing.T) {
	router, e := testWorkspaceRouter(t)
	ctx := context.Background()
	admin := router.spaces["admin"]
	alice := router.spaces["alice"]
	// A migrated legacy object retains its ID and cannot be read in another DB.
	m := models.MonitorTask{ID: "legacy-monitor", Name: "legacy-only", Type: "http", Target: "https://example.com", Visibility: "private"}
	if err := admin.app.GetDatabase().Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	a := models.Agent{ID: "legacy-agent", Name: "legacy-server", Enabled: true}
	if err := admin.app.GetDatabase().Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	adminToken, _ := loginWorkspace(t, e, "admin")
	aliceToken, aliceCookie := loginWorkspace(t, e, "alice")
	for _, path := range []string{"/api/admin/monitors", "/api/monitors", "/api/admin/agents", "/api/agents"} {
		res := requestWorkspace(t, e, "GET", path, aliceToken, "")
		if res.Code != 200 || bytes.Contains(res.Body.Bytes(), []byte("legacy-")) {
			t.Fatalf("alice read %s: %d %s", path, res.Code, res.Body.String())
		}
	}
	for _, path := range []string{"/api/admin/monitors/legacy-monitor", "/api/admin/agents/legacy-agent", "/api/agents/legacy-agent"} {
		res := requestWorkspace(t, e, "GET", path, aliceToken, "")
		if res.Code == 200 {
			t.Fatalf("cross-workspace GET accepted: %s", path)
		}
	}
	res := requestWorkspace(t, e, "GET", "/api/admin/monitors/legacy-monitor", adminToken, "")
	if res.Code != 200 {
		t.Fatalf("legacy owner lost monitor: %d", res.Code)
	}
	// Same IDs/names in two physical databases still cannot target each other.
	m.Name = "alice-only"
	if err := alice.app.GetDatabase().Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/monitors/legacy-monitor", aliceToken, "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), "alice-only") || strings.Contains(res.Body.String(), "legacy-only") {
		t.Fatal(res.Body.String())
	}
	value := `{"name":"settings","value":{"systemNameZh":"Alice private","systemNameEn":"Alice"}}`
	if res = requestWorkspace(t, e, "PUT", "/api/admin/properties/system_config", aliceToken, value); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/properties/system_config", adminToken, "")
	if strings.Contains(res.Body.String(), "Alice private") {
		t.Fatal("property/cache leaked")
	}
	if admin.components.ThemeService == alice.components.ThemeService || admin.components.WSManager == alice.components.WSManager {
		t.Fatal("shared stateful components")
	}
	// A browser cookie selects its own data but cannot authorize mutations alone.
	req := httptest.NewRequest("GET", "/api/monitors", nil)
	req.AddCookie(aliceCookie)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != 200 || strings.Contains(rr.Body.String(), "legacy-only") {
		t.Fatal(rr.Body.String())
	}
	req = httptest.NewRequest("DELETE", "/api/admin/monitors/legacy-monitor", nil)
	req.AddCookie(aliceCookie)
	rr = httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("cookie allowed mutation: %d", rr.Code)
	}
	// Keys route by their stored workspace; agent keys cannot access admin routes.
	key, err := alice.components.ApiKeyService.GenerateApiKey(ctx, "alice-api", "alice", "admin")
	if err != nil {
		t.Fatal(err)
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/monitors", key.Key, "")
	if res.Code != 200 || strings.Contains(res.Body.String(), "legacy-only") {
		t.Fatal(res.Body.String())
	}
	agentKey, err := alice.components.ApiKeyService.GenerateApiKey(ctx, "probe", "alice", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if router.keyWorkspace(ctx, agentKey.Key, "agent") != alice {
		t.Fatal("wrong agent routing")
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/monitors", agentKey.Key, "")
	if res.Code != 401 {
		t.Fatal("agent key allowed admin access")
	}
	if err := alice.components.ApiKeyService.DisableApiKey(ctx, agentKey.ID); err != nil {
		t.Fatal(err)
	}
	if router.keyWorkspace(ctx, agentKey.Key, "agent") != nil {
		t.Fatal("revoked key still routed")
	}

	// Reading, disabling or deleting a guessed key ID cannot affect the owner.
	oldKey, err := admin.components.ApiKeyService.GenerateApiKey(ctx, "old-probe", "admin", "agent")
	if err != nil {
		t.Fatal(err)
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/api-keys/"+oldKey.ID+"/raw", aliceToken, "")
	if res.Code == 200 || strings.Contains(res.Body.String(), oldKey.Key) {
		t.Fatal("other workspace key disclosed")
	}
	_ = requestWorkspace(t, e, "POST", "/api/admin/api-keys/"+oldKey.ID+"/disable", aliceToken, "{}")
	_ = requestWorkspace(t, e, "DELETE", "/api/admin/api-keys/"+oldKey.ID, aliceToken, "")
	if _, err := admin.components.ApiKeyService.ValidateApiKey(ctx, oldKey.Key, "agent"); err != nil {
		t.Fatal("other workspace key altered")
	}
	// Explicit bad Authorization must never fall back to the valid browser cookie.
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.AddCookie(aliceCookie)
	req.Header.Set("Authorization", "Bearer forged")
	rr = httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatal("invalid bearer fell back to cookie")
	}
	// Removing an account revokes existing tokens and cookies immediately on reload.
	delete(router.spaces, "alice")
	res = requestWorkspace(t, e, "GET", "/api/agents", aliceToken, "")
	if res.Code != 401 {
		t.Fatal("removed account bearer accepted")
	}
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.AddCookie(aliceCookie)
	rr = httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatal("removed account cookie fell through to admin")
	}
}

func TestLegacyOwnerBindingCannotBeReassigned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner")
	if err := bindLegacyOwner(path, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := bindLegacyOwner(path, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := bindLegacyOwner(path, "alice"); err == nil {
		t.Fatal("legacy data can be reassigned")
	}
	if strings.ContainsAny(workspaceID("../../alice"), "/\\.") {
		t.Fatal("unsafe workspace directory")
	}
}

func TestWorkspaceWebSocketUsesRegistrationKey(t *testing.T) {
	router, e := testWorkspaceRouter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, space := range router.spaces {
		go space.components.WSManager.Run(ctx)
	}
	server := httptest.NewServer(e)
	defer server.Close()
	for _, name := range []string{"admin", "alice"} {
		space := router.spaces[name]
		key, err := space.components.ApiKeyService.GenerateApiKey(ctx, "probe", name, "agent")
		if err != nil {
			t.Fatal(err)
		}
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/agent", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conn.WriteJSON(protocol.OutboundMessage{Type: protocol.MessageTypeRegister, Data: protocol.RegisterRequest{ApiKey: key.Key, AgentInfo: protocol.AgentInfo{ID: "same-probe-id", Hostname: name + "-host", Name: name + "-server"}}}); err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var ack protocol.InputMessage
		if err := conn.ReadJSON(&ack); err != nil {
			t.Fatal(err)
		}
		var status protocol.RegisterResponse
		if err := json.Unmarshal(ack.Data, &status); err != nil {
			t.Fatal(err)
		}
		if status.Status != "success" || status.AgentID != "same-probe-id" {
			t.Fatalf("registration failed: %+v", status)
		}
	}
	for _, name := range []string{"admin", "alice"} {
		a, err := router.spaces[name].components.AgentService.AgentRepo.FindById(ctx, "same-probe-id")
		if err != nil || a.Hostname != name+"-host" {
			t.Fatalf("registration leaked across accounts: %+v %v", a, err)
		}
	}
}

func TestMultipleWorkspacesRequireProtectedMetricStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "test" || p != "secret" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cfg := &config.VMConfig{Enabled: true, URL: server.URL, Username: "test", Password: "secret"}
	if err := verifyMetricsAuthentication(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Password = "wrong"
	if err := verifyMetricsAuthentication(cfg); err == nil {
		t.Fatal("wrong storage credentials accepted")
	}
	open := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer open.Close()
	cfg.URL = open.URL
	if err := verifyMetricsAuthentication(cfg); err == nil {
		t.Fatal("unauthenticated storage accepted")
	}
}

func TestPublicHomepageDoesNotExposeWorkspaceData(t *testing.T) {
	r, e := testWorkspaceRouter(t)
	r.space("admin").app.GetDatabase().Create(&models.Agent{ID: "private-homepage-agent", Name: "secret-server-identity", Enabled: true})
	for _, cookie := range []string{"", "expired-cookie"} {
		request := httptest.NewRequest("GET", "/", nil)
		if cookie != "" {
			request.AddCookie(&http.Cookie{Name: "pika_private_session", Value: cookie})
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		if response.Code != 200 || !strings.Contains(response.Body.String(), "创建我的监控空间") {
			t.Fatalf("public homepage: %d", response.Code)
		}
		if strings.Contains(response.Body.String(), "secret-server-identity") {
			t.Fatal("homepage leaked workspace data")
		}
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("homepage can be cached across sessions")
		}
	}
	for _, endpoint := range []struct {
		path   string
		status int
	}{
		{"/api/agents", 401}, {"/api/monitors", 401}, {"/api/admin/agents", 401}, {"/t/default/index.html", 401}, {"/servers/private-homepage-agent", 303}, {"/admin/agents", 303},
	} {
		response := requestWorkspace(t, e, "GET", endpoint.path, "", "")
		if response.Code != endpoint.status {
			t.Fatalf("%s: %d", endpoint.path, response.Code)
		}
	}
	if res := requestWorkspace(t, e, "GET", "/", "forged", ""); res.Code != 401 {
		t.Fatal("forged bearer accepted")
	}
}

func TestSignedInHomepageKeepsPrivateTheme(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"pika-theme.json": `{"schemaVersion":1,"id":"default","name":"Test","version":"1.0.0","author":"Test","preview":"preview.png","entry":"dist/index.html","apiVersion":"v1","capabilities":["server-list","server-detail","monitor-list","monitor-detail"]}`,
		"dist/index.html": `<!doctype html><html><head></head><body>PRIVATE_THEME_SENTINEL</body></html>`,
		"preview.png":     "test-preview",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PIKA_DEFAULT_THEME_DIR", root)
	_, e := testWorkspaceRouter(t)
	token, cookie := loginWorkspace(t, e, "alice")
	for _, bearer := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/", nil)
		if bearer {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		if res.Code != 200 || !strings.Contains(res.Body.String(), "PRIVATE_THEME_SENTINEL") || strings.Contains(res.Body.String(), "创建我的监控空间") {
			t.Fatalf("signed-in homepage: %d %s", res.Code, res.Body.String())
		}
	}
	res := requestWorkspace(t, e, "GET", "/", "", "")
	if strings.Contains(res.Body.String(), "PRIVATE_THEME_SENTINEL") {
		t.Fatal("theme leaked to anonymous homepage")
	}
}
