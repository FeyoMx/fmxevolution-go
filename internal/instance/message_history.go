package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

const (
	defaultMessageSearchLimit = 100
	maxMessageSearchLimit     = 250
)

type MessageSearchRequest struct {
	Where  map[string]any `json:"where"`
	Limit  int            `json:"limit"`
	Cursor string         `json:"cursor,omitempty"`
}

type messageSearchFilter struct {
	RemoteJID string
	MessageID string
	Query     string
	Limit     int
	Before    *time.Time
}

type legacyMessageKey struct {
	ID        string `json:"id"`
	FromMe    bool   `json:"fromMe"`
	RemoteJID string `json:"remoteJid"`
}

type legacyMessageRecord struct {
	ID               string           `json:"id"`
	Key              legacyMessageKey `json:"key"`
	PushName         string           `json:"pushName"`
	MessageType      string           `json:"messageType"`
	Message          map[string]any   `json:"message"`
	MessageTimestamp string           `json:"messageTimestamp"`
	InstanceID       string           `json:"instanceId"`
	Source           string           `json:"source"`
}

func normalizeMessageSearchRequest(input MessageSearchRequest) (messageSearchFilter, error) {
	filter := messageSearchFilter{
		Limit: normalizeMessageSearchLimit(input.Limit),
	}

	if filter.Limit == 0 {
		filter.Limit = defaultMessageSearchLimit
	}

	filter.RemoteJID = strings.TrimSpace(
		firstString(
			nestedString(input.Where, "key", "remoteJid"),
			nestedString(input.Where, "key", "jid"),
			stringValue(input.Where["remoteJid"]),
			stringValue(input.Where["chatId"]),
			stringValue(input.Where["jid"]),
		),
	)
	filter.MessageID = strings.TrimSpace(
		firstString(
			nestedString(input.Where, "key", "id"),
			stringValue(input.Where["id"]),
			stringValue(input.Where["messageId"]),
		),
	)
	filter.Query = strings.TrimSpace(
		firstString(
			stringValue(input.Where["query"]),
			stringValue(input.Where["search"]),
			stringValue(input.Where["text"]),
		),
	)

	beforeRaw := strings.TrimSpace(
		firstString(
			input.Cursor,
			stringValue(input.Where["before"]),
			stringValue(input.Where["cursor"]),
		),
	)
	if beforeRaw != "" {
		before, err := parseHistoryCursor(beforeRaw)
		if err != nil {
			return messageSearchFilter{}, fmt.Errorf("%w: invalid message cursor", domain.ErrValidation)
		}
		filter.Before = before
	}

	if filter.RemoteJID == "" {
		return messageSearchFilter{}, fmt.Errorf("%w: where.key.remoteJid is required", domain.ErrValidation)
	}

	return filter, nil
}

func normalizeMessageSearchLimit(value int) int {
	switch {
	case value <= 0:
		return 0
	case value > maxMessageSearchLimit:
		return maxMessageSearchLimit
	default:
		return value
	}
}

func parseHistoryCursor(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		value := ts.UTC()
		return &value, nil
	}
	if numeric, err := strconv.ParseInt(raw, 10, 64); err == nil {
		var value time.Time
		if numeric > 1_000_000_000_000 {
			value = time.UnixMilli(numeric).UTC()
		} else {
			value = time.Unix(numeric, 0).UTC()
		}
		return &value, nil
	}
	return nil, fmt.Errorf("unsupported cursor format")
}

func nestedString(values map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}

	current := any(values)
	for _, key := range keys {
		mapped, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = mapped[key]
		if !ok {
			return ""
		}
	}

	return stringValue(current)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimRemoteJID(remoteJID string) string {
	trimmed := strings.TrimSpace(remoteJID)
	if trimmed == "" {
		return ""
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		return trimmed[:at]
	}
	return trimmed
}

func normalizeStoredMessageType(messageType string) string {
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "conversation", "extendedtextmessage", "extendedtext", "text":
		return "conversation"
	case "imagemessage", "image":
		return "imageMessage"
	case "videomessage", "video":
		return "videoMessage"
	case "audiomessage", "audio":
		return "audioMessage"
	case "documentmessage", "document":
		return "documentMessage"
	default:
		return strings.TrimSpace(messageType)
	}
}

