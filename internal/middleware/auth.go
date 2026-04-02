package middleware

import (
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

const identityKey = "identity"

type AuthMiddleware struct {
	service *auth.Service
}

func NewAuthMiddleware(service *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{service: service}
}

func (m *AuthMiddleware) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		bearer := authorization
		if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			bearer = strings.TrimSpace(authorization[7:])
		}
		bearer = sanitizeCredentialValue(bearer)

		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.GetHeader("apikey")
		}
		apiKey = sanitizeCredentialValue(apiKey)

		identity, err := m.service.Authenticate(c.Request.Context(), bearer, apiKey)
		if err != nil {
			sharedhandler.WriteError(c, err)
			c.Abort()
			return
		}

		c.Set(identityKey, *identity)
		requestContext := domain.WithIdentity(c.Request.Context(), *identity)
		requestContext = domain.WithTenantID(requestContext, identity.TenantID)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func sanitizeCredentialValue(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "undefined", "null":
		return ""
	default:
		return value
	}
}

func MustIdentity(c *gin.Context) domain.Identity {
	value, exists := c.Get(identityKey)
	if !exists {
		return domain.Identity{}
	}
	identity, ok := value.(domain.Identity)
	if !ok {
		return domain.Identity{}
	}
	return identity
}

func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}

	return func(c *gin.Context) {
		identity := MustIdentity(c)
		role := strings.ToLower(strings.TrimSpace(identity.Role))
		if _, ok := allowed[role]; !ok {
			sharedhandler.WriteError(c, fmt.Errorf("%w: insufficient role", domain.ErrForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}
