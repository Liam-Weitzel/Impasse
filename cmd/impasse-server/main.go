package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
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

	// sessions caps how many people can be in the game at once.
	sessions *sessionLimit
}

// sessionLimit counts the sessions currently holding a place.
//
// This counts SSH sessions rather than protocol connections, because the cost
// is the renderer process behind a session, and a session holds its place from
// the menu until it disconnects.
type sessionLimit struct {
	mu  sync.Mutex
	n   int
	max int
}

// enter takes a place if there is one. It reports the current count either way,
// so a refusal can say how busy the server is.
func (l *sessionLimit) enter() (int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.max > 0 && l.n >= l.max {
		return l.n, false
	}
	l.n++
	return l.n, true
}

func (l *sessionLimit) leave() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.n > 0 {
		l.n--
	}
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

	// Every session that reaches the game costs a renderer process, about a
	// core, and a large share of the server's upload. Turning people away
	// politely keeps the game playable for whoever is already in, which matters
	// most at exactly the moment the game gets attention.
	if n, ok := h.sessions.enter(); !ok {
		fmt.Fprintf(s, "\r\nImpasse is full: %d of %d playing.\r\n\r\n"+
			"Matches are two minutes, so a place should come up shortly.\r\n"+
			"Try again in a minute.\r\n\r\n", n, h.maxCons)
		return
	}
	defer h.sessions.leave()

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

	// Start from our own environment so the renderer inherits things like the
	// GL/SDL library paths, then add the few client variables that are safe.
	cmd.Env = append(os.Environ(), safeEnv(s.Environ())...)

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

// safeEnv picks the client supplied variables the renderer is allowed to see.
//
// Anyone on the internet can open a session, and an SSH client may ask for any
// environment variable it likes. Passing those straight to a process running as
// the server's user hands a stranger the dynamic loader: LD_PRELOAD and
// LD_LIBRARY_PATH decide which code gets loaded, and PATH decides which
// binaries get run. An allowlist rather than a denylist, because the list of
// variables that change how a program loads is long and grows.
//
// Only what the renderer actually needs to draw correctly is let through.
// Colour comes from TERM and COLORTERM, and the locale settings decide how
// text is encoded.
func safeEnv(env []string) []string {
	allowed := map[string]bool{
		"TERM":      true,
		"COLORTERM": true,
		"LANG":      true,
	}

	var out []string
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		// LC_ALL, LC_CTYPE and the rest are a family rather than a fixed set.
		if allowed[name] || strings.HasPrefix(name, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// hostKey loads the server's SSH host key, making one on first run.
//
// The host key is the server's identity, and it has to be the same after a
// restart. Without a stable one the ssh library invents a fresh key at every
// start, and every returning player is met with REMOTE HOST IDENTIFICATION HAS
// CHANGED, which is ssh reporting what looks exactly like someone impersonating
// the server. Most of them will not connect again.
func hostKey(path string) (gossh.Signer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return generateHostKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading host key %s: %w", path, err)
	}

	signer, err := gossh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parsing host key %s: %w", path, err)
	}
	return signer, nil
}

// generateHostKey writes a new ed25519 host key and returns it.
func generateHostKey(path string) (gossh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating host key: %w", err)
	}

	block, err := gossh.MarshalPrivateKey(priv, "impasse")
	if err != nil {
		return nil, fmt.Errorf("encoding host key: %w", err)
	}
	data := pem.EncodeToMemory(block)

	// Anyone who can read this can pretend to be the server.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("writing host key %s: %w", path, err)
	}
	log.Printf("new host key written to %s\n", path)

	return gossh.ParsePrivateKey(data)
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
		// Each session in the game costs roughly a core and a large slice of
		// upload, so the ceiling is low on purpose. 0 removes it.
		maxCons  = flag.Int("maxcons", 16, "players allowed in the game at once, 0 for no limit")
		renderer = flag.String("renderer", "impasse-client", "path to renderer")
		mapFile  = flag.String("map", "maps/open.txt", "path to the ASCII map")
		keyFile  = flag.String("key", filepath.Join(dir, "impasse_host_key"),
			"SSH host key file, generated on first run")
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
		sessions: &sessionLimit{max: *maxCons},
	}

	s := &ssh.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: h.sshHandle,
		// A key is welcome but not required. Both handlers are set so a
		// client with a key uses it and a client without still gets in.
		PublicKeyHandler:           h.acceptKey,
		KeyboardInteractiveHandler: h.acceptNoKey,
	}

	// Loaded rather than left to the library, and fatal on failure. Falling
	// back to a throwaway key would look like it worked and then greet every
	// returning player with a host key warning.
	signer, err := hostKey(*keyFile)
	if err != nil {
		log.Fatalf("host key: %v\n", err)
	}
	s.AddHostKey(signer)

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
