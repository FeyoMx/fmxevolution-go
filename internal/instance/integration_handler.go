package instance

import (
	"errors"
	"net/http"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetWebsocketConfig(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	config, _, err := h.service.GetWebsocketConfig(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, config)
}

func (h *Handler) SetWebsocketConfig(c *gin.Context) {
	config, err := decodeEventConnectorUpdate(c, "websocket")
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	saved, _, err := h.service.SetWebsocketConfig(c.Request.Context(), identity.TenantID, c.Param("id"), config)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, saved)
}

func (h *Handler) GetRabbitMQConfig(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	config, _, err := h.service.GetRabbitMQConfig(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, config)
}

func (h *Handler) SetRabbitMQConfig(c *gin.Context) {
	config, err := decodeEventConnectorUpdate(c, "rabbitmq")
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	saved, _, err := h.service.SetRabbitMQConfig(c.Request.Context(), identity.TenantID, c.Param("id"), config)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, saved)
}

func (h *Handler) GetProxyConfig(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	config, _, err := h.service.GetProxyConfig(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, config)
}

func (h *Handler) SetProxyConfig(c *gin.Context) {
	var input ProxyConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteError(c, errors.Join(domain.ErrValidation, err))
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	saved, _, err := h.service.SetProxyConfig(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, saved)
}

func (h *Handler) SearchChats(c *gin.Context) {
	var input ChatSearchRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteError(c, errors.Join(domain.ErrValidation, err))
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	chats, _, err := h.service.SearchChats(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, chats)
}

func (h *Handler) SearchMessages(c *gin.Context) {
	h.writePartialFeature(c, "chat", []string{
		"tenant-safe message search is not available in the current SaaS persistence layer",
		"the current backend does not store the Message[] contract required by the legacy frontend",
	})
}

func (h *Handler) SendMediaMessage(c *gin.Context) {
	var payload mediaMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		sharedhandler.WriteError(c, errors.Join(domain.ErrValidation, err))
		return
	}

	input, err := payload.normalize()
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	result, instance, err := h.service.SendMedia(c.Request.Context(), identity.TenantID, c.Param("id"), *input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildInstanceMessageResponse("media sent successfully", instance, result))
}

func (h *Handler) SendAudioMessage(c *gin.Context) {
	var payload audioMessageEnvelope
	if err := c.ShouldBindJSON(&payload); err != nil {
		sharedhandler.WriteError(c, errors.Join(domain.ErrValidation, err))
		return
	}

	input, err := payload.normalize()
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	result, instance, err := h.service.SendAudio(c.Request.Context(), identity.TenantID, c.Param("id"), *input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, buildInstanceMessageResponse("audio sent successfully", instance, result))
}

func (h *Handler) GetSQSConfig(c *gin.Context) {
	h.writePartialFeature(c, "sqs", []string{
		"no tenant-safe SQS configuration model exists in the current backend",
		"the legacy runtime bridge does not expose per-instance SQS settings",
	})
}

func (h *Handler) SetSQSConfig(c *gin.Context) {
	h.writePartialFeature(c, "sqs", []string{
		"no tenant-safe SQS configuration model exists in the current backend",
		"the legacy runtime bridge does not expose per-instance SQS settings",
	})
}

func (h *Handler) GetChatwootConfig(c *gin.Context) {
	h.writePartialFeature(c, "chatwoot", []string{
		"no tenant-safe Chatwoot configuration model exists in the current backend",
		"the legacy runtime bridge does not expose Chatwoot settings",
	})
}

func (h *Handler) SetChatwootConfig(c *gin.Context) {
	h.writePartialFeature(c, "chatwoot", []string{
		"no tenant-safe Chatwoot configuration model exists in the current backend",
		"the legacy runtime bridge does not expose Chatwoot settings",
	})
}

