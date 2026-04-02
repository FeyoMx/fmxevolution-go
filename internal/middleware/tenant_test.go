package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type tenantResolverMock struct {
	tenant *repository.Tenant
	err    error
}

func (m tenantResolverMock) Get(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, m.err
}

func TestTenantMiddlewareResolvesTenantIntoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1"}
		c.Set(identityKey, identity)
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), identity))
		c.Next()
	})
	router.Use(NewTenantMiddleware(tenantResolverMock{
		tenant: &repository.Tenant{ID: "tenant-1", Slug: "agency"},
	}).Resolve())
	router.GET("/instances", func(c *gin.Context) {
		tenant := MustTenant(c)
		tenantID, ok := domain.TenantIDFromContext(c.Request.Context())
		if !ok {
			t.Fatal("expected tenant id in request context")
		}

		c.JSON(http.StatusOK, gin.H{
			"tenant_id": tenant.ID,
			"slug":      tenant.Slug,
			"context":   tenantID,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/instances", nil)
	req.Header.Set("X-Tenant-ID", "tenant-1")
	req.Header.Set("X-Tenant-Slug", "agency")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTenantMiddlewareRejectsTenantHeaderMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1"}
		c.Set(identityKey, identity)
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), identity))
		c.Next()
	})
	router.Use(NewTenantMiddleware(tenantResolverMock{
		tenant: &repository.Tenant{ID: "tenant-1", Slug: "agency"},
	}).Resolve())
	router.GET("/instances", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/instances", nil)
	req.Header.Set("X-Tenant-ID", "tenant-2")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
