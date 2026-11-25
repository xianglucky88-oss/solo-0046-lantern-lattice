package domain

func (g Grid) position(x, y, z int) Vec {
	return g.Origin.add(Vec{float64(x), float64(y), float64(z)}.scale(g.Step))
}
func (g Grid) index(x, y, z int) int { return x + g.Size[0]*(y+g.Size[1]*z) }
func (g Grid) count() int            { return g.Size[0] * g.Size[1] * g.Size[2] }
