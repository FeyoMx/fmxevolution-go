package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	legacyInstanceService "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	"github.com/EvolutionAPI/evolution-go/pkg/sendstatus"
	"github.com/google/uuid"
)

type Service struct {
	repo           repository.InstanceRepository
	runtime        Runtime
	runtimeFactory func() (Runtime, error)
	runtimeMu      sync.Mutex
	logger         *slog.Logger
}

type CreateInput struct {
	Name             string `json:"name"`
	EngineInstanceID string `json:"engine_instance_id"`
	WebhookURL       string `json:"webhook_url"`
}

type SendTextInput struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	Delay  int32  `json:"delay"`
}

type SendMediaOutput = SendMediaResult

type SendTextJobStatus = sendstatus.JobStatus

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

func (s *Service) SendText(ctx context.Context, tenantID, reference string, input SendTextInput) (*SendTextResult, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	if strings.TrimSpace(input.Number) == "" || strings.TrimSpace(input.Text) == "" {
		return nil, instance, fmt.Errorf("%w: number and text are required", domain.ErrValidation)
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

	message, err := legacyRuntime.SendText(ctx, instance, strings.TrimSpace(input.Number), input.Text)
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"send text failed",
				"instance_id", instance.ID,
				"reference", reference,
				"number", strings.TrimSpace(input.Number),
				"error", err,
			)
		}
		return nil, instance, err
	}

	return message, instance, nil
}

func (s *Service) SendMedia(ctx context.Context, tenantID, reference string, input resolvedMediaMessageInput) (*SendMediaOutput, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	input.Number = strings.TrimSpace(input.Number)
	input.Type = strings.TrimSpace(input.Type)
	input.URL = strings.TrimSpace(input.URL)
	input.FileName = strings.TrimSpace(input.FileName)
	if input.Number == "" || input.Type == "" {
		return nil, instance, fmt.Errorf("%w: number and media type are required", domain.ErrValidation)
	}
	if len(input.FileData) == 0 && input.URL == "" {
		return nil, instance, fmt.Errorf("%w: media payload or url is required", domain.ErrValidation)
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

	message, err := legacyRuntime.SendMedia(ctx, instance, input)
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"send media failed",
				"instance_id", instance.ID,
				"reference", reference,
				"number", input.Number,
				"type", input.Type,
				"error", err,
			)
		}
		return nil, instance, err
	}

	return message, instance, nil
}

func (s *Service) SendAudio(ctx context.Context, tenantID, reference string, input resolvedAudioMessageInput) (*SendMediaOutput, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	input.Number = strings.TrimSpace(input.Number)
	if input.Number == "" || len(input.FileData) == 0 {
		return nil, instance, fmt.Errorf("%w: number and audio payload are required", domain.ErrValidation)
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

	message, err := legacyRuntime.SendAudio(ctx, instance, input)
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"send audio failed",
				"instance_id", instance.ID,
				"reference", reference,
				"number", input.Number,
				"error", err,
			)
		}
		return nil, instance, err
	}

	return message, instance, nil
}

func (s *Service) SearchChats(ctx context.Context, tenantID, reference string, input ChatSearchRequest) ([]chatSearchRecord, *repository.Instance, error) {
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

	chats, err := legacyRuntime.SearchChats(ctx, instance, normalizeChatSearchFilter(input))
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"search chats failed",
				"instance_id", instance.ID,
				"reference", reference,
				"error", err,
			)
		}
		return nil, instance, err
	}

	return chats, instance, nil
}