func toLegacyMessageRecords(items []repository.ConversationMessage) []legacyMessageRecord {
	records := make([]legacyMessageRecord, 0, len(items))
	for _, item := range items {
		records = append(records, legacyMessageRecord{
			ID: firstString(item.ExternalMessageID, item.ID),
			Key: legacyMessageKey{
				ID:        firstString(item.ExternalMessageID, item.ID),
				FromMe:    strings.EqualFold(item.Direction, "outbound"),
				RemoteJID: item.RemoteJID,
			},
			PushName:         firstString(item.PushName, trimRemoteJID(item.RemoteJID)),
			MessageType:      normalizeStoredMessageType(item.MessageType),
			Message:          buildFrontendMessagePayload(item),
			MessageTimestamp: item.MessageTimestamp.UTC().Format(time.RFC3339),
			InstanceID:       item.InstanceID,
			Source:           firstString(item.Source, item.RemoteJID),
		})
	}
	return records
}

func buildFrontendMessagePayload(item repository.ConversationMessage) map[string]any {
	if payload := decodeMessagePayload(item.MessagePayload); payload != nil {
		return payload
	}

	messageType := normalizeStoredMessageType(item.MessageType)
	switch messageType {
	case "conversation":
		return map[string]any{
			"conversation": item.Body,
		}
	case "imageMessage":
		return map[string]any{
			"imageMessage": map[string]any{
				"caption":  item.Caption,
				"mimetype": item.MimeType,
			},
			"mediaUrl": item.MediaURL,
			"mimetype": item.MimeType,
		}
	case "videoMessage":
		return map[string]any{
			"videoMessage": map[string]any{
				"caption":  item.Caption,
				"mimetype": item.MimeType,
			},
			"mediaUrl": item.MediaURL,
			"mimetype": item.MimeType,
		}
	case "audioMessage":
		return map[string]any{
			"audioMessage": map[string]any{
				"mimetype": item.MimeType,
				"ptt":      true,
			},
			"mediaUrl": item.MediaURL,
			"mimetype": item.MimeType,
		}
	case "documentMessage":
		return map[string]any{
			"documentMessage": map[string]any{
				"caption":  item.Caption,
				"mimetype": item.MimeType,
				"fileName": item.FileName,
			},
			"mediaUrl": item.MediaURL,
			"mimetype": item.MimeType,
		}
	default:
		return map[string]any{
			"text": item.Body,
		}
	}
}

func decodeMessagePayload(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}
	return payload
}

func marshalMessagePayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func (s *Service) persistOutboundText(ctx context.Context, tenantID string, instance *repository.Instance, input SendTextInput, result *SendTextResult) {
	if s == nil || s.history == nil || instance == nil || result == nil {
		return
	}

	message := &repository.ConversationMessage{
		TenantID:          tenantID,
		InstanceID:        instance.ID,
		RemoteJID:         firstString(result.Chat, strings.TrimSpace(input.Number)),
		ExternalMessageID: result.MessageID,
		Direction:         "outbound",
		MessageType:       "conversation",
		PushName:          trimRemoteJID(result.Chat),
		Source:            trimRemoteJID(result.Chat),
		Body:              strings.TrimSpace(input.Text),
		Status:            "sent",
		MessageTimestamp:  result.Timestamp.UTC(),
		MessagePayload: marshalMessagePayload(map[string]any{
			"conversation": strings.TrimSpace(input.Text),
		}),
	}

	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.history.Upsert(persistCtx, message); err != nil && s.logger != nil {
		s.logger.Warn("persist outbound text history failed", "instance_id", instance.ID, "message_id", result.MessageID, "error", err)
	}
}

