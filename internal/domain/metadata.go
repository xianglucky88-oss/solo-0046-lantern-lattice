package domain

type Metadata struct {
	Schema        string
	Grid          Grid
	Axes          [6]string
	Exposure      float64
	TextureWidth  int
	TextureHeight int
	Statistics    Stats
	Samples       []Sample

	// Tiled packages (schema lantern-probes/v2) declare every page and fragment.
	// Legacy single-strip packages (v1) leave Pages and Tiles empty.
	Pages []PageSpec `json:",omitempty"`
	Tiles []TileRef  `json:",omitempty"`
}

const (
	schemaV1 = "lantern-probes/v1"
	schemaV2 = "lantern-probes/v2"
)

func describe(r Request, probes []Probe) Metadata {
	s := make([]Sample, 0, len(r.Samples))
	for _, p := range r.Samples {
		s = append(s, sample(r.Grid, probes, p, r.Blocks))
	}
	return Metadata{
		Schema:        schemaV1,
		Grid:          r.Grid,
		Axes:          axisNames,
		Exposure:      r.Exposure,
		TextureWidth:  r.Grid.Size[0] * 6,
		TextureHeight: r.Grid.Size[1] * r.Grid.Size[2],
		Statistics:    inspect(probes, r.Exposure),
		Samples:       s,
	}
}

func describeTiled(r Request, probes []Probe, pages []PageSpec, tiles []TileRef) Metadata {
	m := describe(r, probes)
	m.Schema = schemaV2
	m.Pages = pages
	m.Tiles = tiles
	return m
}
