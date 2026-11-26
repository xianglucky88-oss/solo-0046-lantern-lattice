package domain

import (
	"math"
	"math/rand"
	"testing"
)

func claudeTransformedReference(t *testing.T, r Request, p Placement, world Vec) Sample {
	t.Helper()
	p, err := p.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	g := r.Grid
	var wg Grid
	wg.Step = g.Step * p.Scale
	for a := range 3 {
		k := p.Axis[a]
		wg.Size[a] = g.Size[k]
		lo := p.MapLocal(g.Origin)
		hiLocal := g.Origin
		hiLocal[k] += float64(g.Size[k]-1) * g.Step
		hi := p.MapLocal(hiLocal)
		if p.Sign[a] < 0 {
			wg.Origin[a] = hi[a]
		} else {
			wg.Origin[a] = lo[a]
		}
	}
	wr := Request{Grid: wg, Exposure: r.Exposure}
	for _, l := range r.Lights {
		l.Position = p.MapLocal(l.Position)
		wr.Lights = append(wr.Lights, l)
	}
	for _, b := range r.Blocks {
		a := p.MapLocal(b.Min)
		z := p.MapLocal(b.Max)
		var nb Box
		for axis := range 3 {
			nb.Min[axis] = math.Min(a[axis], z[axis])
			nb.Max[axis] = math.Max(a[axis], z[axis])
		}
		wr.Blocks = append(wr.Blocks, nb)
	}
	probes := bake(wr)
	return sample(wg, probes, world, wr.Blocks)
}

func TestClaudeResourcePlacementMatchesDirectInstances(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	maxDiff := 0.0
	for iter := 0; iter < 3000; iter++ {
		r := claudeRandomRequest(rng)
		res := bakeResource(r)
		perm := rng.Perm(3)
		p := Placement{
			Translation: Vec{rng.Float64()*4 - 2, rng.Float64()*4 - 2, rng.Float64()*4 - 2},
			Scale:       []float64{0.25, 0.5, 1, 1.7, 4}[rng.Intn(5)],
			Axis:        [3]int{perm[0], perm[1], perm[2]},
			Flip:        [3]bool{rng.Intn(2) == 0, rng.Intn(2) == 0, rng.Intn(2) == 0},
		}
		np, err := p.Normalize()
		if err != nil {
			t.Fatal(err)
		}
		q := Vec{
			rng.Float64() * float64(r.Grid.Size[0]-1),
			rng.Float64() * float64(r.Grid.Size[1]-1),
			rng.Float64() * float64(r.Grid.Size[2]-1),
		}.scale(r.Grid.Step).add(r.Grid.Origin)
		// Sometimes query just outside/tangent; usually inside the cell span.
		if rng.Intn(5) == 0 {
			q[rng.Intn(3)] += (rng.Float64() - .5) * r.Grid.Step * .4
		}
		world := np.MapLocal(q)
		got := res.Sample(p, world)
		want := claudeTransformedReference(t, r, p, world)
		if got.State != want.State {
			t.Fatalf("iter %d state = %q want %q p=%+v q=%v world=%v", iter, got.State, want.State, p, q, world)
		}
		if math.Abs(got.Weight-want.Weight) > 1e-9 {
			t.Fatalf("iter %d weight %.12f want %.12f", iter, got.Weight, want.Weight)
		}
		for face := range got.Irradiance {
			for c := range got.Irradiance[face] {
				if d := math.Abs(got.Irradiance[face][c] - want.Irradiance[face][c]); d > maxDiff {
					maxDiff = d
				}
				if math.Abs(got.Irradiance[face][c]-want.Irradiance[face][c]) > 1e-7 {
					t.Fatalf("iter %d face %d c %d = %.12g want %.12g", iter, face, c, got.Irradiance[face][c], want.Irradiance[face][c])
				}
			}
		}
	}
	t.Logf("max irradiance difference %.3g", maxDiff)
}

func claudeRandomRequest(rng *rand.Rand) Request {
	size := [3]int{2 + rng.Intn(4), 2 + rng.Intn(4), 2 + rng.Intn(4)}
	step := 0.25 + rng.Float64()*1.5
	r := Request{
		Grid:     Grid{Origin: Vec{0, 0, 0}, Step: step, Size: size},
		Exposure: 1,
	}
	for i := 0; i < 2+rng.Intn(3); i++ {
		r.Lights = append(r.Lights, Light{
			Position: Vec{rng.Float64()*4 - 1, rng.Float64()*3 - 1, rng.Float64()*3 - 1},
			RGB:      Vec{rng.Float64(), rng.Float64(), rng.Float64()},
			Radius:   0.5 + rng.Float64()*5,
			Power:    rng.Float64() * 8,
		})
	}
	for i := 0; i < 1+rng.Intn(3); i++ {
		center := Vec{rng.Float64()*3 - .5, rng.Float64()*2 - .5, rng.Float64()*2 - .5}
		h := Vec{.05 + rng.Float64()*.4, .05 + rng.Float64()*.4, .05 + rng.Float64()*.4}
		r.Blocks = append(r.Blocks, Box{Min: center.sub(h), Max: center.add(h)})
	}
	return r
}
