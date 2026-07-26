package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// testMenu builds a menu against a real server and store, without a terminal.
func testMenu(t *testing.T) (menuModel, *server, *account) {
	t.Helper()

	srv, _ := testServer(t)
	srv.store = testStore(t)

	user := testUser()
	acc := srv.accounts.forGitHub(user)
	if _, err := srv.store.Ensure(user); err != nil {
		t.Fatalf("ensuring player: %v", err)
	}

	renderer := lipgloss.NewRenderer(&strings.Builder{},
		termenv.WithProfile(termenv.Ascii))

	return newMenuModel(srv, "SHA256:test", acc, ":2223", "impasse.example", renderer), srv, acc
}

// press sends a key and returns the updated model.
func press(t *testing.T, m menuModel, key string) menuModel {
	t.Helper()

	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}

	next, _ := m.Update(msg)
	got, ok := next.(menuModel)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return got
}

func TestMenuStartsOnPlay(t *testing.T) {
	m, _, _ := testMenu(t)

	if m.cursor != 0 {
		t.Errorf("cursor at %d, want the first item", m.cursor)
	}
	if menuItems[m.cursor].label != "Play" {
		t.Errorf("first item is %q, want Play", menuItems[m.cursor].label)
	}
}

func TestMenuPlayChoosesToPlay(t *testing.T) {
	m, _, _ := testMenu(t)

	m = press(t, m, "enter")

	if !m.done {
		t.Fatal("menu did not finish")
	}
	if m.choice != choicePlay {
		t.Errorf("choice %v, want play", m.choice)
	}
}

func TestMenuQuitChoosesToQuit(t *testing.T) {
	m, _, _ := testMenu(t)

	// Walk to Quit, the last item.
	for i := 0; i < len(menuItems)-1; i++ {
		m = press(t, m, "down")
	}
	if menuItems[m.cursor].label != "Quit" {
		t.Fatalf("landed on %q, want Quit", menuItems[m.cursor].label)
	}

	m = press(t, m, "enter")

	if !m.done || m.choice != choiceQuit {
		t.Errorf("done %v choice %v, want quit", m.done, m.choice)
	}
}

// q quits from the main menu, so a player is never stuck in it.
func TestMenuQKeyQuits(t *testing.T) {
	m, _, _ := testMenu(t)

	m = press(t, m, "q")

	if !m.done || m.choice != choiceQuit {
		t.Errorf("done %v choice %v, want quit", m.done, m.choice)
	}
}

func TestMenuCursorStaysInRange(t *testing.T) {
	m, _, _ := testMenu(t)

	for i := 0; i < 20; i++ {
		m = press(t, m, "up")
	}
	if m.cursor != 0 {
		t.Errorf("cursor %d after pressing up repeatedly, want 0", m.cursor)
	}

	for i := 0; i < 40; i++ {
		m = press(t, m, "down")
	}
	if m.cursor != len(menuItems)-1 {
		t.Errorf("cursor %d, want the last item %d", m.cursor, len(menuItems)-1)
	}
}

// esc backs out of a pane rather than leaving the game.
func TestEscapeLeavesAPaneNotTheMenu(t *testing.T) {
	m, _, _ := testMenu(t)

	m = press(t, m, "down") // Leaderboard
	m = press(t, m, "enter")
	if m.pane != paneLeaderboard {
		t.Fatalf("pane %v, want the leaderboard", m.pane)
	}

	m = press(t, m, "esc")
	if m.pane != paneMain {
		t.Errorf("pane %v, want back on the main menu", m.pane)
	}
	if m.done {
		t.Error("escaping a pane quit the menu")
	}
}

