// Package grid holds the authoritative world model: a flat 2D array of cells.
// Everything the server simulates happens here. The 3D geometry players see is
// generated from this, never the other way round.
package grid

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Cell is what occupies one square of the world.
type Cell byte

const (
	Wall  Cell = '#'
	Floor Cell = '.'
	// Spawn is floor that also marks where players enter the world. A map
	// may hold at most one.
	Spawn Cell = 'S'
)

// Walkable reports whether a player may stand on this cell.
func (c Cell) Walkable() bool {
	return c == Floor || c == Spawn
}

// Grid is the world. Origin is the top left, x runs east, y runs south.
type Grid struct {
	width  int
	height int
	cells  []Cell

	spawn    Pos
	hasSpawn bool
}

// Spawn returns the cell marked S, and whether the map had one.
func (g *Grid) Spawn() (Pos, bool) {
	return g.spawn, g.hasSpawn
}

// Pos is a cell coordinate.
type Pos struct {
	X int
	Y int
}

func (g *Grid) Width() int  { return g.width }
func (g *Grid) Height() int { return g.height }

// Contains reports whether p is inside the grid.
func (g *Grid) Contains(p Pos) bool {
	return p.X >= 0 && p.X < g.width && p.Y >= 0 && p.Y < g.height
}

// At returns the cell at p. Out of bounds reads as Wall so callers do not have
// to bounds check before asking about walkability.
func (g *Grid) At(p Pos) Cell {
	if !g.Contains(p) {
		return Wall
	}
	return g.cells[p.Y*g.width+p.X]
}

// Walkable reports whether p is inside the grid and standable.
func (g *Grid) Walkable(p Pos) bool {
	return g.At(p).Walkable()
}

// Direction is one of the eight movement directions, plus None.
type Direction int

const (
	None Direction = iota
	North
	NorthEast
	East
	SouthEast
	South
	SouthWest
	West
	NorthWest
)

// deltas is indexed by Direction.
var deltas = [...]Pos{
	None:      {0, 0},
	North:     {0, -1},
	NorthEast: {1, -1},
	East:      {1, 0},
	SouthEast: {1, 1},
	South:     {0, 1},
	SouthWest: {-1, 1},
	West:      {-1, 0},
	NorthWest: {-1, -1},
}

// Directions lists the eight movement directions, excluding None.
var Directions = [...]Direction{
	North, NorthEast, East, SouthEast,
	South, SouthWest, West, NorthWest,
}

// keys maps the human movement keys to directions. The letters form a 3x3 block
// on the keyboard with S in the middle, so the layout matches the directions.
var keys = map[rune]Direction{
	'q': NorthWest, 'w': North, 'e': NorthEast,
	'a': West, 'd': East,
	'z': SouthWest, 'x': South, 'c': SouthEast,
}

// DirectionForKey maps a movement key to its direction. Returns None for keys
// that do not move.
func DirectionForKey(r rune) Direction {
	if d, ok := keys[r]; ok {
		return d
	}
	return None
}

// Delta returns the cell offset for a direction.
func (d Direction) Delta() Pos {
	if d < None || int(d) >= len(deltas) {
		return deltas[None]
	}
	return deltas[d]
}

// Diagonal reports whether moving in this direction changes both axes.
func (d Direction) Diagonal() bool {
	delta := d.Delta()
	return delta.X != 0 && delta.Y != 0
}

var dirNames = [...]string{
	None: "none", North: "n", NorthEast: "ne", East: "e", SouthEast: "se",
	South: "s", SouthWest: "sw", West: "w", NorthWest: "nw",
}

func (d Direction) String() string {
	if d < None || int(d) >= len(dirNames) {
		return "invalid"
	}
	return dirNames[d]
}

// ParseDirection is the inverse of String, for the wire protocol.
func ParseDirection(s string) (Direction, bool) {
	for d, name := range dirNames {
		if name == s {
			return Direction(d), true
		}
	}
	return None, false
}

