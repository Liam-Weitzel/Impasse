package opengl

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	gl "github.com/go-gl/gl/v3.0/gles2"

	"image/draw"
	_ "image/jpeg"
	_ "image/png"
)

type Texture struct {
	refCount int32
	Width    int32
	Height   int32
	Texture  uint32
}

func (t *Texture) free() {
	//gl.BindTexture(gl.TEXTURE_2D, 0)
	gl.DeleteBuffers(1, &t.Texture)
}

func (t *Texture) Free() {
	if t.refCount > 0 {
		if t.refCount--; t.refCount == 0 {
			t.free()
		}
	}
}

func (t *Texture) Delete() {
	if t.refCount > 0 {
		t.refCount = 0
		t.free()
	}
}

type TextureCache struct {
	directory string
	textures  map[string]*Texture
}

func NewTextureCache(directory string) *TextureCache {
	return &TextureCache{
		directory: directory,
		textures:  map[string]*Texture{},
	}
}

func loadRGBAFromFile(fname string) (*image.RGBA, error) {
	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	if rgba, ok := img.(*image.RGBA); ok {
		yFlip(rgba)
		return rgba, err
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, img.Bounds(), img, image.ZP, draw.Src)
	yFlip(rgba)
	return rgba, err
}

func yFlip(img *image.RGBA) {
	h := img.Bounds().Dy()
	for i, j := 0, h-1; i < j; i, j = i+1, j-1 {
		si := img.PixOffset(0, i)
		sj := img.PixOffset(0, j)
		ri := img.Pix[si : si+img.Stride]
		rj := img.Pix[sj : sj+img.Stride]
		_ = rj[len(ri)-1]
		for k, vi := range ri {
			rj[k], ri[k] = vi, rj[k]
		}
	}
}

func (tc *TextureCache) NumTextures() int {
	return len(tc.textures)
}

func (tc *TextureCache) GetTexture(name string) (*Texture, error) {
	t := tc.textures[name]

	if t == nil || t.refCount < 1 {

		path := filepath.Join(tc.directory, name)

		rgba, err := loadRGBAFromFile(path)
		if err != nil {
			return nil, err
		}

		texture, err := makeTexture(rgba)
		if err != nil {
			return nil, err
		}

		t = &Texture{
			refCount: 1,
			Texture:  texture,
			Width:    int32(rgba.Bounds().Dx()),
			Height:   int32(rgba.Bounds().Dy()),
		}

		tc.textures[name] = t
	} else {
		t.refCount++
	}

	return t, nil
}

func (tc *TextureCache) Delete() {
	ts := tc.textures
	tc.textures = nil

	for _, t := range ts {
		t.Delete()
	}
}

func makeTexture(img *image.RGBA) (uint32, error) {
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	width, height := int32(img.Bounds().Dx()), int32(img.Bounds().Dy())

	gl.TexImage2D(
		gl.TEXTURE_2D, 0,
		gl.RGBA,
		width, height,
		0, gl.RGBA, gl.UNSIGNED_BYTE,
		gl.Ptr(img.Pix))
	gl.GenerateMipmap(gl.TEXTURE_2D)

	if errNo := gl.GetError(); errNo != gl.NO_ERROR {
		gl.DeleteBuffers(1, &texture)
		return 0, fmt.Errorf("creating texture failed: %d", errNo)
	}

	return texture, nil
}
