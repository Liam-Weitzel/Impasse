package gfx

import (
	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"

	_ "github.com/gdamore/tcell/v2/terminfo/extended"
)

const (
	displayWidth  = 640
	displayHeight = 400
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

	var err error

	var window *sdl.Window

	defer sdl.Do(func() {
		if window != nil {
			window.Destroy()
		}
	})

	sdl.Do(func() {
		if err = sdl.Init(sdl.INIT_VIDEO); err != nil {
			return
		}
		if err = sdl.VideoInit("offscreen"); err != nil {
			return
		}

		sdl.GLSetAttribute(
			sdl.GL_CONTEXT_PROFILE_MASK,
			sdl.GL_CONTEXT_PROFILE_ES)
		sdl.GLSetAttribute(
			sdl.GL_CONTEXT_MAJOR_VERSION, 3)
		sdl.GLSetAttribute(
			sdl.GL_CONTEXT_MINOR_VERSION, 0)
		sdl.GLSetAttribute(sdl.GL_DOUBLEBUFFER, 0)

		if window, err = sdl.CreateWindow(
			"SSH3D",
			sdl.WINDOWPOS_UNDEFINED,
			sdl.WINDOWPOS_UNDEFINED,
			displayWidth, displayHeight,
			sdl.WINDOW_OPENGL|sdl.WINDOW_HIDDEN,
			//sdl.WINDOW_OPENGL|sdl.WINDOW_SHOWN,
		); err != nil {
			return
		}

		//log.Printf("INFO: %s\n", gl.GoStr(gl.GetString(gl.VERSION)))
	})
	if err == nil {
		err = fn(window)
	}

	return err
}
