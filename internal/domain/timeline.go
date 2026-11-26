package domain

import "sort"

// motionSegment is one interval on which every geometric predicate used by
// the bake has a constant answer.
type motionSegment struct {
	start, end float64
}

func shutterSegments(animation Animation, events []float64) []motionSegment {
	lo, hi := animation.Shutter.Start, animation.Shutter.End
	times := uniqueTimes(append(events, lo, hi), lo, hi)
	out := make([]motionSegment, 0, len(times)-1)
	for i := 0; i+1 < len(times); i++ {
		if times[i+1] > times[i] {
			out = append(out, motionSegment{times[i], times[i+1]})
		}
	}
	return out
}

// TrackRun is one constant-lobe interval of a probe over the animation
// range. Times are indices into Timeline.Times.
type TrackRun struct {
	From, To int
	Lobes    Lobes
}

// ProbeTrack stores a probe's baked irradiance as piecewise-constant runs
// covering the full time table. Adjacent runs always differ in at least one
// channel.
type ProbeTrack struct {
	Index int
	Runs  []TrackRun
}

// Timeline is the offline, self-contained time-varying probe resource. It
// needs no light list: the lobes are already baked, and geometry (static
// blocks plus movers in the layout document) lets the consumer reproduce the
// same visibility and occupancy predicates.
type Timeline struct {
	Schema  string
	Range   [2]float64
	Times   []float64
	Tracks  []ProbeTrack
	Version int
}

type probeBake struct {
	runs     []TrackRun
	occluded int
}

func timeIndex(times []float64, t float64) int {
	i := sort.Search(len(times), func(i int) bool { return times[i] >= t })
	if i >= len(times) {
		return len(times) - 1
	}
	return i
}

func bakeTimeline(r Request, intervals [][]moverInterval) (Timeline, []probeBake) {
	a := *r.Animation
	lo, hi := a.Range[0], a.Range[1]
	g := r.Grid

	// Global breakpoints: every mover key plus every state-change time for
	// every probe. Between consecutive times all predicates are constant.
	events := make([]float64, 0)
	for _, m := range a.Movers {
		for _, key := range m.Keys {
			events = append(events, key.Time)
		}
	}
	probeEvents := make([][]float64, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				p := g.position(x, y, z)
				ev := make([]float64, 0)
				for _, iv := range intervals {
					ev = append(ev, pointEventTimes(p, iv, lo, hi)...)
					for _, lamp := range r.Lights {
						ev = append(ev, segmentEventTimes(p, lamp.Position, iv, lo, hi)...)
					}
				}
				probeEvents[idx] = ev
				events = append(events, ev...)
			}
		}
	}
	times := uniqueTimes(events, lo, hi)
	if times[0] != lo {
		times = append([]float64{lo}, times...)
	}
	if times[len(times)-1] != hi {
		times = append(times, hi)
	}

	bakes := make([]probeBake, g.count())
	tracks := make([]ProbeTrack, 0, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				p := g.position(x, y, z)
				local := uniqueTimes(append(append([]float64{}, probeEvents[idx]...), lo, hi), lo, hi)
				segments := make([]motionSegment, 0, len(local)-1)
				for i := 0; i+1 < len(local); i++ {
					segments = append(segments, motionSegment{local[i], local[i+1]})
				}
				bake := probeBake{}
				track := ProbeTrack{Index: idx}
				var previous Lobes
				runStart := 0
				flush := func(endIndex int) {
					track.Runs = append(track.Runs, TrackRun{From: runStart, To: endIndex, Lobes: previous})
				}
				for si, seg := range segments {
					mid := (seg.start + seg.end) / 2
					moving := motionBoxesAt(intervals, mid)
					blocks := append(append([]Box{}, r.Blocks...), moving...)
					lobes, _, blocked := incoming(p, r.Lights, blocks)
					bake.occluded += blocked
					if si == 0 {
						previous = lobes
						runStart = timeIndex(times, seg.start)
					} else if lobes != previous {
						flush(timeIndex(times, seg.start))
						previous = lobes
						runStart = timeIndex(times, seg.start)
					}
				}
				if len(segments) > 0 {
					flush(timeIndex(times, segments[len(segments)-1].end))
				} else {
					track.Runs = append(track.Runs, TrackRun{From: 0, To: len(times) - 1, Lobes: Lobes{}})
				}
				bake.runs = track.Runs
				bakes[idx] = bake
				tracks = append(tracks, track)
			}
		}
	}

	return Timeline{
		Schema:  "lantern-timeline/v2",
		Range:   a.Range,
		Times:   times,
		Tracks:  tracks,
		Version: 2,
	}, bakes
}

// lobesAt looks up a probe's baked lobes at time t using its constant runs.
func (tl Timeline) lobesAt(probeIndex int, t float64) Lobes {
	track := tl.Tracks[probeIndex]
	for _, run := range track.Runs {
		if t >= tl.Times[run.From] && t <= tl.Times[run.To] {
			return run.Lobes
		}
	}
	return track.Runs[len(track.Runs)-1].Lobes
}

// runBreakpoints returns the probe's lobe-change times inside [lo, hi].
// Shutter integration must split on these too: a light-sightline crossing
// changes the baked color without changing any geometry predicate.
func (tl Timeline) runBreakpoints(probeIndex int, lo, hi float64) []float64 {
	out := make([]float64, 0)
	for _, run := range tl.Tracks[probeIndex].Runs {
		for _, idx := range [2]int{run.From, run.To} {
			if t := tl.Times[idx]; t >= lo && t <= hi {
				out = append(out, t)
			}
		}
	}
	return out
}
