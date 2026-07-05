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

// ---- Fase B: rich message operations (Evolution API v2 node bodies) ----

type reactionPayload struct {
	Key struct {
		RemoteJID string `json:"remoteJid"`
		FromMe    bool   `json:"fromMe"`
		ID        string `json:"id"`
	} `json:"key"`
	Reaction string `json:"reaction"`
}

// LegacySendReaction implements POST /message/sendReaction/{instance}.
func (h *Handler) LegacySendReaction(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	var p reactionPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid reaction payload; key and reaction are required", err)
		return
	}
	result, instance, err := h.service.SendReaction(
		c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c),
		p.Key.RemoteJID, p.Key.ID, p.Reaction, p.Key.FromMe,
	)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}
	writeEvolutionCompatSuccess(c, http.StatusOK, "reaction sent successfully", buildLegacySendData(instance, result))
}

type pollPayload struct {
	Number          string   `json:"number"`
	Name            string   `json:"name"`
	Values          []string `json:"values"`
	SelectableCount int      `json:"selectableCount"`
}

// LegacySendPoll implements POST /message/sendPoll/{instance}.
func (h *Handler) LegacySendPoll(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	var p pollPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid poll payload; number, name and values are required", err)
		return
	}
	result, instance, err := h.service.SendPoll(
		c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c),
		p.Number, p.Name, p.Values, p.SelectableCount,
	)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}
	writeEvolutionCompatSuccess(c, http.StatusOK, "poll sent successfully", buildLegacySendData(instance, result))
}

type contactPayload struct {
	Number  string `json:"number"`
	Contact []struct {
		FullName     string `json:"fullName"`
		WUID         string `json:"wuid"`
		PhoneNumber  string `json:"phoneNumber"`
		Organization string `json:"organization"`
		Email        string `json:"email"`
		URL          string `json:"url"`
	} `json:"contact"`
}

// LegacySendContact implements POST /message/sendContact/{instance}.
func (h *Handler) LegacySendContact(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	var p contactPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid contact payload; number and contact are required", err)
		return
	}
	cards := make([]ContactCard, 0, len(p.Contact))
	for _, ct := range p.Contact {
		cards = append(cards, ContactCard{
			FullName:     ct.FullName,
			PhoneNumber:  ct.PhoneNumber,
			WUID:         ct.WUID,
			Organization: ct.Organization,
			Email:        ct.Email,
			URL:          ct.URL,
		})
	}
	result, instance, err := h.service.SendContact(
		c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), p.Number, cards,
	)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}
	writeEvolutionCompatSuccess(c, http.StatusOK, "contact sent successfully", buildLegacySendData(instance, result))
}

type whatsappNumbersPayload struct {
	Numbers []string `json:"numbers"`
}

// LegacyCheckNumbers implements POST /chat/whatsappNumbers/{instance}.
func (h *Handler) LegacyCheckNumbers(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	var p whatsappNumbersPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid payload; numbers[] is required", err)
		return
	}
	results, _, err := h.service.CheckNumbers(
		c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), p.Numbers,
	)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}
	// The node expects a plain array of {number, exists, jid}.
	c.JSON(http.StatusOK, results)
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
