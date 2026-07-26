package main

import (
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

	first, err := s.Ensure("SHA256:aaa")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if first.Name == "" {
		t.Error("new account has no default name")
	}
	if first.Matches != 0 || first.Best != 0 {
		t.Errorf("new account starts at %+v, want zeroes", first)
	}

	second, err := s.Ensure("SHA256:aaa")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.Name != first.Name {
		t.Errorf("name changed on re-ensure: %q then %q", first.Name, second.Name)
	}
}

// Opening an existing database must keep what is in it. This is the whole point
// of persisting.
func TestDataSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s, err := openStore(path)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Ensure("SHA256:bbb"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := s.SetName("SHA256:bbb", "liam"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := s.RecordMatch("SHA256:bbb", 7); err != nil {
		t.Fatalf("record: %v", err)
	}
	s.Close()

	again, err := openStore(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer again.Close()

	got, err := again.Get("SHA256:bbb")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "liam" {
		t.Errorf("name %q, want liam", got.Name)
	}
	if got.Best != 7 || got.Total != 7 || got.Matches != 1 {
		t.Errorf("got %+v, want best 7 total 7 matches 1", got)
	}
}

// Best is the high water mark, total accumulates. A worse match must not lower
// the best.
func TestRecordMatchTracksBestAndTotal(t *testing.T) {
	s := testStore(t)
	s.Ensure("SHA256:ccc")

	for _, score := range []int{3, 9, 1} {
		if err := s.RecordMatch("SHA256:ccc", score); err != nil {
			t.Fatalf("record %d: %v", score, err)
		}
	}

	got, err := s.Get("SHA256:ccc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Best != 9 {
		t.Errorf("best %d, want 9", got.Best)
	}
	if got.Total != 13 {
		t.Errorf("total %d, want 13", got.Total)
	}
	if got.Matches != 3 {
		t.Errorf("matches %d, want 3", got.Matches)
	}
}

func TestLeaderboardRanksByBest(t *testing.T) {
	s := testStore(t)

	for _, tc := range []struct {
		fp    string
		name  string
		score int
	}{
		{"SHA256:1", "low", 2},
		{"SHA256:2", "high", 11},
		{"SHA256:3", "mid", 5},
	} {
		s.Ensure(tc.fp)
		s.SetName(tc.fp, tc.name)
		if err := s.RecordMatch(tc.fp, tc.score); err != nil {
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

	s.Ensure("SHA256:played")
	s.RecordMatch("SHA256:played", 1)
	s.Ensure("SHA256:lurker")

	board, err := s.Leaderboard(10)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 1 {
		t.Fatalf("%d entries, want only the one who played", len(board))
	}
	if board[0].Fingerprint != "SHA256:played" {
		t.Errorf("wrong account on the board: %+v", board[0])
	}
}

func TestLeaderboardRespectsLimit(t *testing.T) {
	s := testStore(t)

	for i := 0; i < 10; i++ {
		fp := string(rune('a'+i)) + "-key"
		s.Ensure(fp)
		s.RecordMatch(fp, i)
	}

	board, err := s.Leaderboard(3)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(board) != 3 {
		t.Errorf("%d entries, want 3", len(board))
	}
}

func TestDefaultNameIsStableAndShort(t *testing.T) {
	fp := "SHA256:6YLbYoaaWWwbIRefaN9OmtBLxQZHp8rD1ox0AIj3/pA"

	first := defaultName(fp)
	if first != defaultName(fp) {
		t.Error("default name is not stable")
	}
	if len(first) > 20 {
		t.Errorf("default name %q is too long for a leaderboard", first)
	}
	if defaultName(fp) == defaultName("SHA256:somethingelse") {
		t.Error("two fingerprints share a default name")
	}
}

// A nil store is what a server without persistence has, and it must not panic.
func TestNilStoreIsSafe(t *testing.T) {
	var s *store

	if err := s.RecordMatch("SHA256:x", 1); err != nil {
		t.Errorf("RecordMatch on a nil store: %v", err)
	}
	if board, err := s.Leaderboard(10); err != nil || board != nil {
		t.Errorf("Leaderboard on a nil store: %v %v", board, err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close on a nil store: %v", err)
	}
}

// A result must land even for an account that was never written first. The
// original UPDATE based version matched zero rows and lost the score without
// an error, which only showed up when a real match ended.
func TestRecordMatchWithoutEnsure(t *testing.T) {
	s := testStore(t)

	if err := s.RecordMatch("SHA256:never-seen", 5); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := s.Get("SHA256:never-seen")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Best != 5 || got.Total != 5 || got.Matches != 1 {
		t.Errorf("got %+v, want best 5 total 5 matches 1", got)
	}
	if got.Name == "" {
		t.Error("account created by RecordMatch has no name")
	}
}

// An existing name must not be clobbered by a later result.
func TestRecordMatchKeepsTheChosenName(t *testing.T) {
	s := testStore(t)

	s.Ensure("SHA256:named")
	if err := s.SetName("SHA256:named", "liam"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := s.RecordMatch("SHA256:named", 3); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := s.Get("SHA256:named")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "liam" {
		t.Errorf("name %q, want liam", got.Name)
	}
}
