package main

import (
	"image"
	"image/color"
	"math"
	"math/rand"

	"github.com/Liam-Weitzel/Impasse/gfx"
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// The look of the world.
//
// Worn concrete and oxidising metal, lit by a head light, fading into the dark.
// Detail is blotchy rather than structured, because wear is not regular and
// because blotches survive downsampling better than lines do.
//
// All of it is chosen for a renderer that reduces a 4x8 pixel patch to a single
// character. A floor tile arrives as roughly two dozen colour samples, so value
// contrast and large shapes are the only things that carry. Everything
// saturated is spent on the few things you have to see: players, pickups, and
// the frame of an incoming burst.
var (
	// background is both the clear colour and the fog colour. One value, or
	// distance fades into one colour and then meets a hard edge of another.
	background = mgl32.Vec3{0.03, 0.025, 0.02}
	ambient    = mgl32.Vec3{0.62, 0.56, 0.48}

	// Surface tints. Tiles carry the detail, these carry the lighting.
	floorTint = mgl32.Vec3{0.80, 0.78, 0.76}
	wallTint  = mgl32.Vec3{0.95, 0.86, 0.72}

	objectiveColor = mgl32.Vec3{1.00, 0.78, 0.22}
	arrowColor     = mgl32.Vec3{1.00, 0.70, 0.20}
	telegraphBase  = mgl32.Vec3{0.95, 0.22, 0.14}
	stunnedColor   = mgl32.Vec3{0.22, 0.20, 0.20}
)

const (
	// fogFar is how far away the world dissolves, in world units.
	fogFar = 760
	// selfBoost brightens your own marker so you can find yourself.
	selfBoost = 1.45
)

// playerColor gives each player a stable colour from their id, confined to warm
// hues so a player is the only thing on screen that looks powered, and so no
// marker can land off palette and read as a bug rather than a rival.
func playerColor(id uint64) mgl32.Vec3 {
	rnd := rand.New(rand.NewSource(int64(id) * 2654435761))

	hue := 10 + rnd.Float64()*50
	light := 60 + rnd.Float64()*35

	r, g, b := gfx.HSLToRGB(hue, 75, light*0.55)
	return mgl32.Vec3{float32(r), float32(g), float32(b)}
}

// tileSpec is one tile in the atlas.
type tileSpec struct {
	tile  render.Tile
	base  color.RGBA
	noise int
	// edge darkens the border, which is what actually reads as texture once
	// a cell is only a few characters wide.
	edge float64
	// draw adds the wear, on top of the base and before the edge shading.
	draw func(img *image.RGBA, ox, oy int, rnd *rand.Rand)
}

// buildAtlas draws the tileset. The seed is fixed so every client gets
// identical tiles and a wall does not change as you look at it.
func buildAtlas() (*render.Atlas, error) {
	rnd := rand.New(rand.NewSource(20260726))

	specs := []tileSpec{
		{tileFloorA, color.RGBA{74, 71, 66, 255}, 9, 0.16, blotches(6, 3, 0.82)},
		{tileFloorB, color.RGBA{68, 66, 62, 255}, 11, 0.16, blotches(9, 2, 0.86)},
		{tileFloorC, color.RGBA{80, 76, 70, 255}, 8, 0.16, blotches(4, 4, 0.78)},
		{tileObjectiveMark, color.RGBA{86, 78, 62, 255}, 8, 0.22, plate},
		{tileWallSideA, color.RGBA{104, 88, 70, 255}, 12, 0.34, streaks(4)},
		{tileWallSideB, color.RGBA{96, 82, 66, 255}, 13, 0.34, streaks(6)},
		{tileWallTopA, color.RGBA{86, 76, 64, 255}, 10, 0.26, blotches(7, 3, 0.80)},
		{tileWallTopB, color.RGBA{80, 70, 60, 255}, 11, 0.26, blotches(5, 4, 0.84)},
	}

	img := image.NewRGBA(image.Rect(0, 0, tilePixels*atlasCols, tilePixels*atlasRows))

	for _, s := range specs {
		col := int(s.tile) % atlasCols
		row := int(s.tile) / atlasCols
		ox, oy := col*tilePixels, row*tilePixels

		for y := 0; y < tilePixels; y++ {
			for x := 0; x < tilePixels; x++ {
				n := rnd.Intn(s.noise*2+1) - s.noise
				img.SetRGBA(ox+x, oy+y, color.RGBA{
					R: clampByte(float64(s.base.R) + float64(n)),
					G: clampByte(float64(s.base.G) + float64(n)),
					B: clampByte(float64(s.base.B) + float64(n)),
					A: 255,
				})
			}
		}

		s.draw(img, ox, oy, rnd)
		shadeEdges(img, ox, oy, s.edge)
	}

	return render.NewAtlas(img, atlasCols, atlasRows)
}

// shadeEdges darkens the tile border so cells read as separate squares rather
// than one continuous surface. At this resolution the border is most of what
// the eye actually gets.
func shadeEdges(img *image.RGBA, ox, oy int, edge float64) {
	for y := 0; y < tilePixels; y++ {
		for x := 0; x < tilePixels; x++ {
			d := min(min(x, tilePixels-1-x), min(y, tilePixels-1-y))
			var shade float64
			switch d {
			case 0:
				shade = edge
			case 1:
				shade = edge * 0.45
			default:
				continue
			}
			c := img.RGBAAt(ox+x, oy+y)
			img.SetRGBA(ox+x, oy+y, color.RGBA{
				R: clampByte(float64(c.R) * (1 - shade)),
				G: clampByte(float64(c.G) * (1 - shade)),
				B: clampByte(float64(c.B) * (1 - shade)),
				A: 255,
			})
		}
	}
}

// blotches are patches of wear.
func blotches(count, radius int, shade float64) func(*image.RGBA, int, int, *rand.Rand) {
	return func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
		for i := 0; i < count; i++ {
			cx, cy := rnd.Intn(tilePixels), rnd.Intn(tilePixels)
			r := 1 + rnd.Intn(radius)

			for y := cy - r; y <= cy+r; y++ {
				for x := cx - r; x <= cx+r; x++ {
					if x < 0 || y < 0 || x >= tilePixels || y >= tilePixels {
						continue
					}
					if math.Hypot(float64(x-cx), float64(y-cy)) > float64(r) {
						continue
					}
					c := img.RGBAAt(ox+x, oy+y)
					img.SetRGBA(ox+x, oy+y, color.RGBA{
						R: clampByte(float64(c.R) * shade),
						G: clampByte(float64(c.G) * shade * 0.97),
						B: clampByte(float64(c.B) * shade * 0.93),
						A: 255,
					})
				}
			}
		}
	}
}

