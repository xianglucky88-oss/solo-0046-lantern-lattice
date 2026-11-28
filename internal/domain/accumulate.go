package domain

func incoming(p Vec, lights []Light, blocks []Box, wall wallBasis) (Lobes, int, int) {
	var result Lobes
	var visibleCount, blockedCount int
	for _, lamp := range lights {
		dir := lamp.Position.sub(p)
		d := dir.length()
		w := attenuation(d, lamp.Radius, lamp.Power)
		if w == 0 {
			continue
		}
		if !visible(p, lamp.Position, blocks, wall) {
			blockedCount++
			continue
		}
		visibleCount++
		result.project(dir, d, lamp.RGB.scale(w))
	}
	return result, visibleCount, blockedCount
}
