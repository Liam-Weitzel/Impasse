package opengl

import (
	"errors"
	"log"

	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
)

type CompiledShape struct {
	texture uint32
}

type ShapeCompiler struct {
	textureCache *TextureCache
}

func NewShapeCompiler(tc *TextureCache) *ShapeCompiler {
	return &ShapeCompiler{
		textureCache: tc,
	}
}

func (sc *ShapeCompiler) Compile(s *x3d.Shape) (*CompiledShape, error) {

	ifs := s.Geometry

	if !ifs.Convex {
		return nil, errors.New("non-convex geometries are not supported")
	}

	t, err := sc.textureCache.GetTexture(s.Appearance.Texture.Source)
	if err != nil {
		return nil, err
	}

	// actually indices of indices
	var indices []int32

	split := func(fn func([]int32)) {
		send := func() {
			if l := len(indices); l > 0 {
				if ifs.CoordIndices[indices[0]] == ifs.CoordIndices[l-1] {
					indices = indices[:l-1]
				}
				fn(indices)
				indices = indices[:0]
			}
		}
		for i, idx := range ifs.CoordIndices {
			if idx < 0 {
				send()
			} else {
				indices = append(indices, int32(i))
			}
		}
		send()
	}

	log.Println("New geometry:")

	//gl.MultiDrawArrays

	split(func(ids []int32) {
		log.Printf("\tface: %d\n", len(ids))

		for i, id := range ids {
			var v Vertex
			v.coords = ifs.Coordinates[ifs.CoordIndices[id%int32(len(ifs.CoordIndices))]]
			v.tex = ifs.TextureCoordinates[ifs.TexCoordIndices[id%int32(len(ifs.TexCoordIndices))]]
			if ifs.NormalPerVertex {
				//v.normals = ifs.Nor
			}

			_ = i
		}
	})

	// TODO: Implement me!
	_ = t

	cs := &CompiledShape{
		texture: t.Texture,
	}

	return cs, nil
}
