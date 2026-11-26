package domain

import "encoding/json"

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	if r.Animation == nil {
		return buildStatic(r)
	}
	return buildMotion(r)
}

func buildStatic(r Request) ([]byte, error) {
	probes := bake(r)
	metadata, err := json.MarshalIndent(describe(r, probes), "", "  ")
	if err != nil {
		return nil, err
	}
	bytes, err := texture(r.Grid, probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	return pack(metadata, bytes)
}

func buildMotion(r Request) ([]byte, error) {
	intervals := make([][]moverInterval, len(r.Animation.Movers))
	for i, m := range r.Animation.Movers {
		intervals[i] = moverIntervals(m)
	}
	timeline, bakes := bakeTimeline(r, intervals)
	preview := previews(r.Grid, timeline, r.Blocks, intervals, *r.Animation)
	motionSamples := make([]MotionSample, 0, len(r.Samples))
	for _, p := range r.Samples {
		motionSamples = append(motionSamples, motionSample(r.Grid, timeline, p, r.Blocks, intervals, *r.Animation))
	}
	metadata, err := json.MarshalIndent(describeMotion(r, timeline, preview, bakes, motionSamples), "", "  ")
	if err != nil {
		return nil, err
	}
	atlas, err := motionTexture(r.Grid, preview, r.Exposure)
	if err != nil {
		return nil, err
	}
	timelineBytes, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		return nil, err
	}
	return packMotion(metadata, atlas, timelineBytes)
}
