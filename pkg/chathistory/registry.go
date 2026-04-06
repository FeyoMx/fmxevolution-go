package chathistory

import (
	"strings"
	"sync"
	"time"
)

type InboundMessage struct {
	InstanceID  string
	MessageID   string
	RemoteJID   string
	PushName    string
	MessageType string
	Body        string
	Source      string
	MediaURL    string
	MimeType    string
	FileName    string
	Caption     string
	Message     map[string]any
	Timestamp   time.Time
}

type InboundMessageListener func(InboundMessage)

var inboundRegistry = struct {
	mu        sync.RWMutex
	listeners []InboundMessageListener
}{}

func RegisterInboundMessageListener(listener InboundMessageListener) {
	if listener == nil {
		return
	}

	inboundRegistry.mu.Lock()
	defer inboundRegistry.mu.Unlock()
	inboundRegistry.listeners = append(inboundRegistry.listeners, listener)
}

func NotifyInboundMessage(message InboundMessage) {
	if strings.TrimSpace(message.InstanceID) == "" ||
		strings.TrimSpace(message.MessageID) == "" ||
		strings.TrimSpace(message.RemoteJID) == "" {
		return
	}

	inboundRegistry.mu.RLock()
	listeners := append([]InboundMessageListener(nil), inboundRegistry.listeners...)
	inboundRegistry.mu.RUnlock()

	for _, listener := range listeners {
		go listener(message)
	}
}
