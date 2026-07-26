package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
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
	dead bool

	// acc is set once the client has authenticated, id is the character it
	// drives. An account may have one renderer and one bot, both driving the
	// same character, so a player can watch their bot and take over from it.
	acc  *account
	kind connKind
	id   uint64
}

// label names a connection in logs before and after it has authenticated.
func (c *conn) label() string {
	if c.acc == nil {
		return "unauthenticated"
	}
	return fmt.Sprintf("player %d", c.id)
}

type server struct {
	// addrs are every endpoint to accept on. The renderer talks over a unix
	// socket and bots over TCP, but it is one protocol and one world, so a
	// bot and a human are the same thing to everything below this line.
	addrs     []string
	listeners []net.Listener

	cmds     chan func(*server)
	cons     map[net.Conn]*conn
	world    *world
	accounts *accounts
	store    *store
	quit     bool
}

func newServer(w *world, addrs ...string) *server {
	return &server{
		addrs:    addrs,
		cmds:     make(chan func(*server)),
		cons:     make(map[net.Conn]*conn),
		world:    w,
		accounts: newAccounts(),
	}
}

// connection is the endpoint the renderer should dial, which is the first one.
func (s *server) connection() string {
	if len(s.addrs) == 0 {
		return ""
	}
	return s.addrs[0]
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
		log.Printf("%s: send buffer full, dropping message\n", c.label())
	}
}

func (s *server) broadcastState() {
	state := s.world.state()
	for _, c := range s.cons {
		// Nothing to show a client that has not said who it is yet.
		if c.acc == nil {
			continue
		}
		s.send(c, state)
	}
}

func (s *server) writer(c *conn) {
	w := proto.NewWriter(c.net)
	for msg := range c.out {
		if err := w.Write(msg); err != nil {
			log.Printf("%s: write: %v\n", c.label(), err)
			s.closeCon(c.net)
			return
		}
	}
}

func (s *server) reader(c *conn) {
	defer s.closeCon(c.net)

	r := proto.NewReader(c.net)

	// Nothing is served until the client says who it is.
	if !s.authenticate(c, r) {
		return
	}

	for {
		kind, line, err := r.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("%s: read: %v\n", c.label(), err)
			}
			return
		}

		switch kind {
		case proto.TypeQueue:
			var q proto.Queue
			if err := proto.Decode(line, &q); err != nil {
				log.Printf("%s: bad queue: %v\n", c.label(), err)
				continue
			}
			switch q.Action {
			case proto.ActionMove:
				d, ok := grid.ParseDirection(q.Dir)
				if !ok {
					log.Printf("%s: unknown direction %q\n", c.label(), q.Dir)
					continue
				}
				s.cmds <- func(s *server) {
					s.world.queueMove(c.id, d)
				}
			case proto.ActionLoot:
				s.cmds <- func(s *server) {
					s.world.queueLoot(c.id)
				}
			case proto.ActionStun:
				s.cmds <- func(s *server) {
					s.world.queueStun(c.id)
				}
			default:
				log.Printf("%s: unknown action %q\n", c.label(), q.Action)
			}
		default:
			log.Printf("%s: unexpected message %q\n", c.label(), kind)
		}
	}
}

// authenticate reads the opening auth message and binds the connection to an
// account. It reports whether the client may proceed.
func (s *server) authenticate(c *conn, r *proto.Reader) bool {
	reject := func(reason string) bool {
		log.Printf("auth rejected: %s\n", reason)
		// Sent directly rather than queued, because the connection is about
		// to go and the writer may never get to it.
		proto.NewWriter(c.net).Write(proto.Error{
			Type:    proto.TypeError,
			Message: reason,
		})
		return false
	}

	msgType, line, err := r.Next()
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("unauthenticated: read: %v\n", err)
		}
		return false
	}
	if msgType != proto.TypeAuth {
		return reject("first message must be auth, got " + msgType)
	}

	var auth proto.Auth
	if err := proto.Decode(line, &auth); err != nil {
		return reject("malformed auth message")
	}

	acc, kind, ok := s.accounts.redeem(auth.Token)
	if !ok {
		return reject("unknown or already used token")
	}

	// One renderer and one bot per account, no more. Two terminals on one
	// account would mean two cameras on one character with no answer for
	// which is the real view.
	first, ok := s.accounts.attach(acc, kind)
	if !ok {
		return reject("this account already has a " + string(kind) + " connected")
	}

	done := make(chan struct{})
	s.cmds <- func(s *server) {
		defer close(done)

		c.acc = acc
		c.kind = kind

		if first {
			p := s.world.join()
			s.accounts.setPlayer(acc, p.id)
			c.id = p.id
			log.Printf("player %d: %s joined as %s at %v\n",
				p.id, acc.fingerprint, kind, p.pos)
		} else {
			id, _ := s.accounts.player(acc)
			c.id = id
			log.Printf("player %d: %s attached\n", id, kind)
		}

		s.send(c, s.world.welcome(s.world.players[c.id]))

		s.broadcastState()
	}
	<-done

	return true
}

