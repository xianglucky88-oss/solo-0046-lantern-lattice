package domain

// plan is the resolved form of a request after validation: one or two grids,
// an optional active wall, and the transition width for cross-door blending.
type plan struct {
	grids      []Grid
	wall       wallBasis
	transition float64
	legacy     bool
}

func resolve(r Request) plan {
	p := plan{grids: r.Grids, transition: r.Transition}
	if r.Grid != nil {
		p.grids = []Grid{*r.Grid}
		p.legacy = true
	}
	if r.Wall != nil {
		p.wall, _ = newWallBasis(*r.Wall)
	}
	return p
}

// textureFile is the archive name for grid i. The legacy single-grid artifact
// keeps the historical probes.png; the cross-door artifact names each grid.
func (p plan) textureFile(i int) string {
	if p.legacy {
		return "probes.png"
	}
	return textureName(i)
}
