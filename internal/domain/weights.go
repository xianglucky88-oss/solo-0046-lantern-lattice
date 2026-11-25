package domain

type neighbor struct {
	index  int
	weight float64
}

func corners(g Grid, cell [3]span) []neighbor {
	out := make([]neighbor, 0, 8)
	for mask := range 8 {
		w := 1.0
		var i [3]int
		for a, s := range cell {
			i[a] = s.lower
			if mask&(1<<a) > 0 {
				i[a]++
				w *= s.fraction
			} else {
				w *= 1 - s.fraction
			}
		}
		if w > 0 {
			out = append(out, neighbor{g.index(i[0], i[1], i[2]), w})
		}
	}
	return out
}
