package domain

type Probe struct {
	Position               Vec
	Valid                  bool
	Irradiance             Lobes
	Incident               []Beam
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
					s.Irradiance, s.Incident, s.Contributing, s.Occluded = incomingFull(p, r.Lights, r.Blocks)
				}
				out[g.index(x, y, z)] = s
			}
		}
	}
	return out
}

// incomingFull records both the legacy six-direction lobes and the resolved
// per-beams, which normal sampling recombines per receiver orientation.
func incomingFull(p Vec, lights []Light, blocks []Box) (Lobes, []Beam, int, int) {
	incident, visibleCount, blockedCount := beams(p, lights, blocks)
	var lobes Lobes
	for _, beam := range incident {
		lobes.project(beam.Direction, beam.Distance, beam.RGB)
	}
	return lobes, incident, visibleCount, blockedCount
}
