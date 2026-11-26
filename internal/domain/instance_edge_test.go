package domain

import (
	"math"
	"testing"
)

func TestClaudeResourceArchiveRoundTripAndShrinkRelighting(t *testing.T) {
	r := Request{
		Grid: Grid{Origin: Vec{0, 0, 0}, Step: 0.1, Size: [3]int{3, 2, 2}},
		Lights: []Light{{
			Position: Vec{-0.5, 0, 0},
			RGB:      Vec{1, 0.5, 0.25},
			Radius:   0.2,
			Power:    4,
		}},
		Exposure: 1,
	}
	archive, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := ReadResourceArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(asset.Visibility) != 1 || len(asset.Visibility[0]) != r.Grid.count() {
		t.Fatalf("bad visibility matrix: %#v", asset.Visibility)
	}

	// At original scale every probe is farther than the tiny world radius.
	identity := Placement{}
	before := asset.Sample(identity, Vec{0.05, 0, 0})
	if before.State != "sampled" {
		t.Fatalf("state = %q", before.State)
	}
	var total float64
	for _, lobe := range before.Irradiance {
		for _, v := range lobe {
			total += v
		}
	}
	if total != 0 {
		t.Fatalf("original-size light should not reach sample, got %+v", before.Irradiance)
	}

	// Quarter scale keeps radius in world units and brings the lamp close.
	small := Placement{Scale: 0.25}
	world := small.MapLocal(Vec{0.05, 0, 0})
	after := asset.Sample(small, world)
	if after.State != "sampled" || after.Weight <= 0 {
		t.Fatalf("shrunk sample state=%q weight=%v", after.State, after.Weight)
	}
	var shrunkTotal float64
	for _, lobe := range after.Irradiance {
		for _, v := range lobe {
			shrunkTotal += v
		}
	}
	if shrunkTotal <= 0 {
		t.Fatalf("shrunk showcase should be lit: %+v", after.Irradiance)
	}

	decoded, err := DecodeResource(claudeMustJSON(t, asset))
	if err != nil {
		t.Fatal(err)
	}
	fromJSON := decoded.Sample(small, world)
	if math.Abs(fromJSON.Weight-after.Weight) > 1e-12 {
		t.Fatalf("JSON roundtrip weight %v != %v", fromJSON.Weight, after.Weight)
	}
	for i := range after.Irradiance {
		for c := range after.Irradiance[i] {
			if math.Abs(fromJSON.Irradiance[i][c]-after.Irradiance[i][c]) > 1e-12 {
				t.Fatalf("JSON roundtrip irradiance mismatch at %d/%d", i, c)
			}
		}
	}
}

func claudeMustJSON(t *testing.T, r *Resource) []byte {
	t.Helper()
	data, err := EncodeResource(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestClaudeSignedAxisWorldLobeOrder(t *testing.T) {
	r := Request{
		Grid: Grid{Origin: Vec{-1, -1, -1}, Step: 1, Size: [3]int{3, 3, 3}},
		Lights: []Light{{
			Position: Vec{2, 0, 0},
			RGB:      Vec{1, 1, 1},
			Radius:   10,
			Power:    1,
		}},
	}
	asset := bakeResource(r)
	p := Placement{Scale: 1, Swap: [3]int{1, 0, 2}, FlipX: true}
	// Local origin sees the lamp in +X.  Swapping X/Y sends it to world +Y;
	// flipping world X is perpendicular and must not move the lobe.
	got := asset.Sample(p, p.MapLocal(Vec{0, 0, 0}))
	if got.Irradiance[0][0] != 0 {
		t.Fatalf("world +X should be dark after swap: %+v", got.Irradiance)
	}
	if got.Irradiance[1][0] != 0 {
		t.Fatalf("world -X should be dark after swap: %+v", got.Irradiance)
	}
	if got.Irradiance[2][0] <= 0 { // axes: 0 +X, 1 -X, 2 +Y.
		t.Fatalf("expected world +Y light at face 2, got %+v", got.Irradiance)
	}
	if got.Irradiance[3][0] != 0 {
		t.Fatalf("world -Y should be dark: %+v", got.Irradiance)
	}
}
