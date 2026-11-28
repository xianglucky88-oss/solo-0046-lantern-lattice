package domain

// Contribution reports how one grid fed a query. Support is that grid's own
// neighbour support rate, Nominal is its share of the wall transition, and
// Effective is the product actually used for normalisation.
type Contribution struct {
	Side      int
	Covered   bool
	Support   float64
	Nominal   float64
	Effective float64
}

type Sample struct {
	Position      Vec
	State         string
	Weight        float64
	Irradiance    Lobes
	Contributions []Contribution
}

// supported accumulates one grid's trilinear support at p, keeping only
// neighbours that are valid and reachable without crossing solid geometry.
// It returns the grid's support rate (sum of surviving corner weights) and the
// weight-summed linear colour, still unnormalised.
func supported(g Grid, probes []Probe, p Vec, blocks []Box, wall wallBasis) (float64, Lobes) {
	var support float64
	var color Lobes
	cell, ok := locate(g, p)
	if !ok {
		return 0, color
	}
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks, wall) {
			continue
		}
		support += n.weight
		for i, energy := range probe.Irradiance {
			color[i] = color[i].add(energy.scale(n.weight))
		}
	}
	return support, color
}

// sample resolves one query. States stay distinct for solid geometry, points
// outside all coverage, and points inside coverage that receive no support.
func sample(r Request, p plan, all [][]Probe, at Vec) Sample {
	s := Sample{Position: at, State: "outside"}
	if occupied(at, r.Blocks) || p.wall.onClosedWall(at) {
		s.State = "solid"
		return s
	}

	s.Contributions = make([]Contribution, len(p.grids))
	colors := make([]Lobes, len(p.grids))
	var covers []int
	for gi, g := range p.grids {
		s.Contributions[gi].Side = g.Side
		if _, ok := locate(g, at); !ok {
			continue
		}
		s.Contributions[gi].Covered = true
		s.Contributions[gi].Support, colors[gi] = supported(g, all[gi], at, r.Blocks, p.wall)
		covers = append(covers, gi)
	}
	if len(covers) == 0 {
		return s // outside
	}

	// Nominal shares. A single covering grid keeps its original sampling rule:
	// full share, weight equals its own support. Two covering grids split the
	// wall transition band by signed distance.
	d := p.wall.signedDistance(at)
	for _, gi := range covers {
		if len(covers) == 1 {
			s.Contributions[gi].Nominal = 1
		} else {
			s.Contributions[gi].Nominal = nominalWeight(p.grids[gi].Side, d, p.transition)
		}
	}

	// Effective weight = support rate × nominal share. Their sum is the total
	// support rate; the linear colour is normalised by those same weights.
	var color Lobes
	for _, gi := range covers {
		c := &s.Contributions[gi]
		c.Effective = c.Support * c.Nominal
		s.Weight += c.Effective
		for i := range color {
			color[i] = color[i].add(colors[gi][i].scale(c.Nominal))
		}
	}
	if s.Weight > 0 {
		s.State = "sampled"
		s.Irradiance = normalize(color, s.Weight)
	} else {
		s.State = "unsupported"
	}
	return s
}

func normalize(color Lobes, weight float64) Lobes {
	var out Lobes
	for i := range out {
		out[i] = color[i].scale(1 / weight)
	}
	return out
}
