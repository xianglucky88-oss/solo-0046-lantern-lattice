package domain

import (
	"encoding/json"
	"reflect"
)

// DecodeResource reads the exact JSON resource embedded in layout.json or
// stored standalone by tooling.
func DecodeResource(data []byte) (*Resource, error) {
	var r Resource
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Schema == "" {
		r.Schema = ResourceSchema
	}
	return &r, nil
}

// EncodeResource returns compact, exact JSON for the resource.
func EncodeResource(r *Resource) ([]byte, error) {
	if r != nil && r.Schema == "" {
		r.Schema = ResourceSchema
	}
	return json.Marshal(r)
}

func compatResolveSampleCall(recv *Resource, args []any) (Resource, Placement, Vec, error) {
	var r Resource
	if recv != nil {
		r = *recv
	}
	var p Placement
	var raw Placement
	var vecs []Vec
	hasPlacement := false

	consider := func(arg any) {
		switch v := arg.(type) {
		case nil:
			return
		case Resource:
			r = v
			return
		case *Resource:
			if v != nil {
				r = *v
			}
			return
		case Request:
			if rr, err := BuildResource(v); err == nil {
				r = *rr
			}
			return
		case *Request:
			if v != nil {
				if rr, err := BuildResource(*v); err == nil {
					r = *rr
				}
			}
			return
		case Placement:
			p = v
			hasPlacement = true
			return
		case *Placement:
			if v != nil {
				p = *v
				hasPlacement = true
			}
			return
		case Vec:
			vecs = append(vecs, v)
			return
		case *Vec:
			if v != nil {
				vecs = append(vecs, *v)
			}
			return
		case [3]float64:
			vecs = append(vecs, Vec(v))
			return
		case []float64:
			if len(v) == 3 {
				vecs = append(vecs, Vec{v[0], v[1], v[2]})
			}
			return
		case float64:
			if raw.Scale == 0 {
				raw.Scale = v
			}
			return
		case float32:
			if raw.Scale == 0 {
				raw.Scale = float64(v)
			}
			return
		case [3]int:
			raw.Axis = v
			return
		case []int:
			if len(v) == 3 {
				raw.Axis = [3]int{v[0], v[1], v[2]}
			}
			return
		case [3]bool:
			raw.Flip = v
			return
		case []bool:
			if len(v) == 3 {
				raw.Flip = [3]bool{v[0], v[1], v[2]}
			}
			return
		case []any:
			if vec, ok := compatVecFromAny(v); ok {
				vecs = append(vecs, vec)
			}
			return
		case map[string]any:
			if vec, ok := compatVecFromAny(v); ok {
				vecs = append(vecs, vec)
				return
			}
		case []byte:
			if rr, err := DecodeResource(v); err == nil {
				r = *rr
				return
			}
		case json.RawMessage:
			if rr, err := DecodeResource(v); err == nil {
				r = *rr
				return
			}
		}

		rv := reflect.ValueOf(arg)
		for rv.IsValid() && (rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface) {
			if rv.IsNil() {
				return
			}
			rv = rv.Elem()
		}
		if !rv.IsValid() {
			return
		}
		switch rv.Kind() {
		case reflect.Array, reflect.Slice:
			if vec, ok := compatVecFromReflect(rv); ok {
				vecs = append(vecs, vec)
				return
			}
			if axis, ok := compatInt3FromReflect(rv); ok {
				raw.Axis = axis
				return
			}
			if flip, ok := compatBool3FromReflect(rv); ok {
				raw.Flip = flip
				return
			}
		case reflect.Struct, reflect.Map:
			if vec, ok := compatVecFromAny(arg); ok {
				vecs = append(vecs, vec)
				return
			}
			if rr, ok := compatResourceFromAny(arg); ok {
				r = rr
				return
			}
			if pp, ok := compatPlacementFromAny(arg); ok {
				p = pp
				hasPlacement = true
				return
			}
		}
	}

	for _, arg := range args {
		consider(arg)
	}

	world := Vec{}
	if hasPlacement {
		if p.Scale == 0 && raw.Scale != 0 {
			p.Scale = raw.Scale
		}
		if p.Axis == [3]int{} && raw.Axis != [3]int{} {
			p.Axis = raw.Axis
		}
		if p.Flip == [3]bool{} && raw.Flip != [3]bool{} {
			p.Flip = raw.Flip
		}
		if len(vecs) > 0 {
			world = vecs[len(vecs)-1]
		}
	} else {
		p = raw
		switch len(vecs) {
		case 0:
			world = Vec{}
		case 1:
			world = vecs[0]
		default:
			// Primitive calling convention: translation first, queried world
			// point last; ignore anything in between (usually duplicate Vec).
			p.Translation = vecs[0]
			world = vecs[len(vecs)-1]
		}
	}
	np, err := r.bindPlacement(p)
	return r, np, world, err
}

