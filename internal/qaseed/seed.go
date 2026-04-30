package qaseed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultContactCount         = 125
	defaultMessagesPerChat      = 80
	defaultConversationChatSize = 3
)

type Options struct {
	TenantID      string
	TenantSlug    string
	CreateTenant  bool
	AdminEmail    string
	AdminPassword string
}

type Summary struct {
	TenantID        string
	TenantSlug      string
	InstanceID      string
	EmptyInstanceID string
	Contacts        int
	BroadcastJobs   int
	Recipients      int
	Messages        int
	RuntimeEvents   int
}

func Run(ctx context.Context, cfg *config.Config, stores *repository.Stores, logger *slog.Logger, opts Options) (Summary, error) {
	if err := guardEnabled(cfg); err != nil {
		return Summary{}, err
	}
	if stores == nil || stores.DB == nil {
		return Summary{}, fmt.Errorf("qa seed stores are unavailable")
	}

	opts = normalizeOptions(opts)
	tenant, err := resolveTenant(ctx, stores, opts)
	if err != nil {
		return Summary{}, err
	}

	mainInstance, emptyInstance, err := seedInstances(ctx, stores.DB, tenant.ID)
	if err != nil {
		return Summary{}, err
	}
	contacts, err := seedContacts(ctx, stores.DB, tenant.ID, mainInstance.ID)
	if err != nil {
		return Summary{}, err
	}
	jobs, recipients, err := seedBroadcasts(ctx, stores.DB, tenant.ID, mainInstance.ID, contacts)
	if err != nil {
		return Summary{}, err
	}
	messages, err := seedConversationMessages(ctx, stores, tenant.ID, mainInstance.ID)
	if err != nil {
		return Summary{}, err
	}
	events, err := seedRuntime(ctx, stores.DB, tenant.ID, mainInstance.ID)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		TenantID:        tenant.ID,
		TenantSlug:      tenant.Slug,
		InstanceID:      mainInstance.ID,
		EmptyInstanceID: emptyInstance.ID,
		Contacts:        len(contacts),
		BroadcastJobs:   len(jobs),
		Recipients:      recipients,
		Messages:        messages,
		RuntimeEvents:   events,
	}
	if logger != nil {
		logger.Info(
			"qa seed completed",
			"tenant_id", summary.TenantID,
			"tenant_slug", summary.TenantSlug,
			"instance_id", summary.InstanceID,
			"empty_instance_id", summary.EmptyInstanceID,
			"contacts", summary.Contacts,
			"broadcast_jobs", summary.BroadcastJobs,
			"broadcast_recipients", summary.Recipients,
			"conversation_messages", summary.Messages,
			"runtime_events", summary.RuntimeEvents,
		)
	}
	return summary, nil
}

func guardEnabled(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("qa seed config is unavailable")
	}
	env := strings.ToLower(strings.TrimSpace(cfg.AppEnv))
	if env == "production" || env == "prod" {
		return fmt.Errorf("qa seed refused: APP_ENV=%s is not allowed", cfg.AppEnv)
	}
	if !truthyEnv("QA_SEED_ENABLED") {
		return fmt.Errorf("qa seed disabled: set QA_SEED_ENABLED=true and run cmd/qa-seed explicitly")
	}
	return nil
}

func normalizeOptions(opts Options) Options {
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.TenantSlug = strings.TrimSpace(opts.TenantSlug)
	if opts.TenantSlug == "" {
		opts.TenantSlug = "qa-seed"
	}
	opts.AdminEmail = strings.TrimSpace(opts.AdminEmail)
	if opts.AdminEmail == "" {
		opts.AdminEmail = "qa.admin@example.test"
	}
	opts.AdminPassword = strings.TrimSpace(opts.AdminPassword)
	if opts.AdminPassword == "" {
		opts.AdminPassword = "QaSeed123!"
	}
	return opts
}

