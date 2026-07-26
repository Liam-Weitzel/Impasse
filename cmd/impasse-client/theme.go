package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"sort"

	"github.com/Liam-Weitzel/Impasse/gfx"
	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// A theme is the whole look in one place: what the void is, what the ground and
// walls are made of, and what colour every piece of feedback is.
//
// It has to be one object rather than scattered constants because the pieces
// only work together. The clear colour has to match the fog colour or there is
// a visible horizon where the world ends. Pickups have to be the most saturated
// thing on screen or they vanish into the floor. Getting one right and another
// wrong looks worse than either done badly on its own.
//
// Everything here is chosen for a renderer that reduces a 4x8 pixel patch to a
// single character. Value contrast survives that, fine detail does not.
type theme struct {
	name  string
	story string

	// background is both the clear colour and the fog colour. One value, so
	// distance fades into the void instead of into a seam.
	background mgl32.Vec3
	// fogFar is how far away the world dissolves, in world units.
	fogFar float32
	// ambient is the base light everything gets.
	ambient mgl32.Vec3

	// Surface tints. Tiles carry detail, these carry the lighting response.
	floorTint mgl32.Vec3
	wallTint  mgl32.Vec3

	// Actors and feedback.
	objective mgl32.Vec3
	arrow     mgl32.Vec3
	telegraph mgl32.Vec3
	stunned   mgl32.Vec3
	// self brightens your own marker so you can find yourself in a crowd.
	selfBoost float32

	// playerColor gives each player a stable colour from their id. It is per
	// theme because a randomly hued player wrecks a monochrome look: one
	// blue marker in The Green reads as a bug, not a rival.
	playerColor func(id uint64) mgl32.Vec3

	// palette generates the tile atlas.
	palette func(*rand.Rand) *image.RGBA
}

var themes = map[string]*theme{
	"matrix": matrixTheme,
	"gritty": grittyTheme,
}

