package opengl

import (
	"fmt"
	"unsafe"

	gl "github.com/go-gl/gl/v3.0/gles2"
	"github.com/go-gl/mathgl/mgl32"
)

type Vertex struct {
	coord    mgl32.Vec3
	texCoord mgl32.Vec2
	normal   mgl32.Vec3
}

const (
	vertexSize  = unsafe.Sizeof(Vertex{})
	coordOfs    = unsafe.Offsetof(Vertex{}.coord)
	texCoordOfs = unsafe.Offsetof(Vertex{}.texCoord)
	normalOfs   = unsafe.Offsetof(Vertex{}.normal)
)

func bindVBO(vbo uint32) {
	const (
		positionIdx = iota
		texCoordIdx
		normalIdx
	)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)

	gl.VertexAttribPointer(
		positionIdx, 3, gl.FLOAT, false,
		int32(vertexSize),
		gl.PtrOffset(int(coordOfs)))
	gl.EnableVertexAttribArray(positionIdx)

	gl.VertexAttribPointer(
		texCoordIdx, 2, gl.FLOAT, false,
		int32(vertexSize),
		gl.PtrOffset(int(texCoordOfs)))
	gl.EnableVertexAttribArray(texCoordIdx)

	gl.VertexAttribPointer(
		normalIdx, 3, gl.FLOAT, false,
		int32(vertexSize),
		gl.PtrOffset(int(normalOfs)))
	gl.EnableVertexAttribArray(normalIdx)
}

func createVBO(vertices []Vertex) (uint32, error) {

	var vbo uint32

	const nBuffers = 1

	gl.GenBuffers(nBuffers, &vbo)

	bindVBO(vbo)

	gl.BufferData(
		gl.ARRAY_BUFFER,
		//int(vertexSize)*len(vertices), gl.Ptr(vertices),
		int(vertexSize)*len(vertices), unsafe.Pointer(&vertices[0]),
		gl.STATIC_DRAW)

	//gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)

	errNo := gl.GetError()

	//gl.BindBuffer(gl.ARRAY_BUFFER, 0)

	if errNo != gl.NO_ERROR {
		gl.DeleteBuffers(nBuffers, &vbo)
		return 0, fmt.Errorf("creating VBO failed: %d", errNo)
	}

	return vbo, nil
}

func layoutVBO(vbo uint32) {
}

func createIBO(indices []uint16) (uint32, error) {
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
