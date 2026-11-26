package domain

type Request struct {
	Grid            Grid
	Lights          []Light
	Blocks          []Box
	Samples         []Vec
	Exposure        float64
	MaxTextureWidth  *int
	MaxTextureHeight *int
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
