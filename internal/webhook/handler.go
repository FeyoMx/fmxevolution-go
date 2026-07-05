package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service         *Service
	instanceService legacyInstanceService
}

type legacyInstanceService interface {
	Resolve(ctx context.Context, tenantID, reference string) (*repository.Instance, error)
	SetWebhook(ctx context.Context, tenantID, reference, webhookURL string, events []string, base64 bool, byEvents bool) (*repository.Instance, error)
}

type legacyWebhookPayload struct {
	InstanceName string          `json:"instanceName"`
	Instance     string          `json:"instance"`
	InstanceID   string          `json:"instance_id"`
	WebhookURL2  string          `json:"webhookUrl"`
	URL          string          `json:"url"`
	Name         string          `json:"name"`
	Webhook      json.RawMessage `json:"webhook"`
	WebhookURL   string          `json:"webhook_url"`
	Enabled      *bool           `json:"enabled"`
	Events       json.RawMessage `json:"events"`
}

func NewHandler(service *Service, instanceService legacyInstanceService) *Handler {
	return &Handler{service: service, instanceService: instanceService}
}

func (h *Handler) Create(c *gin.Context) {
	if handled := h.handleLegacyInstanceWebhookCreate(c); handled {
		return
	}

	var input CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		h.service.logger.Warn("webhook create rejected: invalid json payload", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	endpoint, err := h.service.Create(c.Request.Context(), identity.TenantID, input)
	if err != nil {
		h.service.logger.Warn("webhook create rejected", "tenant_id", identity.TenantID, "name", strings.TrimSpace(input.Name), "url", strings.TrimSpace(input.URL), "error", err.Error())
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusCreated, endpoint)
}

func (h *Handler) List(c *gin.Context) {
	if handled := h.handleLegacyInstanceWebhookList(c); handled {
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	endpoints, err := h.service.List(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, endpoints)
}

func (h *Handler) Get(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	endpoint, err := h.service.Get(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, endpoint)
}

// ListDeliveries returns the tenant's webhook delivery log (newest first).
// Query params: endpoint_id, status (pending|delivered|retrying|failed),
// event_type, limit (<=500, default 100), offset.
func (h *Handler) ListDeliveries(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	filter := repository.WebhookDeliveryFilter{
		EndpointID: strings.TrimSpace(c.Query("endpoint_id")),
		Status:     strings.TrimSpace(c.Query("status")),
		EventType:  strings.TrimSpace(c.Query("event_type")),
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			filter.Limit = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	deliveries, err := h.service.ListDeliveries(c.Request.Context(), identity.TenantID, filter)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{"items": deliveries, "count": len(deliveries)})
}

func (h *Handler) handleLegacyInstanceWebhookList(c *gin.Context) bool {
	if h.instanceService == nil {
		return false
	}

	instanceName := strings.TrimSpace(c.Query("instanceName"))
	if instanceName == "" {
		return false
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.instanceService.Resolve(c.Request.Context(), identity.TenantID, instanceName)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return true
	}

	webhookURL := strings.TrimSpace(instance.WebhookURL)
	storedEvents := []string{}
	if instance.WebhookEvents != "" {
		for _, e := range strings.Split(instance.WebhookEvents, ",") {
			if t := strings.TrimSpace(e); t != "" {
				storedEvents = append(storedEvents, t)
			}
		}
	}
	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"enabled":         webhookURL != "",
		"instanceName":    instance.Name,
		"url":             webhookURL,
		"webhook":         webhookURL,
		"webhook_url":     webhookURL,
		"events":          storedEvents,
		"webhookBase64":   instance.WebhookBase64,
		"webhookByEvents": instance.WebhookByEvents,
	})
	return true
}

