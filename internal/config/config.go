package config

import (
	"fmt"
	"strings"
)

// AppConfig 应用配置
type AppConfig struct {
	WorkspaceThemeDir string             `json:"-"` // Internal override; never shared between workspaces.
	Workspaces        *WorkspaceConfig   `json:"Workspaces"`
	JWT               JWTConfig          `json:"JWT"`
	Users             map[string]string  `json:"Users"`           // 用户名 -> bcrypt加密的密码
	OIDC              *OIDCConfig        `json:"OIDC"`            // OIDC配置（可选）
	GitHub            *GitHubOAuthConfig `json:"GitHub"`          // GitHub OAuth配置（可选）
	GeoIP             *GeoIPConfig       `json:"GeoIP"`           // GeoIP配置（可选）
	VictoriaMetrics   *VMConfig          `json:"VictoriaMetrics"` // VictoriaMetrics配置（可选）
	Theme             *ThemeConfig       `json:"Theme"`           // 主题系统配置（可选）
}

// ThemeConfig 可安装主题系统配置。
type ThemeConfig struct {
	Dir string `json:"Dir"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret       string `json:"Secret"`
	ExpiresHours int    `json:"ExpiresHours"`
}

// OIDCConfig OIDC认证配置
type OIDCConfig struct {
	Enabled      bool   `json:"Enabled"`      // 是否启用OIDC
	Issuer       string `json:"Issuer"`       // OIDC Provider的Issuer URL
	ClientID     string `json:"ClientID"`     // Client ID
	ClientSecret string `json:"ClientSecret"` // Client Secret
	RedirectURL  string `json:"RedirectURL"`  // 回调URL
}

// Validate 校验启用 OIDC 所需的静态配置。
// Provider 是否可访问属于运行时状态，不应在应用启动阶段参与配置有效性判断。
func (c *OIDCConfig) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}

	missing := make([]string, 0, 4)
	if strings.TrimSpace(c.Issuer) == "" {
		missing = append(missing, "Issuer")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		missing = append(missing, "ClientID")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		missing = append(missing, "ClientSecret")
	}
	if strings.TrimSpace(c.RedirectURL) == "" {
		missing = append(missing, "RedirectURL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("缺少必要配置: %s", strings.Join(missing, ", "))
	}

	return nil
}

// GitHubOAuthConfig GitHub OAuth认证配置
type GitHubOAuthConfig struct {
	Enabled      bool     `json:"Enabled"`      // 是否启用GitHub登录
	ClientID     string   `json:"ClientID"`     // GitHub OAuth App Client ID
	ClientSecret string   `json:"ClientSecret"` // GitHub OAuth App Client Secret
	RedirectURL  string   `json:"RedirectURL"`  // 回调URL
	AllowedUsers []string `json:"AllowedUsers"` // 允许登录的GitHub用户名白名单（为空则允许所有用户）
}

// GeoIPConfig GeoIP配置
type GeoIPConfig struct {
	Enabled    bool   `json:"Enabled"`    // 是否启用GeoIP查询
	DBPath     string `json:"DBPath"`     // GeoIP数据库文件路径（如：GeoLite2-City.mmdb）
	DBLanguage string `json:"DBLanguage"` // 数据库语言（如：zh-CN、en）
}

// VMConfig VictoriaMetrics配置
type VMConfig struct {
	Username      string `json:"Username"`
	Password      string `json:"Password"`
	Enabled       bool   `json:"Enabled"`       // 是否启用VictoriaMetrics
	URL           string `json:"URL"`           // VictoriaMetrics地址
	RetentionDays int    `json:"RetentionDays"` // 数据保留天数（用于文档说明）
	WriteTimeout  int    `json:"WriteTimeout"`  // 写入超时（秒）
	QueryTimeout  int    `json:"QueryTimeout"`  // 查询超时（秒）
}

// WorkspaceConfig binds the original database to a single account. Other accounts
// receive independent SQLite databases and filesystem directories.
type WorkspaceConfig struct {
	// Nil defaults to open registration. Set false to allow administrator-created accounts only.
	RegistrationEnabled *bool  `json:"RegistrationEnabled"`
	LegacyOwner         string `json:"LegacyOwner"`
	Dir                 string `json:"Dir"`
}
