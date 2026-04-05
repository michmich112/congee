package nostr

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
)

func TestEventSerializeForID_Deterministic(t *testing.T) {
	ev := Event{
		PubKey:    "a" + repeat("b", 63), // 64 hex chars
		CreatedAt: 1,
		Kind:      1,
		Tags:      [][]string{{"e", repeat("c", 64)}},
		Content:   "hello\n\"\\",
	}
	b1, err := ev.SerializeForID()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := ev.SerializeForID()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("serialization not deterministic")
	}
}

func repeat(c string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c[0]
	}
	return string(b)
}

func TestEventComputeID_VerifySig_RoundTrip(t *testing.T) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])

	ev := Event{
		PubKey:    pubHex,
		CreatedAt: 1700000000,
		Kind:      1,
		Tags:      [][]string{{"p", pubHex}},
		Content:   "test note",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := ev.VerifyID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.VerifySig(); err != nil {
		t.Fatal(err)
	}
}

func TestEventVerifySig_WrongSig(t *testing.T) {
	priv, _ := btcec.NewPrivateKey()
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])

	ev := Event{
		PubKey:    pubHex,
		CreatedAt: 1,
		Kind:      1,
		Content:   "x",
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	// Corrupt signature hex while keeping length.
	sig := []byte(ev.Sig)
	sig[0] = 'f'
	if sig[0] == ev.Sig[0] {
		sig[0] = 'e'
	}
	ev.Sig = string(sig)
	if err := ev.VerifySig(); err == nil {
		t.Fatal("expected error")
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	ev := Event{
		ID:        repeat("a", 64),
		PubKey:    repeat("b", 64),
		CreatedAt: 42,
		Kind:      7,
		Tags:      [][]string{{"t", "v"}},
		Content:   "{}",
		Sig:       repeat("c", 128),
	}
	b, err := json.Marshal(&ev)
	if err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ev) {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, ev)
	}
}
