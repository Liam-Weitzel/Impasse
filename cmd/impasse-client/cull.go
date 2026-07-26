package main

import (
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Culling drops the chunks of the map that cannot be on screen. Without it the
// whole map is drawn every frame, which is fine for a 50x30 test map and not
// fine for a real one.
//
// The test is conservative: a chunk is only dropped when every corner of its
// box is outside the same frustum plane. That can keep a chunk that is not
// really visible, which costs a draw call, but it can never drop one that is,
// which would punch a hole in the world.

// visible reports whether a box could be on screen under the given view
// projection matrix.
func visible(viewProj mgl32.Mat4, b render.Bounds) bool {
	corners := b.Corners()

	// One counter per frustum plane, in clip space where a point is inside
	// when -w <= x,y,z <= w.
	var outLeft, outRight, outBottom, outTop, outNear, outFar int

	for _, c := range corners {
		clip := viewProj.Mul4x1(c.Vec4(1))
		x, y, z, w := clip[0], clip[1], clip[2], clip[3]

		if x < -w {
			outLeft++
		}
		if x > w {
			outRight++
		}
		if y < -w {
			outBottom++
		}
		if y > w {
			outTop++
		}
		if z < -w {
			outNear++
		}
		if z > w {
			outFar++
		}
	}

	const all = 8
	if outLeft == all || outRight == all ||
		outBottom == all || outTop == all ||
		outNear == all || outFar == all {
		return false
	}
	return true
}

// cull filters shapes down to those that could be on screen, reusing dst.
func cull(dst []*render.CompiledShape, viewProj mgl32.Mat4, shapes []*render.CompiledShape) []*render.CompiledShape {
	dst = dst[:0]
	for _, cs := range shapes {
		if visible(viewProj, cs.Bounds) {
			dst = append(dst, cs)
		}
	}
	return dst
}
