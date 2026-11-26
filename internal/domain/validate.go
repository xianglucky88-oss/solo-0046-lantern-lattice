package domain

import (
	"fmt"
	"math"
)

func bounded(x, lo, hi float64) bool { return !math.IsNaN(x) && x >= lo && x <= hi }
func spatial(p Vec) bool {
	for _, x := range p {
		if !bounded(x, -1e5, 1e5) {
			return false
		}
	}
	return true
}
func validate(r Request) error {
	if !spatial(r.Grid.Origin) || !bounded(r.Grid.Step, 0.01, 2048) || !bounded(r.Exposure, 0.01, 64) {
		return fmt.Errorf("invalid origin, spacing or exposure")
	}
	for _, n := range r.Grid.Size {
		if n < 2 || n > 16 {
			return fmt.Errorf("grid axes must have 2–16 probes")
		}
	}
	if !spatial(r.Grid.position(r.Grid.Size[0]-1, r.Grid.Size[1]-1, r.Grid.Size[2]-1)) {
		return fmt.Errorf("grid spans beyond scene bounds")
	}
	if len(r.Lights) > 32 || len(r.Blocks) > 64 || len(r.Samples) > 128 {
		return fmt.Errorf("scene exceeds light, blocker or query limits")
	}
	for i, l := range r.Lights {
		if !spatial(l.Position) || !bounded(l.Radius, 0.01, 1e5) || !bounded(l.Power, 0, 64) {
			return fmt.Errorf("invalid light %d", i)
		}
		for c, v := range l.RGB {
			if !bounded(v, 0, 1) {
				return fmt.Errorf("light %d channel %d outside linear RGB", i, c)
			}
		}
	}
	for i, b := range r.Blocks {
		if !spatial(b.Min) || !spatial(b.Max) {
			return fmt.Errorf("block %d outside scene", i)
		}
		for a := range 3 {
			if b.Min[a] >= b.Max[a] {
				return fmt.Errorf("block %d has no volume", i)
			}
		}
	}
	for i, p := range r.Samples {
		if !spatial(p) {
			return fmt.Errorf("query %d invalid", i)
		}
	}
	if err := resolveFaces(&r); err != nil {
		return err
	}
	return nil
}
