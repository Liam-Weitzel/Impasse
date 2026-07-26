package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net"
	"sync"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// An account is one player, and a player is a GitHub account.
//
// SSH keys are not identities. They are optional credentials that point at a
// player, so a second machine reaches the same character and a returning player
// skips signing in again. Somebody with no key at all just signs in each time.
// The rule being enforced is one person one player, and a GitHub account is
// harder to farm than a keypair.
//
// One account owns exactly one character. It is not one connection per
// character: a bot and the terminal its owner is watching from both attach to
// the same one, and whichever queues last before the tick locks is what runs.
type account struct {
	githubID    int64
	githubLogin string
	botToken    string

	// playerID is the character this account owns, valid while attached.
	playerID uint64
	attached bool

	// An account may hold one of each kind at a time: the terminal you play
	// or spectate from, and the bot playing for you. Two terminals on one
	// account is not allowed, so there is never more than one camera and
	// never any question of which view is the real one.
	hasRenderer bool
	hasBot      bool
}

// connKind is which sort of client a connection is, decided by the token it
// presented rather than by anything it claims.
type connKind string

const (
	// kindRenderer is the terminal client, authenticated with the one shot
	// token the SSH server hands its renderer.
	kindRenderer connKind = "terminal"
	// kindBot is a bot, authenticated with the account's long lived token.
	kindBot connKind = "bot"
)

func (a *account) live(kind connKind) bool {
	if kind == kindRenderer {
		return a.hasRenderer
	}
	return a.hasBot
}

func (a *account) setLive(kind connKind, v bool) {
	if kind == kindRenderer {
		a.hasRenderer = v
		return
	}
	a.hasBot = v
}

func (a *account) anyLive() bool {
	return a.hasRenderer || a.hasBot
}

// accounts maps GitHub users and tokens to accounts. It has its own lock rather
// than going through the server command loop, because the SSH handler needs to
// mint a token synchronously while accepting a connection.
type accounts struct {
	mu sync.Mutex

	byGitHub map[int64]*account
	// byToken covers both the long lived bot tokens and the one shot session
	// tokens handed to renderers.
	byToken map[string]*account
	// oneShot marks tokens that are spent on first use.
	oneShot map[string]bool

	// Optional persistence. Function fields rather than a store reference,
	// so this stays testable without a database and does not have to know
	// what a store is.
	loadToken func(githubID int64) (string, bool)
	saveToken func(githubID int64, token string) error
}

func newAccounts() *accounts {
	return &accounts{
		byGitHub: map[int64]*account{},
		byToken:  map[string]*account{},
		oneShot:  map[string]bool{},
	}
}

func newToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and carrying on with a
		// guessable token would be worse than stopping.
		panic("generating token: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// fingerprint identifies a public key. Used only to remember a machine, never
// as an identity.
func fingerprint(key ssh.PublicKey) string {
	return gossh.FingerprintSHA256(key)
}

// forGitHub returns the account for a GitHub user, creating it on first sign in.
func (a *accounts) forGitHub(user GitHubUser) *account {
	a.mu.Lock()
	defer a.mu.Unlock()

	acc := a.byGitHub[user.ID]
	if acc == nil {
		// Reuse the stored token if there is one, so a bot configured
		// before the last restart still works.
		token := ""
		if a.loadToken != nil {
			if stored, ok := a.loadToken(user.ID); ok {
				token = stored
			}
		}
		if token == "" {
			token = newToken()
			if a.saveToken != nil {
				if err := a.saveToken(user.ID, token); err != nil {
					log.Printf("saving bot token for %s: %v\n", user.Login, err)
				}
			}
		}

		acc = &account{
			githubID:    user.ID,
			githubLogin: user.Login,
			botToken:    token,
		}
		a.byGitHub[user.ID] = acc
		a.byToken[token] = acc
	}

	// Logins can be renamed, so keep it current.
	acc.githubLogin = user.Login

	return acc
}

// sessionToken mints a one shot token for a renderer to authenticate with. The
// renderer is a child process, so it cannot sign in itself.
func (a *accounts) sessionToken(acc *account) string {
	token := newToken()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.byToken[token] = acc
	a.oneShot[token] = true

	return token
}

// redeem looks up a token and reports what kind of client it belongs to. One
// shot tokens are consumed, and being one shot is what makes a connection a
// renderer: only the SSH server mints those.
func (a *accounts) redeem(token string) (*account, connKind, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	acc := a.byToken[token]
	if acc == nil {
		return nil, "", false
	}

	kind := kindBot
	if a.oneShot[token] {
		kind = kindRenderer
		delete(a.byToken, token)
		delete(a.oneShot, token)
	}
	return acc, kind, true
}

// botToken returns the account's long lived token, for use by bots.
func (a *accounts) botToken(acc *account) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return acc.botToken
}

// attach records a connection of this kind. It refuses if one is already
// connected, and reports whether this is the first of any kind, meaning a
// character has to be created.
func (a *accounts) attach(acc *account, kind connKind) (first, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if acc.live(kind) {
		return false, false
	}

	first = !acc.anyLive()
	acc.setLive(kind, true)

	return first, true
}

// detach drops a connection and reports whether it was the last, meaning the
// character should go.
func (a *accounts) detach(acc *account, kind connKind) (last bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	acc.setLive(kind, false)

	if !acc.anyLive() {
		acc.attached = false
		return true
	}
	return false
}

func (a *accounts) setPlayer(acc *account, id uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	acc.playerID = id
	acc.attached = true
}

func (a *accounts) player(acc *account) (uint64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return acc.playerID, acc.attached
}

// botCommand is the line a player runs to start a bot, with their own token
// already in it. A command that has to be edited before it works is a command
// people get wrong, and the token is not a secret from the person it belongs
// to.
//
// host is the address the player reached this server on. A bot address of
// ":2223" says which port to dial but not which machine, so the host fills that
// in and the line can be pasted as it stands.
func botCommand(token, botAddr, host string) string {
	address := botAddr

	if h, port, err := net.SplitHostPort(botAddr); err == nil && h == "" && host != "" {
		address = net.JoinHostPort(host, port)
	}

	return "python3 examples/bot.py --address " + address + " --token " + token
}

// hostOf pulls the host out of an address, for filling in a bot command.
func hostOf(a net.Addr) string {
	if a == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return ""
	}
	return host
}

// tokenBanner is what the player sees when they run the token command.
func tokenBanner(token, botAddr, host string) string {
	if botAddr == "" {
		return "" +
			"Your bot token:\n\n  " + token + "\n\n" +
			"The bot API is switched off on this server.\n"
	}
	return "" +
		"Your bot token:\n\n  " + token + "\n\n" +
		"Bots authenticate with it and drive the same character you do, so you\n" +
		"can watch and take over. Whichever queues an action last before the\n" +
		"tick locks is the one that runs.\n\n" +
		"  " + botCommand(token, botAddr, host) + "\n"
}
