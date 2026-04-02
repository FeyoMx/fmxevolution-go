package instance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
)

type Service struct {
	repo          repository.InstanceRepository
	runtime       Runtime
	runtimeFactory func() (Runtime, error)
	runtimeMu     sync.Mutex
	logger        *slog.Logger
}

type CreateInput struct {
	Name             string `json:"name"`
	EngineInstanceID string `json:"engine_instance_id"`
	WebhookURL       string `json:"webhook_url"`
}

func NewService(repo repository.InstanceRepository, runtime Runtime, runtimeFactory func() (Runtime, error), logger *slog.Logger) *Service {
	return &Service{
		repo:           repo,
		runtime:        runtime,
		runtimeFactory: runtimeFactory,
		logger:         logger,
	}
}

func (s *Service) Create(ctx context.Context, tenantID string, input CreateInput) (*repository.Instance, error) {
	if tenantID == "" || input.Name == "" {
		return nil, fmt.Errorf("%w: tenant_id and name are required", domain.ErrValidation)
	}

	instance := &repository.Instance{
		TenantID:         tenantID,
		Name:             input.Name,
		EngineInstanceID: input.EngineInstanceID,
		WebhookURL:       input.WebhookURL,
		Status:           "created",
	}
	if err := s.repo.Create(ctx, instance); err != nil {
		return nil, err
	}

	if strings.TrimSpace(instance.EngineInstanceID) == "" {
		instance.EngineInstanceID = instance.ID
		if err := s.repo.Update(ctx, instance); err != nil {
			return nil, err
		}
	}

	if runtime, runtimeErr := s.ensureRuntime(); runtime != nil {
		if _, err := runtime.Snapshot(ctx, instance); err != nil && s.logger != nil {
			s.logger.Warn("sync legacy instance on create", "instance_id", instance.ID, "error", err)
		}
	} else if runtimeErr != nil && s.logger != nil {
		s.logger.Warn("legacy runtime unavailable on create", "instance_id", instance.ID, "error", runtimeErr)
	}

	return instance, nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]repository.Instance, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *Service) Delete(ctx context.Context, tenantID, instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("%w: instance id is required", domain.ErrValidation)
	}
	if err := s.repo.Delete(ctx, tenantID, instanceID); err != nil {
		return fmt.Errorf("%w: instance not found", domain.ErrNotFound)
	}
	return nil
}

func (s *Service) Get(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	instance, err := s.repo.GetByID(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: instance not found", domain.ErrNotFound)
	}
	return instance, nil
}

func (s *Service) Resolve(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	return s.resolve(ctx, tenantID, reference)
}

func (s *Service) Connect(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, err
	}

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Connect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("connect legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("connect runtime unavailable, using local status only", "instance_id", instance.ID, "reference", reference, "error", ensureErr)
	}

	instance.Status = "connecting"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) ConnectByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, err
	}

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Connect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("connect legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("connect runtime unavailable, using local status only", "instance_id", instance.ID, "error", ensureErr)
	}

	instance.Status = "connecting"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) Disconnect(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, err
	}

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Disconnect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("disconnect legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("disconnect runtime unavailable, using local status only", "instance_id", instance.ID, "reference", reference, "error", ensureErr)
	}

	instance.Status = "disconnected"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) DisconnectByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, err
	}

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Disconnect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("disconnect legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("disconnect runtime unavailable, using local status only", "instance_id", instance.ID, "error", ensureErr)
	}

	instance.Status = "disconnected"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) Status(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, err
	}
	runtime, _ := s.ensureRuntime()
	if runtime == nil {
		return instance, nil
	}

	snapshot, runtimeErr := runtime.Snapshot(ctx, instance)
	if runtimeErr != nil {
		return instance, nil
	}

	return s.applySnapshot(ctx, instance, snapshot)
}

func (s *Service) StatusByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, err
	}
	runtime, _ := s.ensureRuntime()
	if runtime == nil {
		return instance, nil
	}

	snapshot, runtimeErr := runtime.Snapshot(ctx, instance)
	if runtimeErr != nil {
		return instance, nil
	}

	return s.applySnapshot(ctx, instance, snapshot)
}

func (s *Service) QRCode(ctx context.Context, tenantID, reference string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		if ensureErr != nil {
			return instance, nil, ensureErr
		}
		return instance, nil, fmt.Errorf("runtime unavailable")
	}

	snapshot, err := runtime.QRCode(ctx, instance)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("qrcode legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", err)
		}
		return instance, nil, err
	}

	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}

	return instance, snapshot, nil
}