func resolveTenant(ctx context.Context, stores *repository.Stores, opts Options) (*repository.Tenant, error) {
	if opts.TenantID != "" {
		tenant, err := stores.Tenants.GetByID(ctx, opts.TenantID)
		if err != nil {
			return nil, fmt.Errorf("load qa seed tenant by id: %w", err)
		}
		return tenant, nil
	}

	tenant, err := stores.Tenants.GetBySlug(ctx, opts.TenantSlug)
	if err == nil {
		if opts.CreateTenant {
			if err := ensureAdminUser(ctx, stores, tenant.ID, opts.AdminEmail, opts.AdminPassword); err != nil {
				return nil, err
			}
		}
		return tenant, nil
	}
	if !opts.CreateTenant {
		return nil, fmt.Errorf("qa seed tenant %q not found; pass -create-tenant=true or an existing -tenant-id", opts.TenantSlug)
	}

	apiKey, err := repository.GenerateTenantAPIKey()
	if err != nil {
		return nil, err
	}
	hash, err := repository.HashAPIKey(apiKey)
	if err != nil {
		return nil, err
	}
	tenant = &repository.Tenant{
		ID:           stableID("tenant", opts.TenantSlug),
		Name:         "QA Seed Tenant",
		Slug:         opts.TenantSlug,
		APIKeyPrefix: repository.APIKeyPrefix(apiKey),
		APIKeyHash:   hash,
	}
	if err := stores.Tenants.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("create qa seed tenant: %w", err)
	}

	if err := ensureAdminUser(ctx, stores, tenant.ID, opts.AdminEmail, opts.AdminPassword); err != nil {
		return nil, err
	}
	return tenant, nil
}

func ensureAdminUser(ctx context.Context, stores *repository.Stores, tenantID, email, password string) error {
	if _, err := stores.Users.GetByEmail(ctx, tenantID, email); err == nil {
		return nil
	}

	passwordHash, err := repository.HashPassword(password)
	if err != nil {
		return err
	}
	user := &repository.User{
		ID:           stableID("user", tenantID, email),
		TenantID:     tenantID,
		Email:        email,
		PasswordHash: passwordHash,
		Name:         "QA Admin",
		Role:         "owner",
	}
	if err := stores.Users.Create(ctx, user); err != nil {
		return fmt.Errorf("create qa seed admin user: %w", err)
	}
	return nil
}

func seedInstances(ctx context.Context, db *gorm.DB, tenantID string) (repository.Instance, repository.Instance, error) {
	now := time.Now().UTC()
	main := repository.Instance{
		ID:               stableID("instance", tenantID, "qa-main"),
		TenantID:         tenantID,
		Name:             "qa-main",
		Status:           "open",
		EngineInstanceID: stableID("engine", tenantID, "qa-main"),
		WebhookURL:       "https://example.test/webhooks/qa-main",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	empty := repository.Instance{
		ID:               stableID("instance", tenantID, "qa-empty"),
		TenantID:         tenantID,
		Name:             "qa-empty",
		Status:           "created",
		EngineInstanceID: stableID("engine", tenantID, "qa-empty"),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	instances := []repository.Instance{main, empty}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id", "name", "status", "engine_instance_id", "webhook_url", "updated_at",
		}),
	}).Create(&instances).Error; err != nil {
		return repository.Instance{}, repository.Instance{}, err
	}
	return main, empty, nil
}

func seedContacts(ctx context.Context, db *gorm.DB, tenantID, instanceID string) ([]repository.Contact, error) {
	now := time.Now().UTC()
	contacts := make([]repository.Contact, 0, defaultContactCount)
	for i := 1; i <= defaultContactCount; i++ {
		phone := fmt.Sprintf("521555%07d", i)
		contact := repository.Contact{
			ID:         stableID("contact", tenantID, phone),
			TenantID:   tenantID,
			Phone:      phone,
			Name:       fmt.Sprintf("QA Contact %03d", i),
			Email:      fmt.Sprintf("qa.contact.%03d@example.test", i),
			InstanceID: instanceID,
			CreatedAt:  now.Add(-time.Duration(i) * time.Hour),
			UpdatedAt:  now,
		}
		contacts = append(contacts, contact)
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "phone"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "email", "instance_id", "updated_at",
		}),
	}).Create(&contacts).Error; err != nil {
		return nil, err
	}
	return contacts, nil
}

