package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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

func (h *Handler) Login(c *gin.Context) {
	input, err := decodeLoginInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido; usa tenant_slug, email y password", err)
		return
	}

	output, err := h.service.Login(c.Request.Context(), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, output)
}

func (h *Handler) Refresh(c *gin.Context) {
	input, err := decodeRefreshInput(c)
	if err != nil {
		sharedhandler.WriteValidationError(c, "payload inválido; usa refresh_token", err)
		return
	}

	output, err := h.service.Refresh(c.Request.Context(), input)
	if err != nil {
		sharedhandler.WriteError(c, err)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, output)
}

func (h *Handler) Me(c *gin.Context) {
	identity, ok := domain.IdentityFromContext(c.Request.Context())
	if !ok {
		sharedhandler.WriteError(c, domain.ErrUnauthorized)
		return
	}

	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"user_id":      identity.UserID,
		"tenant_id":    identity.TenantID,
		"email":        identity.Email,
		"role":         identity.Role,
		"api_key":      identity.APIKey,
		"api_key_auth": identity.APIKey,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	sharedhandler.WriteJSON(c, http.StatusOK, gin.H{
		"message":  "logout exitoso",
		"accepted": true,
	})
}

func decodeLoginInput(c *gin.Context) (LoginInput, error) {
	input := LoginInput{
		TenantSlug: c.PostForm("tenant_slug"),
		Email:      c.PostForm("email"),
		Password:   c.PostForm("password"),
	}

	if input.TenantSlug == "" {
		input.TenantSlug = c.PostForm("tenant")
	}
	if input.TenantSlug == "" {
		input.TenantSlug = c.Query("tenant_slug")
	}
	if input.TenantSlug == "" {
		input.TenantSlug = c.Query("tenant")
	}
	if input.TenantSlug == "" {
		input.TenantSlug = c.GetHeader("X-Tenant-Slug")
	}
	if input.Email == "" {
		input.Email = c.Query("email")
	}
	if input.Password == "" {
		input.Password = c.Query("password")
	}

	rawBody, err := readRequestBody(c)
	if err != nil {
		return LoginInput{}, err
	}
	if len(rawBody) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err == nil {
			mergeLoginPayload(&input, payload)
		}
	}

	input.Normalize()
	return input, nil
}

func decodeRefreshInput(c *gin.Context) (RefreshInput, error) {
	input := RefreshInput{
		RefreshToken: c.PostForm("refresh_token"),
	}
	if input.RefreshToken == "" {
		input.RefreshToken = c.PostForm("refreshToken")
	}
	if input.RefreshToken == "" {
		input.RefreshToken = c.Query("refresh_token")
	}
	if input.RefreshToken == "" {
		input.RefreshToken = c.Query("refreshToken")
	}

	rawBody, err := readRequestBody(c)
	if err != nil {
		return RefreshInput{}, err
	}
	if len(rawBody) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err == nil {
			mergeRefreshPayload(&input, payload)
		}
	}

	input.Normalize()
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

func mergeLoginPayload(input *LoginInput, payload map[string]any) {
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

	setStringIfEmpty(&input.TenantSlug, "tenant_slug", "tenantSlug", "tenant", "slug")
	setStringIfEmpty(&input.Email, "email", "username", "user")
	setStringIfEmpty(&input.Password, "password", "pass")
}

func mergeRefreshPayload(input *RefreshInput, payload map[string]any) {
	if input == nil {
		return
	}
	if strings.TrimSpace(input.RefreshToken) != "" {
		return
	}

	for _, key := range []string{"refresh_token", "refreshToken", "token"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			input.RefreshToken = text
			return
		}
	}
}
