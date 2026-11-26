package domain

import (
	"math"
	"testing"
)

func moverForTest() Mover {
	return Mover{
		Size: Vec{0.2, 2, 2},
		Keys: []MotionKey{
			{Time: 0, Position: Vec{-0.1, -1, -1}},
			{Time: 3, Position: Vec{2.9, -1, -1}},
		},
	}
}

func animationForTest(shutter Shutter) *Animation {
	return &Animation{
		Range:   [2]float64{0, 3},
		Shutter: shutter,
		Movers:  []Mover{moverForTest()},
	}
}

func TestMoverBoxAt(t *testing.T) {
	intervals := moverIntervals(moverForTest())
	b := boxAt(intervals, 1.5)
	if math.Abs(b.Min[0]-1.4) > 1e-12 || math.Abs(b.Max[0]-1.6) > 1e-12 {
		t.Fatalf("unexpected box at t=1.5: %+v", b)
	}
	b = boxAt(intervals, 0)
	if math.Abs(b.Min[0]-(-0.1)) > 1e-12 || math.Abs(b.Max[1]-1.0) > 1e-12 {
		t.Fatalf("key-time box wrong: %+v", b)
	}
}

func TestPointEventTimesIsolatedContact(t *testing.T) {
	// A box face that only reaches the point at one instant yields a single
	// event time; intervals on either side must classify consistently.
	m := Mover{
		Size: Vec{1, 1, 1},
		Keys: []MotionKey{
			{Time: 0, Position: Vec{-2, -1, -1}},
			{Time: 3, Position: Vec{0, -1, -1}},
		},
	}
	intervals := moverIntervals(m)
	events := pointEventTimes(Vec{0, 0, 0}, intervals, 0, 3)
	found := false
	for _, e := range events {
		if math.Abs(e-1.5) < 1e-9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tangent event at t=1.5, got %v", events)
	}
	// Before t=1.5 the point is outside the closed box; after, it is inside.
	if inside(Vec{0, 0, 0}, boxAt(intervals, 1.5-1e-6)) {
		t.Fatal("point must be outside before the face arrives")
	}
	if !inside(Vec{0, 0, 0}, boxAt(intervals, 1.5+1e-6)) {
		t.Fatal("point must be inside once the face has passed")
	}
	// At the exact tangent time the closed box contains the boundary point.
	if !inside(Vec{0, 0, 0}, boxAt(intervals, math.Nextafter(1.5, math.Inf(1)))) {
		t.Fatal("closed box must contain a tangent boundary point")
	}
}

