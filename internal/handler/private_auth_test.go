package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/service"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const privateTestSecret = "private-mode-test-secret-at-least-32-characters"

func privateTestAccount() *AccountHandler {
	cfg := &config.AppConfig{JWT: config.JWTConfig{Secret: privateTestSecret}}
	return NewAccountHandler(service.NewAccountService(zap.NewNop(), nil, nil, nil, cfg))
}

func privateTestToken(t *testing.T, expiry time.Time) string {
	t.Helper()
	claims := service.JWTClaims{UserID: "admin", Username: "admin", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(expiry)}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(privateTestSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestPrivateReadAuthentication(t *testing.T) {
	valid := privateTestToken(t, time.Now().Add(time.Hour))
	expired := privateTestToken(t, time.Now().Add(-time.Hour))
	for _, test := range []struct {
		name, header, cookie string
		status               int
	}{
		{"anonymous", "", "", 401},
		{"invalid", "Bearer invalid", "", 401},
		{"expired", "", expired, 401},
		{"cookie", "", valid, 200},
		{"bearer", "Bearer " + valid, "", 200},
		{"invalid header does not fall back", "Bearer invalid", valid, 401},
		{"malformed header", "Basic invalid", valid, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			e.GET("/data", func(c *echo.Context) error {
				if c.Get("authenticated") != true || c.Get("username") != "admin" {
					t.Fatal("missing authenticated context")
				}
				return c.String(200, "private-data")
			}, PrivateReadAuth(privateTestAccount(), false))
			req := httptest.NewRequest(http.MethodGet, "/data", nil)
			req.Header.Set("Authorization", test.header)
			if test.cookie != "" {
				req.AddCookie(&http.Cookie{Name: privateSessionCookie, Value: test.cookie})
			}
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			if res.Code != test.status {
				t.Fatalf("status %d, expected %d", res.Code, test.status)
			}
			if !strings.Contains(res.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("private response can be cached")
			}
			if res.Code != 200 && strings.Contains(res.Body.String(), "private-data") {
				t.Fatal("data leaked")
			}
		})
	}
}

func TestPrivatePageBoundaries(t *testing.T) {
	e := echo.New()
	e.GET("/*", func(c *echo.Context) error { return c.String(200, "page") }, PrivatePages(privateTestAccount()))
	for _, path := range []string{"/", "/servers/host", "/monitors/monitor", "/admin", "/admin/agents", "/logo.png", "/admin/login/", "/admin/login/anything"} {
		res := httptest.NewRecorder()
		e.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != 303 || res.Header().Get("Location") != "/admin/login" {
			t.Fatalf("anonymous page %s: %d", path, res.Code)
		}
	}
	for _, path := range []string{"/admin/login", "/admin/github/callback", "/admin/oidc/callback"} {
		res := httptest.NewRecorder()
		e.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != 200 {
			t.Fatalf("login route blocked: %s", path)
		}
	}
	for _, path := range []string{"/api/unknown", "/ws/unknown"} {
		res := httptest.NewRecorder()
		e.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != 404 {
			t.Fatalf("unknown API entered page flow: %s", path)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: privateSessionCookie, Value: privateTestToken(t, time.Now().Add(time.Hour))})
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatal("signed-in dashboard blocked")
	}
}

type privateTestValidator struct{}

func (privateTestValidator) Validate(any) error { return nil }

func TestPrivateLoginAndLogoutCookie(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{JWT: config.JWTConfig{Secret: privateTestSecret}, Users: map[string]string{"admin": string(hash)}}
	account := NewAccountHandler(service.NewAccountService(zap.NewNop(), service.NewUserService(zap.NewNop(), cfg), nil, nil, cfg))
	e := echo.New()
	e.Validator = privateTestValidator{}
	e.POST("/login", account.Login)
	req := httptest.NewRequest(http.MethodPost, "https://pika.example/login", strings.NewReader(`{"username":"admin","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("login failed: %s", res.Body.String())
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("missing session cookie: %v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != privateSessionCookie || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || cookie.Expires.Before(time.Now()) {
		t.Fatalf("unsafe session cookie: %#v", cookie)
	}
	if _, err := account.ValidateToken(cookie.Value); err != nil {
		t.Fatal(err)
	}
	logout := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "https://pika.example/logout", nil), logout)
	c.Set("userID", "admin")
	if err := account.Logout(c); err != nil {
		t.Fatal(err)
	}
	if cleared := logout.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge != -1 || cleared[0].Value != "" {
		t.Fatal("logout did not clear browser session")
	}
}

func TestPrivateReadAuthCannotAuthorizeMutations(t *testing.T) {
	e := echo.New()
	e.POST("/write", func(c *echo.Context) error { t.Fatal("cookie authorized a mutation"); return nil }, PrivateReadAuth(privateTestAccount(), false))
	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	req.AddCookie(&http.Cookie{Name: privateSessionCookie, Value: privateTestToken(t, time.Now().Add(time.Hour))})
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	if res.Code != 405 {
		t.Fatalf("expected 405, got %d", res.Code)
	}
}
