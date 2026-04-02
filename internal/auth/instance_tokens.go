package auth

import (
	"context"
	"fmt"
	"strings"

	pkgconfig "github.com/EvolutionAPI/evolution-go/pkg/config"
	legacyInstanceRepo "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type instanceFinder interface {
	FindByEngineInstanceID(ctx context.Context, engineInstanceID string) (*repository.Instance, error)
	FindByName(ctx context.Context, name string) (*repository.Instance, error)
}

type LegacyInstanceTokenResolver struct {
	instances  instanceFinder
	legacyRepo legacyInstanceRepo.InstanceRepository
}

func NewLegacyInstanceTokenResolver(instances instanceFinder) (*LegacyInstanceTokenResolver, error) {
	cfg := pkgconfig.Load()
	db, err := cfg.CreateUsersDB()
	if err != nil {
		return nil, fmt.Errorf("open legacy users db: %w", err)
	}

	return &LegacyInstanceTokenResolver{
		instances:  instances,
		legacyRepo: legacyInstanceRepo.NewInstanceRepository(db),
	}, nil
}

func (r *LegacyInstanceTokenResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	if r == nil || r.instances == nil || r.legacyRepo == nil {
		return nil, fmt.Errorf("instance token resolver unavailable")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("%w: missing instance token", domain.ErrUnauthorized)
	}

	legacyInstance, err := r.legacyRepo.GetInstanceByToken(token)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid instance token", domain.ErrUnauthorized)
	}

	if legacyInstance == nil {
		return nil, fmt.Errorf("%w: invalid instance token", domain.ErrUnauthorized)
	}

	var instance *repository.Instance
	if engineID := strings.TrimSpace(legacyInstance.Id); engineID != "" {
		instance, err = r.instances.FindByEngineInstanceID(ctx, engineID)
	}
	if (err != nil || instance == nil) && strings.TrimSpace(legacyInstance.Name) != "" {
		instance, err = r.instances.FindByName(ctx, legacyInstance.Name)
	}
	if err != nil || instance == nil {
		return nil, fmt.Errorf("%w: instance not linked to tenant", domain.ErrUnauthorized)
	}

	return &domain.Identity{
		TenantID: instance.TenantID,
		Role:     RoleAdmin,
		APIKey:   true,
	}, nil
}
