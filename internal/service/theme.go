package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pika-monitor/pika/internal/assets"
	"github.com/pika-monitor/pika/internal/config"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/pkg/version"
	"go.uber.org/zap"
	"golang.org/x/text/unicode/norm"
)

// ThemeManifest 是 pika-theme.json 的结构，只含展示用的关键信息。
type ThemeManifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	Author        string   `json:"author"`
	Homepage      string   `json:"homepage"`
	License       string   `json:"license"`
	Preview       string   `json:"preview"`
	Entry         string   `json:"entry"`
	APIVersion    string   `json:"apiVersion"`
	Capabilities  []string `json:"capabilities"`
}

// AppearanceConfig 记录当前启用主题和默认明暗模式，存放在 appearance_config Property。
type AppearanceConfig struct {
	ActiveTheme      string `json:"activeTheme"`
	DefaultColorMode string `json:"defaultColorMode"`
}

type ThemeInfo struct {
	ThemeManifest
	Active             bool   `json:"active"`
	Official           bool   `json:"official"`
	Compatible         bool   `json:"compatible"`
	CompatibilityError string `json:"compatibilityError"`
	PreviewURL         string `json:"previewUrl"`
}

type ThemeInstallResult struct {
	Theme  ThemeInfo `json:"theme"`
	SHA256 string    `json:"sha256"`
}

const (
	DefaultThemeID       = "default"
	ThemeSchemaVersion   = 1
	ThemeAPIVersion      = "v1"
	maxThemeArchiveSize  = 64 << 20
	maxThemeFiles        = 5000
	maxThemeFileSize     = 64 << 20
	maxThemeExtracted    = 256 << 20
	maxThemeManifestSize = 1 << 20
	maxThemePreviewSize  = 5 << 20
	maxThemeIndexSize    = 8 << 20
)

var (
	ErrThemeNotFound = errors.New("主题不存在")
	ErrThemeExists   = errors.New("主题已存在")
	ErrThemeActive   = errors.New("当前主题不能删除")
	ErrDefaultTheme  = errors.New("默认主题不能修改或删除")
	themeIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	reservedThemeIDs = map[string]struct{}{
		"default": {}, "admin": {}, "official": {}, "system": {},
		"api": {}, "assets": {}, "t": {}, "theme-assets": {},
	}
	requiredThemeCaps = []string{"server-list", "server-detail", "monitor-list", "monitor-detail"}
	titleElement      = regexp.MustCompile(`(?is)<title(?:\s[^>]*)?>.*?</title>`)
	descriptionMeta   = regexp.MustCompile(`(?is)<meta\s+[^>]*name\s*=\s*["']description["'][^>]*>`)
	iconLink          = regexp.MustCompile(`(?is)<link\s+[^>]*rel\s*=\s*["'][^"']*icon[^"']*["'][^>]*>`)
	headEnd           = regexp.MustCompile(`(?i)</head\s*>`)
	bodyEnd           = regexp.MustCompile(`(?i)</body\s*>`)
)

// ThemeService 通过扫描本地主题目录提供主题的列表、安装、启用、删除和入口 HTML 渲染。
// 主题数据全部存放在文件系统，不经过数据库。
type ThemeService struct {
	logger          *zap.Logger
	propertyService *PropertyService
	themeDir        string
	defaultThemeDir string
	mu              sync.RWMutex
	renderMu        sync.RWMutex
	renderCacheKey  string
	renderCache     []byte
	fallbackMu      sync.Mutex
}

