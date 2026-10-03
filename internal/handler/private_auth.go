package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/service"
)

const privateSessionCookie = "pika_private_session"

// The cookie authenticates page navigation and read-only dashboard requests.
// Administrative mutations still require the existing Authorization header.
func setPrivateSession(c *echo.Context, login *service.LoginResponse) {
	c.SetCookie(&http.Cookie{
		Name: privateSessionCookie, Value: login.Token, Path: "/",
		Expires:  time.UnixMilli(login.ExpiresAt),
		HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
	})
	c.Response().Header().Set("Cache-Control", "no-store")
}

func clearPrivateSession(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name: privateSessionCookie, Path: "/", MaxAge: -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true, Secure: c.Scheme() == "https", SameSite: http.SameSiteLaxMode,
	})
	c.Response().Header().Set("Cache-Control", "no-store")
}

// PrivateReadAuth must only be attached to GET/HEAD routes. An explicitly
// supplied invalid Authorization header must not fall back to a valid cookie.
func PrivateReadAuth(account *AccountHandler, page bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "private, no-store")
			c.Response().Header().Add("Vary", "Cookie")
			c.Response().Header().Add("Vary", "Authorization")
			if c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead {
				return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method Not Allowed")
			}
			token := ""
			if authorization := c.Request().Header.Get("Authorization"); authorization != "" {
				if strings.HasPrefix(authorization, "Bearer ") {
					token = strings.TrimPrefix(authorization, "Bearer ")
				}
			} else if cookie, err := c.Cookie(privateSessionCookie); err == nil {
				token = cookie.Value
			}
			if token != "" {
				if claims, err := account.ValidateToken(token); err == nil {
					c.Set("userID", claims.UserID)
					c.Set("username", claims.Username)
					c.Set("authenticated", true)
					return next(c)
				}
			}
			if page {
				return c.Redirect(http.StatusSeeOther, "/admin/login")
			}
			return echo.NewHTTPError(http.StatusUnauthorized, "请先登录")
		}
	}
}

// Only the login entry points are anonymous; arbitrary theme paths and files
// pass through the same server-side session check as the dashboard.
func PrivatePages(account *AccountHandler) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		protected := PrivateReadAuth(account, true)(next)
		return func(c *echo.Context) error {
			switch c.Request().URL.Path {
			case "/admin/login", "/admin/register", "/admin/github/callback", "/admin/oidc/callback":
				return next(c)
			}
			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/ws") {
				return echo.NewHTTPError(http.StatusNotFound, "Not Found")
			}
			return protected(c)
		}
	}
}
