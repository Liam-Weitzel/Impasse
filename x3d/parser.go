package x3d

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/go-gl/mathgl/mgl32"
)

type elementEnder interface {
	endElement(*parseContext, xml.EndElement) error
}

type funcElementEnder func(*parseContext, xml.EndElement) error

func (fee funcElementEnder) endElement(pc *parseContext, ee xml.EndElement) error {
	if fee != nil {
		return fee(pc, ee)
	}
	return nil
}

type parseContext struct {
	scene    *Scene
	textures map[string]*Texture
	decoder  *xml.Decoder
	handlers []elementEnder
}

var noEndHandler = funcElementEnder(nil)

func (pc *parseContext) viewpointHandler(se xml.StartElement) (elementEnder, error) {

	type viewpoint struct {
		Description string `xml:"description,attr"`
		Orientation string `xml:"orientation,attr"`
		Position    string `xml:"position,attr"`
	}

	var vp viewpoint

	if err := pc.decoder.DecodeElement(&vp, &se); err != nil {
		return nil, err
	}

	pos, err := parseVector3(vp.Position)
	if err != nil {
		return nil, err
	}

	ori, err := parseVector4(vp.Orientation)
	if err != nil {
		return nil, err
	}

	pc.scene.Viewpoints = append(pc.scene.Viewpoints, &Viewpoint{
		Orientation: ori,
		Position:    pos,
		Description: vp.Description,
	})

	return noEndHandler, nil
}

type shapeHandler struct{ Shape }

func (sh *shapeHandler) endElement(pc *parseContext, se xml.EndElement) error {
	if sh.Appearance == nil {
		return errors.New("missing appearance")
	}
	if sh.Geometry == nil {
		return errors.New("missing geometry")
	}
	pc.scene.Shapes = append(pc.scene.Shapes, &sh.Shape)
	return nil
}

func (pc *parseContext) shapeHandler(xml.StartElement) (elementEnder, error) {
	return &shapeHandler{}, nil
}

type appearanceHandler struct{ *Appearance }

func (pc *parseContext) appearanceHandler(se xml.StartElement) (elementEnder, error) {
	for _, at := range se.Attr {
		switch at.Name.Local {
		case "DEF":
			appearance := pc.scene.Appearances[at.Value]
			if appearance != nil {
				return nil, fmt.Errorf("Appearance '%s' already exists", at.Value)
			}
			appearance = &Appearance{}
			pc.scene.Appearances[at.Value] = appearance
			return &appearanceHandler{appearance}, nil
		case "USE":
			appearance := pc.scene.Appearances[at.Value]
			if appearance == nil {
				return nil, fmt.Errorf("No Appearance '%s' defined", at.Value)
			}
			if !pc.sendAppearanceToShape(appearance) {
				return nil, errors.New("Appearance not in Shape")
			}
			return noEndHandler, nil
		}
	}

	return nil, errors.New("missing DEF of USE attribute in Appearance")
}

func (pc *parseContext) handleImageTexture(se xml.StartElement) (elementEnder, error) {

	url, ok := findAttr("url", se.Attr)
	if !ok {
		return nil, errors.New("missing 'url' attribute in ImageTexture")
	}

	urls := parseMFString(url)
	if len(urls) < 1 {
		return nil, errors.New("no URLs given in 'url' attribute in ImageTexture")
	}
	texture := pc.getTexture(urls[0])
	if !pc.sendTextureToAppearance(texture) {
		return nil, errors.New("ImageTexture not in Appearance")
	}
	return noEndHandler, nil
}

func (pc *parseContext) getTexture(src string) *Texture {
	texture := pc.textures[src]
	if texture == nil {
		texture = &Texture{Source: src}
		pc.textures[src] = texture
	}
	return texture
}

func (pc *parseContext) sendTo(fn func(elementEnder) bool) bool {
	for i := len(pc.handlers) - 1; i >= 0; i-- {
		if fn(pc.handlers[i]) {
			return true
		}
	}
	return false
}

func (pc *parseContext) sendAppearanceToShape(appearance *Appearance) bool {
	return pc.sendTo(func(h elementEnder) bool {
		sh, ok := h.(*shapeHandler)
		if ok {
			sh.Appearance = appearance
		}
		return ok
	})
}

func (pc *parseContext) sendTextureToAppearance(texture *Texture) bool {
	return pc.sendTo(func(h elementEnder) bool {
		ah, ok := h.(*appearanceHandler)
		if ok {
			ah.Appearance.Texture = texture
		}
		return ok
	})
}

