package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

const (
	// TickDuration is the length of one tick. Actions queued during a tick
	// all resolve together when it locks.
	TickDuration = 600 * time.Millisecond

	// LootTicks is how long a player must channel to take a pickup. Long
	// enough to be worth interrupting.
	LootTicks = 4

	// StunStartup is how long after casting the burst lands.
	StunStartup = 1
	// StunTicks is how many ticks a victim loses.
	StunTicks = 2
	// StunCooldown is how long before the caster can go again, measured from
	// the cast.
	//
	// It has to exceed StunTicks or a single attacker locks a victim out
	// forever: cast at T lands T+1 and holds through T+2, so a cooldown of 2
	// would let the next cast land at T+3 with the victim never getting a
	// turn. At 3 the next cast lands at T+4 and the victim gets exactly one
	// free tick at T+3. Do not lower this to match StunTicks.
	StunCooldown = 3
	// StunRadius is the reach in cells. 1 gives the 3x3 block around the
	// caster.
	StunRadius = 1
)

// pendingStun is a cast that has happened but not yet landed.
//
// Targets are chosen when the burst is cast, not when it lands, so a victim
// cannot step clear during the startup tick. Whoever was in range when the
// button went down is caught.
type pendingStun struct {
	caster  uint64
	land    uint64
	targets []uint64
}

// action is what a player has asked for on the next tick.
type action struct {
	kind string
	dir  grid.Direction
}

type player struct {
	id uint64
	// owner is the GitHub account this character belongs to, used to give
	// them their progress back if they reconnect during the same match.
	owner int64
	pos   grid.Pos

	// queued is the action for the next tick. Queuing again before the tick
	// locks replaces it, so only the last one counts.
	queued action

	// channel counts ticks of loot progress on the pickup underfoot. It is
	// per player, so several people can race for the same pickup and each
	// makes their own progress.
	channel int

	// stunned counts down the ticks this player cannot act for.
	stunned int
	// stunReady is the first tick they may cast again.
	stunReady uint64

	score int
}

// world is the authoritative game state. Every method runs on the server
// command loop, so none of it locks.
type world struct {
	g       *grid.Grid
	players map[uint64]*player

	// objectives holds the pickups still uncollected.
	objectives map[grid.Pos]bool

	// pending holds stuns cast but not yet landed.
	pending []pendingStun

	// resumes remembers where a player was and what they had scored when
	// they dropped, so reconnecting inside the same match does not wipe it.
	// Cleared when a match starts, since a new match resets everyone anyway.
	resumes map[int64]resume

	// spawn is where every player enters. All players share one point, so
	// the start of a round is a scramble out of the same door.
	spawn grid.Pos
	// walkable counts every floor cell and reachable counts the ones a
	// player can actually get to from the spawn. They differ when a map has
	// sealed off pockets.
	walkable  int
	reachable int
	// hasMarker records whether the spawn came from an S in the map or from
	// the fallback.
	hasMarker bool
	tick      uint64
	nextID    uint64

	// Round clock. See match.go.
	phase       string
	phaseTicks  int
	matchNumber int

	// tickDuration is how long a tick lasts. It lives here rather than being
	// read straight from the constant so that the value the server ticks at
	// and the value it tells clients can never drift apart, and so tests can
	// run the loop fast.
	tickDuration time.Duration

	// How long a match and the break after it last.
	matchDuration        time.Duration
	intermissionDuration time.Duration
}

func loadWorld(path string) (*world, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	g, err := grid.Parse(f)
	if err != nil {
		return nil, err
	}

	spawn, ok := g.Spawn()
	if !ok {
		// No S in the map. Fall back to the first cell of the largest
		// region rather than refusing to start, but it is worth saying so.
		region := g.LargestRegion()
		if len(region) == 0 {
			return nil, errors.New("map has no walkable cells")
		}
		spawn = region[0]
	}
	if !g.Walkable(spawn) {
		return nil, fmt.Errorf("spawn point %v is not walkable", spawn)
	}

	objectives := make(map[grid.Pos]bool)
	for _, p := range g.Objectives() {
		objectives[p] = true
	}

	w := &world{
		g:          g,
		players:    make(map[uint64]*player),
		objectives: objectives,
		resumes:    map[int64]resume{},
		spawn:      spawn,
		hasMarker:  ok,
		walkable:   len(g.Walkables()),
		// What players can get to is defined by where they start, so
		// measure from the spawn rather than from the largest region.
		reachable:            len(g.Reachable(spawn)),
		tickDuration:         TickDuration,
		matchDuration:        DefaultMatchDuration,
		intermissionDuration: DefaultIntermissionDuration,
	}

	// Open with a break, so anyone connecting as the server comes up gets a
	// countdown rather than dropping into a match already underway.
	w.phase = proto.PhaseIntermission
	w.phaseTicks = w.intermissionTicks()

	return w, nil
}

