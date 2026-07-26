package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// tipDirection turns the arrow's model matrix into the unit vector its tip
// points along, by transforming the north-pointing model direction.
func tipDirection(from, to mgl32.Vec3) mgl32.Vec3 {
	model := arrowTransform(from, to)
	north := mgl32.Vec4{0, 1, 0, 0} // a direction, so w is 0
	out := model.Mul4x1(north)
	return mgl32.Vec3{out[0], out[1], out[2]}.Normalize()
}

// The arrow has to point at the target in world terms. Getting the atan2
// argument order wrong mirrors or rotates it, which is invisible until you play.
func TestArrowPointsAtTheTarget(t *testing.T) {
	origin := mgl32.Vec3{0, 0, 0}

	for _, tc := range []struct {
		name   string
		target mgl32.Vec3
		want   mgl32.Vec3
	}{
		{"north", mgl32.Vec3{0, 100, 0}, mgl32.Vec3{0, 1, 0}},
		{"south", mgl32.Vec3{0, -100, 0}, mgl32.Vec3{0, -1, 0}},
		{"east", mgl32.Vec3{100, 0, 0}, mgl32.Vec3{1, 0, 0}},
		{"west", mgl32.Vec3{-100, 0, 0}, mgl32.Vec3{-1, 0, 0}},
		{"north east", mgl32.Vec3{100, 100, 0},
			mgl32.Vec3{0.70710678, 0.70710678, 0}},
		{"south west", mgl32.Vec3{-100, -100, 0},
			mgl32.Vec3{-0.70710678, -0.70710678, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tipDirection(origin, tc.target)
			for i := range got {
				if math.Abs(float64(got[i]-tc.want[i])) > 1e-5 {
					t.Fatalf("points %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Height is ignored. A pickup floats above the floor, and the arrow lies flat,
// so only the bearing matters.
func TestArrowIgnoresHeight(t *testing.T) {
	from := mgl32.Vec3{0, 0, 0}

	flat := tipDirection(from, mgl32.Vec3{100, 0, 0})
	raised := tipDirection(from, mgl32.Vec3{100, 0, 500})

	for i := range flat {
		if math.Abs(float64(flat[i]-raised[i])) > 1e-5 {
			t.Fatalf("height changed the bearing: %v then %v", flat, raised)
		}
	}
}

func TestArrowSitsAboveThePlayer(t *testing.T) {
	from := mgl32.Vec3{100, 200, 0}
	model := arrowTransform(from, mgl32.Vec3{300, 200, 0})

	origin := model.Mul4x1(mgl32.Vec4{0, 0, 0, 1})

	if origin[0] != from[0] || origin[1] != from[1] {
		t.Errorf("arrow is at (%v, %v), want it over (%v, %v)",
			origin[0], origin[1], from[0], from[1])
	}
	if origin[2] <= from[2] {
		t.Errorf("arrow z %v is not above the player at %v", origin[2], from[2])
	}
}

// Nearest is by straight line, deliberately not by walking distance. A pickup
// just through a wall is nearer than one a long way round an open corridor.
func TestNearestIsStraightLine(t *testing.T) {
	from := mgl32.Vec3{0, 0, 0}

	near := mgl32.Vec3{cellSize, 0, 0}
	far := mgl32.Vec3{0, cellSize * 10, 0}

	got, ok := nearestTo(from, []mgl32.Vec3{far, near})
	if !ok {
		t.Fatal("no target picked")
	}
	if got != near {
		t.Fatalf("picked %v, want %v", got, near)
	}
}

func TestNearestWithNoObjectives(t *testing.T) {
	if _, ok := nearestTo(mgl32.Vec3{}, nil); ok {
		t.Fatal("picked a target from an empty list")
	}
}
