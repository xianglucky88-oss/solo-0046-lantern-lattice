package domain

func inside(p Vec, b Box) bool {
	for axis := range 3 {
		if p[axis] < b.Min[axis] || p[axis] > b.Max[axis] {
			return false
		}
	}
	return true
}
func occupied(p Vec, blocks []Box) bool {
	for _, b := range blocks {
		if inside(p, b) {
			return true
		}
	}
	return false
}
