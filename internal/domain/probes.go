package domain

type Probe struct {
	Position Vec
	Valid    bool
	// Factor is the reduced weight applied to this probe when it supports a
	// query. Unmoved probes keep 1; relocated probes scale linearly from 1 at
	// zero move down to 0 at the maximum move distance.
	Factor                 float64
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
					s.Factor = 1
					s.Irradiance, s.Contributing, s.Occluded = incoming(p, r.Lights, r.Blocks)
				}
				out[g.index(x, y, z)] = s
			}
		}
	}
	return out
}
