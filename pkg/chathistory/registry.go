package chathistory

import (
	"log"
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

// Listeners run on a fixed worker pool instead of one goroutine per message.
// A post-pairing HistorySync delivers hundreds of messages at once; unbounded
// goroutines (each doing DB work) exhausted the 25-connection pool and left an
// instance silently stalled for 7+ hours on 2026-09-09.
const (
	dispatchWorkerCount = 8
	dispatchQueueSize   = 2048
)

type dispatchJob struct {
	listener InboundMessageListener
	message  InboundMessage
}

var dispatchQueue = make(chan dispatchJob, dispatchQueueSize)

func init() {
	for i := 0; i < dispatchWorkerCount; i++ {
		go func() {
			for job := range dispatchQueue {
				job.listener(job.message)
			}
		}()
	}
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
		// Never block the caller: it is whatsmeow's event goroutine, and
		// blocking it would recreate the stall this pool exists to prevent.
		select {
		case dispatchQueue <- dispatchJob{listener: listener, message: message}:
		default:
			log.Printf("[WARN] chathistory dispatch queue full, dropping message %s for instance %s", message.MessageID, message.InstanceID)
		}
	}
}
