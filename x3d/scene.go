package x3d

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

type Viewpoint struct {
	Position    mgl32.Vec3
	Orientation mgl32.Vec4
	Description string
}

type Texture struct {
	Source string
}

type Appearance struct {
	Texture *Texture
}

type IndexedFaceSet struct {
	Coordinates        []mgl32.Vec3
	CoordIndices       []int32
	TextureCoordinates []mgl32.Vec2
	TexCoordIndices    []int32
}

type Shape struct {
	Bounds     AABB
	Appearance *Appearance
	Geometry   *IndexedFaceSet
}

type Scene struct {
	Viewpoints []*Viewpoint
	Shapes     []*Shape

	Appearances map[string]*Appearance
}

func NewScene() *Scene {
	return &Scene{
		Appearances: make(map[string]*Appearance),
	}
}

func (w *Scene) ViewpointByDescrption(desc string) *Viewpoint {
	for _, vp := range w.Viewpoints {
		if vp.Description == desc {
			return vp
		}
	}
	return nil
}

func (ifs *IndexedFaceSet) Bounds(bounds *AABB) {

	bounds.Min = mgl32.Vec3{
		math.MaxFloat32,
		math.MaxFloat32,
		math.MaxFloat32,
	}
	bounds.Max = mgl32.Vec3{
		-math.MaxFloat32,
		-math.MaxFloat32,
		-math.MaxFloat32,
	}

	for _, idx := range ifs.CoordIndices {
		if idx >= 0 {
			bounds.Extend(ifs.Coordinates[idx])
		}
	}
}
