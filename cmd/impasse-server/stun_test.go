package main

import (
	"testing"

	"github.com/Liam-Weitzel/Impasse/grid"
)

// A room wide enough to step in and out of stun range.
const stunRoom = "" +
	"#########\n" +
	"#.......#\n" +
	"#.......#\n" +
	"#.......#\n" +
	"#########"

func stunWorld(t *testing.T) (*world, *player, *player) {
	t.Helper()
	w := testWorld(t, stunRoom)
	a := w.join()
	b := w.join()
	a.pos = grid.Pos{X: 2, Y: 2}
	b.pos = grid.Pos{X: 3, Y: 2}
	return w, a, b
}

// One tick of startup, then the victim loses exactly StunTicks.
func TestStunLandsAfterStartup(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.resolve()

	if b.stunned != 0 {
		t.Fatalf("victim stunned %d during the startup tick, want 0", b.stunned)
	}

	w.resolve()
	if b.stunned != StunTicks-1 {
		t.Fatalf("stunned %d on landing, want %d", b.stunned, StunTicks-1)
	}

	// Count the ticks the victim actually loses.
	lost := 1 // the landing tick
	for i := 0; i < 6; i++ {
		w.queueMove(b.id, grid.East)
		before := b.pos
		w.resolve()
		if b.pos == before {
			lost++
		} else {
			break
		}
	}
	if lost != StunTicks {
		t.Errorf("victim lost %d ticks, want %d", lost, StunTicks)
	}
}

// The burst catches the whole 3x3, not just one target.
func TestStunHitsEveryoneInRange(t *testing.T) {
	w := testWorld(t, stunRoom)
	caster := w.join()
	caster.pos = grid.Pos{X: 2, Y: 2}

	var near []*player
	for _, d := range grid.Directions {
		p := w.join()
		delta := d.Delta()
		p.pos = grid.Pos{X: caster.pos.X + delta.X, Y: caster.pos.Y + delta.Y}
		near = append(near, p)
	}

	far := w.join()
	far.pos = grid.Pos{X: 5, Y: 2}

	w.queueStun(caster.id)
	w.resolve()
	w.resolve()

	for i, p := range near {
		if p.stunned == 0 {
			t.Errorf("neighbour %d at %v was not stunned", i, p.pos)
		}
	}
	if far.stunned != 0 {
		t.Errorf("player two cells away was stunned")
	}
	if caster.stunned != 0 {
		t.Error("caster stunned itself")
	}
}

// Range is measured when the button goes down. Running away during startup
// does not help.
func TestStunStillLandsIfTheTargetFlees(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.queueMove(b.id, grid.East)
	w.resolve()

	if b.pos.X != 4 {
		t.Fatalf("victim at %v, expected to have fled east", b.pos)
	}

	w.resolve()

	if b.stunned == 0 {
		t.Error("victim escaped a burst that was already cast")
	}
}

// Someone who walks into range during startup was not there at cast time, so
// they are not caught.
func TestStunMissesSomeoneWhoArrivesLate(t *testing.T) {
	w := testWorld(t, stunRoom)
	caster := w.join()
	caster.pos = grid.Pos{X: 2, Y: 2}

	late := w.join()
	late.pos = grid.Pos{X: 5, Y: 2}

	w.queueStun(caster.id)
	w.queueMove(late.id, grid.West)
	w.resolve()
	w.resolve()

	if late.stunned != 0 {
		t.Error("caught someone who was out of range when the burst was cast")
	}
}

// A stun wipes loot progress completely.
func TestStunResetsTheChannel(t *testing.T) {
	w := testWorld(t, "#####\n#S*.#\n#...#\n#####")
	victim := w.join()
	caster := w.join()
	victim.pos = grid.Pos{X: 2, Y: 1}
	caster.pos = grid.Pos{X: 3, Y: 1}

	w.queueLoot(victim.id)
	w.resolve()
	w.resolve()
	if victim.channel != 2 {
		t.Fatalf("channel %d, want 2", victim.channel)
	}

	w.queueStun(caster.id)
	w.resolve() // startup
	w.resolve() // lands

	if victim.channel != 0 {
		t.Errorf("channel %d after a stun, want 0", victim.channel)
	}
	if victim.score != 0 {
		t.Errorf("victim collected anyway")
	}
	if len(w.objectives) != 1 {
		t.Errorf("pickup was taken")
	}
}

