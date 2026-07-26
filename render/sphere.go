package render

import (
	"math"

	gl "github.com/go-gl/gl/v3.1/gles2"
)

type Sphere struct {
	radius  float32
	sectors int
	stacks  int
	smooth  bool

	vbo      uint32
	ibo      uint32
	nIndices int32
}

func NewSphere(
	radius float32,
	sectors int,
	stacks int,
	smooth bool,
) (*Sphere, error) {

	sp := Sphere{
		radius:  radius,
		sectors: sectors,
		stacks:  stacks,
		smooth:  smooth,
	}

	/*
		var vertices []Vertex
		var indices []uint16

		if smooth {
			vertices, indices = sp.buildVerticesSmooth()
		} else {
			sp.buildVerticesFlat()
		}
	*/

	vertices, indices := sp.buildVerticesSmooth()

	vbo, err := createVBO(vertices)
	if err != nil {
		return nil, err
	}

	ibo, err := createIBO(indices)
	if err != nil {
		gl.DeleteBuffers(1, &vbo)
		return nil, err
	}

	sp.vbo = vbo
	sp.ibo = ibo
	sp.nIndices = int32(len(indices))

	return &sp, nil
}

func (sp *Sphere) Delete() {
	gl.DeleteBuffers(1, &sp.ibo)
	gl.DeleteBuffers(1, &sp.vbo)
}

func (sp *Sphere) buildVerticesSmooth() ([]Vertex, []uint16) {

	count := (sp.sectors + 1) * (sp.stacks + 1)
	vertices := make([]Vertex, count)
	indices := make([]uint16, 6*sp.sectors*(sp.stacks-1))

	addVertex := func(index int, x, y, z float32) {
		vs := vertices[index].coord[:]
		vs[0] = x
		vs[1] = y
		vs[2] = z
	}

	addNormal := func(index int, x, y, z float32) {
		ns := vertices[index].normal[:]
		ns[0] = x
		ns[1] = y
		ns[2] = z
	}

	addIndices := func(index int, i1, i2, i3 int) {
		ids := indices[index : index+3]
		ids[0] = uint16(i1)
		ids[1] = uint16(i2)
		ids[2] = uint16(i3)
	}

	lengthInv := 1 / sp.radius
	sectorStep := 2 * math.Pi / float32(sp.sectors)
	stackStep := math.Pi / float32(sp.stacks)

	var ii int

	for i := 0; i <= sp.stacks; i++ {
		stackAngle := math.Pi/2 - float32(i)*stackStep
		xy := sp.radius * float32(math.Cos(float64(stackAngle)))
		z := sp.radius * float32(math.Sin(float64(stackAngle)))

		for j := 0; j <= sp.sectors; j++ {

			sectorAngle := float32(j) * sectorStep

			// vertex position
			x := xy * float32(math.Cos(float64(sectorAngle)))
			y := xy * float32(math.Sin(float64(sectorAngle)))
			addVertex(ii, x, y, z)

			// normalized vertex normal
			nx := x * lengthInv
			ny := y * lengthInv
			nz := z * lengthInv
			addNormal(ii, nx, ny, nz)

			ii++
		}
	}

	// indices
	//  k1--k1+1
	//  |  / |
	//  | /  |
	//  k2--k2+1
	var kk int
	for i := 0; i < sp.stacks; i++ {
		k1 := i * (sp.sectors + 1) // beginning of current stack
		k2 := k1 + sp.sectors + 1  // beginning of next stack

		for j := 0; j < sp.sectors; j, k1, k2 = j+1, k1+1, k2+1 {
			// 2 triangles per sector excluding 1st and last stacks
			if i != 0 {
				addIndices(kk, k1, k2, k1+1) // k1---k2---k1+1
				kk += 3
			}

			if i != sp.stacks-1 {
				addIndices(kk, k1+1, k2, k2+1) // k1+1---k2---k2+1
				kk += 3
			}
		}
	}

	return vertices, indices
}

/*

func (s *Sphere) buildVerticesFlat() ([]Vertex, []uint16) {
	return nil, nil
}

*/
