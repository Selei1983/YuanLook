package handler

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-orz/orz"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/assets"
	"github.com/pika-monitor/pika/internal/service"
	"github.com/pika-monitor/pika/pkg/version"
)

type WebHandler struct {
	themeService    *service.ThemeService
	propertyService *service.PropertyService
}

func NewWebHandler(themeService *service.ThemeService, propertyService *service.PropertyService) *WebHandler {
	return &WebHandler{themeService: themeService, propertyService: propertyService}
}

func (h *WebHandler) PublicConfig(c *echo.Context) error {
	runtime, err := h.runtimeConfig(c)
	if err != nil {
		return err
	}
	return orz.Ok(c, runtime)
}

func (h *WebHandler) ServeThemeAsset(c *echo.Context) error {
	_, dist, err := h.themeService.ActiveDistDir(c.Request().Context())
	if err != nil {
		return err
	}
	return serveStaticFile(c, dist, c.Param("*"), false)
}

func (h *WebHandler) ServeSPA(c *echo.Context) error {
	if c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	path := c.Request().URL.Path
	if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/ws") {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	if isAdminPage(path) {
		return h.serveAdminIndex(c)
	}
	if path != "/" {
		_, dist, err := h.themeService.ActiveDistDir(c.Request().Context())
		if err != nil {
			return err
		}
		if full, ok := resolveStaticPath(dist, strings.TrimPrefix(path, "/")); ok {
			return serveResolvedStaticFile(c, full, true)
		}
	}
	if filepath.Ext(path) != "" {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	return h.serveThemeIndex(c)
}

func isAdminPage(path string) bool {
	return path == "/admin" || strings.HasPrefix(path, "/admin/")
}

func (h *WebHandler) serveAdminIndex(c *echo.Context) error {
	root := assets.WebDir()
	path := filepath.Join(root, "index.html")
	if stat, err := os.Stat(path); err != nil || !stat.Mode().IsRegular() {
		return echo.NewHTTPError(http.StatusInternalServerError, "官方管理前端缺失")
	}
	setHTMLHeaders(c)
	return c.FileFS("index.html", os.DirFS(root))
}

// ImmutableStaticHeaders 为带内容摘要文件名的前端构建产物设置长期缓存和安全响应头。
func ImmutableStaticHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")
		c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return next(c)
	}
}

func (h *WebHandler) serveThemeIndex(c *echo.Context) error {
	systemConfig, err := h.propertyService.GetSystemConfig(c.Request().Context())
	if err != nil {
		return err
	}
	runtime, err := h.runtimeConfig(c)
	if err != nil {
		return err
	}
	data, err := h.themeService.RenderIndex(c.Request().Context(), runtime, systemConfig)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "公开主题入口不可用")
	}
	setHTMLHeaders(c)
	return c.Blob(http.StatusOK, "text/html; charset=utf-8", data)
}

func (h *WebHandler) runtimeConfig(c *echo.Context) (map[string]any, error) {
	ctx := c.Request().Context()
	systemConfig, err := h.propertyService.GetSystemConfig(ctx)
	if err != nil {
		return nil, err
	}
	appearance, err := h.propertyService.GetAppearanceConfig(ctx)
	if err != nil {
		return nil, err
	}
	theme, err := h.themeService.Active(ctx)
	if err != nil {
		return nil, err
	}
	legacy := map[string]any{
		"SystemNameZh": systemConfig.SystemNameZh,
		"SystemNameEn": systemConfig.SystemNameEn,
		"ICPCode":      systemConfig.ICPCode,
		"DefaultView":  systemConfig.DefaultView,
		"Version":      version.Version,
	}
	return map[string]any{
		"apiVersion": service.ThemeAPIVersion,
		"system": map[string]any{
			"nameZh":           systemConfig.SystemNameZh,
			"nameEn":           systemConfig.SystemNameEn,
			"logo":             "/api/logo",
			"icpCode":          systemConfig.ICPCode,
			"version":          version.Version,
			"defaultView":      systemConfig.DefaultView,
			"defaultColorMode": appearance.DefaultColorMode,
		},
		"theme": map[string]any{
			"id":      theme.ID,
			"version": theme.Version,
		},
		"features": map[string]bool{
			"serverList": true, "serverDetail": true,
			"monitorList": true, "monitorDetail": true,
		},
		"legacySystemConfig": legacy,
	}, nil
}

func serveStaticFile(c *echo.Context, root, requested string, immutable bool) error {
	full, ok := resolveStaticPath(root, requested)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	return serveResolvedStaticFile(c, full, immutable)
}

func resolveStaticPath(root, requested string) (string, bool) {
	requested = strings.TrimPrefix(requested, "/")
	if requested == "" || strings.Contains(requested, "\\") {
		return "", false
	}
	clean := filepath.Clean(requested)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", false
	}
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	stat, err := os.Lstat(full)
	if err != nil || !stat.Mode().IsRegular() {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	realFile, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", false
	}
	realRel, err := filepath.Rel(realRoot, realFile)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return realFile, true
}

func serveResolvedStaticFile(c *echo.Context, full string, immutable bool) error {
	file, err := os.Open(full)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return echo.NewHTTPError(http.StatusNotFound, "Not Found")
	}
	contentType := mime.TypeByExtension(filepath.Ext(full))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if immutable && c.Response().Header().Get("Cache-Control") == "" {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeContent(c.Response(), c.Request(), stat.Name(), stat.ModTime(), file)
	return nil
}

func setHTMLHeaders(c *echo.Context) {
	c.Response().Header().Set("Cache-Control", "private, no-store")
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
}
