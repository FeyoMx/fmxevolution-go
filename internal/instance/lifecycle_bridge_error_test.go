package instance

import (
	"context"
	"errors"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type lifecycleInstanceRepoMock struct {
	instance *repository.Instance
}

func (m lifecycleInstanceRepoMock) Create(context.Context, *repository.Instance) error {
	return nil
}

func (m lifecycleInstanceRepoMock) ListByTenant(context.Context, string) ([]repository.Instance, error) {
	if m.instance == nil {
		return []repository.Instance{}, nil
	}
	return []repository.Instance{*m.instance}, nil
}

func (m lifecycleInstanceRepoMock) GetByID(context.Context, string, string) (*repository.Instance, error) {
	if m.instance == nil {
		return nil, errors.New("not found")
	}
	return m.instance, nil
}

func (m lifecycleInstanceRepoMock) GetByGlobalID(context.Context, string) (*repository.Instance, error) {
	return nil, errors.New("not implemented")
}

func (m lifecycleInstanceRepoMock) FindByEngineInstanceID(context.Context, string) (*repository.Instance, error) {
	return nil, errors.New("not implemented")
}

func (m lifecycleInstanceRepoMock) FindByName(context.Context, string) (*repository.Instance, error) {
	return nil, errors.New("not implemented")
}

func (m lifecycleInstanceRepoMock) Update(context.Context, *repository.Instance) error {
	return nil
}

func (m lifecycleInstanceRepoMock) Delete(context.Context, string, string) error {
	return nil
}

func TestNormalizeBridgeUnavailableLifecycleErrorMapsToConflict(t *testing.T) {
	err := normalizeBridgeUnavailableLifecycleError(errors.New("legacy client runtime unavailable"), "pair")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if got := err.Error(); got != "conflict: runtime unavailable for pair" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestNormalizeBridgeUnavailableLifecycleErrorPreservesOtherErrors(t *testing.T) {
	original := errors.New("socket timeout")
	err := normalizeBridgeUnavailableLifecycleError(original, "reconnect")
	if !errors.Is(err, original) {
		t.Fatalf("expected original error to be preserved, got %v", err)
	}
}

func TestReconnectByIDBridgeUnavailableReturnsConflict(t *testing.T) {
	service := &Service{
		repo: lifecycleInstanceRepoMock{
			instance: &repository.Instance{
				ID:       "instance-1",
				TenantID: "tenant-1",
				Name:     "alpha",
			},
		},
		runtimeFactory: func() (Runtime, error) {
			return nil, errors.New("legacy runtime unavailable")
		},
	}

	_, _, err := service.ReconnectByID(context.Background(), "tenant-1", "instance-1")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestPairByIDBridgeUnavailableReturnsConflict(t *testing.T) {
	service := &Service{
		repo: lifecycleInstanceRepoMock{
			instance: &repository.Instance{
				ID:       "instance-1",
				TenantID: "tenant-1",
				Name:     "alpha",
			},
		},
		runtimeFactory: func() (Runtime, error) {
			return nil, errors.New("legacy runtime unavailable")
		},
	}

	_, _, err := service.PairByID(context.Background(), "tenant-1", "instance-1", PairInput{Phone: "5215512345678"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestLogoutByIDBridgeUnavailableReturnsConflict(t *testing.T) {
	service := &Service{
		repo: lifecycleInstanceRepoMock{
			instance: &repository.Instance{
				ID:       "instance-1",
				TenantID: "tenant-1",
				Name:     "alpha",
			},
		},
		runtimeFactory: func() (Runtime, error) {
			return nil, errors.New("legacy runtime unavailable")
		},
	}

	_, _, err := service.LogoutByID(context.Background(), "tenant-1", "instance-1")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
