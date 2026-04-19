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
	"github.com/EvolutionAPI/evolution-go/pkg/runtimeobs"
	"github.com/EvolutionAPI/evolution-go/pkg/sendstatus"
	"github.com/google/uuid"
)

type Service struct {
	repo           repository.InstanceRepository
	history        repository.ConversationMessageRepository
	observability  repository.RuntimeObservabilityRepository
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

type PairInput struct {
	Phone string `json:"phone"`
}

type HistoryBackfillInput struct {
	ChatJID   string    `json:"chat_jid"`
	MessageID string    `json:"message_id"`
	Timestamp time.Time `json:"timestamp"`
	IsFromMe  bool      `json:"is_from_me"`
	IsGroup   bool      `json:"is_group"`
	Count     int       `json:"count"`
}

type SendMediaOutput = SendMediaResult

type SendTextJobStatus = sendstatus.JobStatus

func NewService(repo repository.InstanceRepository, history repository.ConversationMessageRepository, observability repository.RuntimeObservabilityRepository, runtime Runtime, runtimeFactory func() (Runtime, error), logger *slog.Logger) *Service {
	return &Service{
		repo:           repo,
		history:        history,
		observability:  observability,
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
	s.logLifecycleAction("connect_requested", instance, reference)

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Connect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("connect legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		s.recordRuntimeObservation(ctx, instance, snapshot, "connected", "api", "instance connect queued", nil)
		s.logLifecycleResult("connect_requested", instance, snapshot, nil)
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("connect runtime unavailable, using local status only", "instance_id", instance.ID, "reference", reference, "error", ensureErr)
	}

	instance.Status = "connecting"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	s.recordRuntimeObservation(ctx, instance, nil, "connected", "api", "instance connect queued", nil)
	s.logLifecycleResult("connect_requested", instance, nil, nil)
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
		s.recordRuntimeObservation(ctx, instance, snapshot, "connected", "api", "instance connect queued", nil)
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("connect runtime unavailable, using local status only", "instance_id", instance.ID, "error", ensureErr)
	}

	instance.Status = "connecting"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	s.recordRuntimeObservation(ctx, instance, nil, "connected", "api", "instance connect queued", nil)
	return instance, nil
}

func (s *Service) Disconnect(ctx context.Context, tenantID, reference string) (*repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, err
	}
	s.logLifecycleAction("disconnect_requested", instance, reference)

	if runtime, ensureErr := s.ensureRuntime(); runtime != nil {
		snapshot, runtimeErr := runtime.Disconnect(ctx, instance)
		if runtimeErr != nil {
			if s.logger != nil {
				s.logger.Error("disconnect legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
			}
			return nil, runtimeErr
		}
		s.recordRuntimeObservation(ctx, instance, snapshot, "disconnected", "api", "instance disconnected", nil)
		s.logLifecycleResult("disconnect_requested", instance, snapshot, nil)
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("disconnect runtime unavailable, using local status only", "instance_id", instance.ID, "reference", reference, "error", ensureErr)
	}

	instance.Status = "disconnected"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	s.recordRuntimeObservation(ctx, instance, nil, "disconnected", "api", "instance disconnected", nil)
	s.logLifecycleResult("disconnect_requested", instance, nil, nil)
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
		s.recordRuntimeObservation(ctx, instance, snapshot, "disconnected", "api", "instance disconnected", nil)
		return s.applySnapshot(ctx, instance, snapshot)
	} else if ensureErr != nil && s.logger != nil {
		s.logger.Warn("disconnect runtime unavailable, using local status only", "instance_id", instance.ID, "error", ensureErr)
	}

	instance.Status = "disconnected"
	if err := s.repo.Update(ctx, instance); err != nil {
		return nil, err
	}
	s.recordRuntimeObservation(ctx, instance, nil, "disconnected", "api", "instance disconnected", nil)
	return instance, nil
}

func (s *Service) Reconnect(ctx context.Context, tenantID, reference string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	s.logLifecycleAction("reconnect_requested", instance, reference)

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		err := normalizeBridgeUnavailableLifecycleError(ensureErr, "reconnect")
		s.logLifecycleResult("reconnect_requested", instance, nil, err)
		return instance, nil, err
	}

	snapshot, runtimeErr := runtime.Reconnect(ctx, instance)
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("reconnect legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
		}
		err := normalizeBridgeUnavailableLifecycleError(runtimeErr, "reconnect")
		s.logLifecycleResult("reconnect_requested", instance, nil, err)
		return instance, nil, err
	}

	s.recordRuntimeObservation(ctx, instance, snapshot, "reconnect_requested", "api", "instance reconnect queued", nil)
	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	s.logLifecycleResult("reconnect_requested", instance, snapshot, nil)
	return instance, snapshot, nil
}

