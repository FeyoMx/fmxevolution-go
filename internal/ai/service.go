package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type outboundDispatcher interface {
	DispatchOutbound(ctx context.Context, tenantID string, input DispatchInput) ([]DeliveryResult, error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type instanceRepository interface {
	GetByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error)
	Update(ctx context.Context, instance *repository.Instance) error
}

type Service struct {
	repo       repository.AIRepository
	instances  instanceRepository
	logger     *slog.Logger
	client     HTTPClient
	cfg        *config.AIConfig
	dispatcher outboundDispatcher
	queue      chan job
	once       sync.Once
}

type TenantSettingsInput struct {
	Enabled      bool   `json:"enabled"`
	AutoReply    bool   `json:"auto_reply"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	SystemPrompt string `json:"system_prompt"`
}

type InstanceSettingsInput struct {
	Enabled   bool `json:"enabled"`
	AutoReply bool `json:"auto_reply"`
}

type IncomingMessageInput struct {
	EventType       string
	InstanceID      string
	ConversationKey string
	MessageID       string
	MessageText     string
	Metadata        map[string]any
}

type DispatchInput struct {
	EventType  string         `json:"event_type"`
	InstanceID string         `json:"instance_id"`
	MessageID  string         `json:"message_id"`
	Data       map[string]any `json:"data"`
}

type DeliveryResult struct {
	EndpointID   string `json:"endpoint_id"`
	EndpointName string `json:"endpoint_name"`
	URL          string `json:"url"`
	Delivered    bool   `json:"delivered"`
	StatusCode   int    `json:"status_code"`
	Error        string `json:"error,omitempty"`
}

type job struct {
	tenantID string
	input    IncomingMessageInput
}

type openAIChatCompletionRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}

func NewService(repo repository.AIRepository, instances instanceRepository, cfg *config.AIConfig, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg == nil {
		cfg = &config.AIConfig{
			BaseURL:     "https://api.openai.com/v1",
			Model:       "gpt-4o-mini",
			Timeout:     15 * time.Second,
			Workers:     2,
			MemoryLimit: 12,
		}
	}

	return &Service{
		repo:      repo,
		instances: instances,
		cfg:       cfg,
		logger:    logger,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		queue: make(chan job, cfg.Workers*8),
	}
}

func (s *Service) SetOutboundDispatcher(dispatcher outboundDispatcher) {
	s.dispatcher = dispatcher
}

func (s *Service) Start(ctx context.Context) {
	workers := s.cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	s.once.Do(func() {
		for i := 0; i < workers; i++ {
			go s.worker(ctx, i+1)
		}
	})
}

func (s *Service) ConfigureTenant(ctx context.Context, tenantID string, input TenantSettingsInput) (*repository.AISettings, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", domain.ErrValidation)
	}

	provider := strings.ToLower(strings.TrimSpace(defaultString(input.Provider, "openai")))
	if provider != "openai" {
		return nil, fmt.Errorf("%w: only openai-compatible provider is currently supported", domain.ErrValidation)
	}
	model := strings.TrimSpace(defaultString(input.Model, s.cfg.Model))
	baseURL := strings.TrimSpace(defaultString(input.BaseURL, s.cfg.BaseURL))
	if model == "" {
		return nil, fmt.Errorf("%w: model is required", domain.ErrValidation)
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%w: base_url is required", domain.ErrValidation)
	}

	settings := &repository.AISettings{
		TenantID:     tenantID,
		Enabled:      input.Enabled,
		AutoReply:    input.AutoReply,
		Provider:     provider,
		Model:        model,
		BaseURL:      baseURL,
		SystemPrompt: strings.TrimSpace(input.SystemPrompt),
	}
	if err := s.repo.Upsert(ctx, settings); err != nil {
		return nil, err
	}
	return s.repo.GetByTenant(ctx, tenantID)
}

func (s *Service) GetTenantSettings(ctx context.Context, tenantID string) (*repository.AISettings, error) {
	settings, err := s.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: ai settings not found", domain.ErrNotFound)
	}
	return settings, nil
}

func (s *Service) ConfigureInstance(ctx context.Context, tenantID, instanceID string, input InstanceSettingsInput) (*repository.Instance, error) {
	instance, err := s.instances.GetByID(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: instance not found", domain.ErrNotFound)
	}

	instance.AIEnabled = input.Enabled
	instance.AIAutoReply = input.AutoReply
	if err := s.instances.Update(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) GetInstanceSettings(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	instance, err := s.instances.GetByID(ctx, tenantID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: instance not found", domain.ErrNotFound)
	}
	return instance, nil
}

func (s *Service) HandleInboundAsync(_ context.Context, tenantID string, input IncomingMessageInput) error {
	if strings.TrimSpace(input.InstanceID) == "" || strings.TrimSpace(input.MessageText) == "" {
		return nil
	}
	if strings.TrimSpace(input.ConversationKey) == "" {
		input.ConversationKey = strings.TrimSpace(input.InstanceID)
	}

	select {
	case s.queue <- job{tenantID: tenantID, input: input}:
		return nil
	default:
		s.logger.Warn("ai queue full, dropping inbound message", "tenant_id", tenantID, "instance_id", input.InstanceID)
		return nil
	}
}

func (s *Service) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			s.process(ctx, workerID, job)
		}
	}
}

func (s *Service) process(ctx context.Context, workerID int, j job) {
	settings, err := s.repo.GetByTenant(ctx, j.tenantID)
	if err != nil || !settings.Enabled || !settings.AutoReply {
		return
	}

	instance, err := s.instances.GetByID(ctx, j.tenantID, j.input.InstanceID)
	if err != nil || !instance.AIEnabled || !instance.AIAutoReply {
		return
	}

	userMessage := &repository.AIConversationMessage{
		TenantID:        j.tenantID,
		InstanceID:      j.input.InstanceID,
		ConversationKey: j.input.ConversationKey,
		Role:            "user",
		Content:         strings.TrimSpace(j.input.MessageText),
	}
	if err := s.repo.AppendConversationMessage(ctx, userMessage); err != nil {
		s.logger.Error("append ai user memory", "worker_id", workerID, "error", err)
		return
	}

	timeout := s.cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reply, err := s.generateReply(runCtx, j.tenantID, j.input.InstanceID, j.input.ConversationKey, settings)
	if err != nil {
		s.logger.Error("generate ai reply", "worker_id", workerID, "tenant_id", j.tenantID, "instance_id", j.input.InstanceID, "error", err)
		return
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return
	}

	assistantMessage := &repository.AIConversationMessage{
		TenantID:        j.tenantID,
		InstanceID:      j.input.InstanceID,
		ConversationKey: j.input.ConversationKey,
		Role:            "assistant",
		Content:         reply,
	}
	if err := s.repo.AppendConversationMessage(ctx, assistantMessage); err != nil {
		s.logger.Error("append ai assistant memory", "worker_id", workerID, "error", err)
		return
	}

	if s.dispatcher != nil {
		_, _ = s.dispatcher.DispatchOutbound(ctx, j.tenantID, DispatchInput{
			EventType:  "ai.reply.generated",
			InstanceID: j.input.InstanceID,
			MessageID:  j.input.MessageID,
			Data: map[string]any{
				"conversation_key": j.input.ConversationKey,
				"reply":            reply,
				"source_event":     j.input.EventType,
				"metadata":         j.input.Metadata,
			},
		})
	}
}

func (s *Service) generateReply(ctx context.Context, tenantID, instanceID, conversationKey string, settings *repository.AISettings) (string, error) {
	limit := s.cfg.MemoryLimit
	if limit <= 0 {
		limit = 12
	}
	memory, err := s.repo.ListConversationMessages(ctx, tenantID, instanceID, conversationKey, limit)
	if err != nil {
		return "", err
	}

	messages := make([]openAIChatMessage, 0, len(memory)+1)
	systemPrompt := strings.TrimSpace(settings.SystemPrompt)
	if systemPrompt != "" {
		messages = append(messages, openAIChatMessage{Role: "system", Content: systemPrompt})
	}
	for _, item := range memory {
		messages = append(messages, openAIChatMessage{
			Role:    item.Role,
			Content: item.Content,
		})
	}

	body, err := json.Marshal(openAIChatCompletionRequest{
		Model:    defaultString(settings.Model, s.cfg.Model),
		Messages: messages,
	})
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(defaultString(settings.BaseURL, s.cfg.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.cfg.OpenAIAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai compatible api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var parsed openAIChatCompletionResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai compatible api returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}
