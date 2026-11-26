package domain

// MotionSample is the time-averaged result of one query point over one
// shutter, using exactly the same trilinear neighbor weights as the static
// sampler.
type MotionSample struct {
	Position        Vec
	State           string
	Weight          float64
	SupportDuration float64
	SolidDuration   float64
	Irradiance      Lobes
}

func motionSample(g Grid, tl Timeline, p Vec, static []Box, intervals [][]moverInterval, a Animation) MotionSample {
	out := MotionSample{Position: p, State: "outside"}
	cell, ok := locate(g, p)
	if !ok {
		return out
	}
	neighbors := corners(g, cell)
	lo, hi := a.Shutter.Start, a.Shutter.End
	total := hi - lo

	events := make([]float64, 0)
	for _, iv := range intervals {
		events = append(events, pointEventTimes(p, iv, lo, hi)...)
		for _, n := range neighbors {
			probePos := g.position(
				n.index%g.Size[0],
				(n.index/g.Size[0])%g.Size[1],
				n.index/(g.Size[0]*g.Size[1]),
			)
			events = append(events, pointEventTimes(probePos, iv, lo, hi)...)
			events = append(events, segmentEventTimes(p, probePos, iv, lo, hi)...)
		}
	}
	// Lobe changes come from light-sightline crossings, which need not move
	// any geometry predicate, so pull the baked run boundaries in explicitly.
	for _, n := range neighbors {
		events = append(events, tl.runBreakpoints(n.index, lo, hi)...)
	}
	segments := shutterSegments(a, events)

	var supportDuration, solidDuration, colorWeight float64
	var colorIntegral Lobes
	for _, seg := range segments {
		mid := (seg.start + seg.end) / 2
		length := seg.end - seg.start
		if occupiedAt(p, static, intervals, mid) {
			solidDuration += length
			continue
		}
		// Per-moment support: trilinear weights of valid, visible neighbors,
		// normalized within this moment before contributing color.
		momentWeight := 0.0
		var momentColor Lobes
		for _, n := range neighbors {
			probePos := g.position(
				n.index%g.Size[0],
				(n.index/g.Size[0])%g.Size[1],
				n.index/(g.Size[0]*g.Size[1]),
			)
			if occupiedAt(probePos, static, intervals, mid) {
				continue
			}
			if !visibleAt(p, probePos, static, intervals, mid) {
				continue
			}
			momentWeight += n.weight
			lobes := tl.lobesAt(n.index, mid)
			for face := range momentColor {
				momentColor[face] = momentColor[face].add(lobes[face].scale(n.weight))
			}
		}
		if momentWeight <= 0 {
			continue
		}
		supportDuration += length
		normalizer := 1 / momentWeight
		for face := range momentColor {
			normalized := momentColor[face].scale(normalizer)
			colorIntegral[face] = colorIntegral[face].add(normalized.scale(length))
		}
		colorWeight += length
	}

	out.SolidDuration = solidDuration
	out.SupportDuration = supportDuration
	epsilon := total * 1e-10
	switch {
	case solidDuration >= total-epsilon:
		out.State = "solid"
		return out
	case supportDuration > epsilon:
		out.State = "sampled"
		out.Weight = supportDuration / total
		for face := range out.Irradiance {
			// Average over supported time only; supported-but-black moments
			// stay in this denominator.
			out.Irradiance[face] = colorIntegral[face].scale(1 / colorWeight)
		}
	default:
		out.State = "unsupported"
	}
	return out
}

// probePreview is the shutter average of one probe: linear lobes averaged
// over the times the probe is not inside any solid box, with no positive
// duration meaning a transparent pixel.
type probePreview struct {
	validDuration float64
	average       Lobes
}

func previews(g Grid, tl Timeline, static []Box, intervals [][]moverInterval, a Animation) []probePreview {
	lo, hi := a.Shutter.Start, a.Shutter.End
	out := make([]probePreview, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				p := g.position(x, y, z)
				events := make([]float64, 0)
				for _, iv := range intervals {
					events = append(events, pointEventTimes(p, iv, lo, hi)...)
				}
				events = append(events, tl.runBreakpoints(idx, lo, hi)...)
				segments := shutterSegments(a, events)
				var preview probePreview
				for _, seg := range segments {
					mid := (seg.start + seg.end) / 2
					if occupiedAt(p, static, intervals, mid) {
						continue
					}
					length := seg.end - seg.start
					lobes := tl.lobesAt(idx, mid)
					for face := range preview.average {
						preview.average[face] = preview.average[face].add(lobes[face].scale(length))
					}
					preview.validDuration += length
				}
				if preview.validDuration > 0 {
					for face := range preview.average {
						preview.average[face] = preview.average[face].scale(1 / preview.validDuration)
					}
				}
				out[idx] = preview
			}
		}
	}
	return out
}
