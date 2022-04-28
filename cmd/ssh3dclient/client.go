package main

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/bamiaux/rez"
	"github.com/gdamore/tcell/v2"
	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/veandco/go-sdl2/sdl"

	_ "embed"
)

const framesPerSecond = 10

const (
	renderWidth  = 640
	renderHeight = 480
)

//go:embed texture.vert
var vertexSrc string

//go:embed texture.frag
var fragSrc string

//go:embed texture.png
var textureSrc []byte

const (
	cubeSize  = 100
	cubeSize2 = cubeSize / 2
)

var (
	t00 = [2]float32{0, 0}
	t10 = [2]float32{1, 0}
	t11 = [2]float32{1, 1}
	t01 = [2]float32{0, 1}

	frontN  = [3]float32{0, 0, +1}
	backN   = [3]float32{0, 0, -1}
	leftN   = [3]float32{-1, 0, 0}
	rightN  = [3]float32{+1, 0, 0}
	topN    = [3]float32{0, +1, 0}
	bottomN = [3]float32{0, -1, 0}

	vertices = []vertex{
		// front
		{[3]float32{-cubeSize2, -cubeSize2, +cubeSize2}, t00, frontN},
		{[3]float32{+cubeSize2, -cubeSize2, +cubeSize2}, t10, frontN},
		{[3]float32{+cubeSize2, +cubeSize2, +cubeSize2}, t11, frontN},
		{[3]float32{-cubeSize2, +cubeSize2, +cubeSize2}, t01, frontN},
		// back
		{[3]float32{+cubeSize2, -cubeSize2, -cubeSize2}, t00, backN},
		{[3]float32{-cubeSize2, -cubeSize2, -cubeSize2}, t10, backN},
		{[3]float32{-cubeSize2, +cubeSize2, -cubeSize2}, t11, backN},
		{[3]float32{+cubeSize2, +cubeSize2, -cubeSize2}, t01, backN},
		// left
		{[3]float32{-cubeSize2, -cubeSize2, -cubeSize2}, t00, leftN},
		{[3]float32{-cubeSize2, -cubeSize2, +cubeSize2}, t10, leftN},
		{[3]float32{-cubeSize2, +cubeSize2, +cubeSize2}, t11, leftN},
		{[3]float32{-cubeSize2, +cubeSize2, -cubeSize2}, t01, leftN},
		// right
		{[3]float32{+cubeSize2, -cubeSize2, +cubeSize2}, t00, rightN},
		{[3]float32{+cubeSize2, -cubeSize2, -cubeSize2}, t10, rightN},
		{[3]float32{+cubeSize2, +cubeSize2, -cubeSize2}, t11, rightN},
		{[3]float32{+cubeSize2, +cubeSize2, +cubeSize2}, t01, rightN},
		// top
		{[3]float32{+cubeSize2, +cubeSize2, -cubeSize2}, t00, topN},
		{[3]float32{-cubeSize2, +cubeSize2, -cubeSize2}, t10, topN},
		{[3]float32{-cubeSize2, +cubeSize2, +cubeSize2}, t11, topN},
		{[3]float32{+cubeSize2, +cubeSize2, +cubeSize2}, t01, topN},
		// bottom
		{[3]float32{-cubeSize2, -cubeSize2, -cubeSize2}, t00, bottomN},
		{[3]float32{+cubeSize2, -cubeSize2, -cubeSize2}, t10, bottomN},
		{[3]float32{+cubeSize2, -cubeSize2, +cubeSize2}, t11, bottomN},
		{[3]float32{-cubeSize2, -cubeSize2, +cubeSize2}, t01, bottomN},
	}
)

type client struct {
	fbo          uint32
	numIndices   int32
	modelMat     mgl32.Mat4
	viewMat      mgl32.Mat4
	mvMatLoc     int32
	normalMatLoc int32
	prevTime     time.Time

	img    *image.RGBA
	canvas *image.RGBA

	geoms bool

	window    *sdl.Window
	glVersion string

	cleanup func(*client)
}

