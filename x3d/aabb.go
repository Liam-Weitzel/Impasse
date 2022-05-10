package x3d

import "github.com/go-gl/mathgl/mgl32"

type AABB struct {
	Min mgl32.Vec3
	Max mgl32.Vec3
}

func (aabb *AABB) ClosestPoint(p mgl32.Vec3) mgl32.Vec3 {
	// TODO: Implement me!
	var r mgl32.Vec3
	for i := range r {
		if p[i] > aabb.Max[i] {
			r[i] = aabb.Max[i]
		} else if p[i] < aabb.Min[i] {
			r[i] = aabb.Min[i]
		} else {
			r[i] = p[i]
		}
	}
	return r
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minVec3(a, b mgl32.Vec3) mgl32.Vec3 {
	return mgl32.Vec3{
		min32(a[0], b[0]),
		min32(a[1], b[1]),
		min32(a[2], b[2]),
	}
}

func maxVec3(a, b mgl32.Vec3) mgl32.Vec3 {
	return mgl32.Vec3{
		max32(a[0], b[0]),
		max32(a[1], b[1]),
		max32(a[2], b[2]),
	}
}

func (aabb *AABB) Extend(v mgl32.Vec3) {
	aabb.Min = minVec3(aabb.Min, v)
	aabb.Max = maxVec3(aabb.Max, v)
}
