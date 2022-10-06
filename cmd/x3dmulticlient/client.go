package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"math/rand"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

const (
	far        = 1300
	nearPlane  = 0.1
	farPlane   = 4096
	minFOV     = 60
	maxFOV     = 130
	defaultFOV = 90
)

type client struct {
	scene      *x3d.Scene
	directory  string
	connection *connection
	userID     uint64

	quit  bool
	dirty bool

	window  *sdl.Window
	context sdl.GLContext
	screen  tcell.Screen

	textureCache *opengl.TextureCache

	shapes           []*opengl.CompiledShape
	visibleShapes    []*opengl.CompiledShape
	visibleAttendees []opengl.SpherePostion

	fov      float32
	camera   *opengl.Camera
	renderer *opengl.Renderer

	fbo           uint32
	freeFBO       func()
	renderedImage *image.RGBA

	renderDuration     time.Duration
	conversionDuration time.Duration
	termDuration       time.Duration

	attendees map[uint64]*attendee
	color     mgl32.Vec3

	rnd *rand.Rand

	sphere *opengl.Sphere

	idleDuration    time.Duration
	sessionDuration time.Duration
	lastAction      time.Time
}

func (c *client) allocFrameBuffer(w, h int) error {

	var err error
	if c.fbo, c.freeFBO, err = opengl.CreateFrameBuffer(
		int32(w), int32(h)); err != nil {
		return err
	}

	c.renderedImage = image.NewRGBA(image.Rect(0, 0, w, h))

	gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
	gl.Viewport(0, 0, int32(w), int32(h))

	return nil
}

func startClient(
	scene *x3d.Scene, directory string,
	connection string, userID uint64,
	screen tcell.Screen, window *sdl.Window,
	idleDuration time.Duration,
	sessionDuration time.Duration,
) error {

	con, err := newConnection(connection)
	if err != nil {
		return err
	}
	conDone := make(chan struct{})
	defer close(conDone)
	con.run(conDone)

	c := &client{
		scene:           scene,
		fov:             defaultFOV,
		directory:       directory,
		window:          window,
		screen:          screen,
		userID:          userID,
		connection:      con,
		attendees:       make(map[uint64]*attendee),
		idleDuration:    idleDuration,
		sessionDuration: sessionDuration,
	}

	//log.Printf("connection: %s\n", connection)
	//log.Printf("user ID: %d\n", userID)

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
		"ESC: Quit|Cursor/WASD: Move|PgUp/PgD: Look up/down|SPACE/C: Up/Down|R: Random Position",
		st)

	width, height := c.screen.Size()

	totalTime := c.renderDuration + c.conversionDuration + c.termDuration

	fps := 1 / totalTime.Seconds()

	gfx.WriteString(c.screen, 0, height-1,
		fmt.Sprintf("3D: %5.2fms|Unicode: %5.2fms|Update: %5.2fms|FPS: %.2f",
			float64(c.renderDuration.Microseconds())/1000,
			float64(c.conversionDuration.Microseconds())/1000,
			float64(c.termDuration.Microseconds())/1000,
			fps), st)

	fovS := fmt.Sprintf("FOV: +/- %3.0f°", c.fov)
	gfx.WriteString(
		c.screen, width-utf8.RuneCountInString(fovS), height-1,
		fovS, st)
}

