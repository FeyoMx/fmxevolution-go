package middleware

import (
	"context"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
)

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return domain.WithTenantID(ctx, tenantID)
}

func TenantIDFromContext(ctx context.Context) (string, bool) {
	return domain.TenantIDFromContext(ctx)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return domain.WithRequestID(ctx, requestID)
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	return domain.RequestIDFromContext(ctx)
}