func (s *Service) QueueSendText(ctx context.Context, tenantID, reference string, input SendTextInput) (string, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return "", nil, err
	}

	input.Number = strings.TrimSpace(input.Number)
	input.Text = strings.TrimSpace(input.Text)
	if input.Number == "" || input.Text == "" {
		return "", instance, fmt.Errorf("%w: number and text are required", domain.ErrValidation)
	}

	jobID := uuid.NewString()
	queuedAt := time.Now().UTC()
	s.storeSendTextJob(tenantID, SendTextJobStatus{
		JobID:        jobID,
		InstanceID:   instance.ID,
		InstanceName: instance.Name,
		Reference:    reference,
		Number:       input.Number,
		Text:         input.Text,
		Status:       "queued",
		QueuedAt:     queuedAt,
	})

	go func(instance *repository.Instance, queuedInput SendTextInput, queuedJobID string) {
		startedAt := time.Now().UTC()
		s.storeSendTextJob(tenantID, SendTextJobStatus{
			JobID:        queuedJobID,
			InstanceID:   instance.ID,
			InstanceName: instance.Name,
			Reference:    instance.ID,
			Number:       queuedInput.Number,
			Text:         queuedInput.Text,
			Status:       "running",
			QueuedAt:     queuedAt,
			StartedAt:    &startedAt,
		})

		sendCtx := context.Background()
		message, _, sendErr := s.SendText(sendCtx, tenantID, instance.ID, queuedInput)
		finishedAt := time.Now().UTC()
		if s.logger == nil {
			if sendErr != nil {
				s.storeSendTextJob(tenantID, SendTextJobStatus{
					JobID:        queuedJobID,
					InstanceID:   instance.ID,
					InstanceName: instance.Name,
					Reference:    instance.ID,
					Number:       queuedInput.Number,
					Text:         queuedInput.Text,
					Status:       "failed",
					Error:        sendErr.Error(),
					QueuedAt:     queuedAt,
					StartedAt:    &startedAt,
					FinishedAt:   &finishedAt,
				})
				return
			}
			s.storeSendTextJob(tenantID, SendTextJobStatus{
				JobID:        queuedJobID,
				InstanceID:   instance.ID,
				InstanceName: instance.Name,
				Reference:    instance.ID,
				Number:       queuedInput.Number,
				Text:         queuedInput.Text,
				Status:       "sent",
				MessageID:    message.MessageID,
				ServerID:     message.ServerID,
				QueuedAt:     queuedAt,
				StartedAt:    &startedAt,
				FinishedAt:   &finishedAt,
			})
			return
		}
		if sendErr != nil {
			s.storeSendTextJob(tenantID, SendTextJobStatus{
				JobID:        queuedJobID,
				InstanceID:   instance.ID,
				InstanceName: instance.Name,
				Reference:    instance.ID,
				Number:       queuedInput.Number,
				Text:         queuedInput.Text,
				Status:       "failed",
				Error:        sendErr.Error(),
				QueuedAt:     queuedAt,
				StartedAt:    &startedAt,
				FinishedAt:   &finishedAt,
			})
			s.logger.Error(
				"queued send text failed",
				"job_id", queuedJobID,
				"instance_id", instance.ID,
				"number", queuedInput.Number,
				"error", sendErr,
			)
			return
		}
		s.storeSendTextJob(tenantID, SendTextJobStatus{
			JobID:        queuedJobID,
			InstanceID:   instance.ID,
			InstanceName: instance.Name,
			Reference:    instance.ID,
			Number:       queuedInput.Number,
			Text:         queuedInput.Text,
			Status:       "sent",
			MessageID:    message.MessageID,
			ServerID:     message.ServerID,
			QueuedAt:     queuedAt,
			StartedAt:    &startedAt,
			FinishedAt:   &finishedAt,
		})
		s.logger.Info(
			"queued send text completed",
			"job_id", queuedJobID,
			"instance_id", instance.ID,
			"number", queuedInput.Number,
			"message_id", message.MessageID,
			"server_id", message.ServerID,
		)
	}(instance, input, jobID)

	return jobID, instance, nil
}

func (s *Service) GetSendTextJob(ctx context.Context, tenantID, reference, jobID string) (*SendTextJobStatus, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	status, ok := s.loadSendTextJob(tenantID, jobID)
	if !ok || status.InstanceID != instance.ID {
		return nil, instance, fmt.Errorf("%w: send job not found", domain.ErrNotFound)
	}

	return &status, instance, nil
}

func (s *Service) GetWebsocketConfig(ctx context.Context, tenantID, reference string) (*EventConnectorConfig, *repository.Instance, error) {
	instance, _, legacyInstance, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	return &EventConnectorConfig{
		Enabled: normalizedConnectorEnabled(legacyInstance.WebSocketEnable),
		Events:  parseLegacyEvents(legacyInstance.Events),
	}, instance, nil
}

