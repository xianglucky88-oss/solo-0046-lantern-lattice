package domain

import "math"

// endpointEpsilon is the segment parameter tolerance used to recognize an
// isolated closed box contact exactly at an exempt endpoint.
const endpointEpsilon = 1e-9

// segmentClear checks every closed block along p->q. Unlike visible, the two
// endpoints may each touch the single reflection box face they belong to;
// every other contact, including travel through a box interior, blocks.
func segmentClear(p, q Vec, blocks []Box, ap, aq *anchor) bool {
	for bi, b := range blocks {
		if segmentBlockedBy(p, q, b, bi, ap, aq) {
			return false
		}
	}
	return true
}

func segmentBlockedBy(p, q Vec, b Box, bi int, ap, aq *anchor) bool {
	d := q.sub(p)
	lo, hi := 0.0, 1.0
	for axis := range 3 {
		if d[axis] == 0 {
			if p[axis] < b.Min[axis] || p[axis] > b.Max[axis] {
				return false
			}
			continue
		}
		tNear := (b.Min[axis] - p[axis]) / d[axis]
		tFar := (b.Max[axis] - p[axis]) / d[axis]
		if tNear > tFar {
			tNear, tFar = tFar, tNear
		}
		lo = math.Max(lo, tNear)
		hi = math.Min(hi, tFar)
		if lo > hi {
			return false
		}
	}
	// Closed interval hit [lo, hi] inside [0,1]. Recognize hits confined to
	// one endpoint; anything spanning the interior blocks.
	if hi <= endpointEpsilon {
		return !anchorAllows(ap, bi)
	}
	if lo >= 1-endpointEpsilon {
		return !anchorAllows(aq, bi)
	}
	return true
}

func anchorAllows(a *anchor, boxIndex int) bool {
	return a != nil && a.box == boxIndex
}