// Casting drops the caster's own channel. This is what makes attacking into a
// standoff lose the pickup.
func TestCastingGivesUpYourOwnChannel(t *testing.T) {
	w := testWorld(t, "#####\n#S*.#\n#...#\n#####")
	a := w.join()
	b := w.join()
	a.pos = grid.Pos{X: 2, Y: 1}
	b.pos = grid.Pos{X: 2, Y: 1}

	// Both reach a full channel and deadlock.
	w.queueLoot(a.id)
	w.queueLoot(b.id)
	for i := 0; i < LootTicks+1; i++ {
		w.resolve()
	}
	if a.channel != LootTicks || b.channel != LootTicks {
		t.Fatalf("channels %d and %d, expected a standoff", a.channel, b.channel)
	}

	// a tries to break it by attacking. That drops a's channel, leaving b
	// as the only finisher on this very tick, a full tick before the burst
	// can land.
	w.queueStun(a.id)
	w.resolve()

	if b.score != 1 {
		t.Errorf("b score %d, want 1. Attacking should hand the pickup over", b.score)
	}
	if a.score != 0 {
		t.Errorf("a score %d, want 0", a.score)
	}
}

// The cooldown must outlast the stun, or one attacker locks a victim out for
// good. Cast at T lands T+1 and holds through T+2, so the next cast has to land
// no earlier than T+4 for the victim to get T+3.
//
// The attacker is pinned next to the victim every tick. Without that the victim
// simply walks out of the 3x3 and the test proves nothing.
func TestVictimGetsAFreeTickBetweenStuns(t *testing.T) {
	w, a, b := stunWorld(t)

	const ticks = 30
	acted := 0

	for i := 0; i < ticks; i++ {
		// Attacker glued to the victim, casting whenever it can.
		a.pos = b.pos
		// resolve() increments the tick first, so the cast is judged
		// against w.tick+1. Gating on w.tick alone makes the attacker
		// idle a tick per cycle and hides the real lock rate.
		if w.tick+1 >= a.stunReady {
			w.queueStun(a.id)
		}

		// Measure by whether the move actually happened. Sampling stunned
		// before resolve would miss the landing tick, since the burst lands
		// inside resolve.
		if b.pos.X > 2 {
			w.queueMove(b.id, grid.West)
		} else {
			w.queueMove(b.id, grid.East)
		}

		before := b.pos
		w.resolve()
		if b.pos != before {
			acted++
		}
	}

	if acted == 0 {
		t.Fatal("victim never acted, a single attacker can lock someone out")
	}

	// One free tick per cooldown cycle is what the numbers predict.
	want := ticks / StunCooldown
	if acted < want-2 || acted > want+2 {
		t.Errorf("victim acted on %d of %d ticks, expected about %d",
			acted, ticks, want)
	}
	t.Logf("victim acted on %d of %d ticks", acted, ticks)
}

// Spelled out, so the arithmetic behind StunCooldown cannot drift unnoticed.
func TestStunLockLeavesExactlyOneFreeTick(t *testing.T) {
	w, a, b := stunWorld(t)

	// Cast, land, hold, then cast again the moment the cooldown allows.
	free := []uint64{}
	for i := 0; i < 15; i++ {
		a.pos = b.pos
		// resolve() increments the tick first, so the cast is judged
		// against w.tick+1. Gating on w.tick alone makes the attacker
		// idle a tick per cycle and hides the real lock rate.
		if w.tick+1 >= a.stunReady {
			w.queueStun(a.id)
		}

		if b.pos.X > 2 {
			w.queueMove(b.id, grid.West)
		} else {
			w.queueMove(b.id, grid.East)
		}

		before := b.pos
		w.resolve()
		if b.pos != before {
			free = append(free, w.tick)
		}
	}

	if len(free) < 3 {
		t.Fatalf("only %d free ticks in 12, want the victim to breathe", len(free))
	}

	// Free ticks should be StunCooldown apart once the cycle settles.
	gap := free[len(free)-1] - free[len(free)-2]
	if gap != StunCooldown {
		t.Errorf("gap between free ticks is %d, want %d", gap, StunCooldown)
	}
}

