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
	tilesLoc      int32
	texturedLoc   int32
	fogColorLoc   int32
	fogFarLoc     int32

	lastCull uint32

	texturedOn    bool
	texturedKnown bool
}

// setTextured flips the shader between sampling the atlas and using a flat
// colour, cached so a run of shapes of the same sort costs one call.
func (s *State) setTextured(on bool) {
	if s.texturedKnown && s.texturedOn == on {
		return
	}
	s.texturedKnown = true
	s.texturedOn = on

	var v int32
	if on {
		v = 1
	}
	gl.Uniform1i(s.texturedLoc, v)
}

type Renderer struct {
	state      *State
	program    uint32
	ambientCol mgl32.Vec3
	atlas      *Atlas
	fogColor   mgl32.Vec3
	fogFar     float32
	ProjMat    mgl32.Mat4
}

// SetAtlas gives the renderer the tile texture to sample. Nil means everything
// draws with flat colours.
func (r *Renderer) SetAtlas(a *Atlas) {
	r.atlas = a
}

// SetFog decides where the world dissolves and into what. The colour has to
// match whatever the framebuffer is cleared to, or distance fades to one colour
// and then meets a hard edge of another.
func (r *Renderer) SetFog(col mgl32.Vec3, far float32) {
	r.fogColor = col
	r.fogFar = far
	gl.ClearColor(col[0], col[1], col[2], 1)
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

	gl.Enable(gl.PRIMITIVE_RESTART_FIXED_INDEX)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)

	r := &Renderer{
		state:      s,
		program:    program,
		ambientCol: ambientCol,
		fogColor:   mgl32.Vec3{0, 0, 0},
		fogFar:     1500,
	}
	r.SetFog(r.fogColor, r.fogFar)

	return r, nil
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

	// One atlas for the whole world, bound once.
	if r.atlas != nil {
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_2D, r.atlas.texture)
		gl.Uniform1i(r.state.tilesLoc, 0)
	}

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &view[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.ProjMat[0])

	eye := mgl32.Vec3{0, 0, 0}
	gl.Uniform3fv(r.state.lightPosLoc, 1, &eye[0])
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])
	gl.Uniform3fv(r.state.fogColorLoc, 1, &r.fogColor[0])
	gl.Uniform1f(r.state.fogFarLoc, r.fogFar)

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
	// Players and pickups are solid colour, never textured.
	r.state.setTextured(false)

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

// RenderShapeAt draws one shape with an extra model transform and its own
// colour, for things that move independently of the world. Call it after
// RenderMesh, which binds the program and sets the shared uniforms.
func (r *Renderer) RenderShapeAt(
	view, model mgl32.Mat4,
	cs *CompiledShape,
	col mgl32.Vec3,
) {
	mvMat := view.Mul4(model)
	normalMat := mvMat.Inv().Transpose()

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.Uniform3fv(r.state.ambientColLoc, 1, &col[0])

	cs.Render(r.state)

	// Leave the shared matrices as the caller set them.
	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &view[0])
	viewNormal := view.Inv().Transpose()
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &viewNormal[0])
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])
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
		{"tiles", &s.tilesLoc},
		{"textured", &s.texturedLoc},
		{"fogColor", &s.fogColorLoc},
		{"fogFar", &s.fogFarLoc},
	} {
		if *l.addr = gl.GetUniformLocation(
			program, gl.Str(l.name+"\x00")); *l.addr < 0 {
			return fmt.Errorf("could not find uniform '%s'", l.name)
		}
	}

	return nil
}
