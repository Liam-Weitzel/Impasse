package x3d

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
)

func parseMFString(s string) []string {
	s = strings.TrimSpace(s)

	mode := 0
	var b strings.Builder

	var res []string

	for _, c := range s {
		switch mode {
		case 0:
			switch c {
			case '\'':
				mode = 1
			case '"':
				mode = 2
			default:
				return []string{s}
			}
		case 1:
			switch c {
			case '\\':
				mode = 3
			case '\'':
				res = append(res, b.String())
				b.Reset()
				mode = 0
			default:
				b.WriteRune(c)
			}
		case 2:
			switch c {
			case '\\':
				mode = 4
			case '"':
				res = append(res, b.String())
				b.Reset()
				mode = 0
			default:
				b.WriteRune(c)
			}
		case 3:
			b.WriteRune(c)
			mode = 1
		case 4:
			b.WriteRune(c)
			mode = 2
		}
	}
	return res
}

func parseVector3(s string) (mgl32.Vec3, error) {
	var v mgl32.Vec3
	_, err := fmt.Sscanf(s, "%f %f %f", &v[0], &v[1], &v[2])
	return v, err
}

func parseVector4(s string) (mgl32.Vec4, error) {
	var v mgl32.Vec4
	_, err := fmt.Sscanf(s, "%f %f %f %f", &v[0], &v[1], &v[2], &v[3])
	return v, err
}

func parseVector3s(s string) ([]mgl32.Vec3, error) {
	fields := strings.Fields(s)
	if len(fields)%3 != 0 {
		return nil, errors.New("not multiple of 3")
	}

	vs := make([]mgl32.Vec3, 0, len(fields))

	for i := 0; i < len(fields); i += 3 {
		x, err := strconv.ParseFloat(fields[i+0], 32)
		if err != nil {
			return nil, err
		}
		y, err := strconv.ParseFloat(fields[i+1], 32)
		if err != nil {
			return nil, err
		}
		z, err := strconv.ParseFloat(fields[i+2], 32)
		if err != nil {
			return nil, err
		}
		vs = append(vs, mgl32.Vec3{
			float32(x), float32(y), float32(z)})
	}

	return vs, nil
}

func parseUVs(s string) ([]mgl32.Vec2, error) {

	fields := strings.Fields(s)
	if len(fields)%2 != 0 {
		return nil, errors.New("not multiple of 2")
	}

	uvs := make([]mgl32.Vec2, 0, len(fields))

	for i := 0; i < len(fields); i += 2 {
		u, err := strconv.ParseFloat(fields[i+0], 32)
		if err != nil {
			return nil, err
		}
		v, err := strconv.ParseFloat(fields[i+1], 32)
		if err != nil {
			return nil, err
		}
		uvs = append(uvs, mgl32.Vec2{float32(u), float32(v)})
	}

	return uvs, nil
}

func findAttr(name string, attrs []xml.Attr) (string, bool) {
	for i := range attrs {
		if attrs[i].Name.Local == name {
			return attrs[i].Value, true
		}
	}
	return "", false
}

type attrParser struct {
	name    string
	handler func(string) error
}

func parseAttrs(attrs []xml.Attr, parsers []attrParser) error {
	for i := range attrs {
		name := attrs[i].Name.Local
		for _, p := range parsers {
			if p.name == name {
				if err := p.handler(attrs[i].Value); err != nil {
					return fmt.Errorf(name+": %v", err)
				}
				break
			}
		}
	}
	return nil

}

func intsParser(p *[]int32) func(string) error {
	return func(v string) error {
		var err error
		*p, err = parseInt32s(v)
		return err
	}
}

func boolParser(p *bool) func(string) error {
	return func(v string) error {
		var err error
		*p, err = strconv.ParseBool(v)
		return err
	}
}

func parseInt32s(s string) ([]int32, error) {
	fields := strings.Fields(s)
	ints := make([]int32, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.ParseInt(f, 10, 32)
		if err != nil {
			return nil, err
		}
		ints = append(ints, int32(v))
	}
	return ints, nil
}
