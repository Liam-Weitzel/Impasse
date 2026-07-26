package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl32"
)

// How long to wait for queued messages when shutting down.
const flushTimeout = time.Second

type connection struct {
	con net.Conn
	out chan string
	in  chan batch
	// closed once outgoing() has drained out and returned.
	flushed chan struct{}
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
		con:     con,
		out:     make(chan string, 5),
		in:      make(chan batch),
		flushed: make(chan struct{}),
	}, nil
}

func (c *connection) run(done chan struct{}) {
	go c.outgoing(done)
	go c.incoming(done)
}

// stop shuts the connection down and waits for the queued messages to
// go out, so the final 'leave' reaches the other attendees instead of
// leaving a ghost behind.
func (c *connection) stop(done chan struct{}) {
	close(done)
	select {
	case <-c.flushed:
	case <-time.After(flushTimeout):
		log.Println("timeout flushing outgoing messages")
	}
}

func (c *connection) outgoing(done chan struct{}) {
	defer close(c.flushed)
	for {
		select {
		case <-done:
			c.drain()
			return
		case msg := <-c.out:
			if !c.write(msg) {
				return
			}
		}
	}
}

// drain writes whatever is still queued without blocking.
func (c *connection) drain() {
	for {
		select {
		case msg := <-c.out:
			if !c.write(msg) {
				return
			}
		default:
			return
		}
	}
}

func (c *connection) write(msg string) bool {
	if _, err := c.con.Write([]byte(msg)); err != nil {
		log.Printf("outgoing error: %v\n", err)
		return false
	}
	return true
}

func (c *connection) incoming(done chan struct{}) {

	defer c.con.Close()

	raw := make(chan string)

	go func() {
		defer close(raw)
		sc := bufio.NewScanner(c.con)
		for sc.Scan() {
			raw <- sc.Text()
		}
	}()

	batching(raw, c.in, dispatchMessage)
}

func (c *connection) send(msg string) {
	c.out <- msg
}

func (c *connection) sendPos(id uint64, pos mgl32.Vec3) {
	c.send(fmt.Sprintf("p %x %f %f %f\n", id, pos[0], pos[1], pos[2]))
}

func (c *connection) sendHello(id uint64, pos, col mgl32.Vec3) {
	c.send(fmt.Sprintf("h %x %f %f %f %x %x %x\n",
		id,
		pos[0], pos[1], pos[2],
		byte(col[0]*255), byte(col[1]*255), byte(col[2]*255)))
}

func (c *connection) sendLeave(id uint64) {
	c.send(fmt.Sprintf("l %x\n", id))
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
		c.moveAttendee(id, float32(x), float32(y), float32(z))
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
		c.helloAttendee(
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
	return func(c *client) { c.leaveAttendee(id) }
}
