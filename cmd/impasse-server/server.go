package main

import (
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
)

// outgoing is the send queue depth per client. A client that falls this far
// behind is dropping state updates, and since each update is a full snapshot
// it will catch up on the next one.
const outgoing = 4

type conn struct {
	net  net.Conn
	out  chan any
	id   uint64
	dead bool
}

type server struct {
	connection string
	listener   net.Listener
	cmds       chan func(*server)
	cons       map[net.Conn]*conn
	world      *world
	quit       bool
}

func newServer(connection string, w *world) *server {
	return &server{
		connection: connection,
		cmds:       make(chan func(*server)),
		cons:       make(map[net.Conn]*conn),
		world:      w,
	}
}

func (s *server) numConnections() (num int) {
	done := make(chan struct{})
	s.cmds <- func(s *server) {
		num = len(s.cons)
		close(done)
	}
	<-done
	return
}

// send queues a message for one client. Never blocks the command loop: a stuck
// client would otherwise freeze the whole server.
func (s *server) send(c *conn, msg any) {
	if c.dead {
		return
	}
	select {
	case c.out <- msg:
	default:
		log.Printf("client %d: send buffer full, dropping message\n", c.id)
	}
}

func (s *server) broadcastState() {
	state := s.world.state()
	for _, c := range s.cons {
		s.send(c, state)
	}
}

func (s *server) writer(c *conn) {
	w := proto.NewWriter(c.net)
	for msg := range c.out {
		if err := w.Write(msg); err != nil {
			log.Printf("client %d: write: %v\n", c.id, err)
			s.closeCon(c.net)
			return
		}
	}
}

func (s *server) reader(c *conn) {
	defer s.closeCon(c.net)

	r := proto.NewReader(c.net)
	for {
		kind, line, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("client %d: read: %v\n", c.id, err)
			}
			return
		}

		switch kind {
		case proto.TypeQueue:
			var q proto.Queue
			if err := proto.Decode(line, &q); err != nil {
				log.Printf("client %d: bad queue: %v\n", c.id, err)
				continue
			}
			switch q.Action {
			case proto.ActionMove:
				d, ok := grid.ParseDirection(q.Dir)
				if !ok {
					log.Printf("client %d: unknown direction %q\n", c.id, q.Dir)
					continue
				}
				s.cmds <- func(s *server) {
					s.world.queueMove(c.id, d)
				}
			case proto.ActionLoot:
				s.cmds <- func(s *server) {
					s.world.queueLoot(c.id)
				}
			default:
				log.Printf("client %d: unknown action %q\n", c.id, q.Action)
			}
		default:
			log.Printf("client %d: unexpected message %q\n", c.id, kind)
		}
	}
}

func (s *server) closeCon(nc net.Conn) {
	s.cmds <- func(s *server) {
		c, ok := s.cons[nc]
		if !ok {
			// Both reader and writer report failures, so this runs twice.
			return
		}
		log.Printf("client %d: disconnected\n", c.id)

		delete(s.cons, nc)
		s.world.remove(c.id)

		c.dead = true
		nc.Close()
		close(c.out)

		s.broadcastState()
	}
}

func (s *server) newConnection(nc net.Conn) {
	s.cmds <- func(s *server) {
		p := s.world.join()

		c := &conn{
			net: nc,
			out: make(chan any, outgoing),
			id:  p.id,
		}
		s.cons[nc] = c

		log.Printf("client %d: connected at %v\n", p.id, p.pos)

		go s.writer(c)
		go s.reader(c)

		s.send(c, s.world.welcome(p))

		// Everyone needs to know about the new player, and the new player
		// needs to know about everyone.
		s.broadcastState()
	}
}

func (s *server) doQuit() { s.quit = true }

func (s *server) accept() {
	for {
		nc, err := s.listener.Accept()
		if err != nil {
			s.cmds <- (*server).doQuit
			return
		}
		s.newConnection(nc)
	}
}

func (s *server) listen() error {
	var network, addr string
	idx := strings.IndexRune(s.connection, ':')
	if idx < 0 {
		network = "unix"
		addr = s.connection
	} else {
		network = s.connection[:idx]
		addr = s.connection[idx+1:]
	}

	listener, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	s.listener = listener
	return nil
}

func (s *server) shutdown() {
	log.Println("shutdown")
	s.listener.Close()
}

func (s *server) run(done chan struct{}) error {

	if err := s.listen(); err != nil {
		return err
	}
	defer s.shutdown()

	go s.accept()

	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	for !s.quit {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			s.world.resolve()
			s.broadcastState()
		case fn := <-s.cmds:
			fn(s)
		}
	}

	return nil
}
