package handler

import (
	"encoding/json"

	"github.com/go-orz/orz"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/service"
	"github.com/pika-monitor/pika/internal/utils"
	"gorm.io/datatypes"
)

// Get 获取探针详情（公开接口，已登录返回全部，未登录返回公开可见）
func (h *AgentHandler) Get(c *echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	// 根据认证状态返回相应的探针
	isAuthenticated := utils.IsAuthenticated(c)
	agent, err := h.agentService.GetAgentByAuth(ctx, id, isAuthenticated)
	if err != nil {
		return err
	}

	// 隐藏敏感配置
	agent.SSHLoginConfig = datatypes.JSONType[models.SSHLoginConfigData]{}
	agent.TamperProtectConfig = datatypes.JSONType[models.TamperProtectConfigData]{}

	// 未登录时隐藏敏感信息
	if !isAuthenticated {
		agent.IP = ""
		agent.IPv4 = ""
		agent.IPv6 = ""
		agent.Hostname = ""
	}

	if !isAuthenticated {
		payload, err := json.Marshal(agent)
		if err != nil {
			return err
		}
		var publicAgent map[string]any
		if err := json.Unmarshal(payload, &publicAgent); err != nil {
			return err
		}
		delete(publicAgent, "enabled")
		return orz.Ok(c, publicAgent)
	}

	return orz.Ok(c, agent)
}

// GetAgents 获取探针列表（公开接口，已登录返回全部，未登录返回公开可见）
func (h *AgentHandler) GetAgents(c *echo.Context) error {
	ctx := c.Request().Context()

	// 根据认证状态返回相应的探针列表
	isAuthenticated := utils.IsAuthenticated(c)
	agents, err := h.agentService.ListByAuth(ctx, isAuthenticated)
	if err != nil {
		return err
	}

	// 排序
	SortAgents(agents)
	checks := map[string]models.AgentAccessCheck{}
	if isAuthenticated {
		checks, err = h.agentService.AgentRepo.FindAccessChecks(ctx, agents)
		if err != nil {
			return err
		}
	}

	result := make([]map[string]interface{}, 0, len(agents))
	for _, agent := range agents {
		item := h.buildAgentListItem(agent, isAuthenticated)
		if check, ok := checks[agent.ID]; ok {
			item["accessLatency"] = check
		}
		result = append(result, item)
	}

	return orz.Ok(c, result)
}

func (h *AgentHandler) buildAgentListItem(agent models.Agent, isAuthenticated bool) map[string]interface{} {
	item := map[string]any{
		"id":         agent.ID,
		"name":       agent.Name,
		"os":         agent.OS,
		"arch":       agent.Arch,
		"version":    agent.Version,
		"tags":       agent.Tags,
		"expireTime": agent.ExpireTime,
		"status":     agent.Status,
		"lastSeenAt": agent.LastSeenAt,
		"visibility": agent.Visibility,
		"weight":     agent.Weight,
	}
	if isAuthenticated {
		item["hostname"] = agent.Hostname
		item["ip"] = agent.IP
		item["ipv4"] = agent.IPv4
		item["ipv6"] = agent.IPv6
		item["accessTarget"] = service.AgentAccessTarget(agent)
	}

	trafficStats := agent.TrafficStats.Data()
	if trafficStats.Enabled {
		item["trafficStats"] = trafficStats
	} else {
		item["trafficStats"] = map[string]any{
			"enabled": false,
		}
	}

	metrics, ok := h.metricService.GetLatestMetrics(agent.ID)
	if ok && metrics != nil {
		if !isAuthenticated {
			metrics.NetworkInterfaces = nil
		}
		item["metrics"] = metrics
	}

	return item
}

// GetTags 获取所有探针的标签
func (h *AgentHandler) GetTags(c *echo.Context) error {
	ctx := c.Request().Context()

	tags, err := h.agentService.GetAllTags(ctx)
	if err != nil {
		return err
	}

	return orz.Ok(c, orz.Map{
		"tags": tags,
	})
}
