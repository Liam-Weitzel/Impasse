package main

import (
	"fmt"
	"unsafe"

	gl "github.com/go-gl/gl/v3.1/gles2"
)

type vertex struct {
	position [3]float32
	texCoord [2]float32
	normal   [3]float32
}

const (
	vertexSize  = unsafe.Sizeof(vertex{})
	texCoordOfs = unsafe.Offsetof(vertex{}.texCoord)
	normalOfs   = unsafe.Offsetof(vertex{}.normal)
)

func vboCreate(vertices []vertex) (uint32, error) {

	var vbo uint32

	const nBuffers = 1

	gl.GenBuffers(nBuffers, &vbo)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	gl.BufferData(
		gl.ARRAY_BUFFER,
		int(vertexSize)*len(vertices), gl.Ptr(vertices),
		gl.STATIC_DRAW)

	errNo := gl.GetError()

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)

	if errNo != gl.NO_ERROR {
		gl.DeleteBuffers(nBuffers, &vbo)
		return 0, fmt.Errorf("creating VBO failed: %d", errNo)
	}

	return vbo, nil
}

func iboCreate(indices []uint16) (uint32, error) {
	var ibo uint32

	const nBuffers = 1

	const sizeUint16 = int(unsafe.Sizeof(uint16(0)))

	gl.GenBuffers(nBuffers, &ibo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo)
	gl.BufferData(
		gl.ELEMENT_ARRAY_BUFFER,
		sizeUint16*len(indices),
		gl.Ptr(indices), gl.STATIC_DRAW)

	if errNo := gl.GetError(); errNo != gl.NO_ERROR {
		gl.DeleteBuffers(nBuffers, &ibo)
		return 0, fmt.Errorf("creating IBO failed: %d", errNo)
	}

	return ibo, nil
}