// Move applies a direction to a position and reports the resulting position
// along with whether the move is legal.
//
// A diagonal needs both adjoining orthogonal cells to be open. Without that a
// player could slip between two walls meeting at a corner, which the generated
// geometry would show as walking through solid wall.
func (g *Grid) Move(from Pos, d Direction) (Pos, bool) {
	if d == None {
		return from, true
	}

	delta := d.Delta()
	to := Pos{X: from.X + delta.X, Y: from.Y + delta.Y}

	if !g.Walkable(to) {
		return from, false
	}

	if d.Diagonal() {
		sideX := Pos{X: from.X + delta.X, Y: from.Y}
		sideY := Pos{X: from.X, Y: from.Y + delta.Y}
		if !g.Walkable(sideX) || !g.Walkable(sideY) {
			return from, false
		}
	}

	return to, true
}

// Parse reads an ASCII map. Every line is a row, '#' is wall, '.' is floor and
// 'S' is the spawn point. Short rows are padded with wall so ragged files still
// load. Blank leading and trailing lines are ignored.
func Parse(r io.Reader) (*Grid, error) {
	var rows [][]Cell

	spawn := Pos{}
	hasSpawn := false

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if len(rows) == 0 && line == "" {
			continue
		}
		row := make([]Cell, len(line))
		for i, r := range line {
			switch Cell(r) {
			case Spawn:
				if hasSpawn {
					return nil, fmt.Errorf(
						"map has more than one spawn point, second at row %d column %d",
						len(rows)+1, i+1)
				}
				spawn = Pos{X: i, Y: len(rows)}
				hasSpawn = true
				row[i] = Spawn
			case Wall, Floor:
				row[i] = Cell(r)
			default:
				return nil, fmt.Errorf(
					"unknown map character %q at row %d column %d",
					r, len(rows)+1, i+1)
			}
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	for len(rows) > 0 && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
	}
	if len(rows) == 0 {
		return nil, errors.New("map is empty")
	}

	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}

	g := &Grid{
		width:    width,
		height:   len(rows),
		cells:    make([]Cell, width*len(rows)),
		spawn:    spawn,
		hasSpawn: hasSpawn,
	}
	for y, row := range rows {
		for x := range g.cells[y*width : (y+1)*width] {
			if x < len(row) {
				g.cells[y*width+x] = row[x]
			} else {
				g.cells[y*width+x] = Wall
			}
		}
	}

	return g, nil
}

// Lines renders the grid back to the ASCII form Parse accepts. This is how the
// map reaches clients.
func (g *Grid) Lines() []string {
	out := make([]string, g.height)
	row := make([]byte, g.width)
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			row[x] = byte(g.cells[y*g.width+x])
		}
		out[y] = string(row)
	}
	return out
}

// Walkables lists every walkable cell, in reading order, whether or not a
// player could ever get to it.
func (g *Grid) Walkables() []Pos {
	var out []Pos
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			p := Pos{X: x, Y: y}
			if g.Walkable(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// Reachable returns every cell reachable from start, in reading order, using
// the real movement rules. Corner cutting is refused, so a diagonal gap counts
// as closed here exactly as it does in play.
func (g *Grid) Reachable(start Pos) []Pos {
	if !g.Walkable(start) {
		return nil
	}

	seen := map[Pos]bool{start: true}
	queue := []Pos{start}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		for _, d := range Directions {
			to, ok := g.Move(p, d)
			if !ok || seen[to] {
				continue
			}
			seen[to] = true
			queue = append(queue, to)
		}
	}

	return inReadingOrder(seen)
}

// LargestRegion returns the biggest group of mutually reachable cells. A map
// with sealed off pockets would otherwise strand any player spawned in one.
func (g *Grid) LargestRegion() []Pos {
	seen := map[Pos]bool{}
	var best []Pos

	for _, p := range g.Walkables() {
		if seen[p] {
			continue
		}
		region := g.Reachable(p)
		for _, q := range region {
			seen[q] = true
		}
		if len(region) > len(best) {
			best = region
		}
	}

	return best
}

func inReadingOrder(set map[Pos]bool) []Pos {
	out := make([]Pos, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}
