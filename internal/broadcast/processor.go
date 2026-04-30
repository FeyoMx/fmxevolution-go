package broadcast

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/instance"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type deliveryProcessor struct {
	repo      repository.BroadcastRepository
	instances instanceFinder
	contacts  contactLister
	sender    textSender
	logger    *slog.Logger
}

func newDeliveryProcessor(repo repository.BroadcastRepository, instances instanceFinder, contacts contactLister, sender textSender, logger *slog.Logger) processor {
	return &deliveryProcessor{
		repo:      repo,
		instances: instances,
		contacts:  contacts,
		sender:    sender,
		logger:    logger,
	}
}

func (p *deliveryProcessor) Process(ctx context.Context, job repository.BroadcastJob) error {
	if p.repo == nil || p.instances == nil || p.contacts == nil || p.sender == nil {
		return permanentProcessorError(fmt.Errorf("%w: broadcast delivery dependencies are unavailable", domain.ErrConflict))
	}

	if strings.TrimSpace(job.TenantID) == "" || strings.TrimSpace(job.InstanceID) == "" || strings.TrimSpace(job.Message) == "" {
		return permanentProcessorError(fmt.Errorf("%w: broadcast job is missing tenant_id, instance_id, or message", domain.ErrValidation))
	}

	instanceRecord, err := p.instances.GetByID(ctx, job.TenantID, job.InstanceID)
	if err != nil {
		return permanentProcessorError(fmt.Errorf("%w: broadcast instance lookup failed: %v", domain.ErrForbidden, err))
	}
	if instanceRecord == nil {
		return permanentProcessorError(fmt.Errorf("%w: broadcast instance not found for tenant", domain.ErrForbidden))
	}

	progress, err := p.loadOrSeedRecipientProgress(ctx, job)
	if err != nil {
		return err
	}
	if len(progress) == 0 {
		return permanentProcessorError(fmt.Errorf("%w: no eligible contacts available for this instance broadcast", domain.ErrConflict))
	}

	pending := pendingRecipientProgress(progress)
	if len(pending) == 0 {
		if p.logger != nil {
			p.logger.Info(
				"broadcast resume found no pending recipients",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"instance_id", job.InstanceID,
				"recipient_total", len(progress),
			)
		}
		return nil
	}

	for idx := range pending {
		recipient := pending[idx]
		if idx > 0 {
			if err := waitForRecipientPacing(ctx, job.RatePerHour); err != nil {
				return retryableProcessorError(fmt.Errorf("broadcast pacing interrupted before recipient %s: %w", recipient.Phone, err))
			}
		}

		attemptedAt := time.Now().UTC()
		recipient.AttemptCount++
		recipient.LastAttemptAt = &attemptedAt
		recipient.LastError = ""
		recipient.DeliveryStatus = recipientStatusPending
		if err := p.repo.SaveRecipientProgress(ctx, &recipient); err != nil {
			return retryableProcessorError(fmt.Errorf("persist broadcast recipient attempt for %s: %w", recipient.Phone, err))
		}

		if p.logger != nil {
			p.logger.Debug(
				"broadcast recipient send attempt",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"instance_id", job.InstanceID,
				"recipient", recipient.Phone,
				"recipient_index", idx+1,
				"recipient_total", len(progress),
				"attempt", recipient.AttemptCount,
				"max_attempts", job.MaxAttempts,
			)
		}

		result, _, sendErr := p.sender.SendText(ctx, job.TenantID, instanceRecord.ID, instance.SendTextInput{
			Number: recipient.Phone,
			Text:   job.Message,
		})
		if sendErr != nil {
			recipient.LastError = sendErr.Error()
			if p.logger != nil {
				p.logger.Warn(
					"broadcast recipient send failed",
					"job_id", job.ID,
					"tenant_id", job.TenantID,
					"instance_id", job.InstanceID,
					"recipient", recipient.Phone,
					"recipient_index", idx+1,
					"recipient_total", len(progress),
					"attempt", recipient.AttemptCount,
					"max_attempts", job.MaxAttempts,
					"error", sendErr.Error(),
				)
			}
			if isPermanentSendError(sendErr) {
				failedAt := time.Now().UTC()
				recipient.DeliveryStatus = recipientStatusFailed
				recipient.FailedAt = &failedAt
				if err := p.repo.SaveRecipientProgress(ctx, &recipient); err != nil {
					return retryableProcessorError(fmt.Errorf("persist permanent broadcast recipient failure for %s: %w", recipient.Phone, err))
				}
				continue
			}
			recipient.DeliveryStatus = recipientStatusPending
			recipient.FailedAt = nil
			if err := p.repo.SaveRecipientProgress(ctx, &recipient); err != nil {
				return retryableProcessorError(fmt.Errorf("persist retryable broadcast recipient failure for %s: %w", recipient.Phone, err))
			}
			return retryableProcessorError(fmt.Errorf("broadcast delivery paused after retryable failure on %s: %w", recipient.Phone, sendErr))
		}
		if !isConfirmedSendAttempt(result) {
			reason := fmt.Errorf("instance send path returned no delivery evidence for recipient %s", recipient.Phone)
			recipient.DeliveryStatus = recipientStatusPending
			recipient.LastError = reason.Error()
			recipient.FailedAt = nil
			if p.logger != nil {
				p.logger.Warn(
					"broadcast recipient send returned no delivery evidence",
					"job_id", job.ID,
					"tenant_id", job.TenantID,
					"instance_id", job.InstanceID,
					"recipient", recipient.Phone,
					"recipient_index", idx+1,
					"recipient_total", len(progress),
					"attempt", recipient.AttemptCount,
					"max_attempts", job.MaxAttempts,
				)
			}
			if err := p.repo.SaveRecipientProgress(ctx, &recipient); err != nil {
				return retryableProcessorError(fmt.Errorf("persist unconfirmed broadcast recipient result for %s: %w", recipient.Phone, err))
			}
			return retryableProcessorError(fmt.Errorf("broadcast delivery paused after unconfirmed send result on %s: %w", recipient.Phone, reason))
		}

		sentAt := attemptedAt
		if !result.Timestamp.IsZero() {
			sentAt = result.Timestamp.UTC()
		}
		recipient.DeliveryStatus = recipientStatusSent
		recipient.LastError = ""
		recipient.SentAt = &sentAt
		recipient.FailedAt = nil
		recipient.LastStatusAt = &sentAt
		recipient.StatusSource = "send_result"
		recipient.MessageID = strings.TrimSpace(result.MessageID)
		recipient.ServerID = result.ServerID
		recipient.ChatJID = strings.TrimSpace(result.Chat)
		if err := p.repo.SaveRecipientProgress(ctx, &recipient); err != nil {
			return retryableProcessorError(fmt.Errorf("persist broadcast recipient success for %s: %w", recipient.Phone, err))
		}
		if p.logger != nil {
			p.logger.Debug(
				"broadcast recipient delivered",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"instance_id", job.InstanceID,
				"recipient", recipient.Phone,
				"recipient_index", idx+1,
				"recipient_total", len(progress),
				"message_id", recipient.MessageID,
			)
		}
	}

	if p.logger != nil {
		summary, summaryErr := p.repo.SummarizeRecipientProgress(ctx, job.TenantID, job.ID)
		if summaryErr == nil {
			p.logger.Info(
				"broadcast delivery completed",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"instance_id", job.InstanceID,
				"recipient_total", summary.TotalRecipients,
				"recipient_sent", summary.Sent,
				"recipient_failed", summary.Failed,
				"recipient_pending", summary.Pending,
			)
			return nil
		}
		p.logger.Info(
			"broadcast delivery completed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"instance_id", job.InstanceID,
			"recipient_total", len(progress),
		)
	}

	return nil
}

