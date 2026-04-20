package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Stores struct {
	DB                   *gorm.DB
	SQL                  *sql.DB
	Tenants              TenantRepository
	Users                UserRepository
	Instances            InstanceRepository
	ConversationMessages ConversationMessageRepository
	RuntimeObservability RuntimeObservabilityRepository
	CRM                  CRMRepository
	Broadcasts           BroadcastRepository
	Webhooks             WebhookRepository
	AI                   AIRepository
}

func NewStores(databaseURL string, maxOpenConns, maxIdleConns int, connMaxLifetime time.Duration) (*Stores, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err := repairConversationMessageSchema(db); err != nil {
		return nil, fmt.Errorf("repair conversation message schema: %w", err)
	}

	if err := db.AutoMigrate(
		&Tenant{},
		&User{},
		&Instance{},
		&Message{},
		&ConversationMessage{},
		&RuntimeSessionState{},
		&RuntimeSessionEvent{},
		&Contact{},
		&Tag{},
		&Note{},
		&Pipeline{},
		&DealStage{},
		&Deal{},
		&BroadcastJob{},
		&BroadcastRecipientProgress{},
		&WebhookEndpoint{},
		&WebhookDelivery{},
		&AISettings{},
		&AIConversationMessage{},
	); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return &Stores{
		DB:                   db,
		SQL:                  sqlDB,
		Tenants:              &gormTenantRepository{db: db},
		Users:                &gormUserRepository{db: db},
		Instances:            &gormInstanceRepository{db: db},
		ConversationMessages: &gormConversationMessageRepository{db: db},
		RuntimeObservability: &gormRuntimeObservabilityRepository{db: db},
		CRM:                  &gormCRMRepository{db: db},
		Broadcasts:           &gormBroadcastRepository{db: db},
		Webhooks:             &gormWebhookRepository{db: db},
		AI:                   &gormAIRepository{db: db},
	}, nil
}

func repairConversationMessageSchema(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	return db.Exec(`
DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'conversation_messages'
		  AND column_name = 'remote_j_id'
	) AND NOT EXISTS (
		SELECT 1
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'conversation_messages'
		  AND column_name = 'remote_jid'
	) THEN
		ALTER TABLE conversation_messages RENAME COLUMN remote_j_id TO remote_jid;
	END IF;
END $$;
`).Error
}

func Close(ctx context.Context, stores *Stores) error {
	if stores == nil || stores.SQL == nil {
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- stores.SQL.Close()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

type gormTenantRepository struct{ db *gorm.DB }

func (r *gormTenantRepository) Create(ctx context.Context, tenant *Tenant) error {
	return r.db.WithContext(ctx).Create(tenant).Error
}

func (r *gormTenantRepository) GetByID(ctx context.Context, tenantID string) (*Tenant, error) {
	var tenant Tenant
	err := r.db.WithContext(ctx).First(&tenant, "id = ?", tenantID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &tenant, nil
}

func (r *gormTenantRepository) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var tenant Tenant
	err := r.db.WithContext(ctx).First(&tenant, "slug = ?", slug).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &tenant, nil
}

func (r *gormTenantRepository) GetByAPIKeyPrefix(ctx context.Context, prefix string) (*Tenant, error) {
	var tenant Tenant
	err := r.db.WithContext(ctx).First(&tenant, "api_key_prefix = ?", prefix).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &tenant, nil
}

type gormUserRepository struct{ db *gorm.DB }

func (r *gormUserRepository) Create(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *gormUserRepository) GetByEmail(ctx context.Context, tenantID, email string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, "tenant_id = ? AND email = ?", tenantID, email).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &user, nil
}

func (r *gormUserRepository) GetByID(ctx context.Context, tenantID, userID string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, "tenant_id = ? AND id = ?", tenantID, userID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &user, nil
}

type gormInstanceRepository struct{ db *gorm.DB }

func (r *gormInstanceRepository) Create(ctx context.Context, instance *Instance) error {
	return r.db.WithContext(ctx).Create(instance).Error
}

func (r *gormInstanceRepository) ListByTenant(ctx context.Context, tenantID string) ([]Instance, error) {
	var instances []Instance
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&instances, "tenant_id = ?", tenantID).Error
	return instances, err
}

