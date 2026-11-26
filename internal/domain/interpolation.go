package domain

type Sample struct {
	Position   Vec
	State      string
	Weight     float64
	Irradiance Lobes
}

func sample(g Grid, probes []Probe, p Vec, blocks []Box) Sample {
	s := Sample{Position: p}
	state, used, weight := admissible(g, probes, p, blocks)
	s.State = state
	s.Weight = weight
	if state != "sampled" {
		return s
	}
	for _, n := range used {
		probe := probes[n.index]
		for i, energy := range probe.Irradiance {
			s.Irradiance[i] = s.Irradiance[i].add(energy.scale(n.weight))
		}
	}
	for i := range s.Irradiance {
		s.Irradiance[i] = s.Irradiance[i].scale(1 / s.Weight)
	}
	return s
}