func (c *client) run() error {

	if len(c.scene.Viewpoints) == 0 {
		return errors.New("no viewpoints defined")
	}

	ambientCol := mgl32.Vec3{0.75, 0.75, 0.75}

	var err error
	if c.renderer, err = opengl.NewRenderer(ambientCol); err != nil {
		return err
	}
	defer c.renderer.Delete()

	sw, sh := c.screen.Size()
	aspect, rw, rh := fitSize(sw*4, sh*8)

	if err := c.allocFrameBuffer(rw, rh); err != nil {
		return err
	}
	defer func() { c.freeFBO() }()

	c.updateProjection(aspect)

	if c.sphere, err = opengl.NewSphere(15, 16, 16, false); err != nil {
		return err
	}
	defer c.sphere.Delete()

	c.textureCache = opengl.NewTextureCache(c.directory)
	defer c.textureCache.Delete()

	if err := c.compileShapes(); err != nil {
		return nil
	}

	c.rnd = rand.New(rand.NewSource(time.Now().Unix()))
	vpN := c.rnd.Intn(len(c.scene.Viewpoints))
	vp := c.scene.Viewpoints[vpN]

	r, g, b := gfx.RandomColor(c.rnd)
	c.color = mgl32.Vec3{float32(r) / 255, float32(g) / 255, float32(b) / 255}

	p := vp.Position
	//p[0], p[2] = p[2], p[0]
	p[1] = -p[1]

	angle := vp.Orientation[3]

	if vp.Orientation[0] > 0 {
		if angle += math.Pi; angle > 2*math.Pi {
			angle -= 2 * math.Pi
		}
	}

	c.camera = &opengl.Camera{
		Position: p, // c.scene.Viewpoints[0].Position,
		Angle:    angle,
	}

	// introduce ourself
	c.connection.sendHello(c.userID, p, c.color)

	// Keybord handling.

	eventsDone := make(chan struct{})
	defer close(eventsDone)

	events := make(chan tcell.Event)
	go c.screen.ChannelEvents(events, eventsDone)

	keys := make(chan batch)

	go batching(events, keys, keyboardConvert)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	start := time.Now()

	c.lastAction = start

	for c.dirty = true; !c.quit; {
		if c.dirty {
			c.dirty = false
			c.render()
		}
		select {
		case now := <-ticker.C:
			if c.idleDuration > 0 {
				if c.lastAction.Add(c.idleDuration).Before(now) {
					c.quit = true
				}
			}
			if !c.quit && c.sessionDuration > 0 && now.Sub(start) > c.sessionDuration {
				c.quit = true
			}
		case k, ok := <-keys:
			if !ok {
				break
			}
			k.run(c)

		case m, ok := <-c.connection.in:
			if !ok {
				break
			}
			m.run(c)
		}
	}

	// TODO: This should block.
	c.connection.sendLeave(c.userID)

	return nil
}

func (c *client) moveAttendee(id uint64, x, y, z float32) {

	// log.Printf("move %d -> [%.2f, %.2f, %.2f]\n", id, x, y, z)
	att := c.attendees[id]
	if att == nil {
		return
	}
	wasVisible := c.withinRange(att.pos)
	att.pos = mgl32.Vec3{x, y, z}
	if wasVisible || c.withinRange(att.pos) {
		c.dirty = true
	}
}

func (c *client) helloAttendee(id uint64, x, y, z float32, r, g, b byte) {

	/*
		log.Printf("hello %d -> [%.2f, %.2f, %.2f] (%02x, %02x, %02x)\n",
			id,
			x, y, z,
			r, g, b)
	*/

	// Don't register if we already know this one.
	if c.attendees[id] != nil {
		return
	}

	pos := mgl32.Vec3{x, y, z}

	c.attendees[id] = &attendee{
		pos: pos,
		col: mgl32.Vec3{float32(r) / 255, float32(g) / 255, float32(b) / 255},
	}

	// introduce ourself
	c.connection.sendHello(
		c.userID,
		c.camera.Position,
		c.color)

	if c.withinRange(pos) {
		c.dirty = true
	}
}

func (c *client) withinRange(pos mgl32.Vec3) bool {
	cpos := c.camera.Position
	return cpos.Sub(pos).LenSqr() < (far+1)*(far+1)
}

func (c *client) leaveAttendee(id uint64) {
	att := c.attendees[id]
	if att == nil {
		return
	}
	if c.withinRange(att.pos) {
		c.dirty = true
	}
	log.Printf("leave object: %d\n", id)
	delete(c.attendees, id)
	log.Printf("attendees left: %d\n", len(c.attendees)+1)
}
