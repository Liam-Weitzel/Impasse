package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// The camera sits above and to the south of its target, looking north and down.
//
// Yaw is limited so north stays roughly up on screen, otherwise world locked
// movement keys stop matching what the player sees. Pitch stops short of 90
// because looking straight down makes the view direction parallel to the up
// vector and the view matrix degenerates.
const (
	defaultPitch = 65 * math.Pi / 180
	minPitch     = 35 * math.Pi / 180
	maxPitch     = 85 * math.Pi / 180
	pitchStep    = 5 * math.Pi / 180

	maxYaw  = 30 * math.Pi / 180
	yawStep = 5 * math.Pi / 180

	// Zooming out stops well inside the fog. Fog is a smoothstep out to
	// fogFar, which is 760 units, so by ten cells the picture is already more
	// than nine tenths fog colour and there is nothing left to make out. The
	// camera used to be allowed past fogFar entirely, which is why it went
	// dark rather than wide.
	minZoom  = 4 * cellSize
	maxZoom  = 10 * cellSize
	zoomStep = cellSize
)

type camera struct {
	// target is the world point the camera looks at.
	target mgl32.Vec3
	yaw    float32
	pitch  float32
	dist   float32
}

func newCamera() *camera {
	return &camera{
		pitch: defaultPitch,
		dist:  9 * cellSize,
	}
}

func (c *camera) rotateLeft()  { c.setYaw(c.yaw - yawStep) }
func (c *camera) rotateRight() { c.setYaw(c.yaw + yawStep) }

func (c *camera) setYaw(y float32) {
	c.yaw = mgl32.Clamp(y, -maxYaw, maxYaw)
}

// pitchUp tilts towards looking straight down, pitchDown towards the horizon.
func (c *camera) pitchUp()   { c.setPitch(c.pitch + pitchStep) }
func (c *camera) pitchDown() { c.setPitch(c.pitch - pitchStep) }

func (c *camera) setPitch(p float32) {
	c.pitch = mgl32.Clamp(p, minPitch, maxPitch)
}

func (c *camera) zoomIn()  { c.setDist(c.dist - zoomStep) }
func (c *camera) zoomOut() { c.setDist(c.dist + zoomStep) }

func (c *camera) setDist(d float32) {
	c.dist = mgl32.Clamp(d, minZoom, maxZoom)
}

// eye is where the camera sits. Yaw zero puts it due south of the target, so
// the view looks north with east on the right.
func (c *camera) eye() mgl32.Vec3 {
	horizontal := c.dist * float32(math.Cos(float64(c.pitch)))
	vertical := c.dist * float32(math.Sin(float64(c.pitch)))

	sin := float32(math.Sin(float64(c.yaw)))
	cos := float32(math.Cos(float64(c.yaw)))

	return mgl32.Vec3{
		c.target[0] + horizontal*sin,
		c.target[1] - horizontal*cos,
		c.target[2] + vertical,
	}
}

func (c *camera) view() mgl32.Mat4 {
	return mgl32.LookAtV(c.eye(), c.target, mgl32.Vec3{0, 0, 1})
}

// far is how far the camera can see. The eye never gets further away than
// maxZoom, so this only has to cover that plus what is on screen around it.
func (c *camera) far() float32 {
	return maxZoom * 4
}
