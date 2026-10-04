package handler

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v5"
)

//go:embed templates/home.html
var homepageHTML []byte

// Home introduces the product without reading workspace data. Signed-in users
// keep the existing theme dashboard and its root-relative navigation.
func (h *WebHandler) Home(account *AccountHandler) echo.HandlerFunc {
	privateDashboard := PrivateReadAuth(account, true)(h.ServeSPA)
	return func(c *echo.Context) error {
		c.Response().Header().Add("Vary", "Cookie")
		c.Response().Header().Add("Vary", "Authorization")
		if c.Request().Header.Get("Authorization") != "" {
			return privateDashboard(c)
		}
		if cookie, err := c.Cookie(privateSessionCookie); err == nil {
			if _, err := account.ValidateToken(cookie.Value); err == nil {
				return privateDashboard(c)
			}
		}
		setHTMLHeaders(c)
		return c.Blob(http.StatusOK, "text/html; charset=utf-8", homepageHTML)
	}
}
