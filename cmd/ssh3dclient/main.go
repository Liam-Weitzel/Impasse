package main

import (
	"log"
	"runtime"

	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	if err := gfx.WrapScreen(func(screen tcell.Screen) error {
		return gfx.WrapWindow(func(window *sdl.Window) error {
			return newClient(window).run(screen)
		})
	}); err != nil {
		log.Fatalf("error: %v\n", err)
	}
}
