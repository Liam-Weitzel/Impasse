package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Liam-Weitzel/Impasse/proto"
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

// winWatch hands window changes to whatever currently owns the screen.
//
// A session reports resizes on one channel for its whole life, but two things
// own the screen in turn: the menu and then the renderer. Letting each range
// over that channel directly means both are reading it at once, and every
// resize goes to exactly one of them, so the renderer misses about half of
// them and the picture stops matching the terminal.
//
// One reader owns the channel instead, and the current owner registers with
// watch.
type winWatch struct {
	mu   sync.Mutex
	fn   func(ssh.Window)
	last ssh.Window

	// resend asks the reader to hand the current size to a new owner.
	resend chan struct{}
}

// newWinWatch starts reading ch. initial is the size from the pty request,
// which the channel itself never reports.
func newWinWatch(initial ssh.Window, ch <-chan ssh.Window) *winWatch {
	w := &winWatch{last: initial, resend: make(chan struct{}, 1)}

	// Everything is delivered from here, never from the goroutine that calls
	// watch. An owner is free to block on delivery: bubbletea's Send does
	// exactly that until its Run loop starts reading.
	go func() {
		for {
			select {
			case win, ok := <-ch:
				if !ok {
					return
				}
				w.mu.Lock()
				w.last = win
				fn := w.fn
				w.mu.Unlock()

				if fn != nil {
					fn(win)
				}

			case <-w.resend:
				w.mu.Lock()
				fn, last := w.fn, w.last
				w.mu.Unlock()

				if fn != nil {
					fn(last)
				}
			}
		}
	}()

	return w
}

// watch makes fn the current owner and arranges for it to be given the size
// straight away, so something taking over does not have to wait for the next
// resize to learn how big the terminal is. A nil fn means nobody is watching.
//
// Never blocks. The size arrives on the reader goroutine shortly after.
func (w *winWatch) watch(fn func(ssh.Window)) {
	w.mu.Lock()
	w.fn = fn
	w.mu.Unlock()

	if fn == nil {
		return
	}

	select {
	case w.resend <- struct{}{}:
	default:
		// One pending resend is enough, it reads the current owner and size
		// when it runs.
	}
}

// sessionInput reads the session's keystrokes once and passes them to whatever
// currently owns the terminal.
//
// A session is read by the menu and then by the renderer, over and over as the
// player goes back and forth. Letting each read the session directly means two
// readers at once, and every keystroke goes to exactly one of them, so half the
// player's typing disappears. Same problem as winWatch, same shape of fix.
type sessionInput struct {
	mu   sync.Mutex
	sink io.Writer
}

// to makes w the current owner. Nil means nobody is reading, and keystrokes
// are dropped rather than queued: they were typed at a screen that is gone.
func (si *sessionInput) to(w io.Writer) {
	si.mu.Lock()
	si.sink = w
	si.mu.Unlock()
}

// Write never reports an error, so the single copy from the session survives an
// owner going away. A renderer exiting closes its pty while a keystroke is in
// flight, and that must not tear down input for the rest of the session.
func (si *sessionInput) Write(p []byte) (int, error) {
	si.mu.Lock()
	w := si.sink
	si.mu.Unlock()

	if w != nil {
		w.Write(p)
	}
	return len(p), nil
}

// runMenu shows the pre-game menu and reports whether the player chose to play.
// runMenu shows the pre-game menu and returns the account to play as, or nil
// if the player quit or never signed in.
func (h *handler) runMenu(
	s ssh.Session,
	fingerprint string,
	acc *account,
	ptyReq ssh.Pty,
	wins *winWatch,
	input *sessionInput,
) *account {
	// Over SSH there is no local terminal to probe, so the colour profile
	// comes from what the client told us.
	renderer := lipgloss.NewRenderer(s,
		termenv.WithProfile(colorProfile(ptyReq.Term, s.Environ())),
		termenv.WithColorCache(true))

	model := newMenuModel(h.server, fingerprint, acc, h.botAddr, hostOf(s.LocalAddr()), renderer)
	model.width, model.height = ptyReq.Window.Width, ptyReq.Window.Height

	// Input arrives through the session's one reader rather than from the
	// session directly, so the menu and the renderer are never reading it at
	// the same time. Closing the write end when the menu is done releases
	// bubbletea's reader.
	keys, typed := io.Pipe()
	input.to(typed)
	defer func() {
		input.to(nil)
		typed.Close()
	}()

	program := tea.NewProgram(model,
		tea.WithInput(keys),
		tea.WithOutput(s),
		tea.WithAltScreen(),
		// The session already handles its own signals, and bubbletea
		// grabbing them would fight the ssh server.
		tea.WithoutSignals(),
		tea.WithContext(s.Context()),
	)

	// bubbletea cannot size a non tty by itself, so feed it the pty size and
	// every resize the client reports. Registering hands over the current size
	// immediately, so there is no separate opening send.
	wins.watch(func(win ssh.Window) {
		program.Send(tea.WindowSizeMsg{Width: win.Width, Height: win.Height})
	})
	// The menu is done with the screen when this returns, and must stop taking
	// resizes, or the renderer never sees them.
	defer wins.watch(nil)

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
		io.WriteString(s, tokenBanner(h.server.accounts.botToken(acc), h.botAddr,
			hostOf(s.LocalAddr())))
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

	// One reader for the session's resizes, handed to the menu and then to the
	// renderer.
	wins := newWinWatch(ptyReq.Window, winCh)

	// One reader for the session's keystrokes, for the same reason.
	input := &sessionInput{}
	go io.Copy(input, s)

	// Menu, play, menu again. Escape in the game comes back here rather than
	// dropping the connection, so a player can check the leaderboard or change
	// their name without reconnecting.
	for {
		// The menu runs here in the server, because it needs the store and the
		// live world. It also owns signing in, so it is what turns a session
		// into a player. The renderer is a separate process and only gets
		// spawned once the player has actually chosen to play.
		acc = h.runMenu(s, fp, acc, ptyReq, wins, input)
		if acc == nil {
			return
		}

		if !h.play(s, acc, ptyReq, wins, input) {
			return
		}
	}
}

