package domain

type Request struct {
	Grid     Grid
	Lights   []Light
	Blocks   []Box
	Samples  []Vec
	Exposure float64
	Rescue   *Rescue
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

// Rescue describes a single-bake probe relocation request. A small number of
// blocked grid slots may each be moved to one of a few candidate landing
// points so that named checkpoints reach a shared support policy.
type Rescue struct {
	MaxMove     float64
	Slots       []RescueSlot
	Checkpoints []Checkpoint
}

// RescueSlot names one blocked grid cell and the landing points it may use.
type RescueSlot struct {
	X, Y, Z    int
	Candidates []Vec
}

// Checkpoint is a support query with the minimum reduced weight it needs.
type Checkpoint struct {
	Position   Vec
	MinSupport float64
}