func seedBroadcasts(ctx context.Context, db *gorm.DB, tenantID, instanceID string, contacts []repository.Contact) ([]repository.BroadcastJob, int, error) {
	now := time.Now().UTC()
	statuses := []string{"queued", "running", "completed", "completed_with_failures", "failed", "completed"}
	jobs := make([]repository.BroadcastJob, 0, len(statuses))
	for i, status := range statuses {
		startedAt := now.Add(-time.Duration(i+2) * time.Hour)
		job := repository.BroadcastJob{
			ID:          stableID("broadcast", tenantID, fmt.Sprintf("qa-%d", i+1)),
			TenantID:    tenantID,
			InstanceID:  instanceID,
			Status:      status,
			Message:     fmt.Sprintf("QA broadcast %d: realistic test message for dense data views.", i+1),
			RatePerHour: 120,
			DelaySec:    0,
			Attempts:    1,
			MaxAttempts: 3,
			AvailableAt: startedAt,
			CreatedAt:   startedAt,
			UpdatedAt:   now,
		}
		if status != "queued" {
			job.StartedAt = &startedAt
		}
		if status == "completed" || status == "completed_with_failures" {
			completedAt := startedAt.Add(20 * time.Minute)
			job.CompletedAt = &completedAt
		}
		if status == "failed" {
			failedAt := startedAt.Add(12 * time.Minute)
			job.FailedAt = &failedAt
			job.LastError = "QA fixture permanent campaign failure"
		}
		jobs = append(jobs, job)
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id", "instance_id", "status", "message", "rate_per_hour", "delay_sec", "attempts", "max_attempts", "worker_id", "last_error", "available_at", "started_at", "completed_at", "failed_at", "updated_at",
		}),
	}).Create(&jobs).Error; err != nil {
		return nil, 0, err
	}

	recipients := make([]repository.BroadcastRecipientProgress, 0, 150)
	recipientStates := []string{"pending", "sent", "delivered", "read", "failed"}
	for jobIndex, job := range jobs[:5] {
		for i := 0; i < 30 && i < len(contacts); i++ {
			contact := contacts[(jobIndex*17+i)%len(contacts)]
			state := recipientStates[(i+jobIndex)%len(recipientStates)]
			at := now.Add(-time.Duration(jobIndex*40+i) * time.Minute)
			progress := repository.BroadcastRecipientProgress{
				ID:             stableID("broadcast-recipient", job.ID, contact.Phone),
				BroadcastID:    job.ID,
				TenantID:       tenantID,
				InstanceID:     instanceID,
				ContactID:      &contact.ID,
				Phone:          contact.Phone,
				DeliveryStatus: state,
				AttemptCount:   1,
				LastAttemptAt:  &at,
				LastStatusAt:   &at,
				StatusSource:   "qa_seed",
				MessageID:      fmt.Sprintf("qa-%s-%s", shortID(job.ID), contact.Phone),
				ServerID:       int64(jobIndex*1000 + i + 1),
				ChatJID:        contact.Phone + "@s.whatsapp.net",
				CreatedAt:      at,
				UpdatedAt:      now,
			}
			switch state {
			case "sent":
				progress.SentAt = &at
			case "delivered":
				progress.SentAt = &at
				deliveredAt := at.Add(2 * time.Minute)
				progress.DeliveredAt = &deliveredAt
				progress.LastStatusAt = &deliveredAt
			case "read":
				progress.SentAt = &at
				deliveredAt := at.Add(2 * time.Minute)
				readAt := at.Add(5 * time.Minute)
				progress.DeliveredAt = &deliveredAt
				progress.ReadAt = &readAt
				progress.LastStatusAt = &readAt
			case "failed":
				progress.FailedAt = &at
				progress.LastError = "QA fixture recipient failure"
			}
			recipients = append(recipients, progress)
		}
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "broadcast_id"}, {Name: "phone"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id", "instance_id", "contact_id", "delivery_status", "attempt_count", "last_error", "last_attempt_at", "sent_at", "delivered_at", "read_at", "failed_at", "last_status_at", "status_source", "message_id", "server_id", "chat_jid", "updated_at",
		}),
	}).Create(&recipients).Error; err != nil {
		return nil, 0, err
	}
	return jobs, len(recipients), nil
}

