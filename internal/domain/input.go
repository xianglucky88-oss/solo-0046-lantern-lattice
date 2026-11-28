package domain

// Request is a single scene bake. The two-grid form drives the cross-door
// feature: two probe grids of differing spacing on opposite sides of one thin,
// tiltable wall with a rectangular doorway, sharing the same lights and solid
// boxes. The legacy single-grid form (Grid instead of Grids, no Wall) is still
// accepted and keeps its original blocking-only behaviour.
type Request struct {
	Grids      []Grid
	Grid       *Grid
	Wall       *Wall
	Transition float64
	Lights     []Light
	Blocks     []Box
	Samples    []Vec
	Exposure   float64
}

type Grid struct {
	// Side is -1 or +1: the wall side this grid nominally belongs to. It is
	// ignored (and omitted from layout.json) for a legacy single grid.
	Side   int `json:",omitempty"`
	Origin Vec
	Step   float64
	Size   [3]int
}

type Light struct {
	Position, RGB Vec
	Radius, Power float64
}

type Box struct{ Min, Max Vec }

// Wall is an infinitely thin rectangle centred at Center, spanned by the
// in-plane axes derived from Up and the plane normal Normal. Both in-plane
// extents are Half, measured from the centre. Opening is a rectangular hole in
// canonical (Right, Up) coordinates; the hole edge is still wall.
type Wall struct {
	Center  Vec
	Normal  Vec
	Up      Vec
	Half    [2]float64
	Opening Opening
}

type Opening struct{ Min, Max [2]float64 }
