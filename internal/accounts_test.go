package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/models"
)

func accountTestRouter(t *testing.T) (*workspaceRouter, *echo.Echo) {
	t.Helper()
	r, e := testWorkspaceRouter(t)
	r.cfg.Workspaces = &config.WorkspaceConfig{}
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, password, ok := req.BasicAuth()
		if !ok || user != "test" || password != "storage" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(vm.Close)
	r.cfg.VictoriaMetrics.URL = vm.URL
	r.cfg.VictoriaMetrics.Username = "test"
	r.cfg.VictoriaMetrics.Password = "storage"
	t.Cleanup(func() {
		for name, space := range r.spaces {
			if name == "admin" || name == "alice" {
				continue
			}
			db, _ := space.app.GetDatabase().DB()
			_ = db.Close()
		}
	})
	return r, e
}

func TestRegistrationAndAccountLifecycle(t *testing.T) {
	r, e := accountTestRouter(t)
	admin, _ := loginWorkspace(t, e, "admin")
	body := `{"username":"new-user","password":"workspace-test-password"}`
	res := requestWorkspace(t, e, "POST", "/api/register", "", body)
	if res.Code != 201 {
		t.Fatalf("register: %d %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "password") || strings.Contains(res.Body.String(), "Hash") {
		t.Fatal("password exposed")
	}
	user, cookie := loginWorkspace(t, e, "new-user")
	r.space("admin").app.GetDatabase().Create(&models.MonitorTask{ID: "owner-private", Name: "owner-private", Type: "http", Target: "https://example.com"})
	res = requestWorkspace(t, e, "GET", "/api/admin/monitors", user, "")
	if res.Code != 200 || strings.Contains(res.Body.String(), "owner-private") {
		t.Fatal("new user data isolation failed")
	}
	for _, endpoint := range []struct{ method, path, body string }{
		{"GET", "/api/admin/accounts", ""},
		{"POST", "/api/admin/accounts", `{"username":"intruder","password":"workspace-test-password"}`},
		{"PUT", "/api/admin/accounts/admin", `{"password":"hijack-password"}`},
	} {
		if got := requestWorkspace(t, e, endpoint.method, endpoint.path, user, endpoint.body); got.Code != 403 {
			t.Fatalf("non-admin %s: %d", endpoint.path, got.Code)
		}
	}
	res = requestWorkspace(t, e, "GET", "/api/admin/accounts", admin, "")
	if res.Code != 200 || !strings.Contains(res.Body.String(), "new-user") || strings.Contains(res.Body.String(), "PasswordHash") {
		t.Fatalf("list: %d %s", res.Code, res.Body.String())
	}
	if got := requestWorkspace(t, e, "PUT", "/api/admin/accounts/admin", admin, `{"enabled":false}`); got.Code != 400 {
		t.Fatal("owner can be disabled")
	}
	if got := requestWorkspace(t, e, "PUT", "/api/admin/accounts/new-user", admin, `{"enabled":false}`); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	if got := requestWorkspace(t, e, "GET", "/api/agents", user, ""); got.Code != 401 {
		t.Fatal("disabled bearer accepted")
	}
	req := httptest.NewRequest("GET", "/api/agents", nil)
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	if recorder.Code != 401 {
		t.Fatal("disabled cookie accepted")
	}
	if got := requestWorkspace(t, e, "POST", "/api/login", "", body); got.Code != 403 {
		t.Fatal("disabled login accepted")
	}
	requestWorkspace(t, e, "PUT", "/api/admin/accounts/new-user", admin, `{"enabled":true}`)
	if got := requestWorkspace(t, e, "GET", "/api/agents", user, ""); got.Code != 401 {
		t.Fatal("re-enable restored old token")
	}
	user, _ = loginWorkspace(t, e, "new-user")
	if got := requestWorkspace(t, e, "PUT", "/api/admin/accounts/new-user", admin, `{"password":"reset-password-123"}`); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	if got := requestWorkspace(t, e, "GET", "/api/agents", user, ""); got.Code != 401 {
		t.Fatal("password reset left old bearer valid")
	}
	if got := requestWorkspace(t, e, "POST", "/api/login", "", body); got.Code != 400 {
		t.Fatal("old password works")
	}
	res = requestWorkspace(t, e, "POST", "/api/login", "", `{"username":"new-user","password":"reset-password-123"}`)
	var session struct {
		Token string `json:"token"`
	}
	json.Unmarshal(res.Body.Bytes(), &session)
	if res.Code != 200 || session.Token == "" {
		t.Fatal("new password failed")
	}
	if got := requestWorkspace(t, e, "POST", "/api/admin/account/password", session.Token, `{"currentPassword":"wrong","password":"another-password-123"}`); got.Code != 400 {
		t.Fatal("change without old password")
	}
	if got := requestWorkspace(t, e, "POST", "/api/admin/account/password", session.Token, `{"currentPassword":"reset-password-123","password":"another-password-123"}`); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	if got := requestWorkspace(t, e, "GET", "/api/admin/account/info", session.Token, ""); got.Code != 401 {
		t.Fatal("change left old session valid")
	}
	// Importing config again (as on restart) must preserve password/status changes.
	requestWorkspace(t, e, "PUT", "/api/admin/accounts/alice", admin, `{"enabled":false,"password":"new-alice-password"}`)
	if err := r.initializeAccounts(); err != nil {
		t.Fatal(err)
	}
	alice, err := r.account("alice")
	if err != nil || alice.Enabled || alice.SessionVersion != 1 || alice.PasswordHash == r.cfg.Users["alice"] {
		t.Fatal("bootstrap overwrote persisted account")
	}
}

func TestAccountBoundariesAndValidation(t *testing.T) {
	r, e := accountTestRouter(t)
	for _, body := range []string{
		`{"username":"../outside","password":"workspace-test-password"}`,
		`{"username":"short","password":"short"}`,
		`{"username":"hacker","password":"workspace-test-password","role":"admin"}`,
		`{"username":"hacker","password":"` + strings.Repeat("a", 73) + `"}`,
	} {
		if got := requestWorkspace(t, e, "POST", "/api/register", "", body); got.Code != 400 {
			t.Fatalf("invalid registration accepted %d", got.Code)
		}
	}
	off := false
	r.cfg.Workspaces.RegistrationEnabled = &off
	if got := requestWorkspace(t, e, "POST", "/api/register", "", `{"username":"closed","password":"workspace-test-password"}`); got.Code != 403 {
		t.Fatal("closed registration accepted")
	}
	admin, _ := loginWorkspace(t, e, "admin")
	if got := requestWorkspace(t, e, "POST", "/api/admin/accounts", admin, `{"username":"created","password":"workspace-test-password"}`); got.Code != 201 {
		t.Fatal(got.Body.String())
	}
	if got := requestWorkspace(t, e, "POST", "/api/admin/accounts", admin, `{"username":"created","password":"workspace-test-password"}`); got.Code != 409 {
		t.Fatal("duplicate registration accepted")
	}
	_, cookie := loginWorkspace(t, e, "admin")
	req := httptest.NewRequest("PUT", "/api/admin/accounts/alice", strings.NewReader(`{"enabled":false}`))
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	if recorder.Code != 401 {
		t.Fatal("cookie-only mutation accepted")
	}
	r.space("admin").app.GetDatabase().Create(&models.ApiKey{ID: "account-test-key", Name: "account-test-key", Key: "not-a-user-jwt", Type: "admin", Enabled: true})
	if got := requestWorkspace(t, e, "GET", "/api/admin/accounts", "not-a-user-jwt", ""); got.Code != 401 {
		t.Fatal("API key can manage accounts")
	}
	if got := requestWorkspace(t, e, "GET", "/api/auth/config", "expired-token", ""); got.Code != 200 {
		t.Fatal("stale token prevents login config")
	}
}

func TestConcurrentAccountCreationAndRequests(t *testing.T) {
	r, e := accountTestRouter(t)
	admin, _ := loginWorkspace(t, e, "admin")
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- requestWorkspace(t, e, "POST", "/api/admin/accounts", admin, `{"username":"concurrent","password":"workspace-test-password"}`).Code
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requestWorkspace(t, e, "GET", "/api/agents", admin, "")
			r.keyWorkspace(t.Context(), "missing-key", "agent")
		}()
	}
	wg.Wait()
	close(statuses)
	seen := map[int]int{}
	for status := range statuses {
		seen[status]++
	}
	if seen[201] != 1 || seen[409] != 1 {
		t.Fatalf("duplicate publication: %v", seen)
	}
}
