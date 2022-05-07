package opengl

import (
	"fmt"
	"log"
	"time"
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

	start := time.Now()

	//pos := mgl32.Vec3{(1280 + 1344) / 2, 520, (-264 + -280) / 2}
	//center := mgl32.Vec3{(1280 + 1344) / 2, 576, (-264 + -280) / 2}
	//up := mgl32.Vec3{0, 0, -1}

	front := mgl32.Vec3{0, 1, 0}
	up := mgl32.Vec3{0, 0, -1}

	viewMat := mgl32.LookAtV(c.Position, c.Position.Add(front), up)
	//viewMat := mgl32.LookAtV(pos, center, up)

	mvMat := viewMat
	normalMat := mvMat.Inv().Transpose()

	gl.UseProgram(r.program)

	gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
	gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.projMat[0])

	// Head light
	gl.Uniform3fv(r.state.lightPosLoc, 1, &c.Position[0])

	//gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Uniform3fv(r.state.ambientColLoc, 1, &r.ambientCol[0])

	for _, cs := range css {
		//gl.UseProgram(r.program)
		gl.BindBuffer(gl.ARRAY_BUFFER, cs.vbo)
		bindAttributes()
		//log.Printf("vbo: %d\n", cs.vbo)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, cs.ibo)

		//gl.UniformMatrix4fv(r.state.mvMatLoc, 1, false, &mvMat[0])
		//gl.UniformMatrix4fv(r.state.normalMatLoc, 1, false, &normalMat[0])
		//	gl.UniformMatrix4fv(r.state.projMatLoc, 1, false, &r.projMat[0])
		gl.BindTexture(gl.TEXTURE_2D, cs.texture)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.ActiveTexture(gl.TEXTURE0)
		//gl.Disable(gl.CULL_FACE)
		//gl.Enable(gl.PRIMITIVE_RESTART_FIXED_INDEX)
		gl.Uniform1i(r.state.texSamplerUniformLoc, 0)
		gl.Uniform3fv(r.state.diffuseColLoc, 1, &cs.diffuseColor[0])

		gl.DrawElements(gl.TRIANGLE_FAN, cs.nIndices, gl.UNSIGNED_SHORT, unsafe.Pointer(nil))

		//gl.Flush()
		//gl.BindBuffer(gl.VERTEX_ARRAY, 0)
		//gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)
	}
	log.Printf("Rendering took: %v\n", time.Since(start))
}

func (s *State) ExtractUniforms(program uint32) error {

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
