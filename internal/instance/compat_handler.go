package instance

import (
	"net/http"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "invalid legacy text payload",
		})
		return
	}

	result, instance, err := h.service.SendText(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildLegacySendResponse("success", instance, result))
}

func (h *Handler) LegacySendMedia(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var payload mediaMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "invalid legacy media payload",
		})
		return
	}

	input, err := payload.normalize()
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	result, instance, err := h.service.SendMedia(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), *input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildLegacySendResponse("success", instance, result))
}

func (h *Handler) LegacySendAudio(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	var payload audioMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "invalid legacy audio payload",
		})
		return
	}

	input, err := payload.normalize()
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	result, instance, err := h.service.SendAudio(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), *input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildLegacySendResponse("success", instance, result))
}

func (h *Handler) LegacyFindChats(c *gin.Context) {
	var input ChatSearchRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	result, _, err := h.service.SearchChats(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	writeChatSearchResult(c, result)
}

func (h *Handler) LegacyFindMessages(c *gin.Context) {
	var input MessageSearchRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "invalid legacy message search payload",
		})
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	messages, _, err := h.service.SearchMessages(c.Request.Context(), identity.TenantID, legacyInstanceReferenceFromParams(c), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, messages)
}

func buildLegacySendResponse(message string, instance *repository.Instance, result any) gin.H {
	response := gin.H{
		"message": message,
		"data":    result,
	}
	if instance != nil {
		response["instance_id"] = instance.ID
		response["instanceName"] = instance.Name
		response["engine_instance_id"] = instance.EngineInstanceID
	}
	return response
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