func (s *Service) SetWebsocketConfig(ctx context.Context, tenantID, reference string, input EventConnectorConfig) (*EventConnectorConfig, *repository.Instance, error) {
	instance, legacyRuntime, legacyInstance, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	events, err := normalizeRequestedEvents(input.Events, legacyInstance.Events, input.Enabled)
	if err != nil {
		return nil, instance, err
	}

	legacyInstance.WebSocketEnable = connectorState(input.Enabled)
	legacyInstance.Events = strings.Join(events, ",")
	if err := legacyRuntime.legacyRepo.Update(legacyInstance); err != nil {
		return nil, instance, err
	}
	s.syncLegacyInstanceSettings(instance, legacyRuntime)

	return &EventConnectorConfig{
		Enabled: input.Enabled,
		Events:  events,
	}, instance, nil
}

func (s *Service) GetRabbitMQConfig(ctx context.Context, tenantID, reference string) (*EventConnectorConfig, *repository.Instance, error) {
	instance, _, legacyInstance, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	return &EventConnectorConfig{
		Enabled: normalizedConnectorEnabled(legacyInstance.RabbitmqEnable),
		Events:  parseLegacyEvents(legacyInstance.Events),
	}, instance, nil
}

func (s *Service) SetRabbitMQConfig(ctx context.Context, tenantID, reference string, input EventConnectorConfig) (*EventConnectorConfig, *repository.Instance, error) {
	instance, legacyRuntime, legacyInstance, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	events, err := normalizeRequestedEvents(input.Events, legacyInstance.Events, input.Enabled)
	if err != nil {
		return nil, instance, err
	}

	legacyInstance.RabbitmqEnable = connectorState(input.Enabled)
	legacyInstance.Events = strings.Join(events, ",")
	if err := legacyRuntime.legacyRepo.Update(legacyInstance); err != nil {
		return nil, instance, err
	}
	s.syncLegacyInstanceSettings(instance, legacyRuntime)

	return &EventConnectorConfig{
		Enabled: input.Enabled,
		Events:  events,
	}, instance, nil
}

func (s *Service) GetProxyConfig(ctx context.Context, tenantID, reference string) (*ProxyConfig, *repository.Instance, error) {
	instance, _, legacyInstance, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	return parseProxyConfig(legacyInstance.Proxy), instance, nil
}

func (s *Service) SetProxyConfig(ctx context.Context, tenantID, reference string, input ProxyConfig) (*ProxyConfig, *repository.Instance, error) {
	instance, legacyRuntime, _, err := s.resolveLegacyInstance(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	normalized, err := normalizeProxyConfig(input)
	if err != nil {
		return nil, instance, err
	}

	if !normalized.Enabled {
		if err := legacyRuntime.legacySvc.RemoveProxy(instance.EngineInstanceID); err != nil {
			if err := legacyRuntime.legacySvc.RemoveProxy(instance.ID); err != nil {
				return nil, instance, err
			}
		}
		return &normalized, instance, nil
	}

	if err := legacyRuntime.legacySvc.SetProxyFromStruct(instance.EngineInstanceID, &legacyInstanceService.SetProxyStruct{
		Host:     normalized.Host,
		Port:     normalized.Port,
		Username: normalized.Username,
		Password: normalized.Password,
	}); err != nil {
		if err := legacyRuntime.legacySvc.SetProxyFromStruct(instance.ID, &legacyInstanceService.SetProxyStruct{
			Host:     normalized.Host,
			Port:     normalized.Port,
			Username: normalized.Username,
			Password: normalized.Password,
		}); err != nil {
			return nil, instance, err
		}
	}

	return &normalized, instance, nil
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

func (s *Service) resolveLegacyInstance(ctx context.Context, tenantID, reference string) (*repository.Instance, *LegacyRuntime, *legacyInstanceModel.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		if ensureErr != nil {
			return instance, nil, nil, ensureErr
		}
		return instance, nil, nil, fmt.Errorf("runtime unavailable")
	}

	legacyRuntime, ok := runtime.(*LegacyRuntime)
	if !ok {
		return instance, nil, nil, fmt.Errorf("legacy runtime unavailable")
	}

	legacyInstance, err := legacyRuntime.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return instance, legacyRuntime, nil, err
	}

	return instance, legacyRuntime, legacyInstance, nil
}

