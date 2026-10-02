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

func TestValidateNIP42RelayAliases(t *testing.T) {
	c := minimalValidConfig()
	c.NIP42.RelayURL = "wss://Relay.EXAMPLE.com/path/"
	c.NIP42.RelayAliases = []string{"wss://Alias.EXAMPLE/extra/"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.NIP42.RelayURL != "wss://relay.example.com/path" {
		t.Fatalf("canonical URL: got %q", c.NIP42.RelayURL)
	}
	if len(c.NIP42.RelayAliases) != 1 || c.NIP42.RelayAliases[0] != "wss://alias.example/extra" {
		t.Fatalf("aliases: %#v", c.NIP42.RelayAliases)
	}

	c.NIP42.RelayAliases = []string{"https://alias.example/"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "nip42.relay_aliases") {
		t.Fatalf("accepted non-WebSocket alias: %v", err)
	}

	c.NIP42.RelayAliases = []string{"wss://relay.example.com/path", "wss://Other.EXAMPLE/a/"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.NIP42.RelayAliases) != 1 || c.NIP42.RelayAliases[0] != "wss://other.example/a" {
		t.Fatalf("duplicate alias should be dropped, got %#v", c.NIP42.RelayAliases)
	}

	c.NIP42.RelayAliases = []string{"  "}
	if err := c.Validate(); err == nil {
		t.Fatal("accepted blank alias")
	}
}

func TestNormalizeNIP42RelayURLKeepsPortPathAndHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"wss://example.com:443/", "wss://example.com:443/"},
		{"ws://example.com:80/path/", "ws://example.com:80/path"},
		{"wss://[::1]:443/path/", "wss://[::1]:443/path"},
		{"wss://münchen.example/", "wss://münchen.example/"},
		{"wss://example.com/foo/../bar", "wss://example.com/foo/../bar"},
		{"wss://Example.COM/Foo/", "wss://example.com/Foo"},
	}
	for _, tc := range cases {
		got, err := NormalizeNIP42RelayURL(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}
