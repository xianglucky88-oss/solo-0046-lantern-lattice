package domain

import "testing"

// TestMultipleCandidatesRequired reproduces the two-checkpoint scenario where
// no single relocated probe is enough: both same-side slots must activate for
// each checkpoint to reach its shared minimum support.
func TestMultipleCandidatesRequired(t *testing.T) {
	g := baseGrid()
	wall := thinWall()
	// Tight boxes around red-row slots (2,1,0) and (3,1,0), i.e. (2,0.5,0)
	// and (3,0.5,0). Candidates move into the gap between the boxes so the
	// central checkpoints see both without any ray crossing a solid box.
	solidA := Box{Min: Vec{1.95, 0.45, -0.05}, Max: Vec{2.05, 0.55, 0.05}}
	solidB := Box{Min: Vec{2.95, 0.45, -0.05}, Max: Vec{3.05, 0.55, 0.05}}
	r := Request{
		Grid:     g,
		Lights:   sideLights(),
		Blocks:   []Box{wall, solidA, solidB},
		Exposure: 3,
		Rescue: &Rescue{
			MaxMove: 1,
			Slots: []RescueSlot{
				{X: 2, Y: 1, Z: 0, Candidates: []Vec{{2.2, 0.5, 0}}},
				{X: 3, Y: 1, Z: 0, Candidates: []Vec{{2.8, 0.5, 0}}},
			},
			Checkpoints: []Checkpoint{
				{Position: Vec{2.4, 0.5, 0}, MinSupport: 0.7},
				{Position: Vec{2.6, 0.5, 0}, MinSupport: 0.7},
			},
		},
	}
	if err := validate(r); err != nil {
		t.Fatalf("fixture should be legal: %v", err)
	}
	probes := bake(r)
	plan := planRescue(r, probes)
	active := activeOf(plan)

	// With everything left invalid, neither checkpoint reaches 0.7.
	if got := plan.countPassing(active, []int{0, 0}); got != 0 {
		t.Fatalf("expected zero passing without rescue, got %d", got)
	}
	// One relocated probe alone still cannot satisfy either 0.7 threshold.
	if got := plan.countPassing(active, []int{1, 0}); got != 0 {
		t.Fatalf("single relocation must not pass a checkpoint, got %d", got)
	}
	if got := plan.countPassing(active, []int{0, 1}); got != 0 {
		t.Fatalf("single relocation must not pass a checkpoint, got %d", got)
	}
	// Joint activation passes both (corner weights sum to 1, factor 0.8).
	if got := plan.countPassing(active, []int{1, 1}); got != 2 {
		t.Fatalf("joint relocation should pass both, got %d", got)
	}
	// The optimizer must therefore choose to move both slots.
	choice := plan.optimize()
	if choice[0] != 1 || choice[1] != 1 {
		t.Fatalf("optimizer should activate both candidates, got %v", choice)
	}
	plan.apply(r, probes, choice)

	// Rays toward the blue corridor remain blocked by the thin wall, so the
	// rescued red-side samples never read the other side's color.
	for _, cp := range r.Rescue.Checkpoints {
		got := sample(g, probes, cp.Position, r.Blocks)
		if got.Weight < cp.MinSupport {
			t.Fatalf("checkpoint %v support %v below %v", cp.Position, got.Weight, cp.MinSupport)
		}
		for _, lobe := range got.Irradiance {
			if lobe[2] > 1e-12 {
				t.Fatalf("opposite-side light leaked through wall: %v", lobe)
			}
		}
	}
}

func activeOf(plan rescuePlan) []int {
	out := make([]int, 0, len(plan.checks))
	for i, cp := range plan.checks {
		if cp.state == "active" {
			out = append(out, i)
		}
	}
	return out
}
