package gfx

import (
	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"

	_ "github.com/gdamore/tcell/v2/terminfo/extended"
)

const (
	displayWidth  = 1
	displayHeight = 1
)

func WrapScreen(fn func(tcell.Screen) error) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()

	return fn(screen)
}

func WrapWindow(fn func(*sdl.Window) error) error {

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return err
	}
	if err := sdl.VideoInit("offscreen"); err != nil {
		return err
	}

	sdl.GLSetAttribute(
		sdl.GL_CONTEXT_PROFILE_MASK,
		sdl.GL_CONTEXT_PROFILE_ES)
	sdl.GLSetAttribute(
		sdl.GL_CONTEXT_MAJOR_VERSION, 3)
	sdl.GLSetAttribute(
		sdl.GL_CONTEXT_MINOR_VERSION, 1)
	sdl.GLSetAttribute(sdl.GL_DOUBLEBUFFER, 0)

	var window *sdl.Window
	var err error

	if window, err = sdl.CreateWindow(
		"Impasse",
		sdl.WINDOWPOS_UNDEFINED,
		sdl.WINDOWPOS_UNDEFINED,
		displayWidth, displayHeight,
		sdl.WINDOW_OPENGL|sdl.WINDOW_HIDDEN,
		//sdl.WINDOW_OPENGL|sdl.WINDOW_SHOWN,
	); err != nil {
		return err
	}

	defer window.Destroy()

	//log.Printf("INFO: %s\n", gl.GoStr(gl.GetString(gl.VERSION)))
	return fn(window)
}
