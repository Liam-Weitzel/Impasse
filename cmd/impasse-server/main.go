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
	gossh "golang.org/x/crypto/ssh"
)

type handler struct {
	renderer string
	args     []string
	maxCons  int
	server   *server
	botAddr  string
}

// contextKeyFingerprint is where the public key handler leaves the key the
// client offered, if it offered one.
const contextKeyFingerprint = "impasse-fingerprint"

// acceptKey records whichever key the client offered and lets everyone in.
//
// The key is not an identity and never decides who you are. It is remembered
// only so a machine that has signed in before can skip doing it again. GitHub
// is the identity, because keys are free and GitHub accounts are not.
func (h *handler) acceptKey(ctx ssh.Context, key ssh.PublicKey) bool {
	ctx.SetValue(contextKeyFingerprint, fingerprint(key))
	return true
}

// acceptNoKey lets a client with no key in at all. Without this, anyone who
// has never run ssh-keygen is locked out, and since the key is not the identity
// there is no reason to demand one.
func (h *handler) acceptNoKey(ctx ssh.Context, challenge gossh.KeyboardInteractiveChallenge) bool {
	return true
}

// signIn resolves who this session belongs to, running the GitHub device flow
// unless the key is one we already know.
func (h *handler) signIn(fp string) *account {
	if fp == "" || h.server.store == nil {
		return nil
	}
	stored, ok := h.server.store.PlayerForKey(fp)
	if !ok {
		return nil
	}
	return h.server.accounts.forGitHub(GitHubUser{
		ID: stored.GitHubID, Login: stored.GitHubLogin,
	})
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}

// runMenu shows the pre-game menu and reports whether the player chose to play.
// runMenu shows the pre-game menu and returns the account to play as, or nil
// if the player quit or never signed in.
func (h *handler) runMenu(
	s ssh.Session,
	fingerprint string,
	acc *account,
	ptyReq ssh.Pty,
	winCh <-chan ssh.Window,
) *account {
	// Over SSH there is no local terminal to probe, so the colour profile
	// comes from what the client told us.
	renderer := lipgloss.NewRenderer(s,
		termenv.WithProfile(colorProfile(ptyReq.Term, s.Environ())),
		termenv.WithColorCache(true))

	model := newMenuModel(h.server, fingerprint, acc, h.botAddr, renderer)
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
		return nil
	}

	if m, ok := final.(menuModel); ok && m.choice == choicePlay {
		return m.acc
	}
	return nil
}

func (h *handler) sshHandle(s ssh.Session) {

	fp, _ := s.Context().Value(contextKeyFingerprint).(string)

	// A machine that has signed in before is recognised straight away.
	// Anyone else has to go through the menu's sign in screen.
	acc := h.signIn(fp)

	// `ssh <host> token` prints the bot token and exits, so the token never
	// has to fight with the game for the screen.
	if cmd := s.Command(); len(cmd) > 0 && cmd[0] == "token" {
		if acc == nil {
			io.WriteString(s,
				"Sign in with GitHub first: ssh into this server without a command.\n")
			return
		}
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
	// live world. It also owns signing in, so it is what turns a session into
	// a player. The renderer is a separate process and only gets spawned once
	// the player has actually chosen to play.
	acc = h.runMenu(s, fp, acc, ptyReq, winCh)
	if acc == nil {
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
		mapFile    = flag.String("map", "maps/open.txt", "path to the ASCII map")
		keyFile    = flag.String("key", "", "path to host key file")
		dbFile     = flag.String("db", "impasse.db", "path to the score database")
		ghClientID = flag.String("github-client-id", "",
			"GitHub OAuth app client id. Also read from IMPASSE_GITHUB_CLIENT_ID")
		matchLen = flag.Duration("match", DefaultMatchDuration, "match length")
		breakLen = flag.Duration("intermission", DefaultIntermissionDuration,
			"break between matches")
	)

	flag.Parse()

	// GitHub sign in is not optional. Without it an SSH key would be the
	// identity again, and keys are free, so one person could hold as many
	// players as they liked.
	clientID := *ghClientID
	if clientID == "" {
		clientID = os.Getenv("IMPASSE_GITHUB_CLIENT_ID")
	}
	if clientID == "" {
		log.Fatalln("a GitHub OAuth client id is required: pass --github-client-id " +
			"or set IMPASSE_GITHUB_CLIENT_ID. The client id is not a secret, " +
			"but the client secret is never needed and must not be given here.")
	}

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

	// Bot tokens outlive the process, so a restart does not silently break
	// every bot that already has one.
	cs.accounts.loadToken = db.BotToken
	cs.accounts.saveToken = db.SetBotToken

	cs.oauth = newGitHubOAuth(clientID)

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
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: h.sshHandle,
		// A key is welcome but not required. Both handlers are set so a
		// client with a key uses it and a client without still gets in.
		PublicKeyHandler:           h.acceptKey,
		KeyboardInteractiveHandler: h.acceptNoKey,
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
