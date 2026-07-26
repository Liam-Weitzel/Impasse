package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The whole point of the host key is that it survives a restart. If it does
// not, every returning player gets REMOTE HOST IDENTIFICATION HAS CHANGED.
func TestHostKeyIsStableAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host_key")

	first, err := hostKey(path)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}

	second, err := hostKey(path)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}

	a := string(first.PublicKey().Marshal())
	b := string(second.PublicKey().Marshal())
	if a != b {
		t.Error("host key changed between starts, so clients would see a warning")
	}
}

// The key lets anyone holding it impersonate the server.
func TestHostKeyIsWrittenPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host_key")

	if _, err := hostKey(path); err != nil {
		t.Fatalf("generating: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode %o, want 600", perm)
	}
}

// A key that cannot be read has to stop the server. Carrying on with a
// throwaway key would look like it worked, and the warning would only turn up
// later, on someone else's machine.
func TestHostKeyRefusesRubbish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host_key")
	if err := os.WriteFile(path, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := hostKey(path); err == nil {
		t.Error("want an error for a file that is not a host key")
	}
}
