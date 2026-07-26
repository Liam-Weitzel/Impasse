package render

import (
	"bytes"
	"errors"
	"fmt"

	"unsafe"

	gl "github.com/go-gl/gl/v3.1/gles2"
)

func byteSliceToString(s []byte) string {
	n := bytes.IndexByte(s, 0)
	if n >= 0 {
		s = s[:n]
	}
	return string(s)
}

func compileShader(shaderType uint32, src string) (uint32, error) {

	src += "\x00"

	shader := gl.CreateShader(shaderType)

	sources, free := gl.Strs(src)
	gl.ShaderSource(shader, 1, sources, nil)
	free()
	gl.CompileShader(shader)
	//log.Printf("gl error: %d\n", gl.GetError())

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)

	if status == gl.FALSE {
		var length int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &length)
		log := make([]byte, length+1)
		gl.GetShaderInfoLog(shader, length, nil, (*uint8)(unsafe.Pointer(&log[0])))
		gl.DeleteShader(shader)
		return 0, fmt.Errorf("failed to compile: %d %v", length, byteSliceToString(log))
	}

	return shader, nil
}

func CreateProgram(vertex, fragment string) (uint32, error) {

	prog := gl.CreateProgram()
	if prog == 0 {
		gl.DeleteProgram(prog)
		return 0, errors.New("Cannot create shader program")
	}

	vs, err := compileShader(gl.VERTEX_SHADER, vertex)
	if err != nil {
		gl.DeleteProgram(prog)
		return 0, fmt.Errorf("vertex shader: %v", err)
	}
	defer gl.DeleteShader(vs)

	fs, err := compileShader(gl.FRAGMENT_SHADER, fragment)
	if err != nil {
		gl.DeleteProgram(prog)
		return 0, fmt.Errorf("fragment shader: %v", err)
	}
	defer gl.DeleteShader(fs)

	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)

	gl.LinkProgram(prog)

	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)

	if status == gl.FALSE {
		var length int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &length)
		log := make([]byte, length+1)
		gl.GetProgramInfoLog(prog, length, nil, (*uint8)(unsafe.Pointer(&log[0])))
		gl.DeleteProgram(prog)
		return 0, fmt.Errorf("linking shader program failed: %v", byteSliceToString(log))
	}

	return prog, nil
}
