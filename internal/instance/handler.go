package instance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

type historyBackfillRequestPayload struct {
	ChatJID     string `json:"chat_jid"`
	Chat        string `json:"chat"`
	RemoteJID   string `json:"remote_jid"`
	MessageID   string `json:"message_id"`
	Timestamp   string `json:"timestamp"`
	IsFromMe    *bool  `json:"is_from_me"`
	IsGroup     *bool  `json:"is_group"`
	Count       int    `json:"count"`
	MessageInfo *struct {
		Chat      string `json:"chat"`
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		IsFromMe  *bool  `json:"isFromMe"`
		IsGroup   *bool  `json:"isGroup"`
	} `json:"messageInfo"`
}

func instanceReferenceFromParams(c *gin.Context) string {
	if ref := strings.TrimSpace(c.Param("id")); ref != "" {
		return ref
	}
	return strings.TrimSpace(c.Param("instanceID"))
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	input, err := decodeCreateInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "invalid instance payload; use name or instanceName", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Create(c.Request.Context(), identity.TenantID, input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusCreated, instance)
}

func (h *Handler) List(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instances, err := h.service.List(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, instances)
}

func (h *Handler) Get(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Get(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	h.writeInstanceDetails(c, identity.TenantID, instance)
}

func (h *Handler) Settings(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Resolve(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	h.writeInstanceDetails(c, identity.TenantID, instance)
}

func (h *Handler) GetAdvancedSettings(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	settings, instance, err := h.service.GetAdvancedSettings(c.Request.Context(), identity.TenantID, instanceReferenceFromParams(c))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	payload := gin.H{
		"alwaysOnline":  settings.AlwaysOnline,
		"rejectCall":    settings.RejectCall,
		"msgRejectCall": settings.MsgRejectCall,
		"readMessages":  settings.ReadMessages,
		"ignoreGroups":  settings.IgnoreGroups,
		"ignoreStatus":  settings.IgnoreStatus,
	}

	if instance != nil {
		payload["instance_id"] = instance.ID
		payload["instanceName"] = instance.Name
		payload["engine_instance_id"] = instance.EngineInstanceID
	}

	sharedhandler.WriteJSON(c, http.StatusOK, payload)
}

func (h *Handler) UpdateAdvancedSettings(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var settings legacyInstanceModel.AdvancedSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		sharedhandler.WriteValidationError(c, "invalid advanced settings payload", err)
		return
	}

	updated, instance, err := h.service.UpdateAdvancedSettings(c.Request.Context(), identity.TenantID, instanceReferenceFromParams(c), &settings)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	payload := gin.H{
		"message": "Advanced settings updated successfully",
		"settings": gin.H{
			"alwaysOnline":  updated.AlwaysOnline,
			"rejectCall":    updated.RejectCall,
			"msgRejectCall": updated.MsgRejectCall,
			"readMessages":  updated.ReadMessages,
			"ignoreGroups":  updated.IgnoreGroups,
			"ignoreStatus":  updated.IgnoreStatus,
		},
	}

	if instance != nil {
		payload["instance_id"] = instance.ID
		payload["instanceName"] = instance.Name
		payload["engine_instance_id"] = instance.EngineInstanceID
	}

	sharedhandler.WriteJSON(c, http.StatusOK, payload)
}

func (h *Handler) SendText(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var input SendTextInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "invalid text message payload; number and text are required", err)
		return
	}

	jobID, instance, err := h.service.QueueSendText(c.Request.Context(), identity.TenantID, instanceReferenceFromParams(c), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	payload := gin.H{
		"message":            "message queued; delivery pending",
		"queued":             true,
		"accepted_only":      true,
		"sent":               false,
		"delivery_confirmed": false,
		"delivery_status":    "queued",
		"job_id":             jobID,
		"number":             strings.TrimSpace(input.Number),
		"text":               input.Text,
		"instance_id":        "",
		"instanceName":       "",
	}
	if instance != nil {
		payload["instance_id"] = instance.ID
		payload["instanceName"] = instance.Name
		payload["engine_instance_id"] = instance.EngineInstanceID
		payload["status_endpoint"] = "/instance/id/" + instance.ID + "/messages/text/" + jobID
	}

	sharedhandler.WriteJSON(c, http.StatusAccepted, payload)
}

