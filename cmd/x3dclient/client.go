package main

import (
	"errors"
	"image"
	"log"

	"github.com/bamiaux/rez"
	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

const (
	displayWidth  = 640
	displayHeight = 400
	fov           = 70
	near          = 1
	far           = 1500
)

type client struct {
	scene     *x3d.Scene
	directory string

	window  *sdl.Window
	context sdl.GLContext

	screen tcell.Screen

	canvas *image.RGBA
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

	if len(c.scene.Viewpoints) == 0 {
		return errors.New("no viewpoints defined")
	}

	fbo, fboFree, err := opengl.CreateFrameBuffer(displayWidth, displayHeight)
	if err != nil {
		return err
	}
	defer fboFree()
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)

	//ambientCol := mgl32.Vec3{0.15, 0.15, 0.15}
	ambientCol := mgl32.Vec3{0.75, 0.75, 0.75}
	//ambientCol := mgl32.Vec3{1.5, 1.5, 1.5}

	projMat := mgl32.Perspective(
		mgl32.DegToRad(fov),
		float32(displayWidth)/displayHeight,
		near, far)

	renderer, err := opengl.NewRenderer(ambientCol, projMat)
	if err != nil {
		return err
	}
	defer renderer.Delete()

	log.Printf("num shapes: %d\n", len(c.scene.Shapes))

	tc := opengl.NewTextureCache(c.directory)
	defer tc.Delete()

	sc := opengl.NewShapeCompiler(tc)

	css := make([]*opengl.CompiledShape, 0, len(c.scene.Shapes))

	for _, s := range c.scene.Shapes {
		cs, err := sc.Compile(s)
		if err != nil {
			return err
		}
		css = append(css, cs)
	}

	camera := &opengl.Camera{
		Position: c.scene.Viewpoints[0].Position,
	}

	out := image.NewRGBA(image.Rect(0, 0, displayWidth, displayHeight))

	done := make(chan struct{})
	defer close(done)

	events := make(chan tcell.Event)
	go c.screen.ChannelEvents(events, done)

	render := func() {
		renderer.Render(camera, css, out)

		swidth, sheight := c.screen.Size()
		sdim := image.Rect(0, 0, 4*swidth, 8*sheight)

		if c.canvas == nil || !c.canvas.Bounds().Eq(sdim) {
			c.canvas = image.NewRGBA(sdim)
		}

		rez.Convert(c.canvas, out, rez.NewBilinearFilter())

		gfx.BlitRunes(c.screen, c.canvas, false)
	}

	for {
		render()

		ev, ok := <-events
		if !ok {
			break
		}
		switch ev := ev.(type) {
		case *tcell.EventResize:
			render()
			c.screen.Sync()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEsc, tcell.KeyCtrlC:
				return nil
			case tcell.KeyRune:
				switch ev.Rune() {
				case 'w':
				case 'a':
				case 's':
				case 'd':
				}
			}
		}

		/*
			f, err := os.Create("out.png")
			if err == nil {
				png.Encode(f, out)
				f.Close()
			}
		*/
	}

	return nil
}
