package domain

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func baseGrid() Grid {
	return Grid{Origin: Vec{0, -0.5, 0}, Step: 1, Size: [3]int{4, 2, 2}}
}

// thinWall splits the volume into a red (y above the wall) and blue (y below)
// corridor. It sits between the two grid rows (y=-0.5 and y=0.5), so rays
// across it are blocked even though those probes are grid-adjacent.
func thinWall() Box {
	return Box{Min: Vec{-0.5, 0.24, -0.5}, Max: Vec{3.5, 0.26, 1.5}}
}

// smallSolid surrounds grid slot (3,1,0) = (3,0.5,0) in the red corridor
// without touching the wall.
func smallSolid() Box {
	return Box{Min: Vec{2.9, 0.45, -0.05}, Max: Vec{3.1, 0.55, 0.05}}
}

func sideLights() []Light {
	return []Light{
		{Position: Vec{1.5, 2, 0.5}, RGB: Vec{1, 0, 0}, Radius: 8, Power: 4},
		{Position: Vec{1.5, -2, 0.5}, RGB: Vec{0, 0, 1}, Radius: 8, Power: 4},
	}
}

// TestThinWallNoLeakage proves a checkpoint in the red corridor cannot read
// blue light from grid-adjacent probes on the other side of the wall.
func TestThinWallNoLeakage(t *testing.T) {
	r := Request{
		Grid:     baseGrid(),
		Lights:   sideLights(),
		Blocks:   []Box{thinWall()},
		Exposure: 3,
	}
	probes := bake(r)
	// Between the wall top (0.26) and the red row (0.5): the blue row is a
	// trilinear neighbor but every ray to it crosses the wall.
	cp := Vec{1.5, 0.4, 0.5}
	got := sample(r.Grid, probes, cp, r.Blocks)
	if got.State != "sampled" {
		t.Fatalf("expected sampled, got %s", got.State)
	}
	red := 0.0
	for _, lobe := range got.Irradiance {
		if lobe[2] > 1e-12 {
			t.Fatalf("blue channel leaked through thin wall: %v", lobe)
		}
		red = math.Max(red, lobe[0])
	}
	if red <= 0 {
		t.Fatalf("expected red light in red corridor, got %v", got.Irradiance)
	}
	// The y fraction of the red row is 0.9; the occluded blue row contributes
	// nothing even though its grid slots are direct trilinear neighbors.
	if !approx(got.Weight, 0.9) {
		t.Fatalf("support should exclude occluded side, got %v want 0.9", got.Weight)
	}
}

// TestRescueFactorAndSupport checks the linear move factor and that a moved
// probe restores support using the reduced weight.
func TestRescueFactorAndSupport(t *testing.T) {
	r := Request{
		Grid:     baseGrid(),
		Lights:   sideLights(),
		Blocks:   []Box{thinWall(), smallSolid()},
		Exposure: 3,
		Rescue: &Rescue{
			MaxMove: 1,
			Slots: []RescueSlot{{
				X: 3, Y: 1, Z: 0,
				Candidates: []Vec{{2.6, 0.5, 0}},
			}},
			// 0.6 is unreachable without the rescue: neighbor (2,1,0) alone
			// carries only 0.15 of the trilinear weight at this checkpoint.
			Checkpoints: []Checkpoint{{Position: Vec{2.85, 0.5, 0}, MinSupport: 0.6}},
		},
	}
	probes := bake(r)
	if probes[r.Grid.index(3, 1, 0)].Valid {
		t.Fatal("slot should start invalid inside solid box")
	}
	plan := planRescue(r, probes)
	move := plan.options[0][1].move
	if !approx(move, 0.4) {
		t.Fatalf("move distance = %v want 0.4", move)
	}
	wantFactor := 1 - 0.4
	if !approx(plan.options[0][1].factor, wantFactor) {
		t.Fatalf("factor = %v want %v", plan.options[0][1].factor, wantFactor)
	}
	choice := plan.optimize()
	if choice[0] != 1 {
		t.Fatalf("optimizer should adopt the candidate, choice=%v", choice)
	}
	plan.apply(r, probes, choice)
	moved := probes[r.Grid.index(3, 1, 0)]
	if !moved.Valid || moved.Position != (Vec{2.6, 0.5, 0}) {
		t.Fatalf("probe not relocated: %+v", moved)
	}
	// 0.15 from the untouched neighbor plus 0.85*0.6 from the rescued probe.
	got := sample(r.Grid, probes, Vec{2.85, 0.5, 0}, r.Blocks)
	if !approx(got.Weight, 0.15+0.85*wantFactor) {
		t.Fatalf("rescued support = %v want %v", got.Weight, 0.15+0.85*wantFactor)
	}
	red := 0.0
	for _, lobe := range got.Irradiance {
		if !approx(lobe[2], 0) {
			t.Fatalf("rescued corridor probe must stay blue-free: %v", lobe)
		}
		red = math.Max(red, lobe[0])
	}
	if red <= 0 {
		t.Fatalf("rescued corridor probe should carry red light: %v", got.Irradiance)
	}
}

