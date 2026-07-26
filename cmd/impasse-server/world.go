package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

// TickDuration is the length of one tick. Actions queued during a tick all
// resolve together when it locks.
const TickDuration = 600 * time.Millisecond

type player struct {
	id  uint64
	pos grid.Pos

	// queued is the action for the next tick. Queuing again before the tick
	// locks replaces it, so only the last one counts.
	queued grid.Direction
}

// world is the authoritative game state. Every method runs on the server
// command loop, so none of it locks.
type world struct {
	g       *grid.Grid
	players map[uint64]*player
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

	return &world{
		g:         g,
		players:   make(map[uint64]*player),
		spawn:     spawn,
		hasMarker: ok,
		walkable:  len(g.Walkables()),
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

// queue records an action for the next tick. An illegal move is kept as queued
// and simply fails to move at resolution, which keeps this cheap and means the
// legality check happens in exactly one place.
func (w *world) queue(id uint64, d grid.Direction) {
	if p := w.players[id]; p != nil {
		p.queued = d
	}
}

// resolve locks the tick and applies every queued action at once.
func (w *world) resolve() {
	w.tick++

	for _, p := range w.players {
		if p.queued != grid.None {
			// Players stack, so the only thing that can block a move is
			// the geometry.
			if to, ok := w.g.Move(p.pos, p.queued); ok {
				p.pos = to
			}
			p.queued = grid.None
		}
	}
}

func (w *world) welcome(p *player) proto.Welcome {
	return proto.Welcome{
		Type:   proto.TypeWelcome,
		ID:     p.id,
		TickMS: int(TickDuration / time.Millisecond),
		Map:    w.g.Lines(),
	}
}

func (w *world) state() proto.State {
	players := make([]proto.Player, 0, len(w.players))
	for _, p := range w.players {
		players = append(players, proto.Player{
			ID: p.id,
			X:  p.pos.X,
			Y:  p.pos.Y,
		})
	}
	return proto.State{
		Type:    proto.TypeState,
		Tick:    w.tick,
		Players: players,
	}
}
