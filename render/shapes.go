package render

import (
	"unsafe"

	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"
)

// Bounds is an axis aligned box around a shape, used to cull it.
type Bounds struct {
	Min mgl32.Vec3
	Max mgl32.Vec3
}

// Corners returns the eight points of the box, for testing against a frustum.
func (b Bounds) Corners() [8]mgl32.Vec3 {
	return [8]mgl32.Vec3{
		{b.Min[0], b.Min[1], b.Min[2]},
		{b.Max[0], b.Min[1], b.Min[2]},
		{b.Min[0], b.Max[1], b.Min[2]},
		{b.Max[0], b.Max[1], b.Min[2]},
		{b.Min[0], b.Min[1], b.Max[2]},
		{b.Max[0], b.Min[1], b.Max[2]},
		{b.Min[0], b.Max[1], b.Max[2]},
		{b.Max[0], b.Max[1], b.Max[2]},
	}
}

// CompiledShape is geometry uploaded to the GPU, ready to draw. Build them with
// a MeshBuilder.
type CompiledShape struct {
	// Bounds is what culling tests. It is only meaningful when the mesh was
	// split by region, which is what buildGridMesh does.
	Bounds Bounds

	vbo          uint32
	ibo          uint32
	diffuseColor mgl32.Vec3
	nIndices     int32
	ccw          bool
	// textured shapes sample the atlas, the rest take a flat colour.
	textured bool
}

func (cs *CompiledShape) Delete() {
	gl.DeleteBuffers(1, &cs.ibo)
	gl.DeleteBuffers(1, &cs.vbo)
}

func (cs *CompiledShape) Render(s *State) {
	s.cullCCW(cs.ccw)
	s.setTextured(cs.textured)
	bindVBO(cs.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, cs.ibo)
	gl.Uniform3fv(s.diffuseColLoc, 1, &cs.diffuseColor[0])
	// Quads are fans of four separated by the restart marker, so one draw
	// covers the whole shape.
	gl.DrawElements(gl.TRIANGLE_FAN, cs.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))
}
