package main

import (
	"os"
	"time"

	"gitlab.com/sascha.l.teichmann/ssh3d/grid"
	"gitlab.com/sascha.l.teichmann/ssh3d/proto"
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
	spawns  []grid.Pos
	tick    uint64
	nextID  uint64
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

	return &world{
		g:       g,
		players: make(map[uint64]*player),
		spawns:  g.Spawns(),
	}, nil
}

// spawn places a new player and returns it. Spawn points cycle through the
// walkable cells so two players joining together do not land on each other.
func (w *world) spawn() *player {
	p := &player{id: w.nextID}
	w.nextID++

	if len(w.spawns) > 0 {
		p.pos = w.spawns[int(p.id)%len(w.spawns)]
	}

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