func (h *Handler) SendTextJobStatus(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	job, instance, err := h.service.GetSendTextJob(c.Request.Context(), identity.TenantID, instanceReferenceFromParams(c), c.Param("jobID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	payload := gin.H{
		"job_id":             job.JobID,
		"status":             job.Status,
		"delivery_status":    job.DeliveryStatus(),
		"sent":               job.Sent(),
		"delivery_confirmed": job.DeliveryConfirmed,
		"number":             job.Number,
		"text":               job.Text,
		"queued_at":          job.QueuedAt,
		"started_at":         job.StartedAt,
		"finished_at":        job.FinishedAt,
	}
	if job.Error != "" {
		payload["error"] = job.Error
	}
	if job.MessageID != "" {
		payload["message_id"] = job.MessageID
	}
	if job.ServerID != 0 {
		payload["server_id"] = job.ServerID
	}
	if job.DeliveredAt != nil {
		payload["delivered_at"] = job.DeliveredAt
	}
	if job.ReadAt != nil {
		payload["read_at"] = job.ReadAt
	}
	if instance != nil {
		payload["instance_id"] = instance.ID
		payload["instanceName"] = instance.Name
		payload["engine_instance_id"] = instance.EngineInstanceID
	}

	sharedhandler.WriteJSON(c, http.StatusOK, payload)
}

func (h *Handler) GetByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Get(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	h.writeInstanceDetails(c, identity.TenantID, instance)
}

func (h *Handler) Delete(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instanceID := c.Param("id")
	if instanceID == "" {
		instanceID = c.Query("id")
	}

	if err := h.service.Delete(c.Request.Context(), identity.TenantID, instanceID); err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	if err := h.service.Delete(c.Request.Context(), identity.TenantID, c.Param("instanceID")); err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) Connect(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Connect(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	response := gin.H{
		"message":      "instance connect queued",
		"instance_id":  instance.ID,
		"instanceName": instance.Name,
		"status":       instance.Status,
	}
	if h.service.runtime != nil {
		if snapshot, snapshotErr := h.service.runtime.Snapshot(c.Request.Context(), instance); snapshotErr == nil && snapshot != nil {
			response["qrcode"] = snapshot.QRCode
			response["code"] = snapshot.PairingCode
			response["connected"] = snapshot.Connected && snapshot.LoggedIn
			response["status"] = snapshot.Status
		}
	}

	sharedhandler.WriteJSON(c, http.StatusOK, response)
}

func (h *Handler) ConnectByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.ConnectByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	response := gin.H{
		"message":      "instance connect queued",
		"instance_id":  instance.ID,
		"instanceName": instance.Name,
		"status":       instance.Status,
	}
	if h.service.runtime != nil {
		if snapshot, snapshotErr := h.service.runtime.Snapshot(c.Request.Context(), instance); snapshotErr == nil && snapshot != nil {
			response["qrcode"] = snapshot.QRCode
			response["code"] = snapshot.PairingCode
			response["connected"] = snapshot.Connected && snapshot.LoggedIn
			response["status"] = snapshot.Status
		}
	}

	sharedhandler.WriteJSON(c, http.StatusOK, response)
}

func (h *Handler) Disconnect(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Disconnect(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"message":      "instance disconnected",
		"instance_id":  instance.ID,
		"instanceName": instance.Name,
		"status":       instance.Status,
	})
}

func (h *Handler) DisconnectByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.DisconnectByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"message":      "instance disconnected",
		"instance_id":  instance.ID,
		"instanceName": instance.Name,
		"status":       instance.Status,
	})
}

func (h *Handler) Reconnect(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.Reconnect(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "instance reconnect queued", instance, snapshot, gin.H{
		"accepted": true,
		"action":   "reconnect",
	})
}

func (h *Handler) ReconnectByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.ReconnectByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "instance reconnect queued", instance, snapshot, gin.H{
		"accepted": true,
		"action":   "reconnect",
	})
}