// streaks are rust running down a wall.
func streaks(count int) func(*image.RGBA, int, int, *rand.Rand) {
	return func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
		for i := 0; i < count; i++ {
			x := rnd.Intn(tilePixels)
			start := rnd.Intn(tilePixels / 2)
			for y := start; y < tilePixels; y++ {
				f := float64(y-start) / float64(tilePixels-start)
				c := img.RGBAAt(ox+x, oy+y)
				img.SetRGBA(ox+x, oy+y, color.RGBA{
					R: clampByte(float64(c.R)*(1-0.25*f) + 40*f),
					G: clampByte(float64(c.G)*(1-0.45*f) + 14*f),
					B: clampByte(float64(c.B) * (1 - 0.60*f)),
					A: 255,
				})
			}
		}
	}
}

// plate is the scuffed marking left where a pickup sits, so a player can see
// where one was after it has gone.
func plate(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
	for y := 3; y < tilePixels-3; y++ {
		for x := 3; x < tilePixels-3; x++ {
			if x != 3 && y != 3 && x != tilePixels-4 && y != tilePixels-4 {
				continue
			}
			c := img.RGBAAt(ox+x, oy+y)
			img.SetRGBA(ox+x, oy+y, color.RGBA{
				R: clampByte(float64(c.R)*0.6 + 60),
				G: clampByte(float64(c.G)*0.6 + 42),
				B: clampByte(float64(c.B) * 0.6),
				A: 255,
			})
		}
	}
}
