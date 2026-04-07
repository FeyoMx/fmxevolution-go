package instance

import (
	"context"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type backfillHistoryRepoMock struct {
	items []repository.ConversationMessage
}

func (m backfillHistoryRepoMock) Upsert(context.Context, *repository.ConversationMessage) error {
	return nil
}

func (m backfillHistoryRepoMock) List(_ context.Context, _, _ string, filter repository.ConversationMessageFilter) ([]repository.ConversationMessage, error) {
	if filter.Limit > 0 && len(m.items) > filter.Limit {
		return m.items[:filter.Limit], nil
	}
	return m.items, nil
}

func (m backfillHistoryRepoMock) MarkReceipt(context.Context, string, string, string, time.Time) error {
	return nil
}

func TestResolveHistoryBackfillRequestAcceptsExplicitAnchor(t *testing.T) {
	service := &Service{}
	instance := &repository.Instance{ID: "instance-1", TenantID: "tenant-1"}
	timestamp := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	request, source, err := service.resolveHistoryBackfillRequest(context.Background(), instance, HistoryBackfillInput{
		ChatJID:   "5215512345678@s.whatsapp.net",
		MessageID: "msg-1",
		Timestamp: timestamp,
		Count:     25,
	})
	if err != nil {
		t.Fatalf("resolve explicit anchor: %v", err)
	}
	if source != "explicit" {
		t.Fatalf("expected explicit source, got %s", source)
	}
	if request.ChatJID != "5215512345678@s.whatsapp.net" || request.MessageID != "msg-1" {
		t.Fatalf("unexpected explicit request: %+v", request)
	}
	if !request.Timestamp.Equal(timestamp) {
		t.Fatalf("unexpected timestamp: %v", request.Timestamp)
	}
}

func TestResolveHistoryBackfillRequestUsesStoredHistoryAnchor(t *testing.T) {
	timestamp := time.Date(2026, 4, 6, 11, 30, 0, 0, time.UTC)
	service := &Service{
		history: backfillHistoryRepoMock{
			items: []repository.ConversationMessage{
				{
					RemoteJID:         "5215512345678@s.whatsapp.net",
					ExternalMessageID: "stored-msg",
					MessageTimestamp:  timestamp,
					Direction:         "inbound",
				},
			},
		},
	}
	instance := &repository.Instance{ID: "instance-1", TenantID: "tenant-1"}

	request, source, err := service.resolveHistoryBackfillRequest(context.Background(), instance, HistoryBackfillInput{
		ChatJID: "5215512345678@s.whatsapp.net",
		Count:   80,
	})
	if err != nil {
		t.Fatalf("resolve stored anchor: %v", err)
	}
	if source != "stored_history" {
		t.Fatalf("expected stored_history source, got %s", source)
	}
	if request.MessageID != "stored-msg" {
		t.Fatalf("unexpected anchor message id: %s", request.MessageID)
	}
	if !request.Timestamp.Equal(timestamp) {
		t.Fatalf("unexpected anchor timestamp: %v", request.Timestamp)
	}
	if request.Count != 80 {
		t.Fatalf("unexpected count: %d", request.Count)
	}
}