func (s *Service) persistOutboundMedia(ctx context.Context, tenantID string, instance *repository.Instance, input resolvedMediaMessageInput, result *SendMediaOutput) {
	if s == nil || s.history == nil || instance == nil || result == nil {
		return
	}

	frontendType := normalizeStoredMessageType(result.MessageType)
	payload := buildOutboundMediaPayload(frontendType, input)
	message := &repository.ConversationMessage{
		TenantID:          tenantID,
		InstanceID:        instance.ID,
		RemoteJID:         firstString(result.Chat, strings.TrimSpace(input.Number)),
		ExternalMessageID: result.MessageID,
		Direction:         "outbound",
		MessageType:       frontendType,
		PushName:          trimRemoteJID(result.Chat),
		Source:            trimRemoteJID(result.Chat),
		Body:              strings.TrimSpace(input.Caption),
		Status:            "sent",
		MessageTimestamp:  result.Timestamp.UTC(),
		MediaURL:          strings.TrimSpace(input.URL),
		MimeType:          strings.TrimSpace(input.MimeType),
		FileName:          strings.TrimSpace(input.FileName),
		Caption:           strings.TrimSpace(input.Caption),
		MessagePayload:    marshalMessagePayload(payload),
	}

	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.history.Upsert(persistCtx, message); err != nil && s.logger != nil {
		s.logger.Warn("persist outbound media history failed", "instance_id", instance.ID, "message_id", result.MessageID, "error", err)
	}
}

func (s *Service) persistOutboundAudio(ctx context.Context, tenantID string, instance *repository.Instance, input resolvedAudioMessageInput, result *SendMediaOutput) {
	if s == nil || s.history == nil || instance == nil || result == nil {
		return
	}

	payload := map[string]any{
		"audioMessage": map[string]any{
			"ptt": true,
		},
	}
	message := &repository.ConversationMessage{
		TenantID:          tenantID,
		InstanceID:        instance.ID,
		RemoteJID:         firstString(result.Chat, strings.TrimSpace(input.Number)),
		ExternalMessageID: result.MessageID,
		Direction:         "outbound",
		MessageType:       "audioMessage",
		PushName:          trimRemoteJID(result.Chat),
		Source:            trimRemoteJID(result.Chat),
		Status:            "sent",
		MessageTimestamp:  result.Timestamp.UTC(),
		MimeType:          "audio/ogg",
		MessagePayload:    marshalMessagePayload(payload),
	}

	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.history.Upsert(persistCtx, message); err != nil && s.logger != nil {
		s.logger.Warn("persist outbound audio history failed", "instance_id", instance.ID, "message_id", result.MessageID, "error", err)
	}
}

func buildOutboundMediaPayload(frontendType string, input resolvedMediaMessageInput) map[string]any {
	switch frontendType {
	case "imageMessage":
		return map[string]any{
			"imageMessage": map[string]any{
				"caption":  strings.TrimSpace(input.Caption),
				"mimetype": strings.TrimSpace(input.MimeType),
			},
			"mediaUrl": strings.TrimSpace(input.URL),
			"mimetype": strings.TrimSpace(input.MimeType),
		}
	case "videoMessage":
		return map[string]any{
			"videoMessage": map[string]any{
				"caption":  strings.TrimSpace(input.Caption),
				"mimetype": strings.TrimSpace(input.MimeType),
			},
			"mediaUrl": strings.TrimSpace(input.URL),
			"mimetype": strings.TrimSpace(input.MimeType),
		}
	case "documentMessage":
		return map[string]any{
			"documentMessage": map[string]any{
				"caption":  strings.TrimSpace(input.Caption),
				"mimetype": strings.TrimSpace(input.MimeType),
				"fileName": strings.TrimSpace(input.FileName),
			},
			"mediaUrl": strings.TrimSpace(input.URL),
			"mimetype": strings.TrimSpace(input.MimeType),
		}
	case "audioMessage":
		return map[string]any{
			"audioMessage": map[string]any{
				"mimetype": strings.TrimSpace(input.MimeType),
				"ptt":      true,
			},
			"mediaUrl": strings.TrimSpace(input.URL),
			"mimetype": strings.TrimSpace(input.MimeType),
		}
	default:
		return map[string]any{
			"text": strings.TrimSpace(input.Caption),
		}
	}
}
