package x3d

import (
	"log"
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

type BoundingSphere struct {
	Radius float32
	Center mgl32.Vec3
}

func (bs *BoundingSphere) IntersectsSqr(aabb *AABB) bool {
	closest := aabb.ClosestPoint(bs.Center)
	diff := closest.Sub(bs.Center)
	dS := diff.LenSqr()
	return dS < bs.Radius
}

func (bs *BoundingSphere) Rotate(m mgl32.Mat3, ofs mgl32.Vec3) BoundingSphere {
	return BoundingSphere{
		Radius: bs.Radius,
		Center: m.Mul3x1(bs.Center).Add(ofs),
	}
}

func FrustumSphere(w, h, n, f, fov float32) BoundingSphere {

	// formulars took from
	// Eric Zhang: Calculate Minimal Bounding Sphere of Frustum
	// https://lxjk.github.io/2017/04/15/Calculate-Minimal-Bounding-Sphere-of-Frustum.html

	k := float32(math.Sqrt(float64(1+((h*h)/(w*w)))) * math.Tan(0.5*float64(fov)))

	k2 := k * k

	if k2 >= (f-n)/(f+n) {
		log.Println("simple case")
		return BoundingSphere{
			Radius: f * float32(math.Sqrt(float64(k2))),
			Center: mgl32.Vec3{0, 0, -f},
		}
	}

	a := (f - n) * (f - n)
	b := 2 * (f*f + n*n) * k2
	c := (f + n) * (f + n) * k2 * k2

	root := a + b + c

	return BoundingSphere{
		Radius: float32(0.5 * math.Sqrt(float64(root))),
		Center: mgl32.Vec3{0, 0, -0.5 * (f + n) * (1 + k2)},
	}
}
