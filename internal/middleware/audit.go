package middleware

import (
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type auditRecorder interface {
	Record(entry repository.AuditLog)
}

// Audit records every state-changing request (POST/PUT/PATCH/DELETE) on the
// protected route group: who did it (user or API key), what (derived action +
// resource), from where (IP), and the resulting status. Reads are skipped to
// keep the log high-signal. Must run after auth + tenant middleware.
func Audit(recorder auditRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		c.Next()

		identity := MustIdentity(c)
		if identity.TenantID == "" {
			return
		}

		path := c.FullPath()
		if path == "" && c.Request.URL != nil {
			path = c.Request.URL.Path
		}

		actorType := "user"
		actorID := identity.UserID
		if identity.APIKey {
			actorType = "api_key"
			actorID = ""
		}

		action := auditAction(method, path)
		if action == "" {
			return
		}

		requestID, _ := c.Get("request_id")
		requestIDValue, _ := requestID.(string)

		recorder.Record(repository.AuditLog{
			TenantID:   identity.TenantID,
			ActorType:  actorType,
			ActorID:    actorID,
			ActorEmail: identity.Email,
			Action:     action,
			Method:     method,
			Path:       path,
			ResourceID: auditResourceID(c),
			Status:     c.Writer.Status(),
			RequestID:  requestIDValue,
			ClientIP:   c.ClientIP(),
		})
	}
}

// auditAction maps a route to a stable, queryable action name.
func auditAction(method, path string) string {
	p := strings.ToLower(path)

	switch {
	case strings.Contains(p, "/messages/text"),
		strings.Contains(p, "/messages/media"),
		strings.Contains(p, "/messages/audio"),
		strings.Contains(p, "/message/sendtext"),
		strings.Contains(p, "/message/sendmedia"),
		strings.Contains(p, "/message/sendwhatsappaudio"):
		return "message.send"
	case strings.Contains(p, "/message/markread"):
		return "message.markread"
	case strings.Contains(p, "/message/presence"), strings.Contains(p, "/setpresence"):
		return "instance.presence"
	case strings.HasSuffix(p, "/connect"):
		return "instance.connect"
	case strings.HasSuffix(p, "/disconnect"):
		return "instance.disconnect"
	case strings.HasSuffix(p, "/reconnect"):
		return "instance.reconnect"
	case strings.HasSuffix(p, "/pair"):
		return "instance.pair"
	case strings.HasSuffix(p, "/logout") && strings.Contains(p, "/instance"):
		return "instance.logout"
	case strings.HasSuffix(p, "/history/backfill"):
		return "instance.backfill"
	case method == "DELETE" && strings.Contains(p, "/instance"):
		return "instance.delete"
	case method == "POST" && p == "/instance":
		return "instance.create"
	case strings.Contains(p, "/instance") && (method == "PUT" || method == "PATCH"):
		return "instance.config_change"
	case p == "/webhook" && method == "POST":
		return "webhook.create"
	case strings.Contains(p, "/webhook/inbound"), strings.Contains(p, "/webhook/outbound"):
		return "webhook.dispatch"
	case strings.HasPrefix(p, "/broadcast"):
		return "broadcast.create"
	case strings.HasPrefix(p, "/contacts"):
		return "contact.write"
	case strings.HasPrefix(p, "/ai/"):
		return "ai.config_change"
	case p == "/auth/logout":
		return "auth.logout"
	case strings.Contains(p, "/chats/search"), strings.Contains(p, "/messages/search"),
		strings.Contains(p, "/chat/findchats"), strings.Contains(p, "/chat/findmessages"):
		return "" // searches are reads via POST; skip
	default:
		return "http." + strings.ToLower(method)
	}
}

func auditResourceID(c *gin.Context) string {
	for _, key := range []string{"id", "instanceID", "instanceName", "resourceId"} {
		if value := strings.TrimSpace(c.Param(key)); value != "" {
			return value
		}
	}
	return ""
}
