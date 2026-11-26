package domain

import (
	"encoding/json"
	"math"
	"testing"
)

func lanternApprox(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// One lamp -> one face -> probe. Every factor is hand computable.
func TestLanternOneBounceExactFactors(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{1, 0, 0}, Step: 1, Size: [3]int{2, 2, 2}},
		Exposure: 3,
		Lights:   []Light{{Position: Vec{0, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 10, Power: 1}},
		Blocks:   []Box{{Min: Vec{2, -1, -1}, Max: Vec{2.2, 1, 1}}},
		Faces: []Face{{
			ID: "wall", Center: Vec{2, 0, 0}, Normal: Vec{-1, 0, 0},
			Reflectance: Vec{0.25, 0.5, 0.75},
		}},
	}
	if err := validate(r); err != nil {
		t.Fatal(err)
	}
	probes, basis := bakeFull(r)
	p0 := probes[0] // origin grid point (1,0,0)
	if !p0.Valid {
		t.Fatal("probe 0 should be valid")
	}
	// Lamp distance 1: attenuation 0.9^2/(1+1)=0.405 into -X lobe.
	if !lanternApprox(p0.Irradiance[1][0], 0.405, 1e-12) {
		t.Fatalf("direct -X = %v want 0.405", p0.Irradiance[1][0])
	}
	// Lamp->face d=2 attenuation 0.8^2/5=0.128, cos=1; face->probe d=1
	// cosOut=1 factor 1/2; +X projection 1. Times reflectance.
	want := []float64{0.064 * 0.25, 0.064 * 0.5, 0.064 * 0.75}
	for c := range 3 {
		if !lanternApprox(p0.Irradiance[0][c], want[c], 1e-12) {
			t.Fatalf("one bounce +X ch%d = %v want %v", c, p0.Irradiance[0][c], want[c])
		}
	}
	if basis == nil || len(basis.FaceIDs) != 1 {
		t.Fatal("basis missing")
	}
}

// A blocker between lamp and face must kill the bounce even though both
// reflection endpoint contacts are exempt on the wall box itself.
func TestLanternReflectionOcclusion(t *testing.T) {
	wall := Box{Min: Vec{2, -1, -1}, Max: Vec{2.2, 1, 1}}
	middle := Box{Min: Vec{1, -2, -2}, Max: Vec{1.2, 2, 2}}
	face := Face{ID: "w", Center: Vec{2, 0, 0}, Normal: Vec{-1, 0, 0}, Reflectance: Vec{1, 1, 1}}
	r := Request{
		Grid:     Grid{Origin: Vec{1.5, 3, 3}, Step: 1, Size: [3]int{2, 2, 2}},
		Exposure: 1,
		Lights:   []Light{{Position: Vec{0, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 100, Power: 1}},
		Blocks:   []Box{wall, middle},
		Faces:    []Face{face},
	}
	if err := validate(r); err != nil {
		t.Fatal(err)
	}
	bd := newBounceData(r)
	if bd.lf[0][0] != 0 {
		t.Fatalf("lamp-face should be blocked, LF=%v", bd.lf[0])
	}
	// Without the middle box the same segment is allowed via end anchor.
	r.Blocks = []Box{wall}
	if err := validate(r); err != nil {
		t.Fatal(err)
	}
	bd = newBounceData(r)
	if bd.lf[0][0] == 0 {
		t.Fatal("lamp-face should transmit with only owning wall box")
	}
}

// A ray from a face center through the interior of its own box is blocked;
// only the isolated endpoint touch is exempt.
func TestLanternOwnBoxInteriorBlocks(t *testing.T) {
	b := Box{Min: Vec{0, 0, 0}, Max: Vec{1, 1, 1}}
	anchor := &anchor{box: 0, axis: 0, side: 1} // +X face center (1,0.5,0.5)
	start := Vec{1, 0.5, 0.5}
	outside := Vec{3, 0.5, 0.5}
	if !segmentClear(start, outside, []Box{b}, anchor, nil) {
		t.Fatal("outward ray from face center should clear endpoint touch")
	}
	through := Vec{-3, 0.5, 0.5}
	if segmentClear(start, through, []Box{b}, anchor, nil) {
		t.Fatal("ray crossing the box interior must be blocked")
	}
}

func TestLanternCoincidentFacePairZero(t *testing.T) {
	b := Box{Min: Vec{0, 0, 0}, Max: Vec{1, 1, 1}}
	f1 := Face{ID: "a", Center: Vec{1, 0.5, 0.5}, Normal: Vec{1, 0, 0}, Reflectance: Vec{1, 1, 1}}
	f2 := Face{ID: "b", Center: Vec{1, 0.5, 0.5}, Normal: Vec{1, 0, 0}, Reflectance: Vec{1, 1, 1}}
	r := Request{
		Grid:     Grid{Origin: Vec{0, 0, 0}, Step: 1, Size: [3]int{2, 2, 2}},
		Exposure: 1,
		Blocks:   []Box{b},
		Faces:    []Face{f1, f2},
	}
	if err := validate(r); err != nil {
		t.Fatal(err)
	}
	bd := newBounceData(r)
	if bd.ff[0][1] != 0 || bd.ff[1][0] != 0 {
		t.Fatalf("coincident face transfer must be zero: %v", bd.ff)
	}
}

// The corner property: on a two bounce only basis, killing red on either
// face removes red while green survives.
func TestLanternTwoBounceChannelKill(t *testing.T) {
	b := &Basis{
		FaceIDs:     []string{"A", "B"},
		Reflectance: []Vec{{1, 1, 1}, {1, 1, 1}},
		ProbeValid:  []bool{true},
		Direct:      []Lobes{{}},
		LightFace:   []Vec{{1, 1, 1}, {0, 0, 0}},
		FaceFace:    [][]float64{{0, 0.25}, {0, 0}},
		FaceProbe:   [][][]float64{{{0, 0, 0, 0, 0, 0}, {0.5, 0, 0, 0, 0, 0}}},
	}
	white := b.Lobes([]Vec{{1, 1, 1}, {1, 1, 1}})[0][0]
	if white[0] <= 0 || white[1] <= 0 {
		t.Fatalf("expected two bounce energy, got %v", white)
	}
	for _, kill := range []int{0, 1} {
		rho := []Vec{{1, 1, 1}, {1, 1, 1}}
		rho[kill][0] = 0
		got := b.Lobes(rho)[0][0]
		if got[0] != 0 {
			t.Fatalf("kill red on face %d: red = %v want 0", kill, got[0])
		}
		if !lanternApprox(got[1], white[1], 1e-15) || got[1] <= 0 {
			t.Fatalf("kill red on face %d: green changed %v -> %v", kill, white[1], got[1])
		}
	}
}

func TestLanternFaceLimitsAndIDs(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{0, 0, 0}, Step: 1, Size: [3]int{2, 2, 2}},
		Exposure: 1,
		Blocks:   []Box{{Min: Vec{0, 0, 0}, Max: Vec{1, 1, 1}}},
	}
	for i := 0; i < 17; i++ {
		r.Faces = append(r.Faces, Face{ID: string(rune('a' + i)), Center: Vec{1, 0.5, 0.5}, Normal: Vec{1, 0, 0}, Reflectance: Vec{1, 1, 1}})
	}
	if err := validate(r); err == nil {
		t.Fatal("17 faces must be rejected")
	}
	r.Faces = r.Faces[:1]
	r.Faces[0].ID = ""
	if err := validate(r); err == nil {
		t.Fatal("empty ID must be rejected")
	}
	r.Faces[0].ID = "x"
	r.Faces[0].Reflectance = Vec{2, 0, 0}
	if err := validate(r); err == nil {
		t.Fatal("reflectance above 1 must be rejected")
	}
}

