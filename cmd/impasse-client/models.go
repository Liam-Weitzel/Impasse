package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/Liam-Weitzel/Impasse/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Player models.
//
// At this resolution a player is a handful of characters, so the only thing
// that carries is silhouette. A sphere and a cube look nearly identical from a
// steep angle, which is why the interesting options are the ones that change
// the outline: something tall, or something with a point.
//
// All of them are built as ordinary shapes and drawn with a model transform,
// rather than going through the specialised sphere path, so a model can rotate
// or be swapped without the renderer knowing anything about it.

type modelKind string

const (
	modelSphere  modelKind = "sphere"
	modelCube    modelKind = "cube"
	modelPrism   modelKind = "prism"
	modelDiamond modelKind = "diamond"
	modelPylon   modelKind = "pylon"
)

// modelSpec is the one place a model's proportions live, so the builder and
// anything that reasons about the shape cannot drift apart.
//
// height is a multiple of the marker radius, and every model stands on z=0
// rather than being centred on it: a marker centred on the origin sinks half
// into the floor.
type modelSpec struct {
	height float32
	build  func(col mgl32.Vec3, r float32) ([]*render.CompiledShape, error)
}

var models = map[modelKind]modelSpec{
	modelSphere: {2.0, func(c mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
		// Many sided, so it reads as round. An icosphere would be nicer,
		// but this is the shape the others are being compared against.
		return buildTaperedPrism(c, r, r, r*2.0, 12)
	}},
	modelCube: {1.6, func(c mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
		return buildBox(c, r*0.9, r*0.9, r*1.6)
	}},
	modelPrism: {1.8, func(c mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
		return buildTaperedPrism(c, r, r, r*1.8, 6)
	}},
	modelDiamond: {1.7, func(c mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
		return buildDiamond(c, r, r*1.7)
	}},
	modelPylon: {2.3, func(c mgl32.Vec3, r float32) ([]*render.CompiledShape, error) {
		// A narrow tapered column. Tallest silhouette of the lot, which is
		// what survives being flattened into block characters, and the
		// taper gives it a direction the others do not have.
		return buildTaperedPrism(c, r*1.05, r*0.35, r*2.3, 6)
	}},
}

var modelKinds = []modelKind{
	modelSphere, modelCube, modelPrism, modelDiamond, modelPylon,
}

