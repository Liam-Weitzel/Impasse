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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/muesli/termenv"
)

type handler struct {
	renderer string
	args     []string
	maxCons  int
	server   *server
	botAddr  string
}

// contextKeyAccount is where the public key handler stashes the account so the
// session handler can pick it up.
const contextKeyAccount = "impasse-account"

// authenticate accepts any public key and treats it as an account. There is no
// registration and no password. A key the server has not seen becomes a new
// account on the spot.
//
// Anyone can generate more keys, so this does not make multi accounting
// impossible. What it does is make one key mean exactly one character, which is
// the property the game needs. Anything stronger is a launch problem.
func (h *handler) authenticate(ctx ssh.Context, key ssh.PublicKey) bool {
	acc := h.server.accounts.forKey(key)

	// Give the account a stored row the first time its key is seen, so it has
	// a name to show and something for results to land on.
	if h.server.store != nil {
		if _, err := h.server.store.Ensure(acc.fingerprint); err != nil {
			log.Printf("ensuring account %s: %v\n", acc.fingerprint, err)
		}
	}

	ctx.SetValue(contextKeyAccount, acc)
	return true
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

// runMenu shows the pre-game menu and reports whether the player chose to play.
func (h *handler) runMenu(
	s ssh.Session,
	acc *account,
	ptyReq ssh.Pty,
	winCh <-chan ssh.Window,
) bool {
	// Over SSH there is no local terminal to probe, so the colour profile
	// comes from what the client told us.
	renderer := lipgloss.NewRenderer(s,
		termenv.WithProfile(colorProfile(ptyReq.Term, s.Environ())),
		termenv.WithColorCache(true))

	model := newMenuModel(h.server, acc, h.botAddr, renderer)
	model.width, model.height = ptyReq.Window.Width, ptyReq.Window.Height

	program := tea.NewProgram(model,
		tea.WithInput(s),
		tea.WithOutput(s),
		tea.WithAltScreen(),
		// The session already handles its own signals, and bubbletea
		// grabbing them would fight the ssh server.
		tea.WithoutSignals(),
		tea.WithContext(s.Context()),
	)

	// bubbletea cannot size a non tty by itself, so feed it the pty size and
	// every resize the client reports.
	sizes := make(chan struct{})
	go func() {
		defer close(sizes)
		program.Send(tea.WindowSizeMsg{
			Width:  ptyReq.Window.Width,
			Height: ptyReq.Window.Height,
		})
		for win := range winCh {
			program.Send(tea.WindowSizeMsg{Width: win.Width, Height: win.Height})
		}
	}()

	final, err := program.Run()
	if err != nil {
		log.Printf("menu: %v\n", err)
		return false
	}

	if m, ok := final.(menuModel); ok {
		return m.choice == choicePlay
	}
	return false
}

func (h *handler) sshHandle(s ssh.Session) {

	acc, _ := s.Context().Value(contextKeyAccount).(*account)
	if acc == nil {
		io.WriteString(s, "no account for this key\n")
		s.Exit(1)
		return
	}

	// `ssh <host> token` prints the bot token and exits, so the token never
	// has to fight with the game for the screen.
	if cmd := s.Command(); len(cmd) > 0 && cmd[0] == "token" {
		io.WriteString(s, tokenBanner(h.server.accounts.botToken(acc), h.botAddr))
		return
	}

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

	// The menu runs here in the server, because it needs the store and the
	// live world. The renderer is a separate process and only gets spawned
	// once the player has actually chosen to play.
	if !h.runMenu(s, acc, ptyReq, winCh) {
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

	// The renderer is a separate process and cannot present the player's key
	// itself, so it gets a one shot token instead. The player id comes back
	// in the welcome message.
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("TERM=%s", ptyReq.Term),
		fmt.Sprintf("IMPASSE_CONNECTION=%s", h.server.connection()),
		fmt.Sprintf("IMPASSE_TOKEN=%s", h.server.accounts.sessionToken(acc)))

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
		dbFile     = flag.String("db", "impasse.db", "path to the score database")
		matchLen   = flag.Duration("match", DefaultMatchDuration, "match length")
		breakLen   = flag.Duration("intermission", DefaultIntermissionDuration,
			"break between matches")
	)

	flag.Parse()

	w, err := loadWorld(*mapFile)
	if err != nil {
		log.Fatalf("loading map: %v\n", err)
	}
	w.matchDuration = *matchLen
	w.intermissionDuration = *breakLen
	w.phaseTicks = w.intermissionTicks()

	log.Printf("map %s loaded: %dx%d, %d walkable cells, spawn at %v\n",
		*mapFile, w.g.Width(), w.g.Height(), w.walkable, w.spawn)
	if !w.hasMarker {
		log.Printf("warning: map has no 'S' spawn marker, falling back to %v\n",
			w.spawn)
	}
	if sealed := w.walkable - w.reachable; sealed > 0 {
		log.Printf("warning: %d cells cannot be reached from the spawn\n", sealed)
	}
	log.Printf("matches last %s with a %s break\n", *matchLen, *breakLen)

	db, err := openStore(*dbFile)
	if err != nil {
		log.Fatalf("opening score database: %v\n", err)
	}
	defer db.Close()
	log.Printf("scores in %s\n", *dbFile)

	addrs := []string{*connection}
	if *botAddr != "" {
		addrs = append(addrs, *botAddr)
	}

	cs := newServer(w, addrs...)
	cs.store = db

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
		botAddr:  *botAddr,
	}

	s := &ssh.Server{
		Addr:             fmt.Sprintf(":%d", *port),
		Handler:          h.sshHandle,
		PublicKeyHandler: h.authenticate,
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
