package instance

import (
	"errors"
	"net/http"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

func legacyInstanceReferenceFromParams(c *gin.Context) string {
	if ref := strings.TrimSpace(c.Param("instanceName")); ref != "" {
		return ref
	}
	return instanceReferenceFromParams(c)
}

func (h *Handler) LegacySendText(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var input SendTextInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid legacy text payload; number and text are required", err)
		return
	}

	result, instance, err := h.service.SendText(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	writeEvolutionCompatSuccess(c, http.StatusOK, "message sent successfully", buildLegacySendData(instance, result))
}

func (h *Handler) LegacySendMedia(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var payload mediaMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid legacy media payload", err)
		return
	}

	input, err := payload.normalize()
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	result, instance, err := h.service.SendMedia(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), *input)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	writeEvolutionCompatSuccess(c, http.StatusOK, "media sent successfully", buildLegacySendData(instance, result))
}

func (h *Handler) LegacySendAudio(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var payload audioMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid legacy audio payload", err)
		return
	}

	input, err := payload.normalize()
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	result, instance, err := h.service.SendAudio(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), *input)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	writeEvolutionCompatSuccess(c, http.StatusOK, "audio sent successfully", buildLegacySendData(instance, result))
}

func (h *Handler) LegacyFindChats(c *gin.Context) {
	var input ChatSearchRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid legacy chat search payload", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	result, _, err := h.service.SearchChats(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	writeChatSearchResult(c, result)
}

func (h *Handler) LegacyFindMessages(c *gin.Context) {
	var input MessageSearchRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid legacy message search payload", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	messages, _, err := h.service.SearchMessages(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, messages)
}

func (h *Handler) LegacySetPresence(c *gin.Context) {
	var payload evolutionPresencePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeEvolutionCompatError(c, http.StatusBadRequest, "invalid presence payload", err)
		return
	}

	desired, chatState, err := normalizeEvolutionPresence(payload)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}
	if chatState != "" {
		writeEvolutionCompatError(c, http.StatusNotImplemented, "chat presence is not supported by cmd/api compatibility routes", errors.New("unsupported_chat_presence"))
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	updated, instance, applied, err := h.service.SetAlwaysOnlineCompat(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), desired)
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	writeEvolutionCompatSuccess(c, http.StatusOK, "presence updated successfully", buildPresenceCompatData(instance, updated, desired, applied))
}

func (h *Handler) LegacyChatPresence(c *gin.Context) {
	h.LegacySetPresence(c)
}

func (h *Handler) LegacyMarkRead(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Resolve(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c))
	if err != nil {
		writeEvolutionCompatServiceError(c, err)
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"message": "markread is not implemented in cmd/api compatibility routes",
		"error":   "unsupported_markread",
		"data": gin.H{
			"instance_id":        instance.ID,
			"instanceName":       instance.Name,
			"engine_instance_id": instance.EngineInstanceID,
			"implemented":        false,
			"code":               "unsupported_markread",
		},
	})
}

type evolutionPresencePayload struct {
	Presence     string `json:"presence"`
	State        string `json:"state"`
	AlwaysOnline *bool  `json:"alwaysOnline"`
}

func normalizeEvolutionPresence(payload evolutionPresencePayload) (bool, string, error) {
	if payload.AlwaysOnline != nil {
		return *payload.AlwaysOnline, "", nil
	}

	value := strings.ToLower(strings.TrimSpace(firstNonEmpty(payload.Presence, payload.State)))
	switch value {
	case "available", "online", "true", "1", "yes":
		return true, "", nil
	case "unavailable", "offline", "false", "0", "no":
		return false, "", nil
	case "composing", "paused", "recording":
		return false, value, nil
	default:
		return false, "", errors.Join(domain.ErrValidation, errors.New("presence, state, or alwaysOnline is required"))
	}
}

func buildLegacySendData(instance *repository.Instance, result any) gin.H {
	data := gin.H{
		"result": result,
	}
	if instance != nil {
		data["instance_id"] = instance.ID
		data["instanceName"] = instance.Name
		data["engine_instance_id"] = instance.EngineInstanceID
	}
	return data
}

func buildPresenceCompatData(instance *repository.Instance, settings *legacyInstanceModel.AdvancedSettings, desired bool, applied bool) gin.H {
	data := gin.H{
		"alwaysOnline": desired,
		"applied":      applied,
	}
	if settings != nil {
		data["settings"] = gin.H{
			"alwaysOnline":  settings.AlwaysOnline,
			"rejectCall":    settings.RejectCall,
			"msgRejectCall": settings.MsgRejectCall,
			"readMessages":  settings.ReadMessages,
			"ignoreGroups":  settings.IgnoreGroups,
			"ignoreStatus":  settings.IgnoreStatus,
		}
	}
	if instance != nil {
		data["instance_id"] = instance.ID
		data["instanceName"] = instance.Name
		data["engine_instance_id"] = instance.EngineInstanceID
	}
	return data
}

func writeEvolutionCompatSuccess(c *gin.Context, status int, message string, data any) {
	c.JSON(status, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

func writeEvolutionCompatServiceError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
	}
	writeEvolutionCompatError(c, status, err.Error(), err)
}

func writeEvolutionCompatError(c *gin.Context, status int, message string, err error) {
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
		"error":   errorMessage,
	})
}

func buildInstanceMessageResponse(message string, instance *repository.Instance, result *SendMediaResult) gin.H {
	payload := gin.H{
		"message": message,
		"data":    result,
	}
	if instance != nil {
		payload["instance_id"] = instance.ID
		payload["instanceName"] = instance.Name
		payload["engine_instance_id"] = instance.EngineInstanceID
	}
	if result != nil {
		payload["message_id"] = result.MessageID
		payload["server_id"] = result.ServerID
		payload["chat"] = result.Chat
		payload["messageType"] = result.MessageType
		payload["timestamp"] = result.Timestamp
	}
	return payload
}
