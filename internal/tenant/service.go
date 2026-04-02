package tenant

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type Service struct {
	tenants repository.TenantRepository
	users   repository.UserRepository
}

type CreateInput struct {
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	AdminName     string `json:"admin_name"`
	AdminEmail    string `json:"admin_email"`
	AdminPassword string `json:"admin_password"`
}

type CreateOutput struct {
	Tenant *repository.Tenant `json:"tenant"`
	User   *repository.User   `json:"user"`
	APIKey string             `json:"api_key"`
}

func NewService(tenants repository.TenantRepository, users repository.UserRepository) *Service {
	return &Service{tenants: tenants, users: users}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Name == "" || input.Slug == "" || input.AdminEmail == "" || input.AdminPassword == "" {
		return nil, fmt.Errorf("%w: missing tenant fields", domain.ErrValidation)
	}

	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	if _, err := s.tenants.GetBySlug(ctx, slug); err == nil {
		return nil, fmt.Errorf("%w: tenant slug already exists", domain.ErrConflict)
	}

	passwordHash, err := repository.HashPassword(input.AdminPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	rawAPIKey, err := repository.GenerateTenantAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}
	apiKeyHash, err := repository.HashAPIKey(rawAPIKey)
	if err != nil {
		return nil, fmt.Errorf("hash api key: %w", err)
	}

	tenant := &repository.Tenant{
		Name:         strings.TrimSpace(input.Name),
		Slug:         slug,
		APIKeyPrefix: repository.APIKeyPrefix(rawAPIKey),
		APIKeyHash:   apiKeyHash,
	}

	if err := s.tenants.Create(ctx, tenant); err != nil {
		return nil, err
	}

	user := &repository.User{
		TenantID:     tenant.ID,
		Email:        strings.ToLower(strings.TrimSpace(input.AdminEmail)),
		PasswordHash: passwordHash,
		Name:         input.AdminName,
		Role:         auth.RoleAdmin,
	}
	if user.Name == "" {
		user.Name = tenant.Name + " Admin"
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return &CreateOutput{
		Tenant: tenant,
		User:   user,
		APIKey: rawAPIKey,
	}, nil
}

func (s *Service) Get(ctx context.Context, tenantID string) (*repository.Tenant, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant missing from context", domain.ErrUnauthorized)
	}
	tenant, err := s.tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: tenant not found", domain.ErrNotFound)
	}
	return tenant, nil
}
