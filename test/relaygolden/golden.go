package relaygolden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AssertGolden compares normalized output to testdata/<name>.golden.
func AssertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	want := strings.TrimRight(string(wantBytes), "\n")
	got = strings.TrimRight(got, "\n")
	if got != want {
		t.Fatalf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
