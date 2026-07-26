package main

import (
	"fmt"
	"image"
	"math"
	"time"
	"unicode/utf8"

	"github.com/Liam-Weitzel/Impasse/gfx"
	"github.com/Liam-Weitzel/Impasse/grid"
	"github.com/Liam-Weitzel/Impasse/proto"
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	nearPlane  = 1
	defaultFOV = 90

	// Redraw rate. Movement is interpolated between ticks, so this is the
	// animation rate and has nothing to do with the tick rate.
	framesPerSecond = 15

	playerRadius    = cellSize * 0.3
	objectiveRadius = cellSize * 0.18
	// Pickups float clear of the floor so they read at a steep camera angle,
	// and bob so they are the only moving thing on an empty floor.
	objectiveHeight  = cellSize * 0.3
	objectiveBob     = cellSize * 0.09
	objectivePeriod  = 2200 * time.Millisecond
	pickupSpinPeriod = 5000 * time.Millisecond
)

// deleteShapes releases a model's geometry.
func deleteShapes(shapes []*render.CompiledShape) {
	for _, cs := range shapes {
		cs.Delete()
	}
}

// attendee is another player, or us. Positions are held as the cell we came
// from and the cell we are going to, so a move can be drawn part way through.
type attendee struct {
	from mgl32.Vec3
	to   mgl32.Vec3
	col  mgl32.Vec3
	// stunned players are drawn washed out, so a fight reads at a glance.
	stunned bool
	// casting marks a burst already cast and about to land.
	casting bool
}

// at interpolates between from and to. alpha runs 0 to 1 across one tick.
func (a *attendee) at(alpha float32) mgl32.Vec3 {
	return a.from.Add(a.to.Sub(a.from).Mul(alpha))
}

type client struct {
	con    *connection
	userID uint64

	g         *grid.Grid
	atlas     *render.Atlas
	tilesPath string
	shapes    []*render.CompiledShape
	// visibleShapes is the culled subset drawn this frame, reused each time.
	visibleShapes []*render.CompiledShape
	// drawn and total report how much culling saved, for the HUD.
	drawn int

	window  *sdl.Window
	context sdl.GLContext
	screen  tcell.Screen

	fov      float32
	camera   *camera
	renderer *render.Renderer

	// playerModel and pickupModel are ordinary shapes drawn with a model
	// transform, so a marker can spin or be swapped without the renderer
	// knowing anything about it.
	playerModel []*render.CompiledShape
	pickupModel []*render.CompiledShape
	instances   []render.Instance

	fbo           uint32
	freeFBO       func()
	renderedImage *image.RGBA

	attendees map[uint64]*attendee

	// objectives are the uncollected pickups, in world coordinates.
	objectives []mgl32.Vec3
	arrow      []*render.CompiledShape
	telegraph  []*render.CompiledShape

	matchPhase  string
	matchNumber int
	matchLeft   time.Duration

	score      int
	channel    int
	stunned    int
	stunCD     int
	casting    int
	lootTicks  int
	stunRadius int

	tickDuration time.Duration
	tickAt       time.Time

	renderDuration     time.Duration
	conversionDuration time.Duration
	termDuration       time.Duration

	quit bool

	idleDuration    time.Duration
	sessionDuration time.Duration
	lastAction      time.Time
}

