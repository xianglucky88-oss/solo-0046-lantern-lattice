package domain

// admissible reproduces, for any probe slice, the exact set of trilinear
// corner neighbors used by the reference sampler: valid probes whose segment
// to p has line of sight. The returned support is the sum of the raw
// (pre-normalization) trilinear weights.
//
// State precedence is identical to sample(): outside first, then solid, then
// unsupported / sampled.
func admissible(g Grid, probes []Probe, p Vec, blocks []Box) (state string, used []neighbor, support float64) {
	cell, ok := locate(g, p)
	if !ok {
		return "outside", nil, 0
	}
	if occupied(p, blocks) {
		return "solid", nil, 0
	}
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		used = append(used, n)
		support += n.weight
	}
	if support > 0 {
		return "sampled", used, support
	}
	return "unsupported", used, 0
}
