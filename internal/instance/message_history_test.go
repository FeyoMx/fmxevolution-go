package instance

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type messageHistoryRepoMock struct {
	items []repository.ConversationMessage
}

func (m *messageHistoryRepoMock) Upsert(_ context.Context, message *repository.ConversationMessage) error {
	if message == nil {
		return nil
	}
	m.items = append(m.items, *message)
	return nil
}

func (m *messageHistoryRepoMock) List(_ context.Context, tenantID, instanceID string, filter repository.ConversationMessageFilter) ([]repository.ConversationMessage, error) {
	result := make([]repository.ConversationMessage, 0, len(m.items))
	for _, item := range m.items {
		if item.TenantID != tenantID || item.InstanceID != instanceID {
			continue
		}
		if filter.RemoteJID != "" && item.RemoteJID != filter.RemoteJID {
			continue
		}
		if filter.ExternalMessageID != "" && item.ExternalMessageID != filter.ExternalMessageID {
			continue
		}
		if filter.Query != "" && !strings.Contains(strings.ToLower(item.Body), strings.ToLower(filter.Query)) {
			continue
		}
		result = append(result, item)
	}
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (m *messageHistoryRepoMock) CountByTenant(context.Context, string) (int64, error) {
	return int64(len(m.items)), nil
}

func (m *messageHistoryRepoMock) MarkReceipt(context.Context, string, string, string, time.Time) error {
	return nil
}

func (m *messageHistoryRepoMock) ListGroups(context.Context, string) ([]repository.GroupSummary, error) {
	return nil, nil
}

func TestNormalizeMessageSearchRequestExtractsLegacyRemoteJID(t *testing.T) {
	filter, err := normalizeMessageSearchRequest(MessageSearchRequest{
		Where: map[string]any{
			"key": map[string]any{
				"remoteJid": "5217712794633@s.whatsapp.net",
			},
			"search": "hola",
		},
		Limit: 500,
	})
	if err != nil {
		t.Fatalf("normalizeMessageSearchRequest returned error: %v", err)
	}

	if filter.RemoteJID != "5217712794633@s.whatsapp.net" {
		t.Fatalf("unexpected remote jid: %s", filter.RemoteJID)
	}
	if filter.Query != "hola" {
		t.Fatalf("unexpected query: %s", filter.Query)
	}
	if filter.Limit != maxMessageSearchLimit {
		t.Fatalf("expected capped limit %d, got %d", maxMessageSearchLimit, filter.Limit)
	}
}

func TestNormalizeMessageSearchRequestAcceptsFrontendAliases(t *testing.T) {
	cases := []MessageSearchRequest{
		{RemoteJID: "5217712794633@s.whatsapp.net"},
		{RemoteJIDAlt: "5217712794633@s.whatsapp.net"},
		{ChatJID: "5217712794633@s.whatsapp.net"},
		{JID: "5217712794633@s.whatsapp.net"},
		{Where: map[string]any{"remote_jid": "5217712794633@s.whatsapp.net"}},
		{Where: map[string]any{"chat_jid": "5217712794633@s.whatsapp.net"}},
		{Where: map[string]any{"key": map[string]any{"remote_jid": "5217712794633@s.whatsapp.net"}}},
		{Where: map[string]any{"key": map[string]any{"chat_jid": "5217712794633@s.whatsapp.net"}}},
	}

	for _, tc := range cases {
		filter, err := normalizeMessageSearchRequest(tc)
		if err != nil {
			t.Fatalf("normalize alias request returned error for %+v: %v", tc, err)
		}
		if filter.RemoteJID != "5217712794633@s.whatsapp.net" {
			t.Fatalf("unexpected remote jid for %+v: %s", tc, filter.RemoteJID)
		}
	}
}

func TestNormalizeMessageSearchRequestRequiresRemoteJID(t *testing.T) {
	_, err := normalizeMessageSearchRequest(MessageSearchRequest{})
	if err == nil {
		t.Fatal("expected validation error when remoteJid is missing")
	}
}

func TestNormalizeMessageSearchRequestRejectsOversizedQuery(t *testing.T) {
	_, err := normalizeMessageSearchRequest(MessageSearchRequest{
		Where: map[string]any{
			"key": map[string]any{
				"remoteJid": "5217712794633@s.whatsapp.net",
			},
			"search": strings.Repeat("x", maxMessageSearchQueryLen+1),
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestToLegacyMessageRecordsMapsConversationHistory(t *testing.T) {
	timestamp := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	items := []repository.ConversationMessage{
		{
			ID:                "db-1",
			InstanceID:        "instance-1",
			RemoteJID:         "5217712794633@s.whatsapp.net",
			ExternalMessageID: "wamid-1",
			Direction:         "outbound",
			MessageType:       "conversation",
			PushName:          "Luis",
			Source:            "5217712794633",
			Body:              "hola mundo",
			MessageTimestamp:  timestamp,
			MessagePayload:    `{"conversation":"hola mundo"}`,
		},
	}

	records := toLegacyMessageRecords(items)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Key.RemoteJID != items[0].RemoteJID {
		t.Fatalf("unexpected key.remoteJid: %s", record.Key.RemoteJID)
	}
	if !record.Key.FromMe {
		t.Fatal("expected outbound record to set fromMe=true")
	}
	if record.MessageType != "conversation" {
		t.Fatalf("unexpected message type: %s", record.MessageType)
	}
	if record.Message["conversation"] != "hola mundo" {
		t.Fatalf("unexpected conversation payload: %#v", record.Message)
	}
	if record.MessageTimestamp != timestamp.Format(time.RFC3339) {
		t.Fatalf("unexpected timestamp: %s", record.MessageTimestamp)
	}
	if record.RemoteJID != items[0].RemoteJID || record.RemoteJIDAlt != items[0].RemoteJID || record.ChatJID != items[0].RemoteJID {
		t.Fatalf("expected frontend jid aliases to be populated, got %+v", record)
	}
	if record.Text != "hola mundo" || record.Body != "hola mundo" || record.Direction != "outbound" || !record.FromMe {
		t.Fatalf("unexpected frontend-compatible fields: %+v", record)
	}
}

func TestSearchMessagesByInstanceIDAndRemoteJIDReturnsEmptyHistory(t *testing.T) {
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "primary"}},
		&messageHistoryRepoMock{},
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	messages, instance, err := service.SearchMessages(context.Background(), "tenant-1", "instance-1", MessageSearchRequest{
		RemoteJID: "5217712794633@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if instance == nil || instance.ID != "instance-1" {
		t.Fatalf("unexpected instance: %+v", instance)
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty history, got %+v", messages)
	}
}

func TestPersistedOutboundTextAppearsInSearchMessages(t *testing.T) {
	history := &messageHistoryRepoMock{}
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "primary"}},
		history,
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	instance := &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "primary"}
	timestamp := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	service.persistOutboundText(context.Background(), "tenant-1", instance, SendTextInput{
		Number: "5217712794633@s.whatsapp.net",
		Text:   "hello from backend",
	}, &SendTextResult{
		MessageID: "wamid-out",
		Chat:      "5217712794633@s.whatsapp.net",
		Timestamp: timestamp,
	})

	messages, _, err := service.SearchMessages(context.Background(), "tenant-1", "instance-1", MessageSearchRequest{
		Where: map[string]any{"key": map[string]any{"remoteJid": "5217712794633@s.whatsapp.net"}},
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", messages)
	}
	if messages[0].ID != "wamid-out" || messages[0].Text != "hello from backend" || !messages[0].FromMe {
		t.Fatalf("unexpected outbound search result: %+v", messages[0])
	}
}

func TestPersistedInboundMessageAppearsInSearchMessages(t *testing.T) {
	timestamp := time.Date(2026, 5, 7, 12, 5, 0, 0, time.UTC)
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "primary"}},
		&messageHistoryRepoMock{items: []repository.ConversationMessage{
			{
				ID:                "db-in",
				TenantID:          "tenant-1",
				InstanceID:        "instance-1",
				RemoteJID:         "5217712794633@s.whatsapp.net",
				ExternalMessageID: "wamid-in",
				Direction:         "inbound",
				MessageType:       "conversation",
				PushName:          "Luis",
				Body:              "inbound hello",
				Status:            "received",
				MessageTimestamp:  timestamp,
				MessagePayload:    `{"conversation":"inbound hello"}`,
			},
		}},
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	messages, _, err := service.SearchMessages(context.Background(), "tenant-1", "instance-1", MessageSearchRequest{
		ChatJID: "5217712794633@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", messages)
	}
	if messages[0].ID != "wamid-in" || messages[0].Text != "inbound hello" || messages[0].FromMe {
		t.Fatalf("unexpected inbound search result: %+v", messages[0])
	}
}
