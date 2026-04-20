package dashboard

import (
	"context"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type metricsService interface {
	Metrics(ctx context.Context, tenantID string) (MetricsSnapshot, error)
}

type instanceRepository interface {
	ListByTenant(ctx context.Context, tenantID string) ([]repository.Instance, error)
}

type contactCounter interface {
	CountContactsByTenant(ctx context.Context, tenantID string) (int64, error)
}

type messageCounter interface {
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
}

type broadcastCounter interface {
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
	SummarizeRecipientProgressByTenant(ctx context.Context, tenantID string) (repository.BroadcastRecipientAnalytics, error)
}

type runtimeStateLister interface {
	ListStatesByTenant(ctx context.Context, tenantID string) ([]repository.RuntimeSessionState, error)
}

type Service struct {
	instances  instanceRepository
	contacts   contactCounter
	messages   messageCounter
	broadcasts broadcastCounter
	runtime    runtimeStateLister
}

type MetricsSnapshot struct {
	InstancesTotal       int
	InstancesActive      int
	InstancesInactive    int
	ContactsTotal        int64
	MessagesTotal        int64
	MessagesTotalPartial bool
	BroadcastTotal       int64
	BroadcastRecipients  repository.BroadcastRecipientAnalytics
	RuntimeHealthy       int
	RuntimeDegraded      int
	RuntimeUnavailable   int
	RuntimeUnknown       int
	RuntimeHealthPartial bool
}

func NewService(instances instanceRepository, contacts contactCounter, messages messageCounter, broadcasts broadcastCounter, runtime runtimeStateLister) *Service {
	return &Service{
		instances:  instances,
		contacts:   contacts,
		messages:   messages,
		broadcasts: broadcasts,
		runtime:    runtime,
	}
}

func (s *Service) Metrics(ctx context.Context, tenantID string) (MetricsSnapshot, error) {
	var snapshot MetricsSnapshot

	instances, err := s.instances.ListByTenant(ctx, tenantID)
	if err != nil {
		return snapshot, err
	}
	snapshot.InstancesTotal = len(instances)

	active := 0
	for _, item := range instances {
		if isInstanceActive(item.Status) {
			active++
		}
	}
	snapshot.InstancesActive = active
	snapshot.InstancesInactive = len(instances) - active

	if s.contacts != nil {
		total, err := s.contacts.CountContactsByTenant(ctx, tenantID)
		if err != nil {
			return snapshot, err
		}
		snapshot.ContactsTotal = total
	}

	if s.messages != nil {
		total, err := s.messages.CountByTenant(ctx, tenantID)
		if err != nil {
			return snapshot, err
		}
		snapshot.MessagesTotal = total
		snapshot.MessagesTotalPartial = true
	}

	if s.broadcasts != nil {
		total, err := s.broadcasts.CountByTenant(ctx, tenantID)
		if err != nil {
			return snapshot, err
		}
		snapshot.BroadcastTotal = total

		recipientSummary, err := s.broadcasts.SummarizeRecipientProgressByTenant(ctx, tenantID)
		if err != nil {
			return snapshot, err
		}
		recipientSummary.Partial = total > 0 && recipientSummary.TrackedBroadcasts < total
		snapshot.BroadcastRecipients = recipientSummary
	}

	if s.runtime != nil {
		states, err := s.runtime.ListStatesByTenant(ctx, tenantID)
		if err != nil {
			return snapshot, err
		}
		stateByInstance := make(map[string]repository.RuntimeSessionState, len(states))
		for _, state := range states {
			stateByInstance[state.InstanceID] = state
		}

		active = 0
		for _, item := range instances {
			if state, ok := stateByInstance[item.ID]; ok {
				if isRuntimeActive(state) {
					active++
				}
				switch classifyRuntimeHealth(state) {
				case "healthy":
					snapshot.RuntimeHealthy++
				case "degraded":
					snapshot.RuntimeDegraded++
				default:
					snapshot.RuntimeUnavailable++
				}
				continue
			}

			if isInstanceActive(item.Status) {
				active++
			}
			snapshot.RuntimeUnknown++
		}
		snapshot.InstancesActive = active
		snapshot.InstancesInactive = len(instances) - active
		snapshot.RuntimeHealthPartial = snapshot.RuntimeUnknown > 0
	}

	return snapshot, nil
}

func isInstanceActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open", "connected":
		return true
	default:
		return false
	}
}

func isRuntimeActive(state repository.RuntimeSessionState) bool {
	return state.Connected || isInstanceActive(state.Status) || isInstanceActive(state.LastSeenStatus)
}

func classifyRuntimeHealth(state repository.RuntimeSessionState) string {
	if state.Connected || isInstanceActive(state.Status) || isInstanceActive(state.LastSeenStatus) {
		return "healthy"
	}
	if state.PairingActive {
		return "degraded"
	}
	if strings.TrimSpace(state.LastError) != "" || strings.TrimSpace(state.DisconnectReason) != "" {
		return "degraded"
	}
	if strings.EqualFold(strings.TrimSpace(state.LastEventType), "logout") ||
		strings.EqualFold(strings.TrimSpace(state.LastEventType), "disconnected") {
		return "unavailable"
	}
	switch strings.ToLower(strings.TrimSpace(state.Status)) {
	case "", "created":
		return "unavailable"
	case "connecting", "qrcode":
		return "degraded"
	default:
		return "unavailable"
	}
}
