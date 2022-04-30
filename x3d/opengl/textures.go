package opengl

import (
	"image"
	"os"
	"path/filepath"

	"image/draw"
	_ "image/jpeg"
	_ "image/png"
)

type Texture struct {
	refCount int
}

func (t *Texture) free() {
	// TODO: Implement me!
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
		return rgba, err
	}

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, img.Bounds(), img, image.ZP, draw.Src)
	return nil, err
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

		// TODO: Implement me!
		_ = rgba

		t = &Texture{refCount: 1}

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