func (h *Handler) handleLegacyInstanceWebhookCreate(c *gin.Context) bool {
	if h.instanceService == nil {
		return false
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.service.logger.Warn("legacy webhook update rejected: failed to read body", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var payload legacyWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.service.logger.Warn("legacy webhook payload typed decode failed; trying flexible decode", "error", err.Error())
		payload = legacyWebhookPayload{}
	}

	rawPayload := map[string]any{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &rawPayload)
	}

	reference := firstNonEmpty(
		strings.TrimSpace(c.Query("instanceName")),
		strings.TrimSpace(c.Query("instance")),
		strings.TrimSpace(c.PostForm("instanceName")),
		strings.TrimSpace(c.PostForm("instance")),
		strings.TrimSpace(c.PostForm("instance_id")),
		strings.TrimSpace(payload.InstanceName),
		strings.TrimSpace(payload.Instance),
		strings.TrimSpace(payload.InstanceID),
		stringFromMap(rawPayload, "instanceName"),
		stringFromMap(rawPayload, "instance"),
		stringFromMap(rawPayload, "instance_id"),
	)
	if reference == "" && legacyWebhookAllowsNameReference(payload, rawPayload) {
		reference = firstNonEmpty(strings.TrimSpace(payload.Name), stringFromMap(rawPayload, "name"))
	}
	if reference == "" {
		if len(body) == 0 {
			return false
		}
		if looksLikeLegacyWebhookPayload(body) {
			h.service.logger.Warn("legacy webhook update rejected: instance reference missing", "body_bytes", len(body), "query_instanceName_present", strings.TrimSpace(c.Query("instanceName")) != "")
			c.JSON(http.StatusBadRequest, gin.H{"error": "instanceName is required for legacy webhook updates"})
			return true
		}
		return false
	}

	webhookURL := firstNonEmpty(
		strings.TrimSpace(c.PostForm("webhook_url")),
		strings.TrimSpace(c.PostForm("webhookUrl")),
		strings.TrimSpace(c.PostForm("webhook")),
		strings.TrimSpace(c.PostForm("url")),
		strings.TrimSpace(payload.WebhookURL),
		strings.TrimSpace(payload.WebhookURL2),
		strings.TrimSpace(payload.URL),
		stringFromMap(rawPayload, "webhook_url"),
		stringFromMap(rawPayload, "webhookUrl"),
		stringFromMap(rawPayload, "url"),
	)
	webhookEnabled := payload.Enabled
	if webhookEnabled == nil {
		if enabled, ok := boolFromMap(rawPayload, "enabled"); ok {
			webhookEnabled = &enabled
		}
	}

	if nestedWebhookURL, nestedWebhookEnabled, ok := nestedWebhookConfig(payload.Webhook, rawPayload["webhook"]); ok {
		if webhookURL == "" {
			webhookURL = nestedWebhookURL
		}
		if webhookEnabled == nil {
			webhookEnabled = nestedWebhookEnabled
		}
	}
	if webhookEnabled != nil && !*webhookEnabled {
		webhookURL = ""
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	normalizedEvents := normalizeLegacyWebhookEvents(payload.Events, rawPayload["events"])
	// Also check nested webhook.events if top-level events were empty
	if len(normalizedEvents) == 0 {
		if nested, ok := rawPayload["webhook"].(map[string]any); ok {
			normalizedEvents = normalizeLegacyWebhookEvents(nil, nested["events"])
		}
	}

	normalizedBase64, normalizedByEvents := normalizeLegacyWebhookFlags(payload, rawPayload)
	instance, err := h.instanceService.SetWebhook(c.Request.Context(), identity.TenantID, reference, webhookURL, normalizedEvents, normalizedBase64, normalizedByEvents)
	if err != nil {
		h.service.logger.Warn("legacy webhook update rejected", "tenant_id", identity.TenantID, "reference", reference, "webhook_url_present", strings.TrimSpace(webhookURL) != "", "body_bytes", len(body), "error", err.Error())
		sharedhandler.WriteError(c, err)
		return true
	}

	h.service.logger.Info("legacy webhook updated", "tenant_id", identity.TenantID, "reference", reference, "webhook_enabled", strings.TrimSpace(instance.WebhookURL) != "")

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"message":         "webhook updated",
		"instance_id":     instance.ID,
		"instanceName":    instance.Name,
		"url":             instance.WebhookURL,
		"webhook":         instance.WebhookURL,
		"webhook_url":     instance.WebhookURL,
		"enabled":         strings.TrimSpace(instance.WebhookURL) != "",
		"events":          normalizedEvents,
		"webhookBase64":   normalizedBase64,
		"webhookByEvents": normalizedByEvents,
	})
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func looksLikeLegacyWebhookPayload(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}

	for _, key := range []string{"webhook", "webhook_url", "webhookUrl", "enabled", "events"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}

	return false
}