func TestValidateAnimationBoundaries(t *testing.T) {
	base := Request{
		Grid:     Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:   []Light{{Position: Vec{-2, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 12, Power: 4}},
		Exposure: 3,
		Animation: &Animation{
			Range:   [2]float64{0, 3},
			Shutter: Shutter{Start: 0, End: 3},
			Movers:  []Mover{moverForTest()},
		},
	}
	cases := []func(*Animation){
		func(a *Animation) { a.Range = [2]float64{3, 0} },
		func(a *Animation) { a.Shutter = Shutter{Start: 2, End: 1} },
		func(a *Animation) { a.Shutter = Shutter{Start: -0.1, End: 1} },
		func(a *Animation) { a.Shutter = Shutter{Start: 0, End: 3.1} },
		func(a *Animation) {
			a.Movers[0].Keys = append([]MotionKey(nil), a.Movers[0].Keys...)
			a.Movers[0].Keys[1].Time = 2
		},
		func(a *Animation) { a.Movers[0].Size[0] = 0 },
		func(a *Animation) { a.Movers = nil },
	}
	for i, mutate := range cases {
		req := base
		animation := *base.Animation
		// Deep-copy the mover so one case cannot mutate later cases.
		animation.Movers = append([]Mover(nil), base.Animation.Movers...)
		mutate(&animation)
		req.Animation = &animation
		if err := validate(req); err == nil {
			t.Fatalf("case %d: expected validation failure", i)
		}
	}
	// Endpoint shutter is accepted.
	if err := validate(base); err != nil {
		t.Fatalf("endpoint shutter must validate: %v", err)
	}
}

func TestBakeTimelineRunCoverage(t *testing.T) {
	r := Request{
		Grid:      Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:    []Light{{Position: Vec{-2, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 12, Power: 4}},
		Exposure:  3,
		Animation: animationForTest(Shutter{Start: 0, End: 3}),
	}
	if err := validate(r); err != nil {
		t.Fatal(err)
	}
	intervals := make([][]moverInterval, len(r.Animation.Movers))
	for i, m := range r.Animation.Movers {
		intervals[i] = moverIntervals(m)
	}
	timeline, bakes := bakeTimeline(r, intervals)
	if len(timeline.Tracks) != r.Grid.count() {
		t.Fatalf("tracks=%d want %d", len(timeline.Tracks), r.Grid.count())
	}
	if timeline.Times[0] != 0 || timeline.Times[len(timeline.Times)-1] != 3 {
		t.Fatal("time table must cover the full animation range")
	}
	for i, track := range timeline.Tracks {
		runs := track.Runs
		if runs[0].From != 0 || runs[len(runs)-1].To != len(timeline.Times)-1 {
			t.Fatalf("track %d does not cover the time table: %+v", i, runs)
		}
		for j := 1; j < len(runs); j++ {
			if runs[j-1].To != runs[j].From {
				t.Fatalf("track %d run %d has a gap", i, j)
			}
			if runs[j-1].Lobes == runs[j].Lobes {
				t.Fatalf("track %d adjacent runs are identical", i)
			}
		}
		if bakes[i].occluded < 0 {
			t.Fatal("occlusion count cannot be negative")
		}
	}
}

func TestMotionSampleSolidAndSupportDurations(t *testing.T) {
	r := Request{
		Grid:      Grid{Origin: Vec{}, Step: 1, Size: [3]int{3, 2, 2}},
		Lights:    []Light{{Position: Vec{-2, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 12, Power: 4}},
		Exposure:  3,
		Samples:   []Vec{{1, 0, 0}},
		Animation: animationForTest(Shutter{Start: 0, End: 3}),
	}
	intervals := make([][]moverInterval, len(r.Animation.Movers))
	for i, m := range r.Animation.Movers {
		intervals[i] = moverIntervals(m)
	}
	timeline, _ := bakeTimeline(r, intervals)
	ms := motionSample(r.Grid, timeline, Vec{1, 0, 0}, r.Blocks, intervals, *r.Animation)
	if ms.State != "sampled" {
		t.Fatalf("state=%s want sampled", ms.State)
	}
	// Door covers x=1 on [0.9, 1.1]: solid 0.2s of the 3s shutter.
	if math.Abs(ms.SolidDuration-0.2) > 1e-9 {
		t.Fatalf("solid duration=%v want 0.2", ms.SolidDuration)
	}
	if math.Abs(ms.Weight-ms.SupportDuration/3) > 1e-9 {
		t.Fatalf("weight must equal support fraction: %v vs %v", ms.Weight, ms.SupportDuration/3)
	}
	if ms.SupportDuration <= 0 {
		t.Fatal("expected positive supported time")
	}

	// A narrow shutter fully inside the solid window must report solid.
	narrow := *r.Animation
	narrow.Shutter = Shutter{Start: 0.95, End: 1.05}
	ms = motionSample(r.Grid, timeline, Vec{1, 0, 0}, r.Blocks, intervals, narrow)
	if ms.State != "solid" {
		t.Fatalf("state=%s want solid", ms.State)
	}
}

func TestPreviewTransparentVsOpaqueBlack(t *testing.T) {
	// The mover never reaches the grid, so every probe keeps full valid
	// duration while the truncated lamp leaves them all at zero energy.
	animation := &Animation{
		Range:   [2]float64{0, 3},
		Shutter: Shutter{Start: 0, End: 3},
		Movers: []Mover{{
			Size: Vec{0.2, 0.2, 0.2},
			Keys: []MotionKey{
				{Time: 0, Position: Vec{10, 10, 10}},
				{Time: 3, Position: Vec{12, 12, 12}},
			},
		}},
	}
	r := Request{
		Grid:      Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:    []Light{{Position: Vec{-50, -50, -50}, RGB: Vec{1, 1, 1}, Radius: 0.1, Power: 1}},
		Exposure:  3,
		Animation: animation,
	}
	intervals := make([][]moverInterval, len(r.Animation.Movers))
	for i, m := range r.Animation.Movers {
		intervals[i] = moverIntervals(m)
	}
	timeline, _ := bakeTimeline(r, intervals)
	preview := previews(r.Grid, timeline, r.Blocks, intervals, *r.Animation)
	atlas, err := motionTexture(r.Grid, preview, r.Exposure)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas) == 0 {
		t.Fatal("empty texture")
	}
	for i, pv := range preview {
		if math.Abs(pv.validDuration-3) > 1e-9 {
			t.Fatalf("probe %d valid duration=%v want 3", i, pv.validDuration)
		}
		for face, lobe := range pv.average {
			for _, e := range lobe {
				if e != 0 {
					t.Fatalf("probe %d face %d expected zero energy, got %v", i, face, e)
				}
			}
		}
	}
	stats := inspectMotion(preview, make([]probeBake, len(preview)), r.Exposure)
	if stats.Valid != 8 || stats.Dark != 8 || stats.Invalid != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestStaticArtifactUnchangedWhenNoAnimation(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{}, Step: 1, Size: [3]int{2, 2, 2}},
		Lights:   []Light{{Position: Vec{-2, 0, 0}, RGB: Vec{1, 1, 1}, Radius: 12, Power: 4}},
		Exposure: 3,
	}
	artifact, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact) == 0 {
		t.Fatal("empty static artifact")
	}
	probes := bake(r)
	if len(probes) != 8 || !probes[0].Valid {
		t.Fatal("static bake regressed")
	}
}

