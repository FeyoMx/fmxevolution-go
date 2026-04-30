package runtimeobs

import (
	"strings"
	"sync"
	"time"
)

type LifecycleEvent struct {
	InstanceID       string
	EventType        string
	EventSource      string
	Status           string
	Connected        bool
	LoggedIn         bool
	PairingActive    bool
	DisconnectReason string
	ErrorMessage     string
	Message          string
	Payload          map[string]any
	OccurredAt       time.Time
}

type LifecycleListener func(LifecycleEvent)

var lifecycleRegistry = struct {
	mu        sync.RWMutex
	listeners []LifecycleListener
	lastSeen  map[string]time.Time
}{}

const lifecycleStatusObservedMinInterval = 15 * time.Second

func RegisterLifecycleListener(listener LifecycleListener) {
	if listener == nil {
		return
	}

	lifecycleRegistry.mu.Lock()
	defer lifecycleRegistry.mu.Unlock()
	lifecycleRegistry.listeners = append(lifecycleRegistry.listeners, listener)
}

func NotifyLifecycleEvent(event LifecycleEvent) {
	if strings.TrimSpace(event.InstanceID) == "" || strings.TrimSpace(event.EventType) == "" {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	event.InstanceID = strings.TrimSpace(event.InstanceID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.EventSource = strings.TrimSpace(event.EventSource)
	event.Status = strings.TrimSpace(event.Status)
	event.DisconnectReason = strings.TrimSpace(event.DisconnectReason)
	event.ErrorMessage = strings.TrimSpace(event.ErrorMessage)
	event.Message = strings.TrimSpace(event.Message)

	if shouldDropDuplicateLifecycleEvent(event) {
		return
	}

	lifecycleRegistry.mu.RLock()
	listeners := append([]LifecycleListener(nil), lifecycleRegistry.listeners...)
	lifecycleRegistry.mu.RUnlock()

	for _, listener := range listeners {
		go listener(event)
	}
}

func shouldDropDuplicateLifecycleEvent(event LifecycleEvent) bool {
	if event.EventType != "status_observed" {
		return false
	}

	key := strings.Join([]string{
		event.InstanceID,
		event.EventType,
		event.Status,
		boolKey(event.Connected),
		boolKey(event.LoggedIn),
		boolKey(event.PairingActive),
		event.DisconnectReason,
		event.ErrorMessage,
	}, "\x00")

	lifecycleRegistry.mu.Lock()
	defer lifecycleRegistry.mu.Unlock()
	if lifecycleRegistry.lastSeen == nil {
		lifecycleRegistry.lastSeen = make(map[string]time.Time)
	}
	if last := lifecycleRegistry.lastSeen[key]; !last.IsZero() && event.OccurredAt.Sub(last) < lifecycleStatusObservedMinInterval {
		return true
	}
	lifecycleRegistry.lastSeen[key] = event.OccurredAt
	return false
}

func boolKey(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
