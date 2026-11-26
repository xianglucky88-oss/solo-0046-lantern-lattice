package domain

import (
	"errors"
	"math"
)

// Sample fetches one face and depth layer at in-plane fractional coordinates
// u, v in [0,1]. Only the logical bilinear footprint of that face and layer is
// used; pages and fragment ordering are irrelevant. Encoded RGB is treated as
// linear texture channels. Opaque pixels contribute to the weight (including
// valid black), transparent pixels do not. A zero total weight reports the
// sample as unavailable.
func (f *Frame) Sample(face, depth int, u, v float64) (SampleResult, error) {
	if face < 0 || face >= 6 {
		return SampleResult{}, errors.New("face must be in 0–5")
	}
	if depth < 0 || depth >= f.Grid.Size[2] {
		return SampleResult{}, errors.New("depth layer outside the grid")
	}
	for _, q := range [2]float64{u, v} {
		if math.IsNaN(q) || q < 0 || q > 1 {
			return SampleResult{}, errors.New("in-plane coordinates must be in [0,1]")
		}
	}
	fx := u * float64(f.Grid.Size[0]-1)
	fy := v * float64(f.Grid.Size[1]-1)
	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	x1 := min(x0+1, f.Grid.Size[0]-1)
	y1 := min(y0+1, f.Grid.Size[1]-1)
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	ox := face * f.Grid.Size[0]
	oy := depth * f.Grid.Size[1]

	type corner struct{ x, y int }
	corners := [4]corner{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}}
	weights := [4]float64{(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty}

	var out SampleResult
	for i, c := range corners {
		px := f.logical.NRGBAAt(ox+c.x, oy+c.y)
		if px.A == 0 {
			continue
		}
		w := weights[i]
		out.Weight += w
		out.R += w * float64(px.R)
		out.G += w * float64(px.G)
		out.B += w * float64(px.B)
	}
	if out.Weight == 0 {
		return SampleResult{}, nil
	}
	out.Available = true
	out.R /= out.Weight
	out.G /= out.Weight
	out.B /= out.Weight
	return out, nil
}
