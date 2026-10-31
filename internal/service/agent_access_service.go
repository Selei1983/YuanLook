package service

import (
	"context"
	"net/netip"
	"strings"
	"time"

	"github.com/pika-monitor/pika/internal/models"
	probing "github.com/prometheus-community/pro-bing"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const agentAccessInterval = 6 * time.Hour

type agentAccessProbe func(context.Context, string) (float64, string)

// AgentAccessService samples enabled devices four times a day, independently
// of page views, probe reconnects and existing service-monitoring schedules.
type AgentAccessService struct {
	db     *gorm.DB
	logger *zap.Logger
	probe  agentAccessProbe
}

func NewAgentAccessService(db *gorm.DB, logger *zap.Logger) *AgentAccessService {
	return &AgentAccessService{db: db, logger: logger, probe: probeAgentAccess}
}

func (s *AgentAccessService) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.checkDue(ctx, time.Now()); err != nil && ctx.Err() == nil {
			s.logger.Warn("device access check failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Persist the attempt BEFORE sending packets. Restarts, failures and concurrent
// schedulers must not cause a device to be tested again within six hours.
func (s *AgentAccessService) checkDue(ctx context.Context, now time.Time) error {
	var agents []models.Agent
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&agents).Error; err != nil {
		return err
	}
	for _, agent := range agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := AgentAccessTarget(agent)
		if target == "" {
			continue
		}
		check := models.AgentAccessCheck{AgentID: agent.ID, Target: target, Status: "checking", CheckedAt: now.UnixMilli()}
		created := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&check)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			claimed := s.db.WithContext(ctx).Model(&models.AgentAccessCheck{}).
				Where("agent_id = ? AND checked_at <= ?", agent.ID, now.Add(-agentAccessInterval).UnixMilli()).
				Updates(map[string]any{"target": target, "status": "checking", "latency_ms": nil, "checked_at": check.CheckedAt})
			if claimed.Error != nil {
				return claimed.Error
			}
			if claimed.RowsAffected == 0 {
				continue
			}
		}
		latency, status := s.probe(ctx, target)
		var latencyValue any
		if status == "success" {
			latencyValue = latency
		}
		if err := s.db.WithContext(ctx).Model(&models.AgentAccessCheck{}).
			Where("agent_id = ? AND checked_at = ?", agent.ID, check.CheckedAt).
			Updates(map[string]any{"status": status, "latency_ms": latencyValue}).Error; err != nil {
			return err
		}
	}
	return nil
}

// Only literal unicast IPs are used: never URLs, shell arguments or hostnames.
func AgentAccessTarget(agent models.Agent) string {
	for _, value := range []string{agent.IPv4, agent.IPv6, agent.IP} {
		ip, err := netip.ParseAddr(strings.TrimSpace(value))
		if err == nil && ip.Zone() == "" && ip.IsGlobalUnicast() && !ip.IsLoopback() {
			return ip.Unmap().String()
		}
	}
	return ""
}

func probeAgentAccess(ctx context.Context, target string) (float64, string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Try an unprivileged ping first; raw ICMP is a fallback for hosts that
	// already grant CAP_NET_RAW. This code never changes host permissions.
	for _, privileged := range []bool{false, true} {
		pinger, err := probing.NewPinger(target)
		if err != nil {
			return 0, "error"
		}
		pinger.Count = 3
		pinger.Interval = 200 * time.Millisecond
		pinger.Timeout = 4 * time.Second
		pinger.SetPrivileged(privileged)
		if err := pinger.RunWithContext(ctx); err != nil {
			if ctx.Err() != nil {
				return 0, "error"
			}
			continue
		}
		stats := pinger.Statistics()
		if stats.PacketsRecv == 0 {
			return 0, "timeout"
		}
		return float64(stats.AvgRtt) / float64(time.Millisecond), "success"
	}
	return 0, "error"
}
