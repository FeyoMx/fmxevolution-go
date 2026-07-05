package audit

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List returns the tenant's audit trail, newest first.
// Query params: action (exact match), limit (<=500, default 100), offset.
func (h *Handler) List(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	filter := repository.AuditLogFilter{
		Action: strings.TrimSpace(c.Query("action")),
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

	entries, err := h.service.List(c.Request.Context(), identity.TenantID, filter)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"items": entries,
		"count": len(entries),
	})
}
