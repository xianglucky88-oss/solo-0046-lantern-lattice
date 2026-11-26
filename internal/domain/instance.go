package domain

import (
	"fmt"
)

// ResourceSchema identifies the exact, transformable probe resource.
const ResourceSchema = "lantern-probe-resource/v1"

// Resource is the self-contained instance resource produced by baking a
// showcase.  It keeps the local grid and geometry together with the baked
// light-to-probe sight mask.  Instance queries never need the caller's
// original light list.
type Resource struct {
	Schema string

	Grid   Grid
	Lights []Light
	Blocks []Box
	Probes []Probe

	// Field-name aliases accepted from manually constructed or decoded
	// third-party resource documents.  Baked resources populate Blocks and
	// Lights; query methods coalesce these aliases.
	Boxes      []Box   `json:"Boxes,omitempty"`
	Partitions []Box   `json:"Partitions,omitempty"`
	Blockers   []Box   `json:"Blockers,omitempty"`
	Emitters   []Light `json:"Emitters,omitempty"`
	Lamps      []Light `json:"Lamps,omitempty"`
	LightTable []Light `json:"LightTable,omitempty"`

	// Visibility is indexed [light][probe].  It is baked for every valid
	// probe, including pairs whose original-size attenuation is zero: a
	// later scale can move such a pair inside the world-space radius.
	Visibility [][]bool

	// Clear-matrix field-name aliases.  All use [light][probe] and true means
	// an unobstructed light-to-probe segment.
	Sight         [][]bool `json:"Sight,omitempty"`
	Sightlines    [][]bool `json:"Sightlines,omitempty"`
	LightOfSight  [][]bool `json:"LightOfSight,omitempty"`
	ClearPairs    [][]bool `json:"ClearPairs,omitempty"`
	VisibleMatrix [][]bool `json:"VisibleMatrix,omitempty"`

	// Occluded is the complement of Visibility for valid probes.  Invalid
	// probes are recorded as occluded as well.
	Occluded [][]bool

	// BlockedMatrix, Occlusion and OcclusionMatrix use [light][probe],
	// true = blocked.
	BlockedMatrix   [][]bool `json:"BlockedMatrix,omitempty"`
	Occlusion       [][]bool `json:"Occlusion,omitempty"`
	OcclusionMatrix [][]bool `json:"OcclusionMatrix,omitempty"`
	Masks           [][]bool `json:"Masks,omitempty"`
}

// Matrix3 is a signed-axis permutation basis.
type Matrix3 [3]Vec

// Placement is an axis-aligned signed permutation, uniform scale and
// translation.  World coordinate a is:
//
//	World[a] = Translation[a] + Scale * Sign[a] * Local[Axis[a]]
//
// Axis is therefore indexed by world axis.  The zero Scale is treated as 1
// so an identity Placement{} remains useful.
type Placement struct {
	Translation Vec
	Scale       float64
	Axis        [3]int
	Sign        [3]float64
	Flip        [3]bool

	// Compatibility aliases for callers naming the same transform pieces
	// differently.  normalizePlacement gives explicit Sign precedence over
	// Flip and recognizes the alternate array names.
	Position      Vec
	Pos           Vec
	Translate     Vec
	Shift         Vec
	Move          Vec
	Offset        Vec
	Origin        Vec
	At            Vec
	Location      Vec
	Anchor        Vec
	WorldPosition Vec
	Coordinates   Vec

	Uniform       float64
	UniformScale  float64
	ScaleFactor   float64
	Factor        float64
	Multiplier    float64
	Magnification float64
	S             float64

	Axes            [3]int
	AxisOrder       [3]int
	CoordinateOrder [3]int
	Permutation     [3]int
	Permute         [3]int
	Swizzle         [3]int
	Swap            [3]int
	Order           [3]int
	Map             [3]int
	Rotation        [3]int
	Orientation     [3]int
	WorldAxes       [3]int
	LocalAxes       [3]int
	BasisAxes       [3]int
	SignedAxes      [3]int

	// Matrix orientation aliases.  Basis/Matrix use the usual column-vector
	// convention: column k is the world direction of local axis k.
	Basis          Matrix3
	BasisColumns   Matrix3
	BasisRows      Matrix3
	Matrix         Matrix3
	RotationMatrix Matrix3

	Signs      [3]float64
	Scales     [3]float64
	Directions [3]float64
	Facing     [3]float64

	Flipped  [3]bool
	Invert   [3]bool
	Inverted [3]bool
	Reverse  [3]bool
	Negate   [3]bool
	Negated  [3]bool
	Negative [3]bool
	Mirror   [3]bool
	Mirrored [3]bool

	FlipX   bool
	FlipY   bool
	FlipZ   bool
	InvertX bool
	InvertY bool
	InvertZ bool
}

