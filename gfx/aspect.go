package gfx

func AspectRatioToFOV(ar float32) float32 {

	return 60

	const (
		_4_3  = float32(4) / 3
		_16_9 = float32(16) / 9
	)

	/*
		f(4/3) = 90
		f(16/9) = 106

		106 = m * _16_9 + b
		90  = m * _4_3  + b

		106 - 90 = m * (_16_9 - _4_3)

		m = (106 - 90) / (_16_9 - 4_3)

		b = 106 - m * _16_9
	*/

	m := (106 - 90) / (_16_9 - _4_3)
	b := 106 - m*_16_9

	fov := m*ar + b

	if fov < 90 {
		fov = 90
	} else if fov > 95 {
		fov = 95
	}

	return fov
}
