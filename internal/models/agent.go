package models

import (
	"net"
	"strings"

	"gorm.io/datatypes"
)

// Agent 探针信息
type Agent struct {
	ID         string                      `gorm:"primaryKey" json:"id"`                       // 探针ID (UUID)
	Name       string                      `gorm:"index" json:"name"`                          // 探针名称
	Hostname   string                      `gorm:"index" json:"hostname,omitempty"`            // 主机名
	IP         string                      `gorm:"index" json:"ip,omitempty"`                  // 连接 IP 地址
	IPv4       string                      `gorm:"index" json:"ipv4,omitempty"`                // 公网 IPv4 地址
	IPv6       string                      `gorm:"index" json:"ipv6,omitempty"`                // 公网 IPv6 地址
	OS         string                      `json:"os"`                                         // 操作系统
	Arch       string                      `json:"arch"`                                       // 架构
	Version    string                      `json:"version"`                                    // 探针版本
	Tags       datatypes.JSONSlice[string] `json:"tags"`                                       // 标签
	ExpireTime int64                       `json:"expireTime"`                                 // 到期时间（时间戳毫秒）
	Enabled    bool                        `gorm:"default:true;not null;index" json:"enabled"` // 是否接收并处理探针数据
	Status     int                         `json:"status"`                                     // 状态: 0-离线, 1-在线
	Visibility string                      `gorm:"default:private" json:"visibility"`           // 可见性: public-匿名可见, private-登录可见
	Weight     int                         `gorm:"default:0;index" json:"weight"`              // 展示顺序（由拖动排序维护，数字越大越靠前）
	Remark     string                      `json:"remark"`                                     // 备注信息
	LastSeenAt int64                       `gorm:"index" json:"lastSeenAt"`                    // 最后上线时间（时间戳毫秒）
	CreatedAt  int64                       `json:"createdAt"`                                  // 创建时间（时间戳毫秒）
	UpdatedAt  int64                       `json:"updatedAt" gorm:"autoUpdateTime:milli"`      // 更新时间（时间戳毫秒）

	// 流量统计相关字段
	TrafficStats datatypes.JSONType[TrafficStatsData] `json:"trafficStats,omitempty"` // 流量统计

	// 防篡改保护配置
	TamperProtectConfig datatypes.JSONType[TamperProtectConfigData] `json:"tamperProtectConfig,omitempty"` // 防篡改保护配置

	// SSH登录监控配置
	SSHLoginConfig datatypes.JSONType[SSHLoginConfigData] `json:"sshLoginConfig,omitempty"` // SSH登录监控配置
}

// TrafficStatsData 流量统计数据
type TrafficStatsData struct {
	Enabled      bool   `json:"enabled"`      // 是否启用
	Type         string `json:"type"`         // 统计类型: "recv"进站, "send"出站, "both"全部
	Limit        uint64 `json:"limit"`        // 流量限额(字节), 0表示不限制
	Used         uint64 `json:"used"`         // 当前周期已使用流量(字节)
	UsedOffset   uint64 `json:"usedOffset"`   // 基线调整使用偏移(字节)
	ResetDay     int    `json:"resetDay"`     // 流量重置日期(1-31), 0表示不自动重置
	PeriodStart  int64  `json:"periodStart"`  // 当前周期开始时间(时间戳毫秒)
	BaselineRecv uint64 `json:"baselineRecv"` // 当前周期流量基线(BytesRecvTotal)
	BaselineSend uint64 `json:"baselineSend"` // 当前周期流量基线(BytesSentTotal)
	AlertSent80  bool   `json:"alertSent80"`  // 是否已发送80%告警
	AlertSent90  bool   `json:"alertSent90"`  // 是否已发送90%告警
	AlertSent100 bool   `json:"alertSent100"` // 是否已发送100%告警
}

// TamperProtectConfigData 防篡改保护配置数据
type TamperProtectConfigData struct {
	Enabled      bool     `json:"enabled"`                // 是否启用
	Paths        []string `json:"paths"`                  // 受保护的目录列表
	ApplyStatus  string   `json:"applyStatus,omitempty"`  // 配置应用状态: success/failed/pending
	ApplyMessage string   `json:"applyMessage,omitempty"` // 应用结果消息
}

// SSHLoginConfigData SSH登录监控配置数据
type SSHLoginConfigData struct {
	Enabled      bool     `json:"enabled"`                // 是否启用
	IPWhitelist  []string `json:"ipWhitelist,omitempty"`  // IP白名单，白名单中的IP只记录不发送通知，支持 IP 或 CIDR
	ApplyStatus  string   `json:"applyStatus,omitempty"`  // 配置应用状态: success/failed/pending
	ApplyMessage string   `json:"applyMessage,omitempty"` // 应用结果消息
}

func (r SSHLoginConfigData) IsIPWhitelisted(ip string) bool {
	if len(r.IPWhitelist) == 0 {
		return false
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, whitelistIP := range r.IPWhitelist {
		if whitelistIP == ip {
			return true
		}
		// 检查是否为 CIDR 格式
		if strings.Contains(whitelistIP, "/") {
			_, ipNet, err := net.ParseCIDR(whitelistIP)
			if err == nil && ipNet.Contains(parsedIP) {
				return true
			}
		}
	}
	return false
}

func (Agent) TableName() string {
	return "agents"
}
