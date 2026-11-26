package domain

// ImportReport summarizes a successful offline import.
type ImportReport struct {
	Schema        string     `json:"Schema"`
	Grid          Grid       `json:"Grid"`
	Axes          [6]string  `json:"Axes"`
	Exposure      float64    `json:"Exposure"`
	TextureWidth  int        `json:"TextureWidth"`
	TextureHeight int        `json:"TextureHeight"`
	Pages         []PageSpec `json:"Pages,omitempty"`
	Tiles         []TileRef  `json:"Tiles,omitempty"`
}

// Report returns the verified structural description of an imported frame.
func (f *Frame) Report() ImportReport {
	return ImportReport{
		Schema:        f.Schema,
		Grid:          f.Grid,
		Axes:          f.Axes,
		Exposure:      f.Exposure,
		TextureWidth:  f.Grid.Size[0] * 6,
		TextureHeight: f.Grid.Size[1] * f.Grid.Size[2],
		Pages:         f.pages,
		Tiles:         f.tiles,
	}
}
