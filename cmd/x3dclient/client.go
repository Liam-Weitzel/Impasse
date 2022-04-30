package main

import (
	"log"

	"gitlab.com/sascha.l.teichmann/ssh3d/x3d"
	"gitlab.com/sascha.l.teichmann/ssh3d/x3d/opengl"
)

type client struct {
	scene     *x3d.Scene
	directory string
}

func newClient(scene *x3d.Scene, directory string) (*client, error) {
	return &client{
		scene:     scene,
		directory: directory,
	}, nil
}

func (c *client) run() error {
	log.Printf("num shapes: %d\n", len(c.scene.Shapes))

	tc := opengl.NewTextureCache(c.directory)
	defer tc.Delete()

	sc := opengl.NewShapeCompiler(tc)

	for _, s := range c.scene.Shapes {
		cs, err := sc.Compile(s)
		if err != nil {
			return err
		}
		// TODO: Implement me!
		_ = cs
	}
	log.Printf("Number of textures: %d\n", tc.NumTextures())
	return nil
}
