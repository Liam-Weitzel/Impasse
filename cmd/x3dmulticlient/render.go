package main

import (
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

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

func (c *client) updateProjectionScreen() {
	bounds := c.renderedImage.Bounds()
	aspect := float32(bounds.Dx()) / float32(bounds.Dy())
	c.updateProjection(aspect)
}

func (c *client) updateProjection(aspect float32) {
	c.renderer.ProjMat = mgl32.Perspective(
		mgl32.DegToRad(c.fov),
		aspect,
		nearPlane, farPlane)

}

func (c *client) compileShapes() error {

	sc := opengl.NewShapeCompiler(c.textureCache)

	c.shapes = make([]*opengl.CompiledShape, 0, len(c.scene.Shapes))

	for _, s := range c.scene.Shapes {
		cs, err := sc.Compile(s)
		if err != nil {
			return err
		}
		c.shapes = append(c.shapes, cs)
	}

	return nil
}

func (c *client) render() {

	if c.visibleShapes == nil {
		c.visibleShapes = make([]*opengl.CompiledShape, 0, len(c.shapes))
	}

	t0 := time.Now()
	center := c.camera.Position
	center[1] = -center[1]
	//fb := bs.Rotate(camera.Rotation(), center)
	bs := x3d.BoundingSphere{
		Radius: 1500 * 1500,
		Center: center,
	}
	for _, cs := range c.shapes {
		if bs.IntersectsSqr(cs.Bounds) {
			c.visibleShapes = append(c.visibleShapes, cs)
		}
	}
	/*
		log.Printf("total: %d vis: %d radius: %f pos: %v\n",
			len(css), len(vis), fb.Radius, camera.Position)
	*/
	c.renderer.RenderShapes(c.camera, c.visibleShapes)

	if len(c.attendees) > 0 {
		for _, att := range c.attendees {
			if c.withinRange(att.pos) {
				c.visibleAttendees = append(c.visibleAttendees,
					opengl.SpherePostion{
						Pos: att.pos,
						Col: att.col,
					})
			}
		}
		if len(c.visibleAttendees) > 0 {
			c.renderer.RenderSpheres(
				c.camera, c.sphere, c.visibleAttendees)
			c.visibleAttendees = c.visibleAttendees[:0]
		}
	}

	c.renderer.ReadImage(c.renderedImage)

	c.visibleShapes = c.visibleShapes[:0]
	t1 := time.Now()

	gfx.BlitRunes(c.screen, c.renderedImage, false)
	t2 := time.Now()
	c.conversionDuration = t2.Sub(t1)

	c.drawHUD()

	c.screen.Show()
	t3 := time.Now()
	c.renderDuration = t1.Sub(t0)
	c.termDuration = t3.Sub(t1)
}
