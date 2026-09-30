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
	c.NIPs.Enabled = []int{1, 11, 42}
	c.NIP42.RelayURL = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "nip42.relay_url") {
		t.Fatalf("expected nip42.relay_url error, got %v", err)
	}
	c.NIP42.RelayURL = "wss://relay.example.com/"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNIP42RelayURL(t *testing.T) {
	valid := []string{
		"wss://relay.example.com/",
		"wss://relay.example.com/path",
		"ws://relay.example.com/",
	}
	for _, u := range valid {
		if err := ValidateNIP42RelayURL(u); err != nil {
			t.Fatalf("expected %q valid, got %v", u, err)
		}
	}
	invalid := []string{
		"",
		"https://x.com",
		"relay.example.com",
		"wss:///",
	}
	for _, u := range invalid {
		if err := ValidateNIP42RelayURL(u); err == nil {
			t.Fatalf("expected %q to be invalid, got nil", u)
		}
	}
}

func TestValidateRelayAliases(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{1, 11, 42}
	c.NIP42.RelayURL = "wss://relay.example.com/"
	c.NIP42.RelayAliases = []string{"wss://alias.example.com/", "ws://other.example.com"}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid aliases, got %v", err)
	}
	c.NIP42.RelayAliases = []string{"https://bad.example.com"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "relay_aliases") {
		t.Fatalf("expected relay_aliases error, got %v", err)
	}
}
