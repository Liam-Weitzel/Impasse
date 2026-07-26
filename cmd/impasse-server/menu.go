package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The pre-game menu, shown over the SSH session before the renderer starts.
//
// It runs in the server process rather than in the renderer, because it needs
// the store and the live world, and because the renderer is a separate process
// that only exists once you have decided to play.

// maxNameLength keeps a display name from running over the leaderboard columns.
const maxNameLength = 16

// menuChoice is what the player picked, read after the program exits.
type menuChoice int

const (
	choiceQuit menuChoice = iota
	choicePlay
)

// pane is which screen the menu is showing.
type pane int

const (
	paneMain pane = iota
	paneLeaderboard
	panePlayers
	paneName
	paneToken
)

// refreshMsg drives the live panes and the match clock.
type refreshMsg time.Time

func refreshEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return refreshMsg(t)
	})
}

type menuItem struct {
	label string
	pane  pane
	// act is set for the items that do something rather than open a pane.
	act menuChoice
	// isAct marks those items, since choiceQuit is the zero value.
	isAct bool
}

var menuItems = []menuItem{
	{label: "Play", act: choicePlay, isAct: true},
	{label: "Leaderboard", pane: paneLeaderboard},
	{label: "Who is playing", pane: panePlayers},
	{label: "Change display name", pane: paneName},
	{label: "Bot token", pane: paneToken},
	{label: "Quit", act: choiceQuit, isAct: true},
}

type menuModel struct {
	server  *server
	store   *store
	acc     *account
	botAddr string

	styles menuStyles
	width  int
	height int

	pane   pane
	cursor int
	choice menuChoice
	done   bool

	name  textinput.Model
	notes string

	account Account
	board   []Account
	lobby   lobbyInfo
	tickMS  int
}

func newMenuModel(s *server, acc *account, botAddr string, renderer *lipgloss.Renderer) menuModel {
	input := textinput.New()
	input.CharLimit = maxNameLength
	input.Prompt = "> "

	m := menuModel{
		server:  s,
		store:   s.store,
		acc:     acc,
		botAddr: botAddr,
		styles:  newMenuStyles(renderer),
		name:    input,
		tickMS:  int(s.world.tickDuration / time.Millisecond),
		width:   80,
		height:  24,
	}
	m.reload()
	return m
}

// reload pulls fresh data. Cheap enough to do once a second.
func (m *menuModel) reload() {
	if m.store != nil {
		if a, err := m.store.Ensure(m.acc.fingerprint); err == nil {
			m.account = a
		}
		if board, err := m.store.Leaderboard(10); err == nil {
			m.board = board
		}
	}
	m.lobby = m.server.lobby()
}

func (m menuModel) Init() tea.Cmd {
	return refreshEvery()
}

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case refreshMsg:
		m.reload()
		return m, refreshEvery()

	case tea.KeyMsg:
		if m.pane == paneName {
			return m.updateName(msg)
		}
		return m.updateMain(msg)
	}

	return m, nil
}

func (m menuModel) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.choice = choiceQuit
		m.done = true
		return m, tea.Quit

	case "esc":
		if m.pane != paneMain {
			m.pane = paneMain
			m.notes = ""
			return m, nil
		}
		m.choice = choiceQuit
		m.done = true
		return m, tea.Quit

	case "up", "k":
		if m.pane == paneMain && m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.pane == paneMain && m.cursor < len(menuItems)-1 {
			m.cursor++
		}

	case "enter", " ":
		if m.pane != paneMain {
			m.pane = paneMain
			return m, nil
		}
		item := menuItems[m.cursor]
		if item.isAct {
			m.choice = item.act
			m.done = true
			return m, tea.Quit
		}
		m.pane = item.pane
		m.notes = ""
		if m.pane == paneName {
			m.name.SetValue(m.account.Name)
			m.name.CursorEnd()
			return m, m.name.Focus()
		}
	}

	return m, nil
}

