package domain

type Request struct {
	Grid     Grid    `json:"Grid"`
	Lights   []Light `json:"Lights"`
	Blocks   []Box   `json:"Blocks"`
	Samples  []Vec   `json:"Samples"`
	Exposure float64 `json:"Exposure"`
	Faces    []Face  `json:"Faces,omitempty"`
}
type Grid struct {
	Origin Vec
	Step   float64
	Size   [3]int
}
type Light struct {
	Position, RGB Vec
	Radius, Power float64
}
type Box struct{ Min, Max Vec }

// Face is one recolorable surface patch represented by the center of a
// face of an existing axis aligned Box and its outward unit normal.
type Face struct {
	ID          string `json:"ID"`
	Center      Vec    `json:"Center"`
	Normal      Vec    `json:"Normal"`
	Reflectance Vec    `json:"Reflectance"`
	BoxIndex    *int   `json:"Box,omitempty"`

	// Resolved geometry, filled in by validate. Not part of the wire contract.
	unit      Vec
	anchor    *anchor
	centerSet bool
}

// anchor identifies the exact closed box face a reflection point belongs to,
// so endpoint contacts with that face can be exempted from occlusion.
type anchor struct {
	box  int
	axis int
	side float64 // -1 for Min face, +1 for Max face
}
