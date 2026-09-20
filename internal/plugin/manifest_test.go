package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecPathIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "conduit")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(pkg, "bin", "p")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	man := &Manifest{ID: "conduit", Exec: map[string]string{runtime.GOOS + "_" + runtime.GOARCH: "bin/p"}}
	got, err := man.execPath("conduit")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("exec path must be absolute, got %q", got)
	}
	cmd := exec.Command(got)
	cmd.Dir = "conduit"
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("spawn with pkg Dir: %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("output %q", out)
	}
}

func TestRelativeExecAfterChdirMissesBinary(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "conduit")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "bin", "p"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	rel := filepath.Join("conduit", "bin", "p")
	cmd := exec.Command(rel)
	cmd.Dir = "conduit"
	if err := cmd.Run(); err == nil {
		t.Fatal("relative exec path with Cmd.Dir should fail")
	}
}
