package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/chathistory"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type aiTrigger interface {
	HandleInboundAsync(ctx context.Context, tenantID string, input AITriggerInput) error
}

type Service struct {
	repo   repository.WebhookRepository
	client HTTPClient
	logger *slog.Logger
	ai     aiTrigger
}

type CreateInput struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	InboundEnabled  bool   `json:"inbound_enabled"`
	OutboundEnabled bool   `json:"outbound_enabled"`
	SigningSecret   string `json:"signing_secret"`
}

type DispatchInput struct {
	EventType  string         `json:"event_type"`
	InstanceID string         `json:"instance_id"`
	MessageID  string         `json:"message_id"`
	Data       map[string]any `json:"data"`
}

type AITriggerInput struct {
	EventType       string
	InstanceID      string
	ConversationKey string
	MessageID       string
	MessageText     string
	Metadata        map[string]any
}

type DeliveryResult struct {
	EndpointID   string `json:"endpoint_id"`
	EndpointName string `json:"endpoint_name"`
	URL          string `json:"url"`
	Delivered    bool   `json:"delivered"`
	StatusCode   int    `json:"status_code"`
	Error        string `json:"error,omitempty"`
}

type eventEnvelope struct {
	TenantID   string         `json:"tenant_id"`
	Direction  string         `json:"direction"`
	EventType  string         `json:"event_type"`
	InstanceID string         `json:"instance_id,omitempty"`
	MessageID  string         `json:"message_id,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	Data       map[string]any `json:"data"`
}

func NewService(repo repository.WebhookRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		repo: repo,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

func (s *Service) SetAITrigger(trigger aiTrigger) {
	s.ai = trigger
}

func (s *Service) Create(ctx context.Context, tenantID string, input CreateInput) (*repository.WebhookEndpoint, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.URL) == "" {
		return nil, fmt.Errorf("%w: name and url are required", domain.ErrValidation)
	}

	if err := validateWebhookURL(input.URL); err != nil {
		return nil, err
	}

	inboundEnabled := input.InboundEnabled
	outboundEnabled := input.OutboundEnabled
	if !inboundEnabled && !outboundEnabled {
		inboundEnabled = true
		outboundEnabled = true
	}

	endpoint := &repository.WebhookEndpoint{
		TenantID:        tenantID,
		Name:            strings.TrimSpace(input.Name),
		URL:             strings.TrimSpace(input.URL),
		InboundEnabled:  inboundEnabled,
		OutboundEnabled: outboundEnabled,
		SigningSecret:   strings.TrimSpace(input.SigningSecret),
	}
	if err := s.repo.Create(ctx, endpoint); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]repository.WebhookEndpoint, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

func (s *Service) Get(ctx context.Context, tenantID, endpointID string) (*repository.WebhookEndpoint, error) {
	endpoint, err := s.repo.GetByID(ctx, tenantID, endpointID)
	if err != nil {
		return nil, fmt.Errorf("%w: webhook endpoint not found", domain.ErrNotFound)
	}
	return endpoint, nil
}

func (s *Service) DispatchInbound(ctx context.Context, tenantID string, input DispatchInput) ([]DeliveryResult, error) {
	notifyInboundConversationFallback(input)

	results, err := s.dispatch(ctx, tenantID, "inbound", input)
	if err != nil {
		return nil, err
	}

	if s.ai != nil {
		_ = s.ai.HandleInboundAsync(ctx, tenantID, AITriggerInput{
			EventType:       strings.TrimSpace(input.EventType),
			InstanceID:      strings.TrimSpace(input.InstanceID),
			ConversationKey: conversationKeyFromData(input),
			MessageID:       strings.TrimSpace(input.MessageID),
			MessageText:     messageTextFromData(input.Data),
			Metadata:        input.Data,
		})
	}

	return results, nil
}

func notifyInboundConversationFallback(input DispatchInput) {
	if !isInboundConversationEvent(input.EventType) {
		return
	}

	instanceID := strings.TrimSpace(input.InstanceID)
	messageID := strings.TrimSpace(input.MessageID)
	remoteJID := firstNonEmptyString(
		anyString(input.Data["remote_jid"]),
		anyString(input.Data["remoteJid"]),
		anyString(input.Data["chat_id"]),
		anyString(input.Data["chatId"]),
		anyString(input.Data["from"]),
	)
	if instanceID == "" || messageID == "" || remoteJID == "" {
		return
	}

	payload := normalizedWebhookMessagePayload(input.Data)
	chathistory.NotifyInboundMessage(chathistory.InboundMessage{
		InstanceID:  instanceID,
		MessageID:   messageID,
		RemoteJID:   remoteJID,
		PushName:    firstNonEmptyString(anyString(input.Data["push_name"]), anyString(input.Data["pushName"])),
		MessageType: firstNonEmptyString(anyString(input.Data["message_type"]), anyString(input.Data["messageType"]), inferredWebhookMessageType(payload)),
		Body:        firstNonEmptyString(messageTextFromData(input.Data), messageTextFromData(payload)),
		Source:      trimRemoteJID(remoteJID),
		MediaURL:    firstNonEmptyString(anyString(input.Data["media_url"]), anyString(input.Data["mediaUrl"]), anyString(payload["media_url"]), anyString(payload["mediaUrl"])),
		MimeType:    firstNonEmptyString(anyString(input.Data["mime_type"]), anyString(input.Data["mimeType"]), anyString(payload["mime_type"]), anyString(payload["mimeType"])),
		FileName:    firstNonEmptyString(anyString(input.Data["file_name"]), anyString(input.Data["fileName"]), anyString(payload["file_name"]), anyString(payload["fileName"])),
		Caption:     firstNonEmptyString(anyString(input.Data["caption"]), anyString(payload["caption"])),
		Message:     payload,
		Timestamp:   timestampFromWebhookInput(input.Data),
	})
}

func (s *Service) DispatchOutbound(ctx context.Context, tenantID string, input DispatchInput) ([]DeliveryResult, error) {
	return s.dispatch(ctx, tenantID, "outbound", input)
}

func (s *Service) dispatch(ctx context.Context, tenantID, direction string, input DispatchInput) ([]DeliveryResult, error) {
	if strings.TrimSpace(input.EventType) == "" {
		return nil, fmt.Errorf("%w: event_type is required", domain.ErrValidation)
	}

	endpoints, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	envelope := eventEnvelope{
		TenantID:   tenantID,
		Direction:  direction,
		EventType:  strings.TrimSpace(input.EventType),
		InstanceID: strings.TrimSpace(input.InstanceID),
		MessageID:  strings.TrimSpace(input.MessageID),
		Timestamp:  time.Now().UTC(),
		Data:       input.Data,
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}

	results := make([]DeliveryResult, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if !shouldDeliver(endpoint, direction) {
			continue
		}
		results = append(results, s.deliver(ctx, endpoint, envelope.EventType, direction, body))
	}

	return results, nil
}

func (s *Service) deliver(ctx context.Context, endpoint repository.WebhookEndpoint, eventType, direction string, body []byte) DeliveryResult {
	result := DeliveryResult{
		EndpointID:   endpoint.ID,
		EndpointName: endpoint.Name,
		URL:          endpoint.URL,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		result.Error = err.Error()
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Evolution-Tenant-ID", endpoint.TenantID)
	req.Header.Set("X-Evolution-Event-Type", eventType)
	req.Header.Set("X-Evolution-Direction", direction)
	if endpoint.SigningSecret != "" {
		req.Header.Set("X-Evolution-Signature", signPayload(endpoint.SigningSecret, body))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		s.logger.Error("webhook delivery failed", "endpoint_id", endpoint.ID, "tenant_id", endpoint.TenantID, "direction", direction, "error", err)
		return result
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	result.StatusCode = resp.StatusCode
	result.Delivered = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.Delivered {
		result.Error = string(responseBody)
	}

	s.logger.Info("webhook delivered",
		"endpoint_id", endpoint.ID,
		"tenant_id", endpoint.TenantID,
		"direction", direction,
		"event_type", eventType,
		"status_code", resp.StatusCode,
	)

	return result
}

func shouldDeliver(endpoint repository.WebhookEndpoint, direction string) bool {
	if direction == "inbound" {
		return endpoint.InboundEnabled
	}
	return endpoint.OutboundEnabled
}

func validateWebhookURL(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w: invalid webhook url", domain.ErrValidation)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: webhook url must use http or https", domain.ErrValidation)
	}
	return nil
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func messageTextFromData(data map[string]any) string {
	for _, key := range []string{"message", "text", "body", "content"} {
		if raw, ok := data[key]; ok {
			if value, ok := raw.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func conversationKeyFromData(input DispatchInput) string {
	for _, key := range []string{"conversation_key", "conversationKey", "from", "contact_id", "contactId", "chat_id", "chatId"} {
		if raw, ok := input.Data[key]; ok {
			if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	if strings.TrimSpace(input.InstanceID) != "" && strings.TrimSpace(input.MessageID) != "" {
		return strings.TrimSpace(input.InstanceID) + ":" + strings.TrimSpace(input.MessageID)
	}
	if strings.TrimSpace(input.InstanceID) != "" {
		return strings.TrimSpace(input.InstanceID)
	}
	return ""
}

func isInboundConversationEvent(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "message", "message.received", "messages.upsert", "message.upsert":
		return true
	default:
		return false
	}
}

func normalizedWebhookMessagePayload(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	if payload, ok := data["Message"].(map[string]any); ok && payload != nil {
		return payload
	}
	if payload, ok := data["message"].(map[string]any); ok && payload != nil {
		return payload
	}
	return data
}

func inferredWebhookMessageType(payload map[string]any) string {
	switch {
	case payload == nil:
		return ""
	case payload["imageMessage"] != nil:
		return "imageMessage"
	case payload["videoMessage"] != nil:
		return "videoMessage"
	case payload["audioMessage"] != nil:
		return "audioMessage"
	case payload["documentMessage"] != nil:
		return "documentMessage"
	case payload["extendedTextMessage"] != nil:
		return "conversation"
	case strings.TrimSpace(anyString(payload["conversation"])) != "":
		return "conversation"
	default:
		return ""
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimRemoteJID(remoteJID string) string {
	trimmed := strings.TrimSpace(remoteJID)
	if idx := strings.Index(trimmed, "@"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}

func timestampFromWebhookInput(data map[string]any) time.Time {
	for _, key := range []string{"timestamp", "message_timestamp", "messageTimestamp"} {
		if raw, ok := data[key]; ok {
			switch value := raw.(type) {
			case string:
				if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
					return parsed.UTC()
				}
			case time.Time:
				return value.UTC()
			}
		}
	}
	return time.Now().UTC()
}
