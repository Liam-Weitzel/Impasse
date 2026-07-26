package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *store {
	t.Helper()

	s, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEnsureCreatesThenReturns(t *testing.T) {
	s := testStore(t)
	user := GitHubUser{ID: 1, Login: "liam"}

	first, err := s.Ensure(user)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first.Name != "liam" {
		t.Errorf("name %q, want the GitHub login", first.Name)
	}
	if first.Matches != 0 || first.Best != 0 {
		t.Errorf("new player starts at %+v, want zeroes", first)
	}

	second, err := s.Ensure(user)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.Name != first.Name {
		t.Errorf("name changed on re-ensure: %q then %q", first.Name, second.Name)
	}
}

// A GitHub rename must follow, since the numeric id is the identity.
func TestEnsureRefreshesTheLogin(t *testing.T) {
	s := testStore(t)

	s.Ensure(GitHubUser{ID: 3, Login: "oldname"})
	got, err := s.Ensure(GitHubUser{ID: 3, Login: "newname"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.GitHubLogin != "newname" {
		t.Errorf("login %q, want it refreshed", got.GitHubLogin)
	}
}

// Everything a player has must survive a restart.
func TestDataSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	user := GitHubUser{ID: 2, Login: "liam"}

	s, err := openStore(path)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Ensure(user); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	s.SetName(user.ID, "chosen")
	s.RecordMatch(user.ID, 7)
	s.SetBotToken(user.ID, "tok")
	s.LinkKey("SHA256:laptop", user.ID)
	s.Close()

	again, err := openStore(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer again.Close()

	got, err := again.Get(user.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "chosen" {
		t.Errorf("name %q, want chosen", got.Name)
	}
	if got.Best != 7 || got.Total != 7 || got.Matches != 1 {
		t.Errorf("got %+v, want best 7 total 7 matches 1", got)
	}
	if tok, ok := again.BotToken(user.ID); !ok || tok != "tok" {
		t.Errorf("token (%q, %v), want tok", tok, ok)
	}
	if _, ok := again.PlayerForKey("SHA256:laptop"); !ok {
		t.Error("the remembered key was forgotten")
	}
}

// Best is the high water mark, total accumulates.
func TestRecordMatchTracksBestAndTotal(t *testing.T) {
	s := testStore(t)
	user := GitHubUser{ID: 4, Login: "p"}
	s.Ensure(user)

	for _, score := range []int{3, 9, 1} {
		if err := s.RecordMatch(user.ID, score); err != nil {
			t.Fatalf("record %d: %v", score, err)
		}
	}

	got, err := s.Get(user.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Best != 9 || got.Total != 13 || got.Matches != 3 {
		t.Errorf("got %+v, want best 9 total 13 matches 3", got)
	}
}

func TestLeaderboardRanksByBest(t *testing.T) {
	s := testStore(t)

	for i, tc := range []struct {
		name  string
		score int
	}{{"low", 2}, {"high", 11}, {"mid", 5}} {
		user := GitHubUser{ID: int64(100 + i), Login: tc.name}
		s.Ensure(user)
		s.SetName(user.ID, tc.name)
		if err := s.RecordMatch(user.ID, tc.score); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	board, err := s.Leaderboard(10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 3 {
		t.Fatalf("%d entries, want 3", len(board))
	}
	for i, want := range []string{"high", "mid", "low"} {
		if board[i].Name != want {
			t.Errorf("place %d is %q, want %q", i+1, board[i].Name, want)
		}
	}
}

// Someone who has never finished a match does not belong on the board.
func TestLeaderboardSkipsPlayersWithNoMatches(t *testing.T) {
	s := testStore(t)

	played := GitHubUser{ID: 200, Login: "played"}
	s.Ensure(played)
	s.RecordMatch(played.ID, 1)
	s.Ensure(GitHubUser{ID: 201, Login: "lurker"})

	board, err := s.Leaderboard(10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 1 || board[0].GitHubID != played.ID {
		t.Fatalf("board is %v, want only the one who played", board)
	}
}

func TestLeaderboardRespectsLimit(t *testing.T) {
	s := testStore(t)

	for i := 0; i < 10; i++ {
		user := GitHubUser{ID: int64(300 + i), Login: "p"}
		s.Ensure(user)
		s.RecordMatch(user.ID, i)
	}

	board, err := s.Leaderboard(3)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 3 {
		t.Errorf("%d entries, want 3", len(board))
	}
}

// A second machine reaches the same player. That is the point: one person, one
// character, however many machines they own.
func TestSecondKeyReachesTheSamePlayer(t *testing.T) {
	s := testStore(t)
	user := GitHubUser{ID: 500, Login: "liam"}

	s.Ensure(user)
	s.SetName(user.ID, "liam")
	s.RecordMatch(user.ID, 6)

	s.LinkKey("SHA256:laptop", user.ID)
	s.LinkKey("SHA256:desktop", user.ID)

	for _, fp := range []string{"SHA256:laptop", "SHA256:desktop"} {
		got, ok := s.PlayerForKey(fp)
		if !ok {
			t.Fatalf("%s does not resolve", fp)
		}
		if got.GitHubID != user.ID {
			t.Errorf("%s resolved to %d, want %d", fp, got.GitHubID, user.ID)
		}
		if got.Best != 6 {
			t.Errorf("%s sees best %d, want 6", fp, got.Best)
		}
	}
}

// A key can move, since whoever holds it just proved they control the account
// they signed into.
func TestKeyCanMoveBetweenPlayers(t *testing.T) {
	s := testStore(t)

	s.Ensure(GitHubUser{ID: 601, Login: "one"})
	s.Ensure(GitHubUser{ID: 602, Login: "two"})

	s.LinkKey("SHA256:shared", 601)
	s.LinkKey("SHA256:shared", 602)

	got, ok := s.PlayerForKey("SHA256:shared")
	if !ok {
		t.Fatal("key no longer resolves")
	}
	if got.GitHubID != 602 {
		t.Errorf("github id %d, want 602", got.GitHubID)
	}
}

func TestUnknownKeyDoesNotResolve(t *testing.T) {
	s := testStore(t)
	if _, ok := s.PlayerForKey("SHA256:stranger"); ok {
		t.Error("an unknown key resolved to a player")
	}
}

func TestBotTokenRoundTrips(t *testing.T) {
	s := testStore(t)
	user := GitHubUser{ID: 700, Login: "p"}

	if _, ok := s.BotToken(user.ID); ok {
		t.Error("reported a token for a player with none")
	}

	s.Ensure(user)
	if err := s.SetBotToken(user.ID, "abc123"); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, ok := s.BotToken(user.ID)
	if !ok || got != "abc123" {
		t.Errorf("got (%q, %v), want abc123", got, ok)
	}
}

func TestDefaultName(t *testing.T) {
	if got := defaultName("liam"); got != "liam" {
		t.Errorf("got %q, want the login", got)
	}
	if got := defaultName(""); got == "" {
		t.Error("an empty login produced an empty name")
	}
}

// A nil store is what a server without persistence has, and it must not panic.
func TestNilStoreIsSafe(t *testing.T) {
	var s *store

	if err := s.RecordMatch(1, 1); err != nil {
		t.Errorf("RecordMatch on a nil store: %v", err)
	}
	if board, err := s.Leaderboard(10); err != nil || board != nil {
		t.Errorf("Leaderboard on a nil store: %v %v", board, err)
	}
	if _, ok := s.PlayerForKey("x"); ok {
		t.Error("PlayerForKey on a nil store returned something")
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close on a nil store: %v", err)
	}
}

// Opening an old database drops the pre-GitHub tables rather than failing.
func TestOldSchemaIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening raw: %v", err)
	}
	if _, err := old.Exec(`
		CREATE TABLE accounts (fingerprint TEXT PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO accounts VALUES ('SHA256:old', 'veteran');
	`); err != nil {
		t.Fatalf("building the old schema: %v", err)
	}
	old.Close()

	s, err := openStore(path)
	if err != nil {
		t.Fatalf("opening a pre-GitHub database: %v", err)
	}
	defer s.Close()

	if _, err := s.Ensure(GitHubUser{ID: 1, Login: "new"}); err != nil {
		t.Errorf("the new schema does not work: %v", err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.db")

	for i := 0; i < 3; i++ {
		s, err := openStore(path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		s.Close()
	}
}
