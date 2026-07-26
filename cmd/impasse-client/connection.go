package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

// connection talks to the server. Incoming states arrive on states, outgoing
// actions go out on a goroutine so a slow socket cannot stall rendering.
type connection struct {
	net     net.Conn
	w       *proto.Writer
	states  chan proto.State
	out     chan proto.Queue
	closing chan struct{}
}

func dial(conS string) (*connection, error) {
	var network, addr string
	idx := strings.IndexRune(conS, ':')
	if idx < 0 {
		network = "unix"
		addr = conS
	} else {
		network = conS[:idx]
		addr = conS[idx+1:]
	}

	nc, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	return &connection{
		net:     nc,
		w:       proto.NewWriter(nc),
		states:  make(chan proto.State, 1),
		out:     make(chan proto.Queue, 4),
		closing: make(chan struct{}),
	}, nil
}

func (c *connection) close() {
	close(c.closing)
	c.net.Close()
}

// handshake blocks until the welcome message arrives, then starts the reader
// and writer. Nothing can be drawn before this returns.
func (c *connection) handshake() (*proto.Welcome, *grid.Grid, error) {
	r := proto.NewReader(c.net)

	kind, line, err := r.Next()
	if err != nil {
		return nil, nil, fmt.Errorf("waiting for welcome: %w", err)
	}
	if kind != proto.TypeWelcome {
		return nil, nil, fmt.Errorf("expected welcome, got %q", kind)
	}

	var welcome proto.Welcome
	if err := proto.Decode(line, &welcome); err != nil {
		return nil, nil, err
	}

	g, err := grid.Parse(strings.NewReader(strings.Join(welcome.Map, "\n")))
	if err != nil {
		return nil, nil, fmt.Errorf("bad map from server: %w", err)
	}

	go c.read(r)
	go c.write()

	return &welcome, g, nil
}

func (c *connection) read(r *proto.Reader) {
	defer close(c.states)

	for {
		kind, line, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("read: %v\n", err)
			}
			return
		}
		if kind != proto.TypeState {
			log.Printf("unexpected message %q\n", kind)
			continue
		}

		var state proto.State
		if err := proto.Decode(line, &state); err != nil {
			log.Printf("bad state: %v\n", err)
			continue
		}

		// Only the newest state matters. Each one is a full snapshot, so
		// dropping a stale one costs nothing.
		select {
		case c.states <- state:
		default:
			select {
			case <-c.states:
			default:
			}
			select {
			case c.states <- state:
			default:
			}
		}
	}
}

func (c *connection) write() {
	for {
		select {
		case <-c.closing:
			return
		case q := <-c.out:
			if err := c.w.Write(q); err != nil {
				log.Printf("write: %v\n", err)
				return
			}
		}
	}
}

func (c *connection) queueMove(d grid.Direction) {
	c.queue(proto.Queue{
		Type:   proto.TypeQueue,
		Action: proto.ActionMove,
		Dir:    d.String(),
	})
}

func (c *connection) queueLoot() {
	c.queue(proto.Queue{
		Type:   proto.TypeQueue,
		Action: proto.ActionLoot,
	})
}

func (c *connection) queueStun() {
	c.queue(proto.Queue{
		Type:   proto.TypeQueue,
		Action: proto.ActionStun,
	})
}

// queue asks the server for an action on the next tick. Dropping it when the
// buffer is full is fine: the player can just press again, and a stale action
// is worse than none.
func (c *connection) queue(msg proto.Queue) {
	select {
	case c.out <- msg:
	default:
	}
}
