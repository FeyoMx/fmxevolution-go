package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var corsAllowedHeaders = strings.Join([]string{
	"Origin",
	"Content-Type",
	"Accept",
	"Authorization",
	"X-API-Key",
	"apikey",
	"X-Platform-Key",
	"X-Tenant-ID",
	"X-Tenant-Slug",
	"X-Instance-ID",
	"X-Requested-With",
}, ", ")

var corsAllowedMethods = strings.Join(
	[]string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, ", ",
)

// CORS builds a CORS middleware backed by an explicit origin allowlist.
//
// Security note: reflecting an arbitrary Origin together with
// Access-Control-Allow-Credentials: true lets any website issue authenticated
// cross-origin requests on behalf of a logged-in user. We therefore only echo
// origins that appear in the configured allowlist. When the allowlist is empty
// no cross-origin credentials are granted (same-origin browser traffic is
// unaffected because browsers do not enforce CORS for same-origin requests).
//
// A single "*" entry enables permissive mode WITHOUT credentials — useful for
// public read-only deployments but never combined with cookies/bearer reflection.
func CORS(allowedOrigins ...string) gin.HandlerFunc {
	allowAll := false
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAll = true
			continue
		}
		allow[strings.ToLower(origin)] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))

		if origin != "" {
			switch {
			case allowAll:
				c.Header("Access-Control-Allow-Origin", "*")
			default:
				if _, ok := allow[strings.ToLower(origin)]; ok {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Access-Control-Allow-Credentials", "true")
				}
			}
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", corsAllowedHeaders)
			c.Header("Access-Control-Allow-Methods", corsAllowedMethods)
			c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
