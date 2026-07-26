package main

import (
	"math"

	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Player and pickup markers.
//
// Players are spheres and pickups are diamonds, so the two never read as the
// same object even when the colours are close.
//
// Both are ordinary shapes drawn with a model transform, so a marker can spin
// without the renderer knowing anything about it. Both stand on z=0 rather than
// being centred on it, or they sink half into the floor.

// Sphere tessellation. A cell is only a few dozen pixels across, so anything
// finer than this is averaged away before it reaches the terminal.
const (
	sphereSectors = 16
	sphereStacks  = 10
)

// buildSphere makes a sphere of radius r sitting on z=0, so its centre is at
// z=r.
//
// Quads run from pole to pole. At either pole the quad's two top or two bottom
// corners are the same point, which collapses one triangle of the fan and
// leaves the other, so the poles need no special case.
func buildSphere(color mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(color)

	// at returns the point on the sphere for stack i, counted from the north
	// pole, and sector j.
	at := func(i, j int) mgl32.Vec3 {
		theta := math.Pi * float64(i) / sphereStacks
		phi := 2 * math.Pi * float64(j) / sphereSectors

		z := float32(math.Cos(theta))
		xy := float32(math.Sin(theta))

		return mgl32.Vec3{
			r * xy * float32(math.Cos(phi)),
			r * xy * float32(math.Sin(phi)),
			r*z + r,
		}
	}

	centre := mgl32.Vec3{0, 0, r}

	for i := 0; i < sphereStacks; i++ {
		for j := 0; j < sphereSectors; j++ {
			a, b := at(i, j), at(i, j+1)
			c, d := at(i+1, j+1), at(i+1, j)

			// The face normal is the direction from the centre to the middle
			// of the quad. On a sphere that is exact enough that the facets do
			// not show once the picture is squeezed into block characters.
			mid := a.Add(b).Add(c).Add(d).Mul(0.25)
			normal := mid.Sub(centre).Normalize()

			if err := mb.AddQuad(a, b, c, d, normal); err != nil {
				return nil, err
			}
		}
	}

	return mb.Compile()
}

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
	return buildSphere(mgl32.Vec3{1, 1, 1}, r)
}

// buildPickup makes the pickup marker, drawn spinning, because a rotating
// silhouette is the cheapest way to make something read as an object rather
// than a mark on the floor.
func buildPickup(r float32) ([]*render.CompiledShape, error) {
	return buildDiamond(mgl32.Vec3{1, 1, 1}, r, r*2)
}
