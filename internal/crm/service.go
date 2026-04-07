package crm

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type Service struct {
	repo repository.CRMRepository
}

type CreateContactInput struct {
	Name       string   `json:"name"`
	Phone      string   `json:"phone"`
	Email      string   `json:"email"`
	InstanceID string   `json:"instance_id"`
	Tags       []string `json:"tags"`
	Notes      []string `json:"notes"`
}

type UpdateContactInput struct {
	Name       string   `json:"name"`
	Phone      string   `json:"phone"`
	Email      string   `json:"email"`
	InstanceID string   `json:"instance_id"`
	Tags       []string `json:"tags"`
}

type CreateNoteInput struct {
	Body string `json:"body"`
}

type AssignTagsInput struct {
	Tags []string `json:"tags"`
}

func NewService(repo repository.CRMRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateContact(ctx context.Context, tenantID string, input CreateContactInput) (*repository.Contact, error) {
	if tenantID == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Phone) == "" {
		return nil, fmt.Errorf("%w: name and phone are required", domain.ErrValidation)
	}

	normalizedPhone := normalizePhone(input.Phone)
	if normalizedPhone == "" {
		return nil, fmt.Errorf("%w: phone must contain digits", domain.ErrValidation)
	}
	if existing, err := s.repo.FindContactByPhone(ctx, tenantID, normalizedPhone); err == nil && existing != nil {
		return nil, fmt.Errorf("%w: contact already exists for phone", domain.ErrConflict)
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}

	contact := &repository.Contact{
		TenantID:   tenantID,
		Name:       strings.TrimSpace(input.Name),
		Phone:      normalizedPhone,
		Email:      strings.ToLower(strings.TrimSpace(input.Email)),
		InstanceID: strings.TrimSpace(input.InstanceID),
	}

	if err := s.repo.CreateContact(ctx, contact); err != nil {
		return nil, err
	}

	if err := s.replaceTags(ctx, tenantID, contact.ID, input.Tags); err != nil {
		return nil, err
	}

	for _, noteBody := range input.Notes {
		if _, err := s.AddNote(ctx, tenantID, contact.ID, CreateNoteInput{Body: noteBody}); err != nil {
			return nil, err
		}
	}

	return s.GetContact(ctx, tenantID, contact.ID)
}

func (s *Service) ListContacts(ctx context.Context, tenantID string) ([]repository.Contact, error) {
	return s.repo.ListContacts(ctx, tenantID)
}

func (s *Service) GetContact(ctx context.Context, tenantID, contactID string) (*repository.Contact, error) {
	contact, err := s.repo.GetContact(ctx, tenantID, contactID)
	if err != nil {
		return nil, fmt.Errorf("%w: contact not found", domain.ErrNotFound)
	}
	return contact, nil
}

func (s *Service) UpdateContact(ctx context.Context, tenantID, contactID string, input UpdateContactInput) (*repository.Contact, error) {
	contact, err := s.GetContact(ctx, tenantID, contactID)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		contact.Name = strings.TrimSpace(input.Name)
	}
	if input.Phone != "" {
		normalizedPhone := normalizePhone(input.Phone)
		if normalizedPhone == "" {
			return nil, fmt.Errorf("%w: phone must contain digits", domain.ErrValidation)
		}
		if existing, lookupErr := s.repo.FindContactByPhone(ctx, tenantID, normalizedPhone); lookupErr == nil && existing != nil && existing.ID != contact.ID {
			return nil, fmt.Errorf("%w: contact already exists for phone", domain.ErrConflict)
		} else if lookupErr != nil && !isNotFound(lookupErr) {
			return nil, lookupErr
		}
		contact.Phone = normalizedPhone
	}
	if input.Email != "" {
		contact.Email = strings.ToLower(strings.TrimSpace(input.Email))
	}
	if input.InstanceID != "" {
		contact.InstanceID = strings.TrimSpace(input.InstanceID)
	}

	if err := s.repo.UpdateContact(ctx, contact); err != nil {
		return nil, err
	}

	if input.Tags != nil {
		if err := s.replaceTags(ctx, tenantID, contact.ID, input.Tags); err != nil {
			return nil, err
		}
	}

	return s.GetContact(ctx, tenantID, contact.ID)
}

func (s *Service) AddNote(ctx context.Context, tenantID, contactID string, input CreateNoteInput) (*repository.Note, error) {
	if strings.TrimSpace(input.Body) == "" {
		return nil, fmt.Errorf("%w: note body is required", domain.ErrValidation)
	}

	if _, err := s.GetContact(ctx, tenantID, contactID); err != nil {
		return nil, err
	}

	note := &repository.Note{
		TenantID:  tenantID,
		ContactID: contactID,
		Body:      strings.TrimSpace(input.Body),
	}
	if err := s.repo.CreateNote(ctx, note); err != nil {
		return nil, err
	}
	return note, nil
}

func (s *Service) AssignTags(ctx context.Context, tenantID, contactID string, input AssignTagsInput) (*repository.Contact, error) {
	if _, err := s.GetContact(ctx, tenantID, contactID); err != nil {
		return nil, err
	}

	if err := s.replaceTags(ctx, tenantID, contactID, input.Tags); err != nil {
		return nil, err
	}

	return s.GetContact(ctx, tenantID, contactID)
}

func (s *Service) replaceTags(ctx context.Context, tenantID, contactID string, tags []string) error {
	if tags == nil {
		return nil
	}

	tagIDs := make([]string, 0, len(tags))
	for _, rawTag := range tags {
		name := strings.TrimSpace(rawTag)
		if name == "" {
			continue
		}

		tag, err := s.repo.FindTagByName(ctx, tenantID, name)
		if err != nil && !isNotFound(err) {
			return err
		}
		if tag == nil {
			tag = &repository.Tag{
				TenantID: tenantID,
				Name:     name,
				Color:    defaultTagColor(name),
			}
			if createErr := s.repo.CreateTag(ctx, tag); createErr != nil {
				return createErr
			}
		}

		tagIDs = append(tagIDs, tag.ID)
	}

	return s.repo.AssignTags(ctx, tenantID, contactID, tagIDs)
}

func normalizePhone(phone string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
}

func defaultTagColor(tag string) string {
	palette := []string{"slate", "blue", "green", "amber", "rose", "violet"}
	total := 0
	for _, r := range tag {
		total += int(r)
	}
	return palette[total%len(palette)]
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}