func startClient(
	con *connection,
	welcome *proto.Welcome,
	g *grid.Grid,
	screen tcell.Screen, window *sdl.Window,
	idleDuration time.Duration,
	sessionDuration time.Duration,
	tilesPath string,
) error {

	c := &client{
		tilesPath:       tilesPath,
		con:             con,
		userID:          welcome.ID,
		g:               g,
		fov:             defaultFOV,
		window:          window,
		screen:          screen,
		camera:          newCamera(),
		attendees:       make(map[uint64]*attendee),
		tickDuration:    time.Duration(welcome.TickMS) * time.Millisecond,
		lootTicks:       welcome.LootTicks,
		stunRadius:      welcome.StunRadius,
		idleDuration:    idleDuration,
		sessionDuration: sessionDuration,
	}
	if c.tickDuration <= 0 {
		c.tickDuration = 600 * time.Millisecond
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

func (c *client) allocFrameBuffer(w, h int) error {
	var err error
	if c.fbo, c.freeFBO, err = render.CreateFrameBuffer(
		int32(w), int32(h)); err != nil {
		return err
	}

	c.renderedImage = image.NewRGBA(image.Rect(0, 0, w, h))

	gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
	gl.Viewport(0, 0, int32(w), int32(h))

	return nil
}

func (c *client) run() error {

	var err error
	if c.renderer, err = render.NewRenderer(ambient); err != nil {
		return err
	}
	defer c.renderer.Delete()

	// Fog and the clear colour are the same value, or the world fades into
	// one colour and then meets a hard edge of another.
	c.renderer.SetFog(background, fogFar)

	sw, sh := c.screen.Size()
	aspect, rw, rh := fitSize(sw*4, sh*8)

	if err := c.allocFrameBuffer(rw, rh); err != nil {
		return err
	}
	defer func() { c.freeFBO() }()

	c.updateProjection(aspect)

	if c.playerModel, err = buildPlayerModel(playerRadius); err != nil {
		return err
	}
	defer deleteShapes(c.playerModel)

	if c.pickupModel, err = buildPickup(objectiveRadius); err != nil {
		return err
	}
	defer deleteShapes(c.pickupModel)

	if c.arrow, err = buildArrow(arrowColor); err != nil {
		return err
	}
	defer func() {
		for _, cs := range c.arrow {
			cs.Delete()
		}
	}()

	if c.telegraph, err = buildTelegraph(c.stunRadius); err != nil {
		return err
	}
	defer func() {
		for _, cs := range c.telegraph {
			cs.Delete()
		}
	}()

	if c.atlas, err = loadAtlas(c.tilesPath); err != nil {
		return err
	}
	defer c.atlas.Delete()
	c.renderer.SetAtlas(c.atlas)

	if c.shapes, err = buildGridMesh(c.g, c.atlas); err != nil {
		return err
	}
	defer func() {
		for _, cs := range c.shapes {
			cs.Delete()
		}
	}()

	events := make(chan tcell.Event)
	eventsDone := make(chan struct{})
	defer close(eventsDone)
	go c.screen.ChannelEvents(events, eventsDone)

	frames := time.NewTicker(time.Second / framesPerSecond)
	defer frames.Stop()

	start := time.Now()
	c.lastAction = start
	c.tickAt = start

	for !c.quit {
		select {
		case <-frames.C:
			c.render()
			c.checkTimeouts(time.Now(), start)

		case ev, ok := <-events:
			if !ok {
				c.quit = true
				break
			}
			c.handleEvent(ev)

		case state, ok := <-c.con.states:
			if !ok {
				// Server went away.
				c.quit = true
				break
			}
			c.applyState(state)
		}
	}

	return nil
}

func (c *client) checkTimeouts(now, start time.Time) {
	if c.idleDuration > 0 && c.lastAction.Add(c.idleDuration).Before(now) {
		c.quit = true
	}
	if c.sessionDuration > 0 && now.Sub(start) > c.sessionDuration {
		c.quit = true
	}
}

// applyState folds an authoritative tick into what is being drawn. Whatever a
// player was drawn at becomes the start of their next move, so a state arriving
// mid interpolation does not make anyone jump.
func (c *client) applyState(state proto.State) {
	alpha := c.alpha()
	seen := make(map[uint64]bool, len(state.Players))

	for _, p := range state.Players {
		seen[p.ID] = true
		to := cellCenter(grid.Pos{X: p.X, Y: p.Y})

		if p.ID == c.userID {
			c.score = p.Score
			c.channel = p.Channel
			c.stunned = p.Stunned
			c.stunCD = p.StunCD
			c.casting = p.Casting
		}

		a := c.attendees[p.ID]
		if a == nil {
			c.attendees[p.ID] = &attendee{
				from:    to,
				to:      to,
				col:     playerColor(p.ID),
				stunned: p.Stunned > 0,
				casting: p.Casting > 0,
			}
			continue
		}
		a.from = a.at(alpha)
		a.to = to
		a.stunned = p.Stunned > 0
		a.casting = p.Casting > 0
	}

	c.matchPhase = state.Match.Phase
	c.matchNumber = state.Match.Number
	c.matchLeft = time.Duration(state.Match.TicksRemaining) * c.tickDuration

	c.objectives = c.objectives[:0]
	for _, o := range state.Objectives {
		pos := cellCenter(grid.Pos{X: o.X, Y: o.Y})
		pos[2] += objectiveHeight
		c.objectives = append(c.objectives, pos)
	}

	for id := range c.attendees {
		if !seen[id] {
			delete(c.attendees, id)
		}
	}

	c.tickAt = time.Now()
}

// alpha is how far through the current tick we are.
func (c *client) alpha() float32 {
	if c.tickDuration <= 0 {
		return 1
	}
	a := float32(time.Since(c.tickAt)) / float32(c.tickDuration)
	return mgl32.Clamp(a, 0, 1)
}
func (c *client) handleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		c.resize()

	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEsc, tcell.KeyCtrlC:
			c.quit = true
		case tcell.KeyUp:
			c.camera.pitchUp()
		case tcell.KeyDown:
			c.camera.pitchDown()
		case tcell.KeyLeft:
			c.camera.rotateLeft()
		case tcell.KeyRight:
			c.camera.rotateRight()
		case tcell.KeyRune:
			c.handleRune(ev.Rune())
		}
	}
}

