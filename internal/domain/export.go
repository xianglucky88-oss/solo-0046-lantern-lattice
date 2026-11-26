package domain

import "encoding/json"

const Filename = "probe-volume.zip"

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	probes := bake(r)
	var report *RescueReport
	if r.Rescue != nil {
		plan := planRescue(r, probes)
		choice := plan.optimize()
		plan.apply(r, probes, choice)
		filled := buildRescueReport(r, probes, plan, choice)
		report = &filled
	}
	metadata, err := json.MarshalIndent(describe(r, probes, report), "", "  ")
	if err != nil {
		return nil, err
	}
	bytes, err := texture(r.Grid, probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	return pack(metadata, bytes)
}