// resume is what a disconnected player gets back if they return during the
// same match.
type resume struct {
	match int
	pos   grid.Pos
	score int
}

// join places a character for an account and returns it.
//
// Dropping out and coming back inside one match gives you your score and your
// position back. Without that, a flaky connection or an accidental Ctrl-C costs
// the whole match, and the punishment lands hardest on exactly the people least
// able to do anything about it. Across a match boundary nothing carries, since
// a new match resets everyone regardless.
func (w *world) join(owner int64) *player {
	p := &player{
		id:    w.nextID,
		owner: owner,
		pos:   w.spawn,
	}
	w.nextID++

	if r, ok := w.resumes[owner]; ok && r.match == w.matchNumber && owner != 0 {
		p.pos = r.pos
		p.score = r.score
		delete(w.resumes, owner)
	}

	w.players[p.id] = p
	return p
}

// remove takes a character out of the world, keeping what it had in case the
// player comes straight back.
func (w *world) remove(id uint64) {
	p := w.players[id]
	if p == nil {
		return
	}

	if p.owner != 0 {
		w.resumes[p.owner] = resume{
			match: w.matchNumber,
			pos:   p.pos,
			score: p.score,
		}
	}

	delete(w.players, id)
}

// queueMove records a move for the next tick. An illegal move is kept and
// simply fails at resolution, which keeps the legality check in one place.
func (w *world) queueMove(id uint64, d grid.Direction) {
	if p := w.players[id]; p != nil {
		p.queued = action{kind: proto.ActionMove, dir: d}
	}
}

// queueLoot asks to channel the pickup underfoot.
func (w *world) queueLoot(id uint64) {
	if p := w.players[id]; p != nil {
		p.queued = action{kind: proto.ActionLoot}
	}
}

// queueStun asks to burst everyone nearby on the next tick.
func (w *world) queueStun(id uint64) {
	if p := w.players[id]; p != nil {
		p.queued = action{kind: proto.ActionStun}
	}
}

// resolve locks the tick and applies every queued action at once.
//
// Order within the tick is fixed and matters:
//
//  1. Stuns cast last tick land, so a victim loses this tick's action.
//  2. Positions are snapshotted, so range is measured before anybody moves
//     and no player's action can depend on another's resolving first.
//  3. Actions run.
//  4. Pickups are awarded, so simultaneous finishers are seen together.
//  5. The round clock advances, which may end the match.
//
// The clock runs last so a pickup taken on the final tick still counts.
func (w *world) resolve() []result {
	w.tick++

	w.landStuns()

	// Range is measured from where everyone stood at the start of the tick.
	positions := make(map[uint64]grid.Pos, len(w.players))
	for id, p := range w.players {
		positions[id] = p.pos
	}

	// Whoever completes a channel this tick, grouped by pickup. Awarding is
	// deferred so that the order players happen to be iterated in cannot
	// decide who wins.
	finishers := map[grid.Pos][]*player{}

	for _, p := range w.players {
		if p.stunned > 0 {
			// Out cold. No action, and nothing carries over to the wake.
			p.stunned--
			p.channel = 0
			p.queued = action{}
			continue
		}

		switch p.queued.kind {
		case proto.ActionStun:
			// Casting is not looting, so the channel goes either way. That
			// is what makes attacking into a standoff lose it.
			p.channel = 0
			p.queued = action{}
			if w.tick >= p.stunReady {
				w.cast(p, positions)
			}

		case proto.ActionLoot:
			if w.phase != proto.PhaseRunning || !w.objectives[p.pos] {
				// Nothing here, or someone already took it.
				p.channel = 0
				continue
			}
			// Capped, so a held channel sits at full rather than running
			// away. A player parked on a contested pickup is finished and
			// waiting, not accumulating.
			if p.channel < LootTicks {
				p.channel++
			}
			if p.channel >= LootTicks {
				finishers[p.pos] = append(finishers[p.pos], p)
			}

		case proto.ActionMove:
			// Any other action gives up the channel.
			p.channel = 0
			if p.queued.dir != grid.None {
				// Players stack, so only geometry can block a move.
				if to, ok := w.g.Move(p.pos, p.queued.dir); ok {
					p.pos = to
				}
			}
			// A move is spent, a loot is not.
			p.queued = action{}

		default:
			p.channel = 0
		}
	}

	for pos, ps := range finishers {
		w.award(pos, ps)
	}

	// Anyone still channelling a pickup that just went has nothing to hold.
	for _, p := range w.players {
		if p.queued.kind == proto.ActionLoot && !w.objectives[p.pos] {
			p.channel = 0
		}
	}

	return w.advanceMatch()
}

