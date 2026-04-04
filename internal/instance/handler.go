package instance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "payload inválido; usa name o instanceName",
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "payload inválido para advanced settings",
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "payload inválido para envío de texto",
		})
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
		"delivery_status":    job.Status,
		"sent":               job.Status == "sent",
		"delivery_confirmed": false,
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
	response := gin.H{
		"message": message,
		"data":    payload,
	}
	for key, value := range payload {
		response[key] = value
	}
	sharedhandler.WriteJSON(c, statusCode, response)
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
