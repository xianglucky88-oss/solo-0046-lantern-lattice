package domain

// visible reports whether the finite segment from p to q reaches q without
// touching an axis-aligned solid or the closed thin wall.
func visible(p, q Vec, blocks []Box, wall wallBasis) bool {
	for _, box := range blocks {
		if intersects(p, q, box) {
			return false
		}
	}
	return !wall.blocks(p, q)
}