// landStuns applies the bursts due this tick. It runs before actions, so a
// victim loses the tick the burst lands on.
func (w *world) landStuns() {
	if len(w.pending) == 0 {
		return
	}

	kept := w.pending[:0]
	for _, ps := range w.pending {
		if ps.land > w.tick {
			kept = append(kept, ps)
			continue
		}
		for _, id := range ps.targets {
			p := w.players[id]
			if p == nil {
				// Left the game between the cast and the landing.
				continue
			}
			p.stunned = StunTicks
			p.channel = 0
			p.queued = action{}
		}
	}
	w.pending = kept
}

// cast schedules a burst around the caster. Everyone in range at this moment
// is caught, wherever they are when it lands.
func (w *world) cast(caster *player, positions map[uint64]grid.Pos) {
	origin := positions[caster.id]

	var targets []uint64
	for id, pos := range positions {
		if id == caster.id {
			continue
		}
		if abs(pos.X-origin.X) <= StunRadius && abs(pos.Y-origin.Y) <= StunRadius {
			targets = append(targets, id)
		}
	}

	caster.stunReady = w.tick + StunCooldown

	// Recorded even with nothing in range. The caster still visibly winds up,
	// and a burst that hits air should look the same to onlookers as one that
	// connects.
	w.pending = append(w.pending, pendingStun{
		caster:  caster.id,
		land:    w.tick + StunStartup,
		targets: targets,
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// award resolves one pickup against everyone who finished channelling it on
// this tick.
//
// A pickup only comes free when exactly one player completes. If two or more
// finish together nobody takes it, and they hold there at a full channel. That
// is a genuine standoff: it lasts until one of them stops, at which point the
// other collects on the very next tick.
//
// It is also why attacking into a standoff loses it. Casting a stun is not
// looting, so it drops the attacker's own channel, leaving the opponent as the
// sole finisher a full tick before the stun can land.
func (w *world) award(pos grid.Pos, finishers []*player) {
	if len(finishers) != 1 {
		// Contested. Everyone keeps their full channel and keeps waiting.
		return
	}

	winner := finishers[0]
	winner.score++
	winner.channel = 0
	winner.queued = action{}

	delete(w.objectives, pos)
}

func (w *world) welcome(p *player) proto.Welcome {
	return proto.Welcome{
		Type:              proto.TypeWelcome,
		ID:                p.id,
		TickMS:            int(w.tickDuration / time.Millisecond),
		LootTicks:         LootTicks,
		StunTicks:         StunTicks,
		StunCooldownTicks: StunCooldown,
		StunRadius:        StunRadius,
		Map:               w.g.Lines(),
	}
}

func (w *world) state() proto.State {
	// Bursts in flight, by caster, so onlookers can see one coming.
	casting := make(map[uint64]int, len(w.pending))
	for _, ps := range w.pending {
		if ps.land > w.tick {
			casting[ps.caster] = int(ps.land - w.tick)
		}
	}

	players := make([]proto.Player, 0, len(w.players))
	for _, p := range w.players {
		cooldown := 0
		if p.stunReady > w.tick {
			cooldown = int(p.stunReady - w.tick)
		}
		players = append(players, proto.Player{
			ID:      p.id,
			X:       p.pos.X,
			Y:       p.pos.Y,
			Score:   p.score,
			Channel: p.channel,
			Stunned: p.stunned,
			StunCD:  cooldown,
			Casting: casting[p.id],
		})
	}

	objectives := make([]proto.Objective, 0, len(w.objectives))
	for pos := range w.objectives {
		objectives = append(objectives, proto.Objective{X: pos.X, Y: pos.Y})
	}

	return proto.State{
		Type:       proto.TypeState,
		Tick:       w.tick,
		Match:      w.matchState(),
		Players:    players,
		Objectives: objectives,
	}
}