func TestStunRespectsCooldown(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.resolve()
	castTick := w.tick

	// Try to cast every tick. Only the ones off cooldown should schedule.
	casts := 0
	for i := 0; i < StunCooldown; i++ {
		w.queueStun(a.id)
		before := len(w.pending)
		w.resolve()
		if len(w.pending) > before {
			casts++
		}
	}

	if casts != 1 {
		t.Errorf("%d casts inside the cooldown window, want 1", casts)
	}
	if a.stunReady <= castTick {
		t.Errorf("stunReady %d is not ahead of the cast tick %d", a.stunReady, castTick)
	}
	_ = b
}

// A stunned player cannot cast, so being burst first wins the exchange.
func TestStunnedPlayersCannotAct(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.resolve()
	w.resolve() // b is stunned now

	if b.stunned == 0 {
		t.Fatal("b should be stunned")
	}

	w.queueStun(b.id)
	before := len(w.pending)
	w.resolve()

	if len(w.pending) != before {
		t.Error("a stunned player managed to cast")
	}
}

// Casting with nobody nearby still burns the cooldown, so it is not free. The
// burst is still recorded, with no targets, so that it telegraphs like any
// other and a wind up cannot be read as proof that someone is in range.
func TestCastingAtNothingStillCosts(t *testing.T) {
	w := testWorld(t, stunRoom)
	a := w.join()
	a.pos = grid.Pos{X: 2, Y: 2}

	w.queueStun(a.id)
	w.resolve()

	if a.stunReady <= w.tick {
		t.Errorf("stunReady %d, want it ahead of tick %d", a.stunReady, w.tick)
	}
	if len(w.pending) != 1 {
		t.Fatalf("%d pending bursts, want the wind up recorded", len(w.pending))
	}
	if len(w.pending[0].targets) != 0 {
		t.Errorf("%d targets, want none", len(w.pending[0].targets))
	}

	// And it clears itself on landing rather than piling up.
	w.resolve()
	if len(w.pending) != 0 {
		t.Errorf("%d pending bursts after landing, want 0", len(w.pending))
	}
}

// A player who leaves between the cast and the landing must not crash the
// server or resurrect anything.
func TestStunSurvivesTargetLeaving(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.resolve()
	w.remove(b.id)
	w.resolve()

	if len(w.pending) != 0 {
		t.Error("pending stun was not cleared")
	}
}

// A burst in flight is visible to everyone for its startup tick, then gone.
// It is already cast and lands regardless, so publishing it reveals no intent.
func TestCastingIsVisibleDuringStartup(t *testing.T) {
	w, a, b := stunWorld(t)

	casting := func(id uint64) int {
		for _, p := range w.state().Players {
			if p.ID == id {
				return p.Casting
			}
		}
		t.Fatalf("player %d missing from state", id)
		return 0
	}

	if casting(a.id) != 0 {
		t.Fatal("casting before anything was cast")
	}

	w.queueStun(a.id)
	w.resolve()

	if got := casting(a.id); got != StunStartup {
		t.Errorf("casting %d during startup, want %d", got, StunStartup)
	}
	if casting(b.id) != 0 {
		t.Error("the victim is reported as casting")
	}

	w.resolve() // lands

	if got := casting(a.id); got != 0 {
		t.Errorf("casting %d after landing, want 0", got)
	}
}

// Casting at empty air still telegraphs, so a wind up cannot be used to tell
// whether anyone is in range.
func TestCastingAtNothingStillTelegraphs(t *testing.T) {
	w := testWorld(t, stunRoom)
	a := w.join()
	a.pos = grid.Pos{X: 2, Y: 2}

	w.queueStun(a.id)
	w.resolve()

	for _, p := range w.state().Players {
		if p.ID == a.id && p.Casting == 0 {
			t.Error("a burst that will hit nothing was not telegraphed")
		}
	}
}

func TestStateCarriesStunFields(t *testing.T) {
	w, a, b := stunWorld(t)

	w.queueStun(a.id)
	w.resolve()
	w.resolve()

	byID := map[uint64]int{}
	cdByID := map[uint64]int{}
	for _, p := range w.state().Players {
		byID[p.ID] = p.Stunned
		cdByID[p.ID] = p.StunCD
	}

	if byID[b.id] == 0 {
		t.Error("victim's stun is not in the state")
	}
	if cdByID[a.id] == 0 {
		t.Error("caster's cooldown is not in the state")
	}
}
