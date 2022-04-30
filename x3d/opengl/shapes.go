package opengl

import "gitlab.com/sascha.l.teichmann/ssh3d/x3d"

type CompiledShape struct {
}

type ShapeCompiler struct {
	textureCache *TextureCache
}

func NewShapeCompiler(tc *TextureCache) *ShapeCompiler {
	return &ShapeCompiler{
		textureCache: tc,
	}
}

func (sc *ShapeCompiler) Compile(s *x3d.Shape) (*CompiledShape, error) {

	t, err := sc.textureCache.GetTexture(s.Appearance.Texture.Source)
	if err != nil {
		return nil, err
	}

	// TODO: Implement me!
	_ = t

	return nil, nil
}