func modelNames() []string {
	out := make([]string, 0, len(models))
	for k := range models {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

func parseModel(name string) (modelKind, error) {
	if name == "" {
		return modelPylon, nil
	}
	if _, ok := models[modelKind(name)]; ok {
		return modelKind(name), nil
	}
	return "", fmt.Errorf("unknown model %q, have %v", name, modelNames())
}

// buildPlayerModel makes the geometry for one player marker, standing on z=0.
// The colour is white so the per player tint does all the work.
func buildPlayerModel(kind modelKind, r float32) ([]*render.CompiledShape, error) {
	spec, ok := models[kind]
	if !ok {
		return nil, fmt.Errorf("unhandled model %q", kind)
	}
	return spec.build(mgl32.Vec3{1, 1, 1}, r)
}

// buildBox makes an axis aligned box sitting on z=0.
//
// Quads are wound counter clockwise seen from outside, matching everything else
// the mesh builder produces.
func buildBox(color mgl32.Vec3, hx, hy, h float32) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(color)

	z0, z1 := float32(0), h

	faces := []struct {
		corners [4]mgl32.Vec3
		normal  mgl32.Vec3
	}{
		{[4]mgl32.Vec3{{hx, -hy, z0}, {hx, hy, z0}, {hx, hy, z1}, {hx, -hy, z1}},
			mgl32.Vec3{1, 0, 0}},
		{[4]mgl32.Vec3{{-hx, hy, z0}, {-hx, -hy, z0}, {-hx, -hy, z1}, {-hx, hy, z1}},
			mgl32.Vec3{-1, 0, 0}},
		{[4]mgl32.Vec3{{hx, hy, z0}, {-hx, hy, z0}, {-hx, hy, z1}, {hx, hy, z1}},
			mgl32.Vec3{0, 1, 0}},
		{[4]mgl32.Vec3{{-hx, -hy, z0}, {hx, -hy, z0}, {hx, -hy, z1}, {-hx, -hy, z1}},
			mgl32.Vec3{0, -1, 0}},
		{[4]mgl32.Vec3{{-hx, -hy, z1}, {hx, -hy, z1}, {hx, hy, z1}, {-hx, hy, z1}},
			mgl32.Vec3{0, 0, 1}},
		{[4]mgl32.Vec3{{-hx, hy, z0}, {hx, hy, z0}, {hx, -hy, z0}, {-hx, -hy, z0}},
			mgl32.Vec3{0, 0, -1}},
	}

	for _, f := range faces {
		if err := mb.AddQuad(
			f.corners[0], f.corners[1], f.corners[2], f.corners[3], f.normal,
		); err != nil {
			return nil, err
		}
	}

	return mb.Compile()
}

// buildTaperedPrism is the same but with a different radius at the top, which
// is what turns a column into something that reads as pointing upwards.
func buildTaperedPrism(color mgl32.Vec3, rBottom, rTop, h float32, sides int) ([]*render.CompiledShape, error) {
	if sides < 3 {
		return nil, fmt.Errorf("a prism needs at least 3 sides, got %d", sides)
	}

	mb := render.NewMeshBuilder(color)

	at := func(i int, radius, z float32) mgl32.Vec3 {
		a := 2 * math.Pi * float64(i) / float64(sides)
		return mgl32.Vec3{
			radius * float32(math.Cos(a)),
			radius * float32(math.Sin(a)),
			z,
		}
	}

	for i := 0; i < sides; i++ {
		b0, b1 := at(i, rBottom, 0), at(i+1, rBottom, 0)
		t0, t1 := at(i, rTop, h), at(i+1, rTop, h)

		// Outward normal, from the midpoint of the face.
		mid := b0.Add(b1).Mul(0.5)
		normal := mgl32.Vec3{mid[0], mid[1], 0}
		if normal.Len() > 0 {
			normal = normal.Normalize()
		}

		if err := mb.AddQuad(b0, b1, t1, t0, normal); err != nil {
			return nil, err
		}
	}

	// Caps, as fans of triangles pushed through AddQuad with the last corner
	// repeated so the second triangle of the fan collapses.
	topC := mgl32.Vec3{0, 0, h}
	botC := mgl32.Vec3{0, 0, 0}
	for i := 0; i < sides; i++ {
		t0, t1 := at(i, rTop, h), at(i+1, rTop, h)
		if err := mb.AddQuad(topC, t0, t1, t1, mgl32.Vec3{0, 0, 1}); err != nil {
			return nil, err
		}

		b0, b1 := at(i, rBottom, 0), at(i+1, rBottom, 0)
		if err := mb.AddQuad(botC, b1, b0, b0, mgl32.Vec3{0, 0, -1}); err != nil {
			return nil, err
		}
	}

	return mb.Compile()
}

// buildDiamond makes an octahedron standing on its lower point, which gives the
// most distinctive outline of the lot at the cost of looking like it is
// balancing.
func buildDiamond(color mgl32.Vec3, r, h float32) ([]*render.CompiledShape, error) {
	mb := render.NewMeshBuilder(color)

	top := mgl32.Vec3{0, 0, h}
	bottom := mgl32.Vec3{0, 0, 0}
	mid := h / 2

	// Equator, counter clockwise seen from above.
	eq := [4]mgl32.Vec3{
		{r, 0, mid}, {0, r, mid}, {-r, 0, mid}, {0, -r, mid},
	}

	for i := 0; i < 4; i++ {
		a, b := eq[i], eq[(i+1)%4]

		up := a.Add(b).Mul(0.5).Sub(bottom)
		upper := mgl32.Vec3{up[0], up[1], mid}.Normalize()

		if err := mb.AddQuad(top, a, b, b, upper); err != nil {
			return nil, err
		}
		if err := mb.AddQuad(bottom, b, a, a,
			mgl32.Vec3{upper[0], upper[1], -upper[2]}); err != nil {
			return nil, err
		}
	}

	return mb.Compile()
}

// buildPickup makes the pickup marker: a small diamond, drawn spinning, because
// a rotating silhouette is the cheapest way to make something read as an object
// rather than a mark on the floor.
func buildPickup(r float32) ([]*render.CompiledShape, error) {
	return buildDiamond(mgl32.Vec3{1, 1, 1}, r, r*2)
}
