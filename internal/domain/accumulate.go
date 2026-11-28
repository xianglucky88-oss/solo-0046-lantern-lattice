package domain

// Beam is one incident light beam that survived occlusion and distance
// attenuation at a probe. Direction points from the probe toward the light
// and Distance is zero only when the light coincides with the probe.
type Beam struct {
	Direction Vec
	Distance  float64
	RGB       Vec
}

// beams collects the scene's incident beams at p. Occlusion and attenuation
// are resolved once here so every receiver normal sees the same geometry.
func beams(p Vec, lights []Light, blocks []Box) ([]Beam, int, int) {
	var out []Beam
	var visibleCount, blockedCount int
	for _, lamp := range lights {
		dir := lamp.Position.sub(p)
		d := dir.length()
		w := attenuation(d, lamp.Radius, lamp.Power)
		if w == 0 {
			continue
		}
		if !visible(p, lamp.Position, blocks) {
			blockedCount++
			continue
		}
		visibleCount++
		out = append(out, Beam{Direction: dir, Distance: d, RGB: lamp.RGB.scale(w)})
	}
	return out, visibleCount, blockedCount
}

// normalEnergy is the linear RGB response of a receiver with direction
// normal at p: each incident beam contributes inside the hemisphere and
// weighted by the incident cosine.
func normalEnergy(incident []Beam, normal Vec) Vec {
	var out Vec
	for _, beam := range incident {
		out.projectNormal(normal, beam.Direction, beam.Distance, beam.RGB)
	}
	return out
}
