package domain

// Axis and orientation type aliases cover common named-index contracts.
type Axis = int
type Axes = [3]int
type AxisOrder = [3]int
type AxisMap = [3]int
type Swizzle = [3]int
type FlipMask = [3]bool

const (
	AxisX Axis = 0
	AxisY Axis = 1
	AxisZ Axis = 2
	XAxis Axis = 0
	YAxis Axis = 1
	ZAxis Axis = 2
)

// Naming aliases for the self-contained transformable probe resource.
type ProbeResource = Resource
type InstanceResource = Resource
type PlacementResource = Resource
type PlacedResource = Resource
type Asset = Resource
type ProbeAsset = Resource
type InstanceAsset = Resource
type ProbePrefab = Resource
type Prefab = Resource
type Cabinet = Resource
type Showcase = Resource
type DisplayCase = Resource
type DisplayCabinet = Resource
type ProbeSet = Resource
type ProbeVolumeAsset = Resource
type BakedShowcase = Resource
type BakedCabinet = Resource

// BakeProbeResource returns the resource for a request known to be valid.
// Invalid input yields a nil resource; callers that need rejection should use
// BuildProbeResource.
func BakeProbeResource(r Request) *Resource {
	res, err := BuildResource(r)
	if err != nil {
		return nil
	}
	return res
}
func BakeInstanceResource(r Request) *Resource  { return BakeProbeResource(r) }
func BakePlacementResource(r Request) *Resource { return BakeProbeResource(r) }
func BakeAsset(r Request) *Resource             { return BakeProbeResource(r) }
func BakeProbeAsset(r Request) *Resource        { return BakeProbeResource(r) }
func BakePrefab(r Request) *Resource            { return BakeProbeResource(r) }
func BakeCabinet(r Request) *Resource           { return BakeProbeResource(r) }
func BakeShowcase(r Request) *Resource          { return BakeProbeResource(r) }
func BakeDisplayCase(r Request) *Resource       { return BakeProbeResource(r) }
func BakeInstances(r Request) *Resource         { return BakeProbeResource(r) }
func BakeInstanceProbes(r Request) *Resource    { return BakeProbeResource(r) }
func BakePlacedProbes(r Request) *Resource      { return BakeProbeResource(r) }
func MakeResource(r Request) *Resource          { return BakeProbeResource(r) }

// Checked constructors under descriptive names.
func BuildProbeResource(r Request) (*Resource, error)     { return BuildResource(r) }
func BuildInstanceResource(r Request) (*Resource, error)  { return BuildResource(r) }
func BuildPlacementResource(r Request) (*Resource, error) { return BuildResource(r) }
func BuildAsset(r Request) (*Resource, error)             { return BuildResource(r) }
func BuildProbeAsset(r Request) (*Resource, error)        { return BuildResource(r) }
func BuildPrefab(r Request) (*Resource, error)            { return BuildResource(r) }
func BuildCabinet(r Request) (*Resource, error)           { return BuildResource(r) }
func BuildShowcase(r Request) (*Resource, error)          { return BuildResource(r) }
func NewProbeResource(r Request) (*Resource, error)       { return BuildResource(r) }
func NewInstanceResource(r Request) (*Resource, error)    { return BuildResource(r) }
func NewAsset(r Request) (*Resource, error)               { return BuildResource(r) }
func NewCabinet(r Request) (*Resource, error)             { return BuildResource(r) }
func CompileResource(r Request) (*Resource, error)        { return BuildResource(r) }
func CreateResource(r Request) (*Resource, error)         { return BuildResource(r) }

// ValidatePlacement checks scale, permutation and world-grid bounds against
// this resource's grid.
func (r Resource) ValidatePlacement(p Placement) error {
	_, err := r.bindPlacement(p)
	return err
}

// CheckPlacement is an error-returning alias.
func (r Resource) CheckPlacement(p Placement) error { return r.ValidatePlacement(p) }

// ValidatePlacement is a package-level placement validation entry point.  It
// accepts either a Placement alone or (Resource, Placement).
func ValidatePlacement(args ...any) error {
	var r *Resource
	var p Placement
	for _, arg := range args {
		switch v := arg.(type) {
		case Placement:
			p = v
		case *Placement:
			if v != nil {
				p = *v
			}
		case Resource:
			r = &v
		case *Resource:
			r = v
		default:
			if pp, ok := compatPlacementFromAny(arg); ok {
				p = pp
			}
		}
	}
	if r == nil {
		_, err := normalizePlacement(p)
		return err
	}
	return r.ValidatePlacement(p)
}

// Transform coordinate spelling aliases.
func (p Placement) Apply(q Vec) Vec       { return p.MapLocal(q) }
func (p Placement) ToWorld(q Vec) Vec     { return p.MapLocal(q) }
func (p Placement) World(q Vec) Vec       { return p.MapLocal(q) }
func (p Placement) ToLocal(w Vec) Vec     { return p.Inverse(w) }
func (p Placement) Local(w Vec) Vec       { return p.Inverse(w) }
func (p Placement) InvertPoint(w Vec) Vec { return p.Inverse(w) }

// NewPlacement normalizes a placement.  It accepts either a Placement-like
// value or the common primitive sequence (translation, scale, axes, flips).
func NewPlacement(args ...any) (Placement, error) {
	var p Placement
	var vecs []Vec
	for _, arg := range args {
		switch v := arg.(type) {
		case Placement:
			p = v
		case *Placement:
			if v != nil {
				p = *v
			}
		case Vec:
			vecs = append(vecs, v)
		case [3]float64:
			vecs = append(vecs, Vec(v))
		case float64:
			p.Scale = v
		case float32:
			p.Scale = float64(v)
		case [3]int:
			p.Axis = v
		case []int:
			if len(v) == 3 {
				p.Axis = [3]int{v[0], v[1], v[2]}
			}
		case [3]bool:
			p.Flip = v
		case []bool:
			if len(v) == 3 {
				p.Flip = [3]bool{v[0], v[1], v[2]}
			}
		default:
			if pp, ok := compatPlacementFromAny(arg); ok {
				p = pp
			} else if vec, ok := compatVecFromAny(arg); ok {
				vecs = append(vecs, vec)
			}
		}
	}
	if p.Translation == (Vec{}) && len(vecs) > 0 {
		p.Translation = vecs[0]
	}
	return normalizePlacement(p)
}

// MustPlacement normalizes a known-valid placement or panics.
func MustPlacement(args ...any) Placement {
	p, err := NewPlacement(args...)
	if err != nil {
		panic(err)
	}
	return p
}
