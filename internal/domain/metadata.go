package domain

type Metadata struct {
	Schema                      string
	Grid                        Grid
	Axes                        [6]string
	Exposure                    float64
	TextureWidth, TextureHeight int
	Statistics                  Stats
	Samples                     []Sample
}

func describe(r Request, probes []Probe) Metadata {
	s := make([]Sample, 0, len(r.Samples))
	for _, p := range r.Samples {
		s = append(s, sample(r.Grid, probes, p, r.Blocks))
	}
	return Metadata{"lantern-probes/v1", r.Grid, axisNames, r.Exposure, r.Grid.Size[0] * 6, r.Grid.Size[1] * r.Grid.Size[2], inspect(probes, r.Exposure), s}
}
