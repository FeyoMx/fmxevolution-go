package domain

import "context"

type contextKey string

const (
	identityContextKey  contextKey = "identity"
	tenantIDContextKey  contextKey = "tenant_id"
	requestIDContextKey contextKey = "request_id"
)

type Identity struct {
	UserID   string
	TenantID string
	Email    string
	Role     string
	APIKey   bool
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	value, ok := ctx.Value(identityContextKey).(Identity)
	return value, ok
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey, tenantID)
}

func TenantIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(tenantIDContextKey).(string)
	return value, ok && value != ""
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(requestIDContextKey).(string)
	return value, ok && value != ""
}