func themeNames() []string {
	out := make([]string, 0, len(themes))
	for name := range themes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func themeByName(name string) (*theme, error) {
	if name == "" {
		return themes["gritty"], nil
	}
	t, ok := themes[name]
	if !ok {
		return nil, fmt.Errorf("unknown theme %q, have %v", name, themeNames())
	}
	return t, nil
}

// The Green, where the machine shows its work.
//
// Impasse is a game about writing programs, so the first look treats the maze
// as what it literally is: a data structure being walked. The world is a
// terminal inside a terminal. Floors are the faint grid of an empty allocation,
// walls are lit traces carrying something, and the pickups are values still
// unread. You are a process. So is everything else in here, and the ones that
// are not yours are racing you to the same memory.
//
// The stun is a scheduler interrupt. It does not damage anything, it just takes
// your turn away, and you can see it coming for exactly one tick, which is
// enough time to understand what is about to happen and not enough to stop it.
var matrixTheme = &theme{
	name: "matrix",
	story: "The Green. The maze is a data structure and you are a process " +
		"walking it. The pickups are values nobody has read yet, and the " +
		"other processes want them first. A stun is an interrupt: it takes " +
		"your turn, not your life.",

	background: mgl32.Vec3{0.01, 0.03, 0.02},
	fogFar:     900,
	ambient:    mgl32.Vec3{0.55, 0.85, 0.60},

	floorTint: mgl32.Vec3{0.55, 1.00, 0.65},
	wallTint:  mgl32.Vec3{0.70, 1.00, 0.75},

	objective: mgl32.Vec3{0.75, 1.00, 0.80},
	arrow:     mgl32.Vec3{0.45, 1.00, 0.55},
	telegraph: mgl32.Vec3{0.90, 1.00, 0.90},
	stunned:   mgl32.Vec3{0.10, 0.25, 0.14},
	selfBoost: 1.5,

	// One hue, separated by brightness. Everything alive is the same green,
	// which is the point: you are all the same kind of thing.
	playerColor: hueRange(120, 150, 0.55, 1.00),

	palette: matrixTiles,
}

// The Substrate, where the machine has been left running too long.
//
// Same maze, opposite reading. This is not a clean abstraction, it is a real
// place that has been executing the same loop for longer than anyone
// remembers, and it shows. The floors are worn where processes have walked
// them. The walls are oxidising. The pickups are the last few values still
// worth anything, and they glow because somebody put a lot of work into them a
// long time ago.
//
// The stun here is not an interrupt, it is a fault. Something in you stops
// responding for two ticks and you watch it happen.
var grittyTheme = &theme{
	name: "gritty",
	story: "The Substrate. The same maze, but it has been running far too " +
		"long and nobody has swept it. Floors are worn where other " +
		"processes walked them, walls are oxidising, and the last few " +
		"values worth anything still glow. A stun is a fault: you stop " +
		"responding for two ticks and you get to watch.",

	background: mgl32.Vec3{0.03, 0.025, 0.02},
	fogFar:     760,
	ambient:    mgl32.Vec3{0.62, 0.56, 0.48},

	floorTint: mgl32.Vec3{0.80, 0.78, 0.76},
	wallTint:  mgl32.Vec3{0.95, 0.86, 0.72},

	objective: mgl32.Vec3{1.00, 0.78, 0.22},
	arrow:     mgl32.Vec3{1.00, 0.70, 0.20},
	telegraph: mgl32.Vec3{0.95, 0.22, 0.14},
	stunned:   mgl32.Vec3{0.22, 0.20, 0.20},
	selfBoost: 1.45,

	// Warm signal colours against cold worn ground, so a player is the only
	// thing on screen that looks powered.
	playerColor: hueRange(10, 60, 0.60, 0.95),

	palette: grittyTiles,
}

// hueRange builds a colour picker confined to a slice of the wheel, so every
// player is distinguishable without any of them being off palette.
//
// The id seeds it, so a player looks the same to everyone and the same across
// sessions.
func hueRange(hueLo, hueHi, lightLo, lightHi float64) func(uint64) mgl32.Vec3 {
	return func(id uint64) mgl32.Vec3 {
		rnd := rand.New(rand.NewSource(int64(id) * 2654435761))

		hue := hueLo + rnd.Float64()*(hueHi-hueLo)
		light := lightLo + rnd.Float64()*(lightHi-lightLo)

		// gfx wants degrees and percentages.
		r, g, b := gfx.HSLToRGB(hue, 75, light*55)
		return mgl32.Vec3{float32(r), float32(g), float32(b)}
	}
}

// tileSpec is one tile in the atlas.
type tileSpec struct {
	tile  render.Tile
	base  color.RGBA
	noise int
	// edge darkens the border, which is what actually reads as texture once
	// a cell is only a few characters wide.
	edge float64
	// draw adds whatever makes this theme look like itself, on top of the
	// base and before the edge shading.
	draw func(img *image.RGBA, ox, oy int, rnd *rand.Rand)
}

func renderTiles(specs []tileSpec, rnd *rand.Rand) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, tilePixels*atlasCols, tilePixels*atlasRows))

	for _, s := range specs {
		col := int(s.tile) % atlasCols
		row := int(s.tile) / atlasCols
		ox, oy := col*tilePixels, row*tilePixels

		for y := 0; y < tilePixels; y++ {
			for x := 0; x < tilePixels; x++ {
				n := 0
				if s.noise > 0 {
					n = rnd.Intn(s.noise*2+1) - s.noise
				}
				img.SetRGBA(ox+x, oy+y, color.RGBA{
					R: clampByte(float64(s.base.R) + float64(n)),
					G: clampByte(float64(s.base.G) + float64(n)),
					B: clampByte(float64(s.base.B) + float64(n)),
					A: 255,
				})
			}
		}

		if s.draw != nil {
			s.draw(img, ox, oy, rnd)
		}

		shadeEdges(img, ox, oy, s.edge)
	}

	return img
}