func (h *Handler) Logout(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.Logout(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "instance logged out", instance, snapshot, gin.H{
		"accepted":  true,
		"action":    "logout",
		"loggedOut": true,
	})
}

func (h *Handler) LogoutByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.LogoutByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "instance logged out", instance, snapshot, gin.H{
		"accepted":  true,
		"action":    "logout",
		"loggedOut": true,
	})
}

func (h *Handler) Status(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Status(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeNormalizedStatus(c, http.StatusOK, instance)
}

func (h *Handler) StatusByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.StatusByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeNormalizedStatus(c, http.StatusOK, instance)
}

func (h *Handler) RuntimeStatus(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, state, snapshot, err := h.service.RuntimeStatus(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildRuntimeStatusEnvelope(instance, state, snapshot))
}

func (h *Handler) RuntimeStatusByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, state, snapshot, err := h.service.RuntimeStatusByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildRuntimeStatusEnvelope(instance, state, snapshot))
}

func (h *Handler) RuntimeHistory(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, events, err := h.service.RuntimeHistory(c.Request.Context(), identity.TenantID, c.Param("id"), runtimeHistoryLimit(c))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildRuntimeHistoryEnvelope(instance, events))
}

func (h *Handler) RuntimeHistoryByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, events, err := h.service.RuntimeHistoryByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"), runtimeHistoryLimit(c))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildRuntimeHistoryEnvelope(instance, events))
}

func (h *Handler) BackfillHistory(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	input, err := decodeHistoryBackfillInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "invalid history backfill payload", err)
		return
	}

	instance, result, anchorSource, err := h.service.BackfillHistory(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildHistoryBackfillEnvelope(instance, result, anchorSource))
}

func (h *Handler) BackfillHistoryByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	input, err := decodeHistoryBackfillInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "invalid history backfill payload", err)
		return
	}

	instance, result, anchorSource, err := h.service.BackfillHistoryByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildHistoryBackfillEnvelope(instance, result, anchorSource))
}

func (h *Handler) QRCode(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.QRCode(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		h.writeQRCodeFallback(c, identity.TenantID, c.Param("id"), err)
		return
	}

	h.writeNormalizedQRCode(c, http.StatusOK, instance, snapshot, "")
}

func (h *Handler) QRCodeByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, snapshot, err := h.service.QRCodeByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		h.writeQRCodeFallback(c, identity.TenantID, c.Param("instanceID"), err)
		return
	}

	h.writeNormalizedQRCode(c, http.StatusOK, instance, snapshot, "")
}

func (h *Handler) Pair(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	input, err := decodePairInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "invalid pairing payload; phone is required", err)
		return
	}

	instance, snapshot, err := h.service.Pair(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "pairing code generated", instance, snapshot, gin.H{
		"accepted":    true,
		"action":      "pair",
		"code":        snapshot.PairingCode,
		"pairingCode": snapshot.PairingCode,
	})
}

func (h *Handler) PairByID(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	input, err := decodePairInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "invalid pairing payload; phone is required", err)
		return
	}

	instance, snapshot, err := h.service.PairByID(c.Request.Context(), identity.TenantID, c.Param("instanceID"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeRuntimeActionEnvelope(c, http.StatusOK, "pairing code generated", instance, snapshot, gin.H{
		"accepted":    true,
		"action":      "pair",
		"code":        snapshot.PairingCode,
		"pairingCode": snapshot.PairingCode,
	})
}

func decodeCreateInput(c *gin.Context) (CreateInput, error) {
	input := CreateInput{
		Name:             c.PostForm("name"),
		EngineInstanceID: c.PostForm("engine_instance_id"),
		WebhookURL:       c.PostForm("webhook_url"),
	}

	if input.Name == "" {
		input.Name = c.PostForm("instanceName")
	}
	if input.Name == "" {
		input.Name = c.PostForm("instance")
	}
	if input.EngineInstanceID == "" {
		input.EngineInstanceID = c.PostForm("engineInstanceId")
	}
	if input.EngineInstanceID == "" {
		input.EngineInstanceID = c.PostForm("integration")
	}
	if input.WebhookURL == "" {
		input.WebhookURL = c.PostForm("webhookUrl")
	}

	rawBody, err := readRequestBody(c)
	if err != nil {
		return CreateInput{}, err
	}
	if len(rawBody) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err == nil {
			mergeCreatePayload(&input, payload)
		}
	}

	input.Name = strings.TrimSpace(input.Name)
	input.EngineInstanceID = strings.TrimSpace(input.EngineInstanceID)
	input.WebhookURL = strings.TrimSpace(input.WebhookURL)
	return input, nil
}

