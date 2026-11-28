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

func validateGrid(i int, g Grid) error {
	if !spatial(g.Origin) || !bounded(g.Step, 0.01, 2048) {
		return fmt.Errorf("invalid grid %d origin or spacing", i)
	}
	for _, n := range g.Size {
		if n < 2 || n > 16 {
			return fmt.Errorf("grid %d axes must have 2–16 probes", i)
		}
	}
	if !spatial(g.position(g.Size[0]-1, g.Size[1]-1, g.Size[2]-1)) {
		return fmt.Errorf("grid %d spans beyond scene bounds", i)
	}
	return nil
}

func validateWall(w Wall) error {
	if _, ok := newWallBasis(w); !ok {
		return fmt.Errorf("wall needs a non-zero normal and an up vector off that normal")
	}
	if !spatial(w.Center) {
		return fmt.Errorf("wall centre outside scene")
	}
	for a := range 2 {
		if !bounded(w.Half[a], 1e-6, 1e5) {
			return fmt.Errorf("wall half extents must be positive")
		}
		if !(w.Opening.Min[a] < w.Opening.Max[a]) {
			return fmt.Errorf("wall opening has no area")
		}
		if w.Opening.Min[a] < -w.Half[a] || w.Opening.Max[a] > w.Half[a] {
			return fmt.Errorf("wall opening must lie inside the wall rectangle")
		}
	}
	return nil
}

// validate accepts either the legacy single-grid form (Grid, no Wall) or the
// cross-door two-grid form (two Grids on opposite sides plus a Wall).
func validate(r Request) error {
	if r.Grid != nil {
		if len(r.Grids) != 0 || r.Wall != nil {
			return fmt.Errorf("use either a single Grid or two Grids with a Wall, not both")
		}
		if err := validateGrid(0, *r.Grid); err != nil {
			return err
		}
	} else {
		if len(r.Grids) != 2 {
			return fmt.Errorf("a cross-door bake needs exactly two grids, one per wall side")
		}
		if r.Wall == nil {
			return fmt.Errorf("two grids require a wall")
		}
		if r.Grids[0].Side != -1 && r.Grids[0].Side != 1 {
			return fmt.Errorf("grid 0 side must be -1 or +1")
		}
		if r.Grids[0].Side == r.Grids[1].Side {
			return fmt.Errorf("the two grids must sit on opposite wall sides")
		}
		for i, g := range r.Grids {
			if err := validateGrid(i, g); err != nil {
				return err
			}
		}
		if err := validateWall(*r.Wall); err != nil {
			return err
		}
		if !bounded(r.Transition, 0.01, 1e5) {
			return fmt.Errorf("transition width must be positive")
		}
	}
	if !bounded(r.Exposure, 0.01, 64) {
		return fmt.Errorf("invalid exposure")
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
	return nil
}
