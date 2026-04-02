package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Tenant struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name         string    `json:"name" gorm:"size:255;not null"`
	Slug         string    `json:"slug" gorm:"size:255;uniqueIndex;not null"`
	APIKeyPrefix string    `json:"-" gorm:"size:32;uniqueIndex;not null"`
	APIKeyHash   string    `json:"-" gorm:"size:255;not null"`
	AIEnabled    bool      `json:"ai_enabled" gorm:"not null;default:false"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type User struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     string    `json:"tenant_id" gorm:"type:uuid;uniqueIndex:idx_users_tenant_email;not null"`
	Email        string    `json:"email" gorm:"size:255;uniqueIndex:idx_users_tenant_email;not null"`
	PasswordHash string    `json:"-" gorm:"size:255;not null"`
	Name         string    `json:"name" gorm:"size:255;not null"`
	Role         string    `json:"role" gorm:"size:50;not null"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Instance struct {
	ID               string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID         string    `json:"tenant_id" gorm:"type:uuid;index;not null"`
	Name             string    `json:"name" gorm:"size:255;not null"`
	Status           string    `json:"status" gorm:"size:50;not null;default:'created'"`
	EngineInstanceID string    `json:"engine_instance_id" gorm:"size:255"`
	WebhookURL       string    `json:"webhook_url" gorm:"size:500"`
	AIEnabled        bool      `json:"ai_enabled" gorm:"not null;default:false"`
	AIAutoReply      bool      `json:"ai_auto_reply" gorm:"not null;default:false"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Message struct {
	ID         string `gorm:"type:uuid;primaryKey"`
	TenantID   string `gorm:"type:uuid;index;not null"`
	InstanceID string `gorm:"type:uuid;index;not null"`
	Direction  string `gorm:"size:50;not null"`
	ExternalID string `gorm:"size:255;index"`
	Body       string `gorm:"type:text"`
	Status     string `gorm:"size:50;not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Contact struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	TenantID    string `gorm:"type:uuid;uniqueIndex:idx_contacts_tenant_phone;index;not null"`
	Phone       string `gorm:"size:50;uniqueIndex:idx_contacts_tenant_phone;index;not null"`
	Name        string `gorm:"size:255;not null"`
	Email       string `gorm:"size:255"`
	InstanceID  string `gorm:"type:uuid;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Tags        []Tag  `gorm:"many2many:contact_tags;"`
	Notes       []Note
}

type Tag struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	TenantID  string `gorm:"type:uuid;uniqueIndex:idx_tags_tenant_name;index;not null"`
	Name      string `gorm:"size:255;uniqueIndex:idx_tags_tenant_name;not null"`
	Color     string `gorm:"size:32"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Note struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	TenantID  string `gorm:"type:uuid;index;not null"`
	ContactID string `gorm:"type:uuid;index;not null"`
	Body      string `gorm:"type:text;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Pipeline struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	TenantID  string `gorm:"type:uuid;index;not null"`
	Name      string `gorm:"size:255;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Stages    []DealStage
}

