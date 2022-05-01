package main

import (
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
)

func main() {
	var err error
	sdl.Main(func() {
		err = gfx.WrapScreen(func(screen tcell.Screen) error {
			return gfx.WrapWindow(func(window *sdl.Window) error {
				return newClient(window).run(screen)
			})
		})
	})
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}
