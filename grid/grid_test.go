package grid

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Grid {
	t.Helper()
	g, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return g
}

func TestParse(t *testing.T) {
	g := mustParse(t, "###\n#.#\n###")

	if g.Width() != 3 || g.Height() != 3 {
		t.Fatalf("got %dx%d, want 3x3", g.Width(), g.Height())
	}
	if !g.Walkable(Pos{1, 1}) {
		t.Error("centre should be walkable")
	}
	if g.Walkable(Pos{0, 0}) {
		t.Error("corner should be wall")
	}
}

func TestParseRaggedRowsPadWithWall(t *testing.T) {
	g := mustParse(t, "####\n#.\n####")

	if g.Width() != 4 {
		t.Fatalf("width %d, want 4", g.Width())
	}
	if g.Walkable(Pos{3, 1}) {
		t.Error("padded cell should be wall")
	}
}

func TestParseRejectsUnknownCharacter(t *testing.T) {
	if _, err := Parse(strings.NewReader("##\n#X")); err == nil {
		t.Fatal("want error for unknown character")
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if _, err := Parse(strings.NewReader("")); err == nil {
		t.Fatal("want error for empty map")
	}
}

func TestOutOfBoundsReadsAsWall(t *testing.T) {
	g := mustParse(t, "..\n..")

	for _, p := range []Pos{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if g.Walkable(p) {
			t.Errorf("%v should not be walkable", p)
		}
	}
}

func TestMoveIntoWallIsRefused(t *testing.T) {
	g := mustParse(t, "###\n#.#\n###")

	for _, d := range []Direction{North, East, South, West} {
		got, ok := g.Move(Pos{1, 1}, d)
		if ok {
			t.Errorf("%v: move should be refused", d)
		}
		if got != (Pos{1, 1}) {
			t.Errorf("%v: position moved to %v on a refused move", d, got)
		}
	}
}

func TestMoveNoneStaysPut(t *testing.T) {
	g := mustParse(t, "...\n...\n...")

	got, ok := g.Move(Pos{1, 1}, None)
	if !ok || got != (Pos{1, 1}) {
		t.Fatalf("got %v %v, want {1 1} true", got, ok)
	}
}

func TestKeyMapping(t *testing.T) {
	want := map[rune]Direction{
		'w': North, 'a': West, 's': South, 'd': East,
	}
	for k, d := range want {
		if got := DirectionForKey(k); got != d {
			t.Errorf("key %q: got %v, want %v", k, got, d)
		}
	}
	// e is the stun, and the old diagonal keys move nothing now.
	for _, k := range []rune{'e', 'q', 'z', 'x', 'c', ' '} {
		if got := DirectionForKey(k); got != None {
			t.Errorf("key %q: got %v, want none", k, got)
		}
	}
}

// Every key moves exactly one cell, and only along one axis. A key that moved
// diagonally would reintroduce the corner problem the rules no longer handle.
func TestEveryDirectionMovesOneCellOrthogonally(t *testing.T) {
	g := mustParse(t, "...\n...\n...")
	start := Pos{1, 1}

	for k, d := range keys {
		got, ok := g.Move(start, d)
		if !ok {
			t.Fatalf("key %q: refused on open ground", k)
		}
		dx, dy := got.X-start.X, got.Y-start.Y
		if abs(dx)+abs(dy) != 1 {
			t.Errorf("key %q: moved by (%d,%d), want one cell on one axis", k, dx, dy)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Cells touching only at a corner are not connected, because there is no
// diagonal move to get between them.
func TestCornerTouchingCellsAreNotConnected(t *testing.T) {
	g := mustParse(t, ""+
		"####\n"+
		"#.##\n"+
		"##.#\n"+
		"####")

	samePositions(t, g.Reachable(Pos{1, 1}), []Pos{{1, 1}})
}

func TestDirectionRoundTrip(t *testing.T) {
	for _, d := range []Direction{None, North, East, South, West} {
		got, ok := ParseDirection(d.String())
		if !ok || got != d {
			t.Errorf("%v round tripped to %v %v", d, got, ok)
		}
	}
	if _, ok := ParseDirection("nonsense"); ok {
		t.Error("nonsense should not parse")
	}
}

func TestLinesRoundTrip(t *testing.T) {
	src := "#####\n#...#\n#.#.#\n#####"

	g := mustParse(t, src)
	got := strings.Join(g.Lines(), "\n")

	if got != src {
		t.Fatalf("got\n%s\nwant\n%s", got, src)
	}
}

func samePositions(t *testing.T, got, want []Pos) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestWalkables(t *testing.T) {
	g := mustParse(t, "###\n#.#\n#.#")
	samePositions(t, g.Walkables(), []Pos{{1, 1}, {1, 2}})
}

func TestReachableStopsAtWalls(t *testing.T) {
	// Two rooms with no way between them.
	g := mustParse(t, ""+
		"#####\n"+
		"#.#.#\n"+
		"#.#.#\n"+
		"#####")

	samePositions(t, g.Reachable(Pos{1, 1}), []Pos{{1, 1}, {1, 2}})
}

func TestReachableFromWallIsEmpty(t *testing.T) {
	g := mustParse(t, "###\n#.#\n###")
	if got := g.Reachable(Pos{0, 0}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestLargestRegionPicksTheBiggest(t *testing.T) {
	// Left pocket has one cell, right room has four.
	g := mustParse(t, ""+
		"######\n"+
		"#.#..#\n"+
		"###..#\n"+
		"######")

	samePositions(t, g.LargestRegion(),
		[]Pos{{3, 1}, {4, 1}, {3, 2}, {4, 2}})
}

func TestLargestRegionIsEveryCellWhenConnected(t *testing.T) {
	g := mustParse(t, "####\n#..#\n#..#\n####")

	if got, want := len(g.LargestRegion()), len(g.Walkables()); got != want {
		t.Fatalf("region has %d cells, want all %d", got, want)
	}
}
