package auth

import (
	"context"
	"crypto/subtle"
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

// minInstanceTokenLength rejects trivially short or empty tokens before they are
// ever used in a database lookup, closing off blank/placeholder tokens.
const minInstanceTokenLength = 16

type LegacyInstanceTokenResolver struct {
	instances  instanceFinder
	legacyRepo legacyInstanceRepo.InstanceRepository
	// platformKey is never a valid instance token. Rejecting it here means a
	// leaked GLOBAL/PLATFORM key cannot be replayed as an instance credential.
	platformKey string
}

func NewLegacyInstanceTokenResolver(instances instanceFinder, platformKey string) (*LegacyInstanceTokenResolver, error) {
	cfg := pkgconfig.Load()
	db, err := cfg.CreateUsersDB()
	if err != nil {
		return nil, fmt.Errorf("open legacy users db: %w", err)
	}

	return &LegacyInstanceTokenResolver{
		instances:   instances,
		legacyRepo:  legacyInstanceRepo.NewInstanceRepository(db),
		platformKey: strings.TrimSpace(platformKey),
	}, nil
}

func (r *LegacyInstanceTokenResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	if r == nil || r.instances == nil || r.legacyRepo == nil {
		return nil, fmt.Errorf("instance token resolver unavailable")
	}

	token = strings.TrimSpace(token)
	if len(token) < minInstanceTokenLength {
		return nil, fmt.Errorf("%w: missing or malformed instance token", domain.ErrUnauthorized)
	}

	// Defense in depth: the platform/global key must never resolve to an instance.
	if r.platformKey != "" && subtle.ConstantTimeCompare([]byte(token), []byte(r.platformKey)) == 1 {
		return nil, fmt.Errorf("%w: platform key is not a valid instance token", domain.ErrUnauthorized)
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