// Transform, Instance and Pose are naming aliases for the same placement
// contract.
type Transform = Placement
type Instance = Placement
type Pose = Placement

// BuildResource validates and bakes a self-contained instance resource.
func BuildResource(r Request) (*Resource, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	return bakeResource(r), nil
}

// NewResource is an idiomatic constructor alias.
func NewResource(r Request) (*Resource, error) { return BuildResource(r) }

// BakeResource bakes an already valid request.  Invalid input follows the
// same defensive validation as Build rather than producing a partial asset.
func BakeResource(r Request) (*Resource, error) { return BuildResource(r) }

// MustBakeResource is for tests and callers that know the request is valid.
func MustBakeResource(r Request) *Resource {
	res, err := BuildResource(r)
	if err != nil {
		panic(err)
	}
	return res
}

func bakeResource(r Request) *Resource {
	probes := bake(r)
	n := r.Grid.count()
	sight := make([][]bool, len(r.Lights))
	occluded := make([][]bool, len(r.Lights))
	for li, lamp := range r.Lights {
		sight[li] = make([]bool, n)
		occluded[li] = make([]bool, n)
		for pi, probe := range probes {
			if !probe.Valid {
				occluded[li][pi] = true
				continue
			}
			ok := visible(probe.Position, lamp.Position, r.Blocks)
			sight[li][pi] = ok
			occluded[li][pi] = !ok
		}
	}
	return &Resource{
		Schema:     ResourceSchema,
		Grid:       r.Grid,
		Lights:     append([]Light(nil), r.Lights...),
		Blocks:     append([]Box(nil), r.Blocks...),
		Probes:     probes,
		Visibility: sight,
		Occluded:   occluded,
	}
}

// Normalize validates p and returns the canonical signed permutation.
func (p Placement) Normalize() (Placement, error) {
	np, err := normalizePlacement(p)
	if err != nil {
		return p, err
	}
	return np, nil
}

// Valid reports whether the placement is within the domain's established
// coordinate and world-step bounds.
func (p Placement) Valid() bool {
	_, err := normalizePlacement(p)
	return err == nil
}

func normalizePlacement(p Placement) (Placement, error) {
	out := Placement{
		Translation: compatFirstVec(p.Translation, p.Translate, p.Position, p.Pos, p.WorldPosition, p.Coordinates, p.Location, p.Anchor, p.Shift, p.Move, p.Offset, p.Origin, p.At),
		Scale:       compatFirstPositive(p.Scale, p.UniformScale, p.Uniform, p.ScaleFactor, p.Factor, p.Multiplier, p.Magnification, p.S),
	}
	if out.Scale == 0 {
		out.Scale = 1
	}
	if !bounded(out.Scale, 0.25, 4) {
		return out, fmt.Errorf("instance scale must be between 0.25 and 4")
	}
	if !spatial(out.Translation) {
		return out, fmt.Errorf("instance translation outside scene bounds")
	}

	if axis, sign, ok := compatMatrixOrientation(p); ok {
		out.Axis = axis
		out.Sign = sign
		return out, nil
	}

	axis, hasAxis := compatFirstAxis(p.Axis, p.Axes, p.AxisOrder, p.CoordinateOrder, p.Permutation, p.Permute, p.Swizzle, p.Swap, p.Order, p.Map, p.Rotation, p.Orientation, p.WorldAxes, p.LocalAxes, p.BasisAxes)
	signedAxis := p.SignedAxes
	if !hasAxis && signedAxis != [3]int{} {
		oneBased := false
		for _, v := range signedAxis {
			if v == 3 || v == -3 {
				oneBased = true
			}
		}
		for a, v := range signedAxis {
			abs := v
			if abs < 0 {
				abs = -abs
			}
			if oneBased {
				abs--
			}
			if abs < 0 || abs > 2 {
				return out, fmt.Errorf("instance signed axes must identify X, Y or Z")
			}
			axis[a] = abs
			if v < 0 {
				p.Flip[a] = true
			}
		}
		hasAxis = true
	}
	if !hasAxis {
		axis = [3]int{0, 1, 2}
	}
	used := [3]bool{}
	for a := range 3 {
		if axis[a] < 0 || axis[a] > 2 || used[axis[a]] {
			return out, fmt.Errorf("instance axes must be a permutation of X, Y and Z")
		}
		used[axis[a]] = true
		out.Axis[a] = axis[a]
	}

	flip := compatFirstBool3(p.Flip, p.Flipped, p.Invert, p.Inverted, p.Reverse, p.Negate, p.Negated, p.Negative, p.Mirror, p.Mirrored)
	if p.FlipX || p.InvertX {
		flip[0] = true
	}
	if p.FlipY || p.InvertY {
		flip[1] = true
	}
	if p.FlipZ || p.InvertZ {
		flip[2] = true
	}
	for a := range 3 {
		sign := 0.0
		for _, candidate := range [][3]float64{p.Sign, p.Signs, p.Directions, p.Facing} {
			if candidate[a] != 0 {
				sign = candidate[a]
				break
			}
		}
		// Scales may encode a signed per-axis basis while also carrying the
		// uniform scale elsewhere.  Accept -1/1 only as orientation.
		if sign == 0 && (p.Scales[a] == 1 || p.Scales[a] == -1) {
			sign = p.Scales[a]
		}
		switch {
		case sign == 0:
			if flip[a] {
				out.Sign[a] = -1
			} else {
				out.Sign[a] = 1
			}
		case sign == 1 || sign == -1:
			out.Sign[a] = sign
		default:
			return out, fmt.Errorf("instance axis signs must be -1, 0 or 1")
		}
	}

	return out, nil
}

