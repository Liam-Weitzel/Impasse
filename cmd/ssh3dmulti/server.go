package main

import (
	"bufio"
	"log"
	"net"
	"strings"
)

type server struct {
	connection string
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

func (s *server) run(done chan struct{}) error {

	var network, addr string
	idx := strings.IndexRune(s.connection, ':')
	if idx < 0 {
		network = "tcp"
		addr = s.connection
	} else {
		network = s.connection[:idx]
		addr = s.connection[idx+1:]
	}

	listener, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	go func() {
		for !s.quit {
			con, err := listener.Accept()
			if err != nil {
				s.cmds <- func(s *server) {
					log.Printf("listen failed: %v\n", err)
					s.quit = true
				}
				return
			}
			log.Println("accepted")
			s.cmds <- func(s *server) {
				log.Println("new connection")

				out := make(chan string, 5)

				go func() {
					for msg := range out {
						if _, err := con.Write([]byte(msg)); err != nil {
							s.cmds <- func(s *server) {
								con.Close()
								delete(s.cons, con)
							}
						}
					}
				}()

				s.cons[con] = out
				go func() {
					sc := bufio.NewScanner(con)
					for sc.Scan() {
						msg := sc.Text()
						s.cmds <- func(s *server) {
							for c, o := range s.cons {
								if c != con {
									o <- msg
								}
							}
						}
					}
					if err := sc.Err(); err != nil {
						log.Printf("error: %v\n", err)
					}
					s.cmds <- func(s *server) {
						con.Close()
						delete(s.cons, con)
					}
				}()
			}
		}
	}()

	for !s.quit {
		select {
		case <-done:
			return nil
		case fn := <-s.cmds:
			fn(s)
		}
	}

	return nil
}
