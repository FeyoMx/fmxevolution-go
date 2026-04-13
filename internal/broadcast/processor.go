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
	instances instanceFinder
	contacts  contactLister
	sender    textSender
	logger    *slog.Logger
}

func newDeliveryProcessor(instances instanceFinder, contacts contactLister, sender textSender, logger *slog.Logger) processor {
	return &deliveryProcessor{
		instances: instances,
		contacts:  contacts,
		sender:    sender,
		logger:    logger,
	}
}

func (p *deliveryProcessor) Process(ctx context.Context, job repository.BroadcastJob) error {
	if p.instances == nil || p.contacts == nil || p.sender == nil {
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

	contacts, err := p.contacts.ListContacts(ctx, job.TenantID)
	if err != nil {
		return retryableProcessorError(fmt.Errorf("load broadcast contacts: %w", err))
	}

	recipients := eligibleBroadcastRecipients(contacts, job.InstanceID)
	if len(recipients) == 0 {
		return permanentProcessorError(fmt.Errorf("%w: no eligible contacts available for this instance broadcast", domain.ErrConflict))
	}

	sentCount := 0
	for idx, recipient := range recipients {
		if idx > 0 {
			if err := waitForRecipientPacing(ctx, job.RatePerHour); err != nil {
				if sentCount > 0 {
					return permanentProcessorError(fmt.Errorf("broadcast partially delivered to %d/%d contacts before pacing interruption: %w", sentCount, len(recipients), err))
				}
				return retryableProcessorError(fmt.Errorf("broadcast pacing interrupted before first delivery: %w", err))
			}
		}

		_, _, sendErr := p.sender.SendText(ctx, job.TenantID, instanceRecord.ID, instance.SendTextInput{
			Number: recipient.Phone,
			Text:   job.Message,
		})
		if sendErr != nil {
			if sentCount > 0 {
				return permanentProcessorError(fmt.Errorf("broadcast partially delivered to %d/%d contacts before failing on %s: %w", sentCount, len(recipients), recipient.Phone, sendErr))
			}
			if isPermanentSendError(sendErr) {
				return permanentProcessorError(fmt.Errorf("broadcast delivery failed before first send attempt completed: %w", sendErr))
			}
			return retryableProcessorError(fmt.Errorf("broadcast delivery failed before first send attempt completed: %w", sendErr))
		}

		sentCount++
		if p.logger != nil {
			p.logger.Info(
				"broadcast recipient delivered",
				"job_id", job.ID,
				"tenant_id", job.TenantID,
				"instance_id", job.InstanceID,
				"recipient", recipient.Phone,
				"recipient_index", sentCount,
				"recipient_total", len(recipients),
			)
		}
	}

	if p.logger != nil {
		p.logger.Info(
			"broadcast delivery completed",
			"job_id", job.ID,
			"tenant_id", job.TenantID,
			"instance_id", job.InstanceID,
			"recipient_total", len(recipients),
		)
	}

	return nil
}

type broadcastRecipient struct {
	Phone string
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
		recipients = append(recipients, broadcastRecipient{Phone: phone})
	}

	return recipients
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
