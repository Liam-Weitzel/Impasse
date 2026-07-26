package main

import (
	"testing"
	"time"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

const matchMap = "" +
	"#####\n" +
	"#S*.#\n" +
	"#...#\n" +
	"#####"

// matchWorld uses a tick short enough that a whole match is a handful of ticks.
func matchWorld(t *testing.T) *world {
	t.Helper()
	w := testWorld(t, matchMap)
	w.tickDuration = time.Second

	// testWorld jumps straight into a match. Wind it back to the opening
	// intermission, which is what a real server actually starts in.
	w.matchNumber = 0
	w.phase = proto.PhaseIntermission
	w.phaseTicks = w.intermissionTicks()
	w.objectives = map[grid.Pos]bool{}

	return w
}

// runTicks resolves n ticks and returns every result they produced.
func runTicks(w *world, n int) []result {
	var all []result
	for i := 0; i < n; i++ {
		all = append(all, w.resolve()...)
	}
	return all
}

func TestServerOpensInIntermission(t *testing.T) {
	w := matchWorld(t)

	if w.phase != proto.PhaseIntermission {
		t.Errorf("phase %q, want intermission", w.phase)
	}
	if w.matchNumber != 0 {
		t.Errorf("match number %d, want 0 before the first match", w.matchNumber)
	}
}

func TestIntermissionBecomesAMatch(t *testing.T) {
	w := matchWorld(t)

	runTicks(w, w.intermissionTicks())

	if w.phase != proto.PhaseRunning {
		t.Fatalf("phase %q, want running", w.phase)
	}
	if w.matchNumber != 1 {
		t.Errorf("match number %d, want 1", w.matchNumber)
	}
	if w.phaseTicks != w.matchTicks() {
		t.Errorf("clock at %d, want a full match of %d", w.phaseTicks, w.matchTicks())
	}
}

// Pickups come back with the match. This is the whole respawn rule.
func TestObjectivesReturnEachMatch(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)

	runTicks(w, w.intermissionTicks())
	if len(w.objectives) != 1 {
		t.Fatalf("%d objectives at match start, want 1", len(w.objectives))
	}

	// Take it.
	p.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(p.id)
	runTicks(w, LootTicks)
	if len(w.objectives) != 0 {
		t.Fatalf("pickup was not collected")
	}

	// Run out the match and the following break.
	runTicks(w, w.phaseTicks)
	if w.phase != proto.PhaseIntermission {
		t.Fatalf("phase %q, want intermission", w.phase)
	}
	if len(w.objectives) != 0 {
		t.Errorf("%d objectives during the break, want none", len(w.objectives))
	}

	runTicks(w, w.phaseTicks)
	if len(w.objectives) != 1 {
		t.Errorf("%d objectives in the new match, want the pickup back",
			len(w.objectives))
	}
}

func TestMatchEndReportsScores(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)

	runTicks(w, w.intermissionTicks())

	p.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(p.id)
	runTicks(w, LootTicks)
	if p.score != 1 {
		t.Fatalf("score %d, want 1", p.score)
	}

	results := runTicks(w, w.phaseTicks)

	if len(results) != 1 {
		t.Fatalf("%d results, want 1", len(results))
	}
	if results[0].playerID != p.id || results[0].score != 1 {
		t.Errorf("result %+v, want player %d with 1", results[0], p.id)
	}
}

// Scores stay up through the break, then clear when the next match starts.
func TestScoresSurviveTheBreakThenReset(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)

	runTicks(w, w.intermissionTicks())
	p.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(p.id)
	runTicks(w, LootTicks)

	runTicks(w, w.phaseTicks) // match ends
	if p.score != 1 {
		t.Errorf("score %d during the break, want it still showing", p.score)
	}

	runTicks(w, w.phaseTicks) // next match starts
	if p.score != 0 {
		t.Errorf("score %d in the new match, want 0", p.score)
	}
}

// Nothing can be collected between matches, even standing on where a pickup
// will be.
func TestNoScoringDuringIntermission(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)
	p.pos = grid.Pos{X: 2, Y: 1}

	w.queueLoot(p.id)
	runTicks(w, LootTicks+2)

	if p.score != 0 {
		t.Errorf("score %d during the break, want 0", p.score)
	}
	if p.channel != 0 {
		t.Errorf("channel %d during the break, want 0", p.channel)
	}
}

// Everyone starts a match at the door, so position is not inherited from
// wherever they were standing when the last one ended.
func TestMatchStartReturnsPlayersToSpawn(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)

	runTicks(w, w.intermissionTicks())
	p.pos = grid.Pos{X: 3, Y: 2}

	runTicks(w, w.phaseTicks) // end
	runTicks(w, w.phaseTicks) // start

	if p.pos != w.spawn {
		t.Errorf("player at %v, want the spawn at %v", p.pos, w.spawn)
	}
}

