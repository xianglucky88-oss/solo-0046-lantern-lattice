package domain

// Metadata is the layout.json payload. The cross-door schema ("v2") lists both
// grids and the wall; the legacy single-grid schema ("v1") keeps its original
// flat Grid/TextureWidth/Statistics fields so earlier consumers still decode.
type Metadata struct {
	Schema        string
	Grid          *Grid      `json:",omitempty"`
	Grids         []GridInfo `json:",omitempty"`
	Wall          *WallInfo  `json:",omitempty"`
	Transition    float64    `json:",omitempty"`
	Axes          [6]string
	Exposure      float64
	TextureWidth  int    `json:",omitempty"`
	TextureHeight int    `json:",omitempty"`
	Statistics    *Stats `json:",omitempty"`
	Samples       []Sample
}

// WallInfo exposes the canonical orthonormal wall frame. Signed wall distance
// is (p-Center)·Normal, and opening coordinates are
// ((p-Center)·Right, (p-Center)·Up).
type WallInfo struct {
	Center  Vec
	Normal  Vec
	Up      Vec
	Right   Vec
	Half    [2]float64
	Opening Opening
}

type GridInfo struct {
	Side          int
	Origin        Vec
	Step          float64
	Size          [3]int
	Texture       string
	TextureWidth  int
	TextureHeight int
	Statistics    Stats
}

func describe(r Request, p plan, probes [][]Probe) Metadata {
	grids := make([]GridInfo, len(p.grids))
	for i, g := range p.grids {
		grids[i] = GridInfo{
			Side:          g.Side,
			Origin:        g.Origin,
			Step:          g.Step,
			Size:          g.Size,
			Texture:       p.textureFile(i),
			TextureWidth:  g.Size[0] * 6,
			TextureHeight: g.Size[1] * g.Size[2],
			Statistics:    inspect(probes[i], r.Exposure),
		}
	}
	samples := make([]Sample, 0, len(r.Samples))
	for _, at := range r.Samples {
		samples = append(samples, sample(r, p, probes, at))
	}
	if p.legacy {
		g := p.grids[0]
		stats := grids[0].Statistics
		return Metadata{
			Schema:        "lantern-probes/v1",
			Grid:          &g,
			Axes:          axisNames,
			Exposure:      r.Exposure,
			TextureWidth:  g.Size[0] * 6,
			TextureHeight: g.Size[1] * g.Size[2],
			Statistics:    &stats,
			Samples:       samples,
		}
	}
	m := Metadata{
		Schema:   "lantern-probes/v2",
		Grids:    grids,
		Axes:     axisNames,
		Exposure: r.Exposure,
		Samples:  samples,
	}
	if p.wall.active {
		m.Wall = &WallInfo{p.wall.center, p.wall.n, p.wall.u, p.wall.r, p.wall.half, r.Wall.Opening}
		m.Transition = p.transition
	}
	return m
}
