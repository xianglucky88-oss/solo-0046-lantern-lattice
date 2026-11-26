package domain

type Sample struct {
	Position   Vec
	State      string
	Weight     float64
	Irradiance Lobes
}

// Link is one frozen trilinear contribution of a grid probe to a query point.
// Validity and line of sight are decided once against the solid geometry and
// never change when face reflectances are edited.
type Link struct {
	Index  int     `json:"Probe"`
	Weight float64 `json:"Weight"`
}

func sample(g Grid, probes []Probe, p Vec, blocks []Box) Sample {
	s, _ := sampleDetail(g, probes, p, blocks)
	return s
}

func sampleDetail(g Grid, probes []Probe, p Vec, blocks []Box) (Sample, []Link) {
	s := Sample{Position: p, State: "outside"}
	cell, ok := locate(g, p)
	if !ok {
		return s, nil
	}
	if occupied(p, blocks) {
		s.State = "solid"
		return s, nil
	}
	s.State = "unsupported"
	var links []Link
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		s.Weight += n.weight
		links = append(links, Link{n.index, n.weight})
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
	return s, links
}