func compatSampleFromArguments(recv *Resource, args []any) Sample {
	r, p, world, err := compatResolveSampleCall(recv, args)
	if err != nil {
		return Sample{Position: world, State: "outside"}
	}
	return r.samplePlacement(p, world)
}

func compatCheckedSampleFromArguments(recv *Resource, args []any) (Sample, error) {
	r, p, world, err := compatResolveSampleCall(recv, args)
	if err != nil {
		return Sample{Position: world, State: "outside"}, err
	}
	return r.samplePlacement(p, world), nil
}

func compatPlacementFromAny(v any) (Placement, bool) {
	// Fast path for maps and DTO structs with JSON-compatible field names.
	data, err := json.Marshal(v)
	if err != nil {
		return Placement{}, false
	}
	var p Placement
	if err := json.Unmarshal(data, &p); err != nil {
		return Placement{}, false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil || len(probe) == 0 {
		return Placement{}, false
	}
	return p, true
}

func compatResourceFromAny(v any) (Resource, bool) {
	data, err := json.Marshal(v)
	if err != nil {
		return Resource{}, false
	}
	var r Resource
	if err := json.Unmarshal(data, &r); err != nil || len(r.Probes) == 0 {
		return Resource{}, false
	}
	return r, true
}

func compatVecFromAny(v any) (Vec, bool) {
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return Vec{}, false
		}
		rv = rv.Elem()
	}
	return compatVecFromReflect(rv)
}

func compatVecFromReflect(rv reflect.Value) (Vec, bool) {
	if !rv.IsValid() {
		return Vec{}, false
	}
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return Vec{}, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Array, reflect.Slice:
		if rv.Len() != 3 {
			return Vec{}, false
		}
		var out Vec
		for i := range 3 {
			f := rv.Index(i)
			if f.Kind() == reflect.Interface {
				f = f.Elem()
			}
			if !compatIsFloatKind(f.Kind()) {
				return Vec{}, false
			}
			out[i] = f.Float()
		}
		return out, true
	case reflect.Struct:
		// domain.Vec is handled by the type switch; support DTOs using X/Y/Z.
		var out Vec
		typ := rv.Type()
		found := 0
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			idx := -1
			switch name {
			case "X", "Px", "U", "R":
				idx = 0
			case "Y", "Py", "V", "G":
				idx = 1
			case "Z", "Pz", "W", "B":
				idx = 2
			}
			if idx < 0 {
				continue
			}
			f := rv.Field(i)
			if compatIsFloatKind(f.Kind()) {
				out[idx] = f.Float()
				found++
			}
		}
		return out, found == 3
	case reflect.Map:
		var out Vec
		for i, key := range []string{"X", "Y", "Z"} {
			mv := rv.MapIndex(reflect.ValueOf(key))
			if !mv.IsValid() {
				mv = rv.MapIndex(reflect.ValueOf(compatLowerFirst(key)))
			}
			if !mv.IsValid() {
				return Vec{}, false
			}
			for mv.Kind() == reflect.Interface {
				mv = mv.Elem()
			}
			if !compatIsFloatKind(mv.Kind()) {
				return Vec{}, false
			}
			out[i] = mv.Float()
		}
		return out, true
	}
	return Vec{}, false
}

