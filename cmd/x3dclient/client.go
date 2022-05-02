package main

import (
	"errors"
	"image"
	"image/png"
	"log"
	"os"
	"unsafe"

	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

const (
	displayWidth  = 640
	displayHeight = 400
	fov           = 60
	near          = 1
	far           = 1000
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

	if len(c.scene.Viewpoints) == 0 {
		return errors.New("no viewpoints defined")
	}

	ambientCol := mgl32.Vec3{0.15, 0.15, 0.15}

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

	css := make([]*opengl.CompiledShape, len(c.scene.Shapes))

	for i, s := range c.scene.Shapes {
		cs, err := sc.Compile(s)
		if err != nil {
			return err
		}
		css[i] = cs
	}

	gl.Enable(gl.PRIMITIVE_RESTART_FIXED_INDEX)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.ActiveTexture(gl.TEXTURE0)

	gl.ClearColor(0, 1, 0, 1)
	gl.ClearDepthf(1)

	camera := &opengl.Camera{
		Position: c.scene.Viewpoints[0].Position,
	}

	renderer.Render(camera, css)

	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

	out := image.NewRGBA(image.Rect(0, 0, displayWidth, displayHeight))

	gl.ReadPixels(
		0, 0,
		displayWidth, displayHeight,
		gl.RGBA, gl.UNSIGNED_BYTE,
		unsafe.Pointer(&out.Pix[0]))

	f, err := os.Create("out.png")
	if err == nil {
		png.Encode(f, out)
		f.Close()
	}

	log.Printf("Number of textures: %d\n", tc.NumTextures())
	return nil
}
