package relaygolden

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	placeholderEventID   = "<EVENT_ID>"
	placeholderSig       = "<SIG>"
	placeholderCreatedAt = "<CREATED_AT>"
	placeholderChallenge = "<CHALLENGE>"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NormalizeOpts controls which client-supplied fields are preserved during comparison.
type NormalizeOpts struct {
	// StableEventIDs lists event ids from test inputs that must not be rewritten.
	StableEventIDs map[string]struct{}
}

// NormalizeMessage rewrites relay-generated timestamps, signatures, and derived event ids
// per design R9 so golden fixtures stay stable across runs.
func NormalizeMessage(raw []byte, opts NormalizeOpts) ([]byte, error) {
	var msg []json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	if len(msg) == 0 {
		return raw, nil
	}
	var typ string
	if err := json.Unmarshal(msg[0], &typ); err != nil {
		return nil, err
	}
	switch typ {
	case "EVENT":
		if len(msg) >= 3 {
			norm, err := normalizeEventJSON(msg[2], opts)
			if err != nil {
				return nil, err
			}
			msg[2] = norm
		}
	case "OK":
		if len(msg) >= 2 {
			var id string
			if err := json.Unmarshal(msg[1], &id); err == nil {
				if shouldNormalizeID(id, opts) {
					msg[1] = json.RawMessage(`"` + placeholderEventID + `"`)
				}
			}
		}
	case "AUTH":
		if len(msg) >= 2 {
			msg[1] = json.RawMessage(`"` + placeholderChallenge + `"`)
		}
	}
	return json.Marshal(msg)
}

// NormalizeMessages applies NormalizeMessage to each frame in order.
func NormalizeMessages(frames [][]byte, opts NormalizeOpts) ([][]byte, error) {
	out := make([][]byte, len(frames))
	for i, f := range frames {
		n, err := NormalizeMessage(f, opts)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// NormalizeLines joins normalized frames as newline-delimited JSON for fixture comparison.
func NormalizeLines(frames [][]byte, opts NormalizeOpts) (string, error) {
	norm, err := NormalizeMessages(frames, opts)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, line := range norm {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(line)
	}
	return b.String(), nil
}

func normalizeEventJSON(raw json.RawMessage, opts NormalizeOpts) (json.RawMessage, error) {
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		return raw, err
	}
	if id, ok := ev["id"].(string); ok && shouldNormalizeID(id, opts) {
		ev["id"] = placeholderEventID
	}
	if _, ok := ev["sig"]; ok {
		ev["sig"] = placeholderSig
	}
	if _, ok := ev["created_at"]; ok {
		ev["created_at"] = placeholderCreatedAt
	}
	return json.Marshal(ev)
}

func shouldNormalizeID(id string, opts NormalizeOpts) bool {
	if !hex64.MatchString(strings.ToLower(id)) {
		return false
	}
	if opts.StableEventIDs != nil {
		if _, ok := opts.StableEventIDs[id]; ok {
			return false
		}
	}
	return true
}
