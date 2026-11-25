package domain

func visible(p, q Vec, blocks []Box) bool {
	for _, box := range blocks {
		if intersects(p, q, box) {
			return false
		}
	}
	return true
}
