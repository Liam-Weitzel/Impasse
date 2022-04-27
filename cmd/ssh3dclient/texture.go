package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"

	gl "github.com/go-gl/gl/v3.0/gles2"
)

func rgbaFromBytes(src []byte) (*image.RGBA, error) {

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}

	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, img.Bounds(), img, image.ZP, draw.Src)
	return rgba, nil
}

func loadTextureFromRGBA(img *image.RGBA) (uint32, error) {
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	width, height := img.Bounds().Dx(), img.Bounds().Dy()

	gl.TexImage2D(
		gl.TEXTURE_2D, 0,
		gl.RGBA,
		int32(width), int32(height),
		0, gl.RGBA, gl.UNSIGNED_BYTE,
		gl.Ptr(img.Pix))

	if errNo := gl.GetError(); errNo != gl.NO_ERROR {
		gl.DeleteBuffers(1, &texture)
		return 0, fmt.Errorf("creating texture failed: %d", errNo)
	}

	return texture, nil
}
