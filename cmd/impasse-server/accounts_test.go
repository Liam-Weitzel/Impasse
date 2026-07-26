package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// testKey makes a fresh public key, standing in for a different machine.
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

// testUser makes a distinct GitHub user.
var testUserID int64

func testUser() GitHubUser {
	testUserID++
	return GitHubUser{ID: testUserID, Login: "user" + string(rune('a'+testUserID))}
}

// The same GitHub user is the same player every time. That is the rule the
// whole design exists to enforce: one person, one player.
func TestSameGitHubUserIsSameAccount(t *testing.T) {
	a := newAccounts()
	user := testUser()

	first := a.forGitHub(user)
	second := a.forGitHub(user)

	if first != second {
		t.Fatal("the same GitHub user produced two accounts")
	}
	if first.botToken == "" {
		t.Error("account has no bot token")
	}
}

func TestDifferentGitHubUsersAreDifferentAccounts(t *testing.T) {
	a := newAccounts()

	one := a.forGitHub(testUser())
	two := a.forGitHub(testUser())

	if one == two {
		t.Fatal("two GitHub users shared an account")
	}
	if one.botToken == two.botToken {
		t.Error("two accounts shared a bot token")
	}
}

// A renamed GitHub login must follow, since the numeric id is the identity.
func TestLoginIsRefreshed(t *testing.T) {
	a := newAccounts()

	acc := a.forGitHub(GitHubUser{ID: 5, Login: "oldname"})
	again := a.forGitHub(GitHubUser{ID: 5, Login: "newname"})

	if again != acc {
		t.Fatal("a rename made a new account")
	}
	if acc.githubLogin != "newname" {
		t.Errorf("login %q, want it refreshed", acc.githubLogin)
	}
}

// A session token is handed to a renderer over an environment variable, so it
// is worth spending on first use rather than leaving it valid forever.
func TestSessionTokenIsOneShot(t *testing.T) {
	a := newAccounts()
	acc := a.forGitHub(testUser())

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
	acc := a.forGitHub(testUser())
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
// driving it does. A player spectating their own bot has a terminal and a bot
// on one character, and closing the terminal must not remove the character.
func TestCharacterOutlivesOneOfTwoConnections(t *testing.T) {
	a := newAccounts()
	acc := a.forGitHub(testUser())

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

// One of each kind, and no more.
func TestOnlyOneOfEachKindPerAccount(t *testing.T) {
	a := newAccounts()
	acc := a.forGitHub(testUser())

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

func TestDetachFreesTheSlot(t *testing.T) {
	a := newAccounts()
	acc := a.forGitHub(testUser())

	a.attach(acc, kindRenderer)
	a.detach(acc, kindRenderer)

	if _, ok := a.attach(acc, kindRenderer); !ok {
		t.Error("could not reconnect a terminal after disconnecting")
	}
}

func TestPlayerBinding(t *testing.T) {
	a := newAccounts()
	acc := a.forGitHub(testUser())

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

// A player seen again after a restart keeps their token, so bots configured
// before the restart still authenticate.
func TestForGitHubReusesAStoredToken(t *testing.T) {
	stored := map[int64]string{}

	a := newAccounts()
	a.loadToken = func(id int64) (string, bool) {
		t, ok := stored[id]
		return t, ok
	}
	a.saveToken = func(id int64, token string) error {
		stored[id] = token
		return nil
	}

	user := testUser()
	first := a.forGitHub(user)

	if stored[user.ID] != first.botToken {
		t.Fatalf("token was not saved: %v", stored)
	}

	// A fresh registry, as after a restart, with the same storage behind it.
	b := newAccounts()
	b.loadToken = a.loadToken
	b.saveToken = a.saveToken

	second := b.forGitHub(user)

	if second.botToken != first.botToken {
		t.Errorf("token changed across a restart: %q then %q",
			first.botToken, second.botToken)
	}
	if _, _, ok := b.redeem(first.botToken); !ok {
		t.Error("the token from before the restart no longer works")
	}
}
