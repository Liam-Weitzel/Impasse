package opengl

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

type Camera struct {
	Position mgl32.Vec3
	Angle    float32
	UpAngle  float32
}

func (c *Camera) Up(width float32) {
	delta := mgl32.Vec3{0, 0, width}
	c.Position = c.Position.Add(delta)
}

func (c *Camera) Down(width float32) {
	delta := mgl32.Vec3{0, 0, width}
	c.Position = c.Position.Sub(delta)
}

func (c *Camera) Forward(width float32) {
	delta := mgl32.Vec3{
		width * float32(math.Cos(float64(c.Angle))),
		width * float32(math.Sin(float64(c.Angle))),
		0,
	}
	c.Position = c.Position.Add(delta)
}

func (c *Camera) Rotation() mgl32.Mat3 {
	a := mgl32.Rotate3DY(c.Angle)
	b := mgl32.Rotate3DX(c.UpAngle)
	return a.Mul3(b)
}

func (c *Camera) Backward(width float32) {
	delta := mgl32.Vec3{
		width * float32(math.Cos(float64(c.Angle))),
		width * float32(math.Sin(float64(c.Angle))),
		0,
	}
	c.Position = c.Position.Sub(delta)
}

func (c *Camera) StrafeLeft(width float32) {
	angle := c.Angle - mgl32.DegToRad(90)
	delta := mgl32.Vec3{
		width * float32(math.Cos(float64(angle))),
		width * float32(math.Sin(float64(angle))),
		0,
	}
	c.Position = c.Position.Add(delta)
}

func (c *Camera) StrafeRight(width float32) {
	angle := c.Angle + mgl32.DegToRad(90)
	delta := mgl32.Vec3{
		width * float32(math.Cos(float64(angle))),
		width * float32(math.Sin(float64(angle))),
		0,
	}
	c.Position = c.Position.Add(delta)
}

func (c *Camera) RotateLeft(angle float32) {
	c.Angle -= angle
	for ; c.Angle < 0; c.Angle += 2 * math.Pi {
	}
}

func (c *Camera) RotateRight(angle float32) {
	c.Angle += angle
	for ; c.Angle > 2*math.Pi; c.Angle -= 2 * math.Pi {
	}
}

func (c *Camera) RotateDown(angle float32) {
	c.UpAngle -= angle
	for ; c.UpAngle < 0; c.UpAngle += 2 * math.Pi {
	}
}

func (c *Camera) RotateUp(angle float32) {
	c.UpAngle += angle
	for ; c.UpAngle > 2*math.Pi; c.UpAngle -= 2 * math.Pi {
	}
}
