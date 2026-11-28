package domain

// nominalWeight is the transition share of a grid on the given side at signed
// wall distance d. The band is centred on the wall and spans width w, so the
// +side grid runs from 0 at d = -w/2 to 1 at d = +w/2 (linearly), and the
// -side grid is its complement. Outside the band the nearest end value holds.
func nominalWeight(side int, d, width float64) float64 {
	h := width / 2
	t := (d + h) / width
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	if side > 0 {
		return t
	}
	return 1 - t
}
