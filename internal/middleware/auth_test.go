package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type middlewareTenantRepoMock struct {
	tenant *repository.Tenant
}

func (m middlewareTenantRepoMock) GetByID(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

func (m middlewareTenantRepoMock) GetBySlug(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

func (m middlewareTenantRepoMock) GetByAPIKeyPrefix(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

type middlewareUserRepoMock struct{}

func (middlewareUserRepoMock) GetByEmail(context.Context, string, string) (*repository.User, error) {
	return nil, nil
}

func (middlewareUserRepoMock) GetByID(context.Context, string, string) (*repository.User, error) {
	return nil, nil
}

func TestAuthMiddlewareAcceptsAPIKeyAndAttachesTenantContext(t *testing.T) {
	rawKey, err := repository.GenerateTenantAPIKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	hash, err := repository.HashAPIKey(rawKey)
	if err != nil {
		t.Fatalf("hash api key: %v", err)
	}

	service := auth.NewService(
		middlewareTenantRepoMock{tenant: &repository.Tenant{
			ID:           "tenant-1",
			Slug:         "agency",
			APIKeyPrefix: repository.APIKeyPrefix(rawKey),
			APIKeyHash:   hash,
		}},
		middlewareUserRepoMock{},
		auth.NewTokenManager("test-secret", time.Hour, 24*time.Hour),
	)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewAuthMiddleware(service).Guard())
	router.GET("/protected", func(c *gin.Context) {
		identity := MustIdentity(c)
		tenantID, ok := domain.TenantIDFromContext(c.Request.Context())
		if !ok {
			t.Fatal("expected tenant id in context")
		}
		c.JSON(http.StatusOK, gin.H{
			"tenant_id": tenantID,
			"role":      identity.Role,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRolesBlocksAgentFromAdminOnlyRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1", Role: auth.RoleAgent}
		c.Set(identityKey, identity)
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), identity))
		c.Next()
	})
	router.GET("/admin", RequireRoles(auth.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
