package render

import (
	"unsafe"

	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"
)

// CompiledShape is geometry uploaded to the GPU, ready to draw. Build them with
// a MeshBuilder.
//
// Shapes carry no bounding volume. Nothing culls yet, and the mesh is split by
// vertex count rather than by region, so a bounding box per chunk would not
// help anyway. Culling wants spatial chunking first.
type CompiledShape struct {
	vbo          uint32
	ibo          uint32
	diffuseColor mgl32.Vec3
	nIndices     int32
	ccw          bool
}

func (cs *CompiledShape) Delete() {
	gl.DeleteBuffers(1, &cs.ibo)
	gl.DeleteBuffers(1, &cs.vbo)
}

func (cs *CompiledShape) Render(s *State) {
	s.cullCCW(cs.ccw)
	bindVBO(cs.vbo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, cs.ibo)
	gl.Uniform3fv(s.diffuseColLoc, 1, &cs.diffuseColor[0])
	// Quads are fans of four separated by the restart marker, so one draw
	// covers the whole shape.
	gl.DrawElements(gl.TRIANGLE_FAN, cs.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))
}
