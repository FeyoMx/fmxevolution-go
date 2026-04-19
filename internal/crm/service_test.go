package crm

import (
	"context"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type crmRepoMock struct {
	contactsByID    map[string]*repository.Contact
	contactsByPhone map[string]*repository.Contact
	tagsByName      map[string]*repository.Tag
	lastAssigned    []string
	noteCount       int
}

func newCRMRepoMock() *crmRepoMock {
	return &crmRepoMock{
		contactsByID:    make(map[string]*repository.Contact),
		contactsByPhone: make(map[string]*repository.Contact),
		tagsByName:      make(map[string]*repository.Tag),
	}
}

func (m *crmRepoMock) CreateContact(_ context.Context, contact *repository.Contact) error {
	if contact.ID == "" {
		contact.ID = "contact-" + contact.Phone
	}
	copied := *contact
	m.contactsByID[contact.ID] = &copied
	m.contactsByPhone[contact.Phone] = &copied
	return nil
}

func (m *crmRepoMock) GetContact(_ context.Context, tenantID, contactID string) (*repository.Contact, error) {
	contact := m.contactsByID[contactID]
	if contact == nil || contact.TenantID != tenantID {
		return nil, repositoryErrNotFound()
	}
	copied := *contact
	return &copied, nil
}

func (m *crmRepoMock) ListContacts(_ context.Context, tenantID string) ([]repository.Contact, error) {
	var contacts []repository.Contact
	for _, contact := range m.contactsByID {
		if contact.TenantID == tenantID {
			contacts = append(contacts, *contact)
		}
	}
	return contacts, nil
}

func (m *crmRepoMock) CountContactsByTenant(_ context.Context, tenantID string) (int64, error) {
	var total int64
	for _, contact := range m.contactsByID {
		if contact.TenantID == tenantID {
			total++
		}
	}
	return total, nil
}

func (m *crmRepoMock) UpdateContact(_ context.Context, contact *repository.Contact) error {
	copied := *contact
	m.contactsByID[contact.ID] = &copied
	m.contactsByPhone[contact.Phone] = &copied
	return nil
}

func (m *crmRepoMock) FindContactByPhone(_ context.Context, tenantID, phone string) (*repository.Contact, error) {
	contact := m.contactsByPhone[phone]
	if contact == nil || contact.TenantID != tenantID {
		return nil, repositoryErrNotFound()
	}
	copied := *contact
	return &copied, nil
}

func (m *crmRepoMock) CreateTag(_ context.Context, tag *repository.Tag) error {
	if tag.ID == "" {
		tag.ID = "tag-" + tag.Name
	}
	copied := *tag
	m.tagsByName[tag.Name] = &copied
	return nil
}

func (m *crmRepoMock) FindTagByName(_ context.Context, tenantID, name string) (*repository.Tag, error) {
	tag := m.tagsByName[name]
	if tag == nil || tag.TenantID != tenantID {
		return nil, repositoryErrNotFound()
	}
	copied := *tag
	return &copied, nil
}

func (m *crmRepoMock) AssignTags(_ context.Context, _ string, contactID string, tagIDs []string) error {
	m.lastAssigned = tagIDs
	contact := m.contactsByID[contactID]
	if contact != nil {
		contact.Tags = make([]repository.Tag, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			for _, tag := range m.tagsByName {
				if tag.ID == tagID {
					contact.Tags = append(contact.Tags, *tag)
				}
			}
		}
	}
	return nil
}

func (m *crmRepoMock) CreateNote(_ context.Context, note *repository.Note) error {
	m.noteCount++
	if note.ID == "" {
		note.ID = "note-id"
	}
	contact := m.contactsByID[note.ContactID]
	if contact != nil {
		contact.Notes = append(contact.Notes, *note)
	}
	return nil
}

func TestCreateContactNormalizesAndAssignsCRMData(t *testing.T) {
	repo := newCRMRepoMock()
	service := NewService(repo)

	contact, err := service.CreateContact(context.Background(), "tenant-1", CreateContactInput{
		Name:  "Alice",
		Phone: "+52 55 1234 5678",
		Email: "ALICE@EXAMPLE.COM",
		Tags:  []string{"VIP", "Lead"},
		Notes: []string{"First touch"},
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	if contact.Phone != "525512345678" {
		t.Fatalf("expected normalized phone, got %s", contact.Phone)
	}
	if contact.Email != "alice@example.com" {
		t.Fatalf("expected normalized email, got %s", contact.Email)
	}
	if len(contact.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(contact.Tags))
	}
	if len(contact.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(contact.Notes))
	}
}

func TestCreateContactRejectsDuplicatePhonePerTenant(t *testing.T) {
	repo := newCRMRepoMock()
	service := NewService(repo)

	_, _ = service.CreateContact(context.Background(), "tenant-1", CreateContactInput{
		Name:  "Alice",
		Phone: "5551234567",
	})

	if _, err := service.CreateContact(context.Background(), "tenant-1", CreateContactInput{
		Name:  "Bob",
		Phone: "(555) 123-4567",
	}); err == nil {
		t.Fatal("expected duplicate phone conflict")
	}
}

type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }

func repositoryErrNotFound() error {
	return notFoundError{}
}
