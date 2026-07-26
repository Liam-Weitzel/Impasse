package render

import "testing"

// UV rectangles must stay inside their own tile, including the inset that stops
// linear filtering from bleeding across the seam and drawing a bright line
// along every cell edge.
func TestAtlasUVStaysInsideItsTile(t *testing.T) {
	const cols, rows = 4, 2
	a := &Atlas{cols: cols, rows: rows}

	w := 1 / float32(cols)
	h := 1 / float32(rows)

	for i := 0; i < cols*rows; i++ {
		uv := a.UV(Tile(i))

		col := float32(i % cols)
		row := float32(i / cols)

		for _, p := range uv {
			if p[0] < col*w || p[0] > (col+1)*w {
				t.Errorf("tile %d u %.4f outside [%.4f, %.4f]",
					i, p[0], col*w, (col+1)*w)
			}
			if p[1] < row*h || p[1] > (row+1)*h {
				t.Errorf("tile %d v %.4f outside [%.4f, %.4f]",
					i, p[1], row*h, (row+1)*h)
			}
		}
	}
}

// The corners must come back in the order AddTexturedQuad wants them, matching
// the quad's own winding, or tiles arrive rotated or mirrored.
func TestAtlasUVCornerOrder(t *testing.T) {
	a := &Atlas{cols: 2, rows: 2}
	uv := a.UV(0)

	bl, br, tr, tl := uv[0], uv[1], uv[2], uv[3]

	if bl[0] != tl[0] {
		t.Error("left corners disagree on u")
	}
	if br[0] != tr[0] {
		t.Error("right corners disagree on u")
	}
	if bl[1] != br[1] {
		t.Error("bottom corners disagree on v")
	}
	if tl[1] != tr[1] {
		t.Error("top corners disagree on v")
	}
	if br[0] <= bl[0] {
		t.Error("u does not increase to the right")
	}
	if bl[1] <= tl[1] {
		t.Error("v does not decrease upwards")
	}
}

// Out of range indices wrap rather than sampling off the atlas.
func TestAtlasUVWrapsOutOfRange(t *testing.T) {
	a := &Atlas{cols: 4, rows: 2}
	count := 8

	if got, want := a.UV(Tile(count)), a.UV(0); got != want {
		t.Error("one past the end did not wrap to the first tile")
	}
	if got, want := a.UV(Tile(-1)), a.UV(Tile(count-1)); got != want {
		t.Error("minus one did not wrap to the last tile")
	}
}

// A zero atlas must not divide by zero.
func TestEmptyAtlasIsSafe(t *testing.T) {
	var a *Atlas
	if got := a.UV(3); got != ([4][2]float32{}) {
		t.Errorf("nil atlas gave %v", got)
	}
	if a.Count() != 0 {
		t.Error("nil atlas has tiles")
	}

	empty := &Atlas{}
	if got := empty.UV(1); got != ([4][2]float32{}) {
		t.Errorf("empty atlas gave %v", got)
	}
}

func TestNewAtlasRejectsBadGrid(t *testing.T) {
	if _, err := NewAtlas(nil, 0, 1); err == nil {
		t.Error("want an error for zero columns")
	}
	if _, err := NewAtlas(nil, 1, 0); err == nil {
		t.Error("want an error for zero rows")
	}
}