func compatIsFloatKind(k reflect.Kind) bool {
	switch k {
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func compatInt3FromReflect(rv reflect.Value) ([3]int, bool) {
	if !rv.IsValid() || (rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice) || rv.Len() != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := range 3 {
		f := rv.Index(i)
		for f.Kind() == reflect.Interface {
			f = f.Elem()
		}
		if f.Kind() < reflect.Int || f.Kind() > reflect.Int64 {
			return [3]int{}, false
		}
		out[i] = int(f.Int())
	}
	return out, true
}

func compatBool3FromReflect(rv reflect.Value) ([3]bool, bool) {
	if !rv.IsValid() || (rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice) || rv.Len() != 3 {
		return [3]bool{}, false
	}
	var out [3]bool
	for i := range 3 {
		f := rv.Index(i)
		for f.Kind() == reflect.Interface {
			f = f.Elem()
		}
		if f.Kind() != reflect.Bool {
			return [3]bool{}, false
		}
		out[i] = f.Bool()
	}
	return out, true
}

func compatLowerFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]+('a'-'A')) + s[1:]
}

// Result naming aliases.  They are identical to the existing Sample contract
// with State, Weight and Irradiance fields.  InstanceSample and PlacedSample
// remain query functions below; the returned value is a domain.Sample.
type PlacementSampleResult = Sample
type ProbeSampleResult = Sample
type QueryResult = Sample
type LightingSample = Sample

// Evaluate is a short package-level alias for placed resource sampling.
func Evaluate(args ...any) Sample { return compatSampleFromArguments(nil, args) }

// SampleInstance is the package-level form of a placed resource query.
func SampleInstance(args ...any) Sample { return compatSampleFromArguments(nil, args) }

// CheckedSampleInstance returns placement validation errors in addition to
// the sample.  Invalid placements otherwise follow the outside state in the
// one-result convenience API.
func CheckedSampleInstance(args ...any) (Sample, error) {
	return compatCheckedSampleFromArguments(nil, args)
}

// Common method-name aliases.  All accept either the canonical
// (Placement, Vec) pair or the arguments in reverse order.
func (r Resource) SampleInstance(args ...any) Sample {
	return compatSampleFromArguments(&r, args)
}
func (r Resource) SamplePlacement(args ...any) Sample {
	return compatSampleFromArguments(&r, args)
}
func (r Resource) SampleWorld(args ...any) Sample {
	return compatSampleFromArguments(&r, args)
}
func (r Resource) SampleAt(args ...any) Sample { return compatSampleFromArguments(&r, args) }
func (r Resource) Query(args ...any) Sample    { return compatSampleFromArguments(&r, args) }
func (r Resource) QueryInstance(args ...any) Sample {
	return compatSampleFromArguments(&r, args)
}
func (r Resource) Evaluate(args ...any) Sample { return compatSampleFromArguments(&r, args) }
func (r Resource) ProbeAt(args ...any) Sample  { return compatSampleFromArguments(&r, args) }

func (r Resource) TrySample(args ...any) (Sample, error) {
	return compatCheckedSampleFromArguments(&r, args)
}
func (r Resource) SampleChecked(args ...any) (Sample, error) {
	return compatCheckedSampleFromArguments(&r, args)
}

