package ai

import (
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetTenantSettings(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	settings, err := h.service.GetTenantSettings(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, settings)
}

func (h *Handler) ConfigureTenant(c *gin.Context) {
	var input TenantSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	settings, err := h.service.ConfigureTenant(c.Request.Context(), identity.TenantID, input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, settings)
}

func (h *Handler) GetInstanceSettings(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.GetInstanceSettings(c.Request.Context(), identity.TenantID, c.Param("instanceID"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"instance_id": c.Param("instanceID"),
		"enabled":     instance.AIEnabled,
		"auto_reply":  instance.AIAutoReply,
	})
}

func (h *Handler) ConfigureInstance(c *gin.Context) {
	var input InstanceSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	instance, err := h.service.ConfigureInstance(c.Request.Context(), identity.TenantID, c.Param("instanceID"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"instance_id": instance.ID,
		"enabled":     instance.AIEnabled,
		"auto_reply":  instance.AIAutoReply,
	})
}
