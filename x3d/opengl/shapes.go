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
}

type ShapeCompiler struct {
	textureCache *TextureCache
}

func NewShapeCompiler(tc *TextureCache) *ShapeCompiler {
	return &ShapeCompiler{
		textureCache: tc,
	}
}

func reverse(a []int32) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
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
				if ifs.CoordIndices[indices[0]] == ifs.CoordIndices[l-1] {
					indices = indices[:l-1]
				}
				if !ifs.CCW {
					reverse(indices)
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

			// TODO: Normalize to texture dimensions.
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

	// fix texCoords
	if needFixing(ifs.TextureCoordinates) {
		for i := range vertices {
			fixTexCoord(&vertices[i].texCoord, t)
		}
	}

	vbo, err := createVBO(vertices)
	if err != nil {
		t.Free()
		return nil, err
	}

	layoutVBO(vbo)

	ibo, err := createIBO(indices)
	if err != nil {
		t.Free()
		gl.DeleteBuffers(1, &vbo)
		return nil, err
	}

	cs := &CompiledShape{
		texture:      t.Texture,
		vbo:          vbo,
		ibo:          ibo,
		diffuseColor: s.Appearance.DiffuseColor,
		nIndices:     int32(len(indices)),
	}

	return cs, nil
}

func needFixing(uvs []mgl32.Vec2) bool {
	for _, uv := range uvs {
		if uv[0] < 0 || uv[0] > 1 || uv[1] < 0 || uv[1] > 1 {
			return true
		}
	}
	return false
}

func fixTexCoord(uv *mgl32.Vec2, t *Texture) {
	for uv[0] < 0 {
		uv[0] += float32(t.Width)
	}
	for uv[1] < 0 {
		uv[1] += float32(t.Height)
	}
	uv[0] /= float32(t.Width)
	uv[1] /= float32(t.Height)
}

func (cs *CompiledShape) Render(s *State) {

	gl.BindTexture(gl.TEXTURE_2D, cs.texture)
	gl.BindBuffer(gl.ARRAY_BUFFER, cs.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, cs.ibo)

	// Bind the uniforms
	//gl.Uniform1i(s.texSamplerUniformLoc, 0)
	gl.Uniform3fv(s.diffuseColLoc, 1, &cs.diffuseColor[0])

	gl.DrawElements(gl.TRIANGLE_FAN, cs.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))
}
