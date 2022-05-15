package opengl

import (
	"fmt"
	"image"
	"math"
	"sort"
	"unsafe"

	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"

	_ "embed"
)

//go:embed texture.vert
var vertexSrc string

//go:embed texture.frag
var fragSrc string

type State struct {
	texSamplerUniformLoc int32
	mvMatLoc             int32
	normalMatLoc         int32
	projMatLoc           int32
	lightPosLoc          int32
	ambientColLoc        int32
	diffuseColLoc        int32
	useTextureLoc        int32

	lastCull    uint32
	lastTexture uint32
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

func (r *Renderer) RenderShapes(c *Camera, css []*CompiledShape) {

	// Sort by textures to reduce texture changes.
	sort.Slice(css, func(i, j int) bool {
		return css[i].texture < css[j].texture
	})

	//pos := mgl32.Vec3{(1280 + 1344) / 2, 520, (-264 + -280) / 2}
	//center := mgl32.Vec3{(1280 + 1344) / 2, 576, (-264 + -280) / 2}
	//up := mgl32.Vec3{0, 0, -1}

	front := mgl32.Vec3{
		float32(math.Cos(float64(c.Angle))),
		float32(math.Sin(float64(c.Angle))),
		0}

	up := mgl32.Vec3{0, 0, -1}

	//up = mgl32.HomogRotate3D(c.UpAngle, mgl32.Vec3{0, 0, 1}).Mul4x1(up.Vec4(1)).Vec3()

	flip := mgl32.Scale3D(1, -1, 1)

	//log.Printf("pos: %v\n", c.Position)

	viewMat := mgl32.LookAtV(c.Position, c.Position.Add(front), up)
	//viewMat := mgl32.LookAtV(pos, center, up)
	rot := mgl32.HomogRotate3D(c.UpAngle, mgl32.Vec3{1, 0, 0})
	viewMat = rot.Mul4(viewMat)

	mvMat := viewMat.Mul4(flip)
	//mvMat := viewMat
	normalMat := mvMat.Inv().Transpose()
	//gl.Viewport(0, 0, int32(img.Bounds().Dx()), int32(img.Bounds().Dy()))

	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.CullFace(gl.BACK)
	gl.Enable(gl.CULL_FACE)
	r.state.lastCull = gl.CW
	gl.FrontFace(r.state.lastCull)

	gl.UseProgram(r.program)

	gl.ActiveTexture(gl.TEXTURE0)

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.ProjMat[0])
	gl.Uniform1i(r.state.useTextureLoc, 1)

	// Head light
	gl.Uniform3fv(r.state.lightPosLoc, 1, &c.Position[0])

	//gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])

	for _, cs := range css {
		cs.Render(r.state)
	}
}

type SpherePostion struct {
	Pos mgl32.Vec3
	Col mgl32.Vec3
}

func (r *Renderer) RenderSpheres(c *Camera, sp *Sphere, positions []SpherePostion) {
	front := mgl32.Vec3{
		float32(math.Cos(float64(c.Angle))),
		float32(math.Sin(float64(c.Angle))),
		0}

	up := mgl32.Vec3{0, 0, -1}

	//up = mgl32.HomogRotate3D(c.UpAngle, mgl32.Vec3{0, 0, 1}).Mul4x1(up.Vec4(1)).Vec3()

	//flip := mgl32.Scale3D(1, -1, 1)

	//log.Printf("pos: %v\n", c.Position)

	viewMat := mgl32.LookAtV(c.Position, c.Position.Add(front), up)
	//viewMat := mgl32.LookAtV(pos, center, up)
	rot := mgl32.HomogRotate3D(c.UpAngle, mgl32.Vec3{1, 0, 0})

	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)

	diff := mgl32.Vec3{0.5, 0.5, 0.5}
	gl.Uniform3fv(r.state.diffuseColLoc, 1, &diff[0])
	gl.Uniform1i(r.state.useTextureLoc, 0)

	for i := range positions {
		vm := rot.Mul4(viewMat).Mul4(
			mgl32.Translate3D(
				positions[i].Pos[0],
				positions[i].Pos[1],
				positions[i].Pos[2]))

		//mvMat := vm.Mul4(flip)
		mvMat := vm
		normalMat := mvMat.Inv().Transpose()

		gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
		gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
		gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.ProjMat[0])

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
		int32(img.Bounds().Dx()), int32(image.Black.Bounds().Dy()),
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

func (s *State) bindTexture(texture uint32) {
	if texture == s.lastTexture {
		return
	}
	s.lastTexture = texture
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.Uniform1i(s.texSamplerUniformLoc, 0)
}

func (s *State) extractUniforms(program uint32) error {

	gl.UseProgram(program)

	for _, l := range []struct {
		name string
		addr *int32
	}{
		//{"texSampler", &s.texSamplerUniformLoc},
		{"mvMat", &s.mvMatLoc},
		{"projMat", &s.projMatLoc},
		{"ambientCol", &s.ambientColLoc},
		{"diffuseCol", &s.diffuseColLoc},
		{"normalMat", &s.normalMatLoc},
		{"lightPos", &s.lightPosLoc},
		{"useTexture", &s.useTextureLoc},
	} {
		if *l.addr = gl.GetUniformLocation(
			program, gl.Str(l.name+"\x00")); *l.addr < 0 {
			return fmt.Errorf("could not find uniform '%s'", l.name)
		}
	}

	return nil
}
