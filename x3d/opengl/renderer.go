package opengl

import (
	"fmt"

	gl "github.com/go-gl/gl/v3.0/gles2"
)

type State struct {
	texSamplerUniformLoc int32
	mvMatLoc             int32
	normalMatLoc         int32
	projMatLoc           int32
	lightPosLoc          int32
	ambientColLoc        int32
	diffuseColLoc        int32
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
