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
)

type aiRepoMock struct {
	settings *repository.AISettings
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
