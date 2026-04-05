package nostr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// Event is a NIP-01 nostr event on the wire.
type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// SerializeForID returns the UTF-8 JSON serialization used for event ID (NIP-01).
func (e *Event) SerializeForID() ([]byte, error) {
	tags := e.Tags
	if tags == nil {
		tags = [][]string{}
	}
	payload := []any{0, e.PubKey, e.CreatedAt, e.Kind, tags, e.Content}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	// Encode adds a trailing newline; NIP-01 serialization must not.
	out := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	return out, nil
}

// ComputeID sets e.ID to the lowercase hex SHA-256 of SerializeForID.
func (e *Event) ComputeID() (string, error) {
	ser, err := e.SerializeForID()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(ser)
	id := hex.EncodeToString(sum[:])
	e.ID = id
	return id, nil
}

// VerifyID checks that id matches the serialized hash.
func (e *Event) VerifyID() error {
	ser, err := e.SerializeForID()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(ser)
	want := hex.EncodeToString(sum[:])
	if e.ID != want {
		return fmt.Errorf("nostr: id mismatch (want %s, got %s)", want, e.ID)
	}
	return nil
}

// VerifySig checks Schnorr signature over the 32-byte event id hash (NIP-01).
func (e *Event) VerifySig() error {
	if err := e.VerifyID(); err != nil {
		return err
	}
	idBytes, err := hex.DecodeString(e.ID)
	if err != nil {
		return fmt.Errorf("nostr: decode id: %w", err)
	}
	if len(idBytes) != 32 {
		return errors.New("nostr: id must be 32 bytes hex")
	}
	sigBytes, err := hex.DecodeString(e.Sig)
	if err != nil {
		return fmt.Errorf("nostr: decode sig: %w", err)
	}
	sig, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("nostr: parse sig: %w", err)
	}
	pubkeyBytes, err := hex.DecodeString(e.PubKey)
	if err != nil {
		return fmt.Errorf("nostr: decode pubkey: %w", err)
	}
	if len(pubkeyBytes) != 32 {
		return errors.New("nostr: pubkey must be 32 bytes (x-only) hex")
	}
	pubkey, err := schnorr.ParsePubKey(pubkeyBytes)
	if err != nil {
		return fmt.Errorf("nostr: parse pubkey: %w", err)
	}
	if !sig.Verify(idBytes, pubkey) {
		return errors.New("nostr: invalid signature")
	}
	return nil
}

// Sign computes the id and sets sig using the given key (Schnorr BIP-340).
func (e *Event) Sign(priv *btcec.PrivateKey) error {
	id, err := e.ComputeID()
	if err != nil {
		return err
	}
	idBytes, err := hex.DecodeString(id)
	if err != nil {
		return err
	}
	sig, err := schnorr.Sign(priv, idBytes)
	if err != nil {
		return err
	}
	e.Sig = hex.EncodeToString(sig.Serialize())
	return nil
}
