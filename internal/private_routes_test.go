package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/handler"
	"github.com/pika-monitor/pika/internal/service"
	"go.uber.org/zap"
)

// Exercise the actual registrations, so accidentally leaving an endpoint on
// optional authentication fails without calling the data services.
func TestPrivateReadRoutesRejectAnonymousAndForgedTokens(t *testing.T) {
	cfg := &config.AppConfig{JWT: config.JWTConfig{Secret: "private-route-test-secret-at-least-32-characters"}}
	account := handler.NewAccountHandler(service.NewAccountService(zap.NewNop(), nil, nil, nil, cfg))
	e := echo.New()
	setupPrivateReadAPI(e, &AppComponents{AccountHandler: account})
	for _, path := range []string{
		"/api/agents", "/api/agents/tags", "/api/agents/legacy-public-agent",
		"/api/agents/legacy-public-agent/metrics", "/api/agents/legacy-public-agent/metrics/latest",
		"/api/agents/legacy-public-agent/network-interfaces", "/api/monitors", "/api/monitors/sparklines",
		"/api/monitors/legacy-public-monitor/stats", "/api/monitors/legacy-public-monitor/agents",
		"/api/monitors/legacy-public-monitor/history", "/api/logo",
	} {
		for _, auth := range []string{"", "Bearer forged"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", auth)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			if res.Code != 401 {
				t.Fatalf("%s with auth=%q returned %d", path, auth, res.Code)
			}
		}
	}
}
