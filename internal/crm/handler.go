package crm

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

func (h *Handler) ListContacts(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	contacts, err := h.service.ListContacts(c.Request.Context(), identity.TenantID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, contacts)
}

func (h *Handler) GetContact(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	contact, err := h.service.GetContact(c.Request.Context(), identity.TenantID, c.Param("id"))
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, contact)
}

func (h *Handler) CreateContact(c *gin.Context) {
	var input CreateContactInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido para contacto", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	contact, err := h.service.CreateContact(c.Request.Context(), identity.TenantID, input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusCreated, contact)
}

func (h *Handler) UpdateContact(c *gin.Context) {
	var input UpdateContactInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido para actualización de contacto", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	contact, err := h.service.UpdateContact(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, contact)
}

func (h *Handler) AddNote(c *gin.Context) {
	var input CreateNoteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido para nota", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	note, err := h.service.AddNote(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusCreated, note)
}

func (h *Handler) AssignTags(c *gin.Context) {
	var input AssignTagsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido para tags", err)
		return
	}

	identity, _ := domain.IdentityFromContext(c.Request.Context())
	contact, err := h.service.AssignTags(c.Request.Context(), identity.TenantID, c.Param("id"), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}
	sharedhandler.WriteJSON(c, http.StatusOK, contact)
}
