package domain

type Sample struct {
	Position   Vec
	State      string
	Weight     float64
	Irradiance Lobes
}

func sample(g Grid, probes []Probe, p Vec, blocks []Box) Sample {
	s := Sample{Position: p, State: "outside"}
	cell, ok := locate(g, p)
	if !ok {
		return s
	}
	if occupied(p, blocks) {
		s.State = "solid"
		return s
	}
	s.State = "unsupported"
	s.Irradiance = query(g, probes, cell, p, blocks, &s.Weight)
	if s.Weight > 0 {
		s.State = "sampled"
		for i := range s.Irradiance {
			s.Irradiance[i] = s.Irradiance[i].scale(1 / s.Weight)
		}
	}
	return s
}

// query accumulates the reduced trilinear support visible from p. It returns
// the unnormalized linear color and, through support, the pre-normalization
// support rate. Relocated probes contribute their corner weight times their
// move factor; invalid or occluded neighbors contribute nothing.
func query(g Grid, probes []Probe, cell [3]span, p Vec, blocks []Box, support *float64) Lobes {
	var color Lobes
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		w := n.weight * probe.Factor
		if w <= 0 {
			continue
		}
		*support += w
		for i, energy := range probe.Irradiance {
			color[i] = color[i].add(energy.scale(w))
		}
	}
	return color
}
