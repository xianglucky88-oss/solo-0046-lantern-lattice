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
	if r.Animation != nil {
		if err := validateAnimation(*r.Animation); err != nil {
			return err
		}
	}
	return nil
}

func validateAnimation(a Animation) error {
	t0, t1 := a.Range[0], a.Range[1]
	if math.IsNaN(t0) || math.IsNaN(t1) || !(t1 > t0) {
		return fmt.Errorf("animation range must span a positive duration")
	}
	if !bounded(t0, -1e5, 1e5) || !bounded(t1, -1e5, 1e5) {
		return fmt.Errorf("animation range outside time bounds")
	}
	if a.Shutter.Start < t0 || a.Shutter.End > t1 || !(a.Shutter.End > a.Shutter.Start) {
		return fmt.Errorf("shutter must be a positive interval inside the animation range")
	}
	if len(a.Movers) == 0 || len(a.Movers) > 8 {
		return fmt.Errorf("an animated scene needs 1–8 moving boxes")
	}
	for i, m := range a.Movers {
		for axis, size := range m.Size {
			if !bounded(size, 0.01, 2048) {
				return fmt.Errorf("mover %d has no volume on axis %d", i, axis)
			}
		}
		if len(m.Keys) < 2 || len(m.Keys) > 32 {
			return fmt.Errorf("mover %d needs 2–32 key times", i)
		}
		for k, key := range m.Keys {
			if !bounded(key.Time, -1e5, 1e5) || !spatial(key.Position) {
				return fmt.Errorf("mover %d key %d invalid", i, k)
			}
			corner := key.Position.add(m.Size)
			if !spatial(corner) {
				return fmt.Errorf("mover %d key %d extends beyond scene bounds", i, k)
			}
			if k > 0 && !(key.Time > m.Keys[k-1].Time) {
				return fmt.Errorf("mover %d key times must be strictly increasing", i)
			}
		}
		if m.Keys[0].Time != t0 || m.Keys[len(m.Keys)-1].Time != t1 {
			return fmt.Errorf("mover %d keys must cover the whole animation range", i)
		}
	}
	return nil
}
