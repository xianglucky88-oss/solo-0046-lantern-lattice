package domain

import (
	"math"
	"slices"
)

// affine is a(t) = C + V*t on a key interval.
type affine struct{ C, V float64 }

// moverInterval describes one mover on one of its key intervals.
type moverInterval struct {
	t0, t1 float64
	min    [3]affine
	half   [3]affine // max corner, i.e. Min + fixed Size
}

func moverIntervals(m Mover) []moverInterval {
	out := make([]moverInterval, 0, len(m.Keys)-1)
	for k := 0; k+1 < len(m.Keys); k++ {
		a, b := m.Keys[k], m.Keys[k+1]
		span := b.Time - a.Time
		var iv moverInterval
		iv.t0, iv.t1 = a.Time, b.Time
		for axis := range 3 {
			v := (b.Position[axis] - a.Position[axis]) / span
			iv.min[axis] = affine{C: a.Position[axis] - v*a.Time, V: v}
			iv.half[axis] = affine{C: a.Position[axis] + m.Size[axis] - v*a.Time, V: v}
		}
		out = append(out, iv)
	}
	return out
}

// boxAt evaluates the moving box at time t using the constant-speed
// interpolation between its bracketing keys.
func boxAt(intervals []moverInterval, t float64) Box {
	iv := intervals[len(intervals)-1]
	for _, candidate := range intervals {
		if t >= candidate.t0 && t <= candidate.t1 {
			iv = candidate
			break
		}
	}
	var b Box
	for axis := range 3 {
		b.Min[axis] = iv.min[axis].eval(t)
		b.Max[axis] = iv.half[axis].eval(t)
	}
	return b
}

func (f affine) eval(t float64) float64 { return f.C + f.V*t }

// root returns the time solving f(t) == g(t), if it is a single finite time.
func (f affine) root(g affine) (float64, bool) {
	den := g.V - f.V
	if den == 0 {
		return 0, false
	}
	t := (f.C - g.C) / den
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return 0, false
	}
	return t, true
}

func clampTime(t, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, t)) }

// pointEventTimes collects every time where p could enter or leave a moving
// box inside [lo, hi]. Between collected times the inside/outside state is
// constant, so isolated boundary contacts alone never gain duration.
func pointEventTimes(p Vec, intervals []moverInterval, lo, hi float64) []float64 {
	var out []float64
	for _, iv := range intervals {
		if iv.t1 <= lo || iv.t0 >= hi {
			continue
		}
		segLo, segHi := math.Max(lo, iv.t0), math.Min(hi, iv.t1)
		for axis := range 3 {
			constant := affine{C: p[axis]}
			for _, edge := range [2]affine{iv.min[axis], iv.half[axis]} {
				if t, ok := constant.root(edge); ok && t > segLo && t < segHi {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

// segmentEventTimes collects every time where the closed segment p->q could
// start or stop intersecting a moving box. Tangent crossings are included, so
// occlusion intervals keep their exact length and tangent-only contacts stay
// isolated points without duration.
func segmentEventTimes(p, q Vec, intervals []moverInterval, lo, hi float64) []float64 {
	var out []float64
	dir := q.sub(p)
	for _, iv := range intervals {
		if iv.t1 <= lo || iv.t0 >= hi {
			continue
		}
		segLo, segHi := math.Max(lo, iv.t0), math.Min(hi, iv.t1)
		for axis := range 3 {
			if dir[axis] == 0 {
				// Segment coordinate is fixed on this axis; its face
				// crossings are the only state-change candidates.
				constant := affine{C: p[axis]}
				for _, edge := range [2]affine{iv.min[axis], iv.half[axis]} {
					if t, ok := constant.root(edge); ok && t > segLo && t < segHi {
						out = append(out, t)
					}
				}
				continue
			}
			// Parametric segment coordinate p[axis] + u*dir[axis]; slab
			// entry/exit fractions are affine in time.
			bounds := [2]affine{
				{C: (iv.min[axis].C - p[axis]) / dir[axis], V: iv.min[axis].V / dir[axis]},
				{C: (iv.half[axis].C - p[axis]) / dir[axis], V: iv.half[axis].V / dir[axis]},
			}
			var lines []affine
			lines = append(lines, bounds[:]...)
			lines = append(lines, affine{}, affine{C: 1}) // u = 0 and u = 1
			for i := 0; i < len(lines); i++ {
				for j := i + 1; j < len(lines); j++ {
					if t, ok := lines[i].root(lines[j]); ok && t > segLo && t < segHi {
						out = append(out, t)
					}
				}
			}
		}
	}
	return out
}

// uniqueTimes merges event times, clamping them into [lo, hi] and folding
// times that coincide within floating-point tolerance.
func uniqueTimes(values []float64, lo, hi float64) []float64 {
	for i := range values {
		values[i] = clampTime(values[i], lo, hi)
	}
	slices.Sort(values)
	out := values[:0]
	epsilon := math.Max(1e-12, (hi-lo)*1e-12)
	for _, t := range values {
		if len(out) == 0 || t-out[len(out)-1] > epsilon {
			out = append(out, t)
		}
	}
	return out
}

func motionBoxesAt(intervals [][]moverInterval, t float64) []Box {
	out := make([]Box, 0, len(intervals))
	for _, iv := range intervals {
		out = append(out, boxAt(iv, t))
	}
	return out
}

// occupiedAt reports whether p is inside any static or moving closed box.
func occupiedAt(p Vec, static []Box, intervals [][]moverInterval, t float64) bool {
	if occupied(p, static) {
		return true
	}
	for _, iv := range intervals {
		if inside(p, boxAt(iv, t)) {
			return true
		}
	}
	return false
}

// visibleAt applies the same closed-box, tangent-blocking ray rule as the
// static bake, with every moving box evaluated at t.
func visibleAt(p, q Vec, static []Box, intervals [][]moverInterval, t float64) bool {
	if !visible(p, q, static) {
		return false
	}
	for _, iv := range intervals {
		if intersects(p, q, boxAt(iv, t)) {
			return false
		}
	}
	return true
}
