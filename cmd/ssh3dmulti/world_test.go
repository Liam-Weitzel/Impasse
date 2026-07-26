package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/sascha.l.teichmann/ssh3d/grid"
)

// testWorld writes an ASCII map to a temp file and loads it, which also covers
// loadWorld itself.
func testWorld(t *testing.T, m string) *world {
	t.Helper()

	path := filepath.Join(t.TempDir(), "map.txt")
	if err := os.WriteFile(path, []byte(m), 0o600); err != nil {
		t.Fatalf("writing map: %v", err)
	}

	w, err := loadWorld(path)
	if err != nil {
		t.Fatalf("loading map: %v", err)
	}
	return w
}

const openRoom = "" +
	"#####\n" +
	"#...#\n" +
	"#...#\n" +
	"#...#\n" +
	"#####"

func TestLoadWorldRejectsMissingFile(t *testing.T) {
	if _, err := loadWorld(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("want an error for a missing map")
	}
}

func TestSpawnAssignsDistinctIDs(t *testing.T) {
	w := testWorld(t, openRoom)

	a := w.spawn()
	b := w.spawn()

	if a.id == b.id {
		t.Fatalf("both players got id %d", a.id)
	}
	if !w.g.Walkable(a.pos) || !w.g.Walkable(b.pos) {
		t.Fatalf("spawned into a wall: %v %v", a.pos, b.pos)
	}
	if a.pos == b.pos {
		t.Errorf("both players spawned on %v", a.pos)
	}
}

func TestResolveAppliesQueuedMove(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()
	p.pos = grid.Pos{X: 1, Y: 1}

	w.queue(p.id, grid.SouthEast)
	w.resolve()

	if want := (grid.Pos{X: 2, Y: 2}); p.pos != want {
		t.Fatalf("pos %v, want %v", p.pos, want)
	}
}

// A queued action is consumed by the tick it resolves on. Without this a single
// key press would move the player every tick forever.
func TestResolveClearsQueue(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()
	p.pos = grid.Pos{X: 1, Y: 1}

	w.queue(p.id, grid.East)
	w.resolve()
	after := p.pos

	w.resolve()

	if p.pos != after {
		t.Fatalf("moved again without a new action: %v then %v", after, p.pos)
	}
	if p.queued != grid.None {
		t.Errorf("queue still holds %v", p.queued)
	}
}

// Queuing again before the tick locks replaces the earlier action.
func TestQueueOverwrites(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()
	p.pos = grid.Pos{X: 1, Y: 1}

	w.queue(p.id, grid.East)
	w.queue(p.id, grid.South)
	w.resolve()

	if want := (grid.Pos{X: 1, Y: 2}); p.pos != want {
		t.Fatalf("pos %v, want %v, the first action won", p.pos, want)
	}
}

func TestResolveRefusesMoveIntoWall(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()
	p.pos = grid.Pos{X: 1, Y: 1}

	w.queue(p.id, grid.North)
	w.resolve()

	if want := (grid.Pos{X: 1, Y: 1}); p.pos != want {
		t.Fatalf("pos %v, want %v", p.pos, want)
	}
}

// The corner rule has to hold through the world, not just in the grid package.
func TestResolveRefusesCornerCut(t *testing.T) {
	w := testWorld(t, ""+
		"###\n"+
		"#.#\n"+
		"##.")
	p := w.spawn()
	p.pos = grid.Pos{X: 1, Y: 1}

	w.queue(p.id, grid.SouthEast)
	w.resolve()

	if want := (grid.Pos{X: 1, Y: 1}); p.pos != want {
		t.Fatalf("cut the corner to %v", p.pos)
	}
}

// Every queued action resolves on the same tick, so nobody moves first.
func TestResolveIsSimultaneous(t *testing.T) {
	w := testWorld(t, openRoom)

	a := w.spawn()
	b := w.spawn()
	a.pos = grid.Pos{X: 1, Y: 1}
	b.pos = grid.Pos{X: 3, Y: 3}

	w.queue(a.id, grid.East)
	w.queue(b.id, grid.West)

	before := w.tick
	w.resolve()

	if w.tick != before+1 {
		t.Errorf("tick %d, want %d", w.tick, before+1)
	}
	if want := (grid.Pos{X: 2, Y: 1}); a.pos != want {
		t.Errorf("a at %v, want %v", a.pos, want)
	}
	if want := (grid.Pos{X: 2, Y: 3}); b.pos != want {
		t.Errorf("b at %v, want %v", b.pos, want)
	}
}

// Players stack. Nothing blocks a move onto an occupied cell, including two
// players swapping cells in one tick.
func TestPlayersStack(t *testing.T) {
	w := testWorld(t, openRoom)

	a := w.spawn()
	b := w.spawn()
	a.pos = grid.Pos{X: 1, Y: 1}
	b.pos = grid.Pos{X: 2, Y: 1}

	w.queue(a.id, grid.East)
	w.resolve()

	if a.pos != b.pos {
		t.Fatalf("a at %v, b at %v, should share a cell", a.pos, b.pos)
	}

	w.queue(a.id, grid.East)
	w.queue(b.id, grid.West)
	w.resolve()

	if a.pos == b.pos {
		t.Fatal("players should have passed through each other")
	}
}

func TestQueueForUnknownPlayerIsIgnored(t *testing.T) {
	w := testWorld(t, openRoom)
	w.queue(999, grid.East)
	w.resolve()
}

func TestRemoveDropsPlayer(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()

	w.remove(p.id)

	if len(w.players) != 0 {
		t.Fatalf("%d players left", len(w.players))
	}
	if len(w.state().Players) != 0 {
		t.Error("removed player still in state")
	}
}

func TestWelcomeCarriesMapAndTick(t *testing.T) {
	w := testWorld(t, openRoom)
	p := w.spawn()

	got := w.welcome(p)

	if got.ID != p.id {
		t.Errorf("id %d, want %d", got.ID, p.id)
	}
	if got.TickMS != 600 {
		t.Errorf("tick_ms %d, want 600", got.TickMS)
	}
	if len(got.Map) != w.g.Height() {
		t.Fatalf("map has %d rows, want %d", len(got.Map), w.g.Height())
	}
	// The client parses this back, so it has to survive the round trip.
	if got.Map[1] != "#...#" {
		t.Errorf("row 1 is %q", got.Map[1])
	}
}

func TestStateListsEveryPlayer(t *testing.T) {
	w := testWorld(t, openRoom)
	a := w.spawn()
	b := w.spawn()

	state := w.state()

	if len(state.Players) != 2 {
		t.Fatalf("%d players in state, want 2", len(state.Players))
	}
	seen := map[uint64]bool{}
	for _, p := range state.Players {
		seen[p.ID] = true
	}
	if !seen[a.id] || !seen[b.id] {
		t.Errorf("state is missing a player: %v", state.Players)
	}
}