func legacyWebhookAllowsNameReference(payload legacyWebhookPayload, raw map[string]any) bool {
	if strings.TrimSpace(payload.Name) == "" && stringFromMap(raw, "name") == "" {
		return false
	}
	if len(payload.Webhook) > 0 || len(payload.Events) > 0 || payload.Enabled != nil {
		return true
	}
	if strings.TrimSpace(payload.WebhookURL) != "" || strings.TrimSpace(payload.WebhookURL2) != "" {
		return true
	}
	for _, key := range []string{"webhook", "webhook_url", "webhookUrl", "enabled", "events"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func stringFromMap(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	value, ok := raw[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func normalizeLegacyWebhookEvents(rawJSON json.RawMessage, rawValue any) []string {
	values := []string{}

	if len(rawJSON) > 0 {
		var fromJSON []string
		if err := json.Unmarshal(rawJSON, &fromJSON); err == nil {
			values = append(values, fromJSON...)
		}
	}

	if len(values) == 0 {
		switch typed := rawValue.(type) {
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					values = append(values, text)
				}
			}
		case []string:
			values = append(values, typed...)
		case string:
			if strings.TrimSpace(typed) != "" {
				values = append(values, typed)
			}
		}
	}

	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; !ok {
			normalized = append(normalized, v)
			seen[v] = struct{}{}
		}
	}

	return normalized
}

func boolFromMap(raw map[string]any, key string) (bool, bool) {
	if raw == nil {
		return false, false
	}
	value, ok := raw[key]
	if !ok {
		return false, false
	}
	enabled, ok := value.(bool)
	return enabled, ok
}

func nestedWebhookConfig(rawJSON json.RawMessage, rawValue any) (string, *bool, bool) {
	type nestedWebhookPayload struct {
		URL         string `json:"url"`
		WebhookURL  string `json:"webhook_url"`
		WebhookURL2 string `json:"webhookUrl"`
		Enabled     *bool  `json:"enabled"`
	}

	var nested nestedWebhookPayload
	if len(rawJSON) > 0 {
		if err := json.Unmarshal(rawJSON, &nested); err == nil {
			return firstNonEmpty(nested.URL, nested.WebhookURL, nested.WebhookURL2), nested.Enabled, true
		}
	}

	object, ok := rawValue.(map[string]any)
	if !ok {
		return "", nil, false
	}

	url := firstNonEmpty(
		stringFromMap(object, "url"),
		stringFromMap(object, "webhook_url"),
		stringFromMap(object, "webhookUrl"),
	)
	var enabledPtr *bool
	if enabled, ok := boolFromMap(object, "enabled"); ok {
		enabledPtr = &enabled
	}
	return url, enabledPtr, true
}

func (h *Handler) DispatchInbound(c *gin.Context) {
	h.dispatch(c, "inbound")
}

func (h *Handler) DispatchOutbound(c *gin.Context) {
	h.dispatch(c, "outbound")
}

func (h *Handler) dispatch(c *gin.Context, direction string) {
	var input DispatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	var (
		results []DeliveryResult
		err     error
	)

	if direction == "inbound" {
		results, err = h.service.DispatchInbound(c.Request.Context(), identity.TenantID, input)
	} else {
		results, err = h.service.DispatchOutbound(c.Request.Context(), identity.TenantID, input)
	}
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"direction": direction,
		"results":   results,
	})
}

func normalizeLegacyWebhookFlags(payload legacyWebhookPayload, raw map[string]any) (base64 bool, byEvents bool) {
	if payload.Webhook != nil {
		var nested struct {
			Base64   bool `json:"base64"`
			ByEvents bool `json:"byEvents"`
		}
		if err := json.Unmarshal(payload.Webhook, &nested); err == nil {
			return nested.Base64, nested.ByEvents
		}
	}
	if w, ok := raw["webhook"].(map[string]any); ok {
		b64, _ := w["base64"].(bool)
		byEv, _ := w["byEvents"].(bool)
		return b64, byEv
	}
	return false, false
}
