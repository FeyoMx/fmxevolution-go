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
			"instance_id", requestInstanceID(c),
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

func requestInstanceID(c *gin.Context) string {
	for _, key := range []string{"instanceID", "instance_id", "id", "instanceName"} {
		if value := strings.TrimSpace(c.Param(key)); value != "" {
			return value
		}
	}
	for _, key := range []string{"instance_id", "instanceId"} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(c.GetHeader("X-Instance-ID"))
}

func shouldSkipRequestLog(path, method string, status int, latency time.Duration) bool {
	if status != 200 {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(path))
	if normalized == "" {
		return false
	}

	if latency >= 500*time.Millisecond {
		return false
	}

	if method == "GET" && isHighFrequencyStatusPath(normalized) {
		return true
	}

	if method == "POST" && strings.HasSuffix(normalized, "/chats/search") {
		return true
	}

	return false
}

func isHighFrequencyStatusPath(path string) bool {
	if strings.HasSuffix(path, "/qr") || strings.HasSuffix(path, "/qrcode") {
		return true
	}
	if strings.HasSuffix(path, "/status") || strings.HasSuffix(path, "/runtime/status") {
		return true
	}
	return strings.Contains(path, "/runtime/")
}
