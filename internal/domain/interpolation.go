package domain

type Sample struct {
	Position   Vec
	State      string
	Weight     float64
	Irradiance Lobes

	// SupportDuration and SolidDuration are reported only by time-varying
	// bakes; the static path leaves them zero and omits them via omitempty.
	SupportDuration float64 `json:",omitempty"`
	SolidDuration   float64 `json:",omitempty"`
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
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		s.Weight += n.weight
		for i, energy := range probe.Irradiance {
			s.Irradiance[i] = s.Irradiance[i].add(energy.scale(n.weight))
		}
	}
	if s.Weight > 0 {
		s.State = "sampled"
		for i := range s.Irradiance {
			s.Irradiance[i] = s.Irradiance[i].scale(1 / s.Weight)
		}
	}
	return s
}
