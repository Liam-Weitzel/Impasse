package main

import (
	"image"
	"image/color"
	"math/rand"

	"github.com/Liam-Weitzel/Impasse/render"
)

// Tiles for the world surfaces.
//
// The renderer turns a 4x8 pixel patch of the framebuffer into one terminal
// character, and a game cell is only a few dozen pixels across even zoomed in.
// So the useful resolution is tiny, and what survives is value contrast and
// large shapes. Fine detail is averaged away before anyone sees it, which is
// why these are 16x16 and why the variation is coarse.
const (
	tilePixels = 16
	atlasCols  = 4
	atlasRows  = 2
)

// Tile slots in the atlas. The order is the atlas layout, left to right then
// top to bottom.
const (
	tileFloorA render.Tile = iota
	tileFloorB
	tileFloorC
	tileObjectiveMark
	tileWallSideA
	tileWallSideB
	tileWallTopA
	tileWallTopB
)

// floorTileFor picks a floor variant from the cell position, so the ground is
// not one flat colour but does not shimmer when the camera moves either.
// Deterministic, so every client draws the same map the same way.
func floorTileFor(x, y int) render.Tile {
	switch hash2(x, y) % 3 {
	case 0:
		return tileFloorA
	case 1:
		return tileFloorB
	default:
		return tileFloorC
	}
}

func wallSideTileFor(x, y int) render.Tile {
	if hash2(x, y)%2 == 0 {
		return tileWallSideA
	}
	return tileWallSideB
}

func wallTopTileFor(x, y int) render.Tile {
	if hash2(x+7, y+13)%2 == 0 {
		return tileWallTopA
	}
	return tileWallTopB
}

// hash2 mixes two coordinates into a stable pseudo random number.
func hash2(x, y int) uint32 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

// buildDefaultAtlas draws a tileset in code, so the game has something to look
// at before any art exists and so a missing file is never fatal.
//
// It is deliberately plain: flat bases with light noise and an edge shade. The
// art direction is not decided, and inventing one here would be pretending it
// is.
func buildDefaultAtlas() (*render.Atlas, error) {
	img := image.NewRGBA(image.Rect(0, 0, tilePixels*atlasCols, tilePixels*atlasRows))

	// Fixed seed, so the tiles are the same every run and every client.
	rnd := rand.New(rand.NewSource(20260726))

	type spec struct {
		tile  render.Tile
		base  color.RGBA
		noise int
		// edge darkens the tile border, which is what actually reads as
		// texture once a cell is only a few characters wide.
		edge float64
	}

	for _, s := range []spec{
		{tileFloorA, color.RGBA{92, 92, 104, 255}, 10, 0.10},
		{tileFloorB, color.RGBA{84, 86, 98, 255}, 12, 0.10},
		{tileFloorC, color.RGBA{98, 96, 106, 255}, 8, 0.10},
		{tileObjectiveMark, color.RGBA{120, 108, 60, 255}, 14, 0.25},
		{tileWallSideA, color.RGBA{140, 116, 88, 255}, 14, 0.28},
		{tileWallSideB, color.RGBA{130, 108, 82, 255}, 16, 0.28},
		{tileWallTopA, color.RGBA{112, 94, 72, 255}, 10, 0.20},
		{tileWallTopB, color.RGBA{104, 88, 68, 255}, 12, 0.20},
	} {
		drawTile(img, s.tile, s.base, s.noise, s.edge, rnd)
	}

	return render.NewAtlas(img, atlasCols, atlasRows)
}

func drawTile(img *image.RGBA, t render.Tile, base color.RGBA, noise int, edge float64, rnd *rand.Rand) {
	col := int(t) % atlasCols
	row := int(t) / atlasCols
	ox, oy := col*tilePixels, row*tilePixels

	for y := 0; y < tilePixels; y++ {
		for x := 0; x < tilePixels; x++ {
			shade := 1.0

			// Darken towards the tile border so cells read as separate
			// squares rather than one continuous surface.
			d := min(min(x, tilePixels-1-x), min(y, tilePixels-1-y))
			if d == 0 {
				shade -= edge
			} else if d == 1 {
				shade -= edge * 0.4
			}

			n := rnd.Intn(noise*2+1) - noise

			img.SetRGBA(ox+x, oy+y, color.RGBA{
				R: clampByte(float64(base.R)*shade + float64(n)),
				G: clampByte(float64(base.G)*shade + float64(n)),
				B: clampByte(float64(base.B)*shade + float64(n)),
				A: 255,
			})
		}
	}
}

func clampByte(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return uint8(v)
	}
}

// loadAtlas uses the tileset at path, falling back to the generated one when no
// path is given. A path that fails to load is an error rather than a silent
// fallback, because a typo would otherwise look like the art simply not
// arriving.
func loadAtlas(path string) (*render.Atlas, error) {
	if path == "" {
		return buildDefaultAtlas()
	}
	return render.LoadAtlas(path, atlasCols, atlasRows)
}