func (r *gormInstanceRepository) GetByID(ctx context.Context, tenantID, instanceID string) (*Instance, error) {
	var instance Instance
	err := r.db.WithContext(ctx).First(&instance, "tenant_id = ? AND id = ?", tenantID, instanceID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &instance, nil
}

func (r *gormInstanceRepository) GetByGlobalID(ctx context.Context, instanceID string) (*Instance, error) {
	var instance Instance
	err := r.db.WithContext(ctx).First(&instance, "id = ?", instanceID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &instance, nil
}

func (r *gormInstanceRepository) FindByEngineInstanceID(ctx context.Context, engineInstanceID string) (*Instance, error) {
	var instance Instance
	err := r.db.WithContext(ctx).First(&instance, "engine_instance_id = ?", engineInstanceID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &instance, nil
}

func (r *gormInstanceRepository) FindByName(ctx context.Context, name string) (*Instance, error) {
	var instance Instance
	err := r.db.WithContext(ctx).First(&instance, "lower(name) = lower(?)", name).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &instance, nil
}

func (r *gormInstanceRepository) Update(ctx context.Context, instance *Instance) error {
	return r.db.WithContext(ctx).Save(instance).Error
}

func (r *gormInstanceRepository) Delete(ctx context.Context, tenantID, instanceID string) error {
	result := r.db.WithContext(ctx).Delete(&Instance{}, "tenant_id = ? AND id = ?", tenantID, instanceID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

type gormCRMRepository struct{ db *gorm.DB }

type gormConversationMessageRepository struct{ db *gorm.DB }

type gormRuntimeObservabilityRepository struct{ db *gorm.DB }

func (r *gormConversationMessageRepository) Upsert(ctx context.Context, message *ConversationMessage) error {
	if message == nil {
		return nil
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "instance_id"},
			{Name: "external_message_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id",
			"remote_jid",
			"direction",
			"message_type",
			"push_name",
			"source",
			"body",
			"status",
			"message_timestamp",
			"media_url",
			"mime_type",
			"file_name",
			"caption",
			"message_payload",
			"delivered_at",
			"read_at",
			"updated_at",
		}),
	}).Create(message).Error
}

func (r *gormConversationMessageRepository) List(ctx context.Context, tenantID, instanceID string, filter ConversationMessageFilter) ([]ConversationMessage, error) {
	var messages []ConversationMessage

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND instance_id = ?", tenantID, instanceID).
		Order("message_timestamp DESC, created_at DESC")

	if strings.TrimSpace(filter.RemoteJID) != "" {
		query = query.Where("remote_jid = ?", strings.TrimSpace(filter.RemoteJID))
	}
	if strings.TrimSpace(filter.ExternalMessageID) != "" {
		query = query.Where("external_message_id = ?", strings.TrimSpace(filter.ExternalMessageID))
	}
	if strings.TrimSpace(filter.Query) != "" {
		query = query.Where("body ILIKE ?", "%"+strings.TrimSpace(filter.Query)+"%")
	}
	if filter.Before != nil && !filter.Before.IsZero() {
		query = query.Where("message_timestamp < ?", filter.Before.UTC())
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (r *gormConversationMessageRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&ConversationMessage{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error
	return total, err
}

func (r *gormConversationMessageRepository) MarkReceipt(ctx context.Context, instanceID, externalMessageID, state string, at time.Time) error {
	instanceID = strings.TrimSpace(instanceID)
	externalMessageID = strings.TrimSpace(externalMessageID)
	state = strings.ToLower(strings.TrimSpace(state))
	if instanceID == "" || externalMessageID == "" {
		return nil
	}

	updates := map[string]any{
		"updated_at": time.Now().UTC(),
	}

	switch state {
	case "delivered":
		timestamp := at.UTC()
		updates["status"] = "delivered"
		updates["delivered_at"] = &timestamp
	case "read":
		timestamp := at.UTC()
		updates["status"] = "read"
		updates["delivered_at"] = &timestamp
		updates["read_at"] = &timestamp
	default:
		return nil
	}

	return r.db.WithContext(ctx).
		Model(&ConversationMessage{}).
		Where("instance_id = ? AND external_message_id = ?", instanceID, externalMessageID).
		Updates(updates).Error
}

func (r *gormRuntimeObservabilityRepository) UpsertState(ctx context.Context, state *RuntimeSessionState) error {
	if state == nil {
		return nil
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "instance_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"status",
			"last_seen_status",
			"last_event_type",
			"last_event_source",
			"connected",
			"logged_in",
			"pairing_active",
			"disconnect_reason",
			"last_error",
			"last_event_at",
			"last_seen_at",
			"last_connected_at",
			"last_disconnected_at",
			"last_paired_at",
			"last_logout_at",
			"updated_at",
		}),
	}).Create(state).Error
}

func (r *gormRuntimeObservabilityRepository) GetState(ctx context.Context, tenantID, instanceID string) (*RuntimeSessionState, error) {
	var state RuntimeSessionState
	err := r.db.WithContext(ctx).First(&state, "tenant_id = ? AND instance_id = ?", tenantID, instanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *gormRuntimeObservabilityRepository) AppendEvent(ctx context.Context, event *RuntimeSessionEvent) error {
	if event == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *gormRuntimeObservabilityRepository) ListEvents(ctx context.Context, tenantID, instanceID string, filter RuntimeSessionEventFilter) ([]RuntimeSessionEvent, error) {
	var events []RuntimeSessionEvent
	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND instance_id = ?", tenantID, instanceID).
		Order("occurred_at DESC, created_at DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func (r *gormRuntimeObservabilityRepository) ListStatesByTenant(ctx context.Context, tenantID string) ([]RuntimeSessionState, error) {
	var states []RuntimeSessionState
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("updated_at DESC").
		Find(&states).Error
	return states, err
}

func (r *gormCRMRepository) CreateContact(ctx context.Context, contact *Contact) error {
	return r.db.WithContext(ctx).Create(contact).Error
}

func (r *gormCRMRepository) GetContact(ctx context.Context, tenantID, contactID string) (*Contact, error) {
	var contact Contact
	err := r.db.WithContext(ctx).
		Preload("Tags").
		Preload("Notes").
		First(&contact, "tenant_id = ? AND id = ?", tenantID, contactID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &contact, nil
}

func (r *gormCRMRepository) ListContacts(ctx context.Context, tenantID string) ([]Contact, error) {
	var contacts []Contact
	err := r.db.WithContext(ctx).
		Preload("Tags").
		Preload("Notes").
		Order("created_at DESC").
		Find(&contacts, "tenant_id = ?", tenantID).Error
	return contacts, err
}

func (r *gormCRMRepository) CountContactsByTenant(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&Contact{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error
	return total, err
}

func (r *gormCRMRepository) UpdateContact(ctx context.Context, contact *Contact) error {
	return r.db.WithContext(ctx).Session(&gorm.Session{FullSaveAssociations: true}).Save(contact).Error
}

func (r *gormCRMRepository) FindContactByPhone(ctx context.Context, tenantID, phone string) (*Contact, error) {
	var contact Contact
	err := r.db.WithContext(ctx).First(&contact, "tenant_id = ? AND phone = ?", tenantID, phone).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &contact, nil
}

func (r *gormCRMRepository) CreateTag(ctx context.Context, tag *Tag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *gormCRMRepository) FindTagByName(ctx context.Context, tenantID, name string) (*Tag, error) {
	var tag Tag
	err := r.db.WithContext(ctx).First(&tag, "tenant_id = ? AND lower(name) = lower(?)", tenantID, name).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &tag, nil
}

func (r *gormCRMRepository) AssignTags(ctx context.Context, tenantID, contactID string, tagIDs []string) error {
	var contact Contact
	if err := r.db.WithContext(ctx).First(&contact, "tenant_id = ? AND id = ?", tenantID, contactID).Error; err != nil {
		return normalizeError(err)
	}

	var tags []Tag
	if err := r.db.WithContext(ctx).Find(&tags, "tenant_id = ? AND id IN ?", tenantID, tagIDs).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(&contact).Association("Tags").Replace(tags)
}

func (r *gormCRMRepository) CreateNote(ctx context.Context, note *Note) error {
	return r.db.WithContext(ctx).Create(note).Error
}

type gormBroadcastRepository struct{ db *gorm.DB }

func (r *gormBroadcastRepository) Create(ctx context.Context, job *BroadcastJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *gormBroadcastRepository) GetByID(ctx context.Context, tenantID, jobID string) (*BroadcastJob, error) {
	var job BroadcastJob
	err := r.db.WithContext(ctx).First(&job, "tenant_id = ? AND id = ?", tenantID, jobID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &job, nil
}

func (r *gormBroadcastRepository) ListByTenant(ctx context.Context, tenantID string, limit int) ([]BroadcastJob, error) {
	var jobs []BroadcastJob
	query := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&jobs).Error
	return jobs, err
}

func (r *gormBroadcastRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&BroadcastJob{}).
		Where("tenant_id = ?", tenantID).
		Count(&total).Error
	return total, err
}

func (r *gormBroadcastRepository) SeedRecipientProgress(ctx context.Context, records []BroadcastRecipientProgress) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "broadcast_id"}, {Name: "phone"}},
		DoNothing: true,
	}).Create(&records).Error
}

