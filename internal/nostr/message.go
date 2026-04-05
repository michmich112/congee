package nostr

import (
	"encoding/json"
	"errors"
	"fmt"
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

// ParseMessage parses a NIP-01 client JSON array: EVENT, REQ, or CLOSE.
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