func (m menuModel) updateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.name.Blur()
		m.pane = paneMain
		m.notes = ""
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.name.Value())
		if name == "" {
			m.notes = "A name cannot be empty."
			return m, nil
		}
		if m.store != nil {
			if err := m.store.SetName(m.acc.fingerprint, name); err != nil {
				m.notes = "Could not save: " + err.Error()
				return m, nil
			}
		}
		m.reload()
		m.name.Blur()
		m.pane = paneMain
		m.notes = "Now playing as " + name + "."
		return m, nil
	}

	var cmd tea.Cmd
	m.name, cmd = m.name.Update(msg)
	return m, cmd
}

// clock renders the match countdown, which is the same information the in game
// HUD shows.
func (m menuModel) clock() string {
	left := time.Duration(m.lobby.Match.TicksRemaining) *
		time.Duration(m.tickMS) * time.Millisecond
	mins := int(left / time.Minute)
	secs := int((left % time.Minute) / time.Second)

	switch m.lobby.Match.Phase {
	case "running":
		return fmt.Sprintf("Match %d in progress, %d:%02d left",
			m.lobby.Match.Number, mins, secs)
	default:
		return fmt.Sprintf("Next match in %d:%02d", mins, secs)
	}
}

func (m menuModel) View() string {
	if m.done {
		return ""
	}

	var body string
	switch m.pane {
	case paneLeaderboard:
		body = m.viewLeaderboard()
	case panePlayers:
		body = m.viewPlayers()
	case paneName:
		body = m.viewName()
	case paneToken:
		body = m.viewToken()
	default:
		body = m.viewMain()
	}

	header := m.styles.title.Render("IMPASSE") + "  " +
		m.styles.clock.Render(m.clock())

	footer := m.styles.help.Render(m.footerHelp())

	parts := []string{header, "", body}
	if m.notes != "" {
		parts = append(parts, "", m.styles.note.Render(m.notes))
	}
	parts = append(parts, "", footer)

	return m.styles.frame.Render(strings.Join(parts, "\n"))
}

func (m menuModel) footerHelp() string {
	switch m.pane {
	case paneMain:
		return "up/down move  enter select  q quit"
	case paneName:
		return "enter save  esc cancel"
	default:
		return "esc back"
	}
}

