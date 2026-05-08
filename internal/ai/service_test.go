package ai

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"gorm.io/gorm"
)

type aiRepoMock struct {
	settings *repository.AISettings
	getErr   error
	messages []repository.AIConversationMessage
	mu       sync.Mutex
}

func (m *aiRepoMock) Upsert(_ context.Context, settings *repository.AISettings) error {
	m.settings = settings
	return nil
}

func (m *aiRepoMock) GetByTenant(_ context.Context, tenantID string) (*repository.AISettings, error) {
	if m.settings != nil {
		return m.settings, nil
	}
	if m.getErr != nil {
		return nil, m.getErr
	}
	return &repository.AISettings{
		TenantID:     tenantID,
		Enabled:      true,
		AutoReply:    true,
		Provider:     "openai",
		Model:        "gpt-test",
		BaseURL:      "https://example.com/v1",
		SystemPrompt: "Be helpful",
	}, nil
}

func (m *aiRepoMock) AppendConversationMessage(_ context.Context, message *repository.AIConversationMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, *message)
	return nil
}

func (m *aiRepoMock) ListConversationMessages(_ context.Context, tenantID, instanceID, conversationKey string, limit int) ([]repository.AIConversationMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]repository.AIConversationMessage, 0, len(m.messages))
	for _, item := range m.messages {
		if item.TenantID == tenantID && item.InstanceID == instanceID && item.ConversationKey == conversationKey {
			result = append(result, item)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}

type aiInstanceRepoMock struct {
	instance *repository.Instance
}

func (m aiInstanceRepoMock) GetByID(_ context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	if m.instance != nil {
		return m.instance, nil
	}
	return &repository.Instance{
		ID:          instanceID,
		TenantID:    tenantID,
		AIEnabled:   true,
		AIAutoReply: true,
	}, nil
}

func (m aiInstanceRepoMock) Update(_ context.Context, instance *repository.Instance) error {
	m.instance = instance
	return nil
}

type aiDispatcherMock struct {
	calls []DispatchInput
	mu    sync.Mutex
}

func (m *aiDispatcherMock) DispatchOutbound(_ context.Context, _ string, input DispatchInput) ([]DeliveryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, input)
	return nil, nil
}

type aiHTTPClientMock struct{}

func (aiHTTPClientMock) Do(*http.Request) (*http.Response, error) {
	payload := `{"choices":[{"message":{"role":"assistant","content":"Automated reply"}}]}`
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestAIHandleInboundAsyncStoresMemoryAndDispatchesWebhook(t *testing.T) {
	repo := &aiRepoMock{}
	dispatcher := &aiDispatcherMock{}
	service := NewService(repo, aiInstanceRepoMock{}, &config.AIConfig{
		OpenAIAPIKey: "test-key",
		BaseURL:      "https://example.com/v1",
		Model:        "gpt-test",
		Timeout:      2 * time.Second,
		Workers:      1,
		MemoryLimit:  10,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.client = aiHTTPClientMock{}
	service.SetOutboundDispatcher(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.Start(ctx)

	if err := service.HandleInboundAsync(ctx, "tenant-1", IncomingMessageInput{
		EventType:       "message.received",
		InstanceID:      "instance-1",
		ConversationKey: "contact-1",
		MessageID:       "msg-1",
		MessageText:     "Hello there",
	}); err != nil {
		t.Fatalf("enqueue ai job: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		repo.mu.Lock()
		messageCount := len(repo.messages)
		repo.mu.Unlock()
		dispatcher.mu.Lock()
		callCount := len(dispatcher.calls)
		dispatcher.mu.Unlock()
		if messageCount >= 2 && callCount >= 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected assistant reply and webhook dispatch, got %d memory messages and %d webhook calls", len(repo.messages), len(dispatcher.calls))
}

func TestGetTenantSettingsReturnsSafeDefaultsWhenMissing(t *testing.T) {
	service := NewService(&aiRepoMock{getErr: gorm.ErrRecordNotFound}, aiInstanceRepoMock{}, &config.AIConfig{
		BaseURL: "https://example.com/v1",
		Model:   "gpt-test",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	settings, err := service.GetTenantSettings(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("get tenant settings: %v", err)
	}

	if settings.TenantID != "tenant-1" {
		t.Fatalf("expected tenant id to be preserved, got %q", settings.TenantID)
	}
	if settings.Enabled || settings.AutoReply {
		t.Fatalf("expected first-use defaults to be disabled, got enabled=%v auto_reply=%v", settings.Enabled, settings.AutoReply)
	}
	if settings.Provider != "openai" || settings.Model != "gpt-test" || settings.BaseURL != "https://example.com/v1" {
		t.Fatalf("unexpected safe defaults: provider=%q model=%q base_url=%q", settings.Provider, settings.Model, settings.BaseURL)
	}
	if settings.SystemPrompt != "" {
		t.Fatalf("expected no system prompt by default, got %q", settings.SystemPrompt)
	}
}

func TestGetInstanceSettingsIncludesTenantDefaultsWhenMissing(t *testing.T) {
	service := NewService(&aiRepoMock{getErr: gorm.ErrRecordNotFound}, aiInstanceRepoMock{
		instance: &repository.Instance{
			ID:          "instance-1",
			TenantID:    "tenant-1",
			AIEnabled:   true,
			AIAutoReply: true,
		},
	}, &config.AIConfig{
		BaseURL: "https://example.com/v1",
		Model:   "gpt-test",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	settings, err := service.GetInstanceSettings(context.Background(), "tenant-1", "instance-1")
	if err != nil {
		t.Fatalf("get instance settings: %v", err)
	}

	if settings.InstanceID != "instance-1" || !settings.Enabled || !settings.AutoReply {
		t.Fatalf("unexpected instance override settings: %+v", settings)
	}
	if settings.TenantSettingsConfigured {
		t.Fatalf("expected tenant settings to be reported as not configured")
	}
	if settings.TenantSettings == nil || settings.TenantSettings.Enabled || settings.TenantSettings.AutoReply {
		t.Fatalf("expected disabled tenant defaults, got %+v", settings.TenantSettings)
	}
	if settings.EffectiveEnabled || settings.EffectiveAutoReply {
		t.Fatalf("expected effective AI to remain disabled without tenant settings, got enabled=%v auto_reply=%v", settings.EffectiveEnabled, settings.EffectiveAutoReply)
	}
}
