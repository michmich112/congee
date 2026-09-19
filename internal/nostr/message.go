package nostr

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Client message types from clients to relay (NIP-01).

type EventMessage struct {
	Event Event
}

type ReqMessage struct {
	SubID   string
	Filters []Filter
}

type CloseMessage struct {
	SubID string
}

// AuthMessage is a client ["AUTH", <signed-event-json>] (NIP-42).
type AuthMessage struct {
	Event Event
}

// NegOpenMessage is ["NEG-OPEN", subID, filter, initialHex] (NIP-77).
type NegOpenMessage struct {
	SubID       string
	Filter      Filter
	InitialHex  string
}

// NegMsgMessage is ["NEG-MSG", subID, messageHex] (NIP-77).
type NegMsgMessage struct {
	SubID      string
	MessageHex string
}

// NegCloseMessage is ["NEG-CLOSE", subID] (NIP-77).
type NegCloseMessage struct {
	SubID string
}

// ParseMessage parses client JSON arrays: EVENT, REQ, CLOSE, AUTH, NEG-* (NIP-77).
func ParseMessage(data []byte) (any, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("nostr: client message: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("nostr: empty client message")
	}
	var typ string
	if err := json.Unmarshal(raw[0], &typ); err != nil {
		return nil, fmt.Errorf("nostr: message type: %w", err)
	}
	switch typ {
	case "EVENT":
		if len(raw) < 2 {
			return nil, errors.New("nostr: EVENT missing payload")
		}
		var ev Event
		if err := json.Unmarshal(raw[1], &ev); err != nil {
			return nil, fmt.Errorf("nostr: EVENT body: %w", err)
		}
		return &EventMessage{Event: ev}, nil
	case "REQ":
		if len(raw) < 3 {
			return nil, errors.New("nostr: REQ missing subscription id or filters")
		}
		var subID string
		if err := json.Unmarshal(raw[1], &subID); err != nil {
			return nil, fmt.Errorf("nostr: REQ sub id: %w", err)
		}
		filters := make([]Filter, 0, len(raw)-2)
		for i := 2; i < len(raw); i++ {
			var f Filter
			if err := json.Unmarshal(raw[i], &f); err != nil {
				return nil, fmt.Errorf("nostr: REQ filter %d: %w", i-2, err)
			}
			filters = append(filters, f)
		}
		return &ReqMessage{SubID: subID, Filters: filters}, nil
	case "CLOSE":
		if len(raw) < 2 {
			return nil, errors.New("nostr: CLOSE missing subscription id")
		}
		var subID string
		if err := json.Unmarshal(raw[1], &subID); err != nil {
			return nil, fmt.Errorf("nostr: CLOSE sub id: %w", err)
		}
		return &CloseMessage{SubID: subID}, nil
	case "AUTH":
		if len(raw) < 2 {
			return nil, errors.New("nostr: AUTH missing signed event")
		}
		payload := bytes.TrimSpace(raw[1])
		if len(payload) == 0 || payload[0] != '{' {
			return nil, errors.New("nostr: client AUTH must carry a signed event JSON object")
		}
		var ev Event
		if err := json.Unmarshal(raw[1], &ev); err != nil {
			return nil, fmt.Errorf("nostr: AUTH event: %w", err)
		}
		return &AuthMessage{Event: ev}, nil
	case "NEG-OPEN":
		if len(raw) < 4 {
			return nil, errors.New("nostr: NEG-OPEN missing sub id, filter, or initial message")
		}
		var subID string
		if err := json.Unmarshal(raw[1], &subID); err != nil {
			return nil, fmt.Errorf("nostr: NEG-OPEN sub id: %w", err)
		}
		var f Filter
		if err := json.Unmarshal(raw[2], &f); err != nil {
			return nil, fmt.Errorf("nostr: NEG-OPEN filter: %w", err)
		}
		var initial string
		if err := json.Unmarshal(raw[3], &initial); err != nil {
			return nil, fmt.Errorf("nostr: NEG-OPEN initial message: %w", err)
		}
		if err := validateNegHex(initial); err != nil {
			return nil, fmt.Errorf("nostr: NEG-OPEN initial message: %w", err)
		}
		return &NegOpenMessage{SubID: subID, Filter: f, InitialHex: strings.ToLower(initial)}, nil
	case "NEG-MSG":
		if len(raw) < 3 {
			return nil, errors.New("nostr: NEG-MSG missing sub id or message")
		}
		var subID string
		if err := json.Unmarshal(raw[1], &subID); err != nil {
			return nil, fmt.Errorf("nostr: NEG-MSG sub id: %w", err)
		}
		var msgHex string
		if err := json.Unmarshal(raw[2], &msgHex); err != nil {
			return nil, fmt.Errorf("nostr: NEG-MSG message: %w", err)
		}
		if err := validateNegHex(msgHex); err != nil {
			return nil, fmt.Errorf("nostr: NEG-MSG message: %w", err)
		}
		return &NegMsgMessage{SubID: subID, MessageHex: strings.ToLower(msgHex)}, nil
	case "NEG-CLOSE":
		if len(raw) < 2 {
			return nil, errors.New("nostr: NEG-CLOSE missing subscription id")
		}
		var subID string
		if err := json.Unmarshal(raw[1], &subID); err != nil {
			return nil, fmt.Errorf("nostr: NEG-CLOSE sub id: %w", err)
		}
		return &NegCloseMessage{SubID: subID}, nil
	default:
		return nil, fmt.Errorf("nostr: unknown message type %q", typ)
	}
}