func (r *gormBroadcastRepository) SaveRecipientProgress(ctx context.Context, progress *BroadcastRecipientProgress) error {
	if progress == nil {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "broadcast_id"},
			{Name: "phone"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id",
			"instance_id",
			"contact_id",
			"delivery_status",
			"attempt_count",
			"last_error",
			"last_attempt_at",
			"sent_at",
			"delivered_at",
			"read_at",
			"failed_at",
			"last_status_at",
			"status_source",
			"message_id",
			"server_id",
			"chat_jid",
			"updated_at",
		}),
	}).Create(progress).Error
}

func (r *gormBroadcastRepository) ListRecipientProgress(ctx context.Context, tenantID, jobID string) ([]BroadcastRecipientProgress, error) {
	var progress []BroadcastRecipientProgress
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND broadcast_id = ?", tenantID, jobID).
		Order("created_at ASC, phone ASC").
		Find(&progress).Error
	return progress, err
}

func (r *gormBroadcastRepository) ListRecipientProgressPage(ctx context.Context, tenantID, jobID string, filter BroadcastRecipientProgressFilter) ([]BroadcastRecipientProgress, int64, error) {
	var (
		progress []BroadcastRecipientProgress
		total    int64
	)

	query := r.db.WithContext(ctx).
		Model(&BroadcastRecipientProgress{}).
		Where("tenant_id = ? AND broadcast_id = ?", tenantID, jobID)

	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where("delivery_status = ?", strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.Query) != "" {
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		query = query.Where("phone ILIKE ? OR CAST(contact_id AS text) ILIKE ?", like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	err := query.
		Order("created_at ASC, phone ASC").
		Limit(limit).
		Offset((page - 1) * limit).
		Find(&progress).Error
	if err != nil {
		return nil, 0, err
	}

	return progress, total, nil
}

func (r *gormBroadcastRepository) MarkRecipientReceipt(ctx context.Context, tenantID, instanceID, messageID, state string, at time.Time) (bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	instanceID = strings.TrimSpace(instanceID)
	messageID = strings.TrimSpace(messageID)
	state = strings.ToLower(strings.TrimSpace(state))
	if tenantID == "" || instanceID == "" || messageID == "" {
		return false, nil
	}

	timestamp := at.UTC()
	updates := map[string]any{
		"updated_at":     time.Now().UTC(),
		"last_status_at": &timestamp,
	}
	query := r.db.WithContext(ctx).
		Model(&BroadcastRecipientProgress{}).
		Where("tenant_id = ? AND instance_id = ? AND message_id = ? AND delivery_status <> ?", tenantID, instanceID, messageID, "failed")

	switch state {
	case "delivered":
		updates["delivery_status"] = "delivered"
		updates["delivered_at"] = gorm.Expr("COALESCE(delivered_at, ?)", timestamp)
		updates["status_source"] = "receipt_delivered"
		query = query.Where("delivery_status <> ?", "read")
	case "read":
		updates["delivery_status"] = "read"
		updates["delivered_at"] = gorm.Expr("COALESCE(delivered_at, ?)", timestamp)
		updates["read_at"] = gorm.Expr("CASE WHEN read_at IS NULL OR read_at < ? THEN ? ELSE read_at END", timestamp, timestamp)
		updates["status_source"] = "receipt_read"
	default:
		return false, nil
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *gormBroadcastRepository) SummarizeRecipientProgress(ctx context.Context, tenantID, jobID string) (BroadcastRecipientAnalytics, error) {
	var summary BroadcastRecipientAnalytics
	err := r.db.WithContext(ctx).
		Model(&BroadcastRecipientProgress{}).
		Select(`
			COUNT(*) AS total_recipients,
			COALESCE(SUM(CASE WHEN attempt_count > 0 THEN 1 ELSE 0 END), 0) AS attempted,
			COALESCE(SUM(CASE WHEN delivery_status IN ('sent', 'delivered', 'read') THEN 1 ELSE 0 END), 0) AS sent,
			COALESCE(SUM(CASE WHEN delivered_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS delivered,
			COALESCE(SUM(CASE WHEN read_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS read,
			COALESCE(SUM(CASE WHEN delivery_status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
			COALESCE(SUM(CASE WHEN delivery_status = 'pending' THEN 1 ELSE 0 END), 0) AS pending
		`).
		Where("tenant_id = ? AND broadcast_id = ?", tenantID, jobID).
		Scan(&summary).Error
	return summary, err
}

func (r *gormBroadcastRepository) SummarizeRecipientProgressByTenant(ctx context.Context, tenantID string) (BroadcastRecipientAnalytics, error) {
	var summary BroadcastRecipientAnalytics
	err := r.db.WithContext(ctx).
		Model(&BroadcastRecipientProgress{}).
		Select(`
			COUNT(DISTINCT broadcast_id) AS tracked_broadcasts,
			COUNT(*) AS total_recipients,
			COALESCE(SUM(CASE WHEN attempt_count > 0 THEN 1 ELSE 0 END), 0) AS attempted,
			COALESCE(SUM(CASE WHEN delivery_status IN ('sent', 'delivered', 'read') THEN 1 ELSE 0 END), 0) AS sent,
			COALESCE(SUM(CASE WHEN delivered_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS delivered,
			COALESCE(SUM(CASE WHEN read_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS read,
			COALESCE(SUM(CASE WHEN delivery_status = 'failed' THEN 1 ELSE 0 END), 0) AS failed,
			COALESCE(SUM(CASE WHEN delivery_status = 'pending' THEN 1 ELSE 0 END), 0) AS pending
		`).
		Where("tenant_id = ?", tenantID).
		Scan(&summary).Error
	return summary, err
}

func (r *gormBroadcastRepository) ClaimNext(ctx context.Context, workerID string, limit int, now time.Time) ([]BroadcastJob, error) {
	jobs := make([]BroadcastJob, 0, limit)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ?", "queued", now).
			Order("available_at ASC, created_at ASC")
		if limit > 0 {
			query = query.Limit(limit)
		}

		if err := query.Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}

		jobIDs := make([]string, 0, len(jobs))
		for i := range jobs {
			jobIDs = append(jobIDs, jobs[i].ID)
		}

		return tx.Model(&BroadcastJob{}).
			Where("id IN ?", jobIDs).
			Updates(map[string]any{
				"status":     "processing",
				"worker_id":  workerID,
				"started_at": now,
				"attempts":   gorm.Expr("attempts + 1"),
			}).Error
	})
	if err != nil {
		return nil, err
	}

	for i := range jobs {
		jobs[i].Status = "processing"
		jobs[i].WorkerID = workerID
		jobs[i].StartedAt = &now
		jobs[i].Attempts++
	}

	return jobs, nil
}

