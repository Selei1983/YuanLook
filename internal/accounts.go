package internal

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/service"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Accounts are platform metadata, independent of each user's monitoring data.
// Configured accounts are imported once; a restart must never restore an old
// password or re-enable an account that the administrator disabled.
type platformAccount struct {
	Username       string    `gorm:"primaryKey;size:64" json:"username"`
	PasswordHash   string    `json:"-"`
	Enabled        bool      `json:"enabled"`
	SessionVersion uint64    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type accountView struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

func registrationEnabled(cfg *config.AppConfig) bool {
	return cfg.Workspaces == nil || cfg.Workspaces.RegistrationEnabled == nil || *cfg.Workspaces.RegistrationEnabled
}

func (r *workspaceRouter) initializeAccounts() error {
	db := r.parent.GetDatabase()
	if err := db.AutoMigrate(&platformAccount{}); err != nil {
		return err
	}
	for name, hash := range r.cfg.Users {
		if strings.TrimSpace(name) != name || name == "" || hash == "" {
			return errors.New("invalid workspace account configuration")
		}
		account := platformAccount{Username: name, PasswordHash: hash, Enabled: true}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *workspaceRouter) account(name string) (*platformAccount, error) {
	var account platformAccount
	err := r.parent.GetDatabase().Where("username = ?", name).First(&account).Error
	return &account, err
}
func (r *workspaceRouter) accountActive(name string) bool {
	a, err := r.account(name)
	return err == nil && a.Enabled
}
func (r *workspaceRouter) validAccountClaims(claims *service.JWTClaims) bool {
	a, err := r.account(claims.Username)
	return err == nil && a.Enabled && a.SessionVersion == claims.SessionVersion
}
func (r *workspaceRouter) view(a *platformAccount) accountView {
	role := "user"
	if a.Username == r.owner {
		role = "admin"
	}
	return accountView{UserID: a.Username, Username: a.Username, Role: role, Enabled: a.Enabled, CreatedAt: a.CreatedAt}
}
func accountError(c *echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"message": message})
}
func accountBody(c *echo.Context, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("请求格式不正确")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("请求只能包含一个 JSON 对象")
	}
	return nil
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$`)

func validateNewPassword(password string) error {
	// bcrypt has a 72-byte limit; do not silently truncate passwords.
	if len(password) < 10 || len(password) > 72 {
		return errors.New("密码长度须为 10–72 字节")
	}
	return nil
}

func (r *workspaceRouter) installAccountRoutes(e *echo.Echo) {
	// All account endpoints require a real user JWT, never a monitoring API key.
	limiter := func(burst int, seconds float64) echo.MiddlewareFunc {
		return middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{Rate: 1 / seconds, Burst: burst, ExpiresIn: time.Hour}))
	}
	noStore := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "private, no-store")
			return next(c)
		}
	}
	e.GET("/api/auth/config", func(c *echo.Context) error {
		return c.JSON(200, map[string]bool{"passwordEnabled": true, "oidcEnabled": false, "githubEnabled": false, "registrationEnabled": registrationEnabled(r.cfg)})
	}, noStore)
	e.POST("/api/login", r.loginAccount, noStore, limiter(20, 3))
	e.POST("/api/register", r.registerAccount, noStore, limiter(5, 600))
	e.GET("/api/admin/account/info", r.currentAccount, noStore)
	e.POST("/api/admin/account/password", r.changePassword, noStore, limiter(10, 10))
	e.GET("/api/admin/accounts", r.listAccounts, noStore)
	e.POST("/api/admin/accounts", r.createAccount, noStore, limiter(20, 3))
	e.PUT("/api/admin/accounts/:username", r.updateAccount, noStore, limiter(30, 2))
}

func (r *workspaceRouter) authenticatedAccount(c *echo.Context, admin bool) (*platformAccount, error) {
	auth := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, echo.NewHTTPError(401, "请先登录")
	}
	claims, err := r.space(r.owner).components.AccountHandler.ValidateToken(strings.TrimPrefix(auth, "Bearer "))
	if err != nil || !r.validAccountClaims(claims) || r.space(claims.Username) == nil {
		return nil, echo.NewHTTPError(401, "登录已失效，请重新登录")
	}
	if admin && claims.Username != r.owner {
		return nil, echo.NewHTTPError(403, "仅平台管理员可以管理账号")
	}
	return r.account(claims.Username)
}

func (r *workspaceRouter) loginAccount(c *echo.Context) error {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := accountBody(c, &input); err != nil {
		return accountError(c, 400, err.Error())
	}
	account, err := r.account(input.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(input.Password)) != nil {
		return accountError(c, 400, "用户名或密码错误")
	}
	if !account.Enabled {
		return accountError(c, 403, "账号已停用，请联系管理员")
	}
	if r.space(account.Username) == nil {
		return accountError(c, 503, "账号空间尚未就绪")
	}
	hours := r.cfg.JWT.ExpiresHours
	if hours <= 0 {
		hours = 168
	}
	expires := time.Now().Add(time.Duration(hours) * time.Hour)
	claims := service.JWTClaims{Username: account.Username, UserID: account.Username, SessionVersion: account.SessionVersion, RegisteredClaims: jwt.RegisteredClaims{Issuer: "pika", Subject: account.Username, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(expires)}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(r.cfg.JWT.Secret))
	if err != nil {
		return accountError(c, 500, "登录失败")
	}
	c.SetCookie(&http.Cookie{Name: "pika_private_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode})
	return c.JSON(200, map[string]any{"token": token, "expiresAt": expires.UnixMilli(), "user": r.view(account)})
}

func (r *workspaceRouter) registerAccount(c *echo.Context) error {
	if !registrationEnabled(r.cfg) {
		return accountError(c, 403, "注册暂未开放")
	}
	return r.addAccount(c)
}
func (r *workspaceRouter) createAccount(c *echo.Context) error {
	if _, err := r.authenticatedAccount(c, true); err != nil {
		return err
	}
	return r.addAccount(c)
}
func (r *workspaceRouter) addAccount(c *echo.Context) error {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := accountBody(c, &input); err != nil {
		return accountError(c, 400, err.Error())
	}
	if !usernamePattern.MatchString(input.Username) {
		return accountError(c, 400, "用户名须为 3–32 位字母、数字、下划线或短横线，并以字母或数字开头")
	}
	if err := validateNewPassword(input.Password); err != nil {
		return accountError(c, 400, err.Error())
	}
	if err := verifyMetricsAuthentication(r.cfg.VictoriaMetrics); err != nil {
		return accountError(c, 503, "账号服务暂不可用，请联系管理员检查指标存储认证")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return accountError(c, 500, "创建账号失败")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Serialize creation with publication to avoid duplicate workspace schedulers.
	if _, err := r.account(input.Username); err == nil {
		return accountError(c, 409, "该用户名已被使用")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return accountError(c, 500, "读取账号失败")
	}
	space, err := r.newWorkspace(input.Username, string(hash))
	if err != nil {
		return accountError(c, 500, "初始化账号空间失败")
	}
	account := platformAccount{Username: input.Username, PasswordHash: string(hash), Enabled: true}
	if err := r.parent.GetDatabase().Create(&account).Error; err != nil {
		db, _ := space.app.GetDatabase().DB()
		if db != nil {
			_ = db.Close()
		}
		return accountError(c, 500, "保存账号失败")
	}
	r.spaces[account.Username] = space
	if r.running {
		startWorkspaceJobs(r.parent.Context(), space.components, space.app.Logger())
	}
	return c.JSON(201, r.view(&account))
}
func (r *workspaceRouter) currentAccount(c *echo.Context) error {
	account, err := r.authenticatedAccount(c, false)
	if err != nil {
		return err
	}
	return c.JSON(200, r.view(account))
}
func (r *workspaceRouter) listAccounts(c *echo.Context) error {
	if _, err := r.authenticatedAccount(c, true); err != nil {
		return err
	}
	var accounts []platformAccount
	if err := r.parent.GetDatabase().Order("created_at ASC, username ASC").Find(&accounts).Error; err != nil {
		return accountError(c, 500, "读取账号列表失败")
	}
	result := make([]accountView, 0, len(accounts))
	for _, a := range accounts {
		result = append(result, r.view(&a))
	}
	return c.JSON(200, result)
}
func (r *workspaceRouter) updateAccount(c *echo.Context) error {
	if _, err := r.authenticatedAccount(c, true); err != nil {
		return err
	}
	var input struct {
		Enabled  *bool  `json:"enabled"`
		Password string `json:"password"`
	}
	if err := accountBody(c, &input); err != nil {
		return accountError(c, 400, err.Error())
	}
	name := c.Param("username")
	if name == r.owner && input.Enabled != nil && !*input.Enabled {
		return accountError(c, 400, "不能停用平台管理员")
	}
	updates := map[string]any{}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}
	if input.Password != "" {
		if err := validateNewPassword(input.Password); err != nil {
			return accountError(c, 400, err.Error())
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return accountError(c, 500, "重置密码失败")
		}
		updates["password_hash"] = string(hash)
	}
	if len(updates) == 0 {
		return accountError(c, 400, "请选择要修改的账号信息")
	}
	updates["session_version"] = gorm.Expr("session_version + 1")
	result := r.parent.GetDatabase().Model(&platformAccount{}).Where("username = ?", name).Updates(updates)
	if result.Error != nil {
		return accountError(c, 500, "更新账号失败")
	}
	if result.RowsAffected == 0 {
		return accountError(c, 404, "账号不存在")
	}
	return c.JSON(200, map[string]string{"message": "账号已更新，旧登录会话已失效"})
}
func (r *workspaceRouter) changePassword(c *echo.Context) error {
	account, err := r.authenticatedAccount(c, false)
	if err != nil {
		return err
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if err := accountBody(c, &input); err != nil {
		return accountError(c, 400, err.Error())
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(input.CurrentPassword)) != nil {
		return accountError(c, 400, "当前密码不正确")
	}
	if err := validateNewPassword(input.Password); err != nil {
		return accountError(c, 400, err.Error())
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return accountError(c, 500, "修改密码失败")
	}
	result := r.parent.GetDatabase().Model(&platformAccount{}).Where("username = ? AND session_version = ? AND enabled = ?", account.Username, account.SessionVersion, true).Updates(map[string]any{"password_hash": string(hash), "session_version": gorm.Expr("session_version + 1")})
	if result.Error != nil {
		return accountError(c, 500, "修改密码失败")
	}
	if result.RowsAffected == 0 {
		return accountError(c, 409, "账号已发生变化，请重新登录")
	}
	c.SetCookie(&http.Cookie{Name: "pika_private_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode})
	return c.JSON(200, map[string]string{"message": "密码已修改，请重新登录"})
}
