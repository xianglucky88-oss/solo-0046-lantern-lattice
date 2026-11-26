package domain

// BasisSchema labels the self contained recoloring section of layout.json.
const BasisSchema = "lantern-preview/v1"

// Basis is the bake-once offline recoloring data. Every geometric factor that
// could require the original scene or ray occlusion is frozen here; changing
// a color only recombines these terms with face reflectances.
type Basis struct {
	Schema      string        `json:"Schema"`
	FaceIDs     []string      `json:"FaceIDs"`
	Reflectance []Vec         `json:"Reflectance"`
	ProbeValid  []bool        `json:"ProbeValid"`
	Direct      []Lobes       `json:"Direct"`
	BakedLobes  []Lobes       `json:"BakedLobes"`
	LightFace   []Vec         `json:"LightFace"`
	FaceFace    [][]float64   `json:"FaceFace"`
	FaceProbe   [][][]float64 `json:"FaceProbe"`
	Samples     []BasisSample `json:"Samples"`
}

// BasisSample freezes one query point's state, support and probe links.
type BasisSample struct {
	Index    int     `json:"Index"`
	Position Vec     `json:"Position"`
	State    string  `json:"State"`
	Weight   float64 `json:"Weight"`
	Links    []Link  `json:"Links"`
}

func (bd *bounceData) exportBasis(r Request, probes []Probe) *Basis {
	n := len(r.Faces)
	b := &Basis{
		Schema:      BasisSchema,
		FaceIDs:     make([]string, n),
		Reflectance: make([]Vec, n),
		ProbeValid:  make([]bool, len(probes)),
		Direct:      bd.direct,
		BakedLobes:  make([]Lobes, len(probes)),
		LightFace:   bd.lf,
		FaceFace:    bd.ff,
		FaceProbe:   bd.fp,
		Samples:     make([]BasisSample, 0, len(r.Samples)),
	}
	for i, f := range r.Faces {
		b.FaceIDs[i] = f.ID
		b.Reflectance[i] = f.Reflectance
	}
	for i, p := range probes {
		b.ProbeValid[i] = p.Valid
		b.BakedLobes[i] = p.Irradiance
		if b.FaceProbe[i] == nil {
			b.FaceProbe[i] = zeroFaceProbe(n)
		}
	}
	for si, q := range r.Samples {
		s, links := sampleDetail(r.Grid, probes, q, r.Blocks)
		b.Samples = append(b.Samples, BasisSample{
			Index:    si,
			Position: q,
			State:    s.State,
			Weight:   s.Weight,
			Links:    links,
		})
	}
	return b
}

func zeroFaceProbe(n int) [][]float64 {
	row := make([][]float64, n)
	for j := range n {
		row[j] = make([]float64, 6)
	}
	return row
}

// Lobes recomputes every grid probe's linear six direction result for the
// supplied per face reflectances (index aligned with FaceIDs).
func (b *Basis) Lobes(rho []Vec) []Lobes {
	out := make([]Lobes, len(b.Direct))
	copy(out, b.Direct)
	for p := range out {
		if p >= len(b.ProbeValid) || !b.ProbeValid[p] || p >= len(b.FaceProbe) {
			continue
		}
		row := b.FaceProbe[p]
		for j := 0; j < len(b.FaceIDs) && j < len(row); j++ {
			var leaving Vec
			for c := range 3 {
				v := b.LightFace[j][c] * rho[j][c]
				for i := 0; i < len(b.FaceIDs); i++ {
					if i >= len(b.FaceFace) || j >= len(b.FaceFace[i]) {
						continue
					}
					v += b.LightFace[i][c] * rho[i][c] * b.FaceFace[i][j] * rho[j][c]
				}
				leaving[c] = v
			}
			for k := range 6 {
				w := row[j][k]
				if w == 0 {
					continue
				}
				for c := range 3 {
					out[p][k][c] += leaving[c] * w
				}
			}
		}
	}
	return out
}

// SampleColors reruns the frozen trilinear links over recolored probe lobes.
// State and Weight are echoed unchanged.
func (b *Basis) SampleColors(lobes []Lobes) []Sample {
	out := make([]Sample, 0, len(b.Samples))
	for _, bs := range b.Samples {
		s := Sample{Position: bs.Position, State: bs.State, Weight: bs.Weight}
		if bs.Weight > 0 {
			for _, link := range bs.Links {
				if link.Index < 0 || link.Index >= len(lobes) {
					continue
				}
				for k := range 6 {
					s.Irradiance[k] = s.Irradiance[k].add(lobes[link.Index][k].scale(link.Weight))
				}
			}
			for k := range 6 {
				s.Irradiance[k] = s.Irradiance[k].scale(1 / bs.Weight)
			}
		}
		out = append(out, s)
	}
	return out
}

// Reflectances fills an ID keyed override map over the initial values.
func (b *Basis) Reflectances(overrides map[string]Vec) ([]Vec, error) {
	rho := append([]Vec(nil), b.Reflectance...)
	index := make(map[string]int, len(b.FaceIDs))
	for i, id := range b.FaceIDs {
		index[id] = i
	}
	for id, value := range overrides {
		i, ok := index[id]
		if !ok {
			return nil, &UnknownFaceError{ID: id}
		}
		for c, v := range value {
			if v < 0 || v > 1 {
				return nil, &InvalidReflectanceError{ID: id, Channel: c, Value: v}
			}
		}
		rho[i] = value
	}
	return rho, nil
}

type UnknownFaceError struct{ ID string }

func (e *UnknownFaceError) Error() string { return "unknown face ID " + e.ID }

type InvalidReflectanceError struct {
	ID      string
	Channel int
	Value   float64
}

func (e *InvalidReflectanceError) Error() string { return "reflectance outside 0..1 for face " + e.ID }
