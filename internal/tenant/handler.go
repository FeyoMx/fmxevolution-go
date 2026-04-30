package tenant

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

func (h *Handler) Create(c *gin.Context) {
	var input CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "invalid tenant payload; name and slug are required", err)
		return
	}

	output, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusCreated, output)
}

func (h *Handler) Get(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	tenant, err := h.service.Get(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, tenant)
}
