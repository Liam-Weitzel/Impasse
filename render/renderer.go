package render

import (
	"fmt"
	"image"
	"unsafe"

	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"

	_ "embed"
)

//go:embed shape.vert
var vertexSrc string

//go:embed shape.frag
var fragSrc string

type State struct {
	mvMatLoc      int32
	normalMatLoc  int32
	projMatLoc    int32
	lightPosLoc   int32
	ambientColLoc int32
	diffuseColLoc int32

	lastCull uint32
}

type Renderer struct {
	state      *State
	program    uint32
	ambientCol mgl32.Vec3
	ProjMat    mgl32.Mat4
}

func NewRenderer(ambientCol mgl32.Vec3) (*Renderer, error) {

	program, err := CreateProgram(vertexSrc, fragSrc)
	if err != nil {
		return nil, err
	}

	s := &State{}

	if err := s.extractUniforms(program); err != nil {
		gl.DeleteProgram(program)
		return nil, err
	}

	gl.Enable(gl.CULL_FACE)
	gl.ClearColor(0, 0.5, 1, 1)

	gl.Enable(gl.PRIMITIVE_RESTART_FIXED_INDEX)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)

	return &Renderer{
		state:      s,
		program:    program,
		ambientCol: ambientCol,
	}, nil
}

func (r *Renderer) Delete() {
	gl.DeleteProgram(r.program)
}

// RenderMesh draws shapes with a view matrix supplied by the caller.
//
// Front faces are clockwise, not counter clockwise. The projection mirrors
// clip space vertically to undo the bottom up order glReadPixels returns, and
// that reverses triangle orientation on screen.
//
// The light is at the eye. lightPos is in view space and the shader wants a
// head light, so the origin is what it needs.
func (r *Renderer) RenderMesh(view mgl32.Mat4, css []*CompiledShape) {

	normalMat := view.Inv().Transpose()

	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	r.state.lastCull = gl.CW
	gl.FrontFace(r.state.lastCull)

	gl.UseProgram(r.program)

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &view[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.ProjMat[0])

	eye := mgl32.Vec3{0, 0, 0}
	gl.Uniform3fv(r.state.lightPosLoc, 1, &eye[0])
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])

	for _, cs := range css {
		cs.Render(r.state)
	}
}

type SpherePosition struct {
	Pos mgl32.Vec3
	Col mgl32.Vec3
}

// RenderSpheresMesh draws one sphere per position, using the same view matrix
// convention as RenderMesh. Call it after RenderMesh, which sets the shared
// uniforms and binds the program.
func (r *Renderer) RenderSpheresMesh(
	view mgl32.Mat4,
	sp *Sphere,
	positions []SpherePosition,
) {
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	r.state.lastCull = gl.CW
	gl.FrontFace(r.state.lastCull)

	diff := mgl32.Vec3{0.5, 0.5, 0.5}
	gl.Uniform3fv(r.state.diffuseColLoc, 1, &diff[0])

	for i := range positions {
		mvMat := view.Mul4(mgl32.Translate3D(
			positions[i].Pos[0],
			positions[i].Pos[1],
			positions[i].Pos[2]))
		normalMat := mvMat.Inv().Transpose()

		gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
		gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])

		gl.Uniform3fv(r.state.ambientColLoc, 1, &positions[i].Col[0])

		bindVBO(sp.vbo)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, sp.ibo)
		gl.DrawElements(gl.TRIANGLES, sp.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))
	}
}

func (r *Renderer) ReadImage(img *image.RGBA) {

	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

	gl.ReadPixels(
		0, 0,
		int32(img.Bounds().Dx()), int32(img.Bounds().Dy()),
		gl.RGBA, gl.UNSIGNED_BYTE,
		unsafe.Pointer(&img.Pix[0]))

}

func (s *State) cullCCW(ccw bool) {
	var value uint32
	if ccw {
		value = gl.CW
	} else {
		value = gl.CCW
	}
	if value != s.lastCull {
		s.lastCull = value
		gl.FrontFace(value)
	}
}

func (s *State) extractUniforms(program uint32) error {

	gl.UseProgram(program)

	for _, l := range []struct {
		name string
		addr *int32
	}{
		{"mvMat", &s.mvMatLoc},
		{"projMat", &s.projMatLoc},
		{"ambientCol", &s.ambientColLoc},
		{"diffuseCol", &s.diffuseColLoc},
		{"normalMat", &s.normalMatLoc},
		{"lightPos", &s.lightPosLoc},
	} {
		if *l.addr = gl.GetUniformLocation(
			program, gl.Str(l.name+"\x00")); *l.addr < 0 {
			return fmt.Errorf("could not find uniform '%s'", l.name)
		}
	}

	return nil
}