func (pc *parseContext) sendDiffuseColorToAppearance(dc mgl32.Vec3) bool {
	return pc.sendTo(func(h elementEnder) bool {
		ah, ok := h.(*appearanceHandler)
		if ok {
			ah.Appearance.DiffuseColor = dc
		}
		return ok
	})
}

func (pc *parseContext) sendGeometryToShape(geometry *IndexedFaceSet) bool {
	return pc.sendTo(func(h elementEnder) bool {
		sh, ok := h.(*shapeHandler)
		if ok {
			sh.Geometry = geometry
			geometry.Bounds(&sh.Bounds)
		}
		return ok
	})
}

func (pc *parseContext) sendCoordinatesToIndexedFaceSet(coords []mgl32.Vec3) bool {
	return pc.sendTo(func(h elementEnder) bool {
		ifsh, ok := h.(*indexedFaceSetHandler)
		if ok {
			ifsh.IndexedFaceSet.Coordinates = coords
		}
		return ok
	})
}

func (pc *parseContext) sendTextureCoordinateToIndexedFaceSet(uvs []mgl32.Vec2) bool {
	return pc.sendTo(func(h elementEnder) bool {
		ifsh, ok := h.(*indexedFaceSetHandler)
		if ok {
			ifsh.IndexedFaceSet.TextureCoordinates = uvs
		}
		return ok
	})
}

func (pc *parseContext) sendNormalToIndexedFaceSet(vs []mgl32.Vec3) bool {
	return pc.sendTo(func(h elementEnder) bool {
		ifsh, ok := h.(*indexedFaceSetHandler)
		if ok {
			ifsh.IndexedFaceSet.Normals = vs
		}
		return ok
	})
}

func (ah *appearanceHandler) endElement(pc *parseContext, se xml.EndElement) error {
	if ah.Texture == nil {
		return errors.New("Appearance has no Texture")
	}
	if !pc.sendAppearanceToShape(ah.Appearance) {
		return errors.New("Appearance not in Shape")
	}
	return nil
}

type indexedFaceSetHandler struct{ IndexedFaceSet }

func (pc *parseContext) handleIndexedFaceSet(se xml.StartElement) (elementEnder, error) {

	var (
		coordIndices    []int32
		texCoordIndices []int32
		normalIndices   []int32
		ccw             bool = true
		normalPerVertex bool = true
		convex          bool = true
	)

	if err := parseAttrs(se.Attr, []attrParser{
		{"coordIndex", intsParser(&coordIndices)},
		{"texCoordIndex", intsParser(&texCoordIndices)},
		{"normalIndex", intsParser(&normalIndices)},
		{"ccw", boolParser(&ccw)},
		{"normalPerVertex", boolParser(&normalPerVertex)},
		{"convex", boolParser(&convex)},
	}); err != nil {
		return nil, err
	}

	if len(coordIndices) == 0 {
		return nil, errors.New("IndexedFaceSet has no attribute 'coordIndex'")
	}
	if len(texCoordIndices) == 0 {
		return nil, errors.New("IndexedFaceSet has no attribute 'texCoordIndex'")
	}
	if len(normalIndices) == 0 {
		return nil, errors.New("IndexedFaceSet has no attribute 'normalIndex'")
	}

	return &indexedFaceSetHandler{
		IndexedFaceSet{
			CoordIndices:    coordIndices,
			TexCoordIndices: texCoordIndices,
			NormalIndices:   normalIndices,
			CCW:             ccw,
			NormalPerVertex: normalPerVertex,
			Convex:          convex,
		},
	}, nil
}

func (ifsh *indexedFaceSetHandler) endElement(pc *parseContext, se xml.EndElement) error {

	ifs := &ifsh.IndexedFaceSet

	if len(ifs.Coordinates) == 0 {
		return errors.New("IndexedFaceSet has no Coordinates")
	}
	if len(ifs.CoordIndices) == 0 {
		return errors.New("IndexedFaceSet has no CoordIndices")
	}

	// check coord indices
	for _, ci := range ifs.CoordIndices {
		if ci == -1 {
			continue
		}
		if ci < 0 || int(ci) >= len(ifs.Coordinates) {
			return fmt.Errorf(
				"coordIndex %d is out of bounds [0-%d]", ci, len(ifs.Coordinates)-1)
		}
	}

	if len(ifs.TextureCoordinates) == 0 {
		return errors.New("IndexedFaceSet has no TextureCoordinates")
	}
	if len(ifs.TexCoordIndices) == 0 {
		return errors.New("IndexedFaceSet has no TexCoordIndices")
	}
	if len(ifs.Normals) == 0 {
		return errors.New("IndexedFaceSet has no Normals")
	}

	// check tex coord indices
	for _, ti := range ifs.TexCoordIndices {
		if ti == -1 {
			continue
		}
		if ti < 0 || int(ti) >= len(ifs.TextureCoordinates) {
			return fmt.Errorf(
				"texCoordIndex %d is out of bounds [0-%d]", ti, len(ifs.TextureCoordinates)-1)
		}
	}

	if !pc.sendGeometryToShape(ifs) {
		return errors.New("IndexedFaceSet not inside Shape")
	}
	return nil
}

