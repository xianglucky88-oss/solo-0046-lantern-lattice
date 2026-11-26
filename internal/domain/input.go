package domain

type Request struct {
	Grid      Grid
	Lights    []Light
	Blocks    []Box
	Samples   []Vec
	Exposure  float64
	Animation *Animation
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

// Animation turns a static bake into a time-varying bake. The grid, lights,
// static Blocks and Samples stay fixed; Movers are additional axis-aligned
// solid boxes with fixed size that translate at constant speed between keys.
type Animation struct {
	Range   [2]float64
	Shutter Shutter
	Movers  []Mover
}
type Shutter struct {
	Start, End float64
}
type Mover struct {
	Size Vec
	Keys []MotionKey
}

// MotionKey pins the mover's Min corner at Time. Times are strictly
// increasing and cover the whole animation range.
type MotionKey struct {
	Time     float64
	Position Vec
}