func (s *Service) ReconnectByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		return instance, nil, normalizeBridgeUnavailableLifecycleError(ensureErr, "reconnect")
	}

	snapshot, runtimeErr := runtime.Reconnect(ctx, instance)
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("reconnect legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
		}
		return instance, nil, normalizeBridgeUnavailableLifecycleError(runtimeErr, "reconnect")
	}

	s.recordRuntimeObservation(ctx, instance, snapshot, "reconnect_requested", "api", "instance reconnect queued", nil)
	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	return instance, snapshot, nil
}

func (s *Service) Logout(ctx context.Context, tenantID, reference string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	s.logLifecycleAction("logout_requested", instance, reference)

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		err := normalizeBridgeUnavailableLifecycleError(ensureErr, "logout")
		s.logLifecycleResult("logout_requested", instance, nil, err)
		return instance, nil, err
	}

	snapshot, runtimeErr := runtime.Logout(ctx, instance)
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("logout legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
		}
		err := normalizeBridgeUnavailableLifecycleError(runtimeErr, "logout")
		s.logLifecycleResult("logout_requested", instance, nil, err)
		return instance, nil, err
	}

	s.recordRuntimeObservation(ctx, instance, snapshot, "logout", "api", "instance logged out", nil)
	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	s.logLifecycleResult("logout_requested", instance, snapshot, nil)
	return instance, snapshot, nil
}

func (s *Service) LogoutByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		return instance, nil, normalizeBridgeUnavailableLifecycleError(ensureErr, "logout")
	}

	snapshot, runtimeErr := runtime.Logout(ctx, instance)
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("logout legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
		}
		return instance, nil, normalizeBridgeUnavailableLifecycleError(runtimeErr, "logout")
	}

	s.recordRuntimeObservation(ctx, instance, snapshot, "logout", "api", "instance logged out", nil)
	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	return instance, snapshot, nil
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
	s.recordRuntimeObservation(ctx, instance, snapshot, "status_observed", "runtime_snapshot", "live runtime status observed", nil)
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
	s.recordRuntimeObservation(ctx, instance, snapshot, "status_observed", "runtime_snapshot", "live runtime status observed", nil)
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

func (s *Service) Pair(ctx context.Context, tenantID, reference string, input PairInput) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	s.logLifecycleAction("pair_requested", instance, reference)

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		err := normalizeBridgeUnavailableLifecycleError(ensureErr, "pair")
		s.logLifecycleResult("pair_requested", instance, nil, err)
		return instance, nil, err
	}

	snapshot, runtimeErr := runtime.Pair(ctx, instance, strings.TrimSpace(input.Phone))
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("pair legacy runtime failed", "instance_id", instance.ID, "reference", reference, "error", runtimeErr)
		}
		err := normalizeBridgeUnavailableLifecycleError(runtimeErr, "pair")
		s.logLifecycleResult("pair_requested", instance, nil, err)
		return instance, nil, err
	}

	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	s.recordRuntimeObservation(ctx, instance, snapshot, "pairing_started", "api", "pairing code generated", nil)
	s.logLifecycleResult("pair_requested", instance, snapshot, nil)
	return instance, snapshot, nil
}

func (s *Service) PairByID(ctx context.Context, tenantID, instanceID string, input PairInput) (*repository.Instance, *RuntimeSnapshot, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, err
	}

	runtime, ensureErr := s.ensureRuntime()
	if runtime == nil {
		return instance, nil, normalizeBridgeUnavailableLifecycleError(ensureErr, "pair")
	}

	snapshot, runtimeErr := runtime.Pair(ctx, instance, strings.TrimSpace(input.Phone))
	if runtimeErr != nil {
		if s.logger != nil {
			s.logger.Error("pair legacy runtime failed", "instance_id", instance.ID, "error", runtimeErr)
		}
		return instance, nil, normalizeBridgeUnavailableLifecycleError(runtimeErr, "pair")
	}

	instance, err = s.applySnapshot(ctx, instance, snapshot)
	if err != nil {
		return nil, nil, err
	}
	s.recordRuntimeObservation(ctx, instance, snapshot, "pairing_started", "api", "pairing code generated", nil)
	return instance, snapshot, nil
}