func (pc *parseContext) handleCoordinate(se xml.StartElement) (elementEnder, error) {
	point, ok := findAttr("point", se.Attr)
	if !ok {
		return nil, errors.New("missing 'point' attribute in Coordinate")
	}
	coords, err := parseVector3s(point)
	if err != nil {
		return nil, err
	}
	if !pc.sendCoordinatesToIndexedFaceSet(coords) {
		return nil, errors.New("Coordinate not in IndexedFaceSet")
	}
	return noEndHandler, nil
}

func (pc *parseContext) handleTextureCoordinate(se xml.StartElement) (elementEnder, error) {

	point, ok := findAttr("point", se.Attr)
	if !ok {
		return nil, errors.New("missing 'point' attribute in TextureCoordinate")
	}

	uvs, err := parseUVs(point)
	if err != nil {
		return nil, fmt.Errorf("TextureCoordinate: %v", err)
	}

	if !pc.sendTextureCoordinateToIndexedFaceSet(uvs) {
		return nil, errors.New("TextureCoordinate not in IndexedFaceSet")
	}

	return noEndHandler, nil
}

func (pc *parseContext) handleNormal(se xml.StartElement) (elementEnder, error) {
	vector, ok := findAttr("vector", se.Attr)
	if !ok {
		return nil, errors.New("missing 'vector' attribute in Normal")
	}

	vs, err := parseVector3s(vector)
	if err != nil {
		return nil, fmt.Errorf("Normal: %v", err)
	}

	if !pc.sendNormalToIndexedFaceSet(vs) {
		return nil, errors.New("Normal not in IndexedFaceSet")
	}
	return noEndHandler, nil
}

func (pc *parseContext) handleMaterial(se xml.StartElement) (elementEnder, error) {
	if v, ok := findAttr("diffuseColor", se.Attr); ok {
		dc, err := parseVector3(v)
		if err != nil {
			return nil, fmt.Errorf("Invalid diffuseColor: %v", err)
		}
		if !pc.sendDiffuseColorToAppearance(dc) {
			return nil, errors.New("Material not in Appearance")
		}
	}
	return noEndHandler, nil
}

var startElements = map[string]func(
	*parseContext, xml.StartElement) (elementEnder, error){
	"Viewpoint":         (*parseContext).viewpointHandler,
	"Shape":             (*parseContext).shapeHandler,
	"Appearance":        (*parseContext).appearanceHandler,
	"ImageTexture":      (*parseContext).handleImageTexture,
	"IndexedFaceSet":    (*parseContext).handleIndexedFaceSet,
	"Coordinate":        (*parseContext).handleCoordinate,
	"TextureCoordinate": (*parseContext).handleTextureCoordinate,
	"Normal":            (*parseContext).handleNormal,
	"Material":          (*parseContext).handleMaterial,
}

func ParseScene(r io.Reader) (*Scene, error) {

	scene := NewScene()

	decoder := xml.NewDecoder(r)

	pc := parseContext{
		scene:    scene,
		textures: map[string]*Texture{},
		decoder:  decoder,
	}

tokens:
	for {
		tok, err := decoder.Token()
		switch {
		case tok == nil && err == io.EOF:
			break tokens
		case err != nil:
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			//log.Printf("start: %s\n", t.Name.Local)
			var eh elementEnder
			if sh := startElements[t.Name.Local]; sh != nil {
				if eh, err = sh(&pc, t); err != nil {
					return nil, err
				}
			} else {
				eh = noEndHandler
			}
			pc.handlers = append(pc.handlers, eh)
		case xml.EndElement:
			eh := pc.handlers[len(pc.handlers)-1]
			pc.handlers[len(pc.handlers)-1] = nil
			pc.handlers = pc.handlers[:len(pc.handlers)-1]
			if err := eh.endElement(&pc, t); err != nil {
				return nil, err
			}
		}
	}

	return scene, nil
}
