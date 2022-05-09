package opengl

import (
	"fmt"
	"image"
	"math"
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

	lastCull    uint32
	lastTexture uint32
}

type Renderer struct {
	state      *State
	program    uint32
	ambientCol mgl32.Vec3
	projMat    mgl32.Mat4
}

func NewRenderer(ambientCol mgl32.Vec3, projMat mgl32.Mat4) (*Renderer, error) {

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
		projMat:    projMat,
	}, nil
}

func (r *Renderer) Delete() {
	gl.DeleteProgram(r.program)
}

func (r *Renderer) Render(c *Camera, css []*CompiledShape, img *image.RGBA) {

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
	gl.Enable(gl.CULL_FACE)
	r.state.lastCull = gl.CW
	gl.FrontFace(r.state.lastCull)

	gl.UseProgram(r.program)

	gl.ActiveTexture(gl.TEXTURE0)

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.projMat[0])

	// Head light
	gl.Uniform3fv(r.state.lightPosLoc, 1, &c.Position[0])

	//gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])

	for _, cs := range css {
		cs.Render(r.state)
	}

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
	} {
		if *l.addr = gl.GetUniformLocation(
			program, gl.Str(l.name+"\x00")); *l.addr < 0 {
			return fmt.Errorf("could not find uniform '%s'", l.name)
		}
	}

	return nil
}
