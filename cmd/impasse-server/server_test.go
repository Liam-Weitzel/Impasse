package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Liam-Weitzel/Impasse/proto"
)

// These drive the real socket path: listener, handshake, JSON on the wire, the
// reader and writer goroutines, and disconnect cleanup. The world tests call
// the simulation directly and would not notice any of that breaking.

const e2eMap = "" +
	"#######\n" +
	"#S....#\n" +
	"#..*..#\n" +
	"#######"

// testServer starts a server on a temp unix socket with a fast tick, and
// returns its address. It shuts down when the test ends.
func testServer(t *testing.T) (*server, string) {
	t.Helper()

	w := testWorld(t, e2eMap)
	// A 600ms tick would make these tests take minutes.
	w.tickDuration = 15 * time.Millisecond

	// Unix socket paths are short by necessity, so keep this out of the long
	// temp dir name.
	dir, err := os.MkdirTemp("", "impasse")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	addr := "unix:" + filepath.Join(dir, "s.sock")
	s := newServer(w, addr)

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := s.run(done); err != nil {
			t.Errorf("server: %v", err)
		}
	}()

	t.Cleanup(func() {
		close(done)
		<-stopped
	})

	// Wait for the listener rather than sleeping a guessed amount.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		network, a := parseAddr(addr)
		if c, err := net.Dial(network, a); err == nil {
			c.Close()
			return s, addr
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("server never came up")
	return nil, ""
}

// testToken mints a bot token for a fresh account, standing in for a player
// who has run `ssh <host> token`.
func testToken(t *testing.T, s *server) string {
	t.Helper()
	return s.accounts.botToken(s.accounts.forGitHub(testUser()))
}

// testAccount makes a fresh account and returns a token of each kind, which is
// what a player driving a bot from one terminal actually holds.
func testAccount(t *testing.T, s *server) (bot, session string) {
	t.Helper()
	acc := s.accounts.forGitHub(testUser())
	return s.accounts.botToken(acc), s.accounts.sessionToken(acc)
}

// botConn is a minimal client, the same thing a real bot would write.
type botConn struct {
	t   *testing.T
	c   net.Conn
	r   *proto.Reader
	w   *proto.Writer
	id  uint64
	cfg proto.Welcome
}

func dialBot(t *testing.T, s *server, addr string) *botConn {
	t.Helper()
	return dialBotWithToken(t, addr, testToken(t, s))
}

func dialBotWithToken(t *testing.T, addr, token string) *botConn {
	t.Helper()

	// Split rather than net.Dial(parseAddr(addr)). Spreading a two value
	// call into Dial panics the vet hostport analyzer in this Go version.
	network, dialAddr := parseAddr(addr)
	c, err := net.Dial(network, dialAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	b := &botConn{t: t, c: c, r: proto.NewReader(c), w: proto.NewWriter(c)}

	if err := b.w.Write(proto.Auth{Type: proto.TypeAuth, Token: token}); err != nil {
		t.Fatalf("sending auth: %v", err)
	}

	kind, line, err := b.r.Next()
	if err != nil {
		t.Fatalf("reading welcome: %v", err)
	}
	if kind != proto.TypeWelcome {
		t.Fatalf("first message is %q, want welcome", kind)
	}
	if err := json.Unmarshal(line, &b.cfg); err != nil {
		t.Fatalf("decoding welcome: %v", err)
	}
	b.id = b.cfg.ID

	return b
}

func (b *botConn) send(msg proto.Queue) {
	b.t.Helper()
	if err := b.w.Write(msg); err != nil {
		b.t.Fatalf("write: %v", err)
	}
}

func (b *botConn) move(dir string) {
	b.send(proto.Queue{Type: proto.TypeQueue, Action: proto.ActionMove, Dir: dir})
}

func (b *botConn) loot() {
	b.send(proto.Queue{Type: proto.TypeQueue, Action: proto.ActionLoot})
}

// nextState reads one state message.
func (b *botConn) nextState() proto.State {
	b.t.Helper()

	b.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer b.c.SetReadDeadline(time.Time{})

	for {
		kind, line, err := b.r.Next()
		if err != nil {
			b.t.Fatalf("read: %v", err)
		}
		if kind != proto.TypeState {
			continue
		}
		var st proto.State
		if err := json.Unmarshal(line, &st); err != nil {
			b.t.Fatalf("decoding state: %v", err)
		}
		return st
	}
}

// waitFor reads states until cond holds, or gives up.
func (b *botConn) waitFor(what string, cond func(proto.State) bool) proto.State {
	b.t.Helper()
	for i := 0; i < 200; i++ {
		st := b.nextState()
		if cond(st) {
			return st
		}
	}
	b.t.Fatalf("gave up waiting for %s", what)
	return proto.State{}
}

func (b *botConn) self(st proto.State) proto.Player {
	b.t.Helper()
	for _, p := range st.Players {
		if p.ID == b.id {
			return p
		}
	}
	b.t.Fatalf("player %d missing from state", b.id)
	return proto.Player{}
}

func TestWelcomeCarriesTheRules(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)

	if b.cfg.LootTicks != LootTicks {
		t.Errorf("loot_ticks %d, want %d", b.cfg.LootTicks, LootTicks)
	}
	if b.cfg.StunTicks != StunTicks {
		t.Errorf("stun_ticks %d, want %d", b.cfg.StunTicks, StunTicks)
	}
	if b.cfg.StunCooldownTicks != StunCooldown {
		t.Errorf("stun_cooldown_ticks %d, want %d",
			b.cfg.StunCooldownTicks, StunCooldown)
	}
	if b.cfg.StunRadius != StunRadius {
		t.Errorf("stun_radius %d, want %d", b.cfg.StunRadius, StunRadius)
	}
	if len(b.cfg.Map) != 4 {
		t.Fatalf("map has %d rows, want 4", len(b.cfg.Map))
	}
	if b.cfg.Map[1] != "#S....#" {
		t.Errorf("map row 1 is %q", b.cfg.Map[1])
	}
}

