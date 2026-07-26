package main

import (
	"math"
	"time"

	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// A burst takes a tick to land, and the block it will cover is drawn on the
// floor for that tick. Everyone sees it, not just the caster: it is already
// cast and lands whatever anyone does, so showing it gives nothing away about
// intent and lets bystanders decide whether to close in.
//
// It is a frame rather than a filled square. A filled one hides whoever is
// standing in it, which is exactly the information you need in the one tick you
// have to react. The frame also pulses, because a static shape on the floor
// reads as scenery and a moving one reads as a warning.
const (
	// Just clear of the floor, or it fights with it for depth.
	telegraphLift = 1.5
	// How thick the frame is, as a fraction of the whole block.
	telegraphEdge = 0.16
	// How far the frame swells over its one tick of life.
	telegraphPulse = 0.10
)

// buildTelegraph makes the floor frame a burst covers, centred on the caster.
// radius is in cells, so 1 gives the 3x3 block.
//
// Four quads, wound counter clockwise seen from above to match the floor.
func buildTelegraph(radius int) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(mgl32.Vec3{1, 1, 1})

	half := float32(radius)*cellSize + cellSize/2
	inner := half * (1 - telegraphEdge*2)
	up := mgl32.Vec3{0, 0, 1}

	// north, south, west and east bars of the frame.
	bars := [][4]mgl32.Vec3{
		{{-half, inner, telegraphLift}, {half, inner, telegraphLift},
			{half, half, telegraphLift}, {-half, half, telegraphLift}},
		{{-half, -half, telegraphLift}, {half, -half, telegraphLift},
			{half, -inner, telegraphLift}, {-half, -inner, telegraphLift}},
		{{-half, -inner, telegraphLift}, {-inner, -inner, telegraphLift},
			{-inner, inner, telegraphLift}, {-half, inner, telegraphLift}},
		{{inner, -inner, telegraphLift}, {half, -inner, telegraphLift},
			{half, inner, telegraphLift}, {inner, inner, telegraphLift}},
	}

	for _, b := range bars {
		if err := mb.AddQuad(b[0], b[1], b[2], b[3], up); err != nil {
			return nil, err
		}
	}

	return mb.Compile()
}

// telegraphTransform puts the frame under a caster and swells it over the tick,
// so it reads as something arriving rather than something painted on.
func telegraphTransform(at mgl32.Vec3, alpha float32) mgl32.Mat4 {
	scale := 1 - telegraphPulse + telegraphPulse*2*alpha

	return mgl32.Translate3D(at[0], at[1], 0).
		Mul4(mgl32.Scale3D(scale, scale, 1))
}

// telegraphColor brightens as the burst gets closer to landing.
func telegraphColor(base mgl32.Vec3, alpha float32) mgl32.Vec3 {
	f := 0.55 + 0.45*alpha
	return base.Mul(f)
}

// pulse is a 0 to 1 triangle wave, for anything that should breathe rather than
// sit still. Driven by the wall clock rather than the tick, because it is
// decoration and should not stutter when a tick is late.
func pulse(period time.Duration) float32 {
	if period <= 0 {
		return 1
	}
	phase := float64(time.Now().UnixNano()%int64(period)) / float64(period)
	return float32(0.5 - 0.5*math.Cos(2*math.Pi*phase))
}