// A burst in flight must not survive the boundary and land into the next match.
func TestMatchStartClearsCombatState(t *testing.T) {
	w := matchWorld(t)
	a := w.join(0)
	b := w.join(0)

	runTicks(w, w.intermissionTicks())

	a.pos = grid.Pos{X: 2, Y: 2}
	b.pos = grid.Pos{X: 2, Y: 2}
	w.queueStun(a.id)
	w.resolve()
	if len(w.pending) == 0 {
		t.Fatal("no burst in flight to test with")
	}

	runTicks(w, w.phaseTicks) // end
	if len(w.pending) != 0 {
		t.Errorf("%d bursts survived the match end", len(w.pending))
	}

	runTicks(w, w.phaseTicks) // start
	if b.stunned != 0 {
		t.Errorf("player stunned %d at the start of a new match", b.stunned)
	}
	if a.stunReady != 0 {
		t.Errorf("cooldown %d carried into a new match", a.stunReady)
	}
}

func TestMatchStateIsPublished(t *testing.T) {
	w := matchWorld(t)

	st := w.state()
	if st.Match.Phase != proto.PhaseIntermission {
		t.Errorf("phase %q in state, want intermission", st.Match.Phase)
	}
	if st.Match.TicksRemaining <= 0 {
		t.Errorf("ticks remaining %d, want a countdown", st.Match.TicksRemaining)
	}

	runTicks(w, w.intermissionTicks())

	st = w.state()
	if st.Match.Phase != proto.PhaseRunning {
		t.Errorf("phase %q, want running", st.Match.Phase)
	}
	if st.Match.Number != 1 {
		t.Errorf("match number %d, want 1", st.Match.Number)
	}
}

// A pickup taken on the very last tick of a match still counts, because the
// clock advances after actions resolve.
func TestPickupOnTheFinalTickCounts(t *testing.T) {
	w := matchWorld(t)
	p := w.join(0)

	runTicks(w, w.intermissionTicks())
	p.pos = grid.Pos{X: 2, Y: 1}

	// Line the channel up to complete exactly as the clock runs out.
	runTicks(w, w.phaseTicks-LootTicks)
	w.queueLoot(p.id)
	results := runTicks(w, LootTicks)

	if len(results) != 1 {
		t.Fatalf("%d results, want the match to have ended", len(results))
	}
	if results[0].score != 1 {
		t.Errorf("final tick pickup scored %d, want 1", results[0].score)
	}
}

func TestTicksIn(t *testing.T) {
	if got := ticksIn(2*time.Minute, 600*time.Millisecond); got != 200 {
		t.Errorf("got %d ticks, want 200", got)
	}
	// Never zero, or a phase would never end.
	if got := ticksIn(time.Millisecond, time.Second); got != 1 {
		t.Errorf("got %d, want at least 1", got)
	}
	if got := ticksIn(time.Second, 0); got != 1 {
		t.Errorf("got %d for a zero tick, want 1", got)
	}
}

// Dropping out and coming back inside one match must not cost the match. A
// flaky connection or an accidental Ctrl-C should not wipe a player's score.
func TestRejoinKeepsScoreWithinAMatch(t *testing.T) {
	w := matchWorld(t)
	runTicks(w, w.intermissionTicks())

	const owner = 42
	p := w.join(owner)
	p.pos = grid.Pos{X: 2, Y: 1}

	w.queueLoot(p.id)
	runTicks(w, LootTicks)
	if p.score != 1 {
		t.Fatalf("score %d, want 1 before dropping", p.score)
	}
	away := p.pos

	w.remove(p.id)

	back := w.join(owner)
	if back.score != 1 {
		t.Errorf("score %d after rejoining, want the 1 they earned", back.score)
	}
	if back.pos != away {
		t.Errorf("came back at %v, want %v", back.pos, away)
	}
}

// A new match resets everyone, so nothing carries across the boundary.
func TestRejoinAfterAMatchStartsFresh(t *testing.T) {
	w := matchWorld(t)
	runTicks(w, w.intermissionTicks())

	const owner = 42
	p := w.join(owner)
	p.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(p.id)
	runTicks(w, LootTicks)

	w.remove(p.id)

	// Run out the match and the break.
	runTicks(w, w.phaseTicks)
	runTicks(w, w.phaseTicks)

	back := w.join(owner)
	if back.score != 0 {
		t.Errorf("score %d in a new match, want 0", back.score)
	}
	if back.pos != w.spawn {
		t.Errorf("started at %v, want the spawn", back.pos)
	}
}

// One player's progress must not be handed to somebody else.
func TestResumeIsPerAccount(t *testing.T) {
	w := matchWorld(t)
	runTicks(w, w.intermissionTicks())

	a := w.join(1)
	a.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(a.id)
	runTicks(w, LootTicks)
	w.remove(a.id)

	other := w.join(2)
	if other.score != 0 {
		t.Errorf("a different account inherited a score of %d", other.score)
	}
	if other.pos != w.spawn {
		t.Errorf("a different account inherited position %v", other.pos)
	}
}

// Accounts are only zero in tests, and a zero owner must not pool everyone's
// progress into one bucket.
func TestZeroOwnerNeverResumes(t *testing.T) {
	w := matchWorld(t)
	runTicks(w, w.intermissionTicks())

	a := w.join(0)
	a.pos = grid.Pos{X: 2, Y: 1}
	w.queueLoot(a.id)
	runTicks(w, LootTicks)
	w.remove(a.id)

	b := w.join(0)
	if b.score != 0 {
		t.Errorf("score %d leaked between anonymous players", b.score)
	}
}