func (c *client) handleRune(r rune) {
	// '=' and '_' are the unshifted keys for '+' and '-', so both spellings
	// zoom rather than only the shifted ones.
	switch r {
	case '+', '=':
		c.camera.zoomIn()
		return
	case '-', '_':
		c.camera.zoomOut()
		return
	}

	// WASD moves, E bursts everyone around you, space loots. Space also
	// serves as hold position when there is nothing underfoot.
	switch r {
	case 'e':
		c.con.queueStun()
		c.lastAction = time.Now()
		return
	case ' ':
		c.con.queueLoot()
		c.lastAction = time.Now()
		return
	}

	if d := grid.DirectionForKey(r); d != grid.None {
		c.con.queueMove(d)
		c.lastAction = time.Now()
	}
}

func (c *client) resize() {
	c.screen.Sync()

	sw, sh := c.screen.Size()
	aspect, rw, rh := fitSize(sw*4, sh*8)

	c.freeFBO()
	if err := c.allocFrameBuffer(rw, rh); err != nil {
		// Nothing can be drawn without a framebuffer.
		c.quit = true
		return
	}

	c.updateProjection(aspect)
}

func (c *client) drawHUD() {
	st := tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorYellow)

	gfx.WriteString(c.screen, 0, 0,
		"ESC: Quit|WASD: Move|Space: Loot|E: Stun|Arrows: Camera|+/-: Zoom",
		st)

	width, height := c.screen.Size()

	status := fmt.Sprintf("%s|Score: %d|Left: %d",
		c.matchClock(), c.score, len(c.objectives))
	if c.channel > 0 {
		status += fmt.Sprintf("|Looting %d/%d", c.channel, c.lootTicks)
	}
	switch {
	case c.stunned > 0:
		status += fmt.Sprintf("|STUNNED %d", c.stunned)
	case c.casting > 0:
		status += "|CASTING"
	case c.stunCD > 0:
		status += fmt.Sprintf("|Stun in %d", c.stunCD)
	default:
		status += "|Stun ready"
	}
	gfx.WriteString(c.screen, width-utf8.RuneCountInString(status), 0, status, st)

	total := c.renderDuration + c.conversionDuration + c.termDuration
	var fps float64
	if total > 0 {
		fps = 1 / total.Seconds()
	}

	gfx.WriteString(c.screen, 0, height-1,
		fmt.Sprintf("3D: %5.2fms|Unicode: %5.2fms|Update: %5.2fms|FPS: %.2f|Chunks: %d/%d",
			float64(c.renderDuration.Microseconds())/1000,
			float64(c.conversionDuration.Microseconds())/1000,
			float64(c.termDuration.Microseconds())/1000,
			fps, c.drawn, len(c.shapes)), st)

	players := fmt.Sprintf("Players: %d", len(c.attendees))
	gfx.WriteString(c.screen, width-len(players), height-1, players, st)
}

