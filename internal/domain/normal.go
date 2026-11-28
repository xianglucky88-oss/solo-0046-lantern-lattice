package domain

import "math"

// unitNormal returns the normalized direction of n. A zero-length (or
// non-finite) normal means "no receiver orientation": callers then fall back
// to the historical six-direction result.
func unitNormal(n Vec) (Vec, bool) {
	l := n.length()
	if !(l > 0) || math.IsInf(l, 0) || math.IsNaN(l) {
		return Vec{}, false
	}
	return n.scale(1 / l), true
}

// projectNormal accumulates one incident beam, already weighted by occlusion
// and distance attenuation, into the linear RGB response of a receiver facing
// unit normal n. The beam contributes only inside n's hemisphere and only in
// proportion to the cosine between its incoming direction and n. A light
// exactly at the probe has no direction; it then contributes to every normal
// equally, matching the legacy coincidence rule.
func (out *Vec) projectNormal(normal, ray Vec, distance float64, rgb Vec) {
	weight := 1.0
	if distance > 0 {
		weight = math.Max(0, ray.dot(normal)/distance)
	}
	if weight <= 0 {
		return
	}
	*out = out.add(rgb.scale(weight))
}
