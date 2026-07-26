package main

import (
	"strings"
	"testing"
)

// Anyone on the internet can ask for any environment variable, and the renderer
// runs as the server's user. Letting a stranger set LD_PRELOAD hands them the
// dynamic loader.
func TestSafeEnvDropsLoaderControls(t *testing.T) {
	got := safeEnv([]string{
		"LD_PRELOAD=/tmp/evil.so",
		"LD_LIBRARY_PATH=/tmp",
		"LD_AUDIT=/tmp/evil.so",
		"PATH=/tmp",
		"HOME=/tmp",
		"IMPASSE_TOKEN=stolen",
		"BASH_ENV=/tmp/evil",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_GB.UTF-8",
		"LC_ALL=en_GB.UTF-8",
	})

	joined := strings.Join(got, " ")
	for _, banned := range []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT",
		"PATH=", "HOME=", "IMPASSE_TOKEN", "BASH_ENV",
	} {
		if strings.Contains(joined, banned) {
			t.Errorf("%s reached the renderer: %q", banned, joined)
		}
	}

	for _, wanted := range []string{
		"TERM=xterm-256color", "COLORTERM=truecolor",
		"LANG=en_GB.UTF-8", "LC_ALL=en_GB.UTF-8",
	} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("%s was dropped, the renderer needs it: %q", wanted, joined)
		}
	}
}

// A name with no "=" is not a variable, and must not be passed on as one.
func TestSafeEnvIgnoresMalformedEntries(t *testing.T) {
	if got := safeEnv([]string{"TERM", "", "COLORTERM"}); len(got) != 0 {
		t.Errorf("got %q, want nothing", got)
	}
}

// The limit is what keeps the game playable for the people already in when the
// server gets attention.
func TestSessionLimitRefusesWhenFull(t *testing.T) {
	l := &sessionLimit{max: 2}

	if _, ok := l.enter(); !ok {
		t.Fatal("first player refused")
	}
	if _, ok := l.enter(); !ok {
		t.Fatal("second player refused")
	}

	n, ok := l.enter()
	if ok {
		t.Error("third player let in past the limit")
	}
	if n != 2 {
		t.Errorf("reported %d playing, want 2 so the message can say so", n)
	}

	// A place freed up is a place reusable.
	l.leave()
	if _, ok := l.enter(); !ok {
		t.Error("place not reused after someone left")
	}
}

// 0 is the escape hatch for running without a ceiling.
func TestSessionLimitZeroMeansUnlimited(t *testing.T) {
	l := &sessionLimit{max: 0}
	for i := 0; i < 100; i++ {
		if _, ok := l.enter(); !ok {
			t.Fatalf("refused at %d with no limit set", i)
		}
	}
}

// leave must not underflow into a negative count, which would hand out places
// that do not exist.
func TestSessionLimitDoesNotUnderflow(t *testing.T) {
	l := &sessionLimit{max: 1}
	l.leave()
	l.leave()

	if _, ok := l.enter(); !ok {
		t.Fatal("refused when empty")
	}
	if _, ok := l.enter(); ok {
		t.Error("limit exceeded after extra leaves")
	}
}