func (s *server) closeCon(nc net.Conn) {
	s.cmds <- func(s *server) {
		c, ok := s.cons[nc]
		if !ok {
			// Both reader and writer report failures, so this runs twice.
			return
		}
		log.Printf("%s: disconnected\n", c.label())

		delete(s.cons, nc)

		// The character only goes when the last connection driving it does,
		// so a player spectating their own bot can drop the spectator
		// session without losing the character.
		if c.acc != nil && s.accounts.detach(c.acc, c.kind) {
			s.world.remove(c.id)
		}

		c.dead = true
		nc.Close()
		close(c.out)

		s.broadcastState()
	}
}

func (s *server) newConnection(nc net.Conn) {
	s.cmds <- func(s *server) {
		// No character yet. The reader authenticates first, and only then
		// does anything join the world.
		c := &conn{
			net: nc,
			out: make(chan any, outgoing),
		}
		s.cons[nc] = c

		go s.writer(c)
		go s.reader(c)
	}
}

// recordResults hands a finished match to the store. Runs on the command loop,
// so it must not block for long.
func (s *server) recordResults(results []result) {
	if s.store == nil {
		return
	}
	for _, r := range results {
		c := s.connForPlayer(r.playerID)
		if c == nil || c.acc == nil {
			continue
		}
		if err := s.store.RecordMatch(c.acc.fingerprint, r.score); err != nil {
			log.Printf("recording result for player %d: %v\n", r.playerID, err)
		}
	}
}

// connForPlayer finds any connection driving a character, so a result can be
// tied back to the account that earned it.
func (s *server) connForPlayer(id uint64) *conn {
	for _, c := range s.cons {
		if c.acc != nil && c.id == id {
			return c
		}
	}
	return nil
}

func (s *server) doQuit() { s.quit = true }

func (s *server) accept(l net.Listener, done chan struct{}) {
	for {
		nc, err := l.Accept()
		if err != nil {
			select {
			case <-done:
				// Expected, the listener was closed on the way out.
			default:
				log.Printf("accept on %s: %v\n", l.Addr(), err)
				select {
				case s.cmds <- (*server).doQuit:
				case <-done:
				}
			}
			return
		}
		s.newConnection(nc)
	}
}

// parseAddr splits a "network:address" string. A bare address is TCP, so
// ":2223" works without ceremony.
func parseAddr(s string) (network, addr string) {
	switch {
	case strings.HasPrefix(s, "unix:"):
		return "unix", strings.TrimPrefix(s, "unix:")
	case strings.HasPrefix(s, "tcp:"):
		return "tcp", strings.TrimPrefix(s, "tcp:")
	default:
		return "tcp", s
	}
}

func (s *server) listen() error {
	for _, a := range s.addrs {
		network, addr := parseAddr(a)
		l, err := net.Listen(network, addr)
		if err != nil {
			s.closeListeners()
			return fmt.Errorf("listening on %s: %w", a, err)
		}
		log.Printf("listening on %s\n", a)
		s.listeners = append(s.listeners, l)
	}
	if len(s.listeners) == 0 {
		return errors.New("no addresses to listen on")
	}
	return nil
}

func (s *server) closeListeners() {
	for _, l := range s.listeners {
		l.Close()
	}
	s.listeners = nil
}

func (s *server) shutdown() {
	log.Println("shutdown")
	s.closeListeners()
}

func (s *server) run(done chan struct{}) error {

	if err := s.listen(); err != nil {
		return err
	}
	defer s.shutdown()

	for _, l := range s.listeners {
		go s.accept(l, done)
	}

	ticker := time.NewTicker(s.world.tickDuration)
	defer ticker.Stop()

	for !s.quit {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			if results := s.world.resolve(); len(results) > 0 {
				s.recordResults(results)
			}
			s.broadcastState()
		case fn := <-s.cmds:
			fn(s)
		}
	}

	return nil
}

// lobbyPlayer is one character as the menu sees it. Names are not resolved
// here, because the server does not own the store.
type lobbyPlayer struct {
	Fingerprint string
	Score       int
	Channel     int
	HasBot      bool
	HasTerminal bool
}

// lobbyInfo is what the pre-game menu shows about the live world.
type lobbyInfo struct {
	Match   proto.Match
	Players []lobbyPlayer
}

// lobby summarises the world for the menu. Goes through the command loop like
// everything else that touches world state.
func (s *server) lobby() lobbyInfo {
	var info lobbyInfo

	done := make(chan struct{})
	s.cmds <- func(s *server) {
		defer close(done)

		info.Match = s.world.matchState()

		byPlayer := map[uint64]*lobbyPlayer{}
		for _, p := range s.world.players {
			byPlayer[p.id] = &lobbyPlayer{Score: p.score, Channel: p.channel}
		}

		// A character can be driven by both a terminal and a bot, so fold
		// every connection in rather than taking the first.
		for _, c := range s.cons {
			if c.acc == nil {
				continue
			}
			lp := byPlayer[c.id]
			if lp == nil {
				continue
			}
			lp.Fingerprint = c.acc.fingerprint
			switch c.kind {
			case kindBot:
				lp.HasBot = true
			case kindRenderer:
				lp.HasTerminal = true
			}
		}

		for _, lp := range byPlayer {
			info.Players = append(info.Players, *lp)
		}
	}
	<-done

	sort.Slice(info.Players, func(i, j int) bool {
		if info.Players[i].Score != info.Players[j].Score {
			return info.Players[i].Score > info.Players[j].Score
		}
		return info.Players[i].Fingerprint < info.Players[j].Fingerprint
	})

	return info
}
