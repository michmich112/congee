package config

import "testing"

func TestNIP17RejectGiftWrapWhenDisabled_DefaultTrue(t *testing.T) {
	c := minimalValidConfig()
	if !NIP17RejectGiftWrapWhenDisabled(c) {
		t.Fatal("expected default true when nip17 section unset")
	}
}

func TestNIP17RejectGiftWrapWhenDisabled_ExplicitFalse(t *testing.T) {
	c := minimalValidConfig()
	f := false
	c.NIP17.RejectGiftWrapWhenDisabled = &f
	if NIP17RejectGiftWrapWhenDisabled(c) {
		t.Fatal("expected false when explicitly disabled")
	}
}

func TestNIP17RejectGiftWrapWhenDisabled_IgnoredWhenNIPEnabled(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{1, 11, 17, 42}
	c.NIP42.RelayURL = "wss://relay.example/"
	tv := true
	c.NIP17.RejectGiftWrapWhenDisabled = &tv
	if NIP17RejectGiftWrapWhenDisabled(c) {
		t.Fatal("reject policy must not apply when NIP-17 is enabled")
	}
}
