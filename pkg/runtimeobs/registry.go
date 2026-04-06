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
}{}

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

	lifecycleRegistry.mu.RLock()
	listeners := append([]LifecycleListener(nil), lifecycleRegistry.listeners...)
	lifecycleRegistry.mu.RUnlock()

	for _, listener := range listeners {
		go listener(event)
	}
}