// play spawns the renderer and runs it until it exits. It reports whether the
// player asked to go back to the menu, as opposed to ending the session.
func (h *handler) play(
	s ssh.Session,
	acc *account,
	ptyReq ssh.Pty,
	wins *winWatch,
	input *sessionInput,
) bool {
	cmdCtx, cancelCmd := context.WithCancel(s.Context())
	defer cancelCmd()

	cmd := exec.CommandContext(
		cmdCtx, h.renderer,
		h.args...)

	// Start from our own environment so the renderer inherits things like
	// the GL/SDL library paths, then let the session env override it.
	cmd.Env = append(os.Environ(), s.Environ()...)

	// The renderer is a separate process and cannot present the player's key
	// itself, so it gets a one shot token instead. A fresh one every time,
	// since redeeming spends it. The player id comes back in the welcome
	// message.
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("TERM=%s", ptyReq.Term),
		fmt.Sprintf("IMPASSE_CONNECTION=%s", h.server.connection()),
		fmt.Sprintf("IMPASSE_TOKEN=%s", h.server.accounts.sessionToken(acc)))

	// Sized at creation. A pty started without a size is 0x0, and the renderer
	// would draw at whatever it falls back to until the first resize arrives.
	f, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(ptyReq.Window.Width),
		Rows: uint16(ptyReq.Window.Height),
	})
	if err != nil {
		io.WriteString(s, fmt.Sprintf("failed to initialize pseudo-terminal: %s\n", err))
		s.Exit(1)
		return false
	}
	defer f.Close()

	// The renderer owns the screen now. Registering also applies the current
	// size, which catches a resize made while the menu was up.
	wins.watch(func(win ssh.Window) {
		setWinsize(f, win.Width, win.Height)
	})
	input.to(f)

	// Returns when the renderer closes its side of the pty, which is how the
	// session learns it has finished drawing.
	io.Copy(s, f)

	wins.watch(nil)
	input.to(nil)

	f.Close()
	err = cmd.Wait()

	// The renderer is a separate process, so its exit status is all there is
	// to go on. Anything but the one code means the session is over.
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == proto.ExitToMenu
}

// resolveClientID picks the GitHub client id out of a flag, a file or the
// environment, in that order.
//
// The file is what systemd's LoadCredential hands over, and on NixOS it is the
// only one of the three that keeps the id out of the world readable store and
// off a command line every process can read. Trailing whitespace is stripped,
// since a file written by a human ends in a newline and a client id with a
// newline on it fails at GitHub with nothing useful to go on.
func resolveClientID(flagValue, file, env string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading github client id: %w", err)
		}
		id := strings.TrimSpace(string(b))
		if id == "" {
			return "", fmt.Errorf("github client id file %s is empty", file)
		}
		return id, nil
	}

	return strings.TrimSpace(env), nil
}

func main() {

	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	con := filepath.Join(dir, "impasse.sock")

	var (
		port       = flag.Int("port", 22, "ssh server port")
		connection = flag.String("connection", "unix:"+con, "renderer socket")
		botAddr    = flag.String("bots", ":2223", "bot API address, empty to disable")
		maxCons    = flag.Int("maxcons", 0, "max number of connections")
		renderer   = flag.String("renderer", "impasse-client", "path to renderer")
		mapFile    = flag.String("map", "maps/open.txt", "path to the ASCII map")
		keyFile    = flag.String("key", "", "path to host key file")
		dbFile     = flag.String("db", "impasse.db", "path to the score database")
		ghClientID = flag.String("github-client-id", "",
			"GitHub OAuth app client id. Also read from IMPASSE_GITHUB_CLIENT_ID")
		ghClientIDFile = flag.String("github-client-id-file", "",
			"file holding the client id, for systemd LoadCredential and the like")
		matchLen = flag.Duration("match", DefaultMatchDuration, "match length")
		breakLen = flag.Duration("intermission", DefaultIntermissionDuration,
			"break between matches")
	)

	flag.Parse()

	// GitHub sign in is not optional. Without it an SSH key would be the
	// identity again, and keys are free, so one person could hold as many
	// players as they liked.
	clientID, err := resolveClientID(*ghClientID, *ghClientIDFile,
		os.Getenv("IMPASSE_GITHUB_CLIENT_ID"))
	if err != nil {
		log.Fatalln(err)
	}
	if clientID == "" {
		log.Fatalln("a GitHub OAuth client id is required: pass --github-client-id, " +
			"--github-client-id-file, or set IMPASSE_GITHUB_CLIENT_ID. The client id " +
			"is not a secret, but the client secret is never needed and must not be " +
			"given here.")
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
			if errors.Is(err, os.ErrPermission) && *port < 1024 {
				log.Printf("cannot bind port %d: ports below 1024 need root or "+
					"setcap cap_net_bind_service+ep on this binary\n", *port)
			}
			log.Printf("error: %v\n", err)
		}
	}()

	// SIGTERM as well as Ctrl-C. Closing the unix listener is what unlinks
	// impasse.sock, so a plain kill that skips this leaves the file behind and
	// the next start fails with "address already in use". os.Kill is not here
	// because SIGKILL cannot be caught, and nothing can be cleaned up after it.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("signal received, shutting down")
	case <-sshDied:
		log.Println("ssh server died")
	case <-connectionDied:
		log.Println("connection server died")
	}
	close(done)

	<-connectionDied
}
