package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveClientIDPrefersFlagThenFileThenEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "id")
	if err := os.WriteFile(file, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name            string
		flag, file, env string
		want            string
	}{
		{"flag wins", "from-flag", file, "from-env", "from-flag"},
		{"file beats env", "", file, "from-env", "from-file"},
		{"env is the fallback", "", "", "from-env", "from-env"},
		{"nothing set", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveClientID(tc.flag, tc.file, tc.env)
			if err != nil {
				t.Fatalf("resolveClientID: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A file written by hand ends in a newline, and a client id carrying one fails
// at GitHub with nothing to go on.
func TestResolveClientIDTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "id")
	if err := os.WriteFile(file, []byte("  Ov23li7SeJ0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveClientID("", file, "")
	if err != nil {
		t.Fatalf("resolveClientID: %v", err)
	}
	if got != "Ov23li7SeJ0" {
		t.Errorf("got %q, want it stripped", got)
	}

	if got, err := resolveClientID("", "", " from-env \n"); err != nil || got != "from-env" {
		t.Errorf("env: got %q err %v, want it stripped", got, err)
	}
}

// An unreadable or empty file must stop the server rather than let it come up
// unable to sign anybody in.
func TestResolveClientIDRejectsBadFile(t *testing.T) {
	if _, err := resolveClientID("", filepath.Join(t.TempDir(), "missing"), ""); err == nil {
		t.Error("want an error for a file that is not there")
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("\n \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveClientID("", empty, ""); err == nil {
		t.Error("want an error for an empty file")
	}
}
