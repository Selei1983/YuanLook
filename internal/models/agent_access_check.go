package models

// AgentAccessCheck is the latest panel-to-device ICMP sample. It is stored in
// each workspace database, separately from agent heartbeats and configuration.
type AgentAccessCheck struct {
	AgentID   string   `gorm:"primaryKey" json:"-"`
	Target    string   `json:"target"`
	Status    string   `json:"status"` // checking, success, timeout, error
	LatencyMS *float64 `json:"latencyMs"`
	CheckedAt int64    `json:"checkedAt"`
}
