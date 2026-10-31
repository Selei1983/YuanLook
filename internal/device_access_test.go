package internal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pika-monitor/pika/internal/models"
)

func TestDeviceAddressesAndLatencyArePrivateAndWorkspaceScoped(t *testing.T) {
	router, e := testWorkspaceRouter(t)
	for name, space := range router.spaces {
		address := "192.0.2.1"
		latency := 12.5
		if name == "alice" {
			address = "2001:db8::2"
			latency = 98.2
		}
		if err := space.app.GetDatabase().Create(&models.Agent{ID: "same-agent", Name: name, Hostname: name + "-host", IP: address, IPv4: "192.0.2.10", IPv6: "2001:db8::10", Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := space.app.GetDatabase().Create(&models.AgentAccessCheck{AgentID: "same-agent", Target: address, Status: "success", LatencyMS: &latency, CheckedAt: time.Now().UnixMilli()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"admin", "alice"} {
		token, _ := loginWorkspace(t, e, name)
		res := requestWorkspace(t, e, "GET", "/api/agents", token, "")
		if res.Code != 200 {
			t.Fatalf("list: %d %s", res.Code, res.Body.String())
		}
		var rows []struct {
			Name     string                  `json:"name"`
			Hostname string                  `json:"hostname"`
			IP       string                  `json:"ip"`
			IPv4     string                  `json:"ipv4"`
			IPv6     string                  `json:"ipv6"`
			Access   models.AgentAccessCheck `json:"accessLatency"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Name != name || rows[0].Hostname != name+"-host" || rows[0].IPv4 == "" || rows[0].IPv6 == "" || rows[0].Access.LatencyMS == nil {
			t.Fatalf("missing or leaked data: %s", res.Body.String())
		}
		wantIP, wantLatency := "192.0.2.1", 12.5
		if name == "alice" {
			wantIP, wantLatency = "2001:db8::2", 98.2
		}
		if rows[0].IP != wantIP || *rows[0].Access.LatencyMS != wantLatency {
			t.Fatalf("workspace leak: %s", res.Body.String())
		}
	}
	res := requestWorkspace(t, e, "GET", "/api/agents", "", "")
	if res.Code != 401 || strings.Contains(res.Body.String(), "192.0.2") || strings.Contains(res.Body.String(), "accessLatency") {
		t.Fatal("anonymous device data leak")
	}
}