func (h *Handler) ListOpenAIResources(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "openai") }
func (h *Handler) CreateOpenAIResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "openai")
}
func (h *Handler) GetOpenAIResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "openai") }
func (h *Handler) UpdateOpenAIResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "openai")
}
func (h *Handler) DeleteOpenAIResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "openai")
}
func (h *Handler) GetOpenAISettings(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "openai") }
func (h *Handler) UpdateOpenAISettings(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "openai")
}
func (h *Handler) ListOpenAISessions(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "openai") }
func (h *Handler) ChangeOpenAIStatus(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "openai") }
func (h *Handler) ListTypebotResources(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) CreateTypebotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) GetTypebotResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "typebot") }
func (h *Handler) UpdateTypebotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) DeleteTypebotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) GetTypebotSettings(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "typebot") }
func (h *Handler) UpdateTypebotSettings(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) ListTypebotSessions(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) ChangeTypebotStatus(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "typebot")
}
func (h *Handler) ListDifyResources(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) CreateDifyResource(c *gin.Context)  { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) GetDifyResource(c *gin.Context)     { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) UpdateDifyResource(c *gin.Context)  { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) DeleteDifyResource(c *gin.Context)  { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) GetDifySettings(c *gin.Context)     { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) UpdateDifySettings(c *gin.Context)  { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) ListDifySessions(c *gin.Context)    { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) ChangeDifyStatus(c *gin.Context)    { h.writeUnsupportedResourceFeature(c, "dify") }
func (h *Handler) ListN8NResources(c *gin.Context)    { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) CreateN8NResource(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) GetN8NResource(c *gin.Context)      { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) UpdateN8NResource(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) DeleteN8NResource(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) GetN8NSettings(c *gin.Context)      { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) UpdateN8NSettings(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) ListN8NSessions(c *gin.Context)     { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) ChangeN8NStatus(c *gin.Context)     { h.writeUnsupportedResourceFeature(c, "n8n") }
func (h *Handler) ListEvoAIResources(c *gin.Context)  { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) CreateEvoAIResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) GetEvoAIResource(c *gin.Context)    { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) UpdateEvoAIResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) DeleteEvoAIResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) GetEvoAISettings(c *gin.Context)    { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) UpdateEvoAISettings(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) ListEvoAISessions(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) ChangeEvoAIStatus(c *gin.Context)   { h.writeUnsupportedResourceFeature(c, "evoai") }
func (h *Handler) ListEvolutionBotResources(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) CreateEvolutionBotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) GetEvolutionBotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) UpdateEvolutionBotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) DeleteEvolutionBotResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) GetEvolutionBotSettings(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) UpdateEvolutionBotSettings(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) ListEvolutionBotSessions(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) ChangeEvolutionBotStatus(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "evolutionBot")
}
func (h *Handler) ListFlowiseResources(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) CreateFlowiseResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) GetFlowiseResource(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "flowise") }
func (h *Handler) UpdateFlowiseResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) DeleteFlowiseResource(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) GetFlowiseSettings(c *gin.Context) { h.writeUnsupportedResourceFeature(c, "flowise") }
func (h *Handler) UpdateFlowiseSettings(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) ListFlowiseSessions(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}
func (h *Handler) ChangeFlowiseStatus(c *gin.Context) {
	h.writeUnsupportedResourceFeature(c, "flowise")
}

func decodeEventConnectorUpdate(c *gin.Context, feature string) (EventConnectorConfig, error) {
	var input EventConnectorUpdateEnvelope
	if err := c.ShouldBindJSON(&input); err != nil {
		return EventConnectorConfig{}, errors.Join(domain.ErrValidation, err)
	}

	selected := eventConnectorPayload{
		Enabled: input.Enabled,
		Events:  input.Events,
	}

	switch strings.ToLower(strings.TrimSpace(feature)) {
	case "websocket":
		if input.Websocket != nil {
			selected = *input.Websocket
		}
	case "rabbitmq":
		if input.Rabbitmq != nil {
			selected = *input.Rabbitmq
		}
	case "sqs":
		if input.SQS != nil {
			selected = *input.SQS
		}
	}

	if selected.Enabled == nil {
		return EventConnectorConfig{}, errors.Join(domain.ErrValidation, errors.New("enabled is required"))
	}

	return EventConnectorConfig{
		Enabled: *selected.Enabled,
		Events:  selected.Events,
	}, nil
}

func (h *Handler) writeUnsupportedResourceFeature(c *gin.Context, feature string) {
	h.writePartialFeature(c, feature, []string{
		"the frontend gap report requires CRUD, settings, sessions, and status routes for this integration",
		"the current SaaS backend does not have a tenant-safe repository/runtime contract for this integration",
	})
}

func (h *Handler) writePartialFeature(c *gin.Context, feature string, blockedBy []string) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.Resolve(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusNotImplemented, PartialFeatureResponse{
		Feature:      feature,
		Status:       "partial",
		Implemented:  false,
		Message:      "This route is intentionally registered as a partial implementation because the current backend cannot complete it safely without reviving unsupported legacy patterns.",
		InstanceID:   instance.ID,
		InstanceName: instance.Name,
		BlockedBy:    blockedBy,
	})
}
