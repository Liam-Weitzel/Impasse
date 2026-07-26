package main

import (
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

// loadAtlas uses the tileset at path, falling back to the theme's generated one
// when no path is given. A path that fails to load is an error rather than a
// silent fallback, because a typo would otherwise look like the art simply not
// arriving.
func loadAtlas(path string, t *theme) (*render.Atlas, error) {
	if path == "" {
		return buildAtlas(t)
	}
	return render.LoadAtlas(path, atlasCols, atlasRows)
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
