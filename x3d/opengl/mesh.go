package opengl

import (
	"math"

	gl "github.com/go-gl/gl/v3.1/gles2"
	"github.com/go-gl/mathgl/mgl32"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
)

// Indices are uint16 and MaxUint16 is the primitive restart marker, so a single
// shape cannot hold more than that many vertices. MeshBuilder starts a new
// shape before it gets close. Splitting also helps culling, since each chunk
// gets its own bounding box.
const (
	primitiveRestart = math.MaxUint16
	maxShapeVertices = 60000
)

// MeshBuilder turns quads into CompiledShapes. Quads are drawn as four vertex
// triangle fans separated by the restart marker, which is what CompiledShape
// already does for X3D faces, so nothing downstream needs to change.
//
// Vertices carry no texture coordinates. Shapes built here are drawn untextured
// and take their colour from the diffuse and ambient uniforms.
type MeshBuilder struct {
	color mgl32.Vec3

	vertices []Vertex
	indices  []uint16
	bounds   x3d.AABB

	shapes []*CompiledShape
}

func emptyAABB() x3d.AABB {
	const big = math.MaxFloat32
	return x3d.AABB{
		Min: mgl32.Vec3{big, big, big},
		Max: mgl32.Vec3{-big, -big, -big},
	}
}

func NewMeshBuilder(color mgl32.Vec3) *MeshBuilder {
	return &MeshBuilder{
		color:  color,
		bounds: emptyAABB(),
	}
}

// AddQuad appends one quad. The corners must be given in order around the face,
// not crosswise, or the fan comes out as a bowtie.
func (mb *MeshBuilder) AddQuad(a, b, c, d, normal mgl32.Vec3) error {
	if len(mb.vertices)+4 > maxShapeVertices {
		if err := mb.flush(); err != nil {
			return err
		}
	}

	base := uint16(len(mb.vertices))

	for _, corner := range [4]mgl32.Vec3{a, b, c, d} {
		mb.vertices = append(mb.vertices, Vertex{
			coord:  corner,
			normal: normal,
		})
		mb.bounds.Extend(corner)
	}

	mb.indices = append(mb.indices,
		base, base+1, base+2, base+3, primitiveRestart)

	return nil
}

// Compile uploads whatever is left and returns every shape built so far. The
// builder is empty afterwards and can be reused.
func (mb *MeshBuilder) Compile() ([]*CompiledShape, error) {
	if err := mb.flush(); err != nil {
		mb.Delete()
		return nil, err
	}
	shapes := mb.shapes
	mb.shapes = nil
	return shapes, nil
}

// Delete releases shapes already uploaded. Only needed when Compile fails part
// way through, since on success the caller owns them.
func (mb *MeshBuilder) Delete() {
	for _, cs := range mb.shapes {
		cs.Delete()
	}
	mb.shapes = nil
}

func (mb *MeshBuilder) flush() error {
	if len(mb.indices) == 0 {
		return nil
	}

	ibo, err := createIBO(mb.indices)
	if err != nil {
		return err
	}
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo)

	vbo, err := createVBO(mb.vertices)
	if err != nil {
		gl.DeleteBuffers(1, &ibo)
		return err
	}

	bounds := mb.bounds

	mb.shapes = append(mb.shapes, &CompiledShape{
		Bounds:       &bounds,
		vbo:          vbo,
		ibo:          ibo,
		diffuseColor: mb.color,
		nIndices:     int32(len(mb.indices)),
	})

	mb.vertices = mb.vertices[:0]
	mb.indices = mb.indices[:0]
	mb.bounds = emptyAABB()

	return nil
}