func TestBotCanMove(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)

	start := b.self(b.nextState())

	b.move("e")
	st := b.waitFor("the move to resolve", func(s proto.State) bool {
		for _, p := range s.Players {
			if p.ID == b.id && p.X != start.X {
				return true
			}
		}
		return false
	})

	got := b.self(st)
	if got.X != start.X+1 || got.Y != start.Y {
		t.Errorf("moved to (%d,%d) from (%d,%d), want one cell east",
			got.X, got.Y, start.X, start.Y)
	}
}

func TestBotCannotWalkThroughWalls(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)

	start := b.self(b.nextState())

	// North of the spawn is the map border.
	for i := 0; i < 5; i++ {
		b.move("n")
		b.nextState()
	}

	got := b.self(b.nextState())
	if got.X != start.X || got.Y != start.Y {
		t.Errorf("walked to (%d,%d), the wall should have stopped it",
			got.X, got.Y)
	}
}

func TestBotCanLootOverTheWire(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)

	st := b.nextState()
	if len(st.Objectives) != 1 {
		t.Fatalf("%d objectives, want 1", len(st.Objectives))
	}
	target := st.Objectives[0]

	// Spawn is (1,1), the pickup is at (3,2). Movement is orthogonal, so east
	// twice then south.
	for i := 0; i < 2; i++ {
		want := 2 + i
		b.move("e")
		b.waitFor("a step east", func(s proto.State) bool {
			return b.self(s).X == want
		})
	}
	b.move("s")
	b.waitFor("arrival", func(s proto.State) bool {
		p := b.self(s)
		return p.X == target.X && p.Y == target.Y
	})

	b.loot()
	st = b.waitFor("the pickup", func(s proto.State) bool {
		return len(s.Objectives) == 0
	})

	if got := b.self(st).Score; got != 1 {
		t.Errorf("score %d, want 1", got)
	}
}

// Two clients on one server see each other, which is the whole point of the
// shared world.
func TestBotsSeeEachOther(t *testing.T) {
	srv, addr := testServer(t)
	a := dialBot(t, srv, addr)
	b := dialBot(t, srv, addr)

	if a.id == b.id {
		t.Fatalf("both clients got id %d", a.id)
	}

	st := b.waitFor("both players", func(s proto.State) bool {
		return len(s.Players) == 2
	})

	seen := map[uint64]bool{}
	for _, p := range st.Players {
		seen[p.ID] = true
	}
	if !seen[a.id] || !seen[b.id] {
		t.Errorf("state is missing someone: %v", st.Players)
	}
}

