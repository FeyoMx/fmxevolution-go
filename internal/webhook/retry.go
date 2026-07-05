package webhook

import (
	"context"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

const retryPollInterval = 15 * time.Second

// Start launches the background retry worker that re-attempts failed webhook
// deliveries whose backoff has elapsed. Safe to call once at app startup.
func (s *Service) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	s.stopWorker = cancel
	s.workerDone = make(chan struct{})

	go func() {
		defer close(s.workerDone)
		ticker := time.NewTicker(retryPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				s.processDueRetries(workerCtx)
			}
		}
	}()
}

// Stop signals the worker to exit and waits briefly for it.
func (s *Service) Stop(ctx context.Context) error {
	if s.stopWorker != nil {
		s.stopWorker()
	}
	if s.workerDone == nil {
		return nil
	}
	select {
	case <-s.workerDone:
	case <-ctx.Done():
	}
	return nil
}

func (s *Service) processDueRetries(ctx context.Context) {
	due, err := s.repo.ListDueRetries(ctx, time.Now().UTC(), 50)
	if err != nil {
		s.logger.Error("list due webhook retries failed", "error", err)
		return
	}
	for i := range due {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.retryDelivery(ctx, &due[i])
	}
}

func (s *Service) retryDelivery(ctx context.Context, delivery *repository.WebhookDelivery) {
	endpoint := repository.WebhookEndpoint{
		ID:       delivery.EndpointID,
		TenantID: delivery.TenantID,
		URL:      delivery.EndpointURL,
	}
	// Re-derive the signing secret from the current endpoint config so rotating
	// a secret does not break in-flight retries.
	if stored, err := s.repo.GetByID(ctx, delivery.TenantID, delivery.EndpointID); err == nil && stored != nil {
		endpoint = *stored
	}

	body := []byte(delivery.RequestBody)
	envelope := eventEnvelope{
		EventID:   delivery.EventID,
		TenantID:  delivery.TenantID,
		Direction: delivery.Direction,
		EventType: delivery.EventType,
		MessageID: delivery.MessageID,
	}

	statusCode, respBody, sendErr := s.sendOnce(ctx, endpoint, envelope, body)
	delivery.Attempts++
	delivery.ResponseStatus = statusCode
	delivery.ResponseBody = respBody

	if sendErr == nil && statusCode >= 200 && statusCode < 300 {
		now := time.Now().UTC()
		delivery.Status = "delivered"
		delivery.DeliveredAt = &now
		delivery.NextAttemptAt = nil
		delivery.ErrorMessage = ""
		s.logger.Info("webhook retry delivered",
			"delivery_id", delivery.ID, "endpoint_id", delivery.EndpointID,
			"tenant_id", delivery.TenantID, "attempts", delivery.Attempts)
	} else {
		delivery.ErrorMessage = deliveryError(sendErr, respBody)
		s.scheduleRetryFields(delivery, delivery.Attempts)
		s.logger.Warn("webhook retry failed",
			"delivery_id", delivery.ID, "endpoint_id", delivery.EndpointID,
			"tenant_id", delivery.TenantID, "attempts", delivery.Attempts,
			"next_status", delivery.Status, "error", delivery.ErrorMessage)
	}

	if err := s.repo.UpdateDelivery(ctx, delivery); err != nil {
		s.logger.Error("update webhook delivery failed", "delivery_id", delivery.ID, "error", err)
	}
}

// ListDeliveries returns delivery records for a tenant.
func (s *Service) ListDeliveries(ctx context.Context, tenantID string, filter repository.WebhookDeliveryFilter) ([]repository.WebhookDelivery, error) {
	return s.repo.ListDeliveries(ctx, tenantID, filter)
}
