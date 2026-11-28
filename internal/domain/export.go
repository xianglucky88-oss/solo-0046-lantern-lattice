package domain

import "encoding/json"

const Filename = "probe-volume.zip"

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	p := resolve(r)
	probes := bake(r, p)
	metadata, err := json.MarshalIndent(describe(r, p, probes), "", "  ")
	if err != nil {
		return nil, err
	}
	textures := make([][]byte, len(p.grids))
	names := make([]string, len(p.grids))
	for i, g := range p.grids {
		names[i] = p.textureFile(i)
		textures[i], err = texture(g, probes[i], r.Exposure)
		if err != nil {
			return nil, err
		}
	}
	return pack(metadata, names, textures)
}
