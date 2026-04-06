package sendstatus

import (
	"strings"
	"sync"
	"time"
)

type JobStatus struct {
	JobID             string     `json:"job_id"`
	InstanceID        string     `json:"instance_id"`
	InstanceName      string     `json:"instanceName"`
	Reference         string     `json:"reference"`
	Number            string     `json:"number"`
	Text              string     `json:"text"`
	Status            string     `json:"status"`
	Error             string     `json:"error,omitempty"`
	MessageID         string     `json:"message_id,omitempty"`
	ServerID          int64      `json:"server_id,omitempty"`
	QueuedAt          time.Time  `json:"queued_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	ReadAt            *time.Time `json:"read_at,omitempty"`
	DeliveryConfirmed bool       `json:"delivery_confirmed"`
}

func (j JobStatus) DeliveryStatus() string {
	if j.ReadAt != nil {
		return "read"
	}
	if j.DeliveredAt != nil {
		return "delivered"
	}
	return j.Status
}

func (j JobStatus) Sent() bool {
	switch j.DeliveryStatus() {
	case "sent", "delivered", "read":
		return true
	default:
		return false
	}
}

type record struct {
	tenantID string
	status   JobStatus
}

type registry struct {
	mu           sync.RWMutex
	jobs         map[string]record
	messageIndex map[string]string
}

var global = &registry{
	jobs:         make(map[string]record),
	messageIndex: make(map[string]string),
}

func Store(tenantID string, status JobStatus) {
	global.mu.Lock()
	defer global.mu.Unlock()

	jobKey := tenantJobKey(tenantID, status.JobID)
	if existing, ok := global.jobs[jobKey]; ok {
		status = mergeStatus(existing.status, status)
	}

	global.jobs[jobKey] = record{
		tenantID: tenantID,
		status:   status,
	}

	if status.InstanceID != "" && status.MessageID != "" {
		global.messageIndex[instanceMessageKey(status.InstanceID, status.MessageID)] = jobKey
	}
}

func Load(tenantID, jobID string) (JobStatus, bool) {
	global.mu.RLock()
	defer global.mu.RUnlock()

	record, ok := global.jobs[tenantJobKey(tenantID, jobID)]
	if !ok {
		return JobStatus{}, false
	}

	return record.status, true
}

func MarkReceipt(instanceID, messageID, state string, at time.Time) bool {
	global.mu.Lock()
	defer global.mu.Unlock()

	jobKey, ok := global.messageIndex[instanceMessageKey(instanceID, messageID)]
	if !ok {
		return false
	}

	record, ok := global.jobs[jobKey]
	if !ok {
		return false
	}

	updated := record.status
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "delivered":
		if updated.DeliveredAt == nil || at.After(*updated.DeliveredAt) {
			timestamp := at.UTC()
			updated.DeliveredAt = &timestamp
		}
		updated.DeliveryConfirmed = true
	case "read":
		if updated.DeliveredAt == nil || at.After(*updated.DeliveredAt) {
			deliveredAt := at.UTC()
			updated.DeliveredAt = &deliveredAt
		}
		if updated.ReadAt == nil || at.After(*updated.ReadAt) {
			readAt := at.UTC()
			updated.ReadAt = &readAt
		}
		updated.DeliveryConfirmed = true
	default:
		return false
	}

	record.status = updated
	global.jobs[jobKey] = record
	return true
}

func tenantJobKey(tenantID, jobID string) string {
	return tenantID + "|" + jobID
}

func instanceMessageKey(instanceID, messageID string) string {
	return instanceID + "|" + messageID
}

func mergeStatus(existing, incoming JobStatus) JobStatus {
	merged := incoming

	if merged.Reference == "" {
		merged.Reference = existing.Reference
	}
	if merged.Number == "" {
		merged.Number = existing.Number
	}
	if merged.Text == "" {
		merged.Text = existing.Text
	}
	if merged.MessageID == "" {
		merged.MessageID = existing.MessageID
	}
	if merged.ServerID == 0 {
		merged.ServerID = existing.ServerID
	}
	if merged.QueuedAt.IsZero() {
		merged.QueuedAt = existing.QueuedAt
	}
	if merged.StartedAt == nil {
		merged.StartedAt = existing.StartedAt
	}
	if merged.FinishedAt == nil {
		merged.FinishedAt = existing.FinishedAt
	}
	if merged.DeliveredAt == nil {
		merged.DeliveredAt = existing.DeliveredAt
	}
	if merged.ReadAt == nil {
		merged.ReadAt = existing.ReadAt
	}
	if !merged.DeliveryConfirmed {
		merged.DeliveryConfirmed = existing.DeliveryConfirmed
	}

	return merged
}
