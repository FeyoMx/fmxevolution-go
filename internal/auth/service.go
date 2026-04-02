package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
	RoleAgent = "agent"
)

type TenantFinder interface {
	GetByID(ctx context.Context, tenantID string) (*repository.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*repository.Tenant, error)
	GetByAPIKeyPrefix(ctx context.Context, prefix string) (*repository.Tenant, error)
}

type UserFinder interface {
	GetByEmail(ctx context.Context, tenantID, email string) (*repository.User, error)
	GetByID(ctx context.Context, tenantID, userID string) (*repository.User, error)
}

type InstanceTokenResolver interface {
	Resolve(ctx context.Context, token string) (*Identity, error)
}

type Service struct {
	tenants TenantFinder
	users   UserFinder
	tokens  *TokenManager
	instanceTokens InstanceTokenResolver
}

type LoginInput struct {
	TenantSlug      string `json:"tenant_slug" form:"tenant_slug"`
	TenantSlugCamel string `json:"tenantSlug" form:"tenantSlug"`
	Tenant          string `json:"tenant" form:"tenant"`
	Email           string `json:"email" form:"email"`
	Password        string `json:"password" form:"password"`
}

type RefreshInput struct {
	RefreshToken      string `json:"refresh_token" form:"refresh_token"`
	RefreshTokenCamel string `json:"refreshToken" form:"refreshToken"`
}

type LoginOutput struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TenantID     string `json:"tenant_id"`
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
	ExpiresIn    int64  `json:"expires_in"`
}

func NewService(tenants TenantFinder, users UserFinder, tokens *TokenManager) *Service {
	return &Service{tenants: tenants, users: users, tokens: tokens}
}

func (s *Service) SetInstanceTokenResolver(resolver InstanceTokenResolver) {
	s.instanceTokens = resolver
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	input.Normalize()
	if input.TenantSlug == "" || input.Email == "" || input.Password == "" {
		return nil, fmt.Errorf("%w: tenant_slug, email and password are required", domain.ErrValidation)
	}

	tenant, err := s.tenants.GetBySlug(ctx, strings.ToLower(strings.TrimSpace(input.TenantSlug)))
	if err != nil {
		return nil, fmt.Errorf("%w: tenant not found", domain.ErrUnauthorized)
	}

	user, err := s.users.GetByEmail(ctx, tenant.ID, strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid credentials", domain.ErrUnauthorized)
	}

	if err := repository.CheckPassword(user.PasswordHash, input.Password); err != nil {
		return nil, fmt.Errorf("%w: invalid credentials", domain.ErrUnauthorized)
	}

	return s.issueTokens(ctx, tenant, user)
}

func (s *Service) Refresh(ctx context.Context, input RefreshInput) (*LoginOutput, error) {
	input.Normalize()
	if strings.TrimSpace(input.RefreshToken) == "" {
		return nil, fmt.Errorf("%w: refresh_token is required", domain.ErrValidation)
	}

	identity, err := s.tokens.ParseRefresh(ctx, input.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid refresh token", domain.ErrUnauthorized)
	}

	tenant, err := s.tenants.GetByID(ctx, identity.TenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: tenant not found", domain.ErrUnauthorized)
	}

	user, err := s.users.GetByID(ctx, identity.TenantID, identity.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: user not found", domain.ErrUnauthorized)
	}

	return s.issueTokens(ctx, tenant, user)
}

func (s *Service) Authenticate(ctx context.Context, bearerToken, apiKey string) (*Identity, error) {
	if apiKey != "" {
		return s.authenticateAPIKey(ctx, apiKey)
	}
	if bearerToken == "" {
		return nil, fmt.Errorf("%w: missing credentials", domain.ErrUnauthorized)
	}

	identity, err := s.tokens.Parse(ctx, bearerToken)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid bearer token", domain.ErrUnauthorized)
	}

	user, err := s.users.GetByID(ctx, identity.TenantID, identity.UserID)
	if err != nil {
		return nil, fmt.Errorf("%w: user not found", domain.ErrUnauthorized)
	}

	identity.Role = user.Role
	identity.Email = user.Email
	return identity, nil
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, apiKey string) (*Identity, error) {
	return s.authenticateAPIKey(ctx, apiKey)
}

func (s *Service) issueTokens(ctx context.Context, tenant *repository.Tenant, user *repository.User) (*LoginOutput, error) {
	pair, err := s.tokens.GeneratePair(ctx, domain.Identity{
		UserID:   user.ID,
		TenantID: tenant.ID,
		Email:    user.Email,
		Role:     user.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	return &LoginOutput{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TenantID:     tenant.ID,
		UserID:       user.ID,
		Role:         user.Role,
		ExpiresIn:    pair.ExpiresIn,
	}, nil
}

func (s *Service) authenticateAPIKey(ctx context.Context, apiKey string) (*Identity, error) {
	prefix := repository.APIKeyPrefix(strings.TrimSpace(apiKey))
	tenant, err := s.tenants.GetByAPIKeyPrefix(ctx, prefix)
	if err == nil {
		if err := repository.CheckAPIKey(tenant.APIKeyHash, apiKey); err == nil {
			return &domain.Identity{
				TenantID: tenant.ID,
				Role:     RoleAdmin,
				APIKey:   true,
			}, nil
		}
	}

	if s.instanceTokens != nil {
		if identity, resolveErr := s.instanceTokens.Resolve(ctx, apiKey); resolveErr == nil && identity != nil {
			return identity, nil
		}
	}

	return nil, fmt.Errorf("%w: invalid api key", domain.ErrUnauthorized)
}

func (i *LoginInput) Normalize() {
	if strings.TrimSpace(i.TenantSlug) == "" {
		switch {
		case strings.TrimSpace(i.TenantSlugCamel) != "":
			i.TenantSlug = i.TenantSlugCamel
		case strings.TrimSpace(i.Tenant) != "":
			i.TenantSlug = i.Tenant
		}
	}

	i.TenantSlug = strings.ToLower(strings.TrimSpace(i.TenantSlug))
	i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	i.Password = strings.TrimSpace(i.Password)
}

func (i *RefreshInput) Normalize() {
	if strings.TrimSpace(i.RefreshToken) == "" && strings.TrimSpace(i.RefreshTokenCamel) != "" {
		i.RefreshToken = i.RefreshTokenCamel
	}
	i.RefreshToken = strings.TrimSpace(i.RefreshToken)
}
