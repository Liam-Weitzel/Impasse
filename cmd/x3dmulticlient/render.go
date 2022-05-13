package main

import (
	"image"
	"time"

	"github.com/bamiaux/rez"
	"gitlab.com/sascha.l.teichmann/ssh3d/gfx"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

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
	if c.renderedImage == nil {
		c.renderedImage = image.NewRGBA(image.Rect(0, 0, displayWidth, displayHeight))
	}

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
	pos := c.scene.Viewpoints[0].Position
	//pos[1] = -pos[1]

	if len(c.attendees) > 0 {

		spheres := make([]opengl.SpherePostion, 0, len(c.attendees))

		for _, att := range c.attendees {
			apos := att.pos
			apos[1] = -1
			visible := apos.Sub(pos).Len() < far+1
			if visible {
				spheres = append(spheres, opengl.SpherePostion{
					Pos: att.pos,
					Col: att.col,
				})
			}
		}

		c.renderer.RenderSpheres(c.camera, c.sphere, spheres)
	}

	c.renderer.ReadImage(c.renderedImage)

	c.visibleShapes = c.visibleShapes[:0]
	t1 := time.Now()

	swidth, sheight := c.screen.Size()
	sdim := image.Rect(0, 0, 4*swidth, 8*sheight)

	if c.canvas == nil || !c.canvas.Bounds().Eq(sdim) {
		c.canvas = image.NewRGBA(sdim)
	}

	rez.Convert(c.canvas, c.renderedImage, rez.NewBilinearFilter())

	gfx.BlitRunes(c.screen, c.canvas, false)

	c.drawHUD()

	c.screen.Show()
	t2 := time.Now()
	c.frameDuration = t1.Sub(t0)
	c.termDuration = t2.Sub(t1)
}
