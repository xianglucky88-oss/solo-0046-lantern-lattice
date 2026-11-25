package domain

type Stats struct {
	Valid, Invalid, Dark, OccludedRays, SaturatedChannels int
	Maximum                                               float64
}

func inspect(probes []Probe, exposure float64) Stats {
	var s Stats
	for _, p := range probes {
		if !p.Valid {
			s.Invalid++
			continue
		}
		s.Valid++
		s.OccludedRays += p.Occluded
		dark := true
		for _, lobe := range p.Irradiance {
			for _, e := range lobe {
				s.Maximum = max(s.Maximum, e)
				if e > 0 {
					dark = false
				}
				if quantize(e, exposure) == 255 {
					s.SaturatedChannels++
				}
			}
		}
		if dark {
			s.Dark++
		}
	}
	return s
}
