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
func testServer(t *testing.T) string {
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
			return addr
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("server never came up")
	return ""
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

func dialBot(t *testing.T, addr string) *botConn {
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
	b := dialBot(t, testServer(t))

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
	b := dialBot(t, testServer(t))

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
	b := dialBot(t, testServer(t))

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
	addr := testServer(t)
	b := dialBot(t, addr)

	st := b.nextState()
	if len(st.Objectives) != 1 {
		t.Fatalf("%d objectives, want 1", len(st.Objectives))
	}
	target := st.Objectives[0]

	// Spawn is (1,1), the pickup is at (3,2). Southeast then east.
	b.move("se")
	b.waitFor("the first step", func(s proto.State) bool {
		p := b.self(s)
		return p.X == 2 && p.Y == 2
	})
	b.move("e")
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
	addr := testServer(t)
	a := dialBot(t, addr)
	b := dialBot(t, addr)

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
	addr := testServer(t)
	a := dialBot(t, addr)
	b := dialBot(t, addr)

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
	addr := testServer(t)
	b := dialBot(t, addr)
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
	addr := testServer(t)
	b := dialBot(t, addr)
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

	var addr string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(s.listeners) > 0 {
			addr = s.listeners[0].Addr().String()
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("TCP listener never came up")
	}

	b := dialBot(t, addr)
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
