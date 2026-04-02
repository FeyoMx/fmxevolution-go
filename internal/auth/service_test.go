package auth

import (
	"context"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type tenantRepoMock struct {
	tenant *repository.Tenant
}

func (m tenantRepoMock) GetByID(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

func (m tenantRepoMock) GetBySlug(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

func (m tenantRepoMock) GetByAPIKeyPrefix(context.Context, string) (*repository.Tenant, error) {
	return m.tenant, nil
}

type userRepoMock struct {
	user *repository.User
}

func (m userRepoMock) GetByEmail(context.Context, string, string) (*repository.User, error) {
	return m.user, nil
}

func (m userRepoMock) GetByID(context.Context, string, string) (*repository.User, error) {
	return m.user, nil
}

func TestLoginReturnsAccessAndRefreshJWT(t *testing.T) {
	passwordHash, err := repository.HashPassword("super-secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	svc := NewService(
		tenantRepoMock{tenant: &repository.Tenant{ID: "tenant-1", Slug: "agency"}},
		userRepoMock{user: &repository.User{
			ID:           "user-1",
			TenantID:     "tenant-1",
			Email:        "owner@example.com",
			PasswordHash: passwordHash,
			Role:         RoleAdmin,
		}},
		NewTokenManager("test-secret", time.Hour, 24*time.Hour),
	)

	output, err := svc.Login(context.Background(), LoginInput{
		TenantSlug: "agency",
		Email:      "owner@example.com",
		Password:   "super-secret",
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}

	if output.AccessToken == "" {
		t.Fatal("expected access token to be populated")
	}
	if output.RefreshToken == "" {
		t.Fatal("expected refresh token to be populated")
	}
	if output.TenantID != "tenant-1" {
		t.Fatalf("unexpected tenant id: %s", output.TenantID)
	}
	if output.Role != RoleAdmin {
		t.Fatalf("unexpected role: %s", output.Role)
	}
}

func TestAuthenticateAcceptsHashedTenantAPIKey(t *testing.T) {
	rawKey, err := repository.GenerateTenantAPIKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	hash, err := repository.HashAPIKey(rawKey)
	if err != nil {
		t.Fatalf("hash api key: %v", err)
	}

	svc := NewService(
		tenantRepoMock{tenant: &repository.Tenant{
			ID:           "tenant-1",
			Slug:         "agency",
			APIKeyPrefix: repository.APIKeyPrefix(rawKey),
			APIKeyHash:   hash,
		}},
		userRepoMock{},
		NewTokenManager("test-secret", time.Hour, 24*time.Hour),
	)

	identity, err := svc.Authenticate(context.Background(), "", rawKey)
	if err != nil {
		t.Fatalf("authenticate api key: %v", err)
	}

	if identity.TenantID != "tenant-1" {
		t.Fatalf("unexpected tenant id: %s", identity.TenantID)
	}
	if identity.Role != RoleAdmin {
		t.Fatalf("unexpected role: %s", identity.Role)
	}
	if !identity.APIKey {
		t.Fatal("expected APIKey identity flag to be true")
	}
}
