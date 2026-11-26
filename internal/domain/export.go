package domain

import "encoding/json"

const Filename = "probe-volume.zip"

// Build runs the full baking pipeline for an ordinary request.
func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	return Assemble(r)
}

// Assemble produces the content-addressed artifact from an already validated
// request. Fresh builds and continued revisions share this single code path so
// their ZIP bytes are identical for identical edited lights.
func Assemble(r Request) ([]byte, error) {
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
