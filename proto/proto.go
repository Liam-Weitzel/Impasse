// Package proto is the wire format between the server and its clients.
//
// Messages are JSON objects, one per line. The same format is used for the
// renderer over a unix socket and, later, for bots over TCP, so there is only
// ever one protocol to keep working.
package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Message type tags.
const (
	TypeAuth    = "auth"
	TypeWelcome = "welcome"
	TypeState   = "state"
	TypeQueue   = "queue"
	TypeError   = "error"
)

// Auth is the first message every client sends, before anything else. The
// server replies with a welcome, or an error and a closed connection.
//
// Humans never type a token. The SSH server mints a one shot one and hands it
// to the renderer it spawns. Bots use the long lived token from the account,
// which `ssh <host> token` prints.
type Auth struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

// Error is sent when the server is about to hang up, so a client can say why
// rather than reporting a bare EOF.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Welcome is the first thing the server sends. It carries everything that does
// not change for the life of the session.
type Welcome struct {
	Type   string `json:"type"`
	ID     uint64 `json:"id"`
	TickMS int    `json:"tick_ms"`
	// LootTicks is how many ticks of channelling a pickup takes. Sent rather
	// than assumed so clients do not carry their own copy of a server rule.
	LootTicks int `json:"loot_ticks"`
	// StunTicks is how long a stun holds a victim, StunCooldownTicks how long
	// before the caster can go again, and StunRadius how far the burst
	// reaches in cells.
	StunTicks         int      `json:"stun_ticks"`
	StunCooldownTicks int      `json:"stun_cooldown_ticks"`
	StunRadius        int      `json:"stun_radius"`
	Map               []string `json:"map"`
}

// Action kinds a client can queue.
const (
	ActionMove = "move"
	ActionLoot = "loot"
	ActionStun = "stun"
)

// Player is one player as of the last resolved tick.
type Player struct {
	ID uint64 `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	// Score is how many objectives this player has collected.
	Score int `json:"score"`
	// Channel is how many ticks of loot progress they hold, 0 when not
	// looting. It is visible to everyone, because who is close to taking a
	// pickup is exactly what opponents need to decide whether to interfere.
	Channel int `json:"channel"`
	// Stunned is how many more ticks they cannot act for, 0 when free.
	Stunned int `json:"stunned"`
	// StunCD is how many ticks until they can cast a stun again.
	StunCD int `json:"stun_cd"`
	// Casting is how many ticks until a burst they have already cast lands,
	// 0 when they are not winding one up.
	//
	// This is resolved state, not queued intent. The cast already happened on
	// a previous tick, and it lands whatever anyone does now, so publishing
	// it gives nothing away about what anyone intends to do next. It lets
	// bystanders see an incoming burst and decide whether to close in.
	Casting int `json:"casting"`
}

// Objective is an uncollected pickup. Collected ones are simply absent.
type Objective struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Match phases.
const (
	// PhaseIntermission is between matches. Scores from the match just
	// finished stay up, and there is nothing to collect.
	PhaseIntermission = "intermission"
	// PhaseRunning is a live match.
	PhaseRunning = "running"
)

// Match is where the round cycle has got to. Everyone gets it, humans and bots
// alike, because how long is left decides whether a pickup across the map is
// worth starting for.
type Match struct {
	Phase string `json:"phase"`
	// Number counts matches since the server started.
	Number int `json:"number"`
	// TicksRemaining is how much of this phase is left. Ticks rather than
	// seconds, because the game is tick based and clients already know how
	// long a tick is from the welcome.
	TicksRemaining int `json:"ticks_remaining"`
}

// State is the authoritative world after a tick has resolved. The server sends
// one per tick.
type State struct {
	Type       string      `json:"type"`
	Tick       uint64      `json:"tick"`
	Match      Match       `json:"match"`
	Players    []Player    `json:"players"`
	Objectives []Objective `json:"objectives"`
}

// Queue is a client asking for an action on the next tick. Sending another one
// before the tick locks replaces the first.
//
// A move is consumed by the tick it resolves on. A loot persists until the
// player queues something else, finishes, or loses the channel, so holding a
// pickup does not mean spamming the key.
type Queue struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	// Dir is only read for a move.
	Dir string `json:"dir,omitempty"`
}

// Writer serialises messages onto a stream.
type Writer struct {
	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write encodes one message followed by a newline. Encoding to a buffer first
// keeps a failed encode from emitting a half written line.
func (pw *Writer) Write(msg any) error {
	line, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = pw.w.Write(line)
	return err
}

// Reader pulls messages off a stream one line at a time.
type Reader struct {
	sc *bufio.Scanner
}

func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Reader{sc: sc}
}

// Next returns the type tag and the raw line. Blank lines are skipped. It
// returns io.EOF when the stream ends.
func (pr *Reader) Next() (string, []byte, error) {
	for pr.sc.Scan() {
		line := pr.sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			return "", nil, fmt.Errorf("bad message: %w", err)
		}
		if envelope.Type == "" {
			return "", nil, fmt.Errorf("message has no type: %s", line)
		}

		// Scanner reuses its buffer, so hand back a copy.
		out := make([]byte, len(line))
		copy(out, line)

		return envelope.Type, out, nil
	}
	if err := pr.sc.Err(); err != nil {
		return "", nil, err
	}
	return "", nil, io.EOF
}

// Decode unpacks a raw line returned by Next.
func Decode(line []byte, msg any) error {
	return json.Unmarshal(line, msg)
}
