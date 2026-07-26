package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"gitlab.com/sascha.l.teichmann/ssh3d/grid"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

// World units. X runs east, Y runs north, Z is up. The floor sits at z=0.
//
// Grid rows count southward, so world Y is the negated grid row. Without that
// the frame is mirrored: a camera sitting south of its target and looking north
// would put east on the left of the screen.
//
// The cell size is arbitrary in itself, but the fragment shader carries Quake
// scaled constants, fogFar in particular, so 64 puts the fog at a sensible
// distance. Change this and retune texture.frag with it.
const (
	cellSize   = 64
	wallHeight = 96
)

var (
	floorColor = mgl32.Vec3{0.35, 0.35, 0.40}
	wallColor  = mgl32.Vec3{0.55, 0.45, 0.35}
)

// cellCenter is the world point at the middle of a cell, on the floor.
func cellCenter(p grid.Pos) mgl32.Vec3 {
	return mgl32.Vec3{
		(float32(p.X) + 0.5) * cellSize,
		-(float32(p.Y) + 0.5) * cellSize,
		0,
	}
}

// buildGridMesh turns the grid into geometry. Only surfaces a player could see
// are emitted: floors of walkable cells, the wall faces that front onto them,
// and the tops of those walls.
//
// Quads are wound counter clockwise seen from the front, which is what the
// renderer culls against.
func buildGridMesh(g *grid.Grid) ([]*opengl.CompiledShape, error) {

	floors := opengl.NewMeshBuilder(floorColor)
	walls := opengl.NewMeshBuilder(wallColor)

	const h = wallHeight

	for y := 0; y < g.Height(); y++ {
		for x := 0; x < g.Width(); x++ {
			p := grid.Pos{X: x, Y: y}

			x0 := float32(x) * cellSize
			x1 := x0 + cellSize
			// Grid rows run south, world Y runs north, so the row's north
			// edge is the larger value.
			y1 := -float32(y) * cellSize
			y0 := y1 - cellSize

			if !g.Walkable(p) {
				// Wall tops, but only where the wall borders somewhere a
				// player can stand. The rest is never visible.
				if wallBordersFloor(g, p) {
					if err := walls.AddQuad(
						mgl32.Vec3{x0, y0, h},
						mgl32.Vec3{x1, y0, h},
						mgl32.Vec3{x1, y1, h},
						mgl32.Vec3{x0, y1, h},
						mgl32.Vec3{0, 0, 1},
					); err != nil {
						return nil, err
					}
				}
				continue
			}

			if err := floors.AddQuad(
				mgl32.Vec3{x0, y0, 0},
				mgl32.Vec3{x1, y0, 0},
				mgl32.Vec3{x1, y1, 0},
				mgl32.Vec3{x0, y1, 0},
				mgl32.Vec3{0, 0, 1},
			); err != nil {
				return nil, err
			}

			// Wall to the north, so the face sits on the north edge and
			// looks back south into this cell.
			if !g.Walkable(grid.Pos{X: x, Y: y - 1}) {
				if err := walls.AddQuad(
					mgl32.Vec3{x0, y1, 0},
					mgl32.Vec3{x1, y1, 0},
					mgl32.Vec3{x1, y1, h},
					mgl32.Vec3{x0, y1, h},
					mgl32.Vec3{0, -1, 0},
				); err != nil {
					return nil, err
				}
			}

			// Wall to the south.
			if !g.Walkable(grid.Pos{X: x, Y: y + 1}) {
				if err := walls.AddQuad(
					mgl32.Vec3{x1, y0, 0},
					mgl32.Vec3{x0, y0, 0},
					mgl32.Vec3{x0, y0, h},
					mgl32.Vec3{x1, y0, h},
					mgl32.Vec3{0, 1, 0},
				); err != nil {
					return nil, err
				}
			}

			// Wall to the west.
			if !g.Walkable(grid.Pos{X: x - 1, Y: y}) {
				if err := walls.AddQuad(
					mgl32.Vec3{x0, y0, 0},
					mgl32.Vec3{x0, y1, 0},
					mgl32.Vec3{x0, y1, h},
					mgl32.Vec3{x0, y0, h},
					mgl32.Vec3{1, 0, 0},
				); err != nil {
					return nil, err
				}
			}

			// Wall to the east.
			if !g.Walkable(grid.Pos{X: x + 1, Y: y}) {
				if err := walls.AddQuad(
					mgl32.Vec3{x1, y1, 0},
					mgl32.Vec3{x1, y0, 0},
					mgl32.Vec3{x1, y0, h},
					mgl32.Vec3{x1, y1, h},
					mgl32.Vec3{-1, 0, 0},
				); err != nil {
					return nil, err
				}
			}
		}
	}

	shapes, err := floors.Compile()
	if err != nil {
		return nil, err
	}

	wallShapes, err := walls.Compile()
	if err != nil {
		for _, cs := range shapes {
			cs.Delete()
		}
		return nil, err
	}

	return append(shapes, wallShapes...), nil
}

// wallBordersFloor reports whether any of the eight neighbours is walkable.
// Diagonals count, otherwise the tops of corner blocks are missing.
func wallBordersFloor(g *grid.Grid, p grid.Pos) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if g.Walkable(grid.Pos{X: p.X + dx, Y: p.Y + dy}) {
				return true
			}
		}
	}
	return false
}
