package main

import (
	"math"

	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// The pointer to the nearest objective. It is a flat arrow lying in the XY
// plane, which reads well from a steep top-down camera, floating above the
// player's own marker.
//
// It points by straight-line bearing and will happily point through a wall.
// That is the design: it tells you where a pickup is, never how to get there.
// Working the route out is the player's job, and a bot is not handed it either.
const (
	arrowScale  = cellSize * 0.5
	arrowHeight = playerRadius*2 + cellSize*0.35
)

var arrowColor = mgl32.Vec3{0.95, 0.85, 0.30}

// buildArrow makes an arrow pointing north, to be rotated at draw time.
//
// Quads are wound counter clockwise seen from above, matching the floor. The
// head is a triangle, which goes through AddQuad as a quad with its last corner
// repeated so the second triangle of the fan collapses.
func buildArrow() ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(arrowColor)

	up := mgl32.Vec3{0, 0, 1}

	const (
		tipY    = 1.0
		barbY   = 0.25
		barbX   = 0.6
		shaftX  = 0.22
		shaftY0 = -1.0
	)

	head := [3]mgl32.Vec3{
		{0, tipY * arrowScale, 0},
		{-barbX * arrowScale, barbY * arrowScale, 0},
		{barbX * arrowScale, barbY * arrowScale, 0},
	}
	if err := mb.AddQuad(head[0], head[1], head[2], head[2], up); err != nil {
		return nil, err
	}

	if err := mb.AddQuad(
		mgl32.Vec3{-shaftX * arrowScale, shaftY0 * arrowScale, 0},
		mgl32.Vec3{shaftX * arrowScale, shaftY0 * arrowScale, 0},
		mgl32.Vec3{shaftX * arrowScale, barbY * arrowScale, 0},
		mgl32.Vec3{-shaftX * arrowScale, barbY * arrowScale, 0},
		up,
	); err != nil {
		return nil, err
	}

	return mb.Compile()
}

// arrowTransform places the arrow above from, turned towards to.
//
// The arrow is modelled pointing north, so rotating north onto the bearing
// needs atan2(-dx, dy): turning +Y about Z by theta gives (-sin, cos).
func arrowTransform(from, to mgl32.Vec3) mgl32.Mat4 {
	dx := to[0] - from[0]
	dy := to[1] - from[1]

	angle := float32(math.Atan2(float64(-dx), float64(dy)))

	return mgl32.Translate3D(from[0], from[1], from[2]+arrowHeight).
		Mul4(mgl32.HomogRotate3DZ(angle))
}

// nearestTo returns the closest position by straight-line distance, which is
// deliberately not walking distance.
func nearestTo(from mgl32.Vec3, candidates []mgl32.Vec3) (mgl32.Vec3, bool) {
	if len(candidates) == 0 {
		return mgl32.Vec3{}, false
	}

	best := candidates[0]
	bestDist := from.Sub(best).LenSqr()

	for _, c := range candidates[1:] {
		if d := from.Sub(c).LenSqr(); d < bestDist {
			best, bestDist = c, d
		}
	}

	return best, true
}
