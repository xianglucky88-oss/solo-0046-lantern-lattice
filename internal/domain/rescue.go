package domain

import "math"

// slotOption is one legal choice for a rescue slot: leave the probe invalid
// (option 0) or move it to a baked candidate landing point.
type slotOption struct {
	position Vec
	move     float64
	factor   float64
	lobe     Lobes
}

type cpPlan struct {
	position   Vec
	minSupport float64
	state      string // "active", "outside" or "solid"
	base       float64
	// deltas[slot][option] is the support change versus leaving every slot
	// invalid; option 0 (stay invalid) is always zero.
	deltas [][]float64
}

type rescuePlan struct {
	options [][]slotOption
	checks  []cpPlan
}

// planRescue validates the geometry already passed by validate, bakes each
// legal landing point, and precomputes how every slot choice changes the
// reduced support of every active checkpoint.
func planRescue(r Request, probes []Probe) rescuePlan {
	g := r.Grid
	resc := r.Rescue
	plan := rescuePlan{
		options: make([][]slotOption, len(resc.Slots)),
		checks:  make([]cpPlan, len(resc.Checkpoints)),
	}
	for i, slot := range resc.Slots {
		center := g.position(slot.X, slot.Y, slot.Z)
		opts := []slotOption{{}} // option 0: remain invalid
		for _, cand := range slot.Candidates {
			move := cand.sub(center).length()
			factor := math.Max(0, 1-move/resc.MaxMove)
			lobe, _, _ := incoming(cand, r.Lights, r.Blocks)
			opts = append(opts, slotOption{position: cand, move: move, factor: factor, lobe: lobe})
		}
		plan.options[i] = opts
	}
	for i, cp := range resc.Checkpoints {
		plan.checks[i] = buildCheckpoint(g, probes, r.Blocks, cp, resc.Slots, plan.options)
	}
	return plan
}

func buildCheckpoint(g Grid, probes []Probe, blocks []Box, cp Checkpoint, slots []RescueSlot, options [][]slotOption) cpPlan {
	out := cpPlan{position: cp.Position, minSupport: cp.MinSupport}
	cell, ok := locate(g, cp.Position)
	if !ok {
		out.state = "outside"
		return out
	}
	if occupied(cp.Position, blocks) {
		out.state = "solid"
		return out
	}
	out.state = "active"
	out.base = supportAt(g, probes, cell, cp.Position, blocks)
	out.deltas = make([][]float64, len(slots))
	for s, slot := range slots {
		idx := g.index(slot.X, slot.Y, slot.Z)
		// Trilinear corner weight of this slot at the checkpoint; zero when
		// the slot is not one of the eight surrounding corners.
		corner := cornerWeight(g, cell, idx)
		row := make([]float64, len(options[s]))
		if corner > 0 {
			for o, opt := range options[s] {
				if o == 0 {
					continue // invalid probe contributes nothing
				}
				if visible(cp.Position, opt.position, blocks) {
					row[o] = corner * opt.factor
				}
			}
		}
		out.deltas[s] = row
	}
	return out
}

// supportAt sums the reduced trilinear weights of valid, visible neighbors.
func supportAt(g Grid, probes []Probe, cell [3]span, p Vec, blocks []Box) float64 {
	support := 0.0
	for _, n := range corners(g, cell) {
		probe := probes[n.index]
		if !probe.Valid || !visible(p, probe.Position, blocks) {
			continue
		}
		support += n.weight * probe.Factor
	}
	return support
}

// cornerWeight returns the trilinear interpolation weight of grid index idx
// for the given cell, or zero when idx is not a corner of that cell.
func cornerWeight(g Grid, cell [3]span, idx int) float64 {
	for _, n := range corners(g, cell) {
		if n.index == idx {
			return n.weight
		}
	}
	return 0
}

// optimize enumerates every slot combination in lexicographic order and keeps
// the one with the most checkpoints reaching their minimum support. Ties are
// broken by the smallest total move distance; iteration order makes equal
// inputs select the identical combination.
func (plan rescuePlan) optimize() []int {
	active := make([]int, 0, len(plan.checks))
	for i, cp := range plan.checks {
		if cp.state == "active" {
			active = append(active, i)
		}
	}
	choice := make([]int, len(plan.options))
	best := make([]int, len(plan.options))
	bestPass := -1
	bestMove := math.Inf(1)

	var visit func(slot int, move float64)
	visit = func(slot int, move float64) {
		if slot == len(plan.options) {
			pass := plan.countPassing(active, choice)
			if pass > bestPass || (pass == bestPass && move < bestMove) {
				bestPass = pass
				bestMove = move
				copy(best, choice)
			}
			return
		}
		for o, opt := range plan.options[slot] {
			choice[slot] = o
			visit(slot+1, move+opt.move)
		}
	}
	visit(0, 0)
	return best
}

func (plan rescuePlan) countPassing(active, choice []int) int {
	passing := 0
	for _, ci := range active {
		cp := plan.checks[ci]
		support := cp.base
		for s, o := range choice {
			support += cp.deltas[s][o]
		}
		if support >= cp.minSupport {
			passing++
		}
	}
	return passing
}

// apply moves each selected slot to its chosen landing point.
func (plan rescuePlan) apply(r Request, probes []Probe, choice []int) {
	g := r.Grid
	for s, slot := range r.Rescue.Slots {
		o := choice[s]
		if o == 0 {
			continue
		}
		opt := plan.options[s][o]
		probes[g.index(slot.X, slot.Y, slot.Z)] = Probe{
			Position:   opt.position,
			Valid:      true,
			Factor:     opt.factor,
			Irradiance: opt.lobe,
		}
	}
}
