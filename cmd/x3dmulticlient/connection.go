package main

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"
)

type connection struct {
	con net.Conn
	out chan string
	in  chan func(*client)
}

func newConnection(conS string) (*connection, error) {

	var network, addr string
	idx := strings.IndexRune(conS, ':')
	if idx < 0 {
		network = "unix"
		addr = conS
	} else {
		network = conS[:idx]
		addr = conS[idx+1:]
	}

	con, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	return &connection{
		con: con,
		out: make(chan string, 5),
		in:  make(chan func(*client), 5),
	}, nil
}

func (c *connection) close() error {
	return c.con.Close()
}

func (c *connection) run(done chan struct{}) {
	go c.outgoing(done)
	go c.incoming(done)
}

func (c *connection) outgoing(done chan struct{}) {
	for {
		select {
		case <-done:
			return
		case msg := <-c.out:
			if _, err := c.con.Write([]byte(msg)); err != nil {
				return
			}
		}
	}
}

func (c *connection) incoming(done chan struct{}) {

	defer c.con.Close()

	in := make(chan string)

	go func() {
		sc := bufio.NewScanner(c.con)
		for sc.Scan() {
			in <- sc.Text()
		}
		close(in)
	}()

	for {
		var ok bool
		var msg string
		select {
		case <-done:
			return
		case msg, ok = <-in:
			if !ok {
				return
			}
		}
		if fn := dispatchMessage(msg); fn != nil {
			select {
			case <-done:
				return
			case c.in <- fn:
			}
		}
	}
}

func (c *connection) send(msg string) {
	c.out <- msg
}

func dispatchMessage(msg string) func(*client) {
	fields := strings.Fields(msg)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "p":
		return position(fields[1:])
	case "h":
		return hello(fields[1:])
	case "l":
		return leave(fields[1:])
	}
	return nil
}

func position(fields []string) func(*client) {
	if len(fields) != 4 {
		log.Printf("position: invalid length: %d\n", len(fields))
		return nil
	}
	id, err := strconv.ParseUint(fields[0], 16, 64)
	if err != nil {
		log.Printf("position: id invalid: %v\n", err)
		return nil
	}

	x, err := strconv.ParseFloat(fields[1], 32)
	if err != nil {
		log.Printf("position: x invalid: %v\n", err)
		return nil
	}
	y, err := strconv.ParseFloat(fields[2], 32)
	if err != nil {
		log.Printf("position: y invalid: %v\n", err)
		return nil
	}
	z, err := strconv.ParseFloat(fields[3], 32)
	if err != nil {
		log.Printf("position: z invalid: %v\n", err)
		return nil
	}
	return func(c *client) {
		c.moveObject(id, float32(x), float32(y), float32(z))
	}
}

func hello(fields []string) func(*client) {
	if len(fields) != 7 {
		log.Printf("hello: invalid length: %d\n", len(fields))
		return nil
	}
	id, err := strconv.ParseUint(fields[0], 16, 64)
	if err != nil {
		log.Printf("hello: id invalid: %v\n", err)
		return nil
	}

	x, err := strconv.ParseFloat(fields[1], 32)
	if err != nil {
		log.Printf("hello: x invalid: %v\n", err)
		return nil
	}
	y, err := strconv.ParseFloat(fields[2], 32)
	if err != nil {
		log.Printf("hello: y invalid: %v\n", err)
		return nil
	}
	z, err := strconv.ParseFloat(fields[3], 32)
	if err != nil {
		log.Printf("hello: x invalid: %v\n", err)
		return nil
	}

	r, err := strconv.ParseUint(fields[4], 16, 8)
	if err != nil {
		log.Printf("hello: r invalid: %v\n", err)
		return nil
	}
	g, err := strconv.ParseUint(fields[5], 16, 8)
	if err != nil {
		log.Printf("hello: g invalid: %v\n", err)
		return nil
	}
	b, err := strconv.ParseUint(fields[6], 16, 8)
	if err != nil {
		log.Printf("hello: b invalid: %v\n", err)
		return nil
	}

	return func(c *client) {
		c.helloObject(
			id,
			float32(x), float32(y), float32(z),
			byte(r), byte(g), byte(b))
	}
}

func leave(fields []string) func(*client) {
	if len(fields) != 1 {
		log.Printf("leave: invalid length: %d\n", len(fields))
		return nil
	}
	id, err := strconv.ParseUint(fields[0], 16, 64)
	if err != nil {
		log.Printf("leave: id invalid: %v\n", err)
		return nil
	}
	return func(c *client) { c.leavObject(id) }
}
