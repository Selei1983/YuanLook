package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestAdminPageRoutingBoundary(t *testing.T) {
	for _, path := range []string{"/admin", "/admin/login", "/admin/github/callback", "/admin/oidc/callback", "/admin/agents"} {
		if !isAdminPage(path) {
			t.Errorf("expected admin route: %s", path)
		}
	}
	for _, path := range []string{"/", "/login", "/github/callback", "/oidc/callback", "/servers/id", "/monitors/id", "/administrator", "/api/admin/themes"} {
		if isAdminPage(path) {
			t.Errorf("public/API route classified as admin page: %s", path)
		}
	}
}

func TestServeSPARejectsAPIWebSocketAssetsAndNonGET(t *testing.T) {
	handler := &WebHandler{}
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/unknown"},
		{http.MethodGet, "/ws/unknown"},
		{http.MethodPost, "/servers/1"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		context := echo.New().NewContext(request, recorder)
		err := handler.ServeSPA(context)
		httpError, ok := err.(*echo.HTTPError)
		if !ok || httpError.Code != http.StatusNotFound {
			t.Errorf("%s %s should return 404, got %v", test.method, test.path, err)
		}
	}
}

func TestResolveStaticPathAndHeaders(t *testing.T) {
	root := t.TempDir()
	asset := filepath.Join(root, "assets", "app.js")
	if err := os.MkdirAll(filepath.Dir(asset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, ok := resolveStaticPath(root, "assets/app.js"); !ok || resolved == "" {
		t.Fatal("valid static asset was rejected")
	}
	for _, path := range []string{"", "../secret", "assets\\app.js", "/../../secret"} {
		if _, ok := resolveStaticPath(root, path); ok {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}

	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.js")
		if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "link.js")); err != nil {
			t.Fatal(err)
		}
		if _, ok := resolveStaticPath(root, "link.js"); ok {
			t.Fatal("symlink asset was accepted")
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/assets/app.js", nil)
	context := echo.New().NewContext(request, recorder)
	if err := serveStaticFile(context, root, "assets/app.js", true); err != nil {
		t.Fatal(err)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" ||
		recorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		recorder.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected static headers: %#v", recorder.Header())
	}
}

func TestEchoWildcardRoutesKeepAdminAndAPISeparated(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "assets", "app.js"), []byte("admin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<html>admin</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIKA_WEB_DIR", webRoot)

	e := echo.New()
	handler := &WebHandler{}
	e.Static("/admin/assets/", filepath.Join(webRoot, "assets"), ImmutableStaticHeaders)
	e.GET("/*", handler.ServeSPA)

	assetRecorder := httptest.NewRecorder()
	e.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/admin/assets/app.js", nil))
	if assetRecorder.Code != http.StatusOK || assetRecorder.Body.String() != "admin" {
		t.Fatalf("wildcard asset route failed: status=%d body=%q", assetRecorder.Code, assetRecorder.Body.String())
	}
	if assetRecorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("admin asset missing immutable cache header: %#v", assetRecorder.Header())
	}

	adminRecorder := httptest.NewRecorder()
	e.ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/admin/agents", nil))
	if adminRecorder.Code != http.StatusOK || adminRecorder.Body.String() != "<html>admin</html>" {
		t.Fatalf("admin SPA route failed: status=%d body=%q", adminRecorder.Code, adminRecorder.Body.String())
	}

	apiRecorder := httptest.NewRecorder()
	e.ServeHTTP(apiRecorder, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))
	if apiRecorder.Code != http.StatusNotFound || apiRecorder.Header().Get("Content-Type") == "text/html; charset=utf-8" {
		t.Fatalf("unknown API entered SPA fallback: status=%d type=%q", apiRecorder.Code, apiRecorder.Header().Get("Content-Type"))
	}
}

func TestPrivateThemeAssetDoesNotOverrideNoStore(t *testing.T) {
	asset := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(asset, []byte("theme"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/assets/app.js", nil), res)
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if err := serveResolvedStaticFile(c, asset, true); err != nil {
		t.Fatal(err)
	}
	if res.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private theme asset became publicly cacheable")
	}
}