func NewThemeService(logger *zap.Logger, propertyService *PropertyService, cfg *config.AppConfig) (*ThemeService, error) {
	themeCfg := &config.ThemeConfig{}
	if cfg != nil && cfg.Theme != nil {
		themeCfg = cfg.Theme
	}
	themeDir := strings.TrimSpace(os.Getenv("PIKA_THEME_DIR"))
	if cfg != nil && cfg.WorkspaceThemeDir != "" {
		themeDir = cfg.WorkspaceThemeDir
	}
	if themeDir == "" {
		themeDir = strings.TrimSpace(themeCfg.Dir)
	}
	if themeDir == "" {
		themeDir = filepath.Join("data", "themes")
	}
	s := &ThemeService{
		logger:          logger,
		propertyService: propertyService,
		themeDir:        themeDir,
		defaultThemeDir: assets.DefaultThemeDir(),
	}
	if err := os.MkdirAll(filepath.Join(themeDir, ".staging"), 0755); err != nil {
		return nil, fmt.Errorf("创建主题目录失败: %w", err)
	}
	if err := validateOfficialWebBuild(assets.WebDir(), s.defaultThemeDir); err != nil {
		return nil, err
	}
	if err := s.recoverFilesystem(); err != nil {
		logger.Warn("主题目录启动恢复存在异常", zap.Error(err))
	}
	return s, nil
}

func (s *ThemeService) ActiveThemeID(ctx context.Context) string {
	appearance, err := s.propertyService.GetAppearanceConfig(ctx)
	if err != nil || appearance.ActiveTheme == "" {
		return DefaultThemeID
	}
	return appearance.ActiveTheme
}

func (s *ThemeService) Active(ctx context.Context) (*ThemeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id := s.ActiveThemeID(ctx)
	info, err := s.getUnlocked(ctx, id)
	if err == nil && info.Compatible {
		return info, nil
	}
	defaultInfo, defaultErr := s.getUnlocked(ctx, DefaultThemeID)
	if defaultErr != nil {
		return nil, fmt.Errorf("当前主题 %q 不可用且默认主题回退失败: %w", id, defaultErr)
	}
	if id != DefaultThemeID {
		s.repairActiveTheme(ctx, id, err, info)
	}
	return defaultInfo, nil
}

func (s *ThemeService) repairActiveTheme(ctx context.Context, brokenID string, loadErr error, info *ThemeInfo) {
	s.fallbackMu.Lock()
	defer s.fallbackMu.Unlock()
	if s.ActiveThemeID(ctx) != brokenID {
		return
	}
	appearance, err := s.propertyService.GetAppearanceConfig(ctx)
	if err != nil {
		s.logger.Error("读取损坏主题的外观配置失败", zap.String("theme", brokenID), zap.Error(err))
		return
	}
	appearance.ActiveTheme = DefaultThemeID
	if err := s.propertyService.SetAppearanceConfig(ctx, *appearance); err != nil {
		s.logger.Error("自动回退默认主题失败", zap.String("theme", brokenID), zap.Error(err))
		return
	}
	reason := loadErr
	if reason == nil && info != nil && info.CompatibilityError != "" {
		reason = errors.New(info.CompatibilityError)
	}
	s.logger.Warn("活动主题不可用，已自动回退默认主题", zap.String("theme", brokenID), zap.Error(reason))
	s.invalidateRenderCache()
}

