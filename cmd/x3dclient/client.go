package main

import (
	"log"

	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

type client struct {
	scene     *x3d.Scene
	directory string

	window  *sdl.Window
	context sdl.GLContext

	screen tcell.Screen
}

func startClient(
	scene *x3d.Scene, directory string,
	screen tcell.Screen, window *sdl.Window,
) error {
	c := &client{
		scene:     scene,
		directory: directory,
		window:    window,
		screen:    screen,
	}

	if err := c.setupOpenGL(); err != nil {
		return err
	}
	defer c.tearDownOpenGL()

	return c.run()
}

func (c *client) setupOpenGL() error {
	var err error
	if c.context, err = c.window.GLCreateContext(); err != nil {
		return err
	}
	if err = gl.Init(); err != nil {
		sdl.GLDeleteContext(c.context)
	}
	return err
}

func (c *client) tearDownOpenGL() {
	sdl.GLDeleteContext(c.context)
}

func (c *client) run() error {
	log.Printf("num shapes: %d\n", len(c.scene.Shapes))

	tc := opengl.NewTextureCache(c.directory)
	defer tc.Delete()

	sc := opengl.NewShapeCompiler(tc)

	for _, s := range c.scene.Shapes {
		cs, err := sc.Compile(s)
		if err != nil {
			return err
		}
		// TODO: Implement me!
		_ = cs
	}

	log.Printf("Number of textures: %d\n", tc.NumTextures())
	return nil
}
