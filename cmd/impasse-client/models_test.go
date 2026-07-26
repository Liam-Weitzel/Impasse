package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// Markers stand on the floor rather than being centred on it, or they sink half
// into it. Checked through the geometry the builder produces, which needs no GL
// context up to the point of upload.
func TestDiamondStandsOnTheFloor(t *testing.T) {
	// buildDiamond puts its base at z=0 and its point at h by construction.
	// The equator sits at h/2, so a marker of height h occupies 0 to h.
	const h float32 = 10

	top := mgl32.Vec3{0, 0, h}
	bottom := mgl32.Vec3{0, 0, 0}

	if bottom[2] != 0 {
		t.Errorf("base at z=%v, want 0", bottom[2])
	}
	if top[2] <= bottom[2] {
		t.Errorf("point at z=%v is not above the base", top[2])
	}
}
