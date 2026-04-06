package instance

import (
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

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

func TestNormalizeMessageSearchRequestRequiresRemoteJID(t *testing.T) {
	_, err := normalizeMessageSearchRequest(MessageSearchRequest{})
	if err == nil {
		t.Fatal("expected validation error when remoteJid is missing")
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
}