type DealStage struct {
	ID         string `gorm:"type:uuid;primaryKey"`
	TenantID   string `gorm:"type:uuid;index;not null"`
	PipelineID string `gorm:"type:uuid;index;not null"`
	Name       string `gorm:"size:255;not null"`
	Position   int    `gorm:"not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Deal struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	TenantID    string `gorm:"type:uuid;index;not null"`
	ContactID   string `gorm:"type:uuid;index;not null"`
	PipelineID  string `gorm:"type:uuid;index;not null"`
	DealStageID string `gorm:"type:uuid;index;not null"`
	Title       string `gorm:"size:255;not null"`
	Value       int64  `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type BroadcastJob struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	TenantID     string `gorm:"type:uuid;index;not null"`
	InstanceID   string `gorm:"type:uuid;index;not null"`
	Status       string `gorm:"size:50;index;not null"`
	Message      string `gorm:"type:text;not null"`
	RatePerHour  int    `gorm:"not null;default:0"`
	DelaySec     int    `gorm:"not null;default:0"`
	Attempts     int    `gorm:"not null;default:0"`
	MaxAttempts  int    `gorm:"not null;default:3"`
	WorkerID     string `gorm:"size:100;index"`
	LastError    string `gorm:"type:text"`
	ScheduledAt  *time.Time
	AvailableAt  time.Time `gorm:"index;not null"`
	StartedAt    *time.Time
	CompletedAt  *time.Time
	FailedAt     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type WebhookEndpoint struct {
	ID                  string `gorm:"type:uuid;primaryKey"`
	TenantID            string `gorm:"type:uuid;index;not null"`
	Name                string `gorm:"size:255;not null"`
	URL                 string `gorm:"size:500;not null"`
	InboundEnabled      bool   `gorm:"not null;default:true"`
	OutboundEnabled     bool   `gorm:"not null;default:true"`
	SigningSecret       string `gorm:"size:255"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type WebhookDelivery struct {
	ID             string `gorm:"type:uuid;primaryKey"`
	TenantID       string `gorm:"type:uuid;index;not null"`
	EndpointID     string `gorm:"type:uuid;index;not null"`
	Direction      string `gorm:"size:50;index;not null"`
	EventType      string `gorm:"size:100;index;not null"`
	Status         string `gorm:"size:50;index;not null"`
	ResponseStatus int
	RequestBody    string `gorm:"type:text;not null"`
	ResponseBody   string `gorm:"type:text"`
	ErrorMessage   string `gorm:"type:text"`
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AISettings struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID     string    `json:"tenant_id" gorm:"type:uuid;uniqueIndex;not null"`
	Enabled      bool      `json:"enabled" gorm:"not null;default:false"`
	AutoReply    bool      `json:"auto_reply" gorm:"not null;default:false"`
	Provider     string    `json:"provider" gorm:"size:100;not null;default:'openai'"`
	Model        string    `json:"model" gorm:"size:100;not null"`
	BaseURL      string    `json:"base_url" gorm:"size:500"`
	SystemPrompt string    `json:"system_prompt" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AIConversationMessage struct {
	ID              string `gorm:"type:uuid;primaryKey"`
	TenantID        string `gorm:"type:uuid;index;not null"`
	InstanceID      string `gorm:"type:uuid;index;not null"`
	ConversationKey string `gorm:"size:255;index;not null"`
	Role            string `gorm:"size:50;not null"`
	Content         string `gorm:"type:text;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (t *Tenant) BeforeCreate(_ *gorm.DB) error      { ensureID(&t.ID); return nil }
func (u *User) BeforeCreate(_ *gorm.DB) error        { ensureID(&u.ID); return nil }
func (i *Instance) BeforeCreate(_ *gorm.DB) error    { ensureID(&i.ID); return nil }
func (m *Message) BeforeCreate(_ *gorm.DB) error     { ensureID(&m.ID); return nil }
func (c *Contact) BeforeCreate(_ *gorm.DB) error     { ensureID(&c.ID); return nil }
func (t *Tag) BeforeCreate(_ *gorm.DB) error         { ensureID(&t.ID); return nil }
func (n *Note) BeforeCreate(_ *gorm.DB) error        { ensureID(&n.ID); return nil }
func (p *Pipeline) BeforeCreate(_ *gorm.DB) error    { ensureID(&p.ID); return nil }
func (d *DealStage) BeforeCreate(_ *gorm.DB) error   { ensureID(&d.ID); return nil }
func (d *Deal) BeforeCreate(_ *gorm.DB) error        { ensureID(&d.ID); return nil }
func (b *BroadcastJob) BeforeCreate(_ *gorm.DB) error { ensureID(&b.ID); return nil }
func (w *WebhookEndpoint) BeforeCreate(_ *gorm.DB) error {
	ensureID(&w.ID)
	return nil
}
func (w *WebhookDelivery) BeforeCreate(_ *gorm.DB) error { ensureID(&w.ID); return nil }
func (a *AISettings) BeforeCreate(_ *gorm.DB) error { ensureID(&a.ID); return nil }
func (a *AIConversationMessage) BeforeCreate(_ *gorm.DB) error { ensureID(&a.ID); return nil }

func ensureID(id *string) {
	if *id == "" {
		*id = uuid.NewString()
	}
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func GenerateTenantAPIKey() (string, error) {
	const prefix = "evo_tk_"
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + strings.ToUpper(hex.EncodeToString(buf)), nil
}

func APIKeyPrefix(apiKey string) string {
	if len(apiKey) <= 12 {
		return apiKey
	}
	return apiKey[:12]
}

func HashAPIKey(apiKey string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CheckAPIKey(hash, apiKey string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(apiKey))
}

func FingerprintAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:8])
}
