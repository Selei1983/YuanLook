package repo

import (
	"context"

	"github.com/pika-monitor/pika/internal/models"
)

func (r *AgentRepo) FindAccessChecks(ctx context.Context, agents []models.Agent) (map[string]models.AgentAccessCheck, error) {
	result := make(map[string]models.AgentAccessCheck)
	if len(agents) == 0 {
		return result, nil
	}
	ids := make([]string, 0, len(agents))
	for _, agent := range agents {
		ids = append(ids, agent.ID)
	}
	var checks []models.AgentAccessCheck
	if err := r.db.WithContext(ctx).Where("agent_id IN ?", ids).Find(&checks).Error; err != nil {
		return nil, err
	}
	for _, check := range checks {
		result[check.AgentID] = check
	}
	return result, nil
}
