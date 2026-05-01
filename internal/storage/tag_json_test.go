package storage_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/michmich112/congee/internal/storage"
)

func TestDecodeTagFullJSON(t *testing.T) {
	t.Parallel()
	raw := `["e","6321fa3cc99b25a833babe8a4ed91c52981238379a0f48fef87bc1c5e8852603"]`
	got, err := storage.DecodeTagFullJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"e", "6321fa3cc99b25a833babe8a4ed91c52981238379a0f48fef87bc1c5e8852603"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw array: got %#v want %#v", got, want)
	}

	double, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := storage.DecodeTagFullJSON(string(double))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got2, want) {
		t.Fatalf("wrapped string: got %#v want %#v", got2, want)
	}
}
