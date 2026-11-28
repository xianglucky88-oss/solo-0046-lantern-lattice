package domain

type Probe struct {
	Position               Vec
	Valid                  bool
	Irradiance             Lobes
	Contributing, Occluded int
}

// bake evaluates every slot of every grid. Slots on the closed wall, or inside
// a solid box (boundary included), stay invalid/transparent; the rest gather
// point-light energy with wall and box occlusion.
func bake(r Request, p plan) [][]Probe {
	out := make([][]Probe, len(p.grids))
	for gi, g := range p.grids {
		probes := make([]Probe, g.count())
		for z := range g.Size[2] {
			for y := range g.Size[1] {
				for x := range g.Size[0] {
					at := g.position(x, y, z)
					s := Probe{Position: at, Valid: !occupied(at, r.Blocks) && !p.wall.onClosedWall(at)}
					if s.Valid {
						s.Irradiance, s.Contributing, s.Occluded = incoming(at, r.Lights, r.Blocks, p.wall)
					}
					probes[g.index(x, y, z)] = s
				}
			}
		}
		out[gi] = probes
	}
	return out
}
