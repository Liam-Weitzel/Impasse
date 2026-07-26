// Package grid holds the authoritative world model: a flat 2D array of cells.
// Everything the server simulates happens here. The 3D geometry players see is
// generated from this, never the other way round.
package grid

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

// Cell is what occupies one square of the world.
type Cell byte

const (
	Wall  Cell = '#'
	Floor Cell = '.'
)

// Walkable reports whether a player may stand on this cell.
func (c Cell) Walkable() bool {
	return c == Floor
}

// Grid is the world. Origin is the top left, x runs east, y runs south.
type Grid struct {
	width  int
	height int
	cells  []Cell
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

// Parse reads an ASCII map. Every line is a row, '#' is wall and '.' is floor.
// Short rows are padded with wall so ragged files still load. Blank leading and
// trailing lines are ignored.
func Parse(r io.Reader) (*Grid, error) {
	var rows [][]Cell

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
		width:  width,
		height: len(rows),
		cells:  make([]Cell, width*len(rows)),
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

// Spawns lists every walkable cell, in reading order. Used to place players
// until objectives exist and dictate spawn placement.
func (g *Grid) Spawns() []Pos {
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
