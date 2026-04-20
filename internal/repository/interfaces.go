package repository

import (
	"context"
	"time"
)

type TenantRepository interface {
	Create(ctx context.Context, tenant *Tenant) error
	GetByID(ctx context.Context, tenantID string) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	GetByAPIKeyPrefix(ctx context.Context, prefix string) (*Tenant, error)
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByEmail(ctx context.Context, tenantID, email string) (*User, error)
	GetByID(ctx context.Context, tenantID, userID string) (*User, error)
}

type InstanceRepository interface {
	Create(ctx context.Context, instance *Instance) error
	ListByTenant(ctx context.Context, tenantID string) ([]Instance, error)
	GetByID(ctx context.Context, tenantID, instanceID string) (*Instance, error)
	GetByGlobalID(ctx context.Context, instanceID string) (*Instance, error)
	FindByEngineInstanceID(ctx context.Context, engineInstanceID string) (*Instance, error)
	FindByName(ctx context.Context, name string) (*Instance, error)
	Update(ctx context.Context, instance *Instance) error
	Delete(ctx context.Context, tenantID, instanceID string) error
}

type ConversationMessageFilter struct {
	RemoteJID         string
	ExternalMessageID string
	Query             string
	Limit             int
	Before            *time.Time
}

type ConversationMessageRepository interface {
	Upsert(ctx context.Context, message *ConversationMessage) error
	List(ctx context.Context, tenantID, instanceID string, filter ConversationMessageFilter) ([]ConversationMessage, error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
	MarkReceipt(ctx context.Context, instanceID, externalMessageID, state string, at time.Time) error
}

type RuntimeSessionEventFilter struct {
	Limit int
}

type RuntimeObservabilityRepository interface {
	UpsertState(ctx context.Context, state *RuntimeSessionState) error
	GetState(ctx context.Context, tenantID, instanceID string) (*RuntimeSessionState, error)
	AppendEvent(ctx context.Context, event *RuntimeSessionEvent) error
	ListEvents(ctx context.Context, tenantID, instanceID string, filter RuntimeSessionEventFilter) ([]RuntimeSessionEvent, error)
	ListStatesByTenant(ctx context.Context, tenantID string) ([]RuntimeSessionState, error)
}

type CRMRepository interface {
	CreateContact(ctx context.Context, contact *Contact) error
	GetContact(ctx context.Context, tenantID, contactID string) (*Contact, error)
	ListContacts(ctx context.Context, tenantID string) ([]Contact, error)
	CountContactsByTenant(ctx context.Context, tenantID string) (int64, error)
	UpdateContact(ctx context.Context, contact *Contact) error
	FindContactByPhone(ctx context.Context, tenantID, phone string) (*Contact, error)
	CreateTag(ctx context.Context, tag *Tag) error
	FindTagByName(ctx context.Context, tenantID, name string) (*Tag, error)
	AssignTags(ctx context.Context, tenantID, contactID string, tagIDs []string) error
	CreateNote(ctx context.Context, note *Note) error
}

type BroadcastRepository interface {
	Create(ctx context.Context, job *BroadcastJob) error
	GetByID(ctx context.Context, tenantID, jobID string) (*BroadcastJob, error)
	ListByTenant(ctx context.Context, tenantID string, limit int) ([]BroadcastJob, error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
	SeedRecipientProgress(ctx context.Context, records []BroadcastRecipientProgress) error
	SaveRecipientProgress(ctx context.Context, progress *BroadcastRecipientProgress) error
	ListRecipientProgress(ctx context.Context, tenantID, jobID string) ([]BroadcastRecipientProgress, error)
	SummarizeRecipientProgress(ctx context.Context, tenantID, jobID string) (BroadcastRecipientAnalytics, error)
	SummarizeRecipientProgressByTenant(ctx context.Context, tenantID string) (BroadcastRecipientAnalytics, error)
	ClaimNext(ctx context.Context, workerID string, limit int, now time.Time) ([]BroadcastJob, error)
	MarkCompleted(ctx context.Context, tenantID, jobID string, completedAt time.Time) error
	MarkCompletedWithFailures(ctx context.Context, tenantID, jobID, message string, completedAt time.Time) error
	MarkFailed(ctx context.Context, tenantID, jobID, message string, failedAt time.Time, retryAt *time.Time) error
}

type WebhookRepository interface {
	Create(ctx context.Context, endpoint *WebhookEndpoint) error
	GetByID(ctx context.Context, tenantID, endpointID string) (*WebhookEndpoint, error)
	ListByTenant(ctx context.Context, tenantID string) ([]WebhookEndpoint, error)
}

type AIRepository interface {
	Upsert(ctx context.Context, settings *AISettings) error
	GetByTenant(ctx context.Context, tenantID string) (*AISettings, error)
	AppendConversationMessage(ctx context.Context, message *AIConversationMessage) error
	ListConversationMessages(ctx context.Context, tenantID, instanceID, conversationKey string, limit int) ([]AIConversationMessage, error)
}
