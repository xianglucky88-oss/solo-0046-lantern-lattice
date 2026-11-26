package domain

// NeighborInfo describes one admissible trilinear contributor for a query.
type NeighborInfo struct {
	Index  int     `json:"Index"`
	Weight float64 `json:"Weight"`
	Fine   bool    `json:"Fine"`
}

// QueryResult is a reference Sample plus the tier that supplied its light and
// the contributor set. Embedding Sample keeps the canonical JSON shape.
type QueryResult struct {
	Sample
	Tier      string         `json:"Tier"`
	Neighbors []NeighborInfo `json:"Neighbors"`
}

// TieredQuery evaluates arbitrary positions against a coarse probe volume.
//
// The geometric state and support weight always come from the same grid,
// validity and line-of-sight rules as the original bake. Fine data only changes
// color values: when every contributing neighbor has an entry in fine, the
// whole query uses those exact lobes; otherwise the whole query stays on coarse.
// Arithmetic mirrors sample() operation-for-operation so the all-fine path is
// bit-for-bit the original linear color.
func TieredQuery(g Grid, blocks []Box, coarse []Probe, fine map[int]Lobes, points []Vec) []QueryResult {
	out := make([]QueryResult, 0, len(points))
	for _, p := range points {
		qr := QueryResult{Sample: Sample{Position: p}}
		state, used, weight := admissible(g, coarse, p, blocks)
		qr.State = state
		qr.Weight = weight
		if state != "sampled" {
			qr.Tier = "none"
			out = append(out, qr)
			continue
		}
		allFine := fine != nil
		if allFine {
			for _, n := range used {
				if _, ok := fine[n.index]; !ok {
					allFine = false
					break
				}
			}
		}
		qr.Neighbors = make([]NeighborInfo, 0, len(used))
		for _, n := range used {
			_, hasFine := fine[n.index]
			qr.Neighbors = append(qr.Neighbors, NeighborInfo{Index: n.index, Weight: n.weight, Fine: allFine && hasFine})
			var lobes Lobes
			if allFine {
				lobes = fine[n.index]
			} else {
				lobes = coarse[n.index].Irradiance
			}
			for i, energy := range lobes {
				qr.Irradiance[i] = qr.Irradiance[i].add(energy.scale(n.weight))
			}
		}
		for i := range qr.Irradiance {
			qr.Irradiance[i] = qr.Irradiance[i].scale(1 / weight)
		}
		if allFine {
			qr.Tier = "fine"
		} else {
			qr.Tier = "coarse"
			for i := range qr.Neighbors {
				qr.Neighbors[i].Fine = false
			}
		}
		out = append(out, qr)
	}
	return out
}

// ProbeIndex converts grid coordinates to the canonical linear probe index.
func (g Grid) ProbeIndex(x, y, z int) int { return g.index(x, y, z) }
