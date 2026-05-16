package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogStoreErrPrimaryLineJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.DebugLevel)
	op := "TestOperation"
	errTest := errors.New("primary failure")
	LogStoreErr(log, zerolog.WarnLevel, op, errTest, "something failed", nil)

	line := strings.TrimSpace(buf.String())
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("unmarshal: %v body=%q", err, line)
	}
	if obj["level"] != "warn" {
		t.Fatalf("level = %v", obj["level"])
	}
	if obj["operation"] != op {
		t.Fatalf("operation = %v", obj["operation"])
	}
	if obj["error"] == nil {
		t.Fatal("expected error field")
	}
}

func TestLogStoreErrDebugDetailWhenDebugFieldsSet(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.DebugLevel)
	LogStoreErr(log, zerolog.ErrorLevel, "OpX", errors.New("e"), "msg", func(e *zerolog.Event) {
		e.Str("extra_key", "extra_val")
	})

	raw := strings.TrimSpace(buf.String())
	lines := strings.Split(raw, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d: %q", len(lines), raw)
	}

	var primary map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &primary); err != nil {
		t.Fatal(err)
	}
	if primary["level"] != "error" {
		t.Fatalf("primary level = %v", primary["level"])
	}
	if primary["operation"] != "OpX" {
		t.Fatalf("primary operation = %v", primary["operation"])
	}

	var dbg map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &dbg); err != nil {
		t.Fatal(err)
	}
	if dbg["level"] != "debug" {
		t.Fatalf("detail level = %v", dbg["level"])
	}
	if dbg["extra_key"] != "extra_val" {
		t.Fatalf("extra_key = %v", dbg["extra_key"])
	}
	if dbg["message"] != "msg detail" {
		t.Fatalf("detail message = %v", dbg["message"])
	}
}

func TestLogStoreErrNilErrorNoOutput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.DebugLevel)
	LogStoreErr(log, zerolog.WarnLevel, "op", nil, "noop", func(e *zerolog.Event) {
		e.Str("x", "y")
	})
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}
