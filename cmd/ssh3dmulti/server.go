package main

import (
	"bufio"
	"log"
	"net"
	"strings"
)

type server struct {
	connection string
	listener   net.Listener
	cmds       chan func(*server)
	cons       map[net.Conn]chan string
	// attendee id per connection, needed to fake a leave message
	// when a client vanishes without saying goodbye.
	ids       map[net.Conn]string
	quit      bool
	uniqueIDs uint64
}

func newServer(connection string) *server {
	return &server{
		connection: connection,
		cmds:       make(chan func(*server)),
		cons:       make(map[net.Conn]chan string),
		ids:        make(map[net.Conn]string),
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

func (s *server) newID() (id uint64) {
	done := make(chan struct{})
	s.cmds <- func(s *server) {
		id = s.uniqueIDs
		s.uniqueIDs++
		close(done)
	}
	<-done
	return
}

// doBroadcast must only be called from the command loop.
func (s *server) doBroadcast(msg string, src net.Conn) {
	for con, out := range s.cons {
		if con != src {
			//log.Println("broadcast:", msg)
			// Never block the command loop on a slow receiver:
			// one stuck client would freeze the whole server.
			select {
			case out <- msg:
			default:
				log.Println("send buffer full: dropping message")
			}
		}
	}
}

func (s *server) broadcast(msg string, src net.Conn) {
	s.cmds <- func(s *server) {
		s.doBroadcast(msg, src)
	}
}

// rememberID records which attendee a connection belongs to.
func (s *server) rememberID(con net.Conn, id string) {
	s.cmds <- func(s *server) {
		s.ids[con] = id
	}
}

func (s *server) closeCon(con net.Conn) {
	s.cmds <- func(s *server) {
		out, ok := s.cons[con]
		if !ok {
			// Already closed: both send() and receive() report failures.
			return
		}
		log.Println("connection close")
		delete(s.cons, con)
		con.Close()
		// Terminates the send() goroutine.
		close(out)

		// The renderer is killed outright when the ssh session goes
		// away, so it never gets to send its own leave message. Do it
		// on its behalf or the others keep seeing a ghost. A duplicate
		// leave is ignored by the clients.
		if id, ok := s.ids[con]; ok {
			delete(s.ids, con)
			s.doBroadcast("l "+id, con)
		}
	}
}

// attendeeID extracts the attendee id from a protocol message.
func attendeeID(msg string) (string, bool) {
	fields := strings.Fields(msg)
	if len(fields) < 2 {
		return "", false
	}
	switch fields[0] {
	case "h", "p", "l":
		return fields[1], true
	}
	return "", false
}

func (s *server) send(con net.Conn, out <-chan string) {
	for msg := range out {
		//log.Println("send:", msg)
		if _, err := con.Write(append([]byte(msg), '\n')); err != nil {
			log.Printf("send error: %v\n", err)
			s.closeCon(con)
			return
		}
	}
}

func (s *server) receive(con net.Conn) {
	defer s.closeCon(con)
	var known bool
	sc := bufio.NewScanner(con)
	for sc.Scan() {
		msg := sc.Text()
		if !known {
			if id, ok := attendeeID(msg); ok {
				s.rememberID(con, id)
				known = true
			}
		}
		s.broadcast(msg, con)
	}
	if err := sc.Err(); err != nil {
		log.Printf("error: %v\n", err)
	}
}

func (s *server) newConnection(con net.Conn) {
	s.cmds <- func(s *server) {
		log.Println("new connection")

		out := make(chan string, 5)
		s.cons[con] = out

		go s.send(con, out)
		go s.receive(con)
	}
}

func (s *server) doQuit() { s.quit = true }

func (s *server) accept() {
	for {
		con, err := s.listener.Accept()
		if err != nil {
			s.cmds <- (*server).doQuit
			return
		}
		log.Println("accepted")
		s.newConnection(con)
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

func (s server) shutdown() {
	log.Println("shutdown")
	s.listener.Close()
}

func (s *server) run(done chan struct{}) error {

	if err := s.listen(); err != nil {
		return err
	}
	defer s.shutdown()

	go s.accept()

	for !s.quit {
		select {
		case <-done:
			log.Println("done closed")
			return nil
		case fn := <-s.cmds:
			fn(s)
		}
	}

	return nil
}
