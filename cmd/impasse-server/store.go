package main

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// store keeps accounts and their scores across restarts.
//
// The driver is the pure Go one, so building the server does not need cgo for
// this on top of the cgo the renderer already needs for SDL.
type store struct {
	db *sql.DB
}

// schema is applied on open. Every statement is idempotent, so opening an
// existing database is the same as opening a new one.
const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	fingerprint TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	best        INTEGER NOT NULL DEFAULT 0,
	total       INTEGER NOT NULL DEFAULT 0,
	matches     INTEGER NOT NULL DEFAULT 0,
	created     INTEGER NOT NULL DEFAULT (unixepoch()),
	seen        INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS accounts_best ON accounts(best DESC);
`

func openStore(path string) (*store, error) {
	// Busy timeout because the tick loop and the menu both touch this, and
	// waiting briefly beats failing.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &store{db: db}, nil
}

func (s *store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Account is a stored player.
type Account struct {
	Fingerprint string
	Name        string
	Best        int
	Total       int
	Matches     int
}

// defaultName is what a new account is called before the player picks
// something. Derived from the fingerprint so it is stable and unmistakable.
func defaultName(fingerprint string) string {
	trimmed := fingerprint
	if len(trimmed) > 8 {
		trimmed = trimmed[len(trimmed)-8:]
	}
	return "player-" + trimmed
}

// Ensure returns the stored account for a key, creating it on first sight and
// bumping the last seen time.
func (s *store) Ensure(fingerprint string) (Account, error) {
	if s == nil {
		return Account{}, errors.New("no store")
	}

	if _, err := s.db.Exec(`
		INSERT INTO accounts (fingerprint, name) VALUES (?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET seen = unixepoch()
	`, fingerprint, defaultName(fingerprint)); err != nil {
		return Account{}, err
	}

	return s.Get(fingerprint)
}

func (s *store) Get(fingerprint string) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT fingerprint, name, best, total, matches
		FROM accounts WHERE fingerprint = ?
	`, fingerprint).Scan(&a.Fingerprint, &a.Name, &a.Best, &a.Total, &a.Matches)
	return a, err
}

// SetName changes the display name. The fingerprint stays the identity, so a
// name is cosmetic and two players may share one.
func (s *store) SetName(fingerprint, name string) error {
	_, err := s.db.Exec(
		`UPDATE accounts SET name = ?, seen = unixepoch() WHERE fingerprint = ?`,
		name, fingerprint)
	return err
}

// RecordMatch folds one finished match into an account's totals. Best is kept
// separately from total, because a leaderboard on total alone just ranks
// whoever left the client running longest.
//
// It inserts rather than updating, so a result cannot be silently dropped for
// an account that was never written. An UPDATE here matches zero rows and
// returns no error, which loses scores quietly.
func (s *store) RecordMatch(fingerprint string, score int) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO accounts (fingerprint, name, best, total, matches)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(fingerprint) DO UPDATE SET
			total   = total + excluded.total,
			matches = matches + 1,
			best    = MAX(best, excluded.best),
			seen    = unixepoch()
	`, fingerprint, defaultName(fingerprint), score, score)
	return err
}

// Leaderboard returns the top accounts by best match, then by total.
func (s *store) Leaderboard(limit int) ([]Account, error) {
	if s == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT fingerprint, name, best, total, matches
		FROM accounts
		WHERE matches > 0
		ORDER BY best DESC, total DESC, name ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(
			&a.Fingerprint, &a.Name, &a.Best, &a.Total, &a.Matches,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
