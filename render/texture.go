package render

import (
	"fmt"
	"image"
	"image/draw"
	"os"

	gl "github.com/go-gl/gl/v3.1/gles2"

	_ "image/png"
)

// A tile atlas: one texture holding a grid of square tiles, indexed left to
// right then top to bottom. One texture means one bind for the whole world, so
// the mesh can be drawn without sorting by material.
//
// Tiles are small on purpose. The renderer converts a 4x8 pixel patch of the
// framebuffer into a single terminal character, so a game cell is only a few
// dozen pixels across even zoomed in. Detail finer than that is thrown away
// before anyone sees it.
type Atlas struct {
	texture uint32
	// tiles across and down.
	cols int
	rows int
}

// Tile is an index into the atlas.
type Tile int

func (a *Atlas) Delete() {
	if a == nil {
		return
	}
	gl.DeleteTextures(1, &a.texture)
}

// Count is how many tiles the atlas holds.
func (a *Atlas) Count() int {
	if a == nil {
		return 0
	}
	return a.cols * a.rows
}

// UV returns the texture coordinates of a tile's corners, in the same order
// AddQuad wants them: bottom left, bottom right, top right, top left.
//
// The rectangle is inset by half a texel so that filtering cannot reach into
// the neighbouring tile, which shows up as a bright seam along every edge.
func (a *Atlas) UV(t Tile) [4][2]float32 {
	if a == nil || a.Count() == 0 {
		return [4][2]float32{}
	}

	i := int(t) % a.Count()
	if i < 0 {
		i += a.Count()
	}
	col := i % a.cols
	row := i / a.cols

	w := 1 / float32(a.cols)
	h := 1 / float32(a.rows)

	// Half a texel, assuming tiles are at least 8 pixels.
	const inset = 0.002
	u0 := float32(col)*w + inset*w
	u1 := float32(col+1)*w - inset*w
	v0 := float32(row)*h + inset*h
	v1 := float32(row+1)*h - inset*h

	return [4][2]float32{
		{u0, v1},
		{u1, v1},
		{u1, v0},
		{u0, v0},
	}
}

// NewAtlas uploads an image as a tile atlas. cols and rows say how it is cut up.
func NewAtlas(img image.Image, cols, rows int) (*Atlas, error) {
	if cols < 1 || rows < 1 {
		return nil, fmt.Errorf("atlas needs at least one tile, got %dx%d", cols, rows)
	}

	rgba := toRGBA(img)

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)

	// Clamped, because tiles are addressed by sub rectangle and wrapping
	// would sample the far side of the atlas.
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	// No mipmaps. Tiles are tiny and mipmapping blends neighbouring tiles
	// together at distance, which is exactly the seam problem again.
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	b := rgba.Bounds()
	gl.TexImage2D(
		gl.TEXTURE_2D, 0, gl.RGBA,
		int32(b.Dx()), int32(b.Dy()),
		0, gl.RGBA, gl.UNSIGNED_BYTE,
		gl.Ptr(rgba.Pix))

	if errNo := gl.GetError(); errNo != gl.NO_ERROR {
		gl.DeleteTextures(1, &texture)
		return nil, fmt.Errorf("uploading atlas failed: %d", errNo)
	}

	return &Atlas{texture: texture, cols: cols, rows: rows}, nil
}

// LoadAtlas reads a PNG tile atlas from disk.
func LoadAtlas(path string, cols, rows int) (*Atlas, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}

	return NewAtlas(img, cols, rows)
}

// toRGBA converts to RGBA and flips vertically, because GL texture rows start
// at the bottom while image rows start at the top.
func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()

	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)

	h := out.Bounds().Dy()
	for y, y2 := 0, h-1; y < y2; y, y2 = y+1, y2-1 {
		top := out.Pix[y*out.Stride : (y+1)*out.Stride]
		bot := out.Pix[y2*out.Stride : (y2+1)*out.Stride]
		for i := range top {
			top[i], bot[i] = bot[i], top[i]
		}
	}

	return out
}
