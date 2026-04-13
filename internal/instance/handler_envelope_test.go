package instance

import (
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

func TestBuildRuntimeStatusEnvelopeIncludesTopLevelCompatibilityFields(t *testing.T) {
	instance := &repository.Instance{
		ID:               "instance-1",
		Name:             "alpha",
		EngineInstanceID: "engine-1",
		Status:           "connecting",
	}
	state := &repository.RuntimeSessionState{
		Status:         "connecting",
		LastSeenStatus: "connecting",
		LastEventType:  "pairing_started",
		Connected:      false,
		LoggedIn:       false,
	}
	snapshot := &RuntimeSnapshot{
		Status:      "connecting",
		Connected:   false,
		LoggedIn:    false,
		PairingCode: "123-456",
	}

	envelope := buildRuntimeStatusEnvelope(instance, state, snapshot)
	data, ok := envelope["data"].(gin.H)
	if !ok {
		t.Fatalf("expected nested data payload, got %#v", envelope["data"])
	}

	if envelope["instance_id"] != "instance-1" {
		t.Fatalf("expected top-level instance_id, got %#v", envelope["instance_id"])
	}
	if envelope["operator_message"] == "" {
		t.Fatal("expected top-level operator_message")
	}
	if envelope["durable"] == nil {
		t.Fatal("expected top-level durable block")
	}
	if data["instance_id"] != "instance-1" {
		t.Fatalf("expected nested instance_id, got %#v", data["instance_id"])
	}
	if data["live"] == nil {
		t.Fatal("expected nested live block")
	}
}

func TestBuildRuntimeHistoryEnvelopeIncludesTopLevelCompatibilityFields(t *testing.T) {
	instance := &repository.Instance{
		ID:               "instance-1",
		Name:             "alpha",
		EngineInstanceID: "engine-1",
		Status:           "open",
	}
	events := []repository.RuntimeSessionEvent{
		{
			ID:         "evt-1",
			EventType:  "connected",
			Status:     "open",
			OccurredAt: time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
		},
	}

	envelope := buildRuntimeHistoryEnvelope(instance, events)
	data, ok := envelope["data"].(gin.H)
	if !ok {
		t.Fatalf("expected nested data payload, got %#v", envelope["data"])
	}

	if envelope["history_count"] != 1 {
		t.Fatalf("expected top-level history_count=1, got %#v", envelope["history_count"])
	}
	if envelope["operator_message"] == "" {
		t.Fatal("expected top-level operator_message")
	}
	if data["history_count"] != 1 {
		t.Fatalf("expected nested history_count=1, got %#v", data["history_count"])
	}
}

func TestBuildHistoryBackfillEnvelopeIncludesTopLevelCompatibilityFields(t *testing.T) {
	instance := &repository.Instance{
		ID:               "instance-1",
		Name:             "alpha",
		EngineInstanceID: "engine-1",
		Status:           "open",
	}
	result := &HistoryBackfillResult{
		Accepted:        true,
		ChatJID:         "5215512345678@s.whatsapp.net",
		AnchorMessageID: "wamid-1",
		AnchorTimestamp: time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
		Count:           50,
	}

	envelope := buildHistoryBackfillEnvelope(instance, result, "explicit")
	data, ok := envelope["data"].(gin.H)
	if !ok {
		t.Fatalf("expected nested data payload, got %#v", envelope["data"])
	}

	if envelope["action"] != "history_backfill" {
		t.Fatalf("expected top-level action, got %#v", envelope["action"])
	}
	if envelope["accepted"] != true {
		t.Fatalf("expected top-level accepted=true, got %#v", envelope["accepted"])
	}
	if envelope["chat_jid"] != "5215512345678@s.whatsapp.net" {
		t.Fatalf("expected top-level chat_jid, got %#v", envelope["chat_jid"])
	}
	if data["anchor_source"] != "explicit" {
		t.Fatalf("expected nested anchor_source=explicit, got %#v", data["anchor_source"])
	}
}