// shadeEdges darkens the tile border so cells read as separate squares rather
// than one continuous surface. At this resolution the border is most of what
// the eye actually gets.
func shadeEdges(img *image.RGBA, ox, oy int, edge float64) {
	if edge <= 0 {
		return
	}
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

// matrixTiles: near black surfaces with lit structure drawn on them. Floors get
// a faint lattice, walls get vertical traces like a bus, and everything is one
// hue so the only colour on screen belongs to the players.
func matrixTiles(rnd *rand.Rand) *image.RGBA {
	lattice := func(step int, bright uint8) func(*image.RGBA, int, int, *rand.Rand) {
		return func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
			for y := 0; y < tilePixels; y++ {
				for x := 0; x < tilePixels; x++ {
					if x%step == 0 || y%step == 0 {
						c := img.RGBAAt(ox+x, oy+y)
						img.SetRGBA(ox+x, oy+y, color.RGBA{
							R: c.R / 2, G: bright, B: c.B, A: 255,
						})
					}
				}
			}
		}
	}

	traces := func(count int, bright uint8) func(*image.RGBA, int, int, *rand.Rand) {
		return func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
			for i := 0; i < count; i++ {
				x := rnd.Intn(tilePixels)
				// Traces run the full height and fade towards the top, so
				// walls read as carrying something downwards.
				for y := 0; y < tilePixels; y++ {
					f := 0.35 + 0.65*float64(y)/float64(tilePixels)
					img.SetRGBA(ox+x, oy+y, color.RGBA{
						R: 10, G: clampByte(float64(bright) * f), B: 24, A: 255,
					})
				}
			}
		}
	}

	// A ring, for the ground under a pickup. Reads as a socket the value was
	// pulled out of.
	socket := func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
		const c = (tilePixels - 1) / 2.0
		for y := 0; y < tilePixels; y++ {
			for x := 0; x < tilePixels; x++ {
				d := math.Hypot(float64(x)-c, float64(y)-c)
				if d > 4.0 && d < 6.0 {
					img.SetRGBA(ox+x, oy+y, color.RGBA{20, 200, 60, 255})
				}
			}
		}
	}

	return renderTiles([]tileSpec{
		{tileFloorA, color.RGBA{6, 18, 9, 255}, 3, 0.30, lattice(8, 70)},
		{tileFloorB, color.RGBA{5, 15, 8, 255}, 3, 0.30, lattice(4, 52)},
		{tileFloorC, color.RGBA{7, 20, 10, 255}, 4, 0.30, lattice(8, 90)},
		{tileObjectiveMark, color.RGBA{6, 22, 10, 255}, 3, 0.28, socket},
		{tileWallSideA, color.RGBA{8, 26, 12, 255}, 4, 0.40, traces(3, 190)},
		{tileWallSideB, color.RGBA{7, 22, 11, 255}, 4, 0.40, traces(2, 150)},
		{tileWallTopA, color.RGBA{10, 34, 15, 255}, 5, 0.34, lattice(4, 110)},
		{tileWallTopB, color.RGBA{9, 30, 13, 255}, 5, 0.34, lattice(8, 130)},
	}, rnd)
}

// grittyTiles: worn concrete and oxidised metal. The detail is blotchy rather
// than structured, because wear is not regular and because blotches survive
// downsampling better than lines do.
func grittyTiles(rnd *rand.Rand) *image.RGBA {
	blotches := func(count, radius int, shade float64) func(*image.RGBA, int, int, *rand.Rand) {
		return func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
			for i := 0; i < count; i++ {
				cx := rnd.Intn(tilePixels)
				cy := rnd.Intn(tilePixels)
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

	// Rust streaks running down a wall.
	streaks := func(count int) func(*image.RGBA, int, int, *rand.Rand) {
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

	// A scuffed plate where a pickup sat.
	plate := func(img *image.RGBA, ox, oy int, rnd *rand.Rand) {
		for y := 3; y < tilePixels-3; y++ {
			for x := 3; x < tilePixels-3; x++ {
				edge := x == 3 || y == 3 || x == tilePixels-4 || y == tilePixels-4
				c := img.RGBAAt(ox+x, oy+y)
				if edge {
					img.SetRGBA(ox+x, oy+y, color.RGBA{
						R: clampByte(float64(c.R)*0.6 + 60),
						G: clampByte(float64(c.G)*0.6 + 42),
						B: clampByte(float64(c.B) * 0.6),
						A: 255,
					})
				}
			}
		}
	}

	return renderTiles([]tileSpec{
		{tileFloorA, color.RGBA{74, 71, 66, 255}, 9, 0.16, blotches(6, 3, 0.82)},
		{tileFloorB, color.RGBA{68, 66, 62, 255}, 11, 0.16, blotches(9, 2, 0.86)},
		{tileFloorC, color.RGBA{80, 76, 70, 255}, 8, 0.16, blotches(4, 4, 0.78)},
		{tileObjectiveMark, color.RGBA{86, 78, 62, 255}, 8, 0.22, plate},
		{tileWallSideA, color.RGBA{104, 88, 70, 255}, 12, 0.34, streaks(4)},
		{tileWallSideB, color.RGBA{96, 82, 66, 255}, 13, 0.34, streaks(6)},
		{tileWallTopA, color.RGBA{86, 76, 64, 255}, 10, 0.26, blotches(7, 3, 0.80)},
		{tileWallTopB, color.RGBA{80, 70, 60, 255}, 11, 0.26, blotches(5, 4, 0.84)},
	}, rnd)
}

// buildAtlas draws the theme's tileset. The seed is fixed so every client gets
// identical tiles.
func buildAtlas(t *theme) (*render.Atlas, error) {
	rnd := rand.New(rand.NewSource(20260726))
	return render.NewAtlas(t.palette(rnd), atlasCols, atlasRows)
}
