package main

import (
	"testing"

	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// testViewProj builds the same matrices the client uses, looking at the origin.
func testViewProj(c *camera) mgl32.Mat4 {
	perspective := mgl32.Perspective(
		mgl32.DegToRad(defaultFOV)*0.5, 2, nearPlane, c.far())
	return flipY.Mul4(perspective).Mul4(c.view())
}

// boxAt makes a one cell box centred on a world point.
func boxAt(x, y float32) render.Bounds {
	const h = wallHeight
	return render.Bounds{
		Min: mgl32.Vec3{x - cellSize/2, y - cellSize/2, 0},
		Max: mgl32.Vec3{x + cellSize/2, y + cellSize/2, h},
	}
}

// What the camera is looking at must never be culled. Getting this wrong
// punches holes in the world.
func TestChunkUnderTheCameraIsVisible(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}
	vp := testViewProj(c)

	if !visible(vp, boxAt(0, 0)) {
		t.Error("the chunk under the camera was culled")
	}
}

// Something far behind the camera is not on screen and should go.
func TestDistantChunkIsCulled(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}
	vp := testViewProj(c)

	var far float32 = cellSize * 400
	if visible(vp, boxAt(0, -far)) {
		t.Error("a chunk far behind the camera was kept")
	}
	if visible(vp, boxAt(far, 0)) {
		t.Error("a chunk far to the east was kept")
	}
}

// Nothing within a few cells of the target may be dropped, at any zoom or
// tilt the player can reach. This is the property that matters: culling may
// waste a draw call, but must never remove something on screen.
func TestNothingNearbyIsEverCulled(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}

	for _, dist := range []float32{minZoom, (minZoom + maxZoom) / 2, maxZoom} {
		for _, pitch := range []float32{minPitch, defaultPitch, maxPitch} {
			for _, yaw := range []float32{-maxYaw, 0, maxYaw} {
				c.setDist(dist)
				c.setPitch(pitch)
				c.setYaw(yaw)
				vp := testViewProj(c)

				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						b := boxAt(float32(dx)*cellSize, float32(dy)*cellSize)
						if !visible(vp, b) {
							t.Fatalf("culled a neighbouring chunk at (%d,%d) "+
								"with dist %.0f pitch %.2f yaw %.2f",
								dx, dy, dist, pitch, yaw)
						}
					}
				}
			}
		}
	}
}

// Culling has to actually remove something, or it is just overhead.
func TestCullDropsOffscreenShapes(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}
	vp := testViewProj(c)

	shapes := []*render.CompiledShape{
		{Bounds: boxAt(0, 0)},
		{Bounds: boxAt(0, cellSize)},
		{Bounds: boxAt(cellSize*500, 0)},
		{Bounds: boxAt(0, -cellSize*500)},
	}

	got := cull(nil, vp, shapes)

	if len(got) == len(shapes) {
		t.Error("nothing was culled")
	}
	if len(got) == 0 {
		t.Fatal("everything was culled, including what is under the camera")
	}
	for _, cs := range got {
		if cs.Bounds.Min[0] > cellSize*100 || cs.Bounds.Min[1] < -cellSize*100 {
			t.Errorf("kept a shape far off screen: %+v", cs.Bounds)
		}
	}
}

// cull reuses its destination slice, so it must not carry stale entries over.
func TestCullReusesTheSlice(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}
	vp := testViewProj(c)

	dst := make([]*render.CompiledShape, 0, 8)

	dst = cull(dst, vp, []*render.CompiledShape{
		{Bounds: boxAt(0, 0)},
		{Bounds: boxAt(0, cellSize)},
	})
	if len(dst) != 2 {
		t.Fatalf("got %d shapes, want 2", len(dst))
	}

	dst = cull(dst, vp, []*render.CompiledShape{{Bounds: boxAt(0, 0)}})
	if len(dst) != 1 {
		t.Errorf("got %d shapes, want 1 after reuse", len(dst))
	}
}

func TestBoundsCorners(t *testing.T) {
	b := render.Bounds{
		Min: mgl32.Vec3{0, 0, 0},
		Max: mgl32.Vec3{1, 2, 3},
	}

	corners := b.Corners()

	seen := map[mgl32.Vec3]bool{}
	for _, c := range corners {
		seen[c] = true
		for i := 0; i < 3; i++ {
			if c[i] != b.Min[i] && c[i] != b.Max[i] {
				t.Errorf("corner %v is not on the box", c)
			}
		}
	}
	if len(seen) != 8 {
		t.Errorf("got %d distinct corners, want 8", len(seen))
	}
}
