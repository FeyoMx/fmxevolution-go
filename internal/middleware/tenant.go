package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	sharedhandler "github.com/EvolutionAPI/evolution-go/internal/handler"
	"github.com/gin-gonic/gin"
)

const tenantKey = "tenant"

type TenantResolver interface {
	Get(ctx context.Context, tenantID string) (*repository.Tenant, error)
}

type TenantMiddleware struct {
	resolver TenantResolver
}

func NewTenantMiddleware(resolver TenantResolver) *TenantMiddleware {
	return &TenantMiddleware{resolver: resolver}
}

func (m *TenantMiddleware) Resolve() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := MustIdentity(c)
		if identity.TenantID == "" {
			sharedhandler.WriteError(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}

		tenant, err := m.resolver.Get(c.Request.Context(), identity.TenantID)
		if err != nil {
			sharedhandler.WriteError(c, fmt.Errorf("%w: tenant not found", domain.ErrUnauthorized))
			c.Abort()
			return
		}

		if err := ensureTenantRequestMatches(c, identity.TenantID, tenant.Slug); err != nil {
			sharedhandler.WriteError(c, err)
			c.Abort()
			return
		}

		c.Set("tenant_id", tenant.ID)
		c.Set(tenantKey, tenant)
		c.Request = c.Request.WithContext(domain.WithTenantID(c.Request.Context(), tenant.ID))
		c.Next()
	}
}

func MustTenant(c *gin.Context) *repository.Tenant {
	value, exists := c.Get(tenantKey)
	if !exists {
		return &repository.Tenant{}
	}

	tenant, ok := value.(*repository.Tenant)
	if !ok {
		return &repository.Tenant{}
	}

	return tenant
}

func ensureTenantRequestMatches(c *gin.Context, tenantID, tenantSlug string) error {
	headerTenantID := strings.TrimSpace(c.GetHeader("X-Tenant-ID"))
	if headerTenantID != "" && headerTenantID != tenantID {
		return fmt.Errorf("%w: tenant id does not match authenticated tenant", domain.ErrForbidden)
	}

	headerTenantSlug := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Tenant-Slug")))
	if headerTenantSlug != "" && headerTenantSlug != strings.ToLower(tenantSlug) {
		return fmt.Errorf("%w: tenant slug does not match authenticated tenant", domain.ErrForbidden)
	}

	return nil
}
