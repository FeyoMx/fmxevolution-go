package instance

import (
	"net/http"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

type groupRecord struct {
	RemoteJID    string `json:"remoteJid"`
	PushName     string `json:"pushName"`
	InstanceID   string `json:"instanceId"`
	MessageCount int64  `json:"messageCount"`
	LastMessage  string `json:"lastMessageAt"`
}

// LegacyFetchAllGroups handles GET /group/fetchAllGroups/:instanceName
func (h *Handler) LegacyFetchAllGroups(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	reference := legacyInstanceReferenceFromParams(c)

	instance, err := h.service.resolve(c.Request.Context(), identity.TenantID, reference)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	summaries, err := h.service.FetchGroups(c.Request.Context(), instance.ID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	groups := make([]groupRecord, 0, len(summaries))
	for _, s := range summaries {
		name := s.PushName
		if name == "" || name == s.RemoteJID {
			name = strings.TrimSuffix(s.RemoteJID, "@g.us")
		}
		groups = append(groups, groupRecord{
			RemoteJID:    s.RemoteJID,
			PushName:     name,
			InstanceID:   instance.ID,
			MessageCount: s.MessageCount,
			LastMessage:  s.LastMessage.UTC().Format(time.RFC3339),
		})
	}

	sharedhandler.WriteJSON(c, http.StatusOK, groups)
}

type findGroupRecord struct {
	ID           string `json:"id"`
	Subject      string `json:"subject"`
	RemoteJID    string `json:"remoteJid"`
	InstanceID   string `json:"instanceId"`
	MessageCount int64  `json:"messageCount"`
	LastMessage  string `json:"lastMessageAt"`
}

// LegacyFindGroup handles GET /v2/group/findGroup/:instanceName
// Returns all groups with subject (display name) field for Evolution API v2 compatibility.
func (h *Handler) LegacyFindGroup(c *gin.Context) {
	identity, _ := domain.IdentityFromContext(c.Request.Context())
	reference := legacyInstanceReferenceFromParams(c)

	instance, err := h.service.resolve(c.Request.Context(), identity.TenantID, reference)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	summaries, err := h.service.FetchGroups(c.Request.Context(), instance.ID)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	groups := make([]findGroupRecord, 0, len(summaries))
	for _, s := range summaries {
		name := s.PushName
		if name == "" || name == s.RemoteJID {
			name = strings.TrimSuffix(s.RemoteJID, "@g.us")
		}
		groups = append(groups, findGroupRecord{
			ID:           s.RemoteJID,
			Subject:      name,
			RemoteJID:    s.RemoteJID,
			InstanceID:   instance.ID,
			MessageCount: s.MessageCount,
			LastMessage:  s.LastMessage.UTC().Format(time.RFC3339),
		})
	}

	sharedhandler.WriteJSON(c, http.StatusOK, groups)
}

