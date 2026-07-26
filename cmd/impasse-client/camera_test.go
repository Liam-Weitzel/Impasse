package main

import (
	"testing"

	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/go-gl/mathgl/mgl32"
)

// project runs a world point all the way to terminal coordinates, using the
// same matrices the client builds.
//
// The result is in screen fractions: x runs 0 at the left edge to 1 at the
// right, y runs 0 at the top row to 1 at the bottom. Testing in these terms
// rather than in clip space is deliberate. The projection mirrors clip space
// vertically to cancel the bottom up order glReadPixels returns, so a clip
// space assertion would pass while the terminal shows the world upside down.
func project(c *camera, p mgl32.Vec3) (x, y float32) {
	perspective := mgl32.Perspective(
		mgl32.DegToRad(defaultFOV)*0.5, 2, nearPlane, c.far())
	proj := flipY.Mul4(perspective)

	clip := proj.Mul4(c.view()).Mul4x1(p.Vec4(1))
	if clip[3] == 0 {
		return 0, 0
	}
	ndcX := clip[0] / clip[3]
	ndcY := clip[1] / clip[3]

	// glReadPixels writes the bottom GL row first and that becomes image row
	// 0, which the converter draws at the top of the terminal. So ndc -1 is
	// the top row.
	return (ndcX + 1) / 2, (ndcY + 1) / 2
}

// The camera looks north from the south, so east belongs on the right of the
// terminal and north towards the top. Getting the horizontal wrong mirrors the
// world, which is what using grid rows as world Y directly does.
func TestCameraOrientation(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}

	if x, _ := project(c, mgl32.Vec3{cellSize, 0, 0}); x <= 0.5 {
		t.Errorf("east is at screen x %.3f, want right of centre", x)
	}
	if x, _ := project(c, mgl32.Vec3{-cellSize, 0, 0}); x >= 0.5 {
		t.Errorf("west is at screen x %.3f, want left of centre", x)
	}
	if _, y := project(c, mgl32.Vec3{0, cellSize, 0}); y >= 0.5 {
		t.Errorf("north is at screen y %.3f, want above centre", y)
	}
	if _, y := project(c, mgl32.Vec3{0, -cellSize, 0}); y <= 0.5 {
		t.Errorf("south is at screen y %.3f, want below centre", y)
	}
}

// Pressing W walks north, which has to look like walking away from the camera,
// towards the top of the terminal. This is the regression that made W feel like
// it went backwards.
func TestNorthIsAwayFromCamera(t *testing.T) {
	c := newCamera()
	c.target = cellCenter(grid.Pos{X: 5, Y: 5})

	_, here := project(c, cellCenter(grid.Pos{X: 5, Y: 5}))
	_, north := project(c, cellCenter(grid.Pos{X: 5, Y: 4}))
	_, south := project(c, cellCenter(grid.Pos{X: 5, Y: 6}))

	if north >= here {
		t.Errorf("the cell north is at screen y %.3f, not above %.3f", north, here)
	}
	if south <= here {
		t.Errorf("the cell south is at screen y %.3f, not below %.3f", south, here)
	}
}

// Up in the world has to be up on the terminal, not down. A wall top drawn
// below its own floor means the picture is upside down.
func TestCameraUpIsUp(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}

	_, floor := project(c, mgl32.Vec3{0, 0, 0})
	_, top := project(c, mgl32.Vec3{0, 0, wallHeight})

	if top >= floor {
		t.Errorf("wall top at screen y %.3f is not above the floor at %.3f",
			top, floor)
	}
}

// Moving south in the grid has to move south in the world, which is downwards
// on screen.
func TestCellCenterMatchesCompass(t *testing.T) {
	origin := cellCenter(grid.Pos{X: 0, Y: 0})

	if e := cellCenter(grid.Pos{X: 1, Y: 0}); e[0] <= origin[0] {
		t.Errorf("grid +X gave world x %.1f, want more than %.1f", e[0], origin[0])
	}
	if s := cellCenter(grid.Pos{X: 0, Y: 1}); s[1] >= origin[1] {
		t.Errorf("grid +Y gave world y %.1f, want less than %.1f", s[1], origin[1])
	}
}

func TestCameraClamps(t *testing.T) {
	c := newCamera()

	for i := 0; i < 50; i++ {
		c.rotateLeft()
		c.pitchDown()
		c.zoomIn()
	}
	if c.yaw < -maxYaw-0.001 {
		t.Errorf("yaw %.3f below the clamp", c.yaw)
	}
	if c.pitch < minPitch-0.001 {
		t.Errorf("pitch %.3f below the clamp", c.pitch)
	}
	if c.dist < minZoom-0.001 {
		t.Errorf("dist %.1f below the clamp", c.dist)
	}

	for i := 0; i < 50; i++ {
		c.rotateRight()
		c.pitchUp()
		c.zoomOut()
	}
	if c.yaw > maxYaw+0.001 {
		t.Errorf("yaw %.3f above the clamp", c.yaw)
	}
	if c.pitch > maxPitch+0.001 {
		t.Errorf("pitch %.3f above the clamp", c.pitch)
	}
	if c.dist > maxZoom+0.001 {
		t.Errorf("dist %.1f above the clamp", c.dist)
	}
}

// Pitch must stay clear of straight down, where the view direction becomes
// parallel to the up vector and LookAtV produces NaNs.
func TestCameraStaysUsableAtMaxPitch(t *testing.T) {
	c := newCamera()
	c.target = mgl32.Vec3{0, 0, 0}
	c.setPitch(maxPitch)

	x, y := project(c, mgl32.Vec3{0, cellSize, 0})
	if x != x || y != y {
		t.Fatalf("projection is NaN at max pitch: %v %v", x, y)
	}
	if y >= 0.5 {
		t.Errorf("north at screen y %.3f, want above centre even looking down", y)
	}
}
