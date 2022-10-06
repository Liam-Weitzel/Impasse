package main

import (
	"log"
	"math"

	"github.com/gdamore/tcell/v2"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	stepWidth = 8
	rotAngel  = 5
)

func (c *client) doQuit() { c.quit = true }

func (c *client) resize() {
	c.screen.Sync()

	sw, sh := c.screen.Size()
	aspect, rw, rh := fitSize(sw*4, sh*8)

	c.freeFBO()
	if err := c.allocFrameBuffer(rw, rh); err != nil {
		log.Fatalf("Allocating framebuffer failed: %v\n", err)
	}

	c.updateProjection(aspect)

	c.dirty = true
}

func (c *client) incFOV() {
	if c.fov < maxFOV {
		if c.fov += 5; c.fov > maxFOV {
			c.fov = maxFOV
		}
		c.updateProjectionScreen()
		c.dirty = true
	}
}

func (c *client) decFOV() {
	if c.fov > minFOV {
		if c.fov -= 5; c.fov < minFOV {
			c.fov = minFOV
		}
		c.updateProjectionScreen()
		c.dirty = true
	}
}

func (c *client) forward() {
	c.camera.Forward(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) strafeLeft() {
	c.camera.StrafeLeft(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) backward() {
	c.camera.Backward(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) strafeRight() {
	c.camera.StrafeRight(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) up() {
	c.camera.Up(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) down() {
	c.camera.Down(stepWidth)
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func (c *client) rotateLeft() {
	c.camera.RotateLeft(mgl32.DegToRad(rotAngel))
	c.dirty = true
}

func (c *client) rotateRight() {
	c.camera.RotateRight(mgl32.DegToRad(rotAngel))
	c.dirty = true
}

func (c *client) rotateUp() {
	c.camera.RotateUp(mgl32.DegToRad(rotAngel))
	c.dirty = true
}

func (c *client) rotateDown() {
	c.camera.RotateDown(mgl32.DegToRad(rotAngel))
	c.dirty = true
}

func (c *client) randomPos() {
	vp := c.scene.Viewpoints[c.rnd.Intn(len(c.scene.Viewpoints))]
	pos := vp.Position
	pos[1] = -pos[1]
	c.camera.Position = pos

	angle := vp.Orientation[3]

	if vp.Orientation[0] > 0 {
		if angle += math.Pi; angle > 2*math.Pi {
			angle -= 2 * math.Pi
		}
	}
	c.camera.Angle = angle
	c.dirty = true
	c.connection.sendPos(c.userID, c.camera.Position)
}

func keyboardConvert(ev tcell.Event) func(*client) {

	switch ev := ev.(type) {
	case *tcell.EventResize:
		return (*client).resize
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEsc, tcell.KeyCtrlC:
			return (*client).doQuit
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'w':
				return (*client).forward
			case 'a':
				return (*client).strafeLeft
			case 's':
				return (*client).backward
			case 'd':
				return (*client).strafeRight
			case ' ':
				return (*client).up
			case 'c':
				return (*client).down
			case 'r':
				return (*client).randomPos
			case '+':
				return (*client).incFOV
			case '-':
				return (*client).decFOV
			}
		case tcell.KeyUp:
			return (*client).forward
		case tcell.KeyDown:
			return (*client).backward
		case tcell.KeyLeft:
			return (*client).rotateLeft
		case tcell.KeyRight:
			return (*client).rotateRight
		case tcell.KeyPgUp:
			return (*client).rotateUp
		case tcell.KeyPgDn:
			return (*client).rotateDown
		}
	}
	return nil
}
