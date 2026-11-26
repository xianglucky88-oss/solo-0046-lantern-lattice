package domain

import (
	"errors"
	"fmt"
)

// StableLight pairs a point light with the stable identity it keeps across
// continued baking revisions.
type StableLight struct {
	ID string
	Light
}

// Revision is a full, versioned scene state. Lights carry stable identifiers
// and keep their declared relative order; newly added lights are appended.
type Revision struct {
	Grid     Grid
	Blocks   []Box
	Samples  []Vec
	Exposure float64
	Lights   []StableLight
}

// RootRevision assigns stable identifiers to the lights of a fresh request in
// list order.
func RootRevision(r Request) Revision {
	lights := make([]StableLight, len(r.Lights))
	for i, l := range r.Lights {
		lights[i] = StableLight{ID: fmt.Sprintf("light-%d", i), Light: l}
	}
	return Revision{Grid: r.Grid, Blocks: r.Blocks, Samples: r.Samples, Exposure: r.Exposure, Lights: lights}
}

// Request renders the revision as an ordinary baking request.
func (rev Revision) Request() Request {
	lights := make([]Light, len(rev.Lights))
	for i, l := range rev.Lights {
		lights[i] = l.Light
	}
	return Request{Grid: rev.Grid, Lights: lights, Blocks: rev.Blocks, Samples: rev.Samples, Exposure: rev.Exposure}
}

// LightOperation mutates one light identified by its stable ID.
type LightOperation struct {
	Action string
	ID     string
	Light  *Light
}

// RevisionRequest continues baking from a stored base version.
type RevisionRequest struct {
	Base       string
	Operations []LightOperation
	Budget     int
}

// RevisionSets reports the probes affected by a revision. Resolved probes are
// re-solved for linear lighting and consume the budget; OcclusionOnly probes
// only have statistics updated and consume no budget.
type RevisionSets struct {
	Resolved      []int
	OcclusionOnly []int
}

// ErrQuota means more valid probes need linear re-solving than the budget allows.
var ErrQuota = errors.New("linear re-solve budget exceeded")

var (
	errBadAction      = errors.New("unknown light operation action")
	errDuplicateOp    = errors.New("light appears in more than one operation")
	errMissingLight   = errors.New("operation references an unknown light")
	errLightExists    = errors.New("added light id already exists")
	errMissingBody    = errors.New("added or changed light requires a light body")
	errUnexpectedBody = errors.New("removed light must not carry a light body")
	errBadBudget      = errors.New("budget must be non-negative")
	errNoOperations   = errors.New("continuation requires at least one light operation")
)

// Apply evaluates a continuation against a base revision and returns the new
// revision together with the affected probe sets. No scene outside the base
// changes: grid, blocks, samples and exposure are preserved verbatim.
func Apply(base Revision, req RevisionRequest) (Revision, RevisionSets, error) {
	if req.Budget < 0 {
		return Revision{}, RevisionSets{}, errBadBudget
	}
	if len(req.Operations) == 0 {
		return Revision{}, RevisionSets{}, errNoOperations
	}
	index := map[string]int{}
	for i, l := range base.Lights {
		index[l.ID] = i
	}
	removed := map[string]bool{}
	changed := map[string]Light{}
	var adds []StableLight
	seen := map[string]bool{}
	for _, op := range req.Operations {
		if op.ID == "" || seen[op.ID] {
			return Revision{}, RevisionSets{}, errDuplicateOp
		}
		seen[op.ID] = true
		switch op.Action {
		case "add":
			if _, ok := index[op.ID]; ok {
				return Revision{}, RevisionSets{}, errLightExists
			}
			if op.Light == nil {
				return Revision{}, RevisionSets{}, errMissingBody
			}
			adds = append(adds, StableLight{ID: op.ID, Light: *op.Light})
		case "remove":
			if _, ok := index[op.ID]; !ok {
				return Revision{}, RevisionSets{}, errMissingLight
			}
			if op.Light != nil {
				return Revision{}, RevisionSets{}, errUnexpectedBody
			}
			removed[op.ID] = true
		case "change":
			if _, ok := index[op.ID]; !ok {
				return Revision{}, RevisionSets{}, errMissingLight
			}
			if op.Light == nil {
				return Revision{}, RevisionSets{}, errMissingBody
			}
			changed[op.ID] = *op.Light
		default:
			return Revision{}, RevisionSets{}, errBadAction
		}
	}

	next := make([]StableLight, 0, len(base.Lights)+len(adds))
	for _, l := range base.Lights {
		if removed[l.ID] {
			continue
		}
		if body, ok := changed[l.ID]; ok {
			next = append(next, StableLight{ID: l.ID, Light: body})
			continue
		}
		next = append(next, l)
	}
	next = append(next, adds...)

	updated := Revision{Grid: base.Grid, Blocks: base.Blocks, Samples: base.Samples, Exposure: base.Exposure, Lights: next}
	if err := validate(updated.Request()); err != nil {
		return Revision{}, RevisionSets{}, err
	}

	sets, err := affected(base, updated, seen)
	if err != nil {
		return Revision{}, RevisionSets{}, err
	}
	if len(sets.Resolved) > req.Budget {
		return Revision{}, RevisionSets{}, ErrQuota
	}
	return updated, sets, nil
}

// lightReaches reports whether a light delivers non-zero energy to a probe,
// mirroring the accumulation rules of incoming.
func lightReaches(p Vec, l Light, blocks []Box) bool {
	dir := l.Position.sub(p)
	d := dir.length()
	if attenuation(d, l.Radius, l.Power) == 0 {
		return false
	}
	return visible(p, l.Position, blocks)
}

func affected(base, updated Revision, changed map[string]bool) (RevisionSets, error) {
	oldProbes := bake(base.Request())
	newProbes := bake(updated.Request())
	oldLights := map[string]Light{}
	for _, l := range base.Lights {
		oldLights[l.ID] = l.Light
	}
	newLights := map[string]Light{}
	for _, l := range updated.Lights {
		newLights[l.ID] = l.Light
	}
	var sets RevisionSets
	resolved := map[int]bool{}
	for i, probe := range oldProbes {
		if !probe.Valid {
			continue
		}
		for id := range changed {
			if l, ok := oldLights[id]; ok && lightReaches(probe.Position, l, base.Blocks) {
				resolved[i] = true
				break
			}
			if l, ok := newLights[id]; ok && lightReaches(probe.Position, l, updated.Blocks) {
				resolved[i] = true
				break
			}
		}
	}
	for i := range resolved {
		sets.Resolved = append(sets.Resolved, i)
	}
	for i, probe := range oldProbes {
		if !probe.Valid || resolved[i] {
			continue
		}
		fresh := newProbes[i]
		if probe.Contributing != fresh.Contributing || probe.Occluded != fresh.Occluded {
			sets.OcclusionOnly = append(sets.OcclusionOnly, i)
		}
	}
	return sets, nil
}
