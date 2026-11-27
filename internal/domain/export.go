package domain

import "encoding/json"

const Filename = "probe-volume.zip"

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	probes := bake(r)
	base := describe(r, probes)

	var (
		metadata []byte
		err      error
	)
	if r.SkyEnabled() || r.EmbedLinear {
		metadata, err = json.MarshalIndent(extendMetadata(r, base, probes), "", "  ")
	} else {
		metadata, err = json.MarshalIndent(base, "", "  ")
	}
	if err != nil {
		return nil, err
	}
	atlas, err := texture(r.Grid, probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	return pack(metadata, atlas)
}