func (s *Service) syncLegacyInstanceSettings(instance *repository.Instance, legacyRuntime *LegacyRuntime) {
	if instance == nil || legacyRuntime == nil || legacyRuntime.whatsmeowSvc == nil {
		return
	}

	if err := legacyRuntime.whatsmeowSvc.UpdateInstanceSettings(instance.EngineInstanceID); err != nil {
		if retryErr := legacyRuntime.whatsmeowSvc.UpdateInstanceSettings(instance.ID); retryErr != nil && s.logger != nil {
			s.logger.Warn("sync instance connector settings failed", "instance_id", instance.ID, "error", retryErr)
		}
	}
}

func normalizeRequestedEvents(events []string, existing string, enabled bool) ([]string, error) {
	if !enabled {
		if len(events) == 0 {
			return parseLegacyEvents(existing), nil
		}
	}

	normalized := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		value := strings.ToUpper(strings.TrimSpace(event))
		if value == "" {
			continue
		}
		if !isSupportedInstanceEvent(value) {
			return nil, fmt.Errorf("%w: unsupported event %q", domain.ErrValidation, value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if len(normalized) > 0 {
		return normalized, nil
	}

	if current := parseLegacyEvents(existing); len(current) > 0 {
		return current, nil
	}

	return []string{"MESSAGE"}, nil
}

func parseLegacyEvents(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	events := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.ToUpper(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		events = append(events, value)
	}
	return events
}

func normalizedConnectorEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "enabled", "global", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func connectorState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func parseProxyConfig(raw string) *ProxyConfig {
	config := &ProxyConfig{
		Enabled:  false,
		Protocol: "socks5",
	}
	if strings.TrimSpace(raw) == "" {
		return config
	}

	type proxyJSON struct {
		Host     string `json:"host"`
		Port     string `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	var payload proxyJSON
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return config
	}

	config.Host = strings.TrimSpace(payload.Host)
	config.Port = strings.TrimSpace(payload.Port)
	config.Username = strings.TrimSpace(payload.Username)
	config.Password = strings.TrimSpace(payload.Password)
	config.Enabled = config.Host != "" && config.Port != ""
	return config
}

func normalizeProxyConfig(input ProxyConfig) (ProxyConfig, error) {
	input.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	if input.Protocol == "" {
		input.Protocol = "socks5"
	}
	if input.Protocol != "socks5" {
		return ProxyConfig{}, fmt.Errorf("%w: only socks5 proxy protocol is supported", domain.ErrValidation)
	}

	input.Host = strings.TrimSpace(input.Host)
	input.Port = strings.TrimSpace(input.Port)
	input.Username = strings.TrimSpace(input.Username)
	input.Password = strings.TrimSpace(input.Password)

	if !input.Enabled {
		return ProxyConfig{
			Enabled:  false,
			Protocol: "socks5",
		}, nil
	}

	if input.Host == "" || input.Port == "" {
		return ProxyConfig{}, fmt.Errorf("%w: host and port are required when proxy is enabled", domain.ErrValidation)
	}

	return input, nil
}

func isSupportedInstanceEvent(value string) bool {
	switch value {
	case "ALL",
		"MESSAGE",
		"SEND_MESSAGE",
		"READ_RECEIPT",
		"PRESENCE",
		"HISTORY_SYNC",
		"CHAT_PRESENCE",
		"CALL",
		"CONNECTION",
		"LABEL",
		"CONTACT",
		"GROUP",
		"NEWSLETTER",
		"QRCODE":
		return true
	default:
		return false
	}
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

func (s *Service) storeSendTextJob(tenantID string, status SendTextJobStatus) {
	sendstatus.Store(tenantID, status)
}

func (s *Service) loadSendTextJob(tenantID, jobID string) (SendTextJobStatus, bool) {
	return sendstatus.Load(tenantID, jobID)
}
