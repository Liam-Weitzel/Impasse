package main

import (
	"log"

	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/veandco/go-sdl2/sdl"

	_ "github.com/gdamore/tcell/v2/terminfo/extended"
)

const (
	displayWidth  = 640
	displayHeight = 480
)

func check(err error) {
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

func main() {
	screen, err := tcell.NewScreen()
	check(err)
	check(screen.Init())

	check(func() error {
		defer screen.Fini()
		var err error
		var window *sdl.Window

		sdl.Main(func() {
			defer sdl.Do(func() {
				if window != nil {
					window.Destroy()
				}
			})
			c := newClient(window)

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

				if _, err = window.GLCreateContext(); err != nil {
					return
				}

				if err = gl.Init(); err != nil {
					return
				}
				//log.Printf("INFO: %s\n", gl.GoStr(gl.GetString(gl.VERSION)))
			})
			if err == nil {
				err = c.run(screen)
			}
		})
		return err
	}())
}
