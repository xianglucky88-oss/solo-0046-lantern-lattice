package domain

type Metadata struct {
	Schema                      string
	Grid                        Grid
	Axes                        [6]string
	Exposure                    float64
	TextureWidth, TextureHeight int
	Statistics                  Stats
	Samples                     []Sample

	// NormalSamples carries the optional receiver-normal readout for each
	// requested query. It is omitted entirely when normals are not enabled,
	// which keeps the legacy layout.json byte-identical.
	NormalSamples []NormalSample `json:",omitempty"`
	// NormalResource is the self-contained offline readout asset. Emitted
	// only when receiver normals were requested.
	NormalResource *NormalResource `json:",omitempty"`
}

func describe(r Request, probes []Probe) Metadata {
	s := make([]Sample, 0, len(r.Samples))
	for _, p := range r.Samples {
		s = append(s, sample(r.Grid, probes, p, r.Blocks))
	}
	m := Metadata{"lantern-probes/v1", r.Grid, axisNames, r.Exposure, r.Grid.Size[0] * 6, r.Grid.Size[1] * r.Grid.Size[2], inspect(probes, r.Exposure), s, nil, nil}
	if r.normalsEnabled() {
		m.NormalSamples = make([]NormalSample, 0, len(r.Samples))
		for i, p := range r.Samples {
			m.NormalSamples = append(m.NormalSamples, sampleNormal(r.Grid, probes, p, r.Normals[i], r.Blocks))
		}
		m.NormalResource = resourceFromProbes(r.Grid, probes, r.Blocks)
	}
	return m
}
