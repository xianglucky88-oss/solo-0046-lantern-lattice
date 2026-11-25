package domain

type Probe struct {
	Position               Vec
	Valid                  bool
	Irradiance             Lobes
	Contributing, Occluded int
}

func bake(r Request) []Probe {
	g := r.Grid
	out := make([]Probe, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				p := g.position(x, y, z)
				s := Probe{Position: p, Valid: !occupied(p, r.Blocks)}
				if s.Valid {
					s.Irradiance, s.Contributing, s.Occluded = incoming(p, r.Lights, r.Blocks)
				}
				out[g.index(x, y, z)] = s
			}
		}
	}
	return out
}
