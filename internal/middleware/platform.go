package middleware

import (
	"crypto/subtle"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

// PlatformGuard restricts a route to callers holding the platform-level API key.
// It is used for agency-wide operations (e.g. creating tenants) that must never
// be exposed to the public internet or to individual tenants.
//
// The key is supplied via the X-Platform-Key header (preferred) or, for backward
// compatibility with tooling, the X-API-Key / apikey headers. Comparison is done
// in constant time to avoid timing side channels.
func PlatformGuard(platformKey string) gin.HandlerFunc {
	expected := strings.TrimSpace(platformKey)

	return func(c *gin.Context) {
		if expected == "" {
			// Fail closed: if no platform key is configured, the route is disabled
			// rather than left open.
			sharedhandler.WriteError(c, fmt.Errorf("%w: platform operations are disabled", domain.ErrForbidden))
			c.Abort()
			return
		}

		provided := sanitizeCredentialValue(c.GetHeader("X-Platform-Key"))
		if provided == "" {
			provided = sanitizeCredentialValue(c.GetHeader("X-API-Key"))
		}
		if provided == "" {
			provided = sanitizeCredentialValue(c.GetHeader("apikey"))
		}

		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			sharedhandler.WriteError(c, fmt.Errorf("%w: invalid platform key", domain.ErrUnauthorized))
			c.Abort()
			return
		}

		c.Next()
	}
}