func (r *gormBroadcastRepository) MarkCompleted(ctx context.Context, tenantID, jobID string, completedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&BroadcastJob{}).
		Where("tenant_id = ? AND id = ?", tenantID, jobID).
		Updates(map[string]any{
			"status":       "completed",
			"completed_at": completedAt,
			"last_error":   "",
			"worker_id":    "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *gormBroadcastRepository) MarkCompletedWithFailures(ctx context.Context, tenantID, jobID, message string, completedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&BroadcastJob{}).
		Where("tenant_id = ? AND id = ?", tenantID, jobID).
		Updates(map[string]any{
			"status":       "completed_with_failures",
			"completed_at": completedAt,
			"last_error":   strings.TrimSpace(message),
			"worker_id":    "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *gormBroadcastRepository) MarkFailed(ctx context.Context, tenantID, jobID, message string, failedAt time.Time, retryAt *time.Time) error {
	updates := map[string]any{
		"last_error": message,
		"worker_id":  "",
		"started_at": nil,
	}
	if retryAt != nil {
		updates["status"] = "queued"
		updates["available_at"] = *retryAt
	} else {
		updates["status"] = "failed"
		updates["failed_at"] = failedAt
	}

	result := r.db.WithContext(ctx).
		Model(&BroadcastJob{}).
		Where("tenant_id = ? AND id = ?", tenantID, jobID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

type gormWebhookRepository struct{ db *gorm.DB }

func (r *gormWebhookRepository) Create(ctx context.Context, endpoint *WebhookEndpoint) error {
	return r.db.WithContext(ctx).Create(endpoint).Error
}

func (r *gormWebhookRepository) GetByID(ctx context.Context, tenantID, endpointID string) (*WebhookEndpoint, error) {
	var endpoint WebhookEndpoint
	err := r.db.WithContext(ctx).First(&endpoint, "tenant_id = ? AND id = ?", tenantID, endpointID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &endpoint, nil
}

func (r *gormWebhookRepository) ListByTenant(ctx context.Context, tenantID string) ([]WebhookEndpoint, error) {
	var endpoints []WebhookEndpoint
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&endpoints, "tenant_id = ?", tenantID).Error
	return endpoints, err
}

type gormAIRepository struct{ db *gorm.DB }

func (r *gormAIRepository) Upsert(ctx context.Context, settings *AISettings) error {
	var existing AISettings
	err := r.db.WithContext(ctx).First(&existing, "tenant_id = ?", settings.TenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(settings).Error
	}
	if err != nil {
		return err
	}

	settings.ID = existing.ID
	return r.db.WithContext(ctx).Save(settings).Error
}

func (r *gormAIRepository) GetByTenant(ctx context.Context, tenantID string) (*AISettings, error) {
	var settings AISettings
	err := r.db.WithContext(ctx).First(&settings, "tenant_id = ?", tenantID).Error
	if err != nil {
		return nil, normalizeError(err)
	}
	return &settings, nil
}

func (r *gormAIRepository) AppendConversationMessage(ctx context.Context, message *AIConversationMessage) error {
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *gormAIRepository) ListConversationMessages(ctx context.Context, tenantID, instanceID, conversationKey string, limit int) ([]AIConversationMessage, error) {
	var messages []AIConversationMessage
	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND instance_id = ? AND conversation_key = ?", tenantID, instanceID, conversationKey).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}
	// reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func normalizeError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("record not found: %w", err)
	}
	return err
}
