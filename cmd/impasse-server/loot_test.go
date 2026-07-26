package main

import (
	"testing"

	"github.com/Liam-Weitzel/Impasse/grid"
)

// One pickup, one space to the east of the spawn.
const lootRoom = "" +
	"#####\n" +
	"#S*.#\n" +
	"#...#\n" +
	"#####"

func lootWorld(t *testing.T) (*world, *player) {
	t.Helper()
	w := testWorld(t, lootRoom)
	p := w.join()
	p.pos = grid.Pos{X: 2, Y: 1}
	return w, p
}

func TestObjectivesLoadFromMap(t *testing.T) {
	w := testWorld(t, lootRoom)

	if len(w.objectives) != 1 {
		t.Fatalf("%d objectives, want 1", len(w.objectives))
	}
	if !w.objectives[grid.Pos{X: 2, Y: 1}] {
		t.Errorf("objective is not where the map put it: %v", w.objectives)
	}
	// The pickup cell is floor, so it has to be walkable.
	if !w.g.Walkable(grid.Pos{X: 2, Y: 1}) {
		t.Error("objective cell is not walkable")
	}
}

// Four ticks of channelling, and not one fewer.
func TestLootTakesFourTicks(t *testing.T) {
	w, p := lootWorld(t)

	w.queueLoot(p.id)

	for i := 1; i < LootTicks; i++ {
		w.resolve()
		if p.channel != i {
			t.Fatalf("after %d ticks channel is %d, want %d", i, p.channel, i)
		}
		if p.score != 0 {
			t.Fatalf("scored after only %d ticks", i)
		}
		if len(w.objectives) != 1 {
			t.Fatalf("objective vanished after %d ticks", i)
		}
	}

	w.resolve()

	if p.score != 1 {
		t.Errorf("score %d, want 1", p.score)
	}
	if p.channel != 0 {
		t.Errorf("channel %d after finishing, want 0", p.channel)
	}
	if len(w.objectives) != 0 {
		t.Errorf("objective still present after collection")
	}
}

// A loot is not consumed by the tick, otherwise holding a pickup would mean
// hammering the key once per tick.
func TestLootPersistsAcrossTicks(t *testing.T) {
	w, p := lootWorld(t)

	w.queueLoot(p.id)
	w.resolve()
	w.resolve()

	if p.channel != 2 {
		t.Fatalf("channel %d, want 2 from a single queued loot", p.channel)
	}
}

// Moving gives up the channel entirely. There is no partial credit.
func TestMovingResetsTheChannel(t *testing.T) {
	w, p := lootWorld(t)

	w.queueLoot(p.id)
	w.resolve()
	w.resolve()
	if p.channel != 2 {
		t.Fatalf("channel %d, want 2", p.channel)
	}

	w.queueMove(p.id, grid.South)
	w.resolve()

	if p.channel != 0 {
		t.Errorf("channel %d after moving, want 0", p.channel)
	}

	// Coming back starts from nothing.
	w.queueMove(p.id, grid.North)
	w.resolve()
	w.queueLoot(p.id)
	w.resolve()

	if p.channel != 1 {
		t.Errorf("channel %d on return, want 1", p.channel)
	}
	if p.score != 0 {
		t.Errorf("score %d, want 0", p.score)
	}
}

func TestLootOnEmptyGroundDoesNothing(t *testing.T) {
	w := testWorld(t, lootRoom)
	p := w.join()
	p.pos = grid.Pos{X: 1, Y: 2}

	w.queueLoot(p.id)
	for i := 0; i < LootTicks+2; i++ {
		w.resolve()
	}

	if p.channel != 0 || p.score != 0 {
		t.Errorf("channel %d score %d, want 0 and 0", p.channel, p.score)
	}
}

// Two players on one pickup each make their own progress. Nothing about being
// contested slows either of them down.
func TestChannelsAreIndependent(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	b.pos = a.pos

	w.queueLoot(a.id)
	w.resolve()
	w.resolve()

	w.queueLoot(b.id)
	w.resolve()

	if a.channel != 3 {
		t.Errorf("a channel %d, want 3", a.channel)
	}
	if b.channel != 1 {
		t.Errorf("b channel %d, want 1", b.channel)
	}
}

