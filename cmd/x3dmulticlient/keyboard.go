package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	stepWidth = 4
	rotAngel  = 5
)

func (c *client) resize() { c.screen.Sync() }

func (c *client) doQuit() { c.quit = true }

func (c *client) forward() {
	c.camera.Forward(stepWidth)
}

func (c *client) strafeLeft() {
	c.camera.StrafeLeft(stepWidth)
}

func (c *client) backward() {
	c.camera.Backward(stepWidth)
}

func (c *client) strafeRight() {
	c.camera.StrafeRight(stepWidth)
}

func (c *client) up() {
	c.camera.Up(stepWidth)
}

func (c *client) down() {
	c.camera.Down(stepWidth)
}

func (c *client) rotateLeft() {
	c.camera.RotateLeft(mgl32.DegToRad(rotAngel))
}

func (c *client) rotateRight() {
	c.camera.RotateRight(mgl32.DegToRad(rotAngel))
}

func (c *client) rotateUp() {
	c.camera.RotateUp(mgl32.DegToRad(rotAngel))
}

func (c *client) rotateDown() {
	c.camera.RotateDown(mgl32.DegToRad(rotAngel))
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
	return func(*client) {}
}