func (m menuModel) viewMain() string {
	var b strings.Builder

	who := m.account.Name
	if who == "" {
		who = defaultName(m.acc.fingerprint)
	}
	b.WriteString(m.styles.dim.Render("Signed in as ") + m.styles.name.Render(who))
	if m.account.Matches > 0 {
		b.WriteString(m.styles.dim.Render(fmt.Sprintf(
			"   best %d   total %d over %d matches",
			m.account.Best, m.account.Total, m.account.Matches)))
	} else {
		b.WriteString(m.styles.dim.Render("   no matches played yet"))
	}
	b.WriteString("\n\n")

	for i, item := range menuItems {
		if i == m.cursor {
			b.WriteString(m.styles.selected.Render("> " + item.label))
		} else {
			b.WriteString(m.styles.item.Render("  " + item.label))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m menuModel) viewLeaderboard() string {
	if len(m.board) == 0 {
		return m.styles.dim.Render("Nobody has finished a match yet.")
	}

	var b strings.Builder
	b.WriteString(m.styles.heading.Render(
		fmt.Sprintf("%-4s %-18s %6s %6s %8s", "", "NAME", "BEST", "TOTAL", "MATCHES")))
	b.WriteString("\n")

	for i, a := range m.board {
		row := fmt.Sprintf("%-4d %-18s %6d %6d %8d",
			i+1, truncate(a.Name, 18), a.Best, a.Total, a.Matches)
		if a.Fingerprint == m.acc.fingerprint {
			b.WriteString(m.styles.selected.Render(row))
		} else {
			b.WriteString(m.styles.item.Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render(
		"Ranked on best single match, so leaving a bot running does not climb it."))

	return b.String()
}

func (m menuModel) viewPlayers() string {
	if len(m.lobby.Players) == 0 {
		return m.styles.dim.Render("Nobody is in the world right now.")
	}

	var b strings.Builder
	b.WriteString(m.styles.heading.Render(
		fmt.Sprintf("%-18s %6s  %s", "NAME", "SCORE", "DRIVEN BY")))
	b.WriteString("\n")

	for _, p := range m.lobby.Players {
		name := defaultName(p.Fingerprint)
		if m.store != nil {
			if a, err := m.store.Get(p.Fingerprint); err == nil {
				name = a.Name
			}
		}

		var driven []string
		if p.HasTerminal {
			driven = append(driven, "terminal")
		}
		if p.HasBot {
			driven = append(driven, "bot")
		}

		row := fmt.Sprintf("%-18s %6d  %s",
			truncate(name, 18), p.Score, strings.Join(driven, " and "))
		if p.Fingerprint == m.acc.fingerprint {
			b.WriteString(m.styles.selected.Render(row))
		} else {
			b.WriteString(m.styles.item.Render(row))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m menuModel) viewName() string {
	return m.styles.item.Render("Display name, shown on the leaderboard.") + "\n" +
		m.styles.dim.Render("Your account is your SSH key, so renaming changes nothing else.") +
		"\n\n" + m.name.View()
}

func (m menuModel) viewToken() string {
	token := ""
	if m.server != nil {
		token = m.server.accounts.botToken(m.acc)
	}

	address := m.botAddr
	if address == "" {
		return m.styles.item.Render("The bot API is switched off on this server.")
	}

	var b strings.Builder
	b.WriteString(m.styles.item.Render("Bot token:"))
	b.WriteString("\n\n  ")
	b.WriteString(m.styles.token.Render(token))
	b.WriteString("\n\n")
	b.WriteString(m.styles.item.Render(
		"python3 examples/bot.py --address " + address + " --token <token>"))
	b.WriteString("\n\n")
	b.WriteString(m.styles.dim.Render(
		"Your bot drives the same character you do, so you can watch it play and\n" +
			"take over. Whichever queues an action last before the tick locks wins.\n" +
			"The token lasts until the server restarts."))

	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "."
}

type menuStyles struct {
	frame    lipgloss.Style
	title    lipgloss.Style
	clock    lipgloss.Style
	heading  lipgloss.Style
	item     lipgloss.Style
	selected lipgloss.Style
	dim      lipgloss.Style
	name     lipgloss.Style
	note     lipgloss.Style
	token    lipgloss.Style
	help     lipgloss.Style
}

func newMenuStyles(r *lipgloss.Renderer) menuStyles {
	return menuStyles{
		frame:    r.NewStyle().Padding(1, 2),
		title:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
		clock:    r.NewStyle().Foreground(lipgloss.Color("14")),
		heading:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("7")),
		item:     r.NewStyle().Foreground(lipgloss.Color("15")),
		selected: r.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
		dim:      r.NewStyle().Foreground(lipgloss.Color("8")),
		name:     r.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
		note:     r.NewStyle().Foreground(lipgloss.Color("10")),
		token:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		help:     r.NewStyle().Foreground(lipgloss.Color("8")),
	}
}

// colorProfile picks what the session's terminal can actually show. Over SSH
// there is no local terminal to probe, so it comes from the client's TERM and
// COLORTERM instead.
func colorProfile(term string, env []string) termenv.Profile {
	for _, e := range env {
		if strings.HasPrefix(e, "COLORTERM=") {
			v := strings.ToLower(strings.TrimPrefix(e, "COLORTERM="))
			if v == "truecolor" || v == "24bit" {
				return termenv.TrueColor
			}
		}
	}

	switch {
	case strings.Contains(term, "truecolor"), strings.Contains(term, "direct"):
		return termenv.TrueColor
	case strings.Contains(term, "256"):
		return termenv.ANSI256
	case term == "", term == "dumb":
		return termenv.Ascii
	default:
		return termenv.ANSI
	}
}
