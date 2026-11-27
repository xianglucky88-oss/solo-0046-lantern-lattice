package domain

type Probe struct {
	Position               Vec
	Valid                  bool
	Irradiance             Lobes
	Lamp                   Lobes
	Sky                    Lobes
	Contributing, Occluded int
	LampOccluded           int
	SkyOccluded            int
}

func bake(r Request) []Probe {
	g := r.Grid
	skyOn := r.SkyEnabled()
	skyColors := r.EffectiveSky()
	out := make([]Probe, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				p := g.position(x, y, z)
				s := Probe{Position: p, Valid: !occupied(p, r.Blocks)}
				if s.Valid {
					lamp, contributing, blocked := incoming(p, r.Lights, r.Blocks)
					s.Lamp = lamp
					s.Contributing = contributing
					s.LampOccluded = blocked
					s.Occluded = blocked
					s.Irradiance = lamp
					if skyOn {
						sky, skyBlocked := skyLobes(p, skyColors, r.Blocks)
						s.Sky = sky
						s.SkyOccluded = skyBlocked
						s.Occluded = blocked + skyBlocked
						for i := range s.Irradiance {
							s.Irradiance[i] = s.Irradiance[i].add(sky[i])
						}
					}
				}
				out[g.index(x, y, z)] = s
			}
		}
	}
	return out
}
