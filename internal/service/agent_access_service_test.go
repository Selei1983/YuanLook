package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/pika-monitor/pika/internal/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func accessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "checks.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.AgentAccessCheck{}); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	return db
}

func TestAgentAccessIntervalSurvivesRestartAndFailure(t *testing.T) {
	db := accessTestDB(t)
	agent := models.Agent{ID: "device", IPv4: "192.0.2.12", Enabled: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	calls := 0
	probe := func(context.Context, string) (float64, string) {
		calls++
		if calls == 2 {
			return 0, "timeout"
		}
		return 12.5, "success"
	}
	// Reconstruct the service on every minute to simulate repeated restarts.
	for minute := 0; minute < 24*60; minute++ {
		s := NewAgentAccessService(db, zap.NewNop())
		s.probe = probe
		if err := s.checkDue(t.Context(), start.Add(time.Duration(minute)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if minute == 6*60 {
			var check models.AgentAccessCheck
			if err := db.First(&check).Error; err != nil {
				t.Fatal(err)
			}
			if check.Status != "timeout" || check.LatencyMS != nil {
				t.Fatalf("failed probe reused previous latency: %+v", check)
			}
		}
	}
	if calls != 4 {
		t.Fatalf("want exactly 4 probes/day, got %d", calls)
	}
}

func TestAgentAccessTargetsAndWorkspaceIsolation(t *testing.T) {
	for _, tc := range []struct {
		agent  models.Agent
		target string
	}{
		{models.Agent{IPv4: "192.0.2.1", IPv6: "2001:db8::1", IP: "192.0.2.2"}, "192.0.2.1"},
		{models.Agent{IPv6: "2001:db8::1", IP: "192.0.2.2"}, "2001:db8::1"},
		{models.Agent{IP: "10.0.0.5"}, "10.0.0.5"},
		{models.Agent{IP: "127.0.0.1"}, ""},
		{models.Agent{IP: "169.254.169.254"}, ""},
		{models.Agent{IP: "https://example.com"}, ""},
		{models.Agent{IP: "224.0.0.1"}, ""},
	} {
		if got := AgentAccessTarget(tc.agent); got != tc.target {
			t.Errorf("target = %q, want %q", got, tc.target)
		}
	}
	first, second := accessTestDB(t), accessTestDB(t)
	for _, db := range []*gorm.DB{first, second} {
		if err := db.Create(&models.Agent{ID: "same-id", IP: "192.0.2.1", Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewAgentAccessService(first, zap.NewNop())
	s.probe = func(context.Context, string) (float64, string) { return 0.2, "success" }
	if err := s.checkDue(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int64
	second.Model(&models.AgentAccessCheck{}).Count(&count)
	if count != 0 {
		t.Fatal("latency crossed workspace boundary")
	}
	if err := first.Model(&models.Agent{}).Where("id = ?", "same-id").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	s.probe = func(context.Context, string) (float64, string) {
		t.Fatal("disabled device was probed")
		return 0, "error"
	}
	if err := s.checkDue(t.Context(), time.Now().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
}