func seedConversationMessages(ctx context.Context, stores *repository.Stores, tenantID, instanceID string) (int, error) {
	total := 0
	now := time.Now().UTC()
	for chat := 1; chat <= defaultConversationChatSize; chat++ {
		remoteJID := fmt.Sprintf("521555%07d@s.whatsapp.net", chat)
		for i := 1; i <= defaultMessagesPerChat; i++ {
			direction := "inbound"
			status := "delivered"
			if i%2 == 0 {
				direction = "outbound"
				status = "sent"
			}
			deliveredAt, readAt := (*time.Time)(nil), (*time.Time)(nil)
			if i%5 == 0 {
				delivered := now.Add(-time.Duration(defaultMessagesPerChat-i) * time.Minute)
				deliveredAt = &delivered
				status = "delivered"
			}
			if i%9 == 0 {
				read := now.Add(-time.Duration(defaultMessagesPerChat-i-1) * time.Minute)
				readAt = &read
				status = "read"
			}
			messageType := "conversation"
			body := fmt.Sprintf("QA chat %d message %03d with searchable dense-history text", chat, i)
			payload := map[string]any{"conversation": body}
			if i%17 == 0 {
				messageType = "imageMessage"
				payload = map[string]any{"imageMessage": map[string]any{"caption": body, "mimetype": "image/jpeg"}, "mediaUrl": "https://example.test/qa-image.jpg"}
			}
			rawPayload, _ := json.Marshal(payload)
			message := &repository.ConversationMessage{
				ID:                stableID("conversation", tenantID, instanceID, remoteJID, fmt.Sprintf("%03d", i)),
				TenantID:          tenantID,
				InstanceID:        instanceID,
				RemoteJID:         remoteJID,
				ExternalMessageID: fmt.Sprintf("qa-msg-%d-%03d", chat, i),
				Direction:         direction,
				MessageType:       messageType,
				PushName:          fmt.Sprintf("QA Chat %d", chat),
				Source:            "qa_seed",
				Body:              body,
				Status:            status,
				MessageTimestamp:  now.Add(-time.Duration(defaultMessagesPerChat*chat-i) * time.Minute),
				MediaURL:          stringValue(i%17 == 0, "https://example.test/qa-image.jpg"),
				MimeType:          stringValue(i%17 == 0, "image/jpeg"),
				Caption:           stringValue(i%17 == 0, body),
				MessagePayload:    string(rawPayload),
				DeliveredAt:       deliveredAt,
				ReadAt:            readAt,
			}
			if err := stores.ConversationMessages.Upsert(ctx, message); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}

func seedRuntime(ctx context.Context, db *gorm.DB, tenantID, instanceID string) (int, error) {
	now := time.Now().UTC()
	lastConnected := now.Add(-2 * time.Hour)
	state := repository.RuntimeSessionState{
		ID:              stableID("runtime-state", tenantID, instanceID),
		TenantID:        tenantID,
		InstanceID:      instanceID,
		Status:          "open",
		LastSeenStatus:  "open",
		LastEventType:   "status_observed",
		LastEventSource: "qa_seed",
		Connected:       true,
		LoggedIn:        true,
		PairingActive:   false,
		LastEventAt:     &now,
		LastSeenAt:      &now,
		LastConnectedAt: &lastConnected,
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "instance_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "last_seen_status", "last_event_type", "last_event_source", "connected", "logged_in", "pairing_active", "disconnect_reason", "last_error", "last_event_at", "last_seen_at", "last_connected_at", "updated_at",
		}),
	}).Create(&state).Error; err != nil {
		return 0, err
	}

	types := []string{"pairing_started", "paired", "connected", "status_observed", "history_sync_requested", "history_sync", "disconnected", "connected", "status_observed"}
	events := make([]repository.RuntimeSessionEvent, 0, len(types))
	for i, eventType := range types {
		occurredAt := now.Add(-time.Duration(len(types)-i) * 15 * time.Minute)
		status := "open"
		connected := true
		loggedIn := true
		pairing := false
		message := "QA runtime event"
		if eventType == "pairing_started" {
			status, connected, loggedIn, pairing = "connecting", false, false, true
		}
		if eventType == "disconnected" {
			status, connected, loggedIn = "close", false, false
			message = "QA transient disconnect"
		}
		events = append(events, repository.RuntimeSessionEvent{
			ID:            stableID("runtime-event", tenantID, instanceID, eventType, fmt.Sprintf("%02d", i)),
			TenantID:      tenantID,
			InstanceID:    instanceID,
			EventType:     eventType,
			EventSource:   "qa_seed",
			Status:        status,
			Connected:     connected,
			LoggedIn:      loggedIn,
			PairingActive: pairing,
			Message:       message,
			Payload:       `{"source":"qa_seed"}`,
			OccurredAt:    occurredAt,
			CreatedAt:     occurredAt,
			UpdatedAt:     now,
		})
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tenant_id", "instance_id", "event_type", "event_source", "status", "connected", "logged_in", "pairing_active", "disconnect_reason", "error_message", "message", "payload", "occurred_at", "updated_at",
		}),
	}).Create(&events).Error; err != nil {
		return 0, err
	}
	return len(events), nil
}

func stableID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join(parts, ":"))).String()
}

func shortID(value string) string {
	trimmed := strings.ReplaceAll(value, "-", "")
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}

func stringValue(ok bool, value string) string {
	if !ok {
		return ""
	}
	return value
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
