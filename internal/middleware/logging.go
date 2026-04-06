package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestLogging(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.NewString()
		c.Set("request_id", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(domain.WithRequestID(c.Request.Context(), requestID))

		start := time.Now()
		c.Next()

		identity, _ := domain.IdentityFromContext(c.Request.Context())
		path := c.FullPath()
		if path == "" && c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		if shouldSkipRequestLog(path, c.Request.Method, c.Writer.Status(), time.Since(start)) {
			return
		}
		logger.Info("http request",
			"request_id", requestID,
			"tenant_id", identity.TenantID,
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

func shouldSkipRequestLog(path, method string, status int, latency time.Duration) bool {
	if method != "GET" || status != 200 {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(path))
	if normalized == "" {
		return false
	}

	// The frontend polls QR routes very aggressively while the session is already
	// connected. Successful low-latency responses add little operational value and
	// can drown out real errors in the backend logs.
	if (strings.HasSuffix(normalized, "/qr") || strings.HasSuffix(normalized, "/qrcode")) && latency < 500*time.Millisecond {
		return true
	}

	return false
}