func isConfirmedSendAttempt(result *instance.SendTextResult) bool {
	if result == nil {
		return false
	}
	return strings.TrimSpace(result.MessageID) != "" ||
		result.ServerID != 0 ||
		!result.Timestamp.IsZero() ||
		strings.TrimSpace(result.Chat) != ""
}

type broadcastRecipient struct {
	ContactID *string
	Phone     string
}

func eligibleBroadcastRecipients(contacts []repository.Contact, instanceID string) []broadcastRecipient {
	recipients := make([]broadcastRecipient, 0, len(contacts))
	seen := make(map[string]struct{}, len(contacts))

	for _, contact := range contacts {
		phone := strings.TrimSpace(contact.Phone)
		if phone == "" {
			continue
		}
		if linkedInstance := strings.TrimSpace(contact.InstanceID); linkedInstance != "" && linkedInstance != instanceID {
			continue
		}
		if _, ok := seen[phone]; ok {
			continue
		}
		seen[phone] = struct{}{}
		var contactID *string
		if strings.TrimSpace(contact.ID) != "" {
			id := strings.TrimSpace(contact.ID)
			contactID = &id
		}
		recipients = append(recipients, broadcastRecipient{ContactID: contactID, Phone: phone})
	}

	return recipients
}

func (p *deliveryProcessor) loadOrSeedRecipientProgress(ctx context.Context, job repository.BroadcastJob) ([]repository.BroadcastRecipientProgress, error) {
	progress, err := p.repo.ListRecipientProgress(ctx, job.TenantID, job.ID)
	if err != nil {
		return nil, retryableProcessorError(fmt.Errorf("load broadcast recipient progress: %w", err))
	}
	if len(progress) > 0 {
		return progress, nil
	}

	contacts, err := p.contacts.ListContacts(ctx, job.TenantID)
	if err != nil {
		return nil, retryableProcessorError(fmt.Errorf("load broadcast contacts: %w", err))
	}

	recipients := eligibleBroadcastRecipients(contacts, job.InstanceID)
	if len(recipients) == 0 {
		return nil, nil
	}

	seed := make([]repository.BroadcastRecipientProgress, 0, len(recipients))
	for _, recipient := range recipients {
		seed = append(seed, repository.BroadcastRecipientProgress{
			BroadcastID:    job.ID,
			TenantID:       job.TenantID,
			InstanceID:     job.InstanceID,
			ContactID:      recipient.ContactID,
			Phone:          recipient.Phone,
			DeliveryStatus: recipientStatusPending,
		})
	}
	if err := p.repo.SeedRecipientProgress(ctx, seed); err != nil {
		return nil, retryableProcessorError(fmt.Errorf("seed broadcast recipient progress: %w", err))
	}

	return p.repo.ListRecipientProgress(ctx, job.TenantID, job.ID)
}

func pendingRecipientProgress(progress []repository.BroadcastRecipientProgress) []repository.BroadcastRecipientProgress {
	pending := make([]repository.BroadcastRecipientProgress, 0, len(progress))
	for _, item := range progress {
		switch strings.ToLower(strings.TrimSpace(item.DeliveryStatus)) {
		case recipientStatusSent, recipientStatusDelivered, recipientStatusRead, recipientStatusFailed:
			continue
		default:
			pending = append(pending, item)
		}
	}
	return pending
}

func waitForRecipientPacing(ctx context.Context, ratePerHour int) error {
	interval := intervalForRate(ratePerHour)
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isPermanentSendError(err error) bool {
	return err != nil && (strings.Contains(strings.ToLower(err.Error()), domain.ErrValidation.Error()) ||
		strings.Contains(strings.ToLower(err.Error()), domain.ErrForbidden.Error()) ||
		strings.Contains(strings.ToLower(err.Error()), domain.ErrNotFound.Error()) ||
		strings.Contains(strings.ToLower(err.Error()), "not registered on whatsapp"))
}
