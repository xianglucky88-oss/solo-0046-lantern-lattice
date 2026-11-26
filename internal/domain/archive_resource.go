package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ResourceFromLayoutJSON extracts a resource from an encoded layout.json.
func ResourceFromLayoutJSON(raw []byte) (*Resource, error) {
	var doc struct {
		Resource         *Resource `json:"Resource"`
		ProbeResource    *Resource `json:"ProbeResource"`
		InstanceResource *Resource `json:"InstanceResource"`
		Asset            *Resource `json:"Asset"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	for _, candidate := range []*Resource{doc.Resource, doc.ProbeResource, doc.InstanceResource, doc.Asset} {
		if candidate != nil && len(candidate.Probes) > 0 {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("probe resource not found in layout")
}

// ReadResourceArchive extracts the exact Resource from a finished ZIP.  The
// ordinary artifact embeds it in layout.json; standalone resource JSON members
// produced by older/draft packagers are accepted as well.
func ReadResourceArchive(data []byte) (*Resource, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	// Prefer layout.json because it is part of every current artifact.
	for _, f := range zr.File {
		if f.Name != "layout.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if resource, err := ResourceFromLayoutJSON(raw); err == nil {
			return resource, nil
		}
	}

	names := map[string]bool{
		"resource.json":          true,
		"probe-resource.json":    true,
		"instance-resource.json": true,
		"instances.json":         true,
		"asset.json":             true,
		"probe-asset.json":       true,
		"cabinet.json":           true,
	}
	for _, f := range zr.File {
		if !names[f.Name] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		return DecodeResource(raw)
	}
	return nil, fmt.Errorf("probe resource not found in archive")
}

// ResourceFromZip is a naming alias for ReadResourceArchive.
func ResourceFromZip(data []byte) (*Resource, error) { return ReadResourceArchive(data) }

// LoadResourceArchive is a naming alias for ReadResourceArchive.
func LoadResourceArchive(data []byte) (*Resource, error) { return ReadResourceArchive(data) }

// ReadProbeResource accepts either standalone resource JSON or the finished
// ZIP archive downloaded from the service.
func ReadProbeResource(data []byte) (*Resource, error) {
	if r, err := DecodeResource(data); err == nil && len(r.Probes) > 0 {
		return r, nil
	}
	return ReadResourceArchive(data)
}
func LoadProbeResource(data []byte) (*Resource, error) { return ReadProbeResource(data) }
func ProbeResourceFromZip(data []byte) (*Resource, error) {
	return ReadResourceArchive(data)
}
func AssetFromZip(data []byte) (*Resource, error) { return ReadResourceArchive(data) }
func ReadAsset(data []byte) (*Resource, error)    { return ReadProbeResource(data) }
func LoadAsset(data []byte) (*Resource, error)    { return ReadProbeResource(data) }
func OpenResource(data []byte) (*Resource, error) { return ReadProbeResource(data) }

// DecodeProbeResource decodes standalone JSON resource bytes.
func DecodeProbeResource(data []byte) (*Resource, error) { return DecodeResource(data) }

// BuildResourceArchive returns the ordinary proof artifact plus a standalone
// exact resource member.  It is useful for engines that prefer a direct JSON
// sidecar after download.
func BuildResourceArchive(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	res := bakeResource(r)
	meta, err := marshalLayout(describe(r, res.Probes), res)
	if err != nil {
		return nil, err
	}
	png, err := texture(r.Grid, res.Probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	resourceJSON, err := EncodeResource(res)
	if err != nil {
		return nil, err
	}
	return packMembers([][2][]byte{
		{[]byte("layout.json"), meta},
		{[]byte("probes.png"), png},
		{[]byte("probe-resource.json"), resourceJSON},
	})
}

func packMembers(files [][2][]byte) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range files {
		name, data := string(file[0]), file[1]
		dest, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err := dest.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
