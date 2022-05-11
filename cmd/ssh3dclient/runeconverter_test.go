package main

import (
	_ "embed"
	"image"
	"testing"

	"github.com/bamiaux/rez"
)

//go:embed texture.png
var demo []byte

func BenchmarkExtract(b *testing.B) {

	b.StopTimer()

	img, err := rgbaFromBytes(demo)
	if err != nil {
		b.Fatalf("bad image: %v", err)
	}
	screen := image.NewRGBA(image.Rect(0, 0, 160*4, 80*8))
	rez.Convert(screen, img, rez.NewBilinearFilter())

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		rc := RuneConverter{}
		width, height := screen.Rect.Dx(), screen.Rect.Dy()

		i := 0
		for y := 0; y < height; y, i = y+8, i+1 {
			j := 0
			for x := 0; x < width; x, j = x+4, j+1 {
				rc.Extract(screen, x, y)
			}
		}
	}
}
