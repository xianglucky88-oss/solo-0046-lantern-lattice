package domain

import "math"

func (out *Lobes) project(ray Vec, distance float64, rgb Vec) {
	for i, axis := range axes {
		weight := 1.0
		if distance > 0 {
			weight = math.Max(0, ray.dot(axis)/distance)
		}
		out[i] = out[i].add(rgb.scale(weight))
	}
}
