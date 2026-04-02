package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

type RateLimitPolicy struct {
	Name   string
	Limit  int
	Window time.Duration
}

type RateLimitDecision struct {
	Allowed     bool
	Remaining   int
	ResetAt     time.Time
	Scope       string
	Policy      string
	Limit       int
	CurrentHits int
}

type RateLimitStore interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (RateLimitDecision, error)
}

type MemoryRateLimitStore struct {
	mu      sync.Mutex
	entries map[string]memoryRateLimitEntry
}

type memoryRateLimitEntry struct {
	count     int
	windowEnd time.Time
}

type RedisRateLimitStore struct{}

func NewRateLimitStore(backend string) RateLimitStore {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "memory":
		return NewMemoryRateLimitStore()
	case "redis":
		// Redis can be plugged in later behind the same interface.
		return NewMemoryRateLimitStore()
	default:
		return NewMemoryRateLimitStore()
	}
}

func NewMemoryRateLimitStore() *MemoryRateLimitStore {
	return &MemoryRateLimitStore{
		entries: make(map[string]memoryRateLimitEntry),
	}
}

func (s *MemoryRateLimitStore) Allow(_ context.Context, key string, limit int, window time.Duration) (RateLimitDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	entry := s.entries[key]
	if entry.windowEnd.IsZero() || now.After(entry.windowEnd) {
		entry = memoryRateLimitEntry{
			count:     0,
			windowEnd: now.Add(window),
		}
	}

	entry.count++
	s.entries[key] = entry

	allowed := entry.count <= limit
	remaining := limit - entry.count
	if remaining < 0 {
		remaining = 0
	}

	return RateLimitDecision{
		Allowed:     allowed,
		Remaining:   remaining,
		ResetAt:     entry.windowEnd,
		Limit:       limit,
		CurrentHits: entry.count,
	}, nil
}

func (RedisRateLimitStore) Allow(context.Context, string, int, time.Duration) (RateLimitDecision, error) {
	return RateLimitDecision{}, fmt.Errorf("redis rate limit store not implemented yet")
}

type RateLimiter struct {
	store  RateLimitStore
	policy RateLimitPolicy
}

func NewRateLimiter(store RateLimitStore, policy RateLimitPolicy) *RateLimiter {
	return &RateLimiter{
		store:  store,
		policy: policy,
	}
}

func MessageRateLimitPolicy(limit int) RateLimitPolicy {
	return RateLimitPolicy{
		Name:   "messages_per_minute",
		Limit:  limit,
		Window: time.Minute,
	}
}

func BroadcastRateLimitPolicy(limit int) RateLimitPolicy {
	return RateLimitPolicy{
		Name:   "broadcast_per_hour",
		Limit:  limit,
		Window: time.Hour,
	}
}

func WebhookRateLimitPolicy(limit int) RateLimitPolicy {
	return RateLimitPolicy{
		Name:   "webhook_calls_per_minute",
		Limit:  limit,
		Window: time.Minute,
	}
}

func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := MustIdentity(c)
		if identity.TenantID == "" {
			sharedhandler.WriteError(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}

		instanceID, err := extractInstanceID(c)
		if err != nil {
			sharedhandler.WriteError(c, fmt.Errorf("%w: invalid request body for rate limiting", domain.ErrValidation))
			c.Abort()
			return
		}

		tenantKey := scopedRateLimitKey(r.policy.Name, "tenant", identity.TenantID)
		tenantDecision, err := r.store.Allow(c.Request.Context(), tenantKey, r.policy.Limit, r.policy.Window)
		if err != nil {
			sharedhandler.WriteError(c, err)
			c.Abort()
			return
		}
		tenantDecision.Scope = "tenant"
		tenantDecision.Policy = r.policy.Name
		if !tenantDecision.Allowed {
			abortRateLimited(c, tenantDecision)
			return
		}

		if instanceID != "" {
			instanceKey := scopedRateLimitKey(r.policy.Name, "instance", identity.TenantID+":"+instanceID)
			instanceDecision, err := r.store.Allow(c.Request.Context(), instanceKey, r.policy.Limit, r.policy.Window)
			if err != nil {
				sharedhandler.WriteError(c, err)
				c.Abort()
				return
			}
			instanceDecision.Scope = "instance"
			instanceDecision.Policy = r.policy.Name
			if !instanceDecision.Allowed {
				abortRateLimited(c, instanceDecision)
				return
			}

			c.Set("rate_limit_instance_id", instanceID)
			c.Writer.Header().Set("X-RateLimit-Instance-Remaining", fmt.Sprintf("%d", instanceDecision.Remaining))
		}

		c.Writer.Header().Set("X-RateLimit-Policy", r.policy.Name)
		c.Writer.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", tenantDecision.Limit))
		c.Writer.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", tenantDecision.Remaining))
		c.Writer.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", tenantDecision.ResetAt.Unix()))
		c.Next()
	}
}

func scopedRateLimitKey(policyName, scope, subject string) string {
	return strings.Join([]string{"rate_limit", policyName, scope, subject}, ":")
}

func abortRateLimited(c *gin.Context, decision RateLimitDecision) {
	c.Header("Retry-After", fmt.Sprintf("%d", int(time.Until(decision.ResetAt).Seconds())))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":       "rate limit exceeded",
		"policy":      decision.Policy,
		"scope":       decision.Scope,
		"limit":       decision.Limit,
		"remaining":   decision.Remaining,
		"retry_after": int(time.Until(decision.ResetAt).Seconds()),
		"reset_at":    decision.ResetAt.UTC(),
	})
}

func extractInstanceID(c *gin.Context) (string, error) {
	if value := strings.TrimSpace(c.GetHeader("X-Instance-ID")); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(c.Param("instance_id")); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(c.Query("instance_id")); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(c.Query("instanceId")); value != "" {
		return value, nil
	}

	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return "", nil
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return "", err
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	for _, key := range []string{"instance_id", "instanceId"} {
		if raw, ok := payload[key]; ok {
			if value, ok := raw.(string); ok {
				return strings.TrimSpace(value), nil
			}
		}
	}

	return "", nil
}