// Whoever finishes first takes it. The loser is left holding a channel on
// nothing, and must not also score.
func TestOnlyOnePlayerCanCollectAPickup(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	b.pos = a.pos

	// a is one tick ahead.
	w.queueLoot(a.id)
	w.resolve()
	w.queueLoot(b.id)

	for i := 0; i < LootTicks; i++ {
		w.resolve()
	}

	if a.score != 1 {
		t.Errorf("a score %d, want 1", a.score)
	}
	if b.score != 0 {
		t.Errorf("b score %d, want 0, the pickup was already gone", b.score)
	}
	if b.channel != 0 {
		t.Errorf("b channel %d, want 0 once the pickup went", b.channel)
	}
	if len(w.objectives) != 0 {
		t.Errorf("%d objectives left, want 0", len(w.objectives))
	}
}

// Two players finishing on the same tick means nobody takes it. The pickup is
// the impasse the game is named for.
func TestSimultaneousCompletionYieldsNothing(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	b.pos = a.pos

	w.queueLoot(a.id)
	w.queueLoot(b.id)

	for i := 0; i < LootTicks; i++ {
		w.resolve()
	}

	if a.score != 0 || b.score != 0 {
		t.Errorf("scores %d and %d, want nobody to collect", a.score, b.score)
	}
	if len(w.objectives) != 1 {
		t.Errorf("%d objectives left, want the pickup untaken", len(w.objectives))
	}
	if a.channel != LootTicks || b.channel != LootTicks {
		t.Errorf("channels %d and %d, want both held at %d",
			a.channel, b.channel, LootTicks)
	}
}

// The standoff does not resolve itself. Channels stay capped rather than
// running away, and neither player gains on the other by waiting.
func TestStandoffHoldsIndefinitely(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	b.pos = a.pos

	w.queueLoot(a.id)
	w.queueLoot(b.id)

	for i := 0; i < LootTicks+20; i++ {
		w.resolve()
	}

	if a.score != 0 || b.score != 0 {
		t.Errorf("scores %d and %d, want the standoff to hold", a.score, b.score)
	}
	if a.channel != LootTicks || b.channel != LootTicks {
		t.Errorf("channels %d and %d, want both pinned at %d",
			a.channel, b.channel, LootTicks)
	}
	if len(w.objectives) != 1 {
		t.Error("pickup was taken during a standoff")
	}
}

// Breaking the standoff hands the pickup over immediately. Whoever stops
// channelling loses it on the very next tick.
func TestLeavingAStandoffGivesThePickupAway(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	b.pos = a.pos

	w.queueLoot(a.id)
	w.queueLoot(b.id)
	for i := 0; i < LootTicks+3; i++ {
		w.resolve()
	}

	// a gives up. b is the only finisher on the next tick.
	w.queueMove(a.id, grid.South)
	w.resolve()

	if b.score != 1 {
		t.Errorf("b score %d, want 1 the tick after a left", b.score)
	}
	if a.score != 0 {
		t.Errorf("a score %d, want 0", a.score)
	}
	if len(w.objectives) != 0 {
		t.Error("pickup should be gone")
	}
}

// Three players deadlock exactly as two do, and it takes all but one dropping
// out before anybody collects.
func TestThreeWayStandoff(t *testing.T) {
	w, a := lootWorld(t)
	b := w.join()
	c := w.join()
	b.pos, c.pos = a.pos, a.pos

	for _, p := range []*player{a, b, c} {
		w.queueLoot(p.id)
	}
	for i := 0; i < LootTicks+2; i++ {
		w.resolve()
	}

	if a.score+b.score+c.score != 0 {
		t.Fatalf("someone collected during a three way standoff")
	}

	// One leaves, still contested by the other two.
	w.queueMove(a.id, grid.South)
	w.resolve()

	if b.score+c.score != 0 {
		t.Fatalf("collected while still contested by two")
	}

	// Second leaves, the last one takes it.
	w.queueMove(b.id, grid.South)
	w.resolve()

	if c.score != 1 {
		t.Errorf("c score %d, want 1 once alone", c.score)
	}
}

func TestStateCarriesObjectivesAndScore(t *testing.T) {
	w, p := lootWorld(t)

	state := w.state()
	if len(state.Objectives) != 1 {
		t.Fatalf("%d objectives in state, want 1", len(state.Objectives))
	}

	w.queueLoot(p.id)
	w.resolve()

	state = w.state()
	if len(state.Players) != 1 {
		t.Fatalf("%d players in state", len(state.Players))
	}
	if got := state.Players[0].Channel; got != 1 {
		t.Errorf("channel %d in state, want 1", got)
	}

	for i := 1; i < LootTicks; i++ {
		w.resolve()
	}

	state = w.state()
	if len(state.Objectives) != 0 {
		t.Errorf("%d objectives in state after collection, want 0",
			len(state.Objectives))
	}
	if got := state.Players[0].Score; got != 1 {
		t.Errorf("score %d in state, want 1", got)
	}
}
