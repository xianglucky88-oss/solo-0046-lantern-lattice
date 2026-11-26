package domain

type Metadata struct {
	Schema                      string
	Grid                        Grid
	Axes                        [6]string
	Exposure                    float64
	TextureWidth, TextureHeight int
	Statistics                  Stats
	Samples                     []Sample

	// Animation is present only for time-varying bakes. Static artifacts
	// keep the original lantern-probes/v1 document byte shape.
	Animation *AnimationMeta `json:",omitempty"`
	Timeline  string         `json:",omitempty"`
}

// AnimationMeta fully describes the moving geometry and the preview shutter
// inside the artifact, so a consumer can re-evaluate visibility, occupancy
// and support for any shutter or sample position without the light list.
type AnimationMeta struct {
	Range        [2]float64
	Shutter      Shutter
	Movers       []Mover
	StaticBlocks []Box
}

type MotionStats struct {
	Valid, Invalid, Dark, SaturatedChannels int
	ValidDuration                           float64
}

func describe(r Request, probes []Probe) Metadata {
	s := make([]Sample, 0, len(r.Samples))
	for _, p := range r.Samples {
		s = append(s, sample(r.Grid, probes, p, r.Blocks))
	}
	return Metadata{
		Schema:        "lantern-probes/v1",
		Grid:          r.Grid,
		Axes:          axisNames,
		Exposure:      r.Exposure,
		TextureWidth:  r.Grid.Size[0] * 6,
		TextureHeight: r.Grid.Size[1] * r.Grid.Size[2],
		Statistics:    inspect(probes, r.Exposure),
		Samples:       s,
	}
}

func describeMotion(r Request, tl Timeline, previews []probePreview, bakes []probeBake, motionSamples []MotionSample) Metadata {
	g := r.Grid
	samples := make([]Sample, 0, len(motionSamples))
	for _, ms := range motionSamples {
		samples = append(samples, Sample{
			Position:        ms.Position,
			State:           ms.State,
			Weight:          ms.Weight,
			Irradiance:      ms.Irradiance,
			SupportDuration: ms.SupportDuration,
			SolidDuration:   ms.SolidDuration,
		})
	}
	a := *r.Animation
	return Metadata{
		Schema:        "lantern-probes/v2",
		Grid:          g,
		Axes:          axisNames,
		Exposure:      r.Exposure,
		TextureWidth:  g.Size[0] * 6,
		TextureHeight: g.Size[1] * g.Size[2],
		Statistics:    inspectMotion(previews, bakes, r.Exposure),
		Samples:       samples,
		Animation: &AnimationMeta{
			Range:        a.Range,
			Shutter:      a.Shutter,
			Movers:       a.Movers,
			StaticBlocks: r.Blocks,
		},
		Timeline: TimelineName,
	}
}
