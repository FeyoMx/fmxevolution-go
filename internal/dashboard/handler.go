package dashboard

import (
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service metricsService
}

func NewHandler(service metricsService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Metrics(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	snapshot, err := h.service.Metrics(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	totalInstances := snapshot.InstancesTotal
	active := snapshot.InstancesActive
	inactiveInstances := snapshot.InstancesInactive

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		// snake_case
		"instances_total":        totalInstances,
		"instances_active":       active,
		"instances_inactive":     inactiveInstances,
		"contacts_total":         snapshot.ContactsTotal,
		"messages_total":         snapshot.MessagesTotal,
		"messages_total_partial": snapshot.MessagesTotalPartial,
		"broadcast_total":        snapshot.BroadcastTotal,
		"runtime_healthy":        snapshot.RuntimeHealthy,
		"runtime_degraded":       snapshot.RuntimeDegraded,
		"runtime_unavailable":    snapshot.RuntimeUnavailable,
		"runtime_unknown":        snapshot.RuntimeUnknown,
		"runtime_health_partial": snapshot.RuntimeHealthPartial,
		"tenants_total":          1,
		"users_total":            0,
		// camelCase compatibility
		"totalInstances":        totalInstances,
		"activeInstances":       active,
		"inactiveInstances":     inactiveInstances,
		"totalContacts":         snapshot.ContactsTotal,
		"totalMessages":         snapshot.MessagesTotal,
		"totalMessagesPartial":  snapshot.MessagesTotalPartial,
		"totalBroadcasts":       snapshot.BroadcastTotal,
		"runtimeHealthy":        snapshot.RuntimeHealthy,
		"runtimeDegraded":       snapshot.RuntimeDegraded,
		"runtimeUnavailable":    snapshot.RuntimeUnavailable,
		"runtimeUnknown":        snapshot.RuntimeUnknown,
		"runtimeHealthPartial":  snapshot.RuntimeHealthPartial,
		"totalTenants":          1,
		"totalUsers":            0,
		"connectedInstances":    active,
		"disconnectedInstances": inactiveInstances,
		// dashboard-style aliases
		"instances":  totalInstances,
		"contacts":   snapshot.ContactsTotal,
		"messages":   snapshot.MessagesTotal,
		"broadcasts": snapshot.BroadcastTotal,
		"runtime_health": gin.H{
			"healthy":     snapshot.RuntimeHealthy,
			"degraded":    snapshot.RuntimeDegraded,
			"unavailable": snapshot.RuntimeUnavailable,
			"unknown":     snapshot.RuntimeUnknown,
			"partial":     snapshot.RuntimeHealthPartial,
		},
		"metrics": gin.H{
			"instances_total":        totalInstances,
			"instances_active":       active,
			"instances_inactive":     inactiveInstances,
			"contacts_total":         snapshot.ContactsTotal,
			"messages_total":         snapshot.MessagesTotal,
			"messages_total_partial": snapshot.MessagesTotalPartial,
			"broadcast_total":        snapshot.BroadcastTotal,
			"runtime_healthy":        snapshot.RuntimeHealthy,
			"runtime_degraded":       snapshot.RuntimeDegraded,
			"runtime_unavailable":    snapshot.RuntimeUnavailable,
			"runtime_unknown":        snapshot.RuntimeUnknown,
			"runtime_health_partial": snapshot.RuntimeHealthPartial,
			"totalInstances":         totalInstances,
			"activeInstances":        active,
			"inactiveInstances":      inactiveInstances,
			"totalContacts":          snapshot.ContactsTotal,
			"totalMessages":          snapshot.MessagesTotal,
			"totalMessagesPartial":   snapshot.MessagesTotalPartial,
			"totalBroadcasts":        snapshot.BroadcastTotal,
			"runtimeHealthy":         snapshot.RuntimeHealthy,
			"runtimeDegraded":        snapshot.RuntimeDegraded,
			"runtimeUnavailable":     snapshot.RuntimeUnavailable,
			"runtimeUnknown":         snapshot.RuntimeUnknown,
			"runtimeHealthPartial":   snapshot.RuntimeHealthPartial,
			"connectedInstances":     active,
			"disconnectedInstances":  inactiveInstances,
		},
	})
}
