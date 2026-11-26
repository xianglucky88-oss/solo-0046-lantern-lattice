package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
)

// RecolorResult is the offline recomputation from a downloaded package.
type RecolorResult struct {
	Layout      Metadata
	Lobes       []Lobes
	Samples     []Sample
	Reflectance map[string]Vec
	PNG         []byte
	LayoutJSON  []byte
	ZIP         []byte
}

// RecolorZip recombines the frozen preview basis inside a downloaded package
// with ID keyed reflectance overrides. It never needs the source scene.
func RecolorZip(packageBytes []byte, overrides map[string]Vec) (*RecolorResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(packageBytes), int64(len(packageBytes)))
	if err != nil {
		return nil, err
	}
	layoutRaw, err := readZipMember(zr, "layout.json")
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(layoutRaw, &meta); err != nil {
		return nil, err
	}
	if meta.Preview == nil {
		return nil, fmt.Errorf("package has no Preview basis")
	}
	rho, err := meta.Preview.Reflectances(overrides)
	if err != nil {
		return nil, err
	}
	lobes := meta.Preview.Lobes(rho)
	samples := meta.Preview.SampleColors(lobes)
	meta.Samples = samples
	pngBytes, err := renderAtlas(meta.Grid, meta.Preview.ProbeValid, lobes, meta.Exposure)
	if err != nil {
		return nil, err
	}
	layoutJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	outZip, err := pack(layoutJSON, pngBytes)
	if err != nil {
		return nil, err
	}
	effective := make(map[string]Vec, len(meta.Preview.FaceIDs))
	for i, id := range meta.Preview.FaceIDs {
		effective[id] = rho[i]
	}
	return &RecolorResult{
		Layout:      meta,
		Lobes:       lobes,
		Samples:     samples,
		Reflectance: effective,
		PNG:         pngBytes,
		LayoutJSON:  layoutJSON,
		ZIP:         outZip,
	}, nil
}

func readZipMember(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("zip member %s not found", name)
}
