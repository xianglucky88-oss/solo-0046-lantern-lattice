package domain

type Sample struct {
	Position   Vec
	State      string
	Weight     float64
	Irradiance Lobes
}

type NormalSample struct {
	Position   Vec
	Normal     Vec
	State      string
	Weight     float64
	Irradiance Vec
}

// sample is the historical spatial query. It is retained unchanged so every
// caller that does not ask for receiver normals keeps identical output.
func sample(g Grid, probes []Probe, p Vec, blocks []Box) Sample {
	s, _ := sampleLinks(g, probes, p, blocks)
	return s
}

// sampleLinks resolves the participating probes once. Both the six-direction
// and receiver-normal results consume exactly these links, which is what
// keeps state, support and the set of contributing probes identical between
// the two entry points.
func sampleLinks(g Grid, probes []Probe, p Vec, blocks []Box) (Sample, []neighbor) {
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
	var used []neighbor
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		s.Weight += n.weight
		used = append(used, n)
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
	return s, used
}

// sampleNormal evaluates one query point against a receiver normal. State and
// support (Weight) come from the same participating-probe set as sample; the
// linear RGB is each participant's own hemispherical response blended by the
// same trilinear weights. At a probe's own position the result reduces to
// that probe's response for the same normal, so the two entry points agree.
func sampleNormal(g Grid, probes []Probe, p, normal Vec, blocks []Box) NormalSample {
	n, ok := unitNormal(normal)
	s := NormalSample{Position: p, Normal: normal, State: "outside"}
	cell, located := locate(g, p)
	if !located {
		return s
	}
	if occupied(p, blocks) {
		s.State = "solid"
		return s
	}
	s.State = "unsupported"
	var sum Vec
	for _, nbr := range corners(g, cell) {
		probe := probes[nbr.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		s.Weight += nbr.weight
		if ok {
			sum = sum.add(normalEnergy(probe.Incident, n).scale(nbr.weight))
		}
	}
	if s.Weight > 0 {
		s.State = "sampled"
		if ok {
			s.Irradiance = sum.scale(1 / s.Weight)
		}
	}
	return s
}