// PeekClientCommand returns the command string from the first element of a client JSON array
// (e.g. "EVENT", "REQ", "COUNT") without validating the rest of the payload.
func PeekClientCommand(data []byte) (string, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("nostr: client message: %w", err)
	}
	if len(raw) == 0 {
		return "", errors.New("nostr: empty client message")
	}
	var typ string
	if err := json.Unmarshal(raw[0], &typ); err != nil {
		return "", fmt.Errorf("nostr: message type: %w", err)
	}
	return typ, nil
}

// Relay message types from relay to clients.

// RelayEventMessage is ["EVENT", subID, event].
func MarshalRelayEvent(subID string, ev *Event) ([]byte, error) {
	if ev == nil {
		return nil, errors.New("nostr: nil event")
	}
	return json.Marshal([]any{"EVENT", subID, ev})
}

// RelayOKMessage is ["OK", eventID, accepted, message].
func MarshalRelayOK(eventID string, accepted bool, msg string) ([]byte, error) {
	return json.Marshal([]any{"OK", eventID, accepted, msg})
}

// RelayEOSEMessage is ["EOSE", subID].
func MarshalRelayEOSE(subID string) ([]byte, error) {
	return json.Marshal([]any{"EOSE", subID})
}

// RelayClosedMessage is ["CLOSED", subID, message].
func MarshalRelayClosed(subID, msg string) ([]byte, error) {
	return json.Marshal([]any{"CLOSED", subID, msg})
}

// RelayNoticeMessage is ["NOTICE", message].
func MarshalRelayNotice(msg string) ([]byte, error) {
	return json.Marshal([]any{"NOTICE", msg})
}

// MarshalRelayAuth encodes ["AUTH", challenge] from relay to client (NIP-42).
func MarshalRelayAuth(challenge string) ([]byte, error) {
	if challenge == "" {
		return nil, errors.New("nostr: empty AUTH challenge")
	}
	return json.Marshal([]any{"AUTH", challenge})
}

func validateNegHex(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("empty hex")
	}
	if len(s)%2 != 0 {
		return errors.New("hex length must be even")
	}
	if _, err := hex.DecodeString(s); err != nil {
		return err
	}
	return nil
}

// MarshalRelayNegMsg encodes ["NEG-MSG", subID, hexMessage] (NIP-77).
func MarshalRelayNegMsg(subID, msgHex string) ([]byte, error) {
	if err := validateNegHex(msgHex); err != nil {
		return nil, fmt.Errorf("nostr: NEG-MSG: %w", err)
	}
	return json.Marshal([]any{"NEG-MSG", subID, strings.ToLower(msgHex)})
}

// MarshalRelayNegErr encodes ["NEG-ERR", subID, reason] (NIP-77).
func MarshalRelayNegErr(subID, reason string) ([]byte, error) {
	return json.Marshal([]any{"NEG-ERR", subID, reason})
}