// TestLampOnlyBreakpoint guards the run-boundary splice: a mover that shades
// lamp sightlines without touching any probe or neighbor segment still has to
// split the color integration at the baked lobe change.
func TestLampOnlyBreakpoint(t *testing.T) {
	r := Request{
		Grid:     Grid{Origin: Vec{}, Step: 1, Size: [3]int{3, 2, 2}},
		Lights:   []Light{{Position: Vec{-3, 2, 0}, RGB: Vec{1, 1, 1}, Radius: 20, Power: 4}},
		Exposure: 3,
		Samples:  []Vec{{1, 0.5, 0}},
		Animation: &Animation{
			Range:   [2]float64{0, 3},
			Shutter: Shutter{Start: 1.8, End: 2.4},
			Movers: []Mover{{
				Size: Vec{4, 0.2, 4},
				Keys: []MotionKey{
					{Time: 0, Position: Vec{-2, 0, -2}},
					{Time: 3, Position: Vec{-2, 2.6, -2}},
				},
			}},
		},
	}
	intervals := make([][]moverInterval, len(r.Animation.Movers))
	for i, m := range r.Animation.Movers {
		intervals[i] = moverIntervals(m)
	}
	timeline, _ := bakeTimeline(r, intervals)
	ms := motionSample(r.Grid, timeline, Vec{1, 0.5, 0}, r.Blocks, intervals, *r.Animation)
	if ms.State != "sampled" || math.Abs(ms.Weight-1) > 1e-9 {
		t.Fatalf("unexpected sample: %+v", ms)
	}
	// The slab never meets the sample: no solid or lost-support time.
	if ms.SolidDuration != 0 || math.Abs(ms.SupportDuration-0.6) > 1e-9 {
		t.Fatalf("durations wrong: solid=%v support=%v", ms.SolidDuration, ms.SupportDuration)
	}
	// At least one neighbor must have a lobe breakpoint strictly inside the
	// shutter, and the averaged -X face must differ from either constant piece.
	cell, _ := locate(r.Grid, Vec{1, 0.5, 0})
	neighbors := corners(r.Grid, cell)
	hadInternalBreak := false
	for _, n := range neighbors {
		for _, bt := range timeline.runBreakpoints(n.index, 1.8, 2.4) {
			if bt > 1.8 && bt < 2.4 {
				hadInternalBreak = true
			}
		}
	}
	if !hadInternalBreak {
		t.Fatal("expected an in-shutter lamp-sightline breakpoint")
	}
	// Average must be strictly between the blocked (zero) and lit lobe values.
	negX := ms.Irradiance[1]
	for _, e := range negX {
		if !(e > 0) {
			t.Fatalf("expected positive averaged -X energy, got %v", negX)
		}
	}
	// The same probe evaluated fully inside the blocked window must be black.
	blocked := *r.Animation
	blocked.Shutter = Shutter{Start: 1.1, End: 1.7}
	msBlocked := motionSample(r.Grid, timeline, Vec{1, 0.5, 0}, r.Blocks, intervals, blocked)
	if msBlocked.State != "sampled" {
		t.Fatalf("blocked window still has support, got state=%s", msBlocked.State)
	}
	for face, lobe := range msBlocked.Irradiance {
		for c, e := range lobe {
			if e != 0 {
				t.Fatalf("blocked window face %d channel %d must be zero, got %v", face, c, e)
			}
		}
	}
}