func TestRenamingPersists(t *testing.T) {
	m, _, acc := testMenu(t)

	// Walk to the rename item and open it.
	for menuItems[m.cursor].label != "Change display name" {
		m = press(t, m, "down")
	}
	m = press(t, m, "enter")
	if m.pane != paneName {
		t.Fatalf("pane %v, want the name editor", m.pane)
	}

	// Clear whatever is prefilled, then type.
	for i := 0; i < maxNameLength+4; i++ {
		m = press(t, m, "backspace")
	}
	for _, r := range "liam" {
		m = press(t, m, string(r))
	}
	m = press(t, m, "enter")

	if m.pane != paneMain {
		t.Errorf("pane %v, want back on the main menu after saving", m.pane)
	}

	stored, err := m.store.Get(acc.githubID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Name != "liam" {
		t.Errorf("stored name %q, want liam", stored.Name)
	}
}

// An empty name is refused rather than silently stored.
func TestEmptyNameIsRefused(t *testing.T) {
	m, _, acc := testMenu(t)

	before, err := m.store.Get(acc.githubID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	for menuItems[m.cursor].label != "Change display name" {
		m = press(t, m, "down")
	}
	m = press(t, m, "enter")

	for i := 0; i < maxNameLength+4; i++ {
		m = press(t, m, "backspace")
	}
	m = press(t, m, "enter")

	if m.pane != paneName {
		t.Error("an empty name was accepted and closed the editor")
	}
	if m.notes == "" {
		t.Error("no explanation was shown")
	}

	after, err := m.store.Get(acc.githubID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Name != before.Name {
		t.Errorf("name changed to %q despite being empty", after.Name)
	}
}

// The clock in the menu says the same thing the in game HUD does.
func TestMenuClockTracksThePhase(t *testing.T) {
	m, srv, _ := testMenu(t)

	srv.world.tickDuration = time.Second
	m.tickMS = 1000

	m.lobby.Match.Phase = "intermission"
	m.lobby.Match.TicksRemaining = 15
	if got := m.clock(); !strings.Contains(got, "0:15") {
		t.Errorf("clock %q, want a 15 second countdown", got)
	}
	if !strings.Contains(m.clock(), "Next match") {
		t.Errorf("clock %q, want it to say the next match", m.clock())
	}

	m.lobby.Match.Phase = "running"
	m.lobby.Match.Number = 3
	m.lobby.Match.TicksRemaining = 125
	got := m.clock()
	if !strings.Contains(got, "2:05") {
		t.Errorf("clock %q, want 2:05 remaining", got)
	}
	if !strings.Contains(got, "3") {
		t.Errorf("clock %q, want the match number", got)
	}
}

// Every pane has to render without panicking, including with nothing in it.
func TestEveryPaneRenders(t *testing.T) {
	m, _, _ := testMenu(t)

	for _, p := range []pane{
		paneMain, paneLeaderboard, panePlayers, paneName, paneToken,
	} {
		m.pane = p
		if out := m.View(); out == "" {
			t.Errorf("pane %v rendered nothing", p)
		}
	}
}

// The leaderboard shows what the store holds, and marks the player's own row.
func TestLeaderboardPaneShowsScores(t *testing.T) {
	m, _, acc := testMenu(t)

	m.store.SetName(acc.githubID, "me")
	m.store.RecordMatch(acc.githubID, 4)
	rival := GitHubUser{ID: 9999, Login: "rival"}
	m.store.Ensure(rival)
	m.store.SetName(rival.ID, "rival")
	m.store.RecordMatch(rival.ID, 9)
	m.reload()

	m.pane = paneLeaderboard
	out := m.View()

	for _, want := range []string{"rival", "me", "9", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("leaderboard is missing %q:\n%s", want, out)
		}
	}
}

// The live list distinguishes a character being played from one being botted.
func TestPlayersPaneNamesTheDriver(t *testing.T) {
	m, _, acc := testMenu(t)

	m.lobby.Players = []lobbyPlayer{
		{GitHubID: acc.githubID, Login: "me", Score: 2, HasTerminal: true, HasBot: true},
		{GitHubID: 4242, Login: "other", Score: 1, HasBot: true},
	}

	m.pane = panePlayers
	out := m.View()

	if !strings.Contains(out, "terminal and bot") {
		t.Errorf("did not show a character driven by both:\n%s", out)
	}
	if !strings.Contains(out, "bot") {
		t.Errorf("did not show the bot only character:\n%s", out)
	}
}

func TestTokenPaneShowsTheToken(t *testing.T) {
	m, srv, acc := testMenu(t)

	token := srv.accounts.botToken(acc)

	m.pane = paneToken
	out := m.View()

	if !strings.Contains(out, token) {
		t.Errorf("token pane does not show the token:\n%s", out)
	}
}

// With the bot API switched off, the pane says so rather than offering a token
// that leads nowhere.
func TestTokenPaneWithoutABotAPI(t *testing.T) {
	m, _, _ := testMenu(t)
	m.botAddr = ""

	m.pane = paneToken
	out := m.View()

	if !strings.Contains(out, "switched off") {
		t.Errorf("did not say the bot API is off:\n%s", out)
	}
}

func TestColorProfileFromTerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		term string
		env  []string
		want termenv.Profile
	}{
		{"truecolor env", "xterm-256color", []string{"COLORTERM=truecolor"}, termenv.TrueColor},
		{"24bit env", "xterm", []string{"COLORTERM=24bit"}, termenv.TrueColor},
		{"256 term", "xterm-256color", nil, termenv.ANSI256},
		{"plain term", "xterm", nil, termenv.ANSI},
		{"dumb term", "dumb", nil, termenv.Ascii},
		{"no term", "", nil, termenv.Ascii},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := colorProfile(tc.term, tc.env); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q, want it untouched", got)
	}
	if got := truncate("averylongnamehere", 8); len(got) != 8 {
		t.Errorf("got %q, want 8 characters", got)
	}
}

// bubbletea folds runes arriving in one read into a single message, so typing
// quickly or pasting produces one KeyMsg holding several runes. Before this was
// handled, "jj" moved the cursor nowhere at all and the keys were lost.
func TestMenuHandlesRunesArrivingTogether(t *testing.T) {
	m, _, _ := testMenu(t)

	burst := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")}
	updated, _ := m.Update(burst)

	got, ok := updated.(menuModel)
	if !ok {
		t.Fatalf("got %T, want a menuModel", updated)
	}
	if got.cursor != 2 {
		t.Errorf("cursor at %d, want 2: a two rune burst must move twice", got.cursor)
	}
}

// The same burst typed into the name field has to land as text, not be split
// into menu commands.
func TestMenuNameFieldTakesRunesArrivingTogether(t *testing.T) {
	m, _, _ := testMenu(t)
	m.pane = paneName
	m.name.Focus()
	m.name.SetValue("")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})

	got, ok := updated.(menuModel)
	if !ok {
		t.Fatalf("got %T, want a menuModel", updated)
	}
	if got.name.Value() != "abc" {
		t.Errorf("name is %q, want %q", got.name.Value(), "abc")
	}
}