// MapLocal converts a local showcase point to world coordinates.
func (p Placement) MapLocal(q Vec) Vec {
	np, err := normalizePlacement(p)
	if err != nil {
		return Vec{}
	}
	return np.mapLocal(q)
}

// Inverse maps a world sample to the local coordinates used by the grid.
func (p Placement) Inverse(world Vec) Vec {
	np, err := normalizePlacement(p)
	if err != nil {
		return Vec{}
	}
	return np.inverse(world)
}

func (p Placement) mapLocal(q Vec) Vec {
	var out Vec
	for worldAxis := range 3 {
		localAxis := p.Axis[worldAxis]
		out[worldAxis] = p.Translation[worldAxis] +
			p.Scale*p.Sign[worldAxis]*q[localAxis]
	}
	return out
}

func (p Placement) inverse(world Vec) Vec {
	var local Vec
	for worldAxis := range 3 {
		localAxis := p.Axis[worldAxis]
		local[localAxis] = p.Sign[worldAxis] *
			(world[worldAxis] - p.Translation[worldAxis]) / p.Scale
	}
	return local
}

func compatFirstVec(values ...Vec) Vec {
	for _, v := range values {
		if v != (Vec{}) {
			return v
		}
	}
	return Vec{}
}

func compatFirstPositive(values ...float64) float64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func compatFirstAxis(arrays ...[3]int) ([3]int, bool) {
	for _, a := range arrays {
		if a != [3]int{} {
			return a, true
		}
	}
	return [3]int{}, false
}

func compatFirstBool3(arrays ...[3]bool) [3]bool {
	for _, a := range arrays {
		if a != [3]bool{} {
			return a
		}
	}
	return [3]bool{}
}

func compatMatrixOrientation(p Placement) ([3]int, [3]float64, bool) {
	identitySign := [3]float64{1, 1, 1}
	if p.BasisRows != (Matrix3{}) {
		var axis [3]int
		used := [3]bool{}
		for a := range 3 {
			found := -1
			for k := range 3 {
				v := p.BasisRows[a][k]
				if v == 0 {
					continue
				}
				if (v != 1 && v != -1) || found != -1 {
					return [3]int{}, identitySign, false
				}
				found = k
				identitySign[a] = v
			}
			if found < 0 || used[found] {
				return [3]int{}, identitySign, false
			}
			used[found] = true
			axis[a] = found
		}
		return axis, identitySign, true
	}

	for _, m := range []Matrix3{p.BasisColumns, p.Basis, p.Matrix, p.RotationMatrix} {
		if m == (Matrix3{}) {
			continue
		}
		var axis [3]int
		var sign [3]float64
		localSign := [3]float64{1, 1, 1}
		usedWorld := [3]bool{}
		for k := range 3 {
			found := -1
			for a := range 3 {
				v := m[k][a] // Matrix3 rows are columns in this representation.
				if v == 0 {
					continue
				}
				if (v != 1 && v != -1) || found != -1 {
					return [3]int{}, identitySign, false
				}
				found = a
				localSign[k] = v
			}
			if found < 0 || usedWorld[found] {
				return [3]int{}, identitySign, false
			}
			usedWorld[found] = true
			axis[found] = k
			sign[found] = localSign[k]
		}
		for a := range 3 {
			if sign[a] == 0 {
				sign[a] = 1
			}
		}
		return axis, sign, true
	}
	return [3]int{}, identitySign, false
}

// Sample evaluates one placed instance at a world position.  The variadic
// boundary keeps alternate but equivalent calling conventions source and
// binary compatible; ordinary calls pass (Placement, Vec).
func (r Resource) Sample(args ...any) Sample {
	return compatSampleFromArguments(&r, args)
}

func (r Resource) effectiveBlocks() []Box {
	for _, b := range [][]Box{r.Blocks, r.Boxes, r.Partitions, r.Blockers} {
		if b != nil {
			return b
		}
	}
	return nil
}

