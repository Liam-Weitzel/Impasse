package main

import (
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// A burst takes a tick to land, and the block it will cover is drawn on the
// floor for that tick. Everyone sees it, not just the caster, which is the
// point: it is already cast and lands whatever anyone does, so showing it gives
// nothing away about intent and lets bystanders decide whether to close in.
const (
	// Just clear of the floor, or it fights with it for depth.
	telegraphLift = 1.5
)

var telegraphColor = mgl32.Vec3{0.90, 0.25, 0.20}

// buildTelegraph makes the floor patch a burst covers, centred on the caster.
// radius is in cells, so 1 gives the 3x3 block.
//
// Wound counter clockwise seen from above, matching the floor.
func buildTelegraph(radius int) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(telegraphColor)

	half := float32(radius)*cellSize + cellSize/2

	if err := mb.AddQuad(
		mgl32.Vec3{-half, -half, telegraphLift},
		mgl32.Vec3{half, -half, telegraphLift},
		mgl32.Vec3{half, half, telegraphLift},
		mgl32.Vec3{-half, half, telegraphLift},
		mgl32.Vec3{0, 0, 1},
	); err != nil {
		return nil, err
	}

	return mb.Compile()
}

// telegraphTransform puts the patch under a caster. Its own height is baked
// into the mesh, so only the cell matters here.
func telegraphTransform(at mgl32.Vec3) mgl32.Mat4 {
	return mgl32.Translate3D(at[0], at[1], 0)
}
