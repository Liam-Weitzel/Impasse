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
)

// action is what a player has asked for on the next tick.
type action struct {
	kind string
	dir  grid.Direction
}

type player struct {
	id  uint64
	pos grid.Pos

	// queued is the action for the next tick. Queuing again before the tick
	// locks replaces it, so only the last one counts.
	queued action

	// channel counts ticks of loot progress on the pickup underfoot. It is
	// per player, so several people can race for the same pickup and each
	// makes their own progress.
	channel int

	score int
}

// world is the authoritative game state. Every method runs on the server
// command loop, so none of it locks.
type world struct {
	g       *grid.Grid
	players map[uint64]*player

	// objectives holds the pickups still uncollected.
	objectives map[grid.Pos]bool

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

	return &world{
		g:          g,
		players:    make(map[uint64]*player),
		objectives: objectives,
		spawn:      spawn,
		hasMarker:  ok,
		walkable:   len(g.Walkables()),
		// What players can get to is defined by where they start, so
		// measure from the spawn rather than from the largest region.
		reachable: len(g.Reachable(spawn)),
	}, nil
}

// join places a new player and returns it. Everyone starts on the same cell.
// Players stack, so there is nothing to resolve when several arrive at once.
func (w *world) join() *player {
	p := &player{
		id:  w.nextID,
		pos: w.spawn,
	}
	w.nextID++

	w.players[p.id] = p
	return p
}

func (w *world) remove(id uint64) {
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

// resolve locks the tick and applies every queued action at once.
func (w *world) resolve() {
	w.tick++

	// Whoever completes a channel this tick, grouped by pickup. Awarding is
	// deferred so that the order players happen to be iterated in cannot
	// decide who wins.
	finishers := map[grid.Pos][]*player{}

	for _, p := range w.players {
		switch p.queued.kind {
		case proto.ActionLoot:
			if !w.objectives[p.pos] {
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
		Type:      proto.TypeWelcome,
		ID:        p.id,
		TickMS:    int(TickDuration / time.Millisecond),
		LootTicks: LootTicks,
		Map:       w.g.Lines(),
	}
}

func (w *world) state() proto.State {
	players := make([]proto.Player, 0, len(w.players))
	for _, p := range w.players {
		players = append(players, proto.Player{
			ID:      p.id,
			X:       p.pos.X,
			Y:       p.pos.Y,
			Score:   p.score,
			Channel: p.channel,
		})
	}

	objectives := make([]proto.Objective, 0, len(w.objectives))
	for pos := range w.objectives {
		objectives = append(objectives, proto.Objective{X: pos.X, Y: pos.Y})
	}

	return proto.State{
		Type:       proto.TypeState,
		Tick:       w.tick,
		Players:    players,
		Objectives: objectives,
	}
}
