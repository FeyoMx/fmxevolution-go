package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/chathistory"
)

type webhookRepoMock struct {
	endpoints []repository.WebhookEndpoint
}

func (m webhookRepoMock) Create(context.Context, *repository.WebhookEndpoint) error {
	return nil
}

func (m webhookRepoMock) GetByID(context.Context, string, string) (*repository.WebhookEndpoint, error) {
	if len(m.endpoints) == 0 {
		return nil, errNotFound()
	}
	return &m.endpoints[0], nil
}

func (m webhookRepoMock) ListByTenant(context.Context, string) ([]repository.WebhookEndpoint, error) {
	return m.endpoints, nil
}

func TestDispatchInboundSignsAndFiltersEndpoints(t *testing.T) {
	var receivedSignature string
	var receivedDirection string
	var receivedEvent string
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Evolution-Signature")
		receivedDirection = r.Header.Get("X-Evolution-Direction")
		receivedEvent = r.Header.Get("X-Evolution-Event-Type")
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	service := NewService(webhookRepoMock{
		endpoints: []repository.WebhookEndpoint{
			{
				ID:             "endpoint-1",
				TenantID:       "tenant-1",
				Name:           "Inbound Hook",
				URL:            server.URL,
				InboundEnabled: true,
				SigningSecret:  "secret",
			},
			{
				ID:              "endpoint-2",
				TenantID:        "tenant-1",
				Name:            "Outbound Only",
				URL:             server.URL,
				InboundEnabled:  false,
				OutboundEnabled: true,
			},
		},
	}, nilLogger())

	results, err := service.DispatchInbound(context.Background(), "tenant-1", DispatchInput{
		EventType:  "message.received",
		InstanceID: "instance-1",
		MessageID:  "message-1",
		Data: map[string]any{
			"text": "hello",
		},
	})
	if err != nil {
		t.Fatalf("dispatch inbound: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 webhook delivery, got %d", len(results))
	}
	if !results[0].Delivered {
		t.Fatal("expected delivery to succeed")
	}
	if receivedSignature == "" {
		t.Fatal("expected signed webhook request")
	}
	if receivedDirection != "inbound" {
		t.Fatalf("expected inbound direction, got %s", receivedDirection)
	}
	if receivedEvent != "message.received" {
		t.Fatalf("expected event header, got %s", receivedEvent)
	}
	if receivedPayload["tenant_id"] != "tenant-1" {
		t.Fatalf("expected tenant payload, got %#v", receivedPayload["tenant_id"])
	}
}

func TestDispatchInboundPublishesConversationFallback(t *testing.T) {
	received := make(chan chathistory.InboundMessage, 1)
	chathistory.RegisterInboundMessageListener(func(message chathistory.InboundMessage) {
		if message.InstanceID == "instance-fallback" && message.MessageID == "message-fallback" {
			select {
			case received <- message:
			default:
			}
		}
	})

	service := NewService(webhookRepoMock{}, nilLogger())

	_, err := service.DispatchInbound(context.Background(), "tenant-1", DispatchInput{
		EventType:  "message.received",
		InstanceID: "instance-fallback",
		MessageID:  "message-fallback",
		Data: map[string]any{
			"remote_jid":   "5215551234567@s.whatsapp.net",
			"push_name":    "Alice",
			"message_type": "conversation",
			"message":      "hello from webhook",
		},
	})
	if err != nil {
		t.Fatalf("dispatch inbound: %v", err)
	}

	select {
	case message := <-received:
		if message.RemoteJID != "5215551234567@s.whatsapp.net" {
			t.Fatalf("unexpected remote jid: %s", message.RemoteJID)
		}
		if message.Body != "hello from webhook" {
			t.Fatalf("unexpected body: %s", message.Body)
		}
		if message.Source != "5215551234567" {
			t.Fatalf("unexpected source: %s", message.Source)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected inbound conversation fallback notification")
	}
}

type webhookNotFound struct{}

func (webhookNotFound) Error() string { return "record not found" }

func errNotFound() error {
	return webhookNotFound{}
}
