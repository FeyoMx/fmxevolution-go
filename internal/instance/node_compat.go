package instance

import (
	"net/http"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

// This file implements Evolution API v2 compatibility for the
// n8n-nodes-evolution-api node, which expects v2 routes and response shapes.

// LegacyFetchInstances implements GET /instance/fetchInstances.
// The n8n node uses this both as its credential connection test and to list
// instances. It returns an array of instances in Evolution API v2 shape.
// Optional ?instanceName= filters to a single instance.
func (h *Handler) LegacyFetchInstances(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	instances, err := h.service.List(c.Request.Context(), identity.TenantID)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	filter := strings.TrimSpace(c.Query("instanceName"))
	out := make([]gin.H, 0, len(instances))
	for i := range instances {
		inst := instances[i]
		if filter != "" && !strings.EqualFold(strings.TrimSpace(inst.Name), filter) {
			continue
		}
		out = append(out, evolutionInstanceView(inst))
	}

	c.JSON(http.StatusOK, out)
}

// evolutionInstanceView maps our Instance to the Evolution API v2 fetchInstances
// item shape. Fields the node reads (name, connectionStatus, integrations) are
// present; unknown-to-us fields default to sane values.
func evolutionInstanceView(inst repository.Instance) gin.H {
	events := []string{}
	if strings.TrimSpace(inst.WebhookEvents) != "" {
		for _, e := range strings.Split(inst.WebhookEvents, ",") {
			if t := strings.TrimSpace(e); t != "" {
				events = append(events, t)
			}
		}
	}

	var webhook any
	if strings.TrimSpace(inst.WebhookURL) != "" {
		webhook = gin.H{
			"enabled": true,
			"url":     inst.WebhookURL,
			"events":  events,
			"base64":  inst.WebhookBase64,
			"byEvents": inst.WebhookByEvents,
		}
	}

	return gin.H{
		"id":                 inst.ID,
		"name":               inst.Name,
		"instanceName":       inst.Name,
		"connectionStatus":   evolutionConnectionStatus(inst.Status),
		"status":             evolutionConnectionStatus(inst.Status),
		"ownerJid":           nil,
		"profileName":        nil,
		"integration":        "WHATSAPP-BAILEYS",
		"engine_instance_id": inst.EngineInstanceID,
		"webhook_url":        inst.WebhookURL,
		"webhook":            webhook,
		"createdAt":          inst.CreatedAt,
		"updatedAt":          inst.UpdatedAt,
	}
}

// evolutionConnectionStatus maps our internal status values to the v2
// connectionStatus vocabulary the node understands (open|connecting|close).
func evolutionConnectionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open", "connected", "online":
		return "open"
	case "connecting", "qr", "qr_pending", "pairing", "reconnecting":
		return "connecting"
	case "close", "closed", "disconnected", "logout", "created":
		return "close"
	default:
		if status == "" {
			return "close"
		}
		return strings.ToLower(status)
	}
}
