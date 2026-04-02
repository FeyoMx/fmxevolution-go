package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/gin-gonic/gin"
)

func TestRateLimiterLimitsPerTenantAndPerInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryRateLimitStore()
	limiter := NewRateLimiter(store, RateLimitPolicy{
		Name:   "broadcast_per_hour",
		Limit:  1,
		Window: time.Hour,
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1", Role: auth.RoleAdmin}
		c.Set(identityKey, identity)
		requestContext := domain.WithIdentity(c.Request.Context(), identity)
		requestContext = domain.WithTenantID(requestContext, "tenant-1")
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	})
	router.POST("/broadcast", limiter.Middleware(), func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	first := httptest.NewRequest(http.MethodPost, "/broadcast", strings.NewReader(`{"instance_id":"instance-1"}`))
	first.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first request to pass, got %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/broadcast", strings.NewReader(`{"instance_id":"instance-1"}`))
	second.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", secondRec.Code)
	}
}

func TestRateLimiterSeparatesDifferentInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryRateLimitStore()
	limiter := NewRateLimiter(store, RateLimitPolicy{
		Name:   "webhook_calls_per_minute",
		Limit:  2,
		Window: time.Minute,
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1", Role: auth.RoleAdmin}
		c.Set(identityKey, identity)
		requestContext := domain.WithIdentity(c.Request.Context(), identity)
		requestContext = domain.WithTenantID(requestContext, "tenant-1")
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	})
	router.POST("/webhook/inbound", limiter.Middleware(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req1 := httptest.NewRequest(http.MethodPost, "/webhook/inbound", strings.NewReader(`{"instance_id":"instance-1"}`))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/webhook/inbound", strings.NewReader(`{"instance_id":"instance-2"}`))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK || rec2.Code != http.StatusOK {
		t.Fatalf("expected both instance-scoped requests to pass, got %d and %d", rec1.Code, rec2.Code)
	}
}