func TestLanternWireAliasesAndNestedFaces(t *testing.T) {
	canonical := `{
      "Grid":{"Origin":[0,0,0],"Step":1,"Size":[2,2,2]},"Exposure":1,
      "Blocks":[{"Min":[0,0,0],"Max":[1,1,1]}],
      "Faces":[{"ID":"a","Center":[1,0.5,0.5],"Normal":[1,0,0],"Reflectance":[0.2,0.4,0.6]}]}`
	var r1 Request
	if err := json.Unmarshal([]byte(canonical), &r1); err != nil {
		t.Fatal(err)
	}
	if err := validate(r1); err != nil {
		t.Fatal(err)
	}

	nested := `{
      "grid":{"origin":[0,0,0],"step":1,"size":[2,2,2]},"exposure":1,
      "blocks":[{"min":[0,0,0],"max":[1,1,1],
        "reflective_faces":[{"id":"a","normal":[1,0,0],"albedo":[0.2,0.4,0.6]}]}]}`
	var r2 Request
	if err := json.Unmarshal([]byte(nested), &r2); err != nil {
		t.Fatal(err)
	}
	if err := validate(r2); err != nil {
		t.Fatalf("nested face with derived center: %v", err)
	}
	if len(r2.Faces) != 1 || r2.Faces[0].Center != (Vec{1, .5, .5}) {
		t.Fatalf("derived center wrong: %+v", r2.Faces)
	}
	if r2.Faces[0].Reflectance != r1.Faces[0].Reflectance {
		t.Fatal("alias reflectance mismatch")
	}

	colorMap := `{
      "Grid":{"Origin":[0,0,0],"Step":1,"Size":[2,2,2]},"Exposure":1,
      "Blocks":[{"Min":[0,0,0],"Max":[1,1,1]}],
      "Faces":[{"ID":"a","Center":[1,0.5,0.5],"Normal":[1,0,0],"Reflectance":[1,1,1]}],
      "FaceColors":{"a":[0.2,0.4,0.6]}}`
	var r3 Request
	if err := json.Unmarshal([]byte(colorMap), &r3); err != nil {
		t.Fatal(err)
	}
	if r3.Faces[0].Reflectance != (Vec{.2, .4, .6}) {
		t.Fatalf("color map not applied: %v", r3.Faces[0].Reflectance)
	}

	var unknown Request
	if err := json.Unmarshal([]byte(`{"Bogus":1}`), &unknown); err == nil {
		t.Fatal("unknown top level field must error")
	}
}
