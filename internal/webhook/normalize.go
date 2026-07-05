package webhook

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Canonical event names emitted to consumers (n8n, Chatwoot). Raw engine event
// names are mapped onto these so downstream flows have a stable contract.
const (
	EventMessageReceived     = "message.received"
	EventMessageSent         = "message.sent"
	EventMessageDelivered    = "message.delivered"
	EventMessageRead         = "message.read"
	EventMessageFailed       = "message.failed"
	EventInstanceConnected   = "instance.connected"
	EventInstanceDisconnected = "instance.disconnected"
	EventQRUpdated           = "qr.updated"
	EventAuthFailed          = "auth.failed"
)

// NormalizeEventType maps a raw engine/legacy event name to a canonical name.
// Unknown events pass through unchanged so nothing is silently dropped.
func NormalizeEventType(raw, direction string, data map[string]any) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "-", ".")
	key = strings.ReplaceAll(key, "_", ".")

	switch key {
	case "message", "messages.upsert", "message.upsert", "message.received", "messages.received":
		if direction == "outbound" || isFromMe(data) {
			return EventMessageSent
		}
		return EventMessageReceived
	case "message.sent", "messages.sent", "send.message", "message.send":
		return EventMessageSent
	case "messages.update", "message.update", "message.ack", "messages.ack":
		return statusFromAck(data)
	case "message.delivered", "messages.delivered":
		return EventMessageDelivered
	case "message.read", "messages.read", "read":
		return EventMessageRead
	case "message.failed", "messages.failed", "send.failed":
		return EventMessageFailed
	case "connection.update", "connection", "instance.connected", "connected", "open":
		return connectionState(data)
	case "instance.disconnected", "disconnected", "close", "logout":
		return EventInstanceDisconnected
	case "qr", "qr.updated", "qrcode.updated", "qrcode", "qr.code":
		return EventQRUpdated
	case "auth.failure", "auth.failed", "authentication.failure":
		return EventAuthFailed
	default:
		return key
	}
}

func connectionState(data map[string]any) string {
	state := strings.ToLower(anyString(data["state"]))
	if state == "" {
		state = strings.ToLower(anyString(data["status"]))
	}
	switch state {
	case "open", "connected", "online":
		return EventInstanceConnected
	case "close", "closed", "disconnected", "offline":
		return EventInstanceDisconnected
	default:
		return EventInstanceConnected
	}
}

func statusFromAck(data map[string]any) string {
	ack := strings.ToLower(anyString(data["ack"]))
	if ack == "" {
		ack = strings.ToLower(anyString(data["status"]))
	}
	switch ack {
	case "read", "3", "4":
		return EventMessageRead
	case "delivered", "delivery.ack", "2":
		return EventMessageDelivered
	case "error", "failed", "-1":
		return EventMessageFailed
	default:
		return EventMessageDelivered
	}
}

func isFromMe(data map[string]any) bool {
	for _, key := range []string{"from_me", "fromMe", "isFromMe"} {
		if raw, ok := data[key]; ok {
			if b, ok := raw.(bool); ok {
				return b
			}
		}
	}
	return false
}

// NormalizePayload produces a stable envelope-data shape with normalized fields
// (from, to, message_id, message_type, text, media, timestamp) while preserving
// the original payload under "raw" so nothing is lost for advanced consumers.
func NormalizePayload(input DispatchInput, direction, eventType string) map[string]any {
	data := input.Data
	if data == nil {
		data = map[string]any{}
	}

	payload := normalizedWebhookMessagePayload(data)
	out := map[string]any{
		"provider":     "whatsapp",
		"direction":    direction,
		"instance_id":  strings.TrimSpace(input.InstanceID),
		"message_id":   firstNonEmptyString(strings.TrimSpace(input.MessageID), anyString(data["message_id"]), anyString(data["wamid"])),
		"wamid":        firstNonEmptyString(anyString(data["wamid"]), strings.TrimSpace(input.MessageID)),
		"from":         firstNonEmptyString(anyString(data["from"]), anyString(data["remote_jid"]), anyString(data["remoteJid"])),
		"to":           firstNonEmptyString(anyString(data["to"]), anyString(data["recipient"])),
		"message_type": firstNonEmptyString(anyString(data["message_type"]), anyString(data["messageType"]), inferredWebhookMessageType(payload)),
		"text":         firstNonEmptyString(messageTextFromData(data), messageTextFromData(payload)),
		"timestamp":    timestampFromWebhookInput(data).UTC(),
		"raw":          data,
	}

	if mediaURL := firstNonEmptyString(anyString(data["media_url"]), anyString(data["mediaUrl"]), anyString(payload["media_url"]), anyString(payload["mediaUrl"])); mediaURL != "" {
		out["media"] = map[string]any{
			"url":       mediaURL,
			"mime_type": firstNonEmptyString(anyString(data["mime_type"]), anyString(data["mimeType"]), anyString(payload["mime_type"]), anyString(payload["mimeType"])),
			"file_name": firstNonEmptyString(anyString(data["file_name"]), anyString(data["fileName"]), anyString(payload["file_name"]), anyString(payload["fileName"])),
			"caption":   firstNonEmptyString(anyString(data["caption"]), anyString(payload["caption"])),
		}
	}

	if lat := anyString(data["latitude"]); lat != "" {
		out["location"] = map[string]any{
			"latitude":  data["latitude"],
			"longitude": data["longitude"],
		}
	}

	return out
}

func newEventID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