func newClient(window *sdl.Window) *client {
	return &client{
		window: window,
		img:    image.NewRGBA(image.Rect(0, 0, renderWidth, renderHeight)),
	}
}

func (c *client) chainCleanUp(fn func(*client)) {
	prev := c.cleanup
	c.cleanup = func(c *client) {
		fn(c)
		if prev != nil {
			prev(c)
		}
	}
}

func (c *client) shutdown() {
	sdl.Do(func() {
		if c.cleanup != nil {
			c.cleanup(c)
			c.cleanup = nil
		}
	})
}

func (c *client) setupOpenGL() error {
	var err error
	sdl.Do(func() { err = c.setupOGL() })
	return err
}

func (c *client) setupOGL() error {

	//var fbo uint32
	gl.GenFramebuffers(1, &c.fbo)

	var colorBuffer uint32
	gl.GenRenderbuffers(1, &colorBuffer)
	gl.BindRenderbuffer(gl.RENDERBUFFER, colorBuffer)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, renderWidth, renderHeight)
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	var depthBuffer uint32
	gl.GenRenderbuffers(1, &depthBuffer)
	gl.BindRenderbuffer(gl.RENDERBUFFER, depthBuffer)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT32F, renderWidth, renderHeight)
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	// attach render buffer to the fbo as depth buffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, colorBuffer)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, depthBuffer)

	c.chainCleanUp(func(*client) {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.DeleteRenderbuffers(1, &colorBuffer)
		gl.DeleteRenderbuffers(1, &depthBuffer)
		gl.DeleteFramebuffers(1, &c.fbo)
	})

	if fbs := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); fbs != gl.FRAMEBUFFER_COMPLETE {
		return fmt.Errorf("fbo status: %d", fbs)
	}

	var prog uint32
	var err error
	//log.Println(vertexSrc)
	prog, err = loadShaderProg(vertexSrc, fragSrc)
	if err != nil {
		return err
	}
	c.chainCleanUp(func(*client) { gl.DeleteProgram(prog) })

	gl.UseProgram(prog)

	var img *image.RGBA
	if img, err = rgbaFromBytes(textureSrc); err != nil {
		return err
	}

	var texture uint32
	if texture, err = loadTextureFromRGBA(img); err != nil {
		return err
	}
	c.chainCleanUp(func(*client) { gl.DeleteBuffers(1, &texture) })

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	var (
		texSamplerUniformLoc int32
		projMatLoc           int32
		lightPosLoc          int32
		ambientColLoc        int32
		diffuseColLoc        int32
	)

	for _, l := range []struct {
		name string
		addr *int32
	}{
		{"texSampler", &texSamplerUniformLoc},
		{"mvMat", &c.mvMatLoc},
		{"projMat", &projMatLoc},
		{"ambientCol", &ambientColLoc},
		{"diffuseCol", &diffuseColLoc},
		{"normalMat", &c.normalMatLoc},
		{"lightPos", &lightPosLoc},
	} {
		if *l.addr = gl.GetUniformLocation(
			prog, gl.Str(l.name+"\x00")); *l.addr < 0 {
			return fmt.Errorf("could not find uniform '%s'", l.name)
		}
	}

	gl.Uniform1i(texSamplerUniformLoc, 0)

	var vbo uint32
	if vbo, err = vboCreate(vertices); err != nil {
		return err
	}
	c.chainCleanUp(func(*client) { gl.DeleteBuffers(1, &vbo) })

	// Enable and set up the depth buffer.

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)

	gl.ClearColor(0, 0, 0, 1)
	gl.ClearDepthf(1)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	indices := genIndices(vertices)
	c.numIndices = int32(len(indices))
	var ibo uint32
	if ibo, err = iboCreate(indices); err != nil {
		return err
	}
	c.chainCleanUp(func(*client) { gl.DeleteBuffers(1, &ibo) })

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo)

	const positionIdx = 0
	gl.VertexAttribPointer(
		positionIdx, 3, gl.FLOAT, false,
		int32(vertexSize), gl.PtrOffset(0))
	gl.EnableVertexAttribArray(positionIdx)

	const texCoordIdx = 1
	gl.VertexAttribPointer(
		texCoordIdx, 2, gl.FLOAT, false,
		int32(vertexSize),
		gl.PtrOffset(int(texCoordOfs)))
	gl.EnableVertexAttribArray(texCoordIdx)

	const normalIdx = 2
	gl.VertexAttribPointer(
		normalIdx, 3, gl.FLOAT, false,
		int32(vertexSize),
		gl.PtrOffset(int(normalOfs)))
	gl.EnableVertexAttribArray(normalIdx)

	// Set the object's pose.

	c.modelMat = mgl32.
		HomogRotate3DX(math.Pi / 4).
		Mul4(mgl32.HomogRotate3DY(math.Pi / 4))

	const (
		camPosX = 0
		camPosY = 0
		camPosZ = 150
	)

	lightPos := mgl32.Vec3{camPosX + 50, camPosY + 80, camPosZ}
	ambientCol := mgl32.Vec3{0.15, 0.15, 0.15}
	diffuseCol := mgl32.Vec3{1.2, 1.2, 1.2}

	c.viewMat = mgl32.Translate3D(-camPosX, -camPosY, -camPosZ)

	projMat := mgl32.Perspective(
		mgl32.DegToRad(60),
		float32(renderWidth)/renderHeight,
		1, 1000)

	gl.UniformMatrix4fv(projMatLoc, 1, false, &projMat[0])

	gl.Uniform3fv(lightPosLoc, 1, &lightPos[0])
	gl.Uniform3fv(ambientColLoc, 1, &ambientCol[0])
	gl.Uniform3fv(diffuseColLoc, 1, &diffuseCol[0])

	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)

	c.prevTime = time.Now()

	return nil
}

