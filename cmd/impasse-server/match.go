package main

import (
	"log"
	"time"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

// The world runs in rounds rather than continuously. A match is a fixed window
// of scoring, then a short break before the next one.
//
// This settles two things the design left open. Match length is now a real
// number instead of a question, and objectives no longer need a respawn rule of
// their own: they come back when the match does. That also stops a session
// grinding to a halt, which the standoff rule made possible, since two players
// deadlocked over the last pickup only hold it until the clock runs out.
// Defaults, overridable with --match and --intermission. Two minutes is a
// starting point, not a considered number.
const (
	DefaultMatchDuration        = 2 * time.Minute
	DefaultIntermissionDuration = 15 * time.Second
)

// ticksIn converts a duration to whole ticks, never fewer than one.
func ticksIn(d, tick time.Duration) int {
	if tick <= 0 {
		return 1
	}
	n := int(d / tick)
	if n < 1 {
		return 1
	}
	return n
}

func (w *world) matchTicks() int {
	return ticksIn(w.matchDuration, w.tickDuration)
}

func (w *world) intermissionTicks() int {
	return ticksIn(w.intermissionDuration, w.tickDuration)
}

// result is one player's finish, handed to whatever is recording scores.
type result struct {
	playerID uint64
	score    int
}

// advanceMatch runs the round clock by one tick and swaps phase when the
// current one runs out. It returns the results of a match that just ended, so
// the caller can persist them without the world knowing about storage.
func (w *world) advanceMatch() []result {
	if w.phaseTicks > 0 {
		w.phaseTicks--
	}
	if w.phaseTicks > 0 {
		return nil
	}

	switch w.phase {
	case proto.PhaseRunning:
		results := w.endMatch()
		return results
	default:
		w.startMatch()
		return nil
	}
}

// endMatch freezes the scoreboard and drops into the break. Scores stay up
// through the intermission so players can see how it went.
func (w *world) endMatch() []result {
	results := make([]result, 0, len(w.players))
	for _, p := range w.players {
		results = append(results, result{playerID: p.id, score: p.score})
	}

	w.phase = proto.PhaseIntermission
	w.phaseTicks = w.intermissionTicks()

	// Nothing to collect during the break, and no half finished channels or
	// bursts carried across the boundary.
	w.objectives = map[grid.Pos]bool{}
	w.pending = nil
	for _, p := range w.players {
		p.channel = 0
		p.stunned = 0
		p.queued = action{}
	}

	log.Printf("match %d over\n", w.matchNumber)
	return results
}

// startMatch resets everything a match owns and starts the clock.
func (w *world) startMatch() {
	w.matchNumber++
	w.phase = proto.PhaseRunning
	w.phaseTicks = w.matchTicks()

	// Pickups come back. This is the whole respawn rule.
	w.objectives = make(map[grid.Pos]bool, len(w.g.Objectives()))
	for _, p := range w.g.Objectives() {
		w.objectives[p] = true
	}

	w.pending = nil

	// Everyone back to the door, so the start is a scramble rather than a
	// reward for wherever you happened to be standing.
	for _, p := range w.players {
		p.pos = w.spawn
		p.score = 0
		p.channel = 0
		p.stunned = 0
		p.stunReady = 0
		p.queued = action{}
	}

	log.Printf("match %d started, %d pickups\n", w.matchNumber, len(w.objectives))
}

func (w *world) matchState() proto.Match {
	return proto.Match{
		Phase:          w.phase,
		Number:         w.matchNumber,
		TicksRemaining: w.phaseTicks,
	}
}
