package domain

type Metadata struct {
	Schema                      string
	Grid                        Grid
	Axes                        [6]string
	Exposure                    float64
	TextureWidth, TextureHeight int
	Statistics                  Stats
	Samples                     []Sample
	Rescue                      *RescueReport `json:",omitempty"`
}

func describe(r Request, probes []Probe, report *RescueReport) Metadata {
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
		Rescue:        report,
	}
}