func genIndices(vertices []vertex) []uint16 {
	fans := []uint16{0, 1, 2, 2, 3, 0}
	const vertsPerSide = 4
	const indicesPerSide = 6
	numSides := len(vertices) / 4
	numIndices := indicesPerSide * numSides
	indices := make([]uint16, numIndices)
	i := 0
	for j := 0; j < numSides; j++ {
		sideBaseIdx := uint16(j * vertsPerSide)
		for _, f := range fans {
			indices[i] = sideBaseIdx + f
			i++
		}
	}
	return indices
}

func (c *client) renderOpenGL() {
	currentTime := time.Now()
	elapsed := float32(currentTime.Sub(c.prevTime).Seconds())
	c.prevTime = currentTime

	const cubeAngVel = float32(0.75) // Radian/s
	cubeRotAxis := mgl32.Vec3{1, 0, 1}.Normalize()

	angle := cubeAngVel * elapsed
	//log.Printf("angle: %f\n", mgl32.RadToDeg(angle))
	rotMat := mgl32.HomogRotate3D(angle, cubeRotAxis)

	c.modelMat = rotMat.Mul4(c.modelMat)
	mvMat := c.viewMat.Mul4(c.modelMat)
	normalMat := mvMat.Inv().Transpose()
	gl.UniformMatrix4fv(c.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(c.normalMatLoc, 1, false, &normalMat[0])

	gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)

	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.DrawElements(gl.TRIANGLES,
		c.numIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))

	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

	gl.ReadPixels(
		0, 0,
		renderWidth, renderHeight,
		gl.RGBA, gl.UNSIGNED_BYTE,
		unsafe.Pointer(&c.img.Pix[0]))

	//c.window.GLSwap()
}

