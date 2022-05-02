package opengl

import "github.com/go-gl/mathgl/mgl32"

type Vertex struct {
	coords  mgl32.Vec3
	tex     mgl32.Vec2
	normals mgl32.Vec3
}
