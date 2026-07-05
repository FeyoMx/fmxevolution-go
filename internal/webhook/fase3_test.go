package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

func TestDeliveryPersistedAndRetryScheduledOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := &webhookRepoMock{
		endpoints: []repository.WebhookEndpoint{{
			ID: "ep-1", TenantID: "t1", URL: server.URL, InboundEnabled: true,
		}},
	}
	svc := NewService(repo, nilLogger())

	results, err := svc.DispatchInbound(context.Background(), "t1", DispatchInput{
		EventType: "message.received", InstanceID: "i1", MessageID: "m1",
		Data: map[string]any{"text": "hi"},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(results) != 1 || results[0].Delivered {
		t.Fatalf("expected 1 undelivered result, got %+v", results)
	}
	if len(repo.deliveries) != 1 {
		t.Fatalf("expected 1 persisted delivery, got %d", len(repo.deliveries))
	}
	d := repo.deliveries[0]
	if d.Status != "retrying" || d.NextAttemptAt == nil {
		t.Fatalf("expected retrying with next_attempt set, got status=%s next=%v", d.Status, d.NextAttemptAt)
	}
	if d.EventID == "" {
		t.Fatal("expected event_id set on delivery")
	}
}

func TestDeliveryPersistedOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Evolution-Event-ID") == "" {
			t.Error("expected X-Evolution-Event-ID header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &webhookRepoMock{
		endpoints: []repository.WebhookEndpoint{{ID: "ep-1", TenantID: "t1", URL: server.URL, InboundEnabled: true}},
	}
	svc := NewService(repo, nilLogger())

	_, err := svc.DispatchInbound(context.Background(), "t1", DispatchInput{
		EventType: "message.received", InstanceID: "i1", MessageID: "m1",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(repo.deliveries) != 1 || repo.deliveries[0].Status != "delivered" {
		t.Fatalf("expected 1 delivered record, got %+v", repo.deliveries)
	}
	if repo.deliveries[0].DeliveredAt == nil {
		t.Fatal("expected delivered_at set")
	}
}

func TestInboundDedupeSkipsRepeatEvent(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &webhookRepoMock{
		endpoints: []repository.WebhookEndpoint{{ID: "ep-1", TenantID: "t1", URL: server.URL, InboundEnabled: true}},
		recentHit: true, // simulate: this message was already delivered recently
	}
	svc := NewService(repo, nilLogger())

	results, err := svc.DispatchInbound(context.Background(), "t1", DispatchInput{
		EventType: "message.received", InstanceID: "i1", MessageID: "dup-1",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected dedupe to skip delivery, got %d results", len(results))
	}
	if hits != 0 {
		t.Fatalf("expected no HTTP call on dedupe, got %d", hits)
	}
}

func TestNormalizeEventType(t *testing.T) {
	cases := []struct {
		raw       string
		direction string
		data      map[string]any
		want      string
	}{
		{"messages.upsert", "inbound", nil, EventMessageReceived},
		{"messages.upsert", "inbound", map[string]any{"fromMe": true}, EventMessageSent},
		{"message.sent", "outbound", nil, EventMessageSent},
		{"connection.update", "inbound", map[string]any{"state": "open"}, EventInstanceConnected},
		{"connection.update", "inbound", map[string]any{"state": "close"}, EventInstanceDisconnected},
		{"qrcode.updated", "inbound", nil, EventQRUpdated},
		{"messages.update", "inbound", map[string]any{"ack": "read"}, EventMessageRead},
		{"messages.update", "inbound", map[string]any{"ack": "delivered"}, EventMessageDelivered},
		{"auth.failure", "inbound", nil, EventAuthFailed},
		{"some.custom.event", "inbound", nil, "some.custom.event"},
	}
	for _, tc := range cases {
		if got := NormalizeEventType(tc.raw, tc.direction, tc.data); got != tc.want {
			t.Errorf("NormalizeEventType(%q,%q,%v) = %q, want %q", tc.raw, tc.direction, tc.data, got, tc.want)
		}
	}
}

func TestBackoffCappedAt30m(t *testing.T) {
	if backoffFor(1) != 30*time.Second {
		t.Errorf("attempt 1 want 30s, got %v", backoffFor(1))
	}
	if backoffFor(2) != 60*time.Second {
		t.Errorf("attempt 2 want 60s, got %v", backoffFor(2))
	}
	if backoffFor(20) != 30*time.Minute {
		t.Errorf("large attempt should cap at 30m, got %v", backoffFor(20))
	}
}

func TestScheduleRetryMarksFailedAtMaxAttempts(t *testing.T) {
	svc := NewService(&webhookRepoMock{}, nilLogger())
	d := &repository.WebhookDelivery{MaxAttempts: 3}
	svc.scheduleRetryFields(d, 3)
	if d.Status != "failed" || d.NextAttemptAt != nil {
		t.Fatalf("expected failed with no next attempt, got status=%s next=%v", d.Status, d.NextAttemptAt)
	}
}