func (c *client) blitRunesConcurrent(s tcell.Screen) {
	width, height := c.canvas.Rect.Dx(), c.canvas.Rect.Dy()

	var wg sync.WaitGroup

	rows := make(chan int)

	n := runtime.NumCPU()

	type cell struct {
		r  rune
		st tcell.Style
	}

	type rowRes struct {
		row   int
		cells []cell
	}

	leaky := make(chan []cell, n)

	alloc := func() []cell {
		select {
		case cells := <-leaky:
			return cells
		default:
			return make([]cell, width/4)
		}
	}

	free := func(cells []cell) {
		select {
		case leaky <- cells:
		default:
		}
	}

	converted := make(chan rowRes, n)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for res := range converted {
			for j := range res.cells {
				cell := &res.cells[j]
				s.SetContent(j, res.row, cell.r, nil, cell.st)
			}
			free(res.cells)
		}
	}()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rc := NewRuneConverter(c.geoms)
			for i := range rows {
				x, y := 0, i*8
				cells := alloc()
				for j := range cells {
					rc.Extract(c.canvas, x, y)
					st := tcell.StyleDefault.
						Foreground(tcell.NewRGBColor(
							int32(rc.FGColor[0]),
							int32(rc.FGColor[1]),
							int32(rc.FGColor[2]))).
						Background(tcell.NewRGBColor(
							int32(rc.BGColor[0]),
							int32(rc.BGColor[1]),
							int32(rc.BGColor[2])))
					cells[j] = cell{
						r:  rc.CodePoint,
						st: st,
					}
					x += 4
				}
				converted <- rowRes{row: i, cells: cells}
			}
		}()
	}

	for i, y := 0, 0; y < height; y, i = y+8, i+1 {
		rows <- i
	}

	close(rows)
	wg.Wait()
	close(converted)
	<-done
}

func writeString(sc tcell.Screen, x, y int, s string, st tcell.Style) {
	for _, r := range s {
		if r != ' ' {
			sc.SetContent(x, y, r, nil, st)
		}
		x++
	}
}

func (c *client) hud(s tcell.Screen, frameTime time.Duration) {
	if c.glVersion == "" {
		sdl.Do(func() {
			c.glVersion = gl.GoStr(gl.GetString(gl.VERSION))
		})
	}
	st := tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorYellow)

	width, height := s.Size()

	var geoms string
	if c.geoms {
		geoms = "on"
	} else {
		geoms = "off"
	}

	writeString(s, 0, 0,
		fmt.Sprintf("ESC: Quit|G: geometrical shapes [%s]", geoms), st)

	driver := fmt.Sprintf("Driver: %s", c.glVersion)

	writeString(s, width-len(driver), height-1, driver, st)

	writeString(s, 0, height-1, fmt.Sprintf("Frame time: %v", frameTime), st)
}

func (c *client) render(screen tcell.Screen) {
	start := time.Now()
	sdl.Do(c.renderOpenGL)

	swidth, sheight := screen.Size()

	sdim := image.Rect(0, 0, 4*swidth, 8*sheight)

	if c.canvas == nil || !c.canvas.Bounds().Eq(sdim) {
		c.canvas = image.NewRGBA(sdim)
	}

	rez.Convert(c.canvas, c.img, rez.NewBilinearFilter())

	c.blitRunesConcurrent(screen)

	c.hud(screen, time.Since(start))

	/*
		f, err := os.Create("xxx.png")
		if err == nil {
			png.Encode(f, c.img)
			f.Close()
		}
	*/
}

func (c *client) run(screen tcell.Screen) error {

	defer c.shutdown()

	if err := c.setupOpenGL(); err != nil {
		return err
	}

	defStyle := tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorWhite)
	screen.SetStyle(defStyle)

	screen.Clear()

	done := make(chan struct{})
	ticker := time.NewTicker(time.Second / framesPerSecond)

	defer func() {
		ticker.Stop()
		close(done)
	}()

	events := make(chan tcell.Event)
	go screen.ChannelEvents(events, done)

	for {
		c.render(screen)
		screen.Show()

		select {
		case <-ticker.C:

		case ev := <-events:
			switch ev := ev.(type) {
			case *tcell.EventResize:
				c.render(screen)
				screen.Sync()
			case *tcell.EventKey:
				switch ev.Key() {
				case tcell.KeyEsc, tcell.KeyCtrlC:
					return nil
				case tcell.KeyRune:
					switch ev.Rune() {
					case 'g':
						c.geoms = !c.geoms
					}
				}
			}
		}
	}

	return nil
}
