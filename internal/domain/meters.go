package domain

type Stats struct {
	Valid, Invalid, Dark, OccludedRays, SaturatedChannels int
	Maximum                                               float64

	// Motion fields are populated only for time-varying bakes; static
	// artifacts omit them.
	ValidDuration float64 `json:",omitempty"`
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

// inspectMotion summarizes the preview texture: probes with any valid time
// in the shutter count as valid, and an all-zero average is an opaque black
// pixel rather than a transparent one.
func inspectMotion(previews []probePreview, bakes []probeBake, exposure float64) Stats {
	var s Stats
	for i, preview := range previews {
		if preview.validDuration <= 0 {
			s.Invalid++
			continue
		}
		s.Valid++
		s.ValidDuration += preview.validDuration
		s.OccludedRays += bakes[i].occluded
		dark := true
		for _, lobe := range preview.average {
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
