package opengl

import (
	"fmt"

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

	if err := s.ExtractUniforms(program); err != nil {
		gl.DeleteProgram(program)
		return nil, err
	}

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

func (r *Renderer) Render(c *Camera, css []*CompiledShape) {

	front := mgl32.Vec3{0, 0, -1}
	up := mgl32.Vec3{0, 1, 0}

	viewMat := mgl32.LookAtV(c.Position, c.Position.Add(front), up)

	mvMat := viewMat
	normalMat := mvMat.Inv().Transpose()

	gl.UseProgram(r.program)

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.projMat[0])

	// Head light
	gl.Uniform3fv(r.state.lightPosLoc, 1, &c.Position[0])

	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	for _, cs := range css {
		cs.Render(r.state)
	}
}

func (s *State) ExtractUniforms(program uint32) error {

	gl.UseProgram(program)

	for _, l := range []struct {
		name string
		addr *int32
	}{
		{"texSampler", &s.texSamplerUniformLoc},
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
