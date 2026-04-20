package broadcast

import (
	"net/http"
	"strconv"

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
		sharedhandler.WriteValidationError(c, "payload inválido para broadcast", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	job, err := h.service.Create(c.Request.Context(), identity.TenantID, input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusCreated, job)
}

func (h *Handler) List(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	jobs, err := h.service.List(c.Request.Context(), identity.TenantID, limit)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, jobs)
}

func (h *Handler) Get(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	job, err := h.service.Get(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, job)
}

func (h *Handler) ListRecipients(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		sharedhandler.WriteValidationError(c, "query inválida para recipients de broadcast", err)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		sharedhandler.WriteValidationError(c, "query inválida para recipients de broadcast", err)
		return
	}

	result, err := h.service.ListRecipients(c.Request.Context(), identity.TenantID, c.Param("id"), ListRecipientsInput{
		Page:   page,
		Limit:  limit,
		Status: c.Query("status"),
		Query:  c.Query("query"),
	})
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, result)
}
