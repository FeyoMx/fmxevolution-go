package instance

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"go.mau.fi/whatsmeow/types"
)

type evolutionMarkReadPayload struct {
	Number      string                 `json:"number"`
	RemoteJID   string                 `json:"remoteJid"`
	Participant string                 `json:"participant"`
	Played      bool                   `json:"played"`
	ID          json.RawMessage        `json:"id"`
	Key         *evolutionMarkReadKey  `json:"key"`
	Options     map[string]interface{} `json:"options"`
}

type evolutionMarkReadKey struct {
	ID          string `json:"id"`
	RemoteJID   string `json:"remoteJid"`
	Participant string `json:"participant"`
	FromMe      bool   `json:"fromMe"`
}

func decodeEvolutionMarkReadPayload(reader io.Reader) (MarkReadInput, error) {
	var payload evolutionMarkReadPayload
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&payload); err != nil {
		return MarkReadInput{}, fmt.Errorf("%w: invalid markread payload: %v", domain.ErrValidation, err)
	}

	ids, err := normalizeEvolutionMessageIDs(payload)
	if err != nil {
		return MarkReadInput{}, err
	}

	chatJID, err := normalizeEvolutionChatJID(payload)
	if err != nil {
		return MarkReadInput{}, err
	}

	senderJID, err := normalizeEvolutionSenderJID(payload, chatJID)
	if err != nil {
		return MarkReadInput{}, err
	}

	return MarkReadInput{
		IDs:       ids,
		ChatJID:   chatJID,
		SenderJID: senderJID,
		Played:    payload.Played,
	}, nil
}

func normalizeEvolutionMessageIDs(payload evolutionMarkReadPayload) ([]types.MessageID, error) {
	rawIDs := make([]string, 0, 1)
	if len(payload.ID) > 0 && string(payload.ID) != "null" {
		var list []string
		if err := json.Unmarshal(payload.ID, &list); err == nil {
			rawIDs = append(rawIDs, list...)
		} else {
			var single string
			if err := json.Unmarshal(payload.ID, &single); err != nil {
				return nil, fmt.Errorf("%w: id must be a string or an array of strings", domain.ErrValidation)
			}
			rawIDs = append(rawIDs, single)
		}
	}
	if payload.Key != nil {
		rawIDs = append(rawIDs, payload.Key.ID)
	}

	ids := make([]types.MessageID, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, types.MessageID(id))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: at least one message id is required", domain.ErrValidation)
	}
	return ids, nil
}

func normalizeEvolutionChatJID(payload evolutionMarkReadPayload) (types.JID, error) {
	rawJID := strings.TrimSpace(payload.RemoteJID)
	if rawJID == "" && payload.Key != nil {
		rawJID = strings.TrimSpace(payload.Key.RemoteJID)
	}
	if rawJID != "" {
		return parseEvolutionJID(rawJID, "remoteJid")
	}

	number := strings.TrimSpace(payload.Number)
	if number == "" {
		return types.EmptyJID, fmt.Errorf("%w: remoteJid or number is required", domain.ErrValidation)
	}
	if strings.Contains(number, "@") {
		return parseEvolutionJID(number, "number")
	}

	digits := digitsOnly(number)
	if digits == "" {
		return types.EmptyJID, fmt.Errorf("%w: number must contain digits", domain.ErrValidation)
	}
	return types.NewJID(digits, types.DefaultUserServer), nil
}

func normalizeEvolutionSenderJID(payload evolutionMarkReadPayload, chatJID types.JID) (types.JID, error) {
	rawParticipant := strings.TrimSpace(payload.Participant)
	if rawParticipant == "" && payload.Key != nil {
		rawParticipant = strings.TrimSpace(payload.Key.Participant)
	}
	if rawParticipant == "" {
		if chatJID.Server == types.GroupServer {
			return types.EmptyJID, fmt.Errorf("%w: participant is required for group markread", domain.ErrValidation)
		}
		return types.EmptyJID, nil
	}

	jid, err := parseEvolutionJID(rawParticipant, "participant")
	if err != nil {
		return types.EmptyJID, err
	}
	if jid.Server == types.GroupServer {
		return types.EmptyJID, fmt.Errorf("%w: participant must be a sender JID, not a group JID", domain.ErrValidation)
	}
	return jid, nil
}

func parseEvolutionJID(value, field string) (types.JID, error) {
	jid, err := types.ParseJID(strings.TrimSpace(value))
	if err != nil {
		return types.EmptyJID, fmt.Errorf("%w: invalid %s: %v", domain.ErrValidation, field, err)
	}
	if jid.IsEmpty() || strings.TrimSpace(jid.User) == "" || strings.TrimSpace(jid.Server) == "" {
		return types.EmptyJID, fmt.Errorf("%w: invalid %s", domain.ErrValidation, field)
	}
	switch jid.Server {
	case types.DefaultUserServer, types.GroupServer, types.HiddenUserServer, types.LegacyUserServer, types.MessengerServer:
		return jid, nil
	default:
		return types.EmptyJID, fmt.Errorf("%w: invalid %s server %q", domain.ErrValidation, field, jid.Server)
	}
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