// Disconnecting has to drop the player, or the world fills with ghosts. This is
// the bug the old relay had.
func TestDisconnectRemovesThePlayer(t *testing.T) {
	srv, addr := testServer(t)
	a := dialBot(t, srv, addr)
	b := dialBot(t, srv, addr)

	a.waitFor("both players", func(s proto.State) bool {
		return len(s.Players) == 2
	})

	b.c.Close()

	st := a.waitFor("the leaver to vanish", func(s proto.State) bool {
		return len(s.Players) == 1
	})
	if st.Players[0].ID != a.id {
		t.Errorf("wrong player left: %v", st.Players)
	}
}

// Rubbish on the wire must not take the server down or drop the connection.
func TestGarbageIsSurvivable(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)
	b.nextState()

	if _, err := b.c.Write([]byte("{\"type\":\"queue\",\"action\":\"nonsense\"}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := b.c.Write([]byte("{\"type\":\"unknown\"}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := b.c.Write([]byte("{\"type\":\"queue\",\"action\":\"move\",\"dir\":\"up\"}\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Still alive and still ticking.
	b.move("e")
	b.waitFor("the server to keep going", func(s proto.State) bool {
		return s.Tick > 0
	})
}

// A malformed line is a protocol error and closes the connection, rather than
// being silently skipped.
func TestMalformedJSONClosesTheConnection(t *testing.T) {
	srv, addr := testServer(t)
	b := dialBot(t, srv, addr)
	b.nextState()

	if _, err := b.c.Write([]byte("this is not json\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	b.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := b.r.Next(); err != nil {
			return // closed, as expected
		}
	}
}

// TCP is the transport bots actually use, and it has to behave identically.
func TestTCPTransportWorksTheSame(t *testing.T) {
	w := testWorld(t, e2eMap)
	w.tickDuration = 15 * time.Millisecond

	// Port 0 lets the kernel pick a free one.
	s := newServer(w, "127.0.0.1:0")

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := s.run(done); err != nil {
			t.Errorf("server: %v", err)
		}
	}()
	t.Cleanup(func() {
		close(done)
		<-stopped
	})

	// Port 0 means only the server knows what it bound to, so ask it. The
	// command loop runs on the same goroutine as listen and only starts once
	// listen has returned, so this waits for the listener and reads it without
	// touching server state from here.
	bound := make(chan string, 1)
	select {
	case s.cmds <- func(s *server) {
		if len(s.listeners) > 0 {
			bound <- s.listeners[0].Addr().String()
			return
		}
		bound <- ""
	}:
	case <-stopped:
		t.Fatal("server stopped before it was listening")
	case <-time.After(2 * time.Second):
		t.Fatal("TCP listener never came up")
	}

	addr := <-bound
	if addr == "" {
		t.Fatal("server is running with no listeners")
	}

	b := dialBot(t, s, addr)
	start := b.self(b.nextState())

	b.move("e")
	st := b.waitFor("the move", func(s proto.State) bool {
		for _, p := range s.Players {
			if p.ID == b.id && p.X != start.X {
				return true
			}
		}
		return false
	})

	if got := b.self(st); got.X != start.X+1 {
		t.Errorf("moved to %d, want %d", got.X, start.X+1)
	}
}

func TestParseAddr(t *testing.T) {
	for _, tc := range []struct {
		in, network, addr string
	}{
		{"unix:/tmp/x.sock", "unix", "/tmp/x.sock"},
		{"tcp:0.0.0.0:2223", "tcp", "0.0.0.0:2223"},
		{":2223", "tcp", ":2223"},
		{"127.0.0.1:2223", "tcp", "127.0.0.1:2223"},
	} {
		network, addr := parseAddr(tc.in)
		if network != tc.network || addr != tc.addr {
			t.Errorf("%q gave (%q, %q), want (%q, %q)",
				tc.in, network, addr, tc.network, tc.addr)
		}
	}
}

// A connection that does not authenticate gets nothing and is hung up on.
func TestUnauthenticatedConnectionIsRejected(t *testing.T) {
	_, addr := testServer(t)

	network, dialAddr := parseAddr(addr)
	c, err := net.Dial(network, dialAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Skip auth and go straight to playing.
	w := proto.NewWriter(c)
	if err := w.Write(proto.Queue{
		Type: proto.TypeQueue, Action: proto.ActionMove, Dir: "e",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	r := proto.NewReader(c)

	kind, line, err := r.Next()
	if err != nil {
		t.Fatalf("expected an error message, got %v", err)
	}
	if kind != proto.TypeError {
		t.Fatalf("got %q, want an error", kind)
	}
	var e proto.Error
	if err := proto.Decode(line, &e); err != nil {
		t.Fatalf("decoding error: %v", err)
	}
	if e.Message == "" {
		t.Error("error message is empty")
	}

	// And the connection closes rather than lingering.
	if _, _, err := r.Next(); err == nil {
		t.Error("connection stayed open after a rejected auth")
	}
}

func TestBadTokenIsRejected(t *testing.T) {
	_, addr := testServer(t)

	network, dialAddr := parseAddr(addr)
	c, err := net.Dial(network, dialAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := proto.NewWriter(c).Write(proto.Auth{
		Type: proto.TypeAuth, Token: "nonsense",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	kind, _, err := proto.NewReader(c).Next()
	if err != nil {
		t.Fatalf("expected an error message, got %v", err)
	}
	if kind != proto.TypeError {
		t.Errorf("got %q, want an error", kind)
	}
}

// One key owns one character. A bot and a terminal drive it together, which is
// what lets a player watch their own bot and take over from it.
func TestTwoConnectionsShareOneCharacter(t *testing.T) {
	srv, addr := testServer(t)

	botToken, sessionToken := testAccount(t, srv)
	a := dialBotWithToken(t, addr, botToken)
	b := dialBotWithToken(t, addr, sessionToken)

	if a.id != b.id {
		t.Fatalf("same account got characters %d and %d", a.id, b.id)
	}

	st := a.waitFor("a settled state", func(s proto.State) bool {
		return len(s.Players) >= 1
	})
	if len(st.Players) != 1 {
		t.Fatalf("%d characters in the world, want 1", len(st.Players))
	}

	// Either connection can drive it. The second one moves.
	start := a.self(st)
	b.move("e")
	moved := a.waitFor("the move", func(s proto.State) bool {
		for _, p := range s.Players {
			if p.ID == a.id && p.X != start.X {
				return true
			}
		}
		return false
	})
	if got := a.self(moved); got.X != start.X+1 {
		t.Errorf("character at %d, want %d", got.X, start.X+1)
	}
}

// Closing one of two connections leaves the character in play. This is the
// spectator dropping out while their bot keeps going.
func TestCharacterSurvivesOneConnectionLeaving(t *testing.T) {
	srv, addr := testServer(t)

	botToken, sessionToken := testAccount(t, srv)
	a := dialBotWithToken(t, addr, botToken)
	b := dialBotWithToken(t, addr, sessionToken)

	a.waitFor("the character", func(s proto.State) bool {
		return len(s.Players) == 1
	})

	b.c.Close()

	// Still there, and still ours to drive.
	start := a.self(a.nextState())
	a.move("e")
	st := a.waitFor("the move after the other connection left",
		func(s proto.State) bool {
			for _, p := range s.Players {
				if p.ID == a.id && p.X != start.X {
					return true
				}
			}
			return false
		})
	if len(st.Players) != 1 {
		t.Errorf("%d characters, want 1", len(st.Players))
	}
}

// Two different keys are two characters, which is the whole anti multi
// accounting rule: one key, one character.
func TestDifferentKeysGetDifferentCharacters(t *testing.T) {
	srv, addr := testServer(t)

	a := dialBotWithToken(t, addr, testToken(t, srv))
	b := dialBotWithToken(t, addr, testToken(t, srv))

	if a.id == b.id {
		t.Fatalf("two keys shared character %d", a.id)
	}

	st := a.waitFor("both characters", func(s proto.State) bool {
		return len(s.Players) == 2
	})
	if len(st.Players) != 2 {
		t.Errorf("%d characters, want 2", len(st.Players))
	}
}

// dialRejected connects, authenticates, and expects to be turned away.
func dialRejected(t *testing.T, addr, token string) string {
	t.Helper()

	network, dialAddr := parseAddr(addr)
	c, err := net.Dial(network, dialAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	if err := proto.NewWriter(c).Write(proto.Auth{
		Type: proto.TypeAuth, Token: token,
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	kind, line, err := proto.NewReader(c).Next()
	if err != nil {
		t.Fatalf("expected a reply, got %v", err)
	}
	if kind != proto.TypeError {
		t.Fatalf("got %q, want the connection to be refused", kind)
	}
	var e proto.Error
	if err := proto.Decode(line, &e); err != nil {
		t.Fatalf("decoding error: %v", err)
	}
	return e.Message
}

// A second bot on one account is refused. One key, one bot.
func TestSecondBotIsRefused(t *testing.T) {
	srv, addr := testServer(t)

	acc := srv.accounts.forGitHub(testUser())
	token := srv.accounts.botToken(acc)

	dialBotWithToken(t, addr, token)

	if msg := dialRejected(t, addr, token); msg == "" {
		t.Error("refusal came with no reason")
	}
}

// And a second terminal is refused, which is the whole point of the limit:
// one character never has two cameras on it.
func TestSecondRendererIsRefused(t *testing.T) {
	srv, addr := testServer(t)

	acc := srv.accounts.forGitHub(testUser())
	first := srv.accounts.sessionToken(acc)
	second := srv.accounts.sessionToken(acc)

	dialBotWithToken(t, addr, first)

	if msg := dialRejected(t, addr, second); msg == "" {
		t.Error("refusal came with no reason")
	}
}

// Disconnecting frees the slot, so a dropped session can be reconnected.
func TestReconnectAfterDisconnect(t *testing.T) {
	srv, addr := testServer(t)

	acc := srv.accounts.forGitHub(testUser())

	first := dialBotWithToken(t, addr, srv.accounts.sessionToken(acc))
	first.waitFor("the character", func(s proto.State) bool {
		return len(s.Players) == 1
	})
	first.c.Close()

	// Give the server a moment to notice.
	time.Sleep(200 * time.Millisecond)

	second := dialBotWithToken(t, addr, srv.accounts.sessionToken(acc))
	second.waitFor("the character again", func(s proto.State) bool {
		return len(s.Players) == 1
	})
}

// Reconnecting inside one match keeps score and position, over the real socket
// path rather than by calling the world directly. The world tests cover the
// rule; this covers the wiring between a dropped connection, the account, and
// the character that comes back.
func TestRejoinOverTheWireKeepsProgress(t *testing.T) {
	srv, addr := testServer(t)

	// A real account, since progress is remembered per GitHub id and an
	// anonymous zero id deliberately never resumes.
	acc := srv.accounts.forGitHub(GitHubUser{ID: 777, Login: "returning"})

	first := dialBotWithToken(t, addr, srv.accounts.sessionToken(acc))
	st := first.waitFor("the character", func(s proto.State) bool {
		return len(s.Players) == 1
	})

	// Walk somewhere and take the pickup, so there is progress worth losing.
	target := st.Objectives[0]
	for i := 0; i < 2; i++ {
		want := 2 + i
		first.move("e")
		first.waitFor("a step east", func(s proto.State) bool {
			return first.self(s).X == want
		})
	}
	first.move("s")
	first.waitFor("arrival", func(s proto.State) bool {
		p := first.self(s)
		return p.X == target.X && p.Y == target.Y
	})
	first.loot()
	st = first.waitFor("the pickup", func(s proto.State) bool {
		return first.self(s).Score == 1
	})

	before := first.self(st)

	// Drop out, as a lost connection or a Ctrl-C would.
	first.c.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if srv.lobby().Players == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Straight back in, same account.
	again := dialBotWithToken(t, addr, srv.accounts.sessionToken(acc))
	st = again.waitFor("the character again", func(s proto.State) bool {
		return len(s.Players) == 1
	})

	got := again.self(st)
	if got.Score != before.Score {
		t.Errorf("score %d after rejoining, want %d", got.Score, before.Score)
	}
	if got.X != before.X || got.Y != before.Y {
		t.Errorf("came back at (%d,%d), want (%d,%d)",
			got.X, got.Y, before.X, before.Y)
	}
}
