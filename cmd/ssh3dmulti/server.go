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
	quit       bool
	uniqueIDs  uint64
}

func newServer(connection string) *server {
	return &server{
		connection: connection,
		cmds:       make(chan func(*server)),
		cons:       make(map[net.Conn]chan string),
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

func (s *server) broadcast(msg string, src net.Conn) {
	s.cmds <- func(s *server) {
		for con, out := range s.cons {
			if con != src {
				//log.Println("broadcast:", msg)
				out <- msg
			}
		}
	}
}

func (s *server) closeCon(con net.Conn) {
	s.cmds <- func(s *server) {
		log.Println("connection close")
		con.Close()
		delete(s.cons, con)
	}
}

func (s *server) send(con net.Conn, out <-chan string) {
	for msg := range out {
		//log.Println("send:", msg)
		if _, err := con.Write(append([]byte(msg), '\n')); err != nil {
			log.Printf("send error: %v\n", err)
			s.closeCon(con)
		}
	}
}

func (s *server) receive(con net.Conn) {
	defer s.closeCon(con)
	sc := bufio.NewScanner(con)
	for sc.Scan() {
		msg := sc.Text()
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