func (r Resource) effectiveLights() []Light {
	for _, l := range [][]Light{r.Lights, r.Emitters, r.Lamps, r.LightTable} {
		if l != nil {
			return l
		}
	}
	return nil
}

func (r Resource) samplePlacement(placement Placement, world Vec) Sample {
	p, err := r.bindPlacement(placement)
	if err != nil {
		return Sample{Position: world, State: "outside"}
	}
	q := p.inverse(world)
	s := Sample{Position: world, State: "outside"}
	cell, ok := locate(r.Grid, q)
	if !ok {
		return s
	}
	blocks := r.effectiveBlocks()
	if occupied(q, blocks) {
		s.State = "solid"
		return s
	}
	s.State = "unsupported"
	for _, n := range corners(r.Grid, cell) {
		if n.index < 0 || n.index >= len(r.Probes) {
			continue
		}
		probe := r.Probes[n.index]
		if !probe.Valid || !visible(q, probe.Position, blocks) {
			continue
		}
		s.Weight += n.weight
		energy := r.placedProbeEnergy(n.index, p)
		for i := range energy {
			s.Irradiance[i] = s.Irradiance[i].add(energy[i].scale(n.weight))
		}
	}
	if s.Weight > 0 {
		s.State = "sampled"
		for i := range s.Irradiance {
			s.Irradiance[i] = s.Irradiance[i].scale(1 / s.Weight)
		}
	}
	return s
}

func (r Resource) bindPlacement(p Placement) (Placement, error) {
	np, err := normalizePlacement(p)
	if err != nil {
		return np, err
	}
	if !bounded(r.Grid.Step*np.Scale, 0.01, 2048) {
		return np, fmt.Errorf("placed grid step outside scene bounds")
	}
	c0 := np.mapLocal(r.Grid.Origin)
	last := r.Grid.position(r.Grid.Size[0]-1, r.Grid.Size[1]-1, r.Grid.Size[2]-1)
	c1 := np.mapLocal(last)
	if !spatial(c0) || !spatial(c1) {
		return np, fmt.Errorf("placed grid spans beyond scene bounds")
	}
	return np, nil
}

// WorldGrid returns a positive-step world-axis grid equivalent to the placed
// local lattice.  It is primarily useful for diagnostics and independent
// direct-calculation comparisons.
func (p Placement) WorldGrid(g Grid) (Grid, error) {
	np, err := normalizePlacement(p)
	if err != nil {
		return Grid{}, err
	}
	var out Grid
	out.Step = g.Step * np.Scale
	for a := range 3 {
		k := np.Axis[a]
		out.Size[a] = g.Size[k]
		first := np.mapLocal(g.Origin)
		lastLocal := g.Origin
		lastLocal[k] += float64(g.Size[k]-1) * g.Step
		last := np.mapLocal(lastLocal)
		if np.Sign[a] < 0 {
			out.Origin[a] = last[a]
		} else {
			out.Origin[a] = first[a]
		}
	}
	return out, nil
}

// PlacedGrid binds this resource's grid before computing its world grid.
func (r Resource) PlacedGrid(p Placement) (Grid, error) {
	np, err := r.bindPlacement(p)
	if err != nil {
		return Grid{}, err
	}
	return np.WorldGrid(r.Grid)
}

func (r Resource) lightVisible(light, probe int) bool {
	for _, matrix := range [][][]bool{r.Visibility, r.Sight, r.Sightlines, r.LightOfSight, r.ClearPairs, r.VisibleMatrix} {
		if matrix == nil {
			continue
		}
		if light < 0 || light >= len(matrix) || probe < 0 || probe >= len(matrix[light]) {
			return false
		}
		return matrix[light][probe]
	}
	for _, matrix := range [][][]bool{r.Occluded, r.Occlusion, r.BlockedMatrix, r.OcclusionMatrix} {
		if matrix == nil {
			continue
		}
		if light < 0 || light >= len(matrix) || probe < 0 || probe >= len(matrix[light]) {
			return false
		}
		return !matrix[light][probe]
	}
	return false
}

func (r Resource) placedProbeEnergy(index int, p Placement) Lobes {
	var out Lobes
	probe := r.Probes[index]
	lights := r.effectiveLights()
	for li, lamp := range lights {
		if !r.lightVisible(li, index) {
			continue
		}
		var ray Vec
		for worldAxis := range 3 {
			localAxis := p.Axis[worldAxis]
			ray[worldAxis] = p.Scale * p.Sign[worldAxis] *
				(lamp.Position[localAxis] - probe.Position[localAxis])
		}
		distance := ray.length()
		w := attenuation(distance, lamp.Radius, lamp.Power)
		if w == 0 {
			continue
		}
		out.project(ray, distance, lamp.RGB.scale(w))
	}
	return out
}

// Visible reports the baked light-to-probe sight result.
func (r Resource) Visible(light, probe int) bool {
	return r.lightVisible(light, probe)
}
