package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"time"

	"github.com/bamiaux/rez"
	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
	//_ "github.com/gdamore/v2/terminfo/extended"
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

	frameDuration time.Duration
	termDuration  time.Duration
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

func (c *client) drawHUD() {
	st := tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorYellow)

	gfx.WriteString(c.screen, 0, 0,
		"ESC: Quit|Cursor/WASD: Move|PgUp/PgD: Look up/down|SPACE/C: Up/Down",
		st)

	_, height := c.screen.Size()

	gfx.WriteString(c.screen, 0, height-1,
		fmt.Sprintf("Frame time: %.2fms [%.2fms]",
			float64(c.frameDuration.Microseconds()/1000),
			float64(c.termDuration.Microseconds())/1000), st)

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

	p := c.scene.Viewpoints[0].Position
	//p[0], p[2] = p[2], p[0]
	p[1] = -p[1]

	camera := &opengl.Camera{
		Position: p, // c.scene.Viewpoints[0].Position,
		Angle:    mgl32.DegToRad(270),
	}

	out := image.NewRGBA(image.Rect(0, 0, displayWidth, displayHeight))

	render := func() {
		t0 := time.Now()
		renderer.Render(camera, css, out)
		t1 := time.Now()

		swidth, sheight := c.screen.Size()
		sdim := image.Rect(0, 0, 4*swidth, 8*sheight)

		if c.canvas == nil || !c.canvas.Bounds().Eq(sdim) {
			c.canvas = image.NewRGBA(sdim)
		}

		rez.Convert(c.canvas, out, rez.NewBilinearFilter())

		gfx.BlitRunes(c.screen, c.canvas, false)

		c.drawHUD()

		c.screen.Show()
		t2 := time.Now()
		c.frameDuration = t1.Sub(t0)
		c.termDuration = t2.Sub(t1)
	}

	const (
		stepWidth = 4
		rotAngel  = 5
	)

	cooked := make(chan func())

	var leave bool

	action := func(ev tcell.Event) func() {
		//log.Println("action called")
		switch ev := ev.(type) {
		case *tcell.EventResize:
			return func() {
				//render()
				c.screen.Sync()
			}
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEsc, tcell.KeyCtrlC:
				return func() {
					//log.Println("ESC pressed")
					leave = true
				}
			case tcell.KeyRune:
				switch ev.Rune() {
				case 'w':
					return func() { camera.Forward(stepWidth) }
				case 'a':
					return func() { camera.StrafeLeft(stepWidth) }
				case 's':
					return func() { camera.Backward(stepWidth) }
				case 'd':
					return func() { camera.StrafeRight(stepWidth) }
				case ' ':
					return func() { camera.Up(stepWidth) }
				case 'c':
					return func() { camera.Down(stepWidth) }
				}
			case tcell.KeyUp:
				return func() { camera.Forward(stepWidth) }
			case tcell.KeyDown:
				return func() { camera.Backward(stepWidth) }
			case tcell.KeyLeft:
				return func() { camera.RotateLeft(mgl32.DegToRad(rotAngel)) }
			case tcell.KeyRight:
				return func() { camera.RotateRight(mgl32.DegToRad(rotAngel)) }
			case tcell.KeyPgUp:
				return func() { camera.RotateUp(mgl32.DegToRad(rotAngel)) }
			case tcell.KeyPgDn:
				return func() { camera.RotateDown(mgl32.DegToRad(rotAngel)) }
			}
		}
		return func() {}
	}

	eventsDone := make(chan struct{})

	events := make(chan tcell.Event)
	go c.screen.ChannelEvents(events, eventsDone)

	cookedDone := make(chan struct{})
	defer close(cookedDone)

	combine := func(a, b func()) func() {
		return func() { a(); b() }
	}

	go func() {
		defer close(eventsDone)
		for {
			//log.Println("A: before first")
			var fn func()
			for fn == nil {
				select {
				case ev, ok := <-events:
					//log.Printf("A: new event %t: %v\n", ok, ev)
					if !ok {
						fn = func() { leave = true }
					} else {
						fn = action(ev)
					}
				case <-cookedDone:
					return
				}
			}
			//log.Println("A: before send")
		send:
			for {
				select {
				case ev := <-events:
					//log.Println("A: second event")
					fn = combine(fn, action(ev))
				case cooked <- fn:
					//log.Println("A: send success")
					fn = nil
					break send
				}
			}
		}
	}()

	for !leave {
		render()
		//log.Println("B: waiting for cooked")
		fn, ok := <-cooked
		//log.Println("B: recieved cooked", ok)
		if !ok {
			break
		}
		fn()
	}

	/*
		f, err := os.Create("out.png")
		if err == nil {
			png.Encode(f, out)
			f.Close()
		}
	*/

	return nil
}
