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
//
// A player is a GitHub account. Keys are optional credentials that point at
// one, so a laptop and a desktop reach the same player, and having no key at
// all just means signing in again.
const schema = `
CREATE TABLE IF NOT EXISTS players (
	github_id    INTEGER PRIMARY KEY,
	github_login TEXT NOT NULL,
	name         TEXT NOT NULL,
	best         INTEGER NOT NULL DEFAULT 0,
	total        INTEGER NOT NULL DEFAULT 0,
	matches      INTEGER NOT NULL DEFAULT 0,
	bot_token    TEXT NOT NULL DEFAULT '',
	created      INTEGER NOT NULL DEFAULT (unixepoch()),
	seen         INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS players_best ON players(best DESC);

CREATE TABLE IF NOT EXISTS player_keys (
	fingerprint TEXT PRIMARY KEY,
	github_id   INTEGER NOT NULL,
	linked      INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS player_keys_github ON player_keys(github_id);
`

// migrations run on every open and have to be safe to repeat.
var migrations = []string{
	// The pre-GitHub tables keyed accounts on an SSH fingerprint, which is
	// not an identity any more. Nothing in them can be mapped to a GitHub
	// user without that person signing in, so they go. This only ever
	// discards pre-release test data.
	`DROP TABLE IF EXISTS accounts`,
	`DROP TABLE IF EXISTS keys`,
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("%s: %w", m, err)
		}
	}
	return nil
}

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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating %s: %w", path, err)
	}
	return &store{db: db}, nil
}

func (s *store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Account is a stored player, identified by their GitHub account.
type Account struct {
	GitHubID    int64
	GitHubLogin string
	Name        string
	Best        int
	Total       int
	Matches     int
}

// defaultName is what a new player is called before they pick something. Their
// GitHub login is the obvious choice, since that is who they are.
func defaultName(login string) string {
	if login == "" {
		return "player"
	}
	return login
}

// Ensure returns the player for a GitHub user, creating them on first sign in
// and keeping the login fresh, since GitHub logins can be renamed.
func (s *store) Ensure(user GitHubUser) (Account, error) {
	if s == nil {
		return Account{}, errors.New("no store")
	}

	if _, err := s.db.Exec(`
		INSERT INTO players (github_id, github_login, name) VALUES (?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			github_login = excluded.github_login,
			seen         = unixepoch()
	`, user.ID, user.Login, defaultName(user.Login)); err != nil {
		return Account{}, err
	}

	return s.Get(user.ID)
}

func (s *store) Get(githubID int64) (Account, error) {
	var a Account
	err := s.db.QueryRow(`
		SELECT github_id, github_login, name, best, total, matches
		FROM players WHERE github_id = ?
	`, githubID).Scan(&a.GitHubID, &a.GitHubLogin, &a.Name,
		&a.Best, &a.Total, &a.Matches)
	return a, err
}

// SetName changes the display name. GitHub is the identity underneath, so a
// name is cosmetic and two players may share one.
func (s *store) SetName(githubID int64, name string) error {
	_, err := s.db.Exec(
		`UPDATE players SET name = ?, seen = unixepoch() WHERE github_id = ?`,
		name, githubID)
	return err
}

// RecordMatch folds one finished match into a player's totals. Best is kept
// separately from total, because a leaderboard on total alone just ranks
// whoever left their bot running longest.
func (s *store) RecordMatch(githubID int64, score int) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(`
		UPDATE players
		SET total   = total + ?,
		    matches = matches + 1,
		    best    = MAX(best, ?),
		    seen    = unixepoch()
		WHERE github_id = ?
	`, score, score, githubID)
	return err
}

// BotToken returns a player's token, if they have one.
func (s *store) BotToken(githubID int64) (string, bool) {
	if s == nil {
		return "", false
	}
	var token string
	err := s.db.QueryRow(
		`SELECT bot_token FROM players WHERE github_id = ?`, githubID).Scan(&token)
	if err != nil || token == "" {
		return "", false
	}
	return token, true
}

// SetBotToken stores a player's token. Stored rather than kept in memory so a
// restart does not break every bot that is already configured.
func (s *store) SetBotToken(githubID int64, token string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE players SET bot_token = ? WHERE github_id = ?`, token, githubID)
	return err
}

// LinkKey remembers an SSH key so that machine can skip signing in next time.
// A key that pointed at somebody else moves, since whoever holds it just proved
// they control the GitHub account.
func (s *store) LinkKey(fingerprint string, githubID int64) error {
	if s == nil || fingerprint == "" {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO player_keys (fingerprint, github_id) VALUES (?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET
			github_id = excluded.github_id,
			linked    = unixepoch()
	`, fingerprint, githubID)
	return err
}

// PlayerForKey returns the player a remembered key belongs to.
func (s *store) PlayerForKey(fingerprint string) (Account, bool) {
	if s == nil || fingerprint == "" {
		return Account{}, false
	}

	var githubID int64
	if err := s.db.QueryRow(
		`SELECT github_id FROM player_keys WHERE fingerprint = ?`,
		fingerprint).Scan(&githubID); err != nil {
		return Account{}, false
	}

	a, err := s.Get(githubID)
	if err != nil {
		return Account{}, false
	}
	return a, true
}

// Leaderboard returns the top players by best match, then by total.
func (s *store) Leaderboard(limit int) ([]Account, error) {
	if s == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT github_id, github_login, name, best, total, matches
		FROM players
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
		if err := rows.Scan(&a.GitHubID, &a.GitHubLogin, &a.Name,
			&a.Best, &a.Total, &a.Matches); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
