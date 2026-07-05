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
	repo        repository.WebhookRepository
	client      HTTPClient
	logger      *slog.Logger
	ai          aiTrigger
	maxAttempts int
	dedupeWindow time.Duration
	stopWorker  context.CancelFunc
	workerDone  chan struct{}
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
	DeliveryID   string `json:"delivery_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type eventEnvelope struct {
	// EventID is stable across retries of the same dispatch so that consumers
	// (n8n, Chatwoot bridges) can deduplicate deliveries.
	EventID      string         `json:"event_id"`
	TenantID     string         `json:"tenant_id"`
	Direction    string         `json:"direction"`
	EventType    string         `json:"event_type"`
	RawEventType string         `json:"raw_event_type,omitempty"`
	InstanceID   string         `json:"instance_id,omitempty"`
	MessageID    string         `json:"message_id,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
	Data         map[string]any `json:"data"`
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
		logger:       logger,
		maxAttempts:  5,
		dedupeWindow: 10 * time.Minute,
	}
}

// SetMaxAttempts overrides the delivery attempt budget (sync try + retries).
func (s *Service) SetMaxAttempts(attempts int) {
	if attempts > 0 {
		s.maxAttempts = attempts
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
	rawEventType := strings.TrimSpace(input.EventType)
	if rawEventType == "" {
		return nil, fmt.Errorf("%w: event_type is required", domain.ErrValidation)
	}
	normalizedEventType := NormalizeEventType(rawEventType, direction, input.Data)
	messageID := strings.TrimSpace(input.MessageID)

	// Idempotency: for inbound message events carrying a message id, skip a
	// re-dispatch of the same logical event within the dedupe window.
	if direction == "inbound" && messageID != "" {
		since := time.Now().Add(-s.dedupeWindow)
		if seen, err := s.repo.HasRecentDelivery(ctx, tenantID, direction, normalizedEventType, messageID, since); err == nil && seen {
			s.logger.Info("webhook dispatch deduplicated",
				"tenant_id", tenantID, "event_type", normalizedEventType, "message_id", messageID)
			return []DeliveryResult{}, nil
		}
	}

	endpoints, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	eventID := newEventID()
	envelope := eventEnvelope{
		EventID:      eventID,
		TenantID:     tenantID,
		Direction:    direction,
		EventType:    normalizedEventType,
		RawEventType: rawEventType,
		InstanceID:   strings.TrimSpace(input.InstanceID),
		MessageID:    messageID,
		Timestamp:    time.Now().UTC(),
		Data:         NormalizePayload(input, direction, normalizedEventType),
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}

	results := make([]DeliveryResult, 0, len(endpoints))
	for i := range endpoints {
		endpoint := endpoints[i]
		if !shouldDeliver(endpoint, direction) {
			continue
		}
		results = append(results, s.deliver(ctx, endpoint, envelope, body))
	}

	return results, nil
}

func (s *Service) deliver(ctx context.Context, endpoint repository.WebhookEndpoint, envelope eventEnvelope, body []byte) DeliveryResult {
	result := DeliveryResult{
		EndpointID:   endpoint.ID,
		EndpointName: endpoint.Name,
		URL:          endpoint.URL,
	}

	delivery := &repository.WebhookDelivery{
		TenantID:    endpoint.TenantID,
		EndpointID:  endpoint.ID,
		EndpointURL: endpoint.URL,
		Direction:   envelope.Direction,
		EventType:   envelope.EventType,
		EventID:     envelope.EventID,
		MessageID:   envelope.MessageID,
		Status:      "pending",
		Attempts:    0,
		MaxAttempts: s.maxAttempts,
		RequestBody: string(body),
	}

	statusCode, respBody, sendErr := s.sendOnce(ctx, endpoint, envelope, body)
	delivery.Attempts = 1
	delivery.ResponseStatus = statusCode
	delivery.ResponseBody = respBody
	result.StatusCode = statusCode

	if sendErr == nil && statusCode >= 200 && statusCode < 300 {
		now := time.Now().UTC()
		delivery.Status = "delivered"
		delivery.DeliveredAt = &now
		result.Delivered = true
		s.logger.Info("webhook delivered",
			"endpoint_id", endpoint.ID, "tenant_id", endpoint.TenantID,
			"direction", envelope.Direction, "event_type", envelope.EventType, "status_code", statusCode)
	} else {
		result.Error = deliveryError(sendErr, respBody)
		delivery.ErrorMessage = result.Error
		s.scheduleRetryFields(delivery, 1)
		s.logger.Warn("webhook delivery attempt failed",
			"endpoint_id", endpoint.ID, "tenant_id", endpoint.TenantID,
			"direction", envelope.Direction, "event_type", envelope.EventType,
			"status_code", statusCode, "next_status", delivery.Status, "error", result.Error)
	}

	if err := s.repo.CreateDelivery(ctx, delivery); err != nil {
		s.logger.Error("persist webhook delivery failed",
			"endpoint_id", endpoint.ID, "tenant_id", endpoint.TenantID, "error", err)
	}
	result.DeliveryID = delivery.ID
	return result
}

// sendOnce performs a single HTTP POST attempt and returns status/body/error.
func (s *Service) sendOnce(ctx context.Context, endpoint repository.WebhookEndpoint, envelope eventEnvelope, body []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Evolution-Tenant-ID", endpoint.TenantID)
	req.Header.Set("X-Evolution-Event-Type", envelope.EventType)
	req.Header.Set("X-Evolution-Direction", envelope.Direction)
	req.Header.Set("X-Evolution-Event-ID", envelope.EventID)
	if endpoint.SigningSecret != "" {
		req.Header.Set("X-Evolution-Signature", signPayload(endpoint.SigningSecret, body))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(responseBody), nil
}

// scheduleRetryFields sets status/next-attempt for a delivery that just failed
// its attemptNumber-th attempt. Exponential backoff capped at 30m.
func (s *Service) scheduleRetryFields(delivery *repository.WebhookDelivery, attemptNumber int) {
	if attemptNumber >= delivery.MaxAttempts {
		now := time.Now().UTC()
		delivery.Status = "failed"
		delivery.NextAttemptAt = nil
		delivery.DeliveredAt = nil
		_ = now
		return
	}
	delivery.Status = "retrying"
	next := time.Now().UTC().Add(backoffFor(attemptNumber))
	delivery.NextAttemptAt = &next
}

func backoffFor(attemptNumber int) time.Duration {
	// attempt 1 -> 30s, 2 -> 60s, 3 -> 120s, 4 -> 240s ... capped at 30m.
	base := 30 * time.Second
	d := base << (attemptNumber - 1)
	if d > 30*time.Minute {
		return 30 * time.Minute
	}
	return d
}

func deliveryError(sendErr error, respBody string) string {
	if sendErr != nil {
		return sendErr.Error()
	}
	if respBody != "" {
		return respBody
	}
	return "non-2xx response"
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
