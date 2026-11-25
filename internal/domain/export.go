package domain

import "encoding/json"

const Filename = "probe-volume.zip"

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
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
