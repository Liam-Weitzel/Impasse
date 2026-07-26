package main

import (
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Player and pickup markers.
//
// At this resolution a marker is a handful of characters, so silhouette is the
// only thing that carries. A sphere and a cube are nearly identical from a
// steep angle; a diamond has a point, which is why it is the one used.
//
// Both are ordinary shapes drawn with a model transform, so a marker can spin
// without the renderer knowing anything about it. Both stand on z=0 rather than
// being centred on it, or they sink half into the floor.

// buildDiamond makes an octahedron standing on its lower point.
func buildDiamond(color mgl32.Vec3, r, h float32) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(color)

	top := mgl32.Vec3{0, 0, h}
	bottom := mgl32.Vec3{0, 0, 0}
	mid := h / 2

	// Equator, counter clockwise seen from above so the faces wind outward.
	eq := [4]mgl32.Vec3{
		{r, 0, mid}, {0, r, mid}, {-r, 0, mid}, {0, -r, mid},
	}

	for i := 0; i < 4; i++ {
		a, b := eq[i], eq[(i+1)%4]

		out := a.Add(b).Mul(0.5)
		upper := mgl32.Vec3{out[0], out[1], mid}.Normalize()

		// Triangles go through AddQuad with the last corner repeated, so the
		// second triangle of the fan collapses.
		if err := mb.AddQuad(top, a, b, b, upper); err != nil {
			return nil, err
		}
		if err := mb.AddQuad(bottom, b, a, a,
			mgl32.Vec3{upper[0], upper[1], -upper[2]}); err != nil {
			return nil, err
		}
	}

	return mb.Compile()
}

// buildPlayerModel makes a player marker. White, so the per player tint does
// all the work.
func buildPlayerModel(r float32) ([]*render.CompiledShape, error) {
	return buildDiamond(mgl32.Vec3{1, 1, 1}, r, r*1.7)
}

// buildPickup makes the pickup marker, drawn spinning, because a rotating
// silhouette is the cheapest way to make something read as an object rather
// than a mark on the floor.
func buildPickup(r float32) ([]*render.CompiledShape, error) {
	return buildDiamond(mgl32.Vec3{1, 1, 1}, r, r*2)
}
