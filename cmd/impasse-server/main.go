package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

type handler struct {
	renderer string
	args     []string
	maxCons  int
	server   *server
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

func (h *handler) sshHandle(s ssh.Session) {

	if h.maxCons > 0 && h.server.numConnections() >= h.maxCons {
		fmt.Fprintf(s, "Max number of connections (%d) reached. Try again later.\n",
			h.maxCons)
		return
	}

	ptyReq, winCh, isPty := s.Pty()
	if !isPty {
		io.WriteString(s, "non-interactive terminals are not supported\n")
		s.Exit(1)
		return
	}

	cmdCtx, cancelCmd := context.WithCancel(s.Context())
	defer cancelCmd()

	cmd := exec.CommandContext(
		cmdCtx, h.renderer,
		h.args...)

	// Start from our own environment so the renderer inherits things like
	// the GL/SDL library paths, then let the session env override it.
	cmd.Env = append(os.Environ(), s.Environ()...)

	// The player id is assigned when the renderer connects to the socket and
	// arrives in the welcome message, so it is not passed here.
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("TERM=%s", ptyReq.Term),
		fmt.Sprintf("IMPASSE_CONNECTION=%s", h.server.connection()))

	f, err := pty.Start(cmd)
	if err != nil {
		io.WriteString(s, fmt.Sprintf("failed to initialize pseudo-terminal: %s\n", err))
		s.Exit(1)
		return
	}
	defer f.Close()

	go func() {
		for win := range winCh {
			setWinsize(f, win.Width, win.Height)
		}
	}()

	go func() {
		io.Copy(f, s)
	}()
	io.Copy(s, f)

	f.Close()
	cmd.Wait()
}

func main() {

	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	con := filepath.Join(dir, "impasse.sock")

	var (
		port       = flag.Int("port", 2222, "ssh server port")
		connection = flag.String("connection", "unix:"+con, "renderer socket")
		botAddr    = flag.String("bots", ":2223", "bot API address, empty to disable")
		maxCons    = flag.Int("maxcons", 0, "max number of connections")
		renderer   = flag.String("renderer", "impasse-client", "path to renderer")
		mapFile    = flag.String("map", "maps/test.txt", "path to the ASCII map")
		keyFile    = flag.String("key", "", "path to host key file")
	)

	flag.Parse()

	w, err := loadWorld(*mapFile)
	if err != nil {
		log.Fatalf("loading map: %v\n", err)
	}
	log.Printf("map %s loaded: %dx%d, %d walkable cells, spawn at %v\n",
		*mapFile, w.g.Width(), w.g.Height(), w.walkable, w.spawn)
	if !w.hasMarker {
		log.Printf("warning: map has no 'S' spawn marker, falling back to %v\n",
			w.spawn)
	}
	if sealed := w.walkable - w.reachable; sealed > 0 {
		log.Printf("warning: %d cells cannot be reached from the spawn\n", sealed)
	}

	addrs := []string{*connection}
	if *botAddr != "" {
		addrs = append(addrs, *botAddr)
	}

	cs := newServer(w, addrs...)

	done := make(chan struct{})

	connectionDied := make(chan struct{})

	go func() {
		defer close(connectionDied)
		if err := cs.run(done); err != nil {
			log.Printf("connection error: %v\n", err)
		}
	}()

	h := handler{
		renderer: *renderer,
		args:     flag.Args(),
		maxCons:  *maxCons,
		server:   cs,
	}

	s := &ssh.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: h.sshHandle,
	}

	if *keyFile != "" {
		s.SetOption(ssh.HostKeyFile(*keyFile))
	}

	sshDied := make(chan struct{})

	go func() {
		defer close(sshDied)
		if err := s.ListenAndServe(); err != nil {
			log.Printf("error: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill)

	select {
	case <-sigChan:
		log.Println("killed by Ctrl-C")
	case <-sshDied:
		log.Println("ssh server died")
	case <-connectionDied:
		log.Println("connection server died")
	}
	close(done)

	<-connectionDied
}
