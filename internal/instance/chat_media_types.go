package instance

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
)

type chatSearchFilter struct {
	RemoteJID string
	Query     string
}

type chatSearchRecord struct {
	ID            string            `json:"id"`
	PushName      string            `json:"pushName"`
	RemoteJID     string            `json:"remoteJid"`
	Labels        []string          `json:"labels"`
	ProfilePicURL string            `json:"profilePicUrl"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
	InstanceID    string            `json:"instanceId"`
	LastMessage   chatMessageMarker `json:"lastMessage"`
}

type chatMessageMarker struct {
	MessageTimestamp string `json:"messageTimestamp"`
}

type mediaMessageEnvelope struct {
	Number       string                    `json:"number"`
	Type         string                    `json:"type"`
	MediaType    string                    `json:"mediatype"`
	MimeType     string                    `json:"mimetype"`
	Caption      string                    `json:"caption"`
	Media        string                    `json:"media"`
	URL          string                    `json:"url"`
	FileName     string                    `json:"fileName"`
	Filename     string                    `json:"filename"`
	Delay        int32                     `json:"delay"`
	Options      *messageOptionsEnvelope   `json:"options"`
	MediaMessage *nestedMediaMessageFields `json:"mediaMessage"`
}

type nestedMediaMessageFields struct {
	MediaType string `json:"mediatype"`
	MimeType  string `json:"mimetype"`
	Caption   string `json:"caption"`
	Media     string `json:"media"`
	URL       string `json:"url"`
	FileName  string `json:"fileName"`
	Filename  string `json:"filename"`
}

type audioMessageEnvelope struct {
	Number       string                   `json:"number"`
	Delay        int32                    `json:"delay"`
	Options      *messageOptionsEnvelope  `json:"options"`
	AudioMessage *nestedAudioMessageField `json:"audioMessage"`
	Audio        string                   `json:"audio"`
}

type nestedAudioMessageField struct {
	Audio string `json:"audio"`
}

type messageOptionsEnvelope struct {
	Delay int32  `json:"delay"`
	State string `json:"presence"`
}

type resolvedMediaMessageInput struct {
	Number   string
	Type     string
	MimeType string
	Caption  string
	FileName string
	URL      string
	FileData []byte
	Delay    int32
}

type resolvedAudioMessageInput struct {
	Number   string
	FileData []byte
	Delay    int32
}

const emptyChatTimestamp = "0001-01-01T00:00:00Z"

func normalizeChatSearchFilter(input ChatSearchRequest) chatSearchFilter {
	var filter chatSearchFilter

	if value, ok := input.Where["remoteJid"].(string); ok {
		filter.RemoteJID = strings.TrimSpace(value)
	}
	if value, ok := input.Where["query"].(string); ok {
		filter.Query = strings.TrimSpace(value)
	}
	if value, ok := input.Where["search"].(string); ok && filter.Query == "" {
		filter.Query = strings.TrimSpace(value)
	}

	return filter
}

func (m mediaMessageEnvelope) normalize() (*resolvedMediaMessageInput, error) {
	number := strings.TrimSpace(m.Number)
	if number == "" {
		return nil, fmt.Errorf("%w: number is required", domain.ErrValidation)
	}

	delay := m.Delay
	if m.Options != nil && m.Options.Delay > 0 {
		delay = m.Options.Delay
	}

	mediaType := firstNonEmpty(
		m.Type,
		m.MediaType,
		valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.MediaType }),
	)
	mediaType = normalizeMediaKind(mediaType, firstNonEmpty(
		m.MimeType,
		valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.MimeType }),
	))
	if mediaType == "" {
		return nil, fmt.Errorf("%w: media type is required", domain.ErrValidation)
	}

	input := &resolvedMediaMessageInput{
		Number:   number,
		Type:     mediaType,
		MimeType: firstNonEmpty(m.MimeType, valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.MimeType })),
		Caption:  firstNonEmpty(m.Caption, valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.Caption })),
		FileName: firstNonEmpty(m.FileName, m.Filename, valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return firstNonEmpty(v.FileName, v.Filename) })),
		URL:      firstNonEmpty(m.URL, valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.URL })),
		Delay:    delay,
	}

	rawMedia := firstNonEmpty(m.Media, valueOrEmpty(m.MediaMessage, func(v *nestedMediaMessageFields) string { return v.Media }))
	if strings.TrimSpace(rawMedia) == "" && strings.TrimSpace(input.URL) == "" {
		return nil, fmt.Errorf("%w: media or url is required", domain.ErrValidation)
	}

	if strings.TrimSpace(rawMedia) != "" {
		decoded, err := decodeBase64Payload(rawMedia)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid media payload", domain.ErrValidation)
		}
		input.FileData = decoded
	}

	if input.Type == "document" && strings.TrimSpace(input.FileName) == "" {
		input.FileName = "document"
	}

	return input, nil
}

func (a audioMessageEnvelope) normalize() (*resolvedAudioMessageInput, error) {
	number := strings.TrimSpace(a.Number)
	if number == "" {
		return nil, fmt.Errorf("%w: number is required", domain.ErrValidation)
	}

	delay := a.Delay
	if a.Options != nil && a.Options.Delay > 0 {
		delay = a.Options.Delay
	}

	audioPayload := firstNonEmpty(a.Audio, valueOrEmpty(a.AudioMessage, func(v *nestedAudioMessageField) string { return v.Audio }))
	if strings.TrimSpace(audioPayload) == "" {
		return nil, fmt.Errorf("%w: audio payload is required", domain.ErrValidation)
	}

	decoded, err := decodeBase64Payload(audioPayload)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid audio payload", domain.ErrValidation)
	}

	return &resolvedAudioMessageInput{
		Number:   number,
		FileData: decoded,
		Delay:    delay,
	}, nil
}

func decodeBase64Payload(value string) ([]byte, error) {
	payload := strings.TrimSpace(value)
	if payload == "" {
		return nil, fmt.Errorf("empty payload")
	}
	if comma := strings.Index(payload, ","); comma >= 0 {
		payload = payload[comma+1:]
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err == nil {
		return decoded, nil
	}

	return base64.RawStdEncoding.DecodeString(payload)
}

func normalizeMediaKind(rawType, mimeType string) string {
	kind := strings.ToLower(strings.TrimSpace(rawType))
	switch kind {
	case "image", "video", "audio", "document":
		return kind
	case "":
	default:
		return kind
	}

	mime := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case mime != "":
		return "document"
	default:
		return ""
	}
}

func newChatSearchRecord(instanceID, remoteJID, pushName string) chatSearchRecord {
	now := time.Now().UTC().Format(time.RFC3339)
	return chatSearchRecord{
		ID:            remoteJID,
		PushName:      pushName,
		RemoteJID:     remoteJID,
		Labels:        []string{},
		ProfilePicURL: "",
		CreatedAt:     now,
		UpdatedAt:     now,
		InstanceID:    instanceID,
		LastMessage: chatMessageMarker{
			MessageTimestamp: emptyChatTimestamp,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func valueOrEmpty[T any](value *T, getter func(*T) string) string {
	if value == nil {
		return ""
	}
	return getter(value)
}
