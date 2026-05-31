package config

import (
	"strings"
	"testing"
)

func TestNormalizeNIP42RelayURL(t *testing.T) {
	got, err := NormalizeNIP42RelayURL("wss://Relay.EXAMPLE.com/path/")
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://relay.example.com/path"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	_, err = NormalizeNIP42RelayURL("https://x.com")
	if err == nil {
		t.Fatal("expected error for https")
	}
}

func TestValidateNIP42RelayURLWhenEnabled(t *testing.T) {
	c := minimalValidConfig()
	c.NIP42.Enabled = true
	c.NIP42.RelayURL = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "nip42.relay_url") {
		t.Fatalf("expected nip42.relay_url error, got %v", err)
	}
	c.NIP42.RelayURL = "wss://relay.example.com/"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