func (s *ThemeService) List(ctx context.Context) ([]ThemeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active := s.ActiveThemeID(ctx)
	items := make([]ThemeInfo, 0)
	info, err := s.infoFromRoot(DefaultThemeID, s.defaultThemeDir, true, active)
	if err != nil {
		return nil, fmt.Errorf("默认主题无效: %w", err)
	}
	items = append(items, *info)
	entries, err := os.ReadDir(s.themeDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		item, loadErr := s.infoFromRoot(entry.Name(), filepath.Join(s.themeDir, entry.Name()), false, active)
		if loadErr != nil {
			s.logger.Warn("忽略无效主题目录", zap.String("theme", entry.Name()), zap.Error(loadErr))
			continue
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *ThemeService) Get(ctx context.Context, id string) (*ThemeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getUnlocked(ctx, id)
}

func (s *ThemeService) getUnlocked(ctx context.Context, id string) (*ThemeInfo, error) {
	if id != DefaultThemeID && !validThemeID(id) {
		return nil, ErrThemeNotFound
	}
	root := s.themeRoot(id)
	if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
		return nil, ErrThemeNotFound
	}
	return s.infoFromRoot(id, root, id == DefaultThemeID, s.ActiveThemeID(ctx))
}

func (s *ThemeService) infoFromRoot(id, root string, official bool, active string) (*ThemeInfo, error) {
	manifest, err := loadThemeManifest(filepath.Join(root, "pika-theme.json"))
	if err != nil {
		return nil, err
	}
	if manifest.ID != id {
		return nil, fmt.Errorf("清单 id %q 与主题目录 %q 不一致", manifest.ID, id)
	}
	compatErr := validateThemeManifest(manifest, root)
	info := &ThemeInfo{
		ThemeManifest: *manifest,
		Active:        active == id,
		Official:      official,
		Compatible:    compatErr == nil,
		PreviewURL:    "/api/admin/themes/" + id + "/preview",
	}
	if compatErr != nil {
		info.CompatibilityError = compatErr.Error()
	}
	return info, nil
}

func (s *ThemeService) Install(ctx context.Context, reader io.Reader) (*ThemeInstallResult, error) {
	data, hash, err := readThemeArchive(reader)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.installArchiveLocked(ctx, data, "", false)
	if err != nil {
		return nil, err
	}
	return &ThemeInstallResult{Theme: *info, SHA256: hash}, nil
}

func (s *ThemeService) installArchiveLocked(ctx context.Context, data []byte, expectedID string, replace bool) (*ThemeInfo, error) {
	stage := filepath.Join(s.themeDir, ".staging", uuid.NewString())
	unpacked := filepath.Join(stage, "unpacked")
	if err := os.MkdirAll(unpacked, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if err := extractThemeArchive(data, unpacked); err != nil {
		return nil, err
	}
	manifest, err := loadThemeManifest(filepath.Join(unpacked, "pika-theme.json"))
	if err != nil {
		return nil, err
	}
	if expectedID != "" && manifest.ID != expectedID {
		return nil, errors.New("更新包的主题 id 与目标主题不一致")
	}
	if manifest.ID == DefaultThemeID {
		return nil, ErrDefaultTheme
	}
	if err := validateThemeManifest(manifest, unpacked); err != nil {
		return nil, err
	}
	if existing, found, err := s.findInstalledThemeID(manifest.ID); err != nil {
		return nil, err
	} else if found && (!replace || existing != manifest.ID) {
		return nil, ErrThemeExists
	}
	destination := filepath.Join(s.themeDir, manifest.ID)
	if _, err := os.Stat(destination); err == nil && !replace {
		return nil, ErrThemeExists
	}
	backup := ""
	if replace {
		if _, err := os.Stat(destination); err != nil {
			return nil, ErrThemeNotFound
		}
		backup = destination + ".backup-" + uuid.NewString()
		if err := os.Rename(destination, backup); err != nil {
			return nil, fmt.Errorf("备份旧主题失败: %w", err)
		}
	}
	if err := os.Rename(unpacked, destination); err != nil {
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return nil, fmt.Errorf("安装主题失败: %w", err)
	}
	installedInfo, err := s.infoFromRoot(manifest.ID, destination, false, s.ActiveThemeID(ctx))
	if err != nil {
		_ = os.RemoveAll(destination)
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return nil, fmt.Errorf("安装后健康检查失败: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	s.invalidateRenderCache()
	return installedInfo, nil
}

func (s *ThemeService) findInstalledThemeID(id string) (string, bool, error) {
	entries, err := os.ReadDir(s.themeDir)
	if err != nil {
		return "", false, err
	}
	wanted := strings.ToLower(norm.NFC.String(id))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.Contains(entry.Name(), ".backup-") {
			continue
		}
		if strings.ToLower(norm.NFC.String(entry.Name())) == wanted {
			return entry.Name(), true, nil
		}
	}
	return "", false, nil
}

func (s *ThemeService) Activate(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := s.getUnlocked(ctx, id)
	if err != nil {
		return err
	}
	if !info.Compatible {
		return errors.New(info.CompatibilityError)
	}
	appearance, err := s.propertyService.GetAppearanceConfig(ctx)
	if err != nil {
		return err
	}
	appearance.ActiveTheme = id
	if err := s.propertyService.SetAppearanceConfig(ctx, *appearance); err != nil {
		return err
	}
	s.invalidateRenderCache()
	return nil
}

func (s *ThemeService) Delete(ctx context.Context, id string) error {
	if id == DefaultThemeID {
		return ErrDefaultTheme
	}
	if !validThemeID(id) {
		return ErrThemeNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveThemeID(ctx) == id {
		return ErrThemeActive
	}
	root := filepath.Join(s.themeDir, id)
	if _, err := os.Stat(root); err != nil {
		return ErrThemeNotFound
	}
	trash := root + ".delete-" + uuid.NewString()
	if err := os.Rename(root, trash); err != nil {
		return err
	}
	if err := os.RemoveAll(trash); err != nil {
		return err
	}
	s.invalidateRenderCache()
	return nil
}

func (s *ThemeService) ActiveDistDir(ctx context.Context) (string, string, error) {
	info, err := s.Active(ctx)
	if err != nil {
		return "", "", err
	}
	return info.ID, filepath.Join(s.themeRoot(info.ID), "dist"), nil
}

func (s *ThemeService) PreviewPath(ctx context.Context, id string) (string, string, error) {
	info, err := s.Get(ctx, id)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(s.themeRoot(id), filepath.Clean(info.Preview))
	return path, mime.TypeByExtension(filepath.Ext(path)), nil
}

func (s *ThemeService) themeRoot(id string) string {
	if id == DefaultThemeID {
		return s.defaultThemeDir
	}
	return filepath.Join(s.themeDir, id)
}

func (s *ThemeService) RenderIndex(ctx context.Context, runtime any, systemConfig *models.SystemConfig) ([]byte, error) {
	_, dist, err := s.ActiveDistDir(ctx)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dist, "index.html"))
	if err != nil {
		return nil, err
	}
	runtimeJSON, err := json.Marshal(runtime)
	if err != nil {
		return nil, err
	}
	customCSS, customJS := "", ""
	if systemConfig != nil {
		customCSS = systemConfig.CustomCSS
		customJS = systemConfig.CustomJS
	}
	cacheHash := sha256.New()
	cacheHash.Write(data)
	cacheHash.Write(runtimeJSON)
	io.WriteString(cacheHash, "\x00"+customCSS+"\x00"+customJS+"\x00"+version.Version)
	cacheKey := hex.EncodeToString(cacheHash.Sum(nil))
	s.renderMu.RLock()
	if s.renderCacheKey == cacheKey && s.renderCache != nil {
		cached := append([]byte(nil), s.renderCache...)
		s.renderMu.RUnlock()
		return cached, nil
	}
	s.renderMu.RUnlock()

	injection := `<script id="pika-runtime-config">window.PikaRuntime=` + string(runtimeJSON) + `;window.SystemConfig=window.PikaRuntime.legacySystemConfig;</script>`
	if customCSS != "" {
		injection += `<style id="pika-custom-css">` + customCSS + `</style>`
	}
	htmlText := string(data)
	htmlText = applySystemMetadata(htmlText, systemConfig)
	if strings.Contains(htmlText, "<!-- pika:head -->") {
		htmlText = strings.Replace(htmlText, "<!-- pika:head -->", injection, 1)
	} else {
		htmlText = replaceBeforeFirst(htmlText, headEnd, injection)
	}
	if customJS != "" {
		script := `<script id="pika-custom-js">` + customJS + `</script>`
		if strings.Contains(htmlText, "<!-- pika:body -->") {
			htmlText = strings.Replace(htmlText, "<!-- pika:body -->", script, 1)
		} else {
			htmlText = replaceBeforeFirst(htmlText, bodyEnd, script)
		}
	}
	rendered := []byte(htmlText)
	s.renderMu.Lock()
	s.renderCacheKey = cacheKey
	s.renderCache = append(s.renderCache[:0], rendered...)
	s.renderMu.Unlock()
	return append([]byte(nil), rendered...), nil
}

func applySystemMetadata(htmlText string, systemConfig *models.SystemConfig) string {
	if systemConfig == nil {
		return htmlText
	}
	nameZh := strings.TrimSpace(systemConfig.SystemNameZh)
	nameEn := strings.TrimSpace(systemConfig.SystemNameEn)
	title := nameZh
	if title == "" {
		title = nameEn
	} else if nameEn != "" && nameEn != nameZh {
		title += " | " + nameEn
	}
	if title == "" {
		title = "Pika Monitor"
	}
	titleTag := "<title>" + html.EscapeString(title) + "</title>"
	description := html.EscapeString(title + " - 服务器状态与监控")
	descriptionTag := `<meta name="description" content="` + description + `">`
	iconTag := `<link rel="icon" href="/api/logo">`
	if titleElement.MatchString(htmlText) {
		htmlText = titleElement.ReplaceAllStringFunc(htmlText, func(string) string { return titleTag })
	} else {
		htmlText = replaceBeforeFirst(htmlText, headEnd, titleTag)
	}
	if descriptionMeta.MatchString(htmlText) {
		htmlText = descriptionMeta.ReplaceAllStringFunc(htmlText, func(string) string { return descriptionTag })
	} else {
		htmlText = replaceBeforeFirst(htmlText, headEnd, descriptionTag)
	}
	if iconLink.MatchString(htmlText) {
		htmlText = iconLink.ReplaceAllStringFunc(htmlText, func(string) string { return iconTag })
	} else {
		htmlText = replaceBeforeFirst(htmlText, headEnd, iconTag)
	}
	return htmlText
}

func (s *ThemeService) invalidateRenderCache() {
	s.renderMu.Lock()
	s.renderCacheKey = ""
	s.renderCache = nil
	s.renderMu.Unlock()
}

func (s *ThemeService) recoverFilesystem() error {
	var recoveryErrors []error
	staging := filepath.Join(s.themeDir, ".staging")
	entries, err := os.ReadDir(staging)
	if err == nil {
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, entry := range entries {
			info, infoErr := entry.Info()
			if infoErr == nil && info.ModTime().Before(cutoff) {
				_ = os.RemoveAll(filepath.Join(staging, entry.Name()))
			}
		}
	}
	entries, err = os.ReadDir(s.themeDir)
	if err != nil {
		return err
	}
	backups := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		marker := strings.Index(name, ".backup-")
		if marker <= 0 || !entry.IsDir() {
			continue
		}
		id := name[:marker]
		backups[id] = append(backups[id], filepath.Join(s.themeDir, name))
	}
	for id, candidates := range backups {
		official := filepath.Join(s.themeDir, id)
		if themeRootValid(id, official) {
			for _, candidate := range candidates {
				if err := os.RemoveAll(candidate); err != nil {
					recoveryErrors = append(recoveryErrors, fmt.Errorf("清理主题 %s 旧备份失败: %w", id, err))
				}
			}
			continue
		}
		validBackups := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			if themeRootValid(id, candidate) {
				validBackups = append(validBackups, candidate)
			}
		}
		if len(validBackups) != 1 || len(candidates) != 1 {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("主题 %s 的备份状态不明确，已保留全部目录", id))
			continue
		}
		if _, statErr := os.Stat(official); statErr == nil {
			broken := filepath.Join(s.themeDir, ".broken-"+id+"-"+uuid.NewString())
			if err := os.Rename(official, broken); err != nil {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("隔离主题 %s 损坏目录失败: %w", id, err))
				continue
			}
		}
		if err := os.Rename(validBackups[0], official); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("恢复主题 %s 备份失败: %w", id, err))
		}
	}
	return errors.Join(recoveryErrors...)
}

func themeRootValid(id, root string) bool {
	manifest, err := loadThemeManifest(filepath.Join(root, "pika-theme.json"))
	return err == nil && manifest.ID == id && validateThemeManifest(manifest, root) == nil
}
