package domain

import "math"

type span struct {
	lower    int
	fraction float64
}

func locate(g Grid, p Vec) ([3]span, bool) {
	var coordinates [3]span
	for a := range 3 {
		at := (p[a] - g.Origin[a]) / g.Step
		top := float64(g.Size[a] - 1)
		if at < -1e-9 || at > top+1e-9 {
			return coordinates, false
		}
		at = math.Max(0, math.Min(top, at))
		lo := min(int(math.Floor(at)), g.Size[a]-2)
		coordinates[a] = span{lo, at - float64(lo)}
	}
	return coordinates, true
}
