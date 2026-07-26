package gfx

import (
	"image"
	"testing"
)

// testImage builds a deterministic gradient so the converter sees a
// spread of colours instead of a flat surface.
func testImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			o := img.PixOffset(x, y)
			img.Pix[o+0] = byte(x)
			img.Pix[o+1] = byte(y)
			img.Pix[o+2] = byte(x ^ y)
			img.Pix[o+3] = 0xff
		}
	}
	return img
}

func TestExtractCoversScreen(t *testing.T) {
	const columns, rows = 160, 80

	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"exact", columns * 4, rows * 8},
		{"interpolated", columns*4 - 17, rows*8 - 13},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := testImage(tc.width, tc.height)
			rc := NewRuneConverter(tc.width, tc.height, columns, rows, false)

			for y := 0; y < rows; y++ {
				for x := 0; x < columns; x++ {
					rc.Extract(img.Pix, x, y)
					if rc.CodePoint == 0 {
						t.Fatalf("no code point at (%d, %d)", x, y)
					}
				}
			}
		})
	}
}

func BenchmarkExtract(b *testing.B) {
	const columns, rows = 160, 80

	img := testImage(columns*4, rows*8)
	rc := NewRuneConverter(columns*4, rows*8, columns, rows, false)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for y := 0; y < rows; y++ {
			for x := 0; x < columns; x++ {
				rc.Extract(img.Pix, x, y)
			}
		}
	}
}
