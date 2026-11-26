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
	if r.Rescue != nil {
		if err := validateRescue(r); err != nil {
			return err
		}
	}
	return nil
}

func validateRescue(r Request) error {
	resc := r.Rescue
	if !bounded(resc.MaxMove, 0.01, 2048) {
		return fmt.Errorf("rescue max move out of range")
	}
	if len(resc.Slots) == 0 || len(resc.Slots) > 8 {
		return fmt.Errorf("rescue allows 1–8 slots")
	}
	if len(resc.Checkpoints) == 0 || len(resc.Checkpoints) > 128 {
		return fmt.Errorf("rescue allows 1–128 checkpoints")
	}
	g := r.Grid
	seenSlots := make(map[int]bool)
	for i, slot := range resc.Slots {
		if slot.X < 0 || slot.X >= g.Size[0] || slot.Y < 0 || slot.Y >= g.Size[1] || slot.Z < 0 || slot.Z >= g.Size[2] {
			return fmt.Errorf("rescue slot %d outside grid", i)
		}
		idx := g.index(slot.X, slot.Y, slot.Z)
		if seenSlots[idx] {
			return fmt.Errorf("rescue slot %d listed twice", i)
		}
		seenSlots[idx] = true
		if !occupied(g.position(slot.X, slot.Y, slot.Z), r.Blocks) {
			return fmt.Errorf("rescue slot %d is already valid", i)
		}
		if len(slot.Candidates) == 0 || len(slot.Candidates) > 4 {
			return fmt.Errorf("rescue slot %d allows 1–4 candidates", i)
		}
		center := g.position(slot.X, slot.Y, slot.Z)
		seenPoints := make(map[Vec]bool)
		for j, cand := range slot.Candidates {
			if !spatial(cand) {
				return fmt.Errorf("rescue slot %d candidate %d invalid", i, j)
			}
			if !inGridBounds(g, cand) {
				return fmt.Errorf("rescue slot %d candidate %d outside grid", i, j)
			}
			if cand.sub(center).length() > resc.MaxMove+1e-9 {
				return fmt.Errorf("rescue slot %d candidate %d beyond max move", i, j)
			}
			if occupied(cand, r.Blocks) {
				return fmt.Errorf("rescue slot %d candidate %d inside solid box", i, j)
			}
			if seenPoints[cand] {
				return fmt.Errorf("rescue slot %d candidate %d duplicated", i, j)
			}
			seenPoints[cand] = true
		}
	}
	for i, cp := range resc.Checkpoints {
		if !spatial(cp.Position) {
			return fmt.Errorf("checkpoint %d invalid", i)
		}
		if !bounded(cp.MinSupport, 0, 1) {
			return fmt.Errorf("checkpoint %d minimum support out of range", i)
		}
	}
	return nil
}

// inGridBounds reports whether p lies within the axis-aligned extent of the
// original grid, including its boundary.
func inGridBounds(g Grid, p Vec) bool {
	for a := range 3 {
		lo := g.Origin[a]
		hi := lo + float64(g.Size[a]-1)*g.Step
		if lo > hi {
			lo, hi = hi, lo
		}
		if p[a] < lo-1e-9 || p[a] > hi+1e-9 {
			return false
		}
	}
	return true
}