func (s *Service) RuntimeStatus(ctx context.Context, tenantID, reference string) (*repository.Instance, *repository.RuntimeSessionState, *RuntimeSnapshot, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, nil, err
	}
	state, err := s.loadRuntimeState(ctx, instance)
	if err != nil {
		return instance, nil, nil, err
	}
	snapshot := s.tryRuntimeSnapshot(ctx, instance)
	if snapshot != nil {
		s.recordRuntimeObservation(ctx, instance, snapshot, "status_observed", "runtime_snapshot", "live runtime status observed", nil)
		if latest, latestErr := s.loadRuntimeState(ctx, instance); latestErr == nil && latest != nil {
			state = latest
		}
	}
	return instance, state, snapshot, nil
}

func (s *Service) RuntimeStatusByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, *repository.RuntimeSessionState, *RuntimeSnapshot, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, nil, err
	}
	state, err := s.loadRuntimeState(ctx, instance)
	if err != nil {
		return instance, nil, nil, err
	}
	snapshot := s.tryRuntimeSnapshot(ctx, instance)
	if snapshot != nil {
		s.recordRuntimeObservation(ctx, instance, snapshot, "status_observed", "runtime_snapshot", "live runtime status observed", nil)
		if latest, latestErr := s.loadRuntimeState(ctx, instance); latestErr == nil && latest != nil {
			state = latest
		}
	}
	return instance, state, snapshot, nil
}

func (s *Service) RuntimeHistory(ctx context.Context, tenantID, reference string, limit int) (*repository.Instance, []repository.RuntimeSessionEvent, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}
	events, err := s.listRuntimeEvents(ctx, instance, limit)
	if err != nil {
		return instance, nil, err
	}
	return instance, events, nil
}

func (s *Service) RuntimeHistoryByID(ctx context.Context, tenantID, instanceID string, limit int) (*repository.Instance, []repository.RuntimeSessionEvent, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, err
	}
	events, err := s.listRuntimeEvents(ctx, instance, limit)
	if err != nil {
		return instance, nil, err
	}
	return instance, events, nil
}

func (s *Service) BackfillHistory(ctx context.Context, tenantID, reference string, input HistoryBackfillInput) (*repository.Instance, *HistoryBackfillResult, string, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, "", err
	}
	return s.backfillHistoryForInstance(ctx, instance, input)
}

func (s *Service) BackfillHistoryByID(ctx context.Context, tenantID, instanceID string, input HistoryBackfillInput) (*repository.Instance, *HistoryBackfillResult, string, error) {
	instance, err := s.Get(ctx, tenantID, instanceID)
	if err != nil {
		return nil, nil, "", err
	}
	return s.backfillHistoryForInstance(ctx, instance, input)
}

func (s *Service) backfillHistoryForInstance(ctx context.Context, instance *repository.Instance, input HistoryBackfillInput) (*repository.Instance, *HistoryBackfillResult, string, error) {
	if instance == nil {
		return nil, nil, "", fmt.Errorf("%w: instance not found", domain.ErrNotFound)
	}

	request, anchorSource, err := s.resolveHistoryBackfillRequest(ctx, instance, input)
	if err != nil {
		return instance, nil, "", err
	}
	if s.logger != nil {
		s.logger.Info(
			"history backfill requested",
			"instance_id", instance.ID,
			"tenant_id", instance.TenantID,
			"chat_jid", request.ChatJID,
			"count", request.Count,
			"anchor_source", anchorSource,
			"message_id", request.MessageID,
		)
	}

	runtime, runtimeErr := s.ensureRuntime()
	if runtimeErr != nil && s.logger != nil {
		s.logger.Warn("history backfill runtime unavailable", "instance_id", instance.ID, "error", runtimeErr)
	}
	if runtime == nil {
		return instance, nil, "", fmt.Errorf("%w: runtime unavailable for history backfill", domain.ErrConflict)
	}

	result, err := runtime.RequestHistorySync(ctx, instance, request)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("history backfill request failed", "instance_id", instance.ID, "chat_jid", request.ChatJID, "error", err)
		}
		return instance, nil, anchorSource, err
	}

	s.recordRuntimeObservation(ctx, instance, s.tryRuntimeSnapshot(ctx, instance), "history_sync_requested", "api", "history backfill requested", nil)
	if s.logger != nil {
		s.logger.Info(
			"history backfill accepted",
			"instance_id", instance.ID,
			"tenant_id", instance.TenantID,
			"chat_jid", request.ChatJID,
			"count", request.Count,
			"anchor_source", anchorSource,
		)
	}
	return instance, result, anchorSource, nil
}

