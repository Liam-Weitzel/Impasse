package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// testKey makes a fresh public key, standing in for a different player each
// time it is called.
func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	key, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("wrapping key: %v", err)
	}
	return key
}

// The same key is the same account every time, which is what makes a public key
// usable as an identity with no registration step.
func TestSameKeyIsSameAccount(t *testing.T) {
	a := newAccounts()
	key := testKey(t)

	first := a.forKey(key)
	second := a.forKey(key)

	if first != second {
		t.Fatal("the same key produced two accounts")
	}
	if first.botToken == "" {
		t.Error("account has no bot token")
	}
}

func TestDifferentKeysAreDifferentAccounts(t *testing.T) {
	a := newAccounts()

	one := a.forKey(testKey(t))
	two := a.forKey(testKey(t))

	if one == two {
		t.Fatal("two keys shared an account")
	}
	if one.botToken == two.botToken {
		t.Error("two accounts shared a bot token")
	}
}

// A session token is handed to a renderer over an environment variable, so it
// is worth spending on first use rather than leaving it valid forever.
func TestSessionTokenIsOneShot(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))

	token := a.sessionToken(acc)

	got, kind, ok := a.redeem(token)
	if !ok || got != acc {
		t.Fatal("session token did not redeem")
	}
	if kind != kindRenderer {
		t.Errorf("session token gave kind %q, want a renderer", kind)
	}
	if _, _, ok := a.redeem(token); ok {
		t.Error("session token redeemed twice")
	}
}

// A bot token is long lived, because a bot restarts far more often than a
// player fetches a new one.
func TestBotTokenIsReusable(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))
	token := a.botToken(acc)

	for i := 0; i < 3; i++ {
		got, kind, ok := a.redeem(token)
		if !ok || got != acc {
			t.Fatalf("bot token failed on use %d", i+1)
		}
		if kind != kindBot {
			t.Errorf("bot token gave kind %q, want a bot", kind)
		}
	}
}

func TestUnknownTokenIsRejected(t *testing.T) {
	a := newAccounts()
	if _, _, ok := a.redeem("not-a-real-token"); ok {
		t.Fatal("a made up token was accepted")
	}
}

func TestTokensAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		token := newToken()
		if seen[token] {
			t.Fatalf("newToken repeated %q", token)
		}
		seen[token] = true
	}
}

// The character belongs to the account and only goes when the last connection
// driving it does. A player spectating their own bot has a renderer and a bot
// on one character, and closing the terminal must not remove the character.
func TestCharacterOutlivesOneOfTwoConnections(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))

	first, ok := a.attach(acc, kindBot)
	if !ok || !first {
		t.Fatal("the first connection should create the character")
	}
	first, ok = a.attach(acc, kindRenderer)
	if !ok {
		t.Fatal("a renderer should be allowed alongside a bot")
	}
	if first {
		t.Fatal("the second connection should join the existing character")
	}

	if last := a.detach(acc, kindRenderer); last {
		t.Fatal("dropping the terminal removed the character from under the bot")
	}
	if last := a.detach(acc, kindBot); !last {
		t.Fatal("dropping the last connection should remove the character")
	}
}

// One of each kind, and no more. Two terminals on one account would mean two
// cameras on one character.
func TestOnlyOneOfEachKindPerAccount(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))

	if _, ok := a.attach(acc, kindRenderer); !ok {
		t.Fatal("the first terminal was refused")
	}
	if _, ok := a.attach(acc, kindRenderer); ok {
		t.Error("a second terminal was allowed on one account")
	}

	if _, ok := a.attach(acc, kindBot); !ok {
		t.Fatal("a bot was refused alongside a terminal")
	}
	if _, ok := a.attach(acc, kindBot); ok {
		t.Error("a second bot was allowed on one account")
	}
}

// Disconnecting frees the slot, so reconnecting after a dropped session works.
func TestDetachFreesTheSlot(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))

	a.attach(acc, kindRenderer)
	a.detach(acc, kindRenderer)

	if _, ok := a.attach(acc, kindRenderer); !ok {
		t.Error("could not reconnect a terminal after disconnecting")
	}
}

func TestPlayerBinding(t *testing.T) {
	a := newAccounts()
	acc := a.forKey(testKey(t))

	if _, ok := a.player(acc); ok {
		t.Error("reported a character before one was attached")
	}

	a.setPlayer(acc, 7)

	id, ok := a.player(acc)
	if !ok || id != 7 {
		t.Errorf("got (%d, %v), want (7, true)", id, ok)
	}
}

func TestFingerprintIsStable(t *testing.T) {
	key := testKey(t)
	if fingerprint(key) != fingerprint(key) {
		t.Error("fingerprint is not stable for one key")
	}
	if fingerprint(key) == fingerprint(testKey(t)) {
		t.Error("two keys share a fingerprint")
	}
}
