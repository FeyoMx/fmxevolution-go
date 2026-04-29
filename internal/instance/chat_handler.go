package instance

import (
	"net/http"
	"strconv"
	"time"

	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

func writeChatSearchResult(c *gin.Context, result *ChatSearchResult) {
	if result == nil {
		sharedhandler.WriteJSON(c, http.StatusOK, []chatSearchRecord{})
		return
	}

	meta := result.Meta
	c.Header("X-Evolution-Chat-Source", meta.Source)
	c.Header("X-Evolution-Chat-Cached", strconv.FormatBool(meta.Cached))
	c.Header("X-Evolution-Chat-Stale", strconv.FormatBool(meta.Stale))
	c.Header("X-Evolution-Chat-Cache-TTL", strconv.Itoa(meta.TTLSeconds))
	c.Header("X-Evolution-Chat-Cache-Stale-TTL", strconv.Itoa(meta.StaleTTLSeconds))
	if meta.RefreshedAt != nil {
		c.Header("X-Evolution-Chat-Refreshed-At", meta.RefreshedAt.UTC().Format(time.RFC3339))
	}
	if meta.Reason != "" {
		c.Header("X-Evolution-Chat-Cache-Reason", meta.Reason)
	}
	if meta.OperatorMessage != "" {
		c.Header("X-Evolution-Operator-Message", meta.OperatorMessage)
	}

	sharedhandler.WriteJSON(c, http.StatusOK, result.Items)
}