// TestOptimizerTieBreak verifies maximum passing count first, then shortest
// total move, and determinism across repeated runs.
func TestOptimizerTieBreak(t *testing.T) {
	r := Request{
		Grid:     baseGrid(),
		Lights:   sideLights(),
		Blocks:   []Box{thinWall(), smallSolid()},
		Exposure: 3,
		Rescue: &Rescue{
			MaxMove: 1,
			Slots: []RescueSlot{{
				X: 3, Y: 1, Z: 0,
				// The checkpoint never neighbors this slot, so every option
				// yields the same pass count; shortest total move must win.
				Candidates: []Vec{{2.8, 0.5, 0}, {2.5, 0.5, 0}},
			}},
			Checkpoints: []Checkpoint{{Position: Vec{1.5, 0.5, 0.5}, MinSupport: 0}},
		},
	}
	probes := bake(r)
	plan := planRescue(r, probes)
	first := plan.optimize()
	for range 5 {
		if got := plan.optimize(); !equalInts(got, first) {
			t.Fatalf("non-deterministic choice: %v vs %v", got, first)
		}
	}
	// Option 0 (stay invalid) already passes; it has move 0 and must win.
	if first[0] != 0 {
		t.Fatalf("expected no relocation on tie, got option %d", first[0])
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRescueValidation exercises the domain rejection boundaries.
func TestRescueValidation(t *testing.T) {
	solidAt := Box{Min: Vec{0.9, 0.45, -0.05}, Max: Vec{1.1, 0.55, 0.05}}
	good := func() Request {
		return Request{
			Grid:     baseGrid(),
			Lights:   sideLights(),
			Blocks:   []Box{thinWall(), solidAt},
			Exposure: 3,
			Rescue: &Rescue{
				MaxMove: 1,
				Slots: []RescueSlot{{
					X: 1, Y: 1, Z: 0,
					Candidates: []Vec{{1.2, 0.5, 0}},
				}},
				Checkpoints: []Checkpoint{{Position: Vec{1, 0.5, 0}, MinSupport: 0.5}},
			},
		}
	}
	// Candidate beyond max move.
	r := good()
	r.Rescue.Slots[0].Candidates[0] = Vec{2.5, 0.5, 0}
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for candidate beyond max move")
	}
	// Candidate inside a solid box (the thin wall).
	r = good()
	r.Rescue.Slots[0].Candidates[0] = Vec{1, 0.25, 0}
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for candidate inside wall")
	}
	// Slot already valid (remove the box that blocks it).
	r = good()
	r.Blocks = []Box{thinWall()}
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for rescuing a valid slot")
	}
	// Candidate outside the grid extent.
	r = good()
	r.Rescue.Slots[0].Candidates[0] = Vec{1, 0.5, 5}
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for candidate outside grid")
	}
	// More than eight slots.
	r = good()
	r.Rescue.Slots = make([]RescueSlot, 9)
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for more than eight slots")
	}
	// Duplicate slot entry.
	r = good()
	r.Rescue.Slots = []RescueSlot{
		{X: 1, Y: 1, Z: 0, Candidates: []Vec{{1.2, 0.5, 0}}},
		{X: 1, Y: 1, Z: 0, Candidates: []Vec{{1.1, 0.5, 0}}},
	}
	if err := validate(r); err == nil {
		t.Fatal("expected rejection for duplicated slot")
	}
}

// TestRescueEndToEndBuild runs the full Build path and checks the report and
// that texture addressing still follows the original slot index.
func TestRescueEndToEndBuild(t *testing.T) {
	r := Request{
		Grid:     baseGrid(),
		Lights:   sideLights(),
		Blocks:   []Box{thinWall(), smallSolid()},
		Exposure: 3,
		Rescue: &Rescue{
			MaxMove: 1,
			Slots: []RescueSlot{{
				X: 3, Y: 1, Z: 0,
				Candidates: []Vec{{2.6, 0.5, 0}},
			}},
			Checkpoints: []Checkpoint{
				{Position: Vec{2.85, 0.5, 0}, MinSupport: 0.6},
				{Position: Vec{0, 0.25, 0}, MinSupport: 0.5}, // inside the wall
				{Position: Vec{9, 0, 0}, MinSupport: 0.5},    // outside the grid
			},
		},
	}
	raw, err := Build(r)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	layout := readLayout(t, raw)
	if layout.Rescue == nil {
		t.Fatal("metadata missing rescue report")
	}
	if len(layout.Rescue.Slots) != 1 || !layout.Rescue.Slots[0].Moved {
		t.Fatalf("unexpected slot report: %+v", layout.Rescue.Slots)
	}
	states := map[string]bool{}
	for _, cp := range layout.Rescue.Checkpoints {
		states[cp.State] = true
		switch cp.State {
		case "active":
			if cp.Linear == nil || !cp.Pass {
				t.Fatalf("active checkpoint should pass with linear color: %+v", cp)
			}
		case "solid", "outside":
			if cp.Pass || cp.Linear != nil {
				t.Fatalf("wall/outside checkpoint must keep original state: %+v", cp)
			}
		}
	}
	if !states["solid"] || !states["outside"] || !states["active"] {
		t.Fatalf("missing checkpoint states: %v", states)
	}
	// Texture still addresses the original slot; the rescued pixel is opaque.
	if !pixelOpaque(t, raw, baseGrid(), 3, 1, 0) {
		t.Fatal("rescued slot texture pixel should be opaque at original address")
	}
}
