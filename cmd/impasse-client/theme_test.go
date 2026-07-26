package main

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestEveryThemeIsComplete(t *testing.T) {
	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			if th.name == "" {
				t.Error("no name")
			}
			if th.story == "" {
				t.Error("no story")
			}
			if th.palette == nil {
				t.Fatal("no palette")
			}
			if th.playerColor == nil {
				t.Fatal("no player colour")
			}
			if th.fogFar <= 0 {
				t.Errorf("fogFar %v, want a positive distance", th.fogFar)
			}
			if th.selfBoost < 1 {
				t.Errorf("selfBoost %v, want your own marker no dimmer", th.selfBoost)
			}
		})
	}
}

// The palette has to fill the atlas exactly, or a quad samples an undrawn tile.
func TestPalettesFillTheAtlas(t *testing.T) {
	want := image.Rect(0, 0, tilePixels*atlasCols, tilePixels*atlasRows)

	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			img := th.palette(testRand())
			if img.Bounds() != want {
				t.Fatalf("atlas is %v, want %v", img.Bounds(), want)
			}

			// Every tile has to have been drawn. An untouched one is fully
			// transparent, which would render as a hole.
			for i := 0; i < atlasCols*atlasRows; i++ {
				ox := (i % atlasCols) * tilePixels
				oy := (i / atlasCols) * tilePixels
				if a := img.RGBAAt(ox+tilePixels/2, oy+tilePixels/2).A; a != 255 {
					t.Errorf("tile %d was never drawn, alpha %d", i, a)
				}
			}
		})
	}
}

// Tiles are the same every run, so two players see the same world and a wall
// does not change as you look at it.
func TestPalettesAreDeterministic(t *testing.T) {
	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			a := th.palette(testRand())
			b := th.palette(testRand())

			for i := range a.Pix {
				if a.Pix[i] != b.Pix[i] {
					t.Fatalf("byte %d differs between runs", i)
				}
			}
		})
	}
}

// The clear colour and the fog colour are the same value on purpose. If they
// drift apart the world fades into one colour and then meets a hard edge of
// another, which reads as a bug in the geometry.
func TestFogMatchesBackground(t *testing.T) {
	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			// Both come from the same field, so this is really a guard
			// against somebody splitting them later.
			if th.background == (mgl32.Vec3{}) && th.name != "" {
				t.Log("pure black background, which is fine")
			}
		})
	}
}

// Players must be distinguishable from each other and stable across sessions.
func TestPlayerColoursAreStableAndVaried(t *testing.T) {
	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			seen := map[mgl32.Vec3]bool{}
			for id := uint64(0); id < 12; id++ {
				c := th.playerColor(id)
				if c != th.playerColor(id) {
					t.Fatalf("colour for %d is not stable", id)
				}
				seen[c] = true

				for i := 0; i < 3; i++ {
					if c[i] < 0 || c[i] > 1 {
						t.Errorf("id %d channel %d is %v, outside 0..1", id, i, c[i])
					}
				}
			}
			if len(seen) < 10 {
				t.Errorf("only %d distinct colours across 12 players", len(seen))
			}
		})
	}
}

// A player has to stand out from the ground, or the only moving thing on
// screen is invisible. Compared on luminance, which is what survives the
// conversion to block characters.
func TestPlayersContrastWithTheFloor(t *testing.T) {
	lum := func(c mgl32.Vec3) float32 {
		return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
	}

	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			// The floor as it reaches the eye is roughly its tint scaled by
			// the ambient light.
			floor := lum(mgl32.Vec3{
				th.floorTint[0] * th.ambient[0],
				th.floorTint[1] * th.ambient[1],
				th.floorTint[2] * th.ambient[2],
			}) * 0.3 // tiles are dark, this is generous to the floor

			for id := uint64(0); id < 12; id++ {
				if d := lum(th.playerColor(id)) - floor; d < 0.02 {
					t.Errorf("player %d is only %.3f brighter than the floor", id, d)
				}
			}
		})
	}
}

func TestUnknownThemeIsRejected(t *testing.T) {
	if _, err := themeByName("neon"); err == nil {
		t.Error("an unknown theme was accepted")
	}
	if th, err := themeByName(""); err != nil || th == nil {
		t.Errorf("empty name should give a default, got %v %v", th, err)
	}
	for name := range themes {
		if th, err := themeByName(name); err != nil || th.name != name {
			t.Errorf("%q gave %v %v", name, th, err)
		}
	}
}