func (s *Service) SetWebhook(ctx context.Context, tenantID, reference, webhookURL string, events []string) (*repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, err
	}

	instance.WebhookURL = strings.TrimSpace(webhookURL)
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}

	runtime, _ := s.ensureRuntime()
	if runtime == nil {
		return instance, nil
	}

	if legacyRuntime, ok := runtime.(*LegacyRuntime); ok {
		legacyInstance, ensureErr := legacyRuntime.ensureLegacyInstance(ctx, instance)
		if ensureErr != nil {
			if s.logger != nil {
				s.logger.Warn("sync webhook settings with legacy runtime failed", "instance_id", instance.ID, "error", ensureErr)
			}
		} else {
			changed := false
			if strings.TrimSpace(legacyInstance.Webhook) != strings.TrimSpace(instance.WebhookURL) {
				legacyInstance.Webhook = strings.TrimSpace(instance.WebhookURL)
				changed = true
			}

			if len(events) > 0 {
				normalized := strings.Join(events, ",")
				if strings.TrimSpace(legacyInstance.Events) != normalized {
					legacyInstance.Events = normalized
					changed = true
				}
			}

			if changed {
				if err := legacyRuntime.legacyRepo.Update(legacyInstance); err != nil && s.logger != nil {
					s.logger.Warn("persist legacy webhook settings failed", "instance_id", instance.ID, "error", err)
				}
			}
		}
	}

	snapshot, runtimeErr := runtime.Snapshot(ctx, instance)
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Warn("sync webhook with legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
		}
		return instance, nil
	}

	return s.applySnapshot(ctx, instance, snapshot)
}

func (s *Service) QRCodeByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, err
	}
	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		if ensureErr != nil {
			return instance, nil, ensureErr
		}
		return instance, nil, fmt.Errorf("runtime unavailable")
	}

	snapshot, err := runtime.QRCode(ctx, instance)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("qrcode legacy runtime failed", "instance_id", instance.ID, "error", err)
		}
		return instance, nil, err
	}

	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}

	return instance, snapshot, nil
}

func (s *Service) GetAdvancedSettings(ctx context.Context, tenantID, reference string) (*legacyInstanceModel.AdvancedSettings, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		if ensureErr != nil {
			return nil, instance, ensureErr
		}
		return nil, instance, fmt.Errorf("runtime unavailable")
	}

	legacyRuntime, ok := runtime.(*LegacyRuntime)
	if !ok {
		return nil, instance, fmt.Errorf("legacy runtime unavailable")
	}

	settings, err := legacyRuntime.GetAdvancedSettings(ctx, instance)
	if err != nil {
		return nil, instance, err
	}

	return settings, instance, nil
}

func (s *Service) UpdateAdvancedSettings(ctx context.Context, tenantID, reference string, settings *legacyInstanceModel.AdvancedSettings) (*legacyInstanceModel.AdvancedSettings, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		if ensureErr != nil {
			return nil, instance, ensureErr
		}
		return nil, instance, fmt.Errorf("runtime unavailable")
	}

	legacyRuntime, ok := runtime.(*LegacyRuntime)
	if !ok {
		return nil, instance, fmt.Errorf("legacy runtime unavailable")
	}

	snapshot, err := legacyRuntime.UpdateAdvancedSettings(ctx, instance, settings)
	if err != nil {
		return nil, instance, err
	}

	if _, err := s.applySnapshot(ctx, instance, snapshot); err != nil {
		return nil, nil, err
	}

	updated, err := legacyRuntime.GetAdvancedSettings(ctx, instance)
	if err != nil {
		return nil, instance, err
	}

	return updated, instance, nil
}

func (s *Service) resolve(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, fmt.Errorf("%w: instance reference is required", domain.ErrValidation)
	}

	instance, err := s.repo.GetByID(ctx, tenantID, reference)
	if err == nil {
		return instance, nil
	}

	instances, listErr := s.repo.ListByTenant(ctx, tenantID)
	if listErr != nil {
		return nil, listErr
	}

	for idx := range instances {
		if strings.EqualFold(strings.TrimSpace(instances[idx].Name), reference) {
			return &instances[idx], nil
		}
	}

	return nil, fmt.Errorf("%w: instance not found", domain.ErrNotFound)
}

func (s *Service) applySnapshot(ctx context.Context, instance *repository.Instance, snapshot *RuntimeSnapshot) (*repository.Instance, error) {
	if instance == nil || snapshot == nil {
		return instance, nil
	}

	status := strings.TrimSpace(snapshot.Status)
	if status == "" {
		status = instance.Status
	}
	if status != instance.Status {
		instance.Status = status
		if err := s.repo.Update(ctx, instance); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

func (s *Service) ensureRuntime() (Runtime, error) {
	if s.runtime != nil {
		return s.runtime, nil
	}
	if s.runtimeFactory == nil {
		return nil, nil
	}

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if s.runtime != nil {
		return s.runtime, nil
	}

	runtime, err := s.runtimeFactory()
	if err != nil {
		return nil, err
	}

	s.runtime = runtime
	return s.runtime, nil
}
