package dashboard

import (
	"context"
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	lister instanceListService
}

type instanceListService interface {
	List(ctx context.Context, tenantID string) ([]MetricInstance, error)
}

func NewHandler(lister instanceListService) *Handler {
	return &Handler{lister: lister}
}

func (h *Handler) Metrics(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	instances, err := h.lister.List(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	active := 0
	for _, item := range instances {
		if item.Status == "open" || item.Status == "connected" {
			active++
		}
	}

	totalInstances := len(instances)
	inactiveInstances := totalInstances - active

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		// snake_case
		"instances_total":    totalInstances,
		"instances_active":   active,
		"instances_inactive": inactiveInstances,
		"contacts_total":     0,
		"messages_total":     0,
		"broadcast_total":    0,
		"tenants_total":      1,
		"users_total":        0,
		// camelCase compatibility
		"totalInstances":    totalInstances,
		"activeInstances":   active,
		"inactiveInstances": inactiveInstances,
		"totalContacts":     0,
		"totalMessages":     0,
		"totalBroadcasts":   0,
		"totalTenants":      1,
		"totalUsers":        0,
		"connectedInstances": active,
		"disconnectedInstances": inactiveInstances,
		// dashboard-style aliases
		"instances": totalInstances,
		"contacts":  0,
		"messages":  0,
		"broadcasts": 0,
		"metrics": gin.H{
			"instances_total":       totalInstances,
			"instances_active":      active,
			"instances_inactive":    inactiveInstances,
			"contacts_total":        0,
			"messages_total":        0,
			"broadcast_total":       0,
			"totalInstances":        totalInstances,
			"activeInstances":       active,
			"inactiveInstances":     inactiveInstances,
			"totalContacts":         0,
			"totalMessages":         0,
			"totalBroadcasts":       0,
			"connectedInstances":    active,
			"disconnectedInstances": inactiveInstances,
		},
	})
}

type MetricInstance struct {
	Status string
}
