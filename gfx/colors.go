package gfx

func HSLToRGB(h, s, l float64) (float64, float64, float64) {

	h /= 360
	s /= 100
	l /= 100

	if s == 0 {
		// Achromatic: lightness in all three channels, not always white.
		return l, l, l
	}

	var v2 float64

	if l < 0.5 {
		v2 = l * (1 + s)
	} else {
		v2 = (l + s) - (s * l)
	}

	v1 := 2*l - v2

	r := HueToRGB(v1, v2, h+(1.0/3.0))
	g := HueToRGB(v1, v2, h)
	b := HueToRGB(v1, v2, h-(1.0/3.0))

	return r, g, b
}

func HueToRGB(v1, v2, h float64) float64 {
	if h < 0 {
		h += 1
	}
	if h > 1 {
		h -= 1
	}
	switch {
	case 6*h < 1:
		return (v1 + (v2-v1)*6*h)
	case 2*h < 1:
		return v2
	case 3*h < 2:
		return v1 + (v2-v1)*((2.0/3.0)-h)*6
	}
	return v1
}