func (c *client) render() {
	alpha := c.alpha()

	if me := c.attendees[c.userID]; me != nil {
		c.camera.target = me.at(alpha)
	}
	view := c.camera.view()

	t0 := time.Now()

	// Only the chunks that could be on screen. On a large map this is most
	// of the work saved.
	c.visibleShapes = cull(c.visibleShapes, c.renderer.ProjMat.Mul4(view), c.shapes)
	c.drawn = len(c.visibleShapes)

	c.renderer.RenderMesh(view, c.visibleShapes)

	c.drawTelegraphs(view, alpha)

	c.instances = c.instances[:0]
	for id, a := range c.attendees {
		pos := a.at(alpha)
		col := a.col
		if id == c.userID {
			col = col.Mul(selfBoost)
		}
		if a.stunned {
			col = stunnedColor
		}
		c.instances = append(c.instances, render.Instance{
			Model: mgl32.Translate3D(pos[0], pos[1], pos[2]),
			Col:   col,
		})
	}
	c.renderer.RenderInstances(view, c.playerModel, c.instances)

	if len(c.objectives) > 0 {
		// Bob and spin. A rotating silhouette is the cheapest way to make
		// something read as an object rather than a mark on the floor.
		bob := objectiveBob * (pulse(objectivePeriod) - 0.5) * 2
		spin := pulse(pickupSpinPeriod) * 2 * math.Pi

		c.instances = c.instances[:0]
		for _, pos := range c.objectives {
			c.instances = append(c.instances, render.Instance{
				Model: mgl32.Translate3D(pos[0], pos[1], pos[2]+bob).
					Mul4(mgl32.HomogRotate3DZ(spin)),
				Col: objectiveColor,
			})
		}
		c.renderer.RenderInstances(view, c.pickupModel, c.instances)
	}

	c.drawArrow(view, alpha)

	c.renderer.ReadImage(c.renderedImage)
	t1 := time.Now()

	gfx.BlitRunes(c.screen, c.renderedImage, false)
	t2 := time.Now()

	c.drawHUD()
	c.screen.Show()
	t3 := time.Now()

	c.renderDuration = t1.Sub(t0)
	c.conversionDuration = t2.Sub(t1)
	c.termDuration = t3.Sub(t2)
}

// matchClock is the round timer, counting down whichever phase is running.
func (c *client) matchClock() string {
	left := c.matchLeft
	if left < 0 {
		left = 0
	}
	mins := int(left / time.Minute)
	secs := int((left % time.Minute) / time.Second)

	switch c.matchPhase {
	case proto.PhaseRunning:
		return fmt.Sprintf("Match %d %d:%02d", c.matchNumber, mins, secs)
	case proto.PhaseIntermission:
		return fmt.Sprintf("Next match in %d:%02d", mins, secs)
	default:
		return "Waiting"
	}
}

// drawTelegraphs marks the ground every in-flight burst is about to cover.
func (c *client) drawTelegraphs(view mgl32.Mat4, alpha float32) {
	col := telegraphColor(telegraphBase, alpha)
	for _, a := range c.attendees {
		if !a.casting {
			continue
		}
		model := telegraphTransform(a.at(alpha), alpha)
		for _, cs := range c.telegraph {
			c.renderer.RenderShapeAt(view, model, cs, col)
		}
	}
}

// drawArrow points at the nearest uncollected pickup by straight-line bearing.
// Nothing is drawn once they are all gone.
func (c *client) drawArrow(view mgl32.Mat4, alpha float32) {
	me := c.attendees[c.userID]
	if me == nil {
		return
	}

	from := me.at(alpha)
	target, ok := nearestTo(from, c.objectives)
	if !ok {
		return
	}

	model := arrowTransform(from, target)
	for _, cs := range c.arrow {
		c.renderer.RenderShapeAt(view, model, cs, arrowColor)
	}
}

func (c *client) updateProjection(aspect float32) {
	perspective := mgl32.Perspective(
		// Halved to compensate for terminal cells being about twice as
		// tall as they are wide.
		mgl32.DegToRad(c.fov)*0.5,
		aspect,
		nearPlane, c.camera.far())

	// glReadPixels hands back rows starting at the bottom left, because the
	// GL origin is down there, while image.RGBA row 0 is the top. Without
	// this the picture reaches the terminal upside down. Flipping here costs
	// nothing, where flipping the image after readback would copy the whole
	// framebuffer every frame.
	//
	// It reverses triangle orientation on screen, so the renderer treats
	// clockwise as front facing. See RenderMesh.
	c.renderer.ProjMat = flipY.Mul4(perspective)
}

// flipY mirrors clip space vertically.
var flipY = mgl32.Scale3D(1, -1, 1)

const (
	maxWidth  = 1360
	maxHeight = 768
)

func fit(w, h int) bool {
	return w <= maxWidth && h <= maxHeight
}

func aspectScale(x int, aspect float32) int {
	return int(math.Ceil(float64(float32(x) * aspect)))
}

func fitSize(w, h int) (float32, int, int) {
	aspect := float32(w) / float32(h)

	if fit(w, h) {
		return aspect, w, h
	}

	c1w, c1h := maxWidth, aspectScale(maxWidth, 1/aspect)
	c2w, c2h := aspectScale(maxHeight, aspect), maxHeight

	c1f := fit(c1w, c1h)
	c2f := fit(c2w, c2h)

	if c1f && c2f {
		if c1w*c1h > c2w*c2h {
			return aspect, c1w, c1h
		}
		return aspect, c2w, c2h
	}

	if c1f {
		return aspect, c1w, c1h
	}
	return aspect, c2w, c2h
}
