package domain

// SlotResult records where a rescued grid slot ended up after the bake.
type SlotResult struct {
	X, Y, Z  int
	Moved    bool
	Position Vec     `json:"Position"`
	Move     float64 `json:"Move"`
	Factor   float64 `json:"Factor"`
}

// CheckpointResult reports the policy outcome for one checkpoint. Checkpoints
// inside a wall or outside the grid keep their original state, carry no
// linear color and never count as passing.
type CheckpointResult struct {
	Position   Vec     `json:"Position"`
	State      string  `json:"State"`
	Pass       bool    `json:"Pass"`
	Support    float64 `json:"Support"`
	MinSupport float64 `json:"MinSupport"`
	Linear     *Lobes  `json:"Linear"`
}

// RescueReport is the deterministic record of one relocation bake.
type RescueReport struct {
	MaxMove     float64            `json:"MaxMove"`
	Slots       []SlotResult       `json:"Slots"`
	Checkpoints []CheckpointResult `json:"Checkpoints"`
}

func buildRescueReport(r Request, probes []Probe, plan rescuePlan, choice []int) RescueReport {
	g := r.Grid
	report := RescueReport{MaxMove: r.Rescue.MaxMove}
	report.Slots = make([]SlotResult, 0, len(r.Rescue.Slots))
	for s, slot := range r.Rescue.Slots {
		result := SlotResult{X: slot.X, Y: slot.Y, Z: slot.Z, Moved: choice[s] != 0}
		if result.Moved {
			opt := plan.options[s][choice[s]]
			result.Position = opt.position
			result.Move = opt.move
			result.Factor = opt.factor
		} else {
			result.Position = g.position(slot.X, slot.Y, slot.Z)
		}
		report.Slots = append(report.Slots, result)
	}
	report.Checkpoints = make([]CheckpointResult, 0, len(r.Rescue.Checkpoints))
	for i, cp := range r.Rescue.Checkpoints {
		planned := plan.checks[i]
		result := CheckpointResult{Position: cp.Position, State: planned.state, MinSupport: cp.MinSupport}
		if planned.state != "active" {
			result.Linear = nil
			report.Checkpoints = append(report.Checkpoints, result)
			continue
		}
		s := sample(g, probes, cp.Position, r.Blocks)
		result.Support = s.Weight
		result.Pass = s.Weight >= cp.MinSupport
		lobe := s.Irradiance
		result.Linear = &lobe
		report.Checkpoints = append(report.Checkpoints, result)
	}
	return report
}