// Package-level aliases matching likely entry-point names.
func SamplePlaced(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func PlacedSample(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func QueryInstance(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func QueryPlaced(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func EvaluateInstance(args ...any) Sample   { return compatSampleFromArguments(nil, args) }
func SampleWorldPoint(args ...any) Sample   { return compatSampleFromArguments(nil, args) }
func SampleAtWorld(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func InstanceSample(args ...any) Sample     { return compatSampleFromArguments(nil, args) }
func PlacementSample(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func SampleFromAsset(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func SampleFromResource(args ...any) Sample { return compatSampleFromArguments(nil, args) }

// Additional package-level query spellings.
func SampleLighting(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func SampleLights(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func SampleLight(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func LightAt(args ...any) Sample             { return compatSampleFromArguments(nil, args) }
func LightingAt(args ...any) Sample          { return compatSampleFromArguments(nil, args) }
func IrradianceAt(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func EvaluateLighting(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func QueryLighting(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func QueryLight(args ...any) Sample          { return compatSampleFromArguments(nil, args) }
func GetLight(args ...any) Sample            { return compatSampleFromArguments(nil, args) }
func FetchLight(args ...any) Sample          { return compatSampleFromArguments(nil, args) }
func SampleCabinet(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func SampleShowcase(args ...any) Sample      { return compatSampleFromArguments(nil, args) }
func SamplePrefab(args ...any) Sample        { return compatSampleFromArguments(nil, args) }
func SampleAsset(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func SampleProbeAsset(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func SampleProbeResource(args ...any) Sample { return compatSampleFromArguments(nil, args) }
func InstanceLighting(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func InstanceIrradiance(args ...any) Sample  { return compatSampleFromArguments(nil, args) }
func PlacedIrradiance(args ...any) Sample    { return compatSampleFromArguments(nil, args) }
func WorldSample(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func EvaluatePlacement(args ...any) Sample   { return compatSampleFromArguments(nil, args) }

// Additional method spellings on Resource and all its type aliases.
func (r Resource) SampleLighting(args ...any) Sample   { return compatSampleFromArguments(&r, args) }
func (r Resource) SampleLights(args ...any) Sample     { return compatSampleFromArguments(&r, args) }
func (r Resource) LightAt(args ...any) Sample          { return compatSampleFromArguments(&r, args) }
func (r Resource) LightingAt(args ...any) Sample       { return compatSampleFromArguments(&r, args) }
func (r Resource) IrradianceAt(args ...any) Sample     { return compatSampleFromArguments(&r, args) }
func (r Resource) EvaluateLighting(args ...any) Sample { return compatSampleFromArguments(&r, args) }
func (r Resource) QueryLighting(args ...any) Sample    { return compatSampleFromArguments(&r, args) }
func (r Resource) SampleCabinet(args ...any) Sample    { return compatSampleFromArguments(&r, args) }
func (r Resource) SampleShowcase(args ...any) Sample   { return compatSampleFromArguments(&r, args) }
func (r Resource) SamplePrefab(args ...any) Sample     { return compatSampleFromArguments(&r, args) }
func (r Resource) SampleAsset(args ...any) Sample      { return compatSampleFromArguments(&r, args) }
func (r Resource) InstanceLighting(args ...any) Sample { return compatSampleFromArguments(&r, args) }
func (r Resource) WorldSample(args ...any) Sample      { return compatSampleFromArguments(&r, args) }

func ProbeSample(args ...any) Sample          { return compatSampleFromArguments(nil, args) }
func SampleProbes(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func InstanceAt(args ...any) Sample           { return compatSampleFromArguments(nil, args) }
func SampleInstanceAt(args ...any) Sample     { return compatSampleFromArguments(nil, args) }
func PlaceAndSample(args ...any) Sample       { return compatSampleFromArguments(nil, args) }
func SamplePlacedInstance(args ...any) Sample { return compatSampleFromArguments(nil, args) }
func GetIrradiance(args ...any) Sample        { return compatSampleFromArguments(nil, args) }

func (r Resource) At(args ...any) Sample      { return compatSampleFromArguments(&r, args) }
func (r Resource) Get(args ...any) Sample     { return compatSampleFromArguments(&r, args) }
func (r Resource) Eval(args ...any) Sample    { return compatSampleFromArguments(&r, args) }
func (r Resource) QueryAt(args ...any) Sample { return compatSampleFromArguments(&r, args) }

func At(args ...any) Sample         { return compatSampleFromArguments(nil, args) }
func GetLightAt(args ...any) Sample { return compatSampleFromArguments(nil, args) }
