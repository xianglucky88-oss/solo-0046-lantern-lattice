package domain

import (
	"fmt"
	"math"
)

const maxFaces = 16

// resolveFaces validates face material values and associates every face with
// exactly one face of an existing closed box.
func resolveFaces(r *Request) error {
	if len(r.Faces) > maxFaces {
		return fmt.Errorf("at most %d recolorable faces", maxFaces)
	}
	seen := make(map[string]bool, len(r.Faces))
	for i := range r.Faces {
		f := &r.Faces[i]
		if f.ID == "" {
			return fmt.Errorf("face %d is missing a stable ID", i)
		}
		if seen[f.ID] {
			return fmt.Errorf("duplicate face ID %q", f.ID)
		}
		seen[f.ID] = true
		if !spatial(f.Center) || !spatial(f.Normal) {
			return fmt.Errorf("face %q has invalid center or normal", f.ID)
		}
		nlen := f.Normal.length()
		if !bounded(nlen, 1e-12, 1e12) {
			return fmt.Errorf("face %q normal must be non zero", f.ID)
		}
		n := f.Normal.scale(1 / nlen)
		for c, v := range f.Reflectance {
			if !bounded(v, 0, 1) {
				return fmt.Errorf("face %q channel %d outside linear RGB", f.ID, c)
			}
		}
		axis := 0
		for a := 1; a < 3; a++ {
			if math.Abs(n[a]) > math.Abs(n[axis]) {
				axis = a
			}
		}
		if math.Abs(math.Abs(n[axis])-1) > 1e-6 {
			return fmt.Errorf("face %q normal must be axis aligned", f.ID)
		}
		for a := range 3 {
			if a != axis && math.Abs(n[a]) > 1e-6 {
				return fmt.Errorf("face %q normal must be axis aligned", f.ID)
			}
		}
		side := 1.0
		if n[axis] < 0 {
			side = -1.0
		}
		f.unit = Vec{}
		f.unit[axis] = side

		first, last := 0, len(r.Blocks)
		if f.BoxIndex != nil {
			bi := *f.BoxIndex
			if bi < 0 || bi >= len(r.Blocks) {
				return fmt.Errorf("face %q references unknown block %d", f.ID, bi)
			}
			first, last = bi, bi+1
			if !f.centerSet {
				b := r.Blocks[bi]
				for a := range 3 {
					f.Center[a] = (b.Min[a] + b.Max[a]) * 0.5
				}
				if side > 0 {
					f.Center[axis] = b.Max[axis]
				} else {
					f.Center[axis] = b.Min[axis]
				}
				f.centerSet = true
			}
		}
		match := -1
		for bi := first; bi < last; bi++ {
			if faceOnBox(f.Center, axis, side, r.Blocks[bi]) {
				match = bi
				break
			}
		}
		if match < 0 {
			return fmt.Errorf("face %q is not a face of any block", f.ID)
		}
		f.anchor = &anchor{box: match, axis: axis, side: side}
	}
	return nil
}

func faceTolerance(values ...float64) float64 {
	scale := 1.0
	for _, v := range values {
		scale = math.Max(scale, math.Abs(v))
	}
	return 1e-7 * scale
}

// faceOnBox reports whether center sits on the indicated outward face.
func faceOnBox(c Vec, axis int, side float64, b Box) bool {
	bound := b.Max[axis]
	if side < 0 {
		bound = b.Min[axis]
	}
	eps := faceTolerance(c[axis], bound)
	if math.Abs(c[axis]-bound) > eps {
		return false
	}
	for a := range 3 {
		if a == axis {
			continue
		}
		e := faceTolerance(c[a], b.Min[a], b.Max[a])
		if c[a] < b.Min[a]-e || c[a] > b.Max[a]+e {
			return false
		}
	}
	return true
}
