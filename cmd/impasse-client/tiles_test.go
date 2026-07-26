package main

import (
	"testing"

	"github.com/Liam-Weitzel/Impasse/render"
)

// Variation has to be stable. If it were random per run, two players would see
// different maps and the same wall would shimmer as the camera moved.
func TestTileChoiceIsDeterministic(t *testing.T) {
	for i := 0; i < 50; i++ {
		x, y := i*3, i*7
		if floorTileFor(x, y) != floorTileFor(x, y) {
			t.Fatalf("floor tile at (%d,%d) is not stable", x, y)
		}
		if wallSideTileFor(x, y) != wallSideTileFor(x, y) {
			t.Fatalf("wall tile at (%d,%d) is not stable", x, y)
		}
	}
}

// It also has to actually vary, or the tiles are pointless.
func TestTileChoiceVaries(t *testing.T) {
	seen := map[render.Tile]bool{}
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			seen[floorTileFor(x, y)] = true
		}
	}
	if len(seen) < 2 {
		t.Errorf("floors only ever use %d tile(s), want variation", len(seen))
	}

	seen = map[render.Tile]bool{}
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			seen[wallSideTileFor(x, y)] = true
		}
	}
	if len(seen) < 2 {
		t.Errorf("walls only ever use %d tile(s), want variation", len(seen))
	}
}

// Every tile slot must land inside the atlas, or a quad samples somebody else's
// tile.
func TestTileIndicesAreInsideTheAtlas(t *testing.T) {
	count := atlasCols * atlasRows

	for _, tile := range []render.Tile{
		tileFloorA, tileFloorB, tileFloorC, tileObjectiveMark,
		tileWallSideA, tileWallSideB, tileWallTopA, tileWallTopB,
	} {
		if int(tile) < 0 || int(tile) >= count {
			t.Errorf("tile %d is outside an atlas of %d", tile, count)
		}
	}
}

func TestHashSpreads(t *testing.T) {
	counts := map[uint32]int{}
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			counts[hash2(x, y)%3]++
		}
	}
	if len(counts) != 3 {
		t.Fatalf("hash only produced %d buckets of 3", len(counts))
	}
	// Nothing rigorous, just that no bucket is nearly empty.
	for bucket, n := range counts {
		if n < 1600/6 {
			t.Errorf("bucket %d got %d of 1600, badly skewed", bucket, n)
		}
	}
}
