package domain

import "math"

func intersects(p, q Vec, b Box) bool {
	dir := q.sub(p)
	near, far := 0.0, 1.0
	for axis := range 3 {
		if dir[axis] == 0 {
			if p[axis] < b.Min[axis] || p[axis] > b.Max[axis] {
				return false
			}
			continue
		}
		lo := (b.Min[axis] - p[axis]) / dir[axis]
		hi := (b.Max[axis] - p[axis]) / dir[axis]
		if lo > hi {
			lo, hi = hi, lo
		}
		near = math.Max(near, lo)
		far = math.Min(far, hi)
		if near > far {
			return false
		}
	}
	return true
}
