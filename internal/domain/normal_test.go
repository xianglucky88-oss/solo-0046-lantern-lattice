package domain

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9 }

func testScene() Request {
	return Request{
		Grid: Grid{Origin: Vec{0, 0, 0}, Step: 1, Size: [3]int{3, 2, 2}},
		Lights: []Light{
			{Position: Vec{-1, 0.5, 0}, RGB: Vec{1, 0.55, 0.2}, Radius: 6, Power: 4},
			{Position: Vec{2, 3, 1}, RGB: Vec{0.2, 0.9, 0.4}, Radius: 20, Power: 2.5},
		},
		Blocks:   []Box{{Min: Vec{0.8, -0.2, -0.2}, Max: Vec{1.2, 0.2, 0.2}}},
		Samples:  []Vec{{0.5, 0.5, 0.5}, {1.5, 1.25, 0.75}, {2, 2, 1}},
		Normals:  []Vec{{1, 0, 0}, {-1, 0, 0}, {0.6, 0.8, 0}},
		Exposure: 3,
	}
}

// Axis-aligned normals must reproduce the legacy six-direction lobes exactly.
func TestNormalMatchesLegacyAxes(t *testing.T) {
	r := testScene()
	probes := bake(r)
	for _, p := range r.Samples {
		legacy := sample(r.Grid, probes, p, r.Blocks)
		for k, axis := range axes {
			got := sampleNormal(r.Grid, probes, p, axis, r.Blocks)
			if got.State != legacy.State || !near(got.Weight, legacy.Weight) {
				t.Fatalf("state/support mismatch at %v axis %d", p, k)
			}
			for c := range 3 {
				if !near(got.Irradiance[c], legacy.Irradiance[k][c]) {
					t.Fatalf("channel %d axis %d: got %v want %v", c, k, got.Irradiance[c], legacy.Irradiance[k][c])
				}
			}
		}
	}
}

// Edge-on and back-facing beams contribute exactly zero.
func TestNormalHemisphereEdgeIsZero(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:   []Light{{Position: Vec{-2, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 10, Power: 3}},
		Samples:  []Vec{{0, 0, 0}},
		Normals:  []Vec{{1, 0, 0}},
		Exposure: 1,
	}
	probes := bake(r)
	if got := sampleNormal(r.Grid, probes, Vec{}, Vec{0, 1, 0}, nil); maxAbs(got.Irradiance) != 0 {
		t.Fatalf("edge normal gathered %v", got.Irradiance)
	}
	if got := sampleNormal(r.Grid, probes, Vec{}, Vec{1, 0, 0}, nil); maxAbs(got.Irradiance) != 0 {
		t.Fatalf("back normal gathered %v", got.Irradiance)
	}
	if got := sampleNormal(r.Grid, probes, Vec{}, Vec{-1, 0, 0}, nil); maxAbs(got.Irradiance) == 0 {
		t.Fatalf("facing normal gathered nothing")
	}
}

// A light coincident with a probe contributes equally to every normal.
func TestNormalCoincidentLightIsIsotropic(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:   []Light{{Position: Vec{}, RGB: Vec{0.25, 0.5, 1}, Radius: 5, Power: 2}},
		Samples:  []Vec{{0, 0, 0}},
		Normals:  []Vec{{1, 0, 0}},
		Exposure: 1,
	}
	probes := bake(r)
	first := sampleNormal(r.Grid, probes, Vec{}, Vec{1, 0, 0}, nil).Irradiance
	if maxAbs(first) == 0 {
		t.Fatal("coincident light produced nothing")
	}
	for _, n := range append([][3]float64{{0, 1, 0}, {0, 0, 1}}, [3]float64{0.6, 0.8, 0}) {
		if got := sampleNormal(r.Grid, probes, Vec{}, Vec(n), nil).Irradiance; !near(got[0], first[0]) || !near(got[1], first[1]) || !near(got[2], first[2]) {
			t.Fatalf("normal %v got %v want %v", n, got, first)
		}
	}
}

// The downloadable resource must reproduce the in-process query exactly.
func TestNormalResourceReadoutMatchesSample(t *testing.T) {
	r := testScene()
	probes := bake(r)
	resource := resourceFromProbes(r.Grid, probes, r.Blocks)
	for _, p := range r.Samples {
		for _, n := range append(append([]Vec{}, r.Normals...), Vec{0, 0, 1}) {
			want := sampleNormal(r.Grid, probes, p, n, r.Blocks)
			got := resource.Sample(p, n)
			if got.State != want.State || !near(got.Weight, want.Weight) {
				t.Fatalf("resource state/support mismatch at %v", p)
			}
			for c := range 3 {
				if !near(got.Irradiance[c], want.Irradiance[c]) {
					t.Fatalf("resource channel %d at %v: got %v want %v", c, p, got.Irradiance[c], want.Irradiance[c])
				}
			}
		}
	}
}

// Omitting normals keeps the encoded layout identical to the legacy document.
func TestNormalsDisabledKeepsLegacyLayout(t *testing.T) {
	r := testScene()
	r.Normals = nil
	probes := bake(r)
	meta := describe(r, probes)
	if meta.NormalSamples != nil || meta.NormalResource != nil {
		t.Fatal("legacy metadata gained normal sections")
	}
}

func maxAbs(v Vec) float64 {
	return math.Max(math.Abs(v[0]), math.Max(math.Abs(v[1]), math.Abs(v[2])))
}
