package x3d

import (
	"github.com/go-gl/mathgl/mgl32"
)

type BoundingSphere struct {
	// RadiusSqr is the squared radius: everything here works in
	// squared distances to avoid the square roots.
	RadiusSqr float32
	Center    mgl32.Vec3
}

func (bs *BoundingSphere) IntersectsSqr(aabb *AABB) bool {
	closest := aabb.ClosestPoint(bs.Center)
	diff := closest.Sub(bs.Center)
	dS := diff.LenSqr()
	return dS < bs.RadiusSqr
}
