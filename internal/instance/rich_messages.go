package instance

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

// legacyRuntimeFor resolves the instance and returns the connected LegacyRuntime,
// shared by all rich-message service methods.
func (s *Service) legacyRuntimeFor(ctx context.Context, tenantID, reference string) (*repository.Instance, *LegacyRuntime, error) {
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
	legacyRuntime, ok := runtime.(*LegacyRuntime)
	if !ok {
		return instance, nil, fmt.Errorf("legacy runtime unavailable")
	}
	return instance, legacyRuntime, nil
}

// SendReaction sends an emoji reaction to a message.
func (s *Service) SendReaction(ctx context.Context, tenantID, reference, number, messageID, reaction string, fromMe bool) (*SendTextResult, *repository.Instance, error) {
	instance, rt, err := s.legacyRuntimeFor(ctx, tenantID, reference)
	if err != nil {
		return nil, instance, err
	}
	if strings.TrimSpace(number) == "" || strings.TrimSpace(messageID) == "" {
		return nil, instance, fmt.Errorf("%w: number and message id are required", domain.ErrValidation)
	}
	result, err := rt.SendReaction(ctx, instance, number, messageID, reaction, fromMe)
	return result, instance, err
}

// SendPoll sends a poll message.
func (s *Service) SendPoll(ctx context.Context, tenantID, reference, number, name string, values []string, selectableCount int) (*SendTextResult, *repository.Instance, error) {
	instance, rt, err := s.legacyRuntimeFor(ctx, tenantID, reference)
	if err != nil {
		return nil, instance, err
	}
	result, err := rt.SendPoll(ctx, instance, number, name, values, selectableCount)
	return result, instance, err
}

// SendContact sends one or more contact cards.
func (s *Service) SendContact(ctx context.Context, tenantID, reference, number string, contacts []ContactCard) (*SendTextResult, *repository.Instance, error) {
	instance, rt, err := s.legacyRuntimeFor(ctx, tenantID, reference)
	if err != nil {
		return nil, instance, err
	}
	result, err := rt.SendContact(ctx, instance, number, contacts)
	return result, instance, err
}

// CheckNumbers reports which numbers are registered on WhatsApp.
func (s *Service) CheckNumbers(ctx context.Context, tenantID, reference string, numbers []string) ([]NumberCheckResult, *repository.Instance, error) {
	instance, rt, err := s.legacyRuntimeFor(ctx, tenantID, reference)
	if err != nil {
		return nil, instance, err
	}
	if len(numbers) == 0 {
		return nil, instance, fmt.Errorf("%w: at least one number is required", domain.ErrValidation)
	}
	result, err := rt.CheckNumbers(ctx, instance, numbers)
	return result, instance, err
}