func decodePairInput(c *gin.Context) (PairInput, error) {
	input := PairInput{
		Phone: c.PostForm("phone"),
	}
	if strings.TrimSpace(input.Phone) == "" {
		input.Phone = c.PostForm("number")
	}

	rawBody, err := readRequestBody(c)
	if err != nil {
		return PairInput{}, err
	}
	if len(rawBody) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err == nil {
			if value := firstStringValue(payload, "phone", "number"); strings.TrimSpace(value) != "" {
				input.Phone = value
			}
		}
	}

	input.Phone = strings.TrimSpace(input.Phone)
	if input.Phone == "" {
		return PairInput{}, fmt.Errorf("%w: phone is required", domain.ErrValidation)
	}

	return input, nil
}

func readRequestBody(c *gin.Context) ([]byte, error) {
	if c.Request == nil || c.Request.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}

	return body, nil
}

func mergeCreatePayload(input *CreateInput, payload map[string]any) {
	if input == nil {
		return
	}

	setStringIfEmpty := func(target *string, keys ...string) {
		if strings.TrimSpace(*target) != "" {
			return
		}
		for _, key := range keys {
			value, ok := payload[key]
			if !ok {
				continue
			}
			text, ok := value.(string)
			if ok && strings.TrimSpace(text) != "" {
				*target = text
				return
			}
		}
	}

	setStringIfEmpty(&input.Name, "name", "instanceName", "instance")
	setStringIfEmpty(&input.EngineInstanceID, "engine_instance_id", "engineInstanceId", "integration", "provider")
	setStringIfEmpty(&input.WebhookURL, "webhook_url", "webhookUrl", "webhook")
}

func firstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func (h *Handler) writeQRCodeFallback(c *gin.Context, tenantID, reference string, err error) {
	if h.service.logger != nil {
		h.service.logger.Warn("qrcode pending", "reference", reference, "error", err)
	}

	instance, lookupErr := h.service.Status(c.Request.Context(), tenantID, reference)
	if lookupErr != nil || instance == nil {
		sharedhandler.WriteError(c, err)
		return
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	statusCode := http.StatusAccepted
	status := instance.Status

	switch {
	case strings.Contains(message, "session already logged in"):
		statusCode = http.StatusOK
		status = "open"
	case strings.Contains(message, "no qr code available"),
		strings.Contains(message, "failed to start instance"),
		strings.Contains(message, "runtime unavailable"),
		strings.Contains(message, "legacy runtime unavailable"):
		if status == "" || status == "created" || status == "close" {
			status = "connecting"
		}
	default:
		sharedhandler.WriteError(c, err)
		return
	}

	h.writeNormalizedQRCode(c, statusCode, &repository.Instance{
		ID:               instance.ID,
		Name:             instance.Name,
		EngineInstanceID: instance.EngineInstanceID,
		Status:           status,
	}, nil, err.Error())
}

func (h *Handler) writeNormalizedStatus(c *gin.Context, statusCode int, instance *repository.Instance) {
	payload := buildStatusPayload(instance)
	if h.service.runtime != nil && instance != nil {
		if snapshot, err := h.service.runtime.Snapshot(c.Request.Context(), instance); err == nil && snapshot != nil {
			enrichPayloadWithSnapshot(payload, snapshot)
		}
	}
	writeLegacyCompatibleEnvelope(c, statusCode, "success", payload)
}

func (h *Handler) writeNormalizedQRCode(c *gin.Context, statusCode int, instance *repository.Instance, snapshot *RuntimeSnapshot, pendingReason string) {
	payload := buildQRCodePayload(instance, snapshot)
	enrichPayloadWithSnapshot(payload, snapshot)
	if strings.TrimSpace(pendingReason) != "" {
		payload["pending_reason"] = strings.TrimSpace(pendingReason)
	}
	writeLegacyCompatibleEnvelope(c, statusCode, "success", payload)
}

func (h *Handler) writeRuntimeActionEnvelope(c *gin.Context, statusCode int, message string, instance *repository.Instance, snapshot *RuntimeSnapshot, extras gin.H) {
	payload := buildStatusPayload(instance)
	enrichPayloadWithSnapshot(payload, snapshot)
	payload["operator_message"] = message
	payload["bridge_dependent"] = true
	payload["status_refresh"] = true
	for key, value := range extras {
		payload[key] = value
	}
	writeLegacyCompatibleEnvelope(c, statusCode, message, payload)
}

func buildStatusPayload(instance *repository.Instance) gin.H {
	payload := gin.H{
		"instance_id":        "",
		"instanceName":       "",
		"engine_instance_id": "",
		"status":             "close",
		"connected":          false,
	}
	if instance == nil {
		return payload
	}

	payload["instance_id"] = instance.ID
	payload["instanceName"] = instance.Name
	payload["engine_instance_id"] = instance.EngineInstanceID
	if strings.TrimSpace(instance.Status) != "" {
		payload["status"] = instance.Status
	}
	payload["connected"] = instance.Status == "open" || instance.Status == "connected"
	return payload
}

func buildRuntimeStatusEnvelope(instance *repository.Instance, state *repository.RuntimeSessionState, snapshot *RuntimeSnapshot) gin.H {
	payload := buildStatusPayload(instance)

	durable := gin.H{
		"available":            state != nil,
		"status":               "",
		"last_seen_status":     "",
		"last_event_type":      "",
		"last_event_source":    "",
		"connected":            false,
		"logged_in":            false,
		"pairing_active":       false,
		"disconnect_reason":    "",
		"last_error":           "",
		"last_seen_at":         nil,
		"last_event_at":        nil,
		"last_connected_at":    nil,
		"last_disconnected_at": nil,
		"last_paired_at":       nil,
		"last_logout_at":       nil,
		"bridge_dependent":     false,
	}
	if state != nil {
		durable["status"] = state.Status
		durable["last_seen_status"] = state.LastSeenStatus
		durable["last_event_type"] = state.LastEventType
		durable["last_event_source"] = state.LastEventSource
		durable["connected"] = state.Connected
		durable["logged_in"] = state.LoggedIn
		durable["pairing_active"] = state.PairingActive
		durable["disconnect_reason"] = state.DisconnectReason
		durable["last_error"] = state.LastError
		durable["last_seen_at"] = state.LastSeenAt
		durable["last_event_at"] = state.LastEventAt
		durable["last_connected_at"] = state.LastConnectedAt
		durable["last_disconnected_at"] = state.LastDisconnectedAt
		durable["last_paired_at"] = state.LastPairedAt
		durable["last_logout_at"] = state.LastLogoutAt
	}

	var live any = nil
	if snapshot != nil {
		livePayload := gin.H{}
		enrichPayloadWithSnapshot(livePayload, snapshot)
		livePayload["available"] = true
		livePayload["bridge_dependent"] = true
		live = livePayload
		enrichPayloadWithSnapshot(payload, snapshot)
	}

	payload["durable"] = durable
	payload["live"] = live
	payload["observability"] = gin.H{
		"durable_status_available": state != nil,
		"live_status_available":    snapshot != nil,
		"history_durable":          true,
		"bridge_required_for_live": true,
	}
	payload["operator_message"] = "runtime status reflects durable SaaS state plus optional live bridge state"

	return buildLegacyCompatibleEnvelope("success", payload)
}

func buildRuntimeHistoryEnvelope(instance *repository.Instance, events []repository.RuntimeSessionEvent) gin.H {
	items := make([]gin.H, 0, len(events))
	for _, event := range events {
		item := gin.H{
			"id":                event.ID,
			"event_type":        event.EventType,
			"event_source":      event.EventSource,
			"status":            event.Status,
			"connected":         event.Connected,
			"logged_in":         event.LoggedIn,
			"pairing_active":    event.PairingActive,
			"disconnect_reason": event.DisconnectReason,
			"error_message":     event.ErrorMessage,
			"message":           event.Message,
			"payload":           decodeJSONText(event.Payload),
			"occurred_at":       event.OccurredAt,
		}
		items = append(items, item)
	}

	payload := buildStatusPayload(instance)
	payload["history"] = items
	payload["history_count"] = len(items)
	payload["history_durable"] = true
	payload["live_bridge_required_for_new_events"] = true
	payload["operator_message"] = "runtime history is durable for stored events; new live events still depend on the bridge"

	return buildLegacyCompatibleEnvelope("success", payload)
}

func buildHistoryBackfillEnvelope(instance *repository.Instance, result *HistoryBackfillResult, anchorSource string) gin.H {
	payload := buildStatusPayload(instance)
	payload["accepted"] = result != nil && result.Accepted
	payload["action"] = "history_backfill"
	payload["anchor_source"] = strings.TrimSpace(anchorSource)
	payload["bridge_dependent"] = true
	payload["historical_ingestion"] = "history_sync"
	payload["operator_message"] = "history backfill was requested from the live bridge; durable history will update only if the bridge returns a sync blob"
	if result != nil {
		payload["chat_jid"] = result.ChatJID
		payload["anchor_message_id"] = result.AnchorMessageID
		payload["anchor_timestamp"] = result.AnchorTimestamp
		payload["count"] = result.Count
	}

	return buildLegacyCompatibleEnvelope("history backfill requested", payload)
}

func buildQRCodePayload(instance *repository.Instance, snapshot *RuntimeSnapshot) gin.H {
	payload := buildStatusPayload(instance)
	payload["qrcode"] = ""
	payload["code"] = ""

	if snapshot == nil {
		return payload
	}

	if strings.TrimSpace(snapshot.Status) != "" {
		payload["status"] = snapshot.Status
	}
	payload["connected"] = snapshot.Connected && snapshot.LoggedIn
	if strings.TrimSpace(snapshot.QRCode) != "" {
		payload["qrcode"] = snapshot.QRCode
	}
	if strings.TrimSpace(snapshot.PairingCode) != "" {
		payload["code"] = snapshot.PairingCode
	}
	return payload
}

func enrichPayloadWithSnapshot(payload gin.H, snapshot *RuntimeSnapshot) {
	if payload == nil || snapshot == nil {
		return
	}

	if token := strings.TrimSpace(snapshot.Token); token != "" {
		payload["apikey"] = token
		payload["apiKey"] = token
		payload["token"] = token
	}
	if jid := strings.TrimSpace(snapshot.JID); jid != "" {
		payload["owner"] = jid
		payload["jid"] = jid
	}
	if profileName := strings.TrimSpace(snapshot.ProfileName); profileName != "" {
		payload["profileName"] = profileName
	}
	if webhook := strings.TrimSpace(snapshot.Webhook); webhook != "" {
		payload["webhook"] = webhook
		payload["webhook_url"] = webhook
	}
	if events := strings.TrimSpace(snapshot.Events); events != "" {
		payload["events"] = events
	}
	if strings.TrimSpace(snapshot.Status) != "" {
		payload["status"] = snapshot.Status
	}
	payload["connected"] = snapshot.Connected && snapshot.LoggedIn
	if strings.TrimSpace(snapshot.QRCode) != "" {
		payload["qrcode"] = snapshot.QRCode
	}
	if strings.TrimSpace(snapshot.PairingCode) != "" {
		payload["code"] = snapshot.PairingCode
	}
	payload["alwaysOnline"] = snapshot.AlwaysOnline
	payload["rejectCall"] = snapshot.RejectCall
	payload["readMessages"] = snapshot.ReadMessages
	payload["ignoreGroups"] = snapshot.IgnoreGroups
	payload["ignoreStatus"] = snapshot.IgnoreStatus
}

func writeLegacyCompatibleEnvelope(c *gin.Context, statusCode int, message string, payload gin.H) {
	sharedhandler.WriteJSON(c, statusCode, buildLegacyCompatibleEnvelope(message, payload))
}

func buildLegacyCompatibleEnvelope(message string, payload gin.H) gin.H {
	response := gin.H{
		"message": message,
		"data":    payload,
	}
	for key, value := range payload {
		response[key] = value
	}
	return response
}

func runtimeHistoryLimit(c *gin.Context) int {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return 50
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}

func decodeJSONText(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	return payload
}

func decodeHistoryBackfillInput(c *gin.Context) (HistoryBackfillInput, error) {
	var payload historyBackfillRequestPayload
	rawBody, err := readRequestBody(c)
	if err != nil {
		return HistoryBackfillInput{}, err
	}
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			return HistoryBackfillInput{}, err
		}
	}

	input := HistoryBackfillInput{
		ChatJID:   strings.TrimSpace(firstNonEmptyHandlerString(payload.ChatJID, payload.Chat, payload.RemoteJID)),
		MessageID: strings.TrimSpace(payload.MessageID),
		Count:     payload.Count,
	}

	if payload.IsFromMe != nil {
		input.IsFromMe = *payload.IsFromMe
	}
	if payload.IsGroup != nil {
		input.IsGroup = *payload.IsGroup
	}

	if payload.MessageInfo != nil {
		input.ChatJID = strings.TrimSpace(firstNonEmptyHandlerString(input.ChatJID, payload.MessageInfo.Chat))
		input.MessageID = strings.TrimSpace(firstNonEmptyHandlerString(input.MessageID, payload.MessageInfo.ID))
		if payload.MessageInfo.IsFromMe != nil {
			input.IsFromMe = *payload.MessageInfo.IsFromMe
		}
		if payload.MessageInfo.IsGroup != nil {
			input.IsGroup = *payload.MessageInfo.IsGroup
		}
		if strings.TrimSpace(payload.MessageInfo.Timestamp) != "" {
			timestamp, ok := parseHistoryBackfillTimestamp(payload.MessageInfo.Timestamp)
			if !ok {
				return HistoryBackfillInput{}, fmt.Errorf("%w: invalid messageInfo.timestamp", domain.ErrValidation)
			}
			input.Timestamp = timestamp
		}
	}

	if strings.TrimSpace(payload.Timestamp) != "" {
		timestamp, ok := parseHistoryBackfillTimestamp(payload.Timestamp)
		if !ok {
			return HistoryBackfillInput{}, fmt.Errorf("%w: invalid timestamp", domain.ErrValidation)
		}
		input.Timestamp = timestamp
	}
	if input.Count <= 0 {
		input.Count = 50
	}
	if input.Count > 200 {
		input.Count = 200
	}

	return input, nil
}

func parseHistoryBackfillTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if value > 1_000_000_000_000 {
			return time.UnixMilli(value).UTC(), true
		}
		return time.Unix(value, 0).UTC(), true
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), true
	}

	return time.Time{}, false
}

func firstNonEmptyHandlerString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *Handler) writeInstanceDetails(c *gin.Context, tenantID string, instance *repository.Instance) {
	if instance == nil {
		sharedhandler.WriteJSON(c, http.StatusOK, gin.H{})
		return
	}

	payload := gin.H{
		"id":                 instance.ID,
		"instance_id":        instance.ID,
		"instanceName":       instance.Name,
		"name":               instance.Name,
		"status":             instance.Status,
		"engine_instance_id": instance.EngineInstanceID,
		"webhook_url":        instance.WebhookURL,
		"webhook":            instance.WebhookURL,
		"connected":          instance.Status == "open" || instance.Status == "connected",
		"tenant_id":          tenantID,
	}

	if h.service.runtime == nil {
		sharedhandler.WriteJSON(c, http.StatusOK, payload)
		return
	}

	snapshot, err := h.service.runtime.Snapshot(c.Request.Context(), instance)
	if err != nil {
		sharedhandler.WriteJSON(c, http.StatusOK, payload)
		return
	}

	enrichPayloadWithSnapshot(payload, snapshot)

	sharedhandler.WriteJSON(c, http.StatusOK, payload)
}