func (s *Service) resolveHistoryBackfillRequest(ctx context.Context, instance *repository.Instance, input HistoryBackfillInput) (HistoryBackfillRequest, string, error) {
	request := HistoryBackfillRequest{
		ChatJID:   strings.TrimSpace(input.ChatJID),
		MessageID: strings.TrimSpace(input.MessageID),
		Timestamp: input.Timestamp.UTC(),
		IsFromMe:  input.IsFromMe,
		IsGroup:   input.IsGroup,
		Count:     input.Count,
	}

	if request.Count <= 0 {
		request.Count = 50
	}
	if request.Count > 200 {
		request.Count = 200
	}

	if request.ChatJID != "" && request.MessageID != "" && !request.Timestamp.IsZero() {
		if !request.IsGroup {
			request.IsGroup = strings.HasSuffix(request.ChatJID, "@g.us")
		}
		return request, "explicit", nil
	}

	if request.ChatJID == "" {
		return HistoryBackfillRequest{}, "", fmt.Errorf("%w: chat_jid is required for history backfill", domain.ErrValidation)
	}
	if request.MessageID != "" || !request.Timestamp.IsZero() {
		return HistoryBackfillRequest{}, "", fmt.Errorf("%w: message_id and timestamp must be provided together", domain.ErrValidation)
	}
	if s.history == nil {
		return HistoryBackfillRequest{}, "", fmt.Errorf("%w: no stored history is available to derive a backfill anchor", domain.ErrConflict)
	}

	items, err := s.history.List(ctx, instance.TenantID, instance.ID, repository.ConversationMessageFilter{
		RemoteJID: request.ChatJID,
		Limit:     1,
	})
	if err != nil {
		return HistoryBackfillRequest{}, "", err
	}
	if len(items) == 0 {
		return HistoryBackfillRequest{}, "", fmt.Errorf("%w: no stored message anchor found for chat_jid", domain.ErrConflict)
	}

	anchor := items[len(items)-1]
	if strings.TrimSpace(anchor.ExternalMessageID) == "" || anchor.MessageTimestamp.IsZero() {
		return HistoryBackfillRequest{}, "", fmt.Errorf("%w: stored history anchor is incomplete", domain.ErrConflict)
	}

	request.MessageID = strings.TrimSpace(anchor.ExternalMessageID)
	request.Timestamp = anchor.MessageTimestamp.UTC()
	request.IsFromMe = strings.EqualFold(anchor.Direction, "outbound")
	request.IsGroup = strings.HasSuffix(request.ChatJID, "@g.us")
	return request, "stored_history", nil
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

	s.persistOutboundText(ctx, tenantID, instance, input, message)
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

	s.persistOutboundMedia(ctx, tenantID, instance, input, message)
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

	s.persistOutboundAudio(ctx, tenantID, instance, input, message)
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

func (s *Service) SearchMessages(ctx context.Context, tenantID, reference string, input MessageSearchRequest) ([]legacyMessageRecord, *repository.Instance, error) {
	instance, err := s.resolve(ctx, tenantID, reference)
	if err != nil {
		return nil, nil, err
	}

	filter, err := normalizeMessageSearchRequest(input)
	if err != nil {
		return nil, instance, err
	}
	if s.history == nil {
		return nil, instance, fmt.Errorf("message history repository unavailable")
	}

	messages, err := s.history.List(ctx, tenantID, instance.ID, repository.ConversationMessageFilter{
		RemoteJID:         filter.RemoteJID,
		ExternalMessageID: filter.MessageID,
		Query:             filter.Query,
		Limit:             filter.Limit,
		Before:            filter.Before,
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Error(
				"search messages failed",
				"instance_id", instance.ID,
				"reference", reference,
				"remote_jid", filter.RemoteJID,
				"error", err,
			)
		}
		return nil, instance, err
	}

	return toLegacyMessageRecords(messages), instance, nil
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

	s.recordRuntimeObservation(ctx, instance, snapshot, "status_observed", "runtime_snapshot", "live runtime status observed", nil)
	return instance, nil
}

func (s *Service) loadRuntimeState(ctx context.Context, instance *repository.Instance) (*repository.RuntimeSessionState, error) {
	if instance == nil || s.observability == nil {
		return nil, nil
	}
	return s.observability.GetState(ctx, instance.TenantID, instance.ID)
}

func (s *Service) listRuntimeEvents(ctx context.Context, instance *repository.Instance, limit int) ([]repository.RuntimeSessionEvent, error) {
	if instance == nil || s.observability == nil {
		return []repository.RuntimeSessionEvent{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.observability.ListEvents(ctx, instance.TenantID, instance.ID, repository.RuntimeSessionEventFilter{Limit: limit})
}

func (s *Service) tryRuntimeSnapshot(ctx context.Context, instance *repository.Instance) *RuntimeSnapshot {
	runtime, _ := s.ensureRuntime()
	if runtime == nil || instance == nil {
		return nil
	}
	snapshot, err := runtime.Snapshot(ctx, instance)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("runtime snapshot failed", "instance_id", instance.ID, "tenant_id", instance.TenantID, "error", err)
		}
		return nil
	}
	return snapshot
}

func (s *Service) recordRuntimeObservation(ctx context.Context, instance *repository.Instance, snapshot *RuntimeSnapshot, eventType, source, message string, err error) {
	if instance == nil {
		return
	}

	now := time.Now().UTC()
	status := firstStatus(strings.TrimSpace(instance.Status), "created")
	connected := status == "open" || status == "connected"
	loggedIn := connected
	pairingActive := status == "qrcode" || status == "connecting"
	disconnectReason := ""
	errorMessage := ""

	if snapshot != nil {
		status = firstStatus(strings.TrimSpace(snapshot.Status), status)
		connected = snapshot.Connected
		loggedIn = snapshot.LoggedIn
		pairingActive = !snapshot.LoggedIn && (strings.TrimSpace(snapshot.QRCode) != "" || strings.TrimSpace(snapshot.PairingCode) != "" || status == "qrcode" || status == "connecting")
	}
	if strings.TrimSpace(eventType) == "logout" || strings.TrimSpace(eventType) == "disconnected" {
		connected = false
		loggedIn = false
		pairingActive = false
		if strings.TrimSpace(eventType) == "logout" && status == "open" {
			status = "close"
		}
	}
	if err != nil {
		errorMessage = strings.TrimSpace(err.Error())
	}

	runtimeobs.NotifyLifecycleEvent(runtimeobs.LifecycleEvent{
		InstanceID:       instance.ID,
		EventType:        strings.TrimSpace(eventType),
		EventSource:      firstStatus(source, "api"),
		Status:           status,
		Connected:        connected,
		LoggedIn:         loggedIn,
		PairingActive:    pairingActive,
		DisconnectReason: disconnectReason,
		ErrorMessage:     errorMessage,
		Message:          strings.TrimSpace(message),
		Payload:          runtimeObservationPayload(snapshot),
		OccurredAt:       now,
	})
}

func (s *Service) logLifecycleAction(action string, instance *repository.Instance, reference string) {
	if s.logger == nil || instance == nil {
		return
	}
	s.logger.Info(
		"instance lifecycle action requested",
		"action", strings.TrimSpace(action),
		"instance_id", instance.ID,
		"tenant_id", instance.TenantID,
		"reference", strings.TrimSpace(reference),
		"status", strings.TrimSpace(instance.Status),
	)
}

func (s *Service) logLifecycleResult(action string, instance *repository.Instance, snapshot *RuntimeSnapshot, err error) {
	if s.logger == nil || instance == nil {
		return
	}

	status := strings.TrimSpace(instance.Status)
	if snapshot != nil && strings.TrimSpace(snapshot.Status) != "" {
		status = strings.TrimSpace(snapshot.Status)
	}
	if err != nil {
		s.logger.Warn(
			"instance lifecycle action failed",
			"action", strings.TrimSpace(action),
			"instance_id", instance.ID,
			"tenant_id", instance.TenantID,
			"status", status,
			"error", err,
		)
		return
	}

	s.logger.Info(
		"instance lifecycle action completed",
		"action", strings.TrimSpace(action),
		"instance_id", instance.ID,
		"tenant_id", instance.TenantID,
		"status", status,
	)
}

func normalizeBridgeUnavailableLifecycleError(err error, action string) error {
	if err == nil {
		err = fmt.Errorf("runtime unavailable")
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "runtime unavailable"),
		strings.Contains(message, "legacy runtime unavailable"),
		strings.Contains(message, "legacy client runtime unavailable"):
		return fmt.Errorf("%w: runtime unavailable for %s", domain.ErrConflict, strings.TrimSpace(action))
	default:
		return err
	}
}

func runtimeObservationPayload(snapshot *RuntimeSnapshot) map[string]any {
	if snapshot == nil {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil
	}
	return decoded
}

func firstStatus(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
