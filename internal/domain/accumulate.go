package domain

import (
	"fmt"
	"math"
)

type Probe struct {
	Position               Vec
	Valid                  bool
	Irradiance             Lobes
	Contributing, Occluded int
}

// bake keeps the historical entry point stable for callers and tests.
func bake(r Request) []Probe {
	probes, _ := bakeFull(r)
	return probes
}

func bakeFull(r Request) ([]Probe, *Basis) {
	g := r.Grid
	data := newBounceData(r)
	out := make([]Probe, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				p := g.position(x, y, z)
				s := Probe{Position: p, Valid: !occupied(p, r.Blocks)}
				if s.Valid {
					s.Irradiance, s.Contributing, s.Occluded = incoming(p, r.Lights, r.Blocks)
					data.direct[idx] = s.Irradiance
					if len(r.Faces) > 0 {
						data.fillProbeRow(idx, p)
						data.addReflections(&s.Irradiance, idx)
					}
				}
				out[idx] = s
			}
		}
	}
	var basis *Basis
	if len(r.Faces) > 0 {
		basis = data.exportBasis(r, out)
	}
	return out, basis
}

type bounceData struct {
	faces  []Face
	lights []Light
	blocks []Box
	lf     []Vec
	ff     [][]float64
	fp     [][][]float64
	direct []Lobes
	vis    map[[2]string]bool
}

func newBounceData(r Request) *bounceData {
	n := len(r.Faces)
	bd := &bounceData{
		faces:  r.Faces,
		lights: r.Lights,
		blocks: r.Blocks,
		lf:     make([]Vec, n),
		ff:     make([][]float64, n),
		fp:     make([][][]float64, r.Grid.count()),
		direct: make([]Lobes, r.Grid.count()),
		vis:    make(map[[2]string]bool),
	}
	for i := range r.Faces {
		bd.ff[i] = make([]float64, n)
	}
	bd.computeLightFace()
	bd.computeFaceFace()
	return bd
}

func (bd *bounceData) clear(from, to string, p, q Vec, ap, aq *anchor) bool {
	key := [2]string{from, to}
	if v, ok := bd.vis[key]; ok {
		return v
	}
	v := segmentClear(p, q, bd.blocks, ap, aq)
	bd.vis[key] = v
	return v
}

// computeLightFace stores, per face, the summed linear light that reaches it
// before its own reflectance: light color times lamp attenuation times the
// non negative incident cosine, gated by closed box occlusion.
func (bd *bounceData) computeLightFace() {
	for i, f := range bd.faces {
		var sum Vec
		for li, lamp := range bd.lights {
			delta := f.Center.sub(lamp.Position)
			d := delta.length()
			if d == 0 {
				continue // coincident segment contributes nothing
			}
			w := attenuation(d, lamp.Radius, lamp.Power)
			if w == 0 {
				continue
			}
			cosIn := math.Max(0, f.unit.dot(lamp.Position.sub(f.Center).scale(1/d)))
			if cosIn == 0 {
				continue
			}
			if !bd.clear(fmt.Sprintf("L%d", li), fmt.Sprintf("F%d", i),
				lamp.Position, f.Center, nil, f.anchor) {
				continue
			}
			sum = sum.add(lamp.RGB.scale(w * cosIn))
		}
		bd.lf[i] = sum
	}
}

// computeFaceFace stores the scalar geometric transfer between ordered face
// pairs: both facing cosines over one plus squared distance.
func (bd *bounceData) computeFaceFace() {
	for i, a := range bd.faces {
		for j, b := range bd.faces {
			if i == j {
				continue
			}
			delta := b.Center.sub(a.Center)
			d := delta.length()
			if d == 0 {
				continue // coincident reflection segment contributes zero
			}
			dir := delta.scale(1 / d)
			cosOut := math.Max(0, a.unit.dot(dir))
			cosIn := math.Max(0, b.unit.dot(dir.scale(-1)))
			if cosOut == 0 || cosIn == 0 {
				continue
			}
			if !bd.clear(fmt.Sprintf("F%d", i), fmt.Sprintf("F%d", j),
				a.Center, b.Center, a.anchor, b.anchor) {
				continue
			}
			bd.ff[i][j] = cosOut * cosIn / (1 + d*d)
		}
	}
}

// fillProbeRow stores FaceProbe[probe][face][axis]: outgoing face cosine and
// distance factor followed by the original six direction probe projection.
func (bd *bounceData) fillProbeRow(index int, p Vec) {
	row := make([][]float64, len(bd.faces))
	for j, f := range bd.faces {
		weights := make([]float64, 6)
		delta := p.sub(f.Center)
		d := delta.length()
		if d != 0 {
			cosOut := math.Max(0, f.unit.dot(delta.scale(1/d)))
			if cosOut > 0 && bd.clear(fmt.Sprintf("F%d", j), fmt.Sprintf("P%d", index),
				f.Center, p, f.anchor, nil) {
				geom := cosOut / (1 + d*d)
				ray := f.Center.sub(p) // probe points back toward the face
				for k, axis := range axes {
					weights[k] = geom * math.Max(0, ray.dot(axis)/d)
				}
			}
		}
		row[j] = weights
	}
	bd.fp[index] = row
}

// addReflections accumulates one and two face bounces into a probe lobe set.
func (bd *bounceData) addReflections(out *Lobes, index int) {
	row := bd.fp[index]
	for j := range bd.faces {
		var leaving Vec
		for c := range 3 {
			v := bd.lf[j][c] * bd.faces[j].Reflectance[c]
			for i := range bd.faces {
				v += bd.lf[i][c] * bd.faces[i].Reflectance[c] * bd.ff[i][j] * bd.faces[j].Reflectance[c]
			}
			leaving[c] = v
		}
		for k := range 6 {
			w := row[j][k]
			if w == 0 {
				continue
			}
			for c := range 3 {
				out[k][c] += leaving[c] * w
			}
		}
	}
}
