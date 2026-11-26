package domain

import (
	"encoding/json"
	"errors"
)

const Filename = "probe-volume.zip"

// Build bakes the probe volume. When both texture limits are given the output is
// a tiled package whose every page fits the device limits; otherwise the legacy
// single-strip package is produced.
func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	if (r.MaxTextureWidth == nil) != (r.MaxTextureHeight == nil) {
		return nil, errors.New("texture limits require both MaxTextureWidth and MaxTextureHeight")
	}
	probes := bake(r)
	atlas := logicalAtlas(r.Grid, probes, r.Exposure)

	if r.MaxTextureWidth == nil {
		metadata, err := json.MarshalIndent(describe(r, probes), "", "  ")
		if err != nil {
			return nil, err
		}
		pngBytes, err := encodePNG(atlas)
		if err != nil {
			return nil, err
		}
		return pack(metadata, pngBytes)
	}

	rendered, refs := buildTiled(atlas, r.Grid, *r.MaxTextureWidth, *r.MaxTextureHeight)
	specs := make([]PageSpec, len(rendered))
	for i, pg := range rendered {
		specs[i] = pg.spec
	}
	metadata, err := json.MarshalIndent(describeTiled(r, probes, specs, refs), "", "  ")
	if err != nil {
		return nil, err
	}
	return packTiled(metadata, rendered)
}
