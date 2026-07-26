package main

import (
	"strings"
	"testing"
)

// The command has to be runnable as it stands. A placeholder token means every
// player has to edit it first, which is what it used to make them do.
func TestBotCommandCarriesTheRealToken(t *testing.T) {
	got := botCommand("tok-abc123", ":2223", "impasse.example")

	if strings.Contains(got, "<token>") {
		t.Errorf("got %q, want the token rather than a placeholder", got)
	}
	if !strings.Contains(got, "--token tok-abc123") {
		t.Errorf("got %q, want the token in it", got)
	}
}

// ":2223" names a port but not a machine, so a player on another host cannot
// use it. The host they reached the server on fills the gap.
func TestBotCommandFillsInTheHost(t *testing.T) {
	for _, tc := range []struct {
		name, botAddr, host, want string
	}{
		{"bare port", ":2223", "impasse.example", "impasse.example:2223"},
		{"host already set", "bots.example:2223", "impasse.example", "bots.example:2223"},
		{"no host to use", ":2223", "", ":2223"},
		{"ipv6 host", ":2223", "::1", "[::1]:2223"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := botCommand("tok", tc.botAddr, tc.host)
			if !strings.Contains(got, "--address "+tc.want+" ") {
				t.Errorf("got %q, want address %q", got, tc.want)
			}
		})
	}
}

func TestTokenBannerShowsARunnableCommand(t *testing.T) {
	got := tokenBanner("tok-abc123", ":2223", "impasse.example")

	if strings.Contains(got, "<token>") {
		t.Errorf("banner still has a placeholder:\n%s", got)
	}
	if !strings.Contains(got, "--address impasse.example:2223 --token tok-abc123") {
		t.Errorf("banner has no runnable command:\n%s", got)
	}

	// With the bot API off there is no command to give, but the token is still
	// the player's.
	off := tokenBanner("tok-abc123", "", "impasse.example")
	if strings.Contains(off, "bot.py") {
		t.Errorf("offered a command with the bot API off:\n%s", off)
	}
	if !strings.Contains(off, "tok-abc123") {
		t.Errorf("dropped the token:\n%s", off)
	}
}
