package domain

import "encoding/json"

const Filename = "probe-volume.zip"

// metadataDocument keeps Metadata's positional compatibility while adding the
// exact instance resource to the encoded layout.json.
type metadataDocument struct {
	Metadata
	Resource *Resource `json:"Resource,omitempty"`
}

func marshalLayout(meta Metadata, resource *Resource) ([]byte, error) {
	return json.MarshalIndent(metadataDocument{Metadata: meta, Resource: resource}, "", "  ")
}

func Build(r Request) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	resource := bakeResource(r)
	metadata, err := marshalLayout(describe(r, resource.Probes), resource)
	if err != nil {
		return nil, err
	}
	bytes, err := texture(r.Grid, resource.Probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	return pack(metadata, bytes)
}
