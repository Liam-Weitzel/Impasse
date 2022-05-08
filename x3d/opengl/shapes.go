package opengl

import (
	"errors"
	"math"
	"unsafe"

	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
)

type CompiledShape struct {
	texture      uint32
	vbo          uint32
	ibo          uint32
	diffuseColor mgl32.Vec3
	nIndices     int32
	ccw          bool
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

	split := func(fn func([]int32)) {
		// actually indices of indices
		var indices []int32
		send := func() {
			if l := len(indices); l > 0 {
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

	var (
		vertices []Vertex
		indices  []uint16
		v2i      = map[Vertex]uint16{}
	)

	split(func(ids []int32) {
		//log.Printf("\tface: %d\n", len(ids))
		if len(indices) > 0 {
			indices = append(indices, math.MaxUint16)
		}

		for _, id := range ids {
			var v Vertex
			v.coord = ifs.Coordinates[ifs.CoordIndices[id%int32(len(ifs.CoordIndices))]]
			//log.Printf("%v\n", v.coord)

			v.texCoord = ifs.TextureCoordinates[ifs.TexCoordIndices[id%int32(len(ifs.TexCoordIndices))]]

			if ifs.NormalPerVertex {
				v.normal = ifs.Normals[ifs.NormalIndices[id%int32(len(ifs.NormalIndices))]]
			} else {
				v.normal = ifs.Normals[ifs.NormalIndices[0]]
			}

			idx, ok := v2i[v]
			if !ok {
				idx = uint16(len(vertices))
				v2i[v] = idx
				vertices = append(vertices, v)
			}
			indices = append(indices, idx)
		}
	})

	ibo, err := createIBO(indices)
	if err != nil {
		t.Free()
		return nil, err
	}

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo)

	vbo, err := createVBO(vertices)
	if err != nil {
		gl.DeleteBuffers(1, &ibo)
		t.Free()
		return nil, err
	}

	layoutVBO(vbo)

	cs := &CompiledShape{
		texture:      t.Texture,
		vbo:          vbo,
		ibo:          ibo,
		diffuseColor: s.Appearance.DiffuseColor,
		nIndices:     int32(len(indices)),
		ccw:          s.Geometry.CCW,
	}

	return cs, nil
}

func (cs *CompiledShape) Render(s *State) {
	s.cullCCW(cs.ccw)
	bindVBO(cs.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, cs.ibo)
	s.bindTexture(cs.texture)
	gl.DrawElements(gl.TRIANGLE_FAN, cs.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))
}
