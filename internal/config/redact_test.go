package config

import "testing"

func TestRedactSecretsForLog(t *testing.T) {
	in := []byte(`{"plugins":{"items":[{"id":"conduit","settings":{"postgres_password":"secret","max_results":10}}]}}`)
	out := RedactSecretsForLog(in)
	if string(out) == string(in) {
		t.Fatal("expected redaction")
	}
	if !containsStr(string(out), `"postgres_password":""`) && !containsStr(string(out), `"postgres_password": ""`) {
		t.Fatalf("password not redacted: %s", out)
	}
	if !containsStr(string(out), "max_results") {
		t.Fatal("non-secret field missing")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
