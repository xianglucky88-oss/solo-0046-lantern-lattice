package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// NormalResourceSchema labels the receiver-normal readout resource embedded in
// layout.json. It is emitted only when the build requested receiver normals.
const NormalResourceSchema = "lantern-normal-probes/v1"

// NormalResource is the offline readout asset: everything an engine needs to
// turn a downloaded layout.json plus a position and a receiver normal into
// linear RGB, with no server and no original scene. Every probe keeps its own
// resolved incident beams (occlusion and distance attenuation already applied)
// so a different normal changes only the cosine weighting.
type NormalResource struct {
	Schema string
	Grid   Grid
	Blocks []Box
	Probes []ProbeResource
}

// ProbeResource is one probe's frozen state and incident beams. Direction is
// the unit vector from the probe toward the light; Distance is zero only for a
// light coincident with the probe.
type ProbeResource struct {
	Position Vec
	Valid    bool
	Beams    []BeamResource
}

// BeamResource is a beam normalized for stable JSON: unit direction, distance
// and the linear RGB already scaled by power and distance attenuation.
type BeamResource struct {
	Direction Vec
	Distance  float64
	RGB       Vec
}

func resourceFromProbes(g Grid, probes []Probe, blocks []Box) *NormalResource {
	out := &NormalResource{
		Schema: NormalResourceSchema,
		Grid:   g,
		Blocks: append([]Box{}, blocks...),
		Probes: make([]ProbeResource, len(probes)),
	}
	for i, p := range probes {
		pr := ProbeResource{Position: p.Position, Valid: p.Valid}
		for _, b := range p.Incident {
			dir := b.Direction
			if b.Distance > 0 {
				dir = dir.scale(1 / b.Distance)
			} else {
				// Coincident light: keep an explicit zero direction so the
				// engine's coincidence rule (equal for every normal) holds.
				dir = Vec{}
			}
			pr.Beams = append(pr.Beams, BeamResource{Direction: dir, Distance: b.Distance, RGB: b.RGB})
		}
		if pr.Beams == nil {
			pr.Beams = []BeamResource{}
		}
		out.Probes[i] = pr
	}
	return out
}

// Beams restores the probe's incident beams in the form normal sampling uses.
func (p ProbeResource) incident() []Beam {
	out := make([]Beam, len(p.Beams))
	for i, b := range p.Beams {
		dir := b.Direction
		if b.Distance > 0 {
			dir = dir.scale(b.Distance)
		}
		out[i] = Beam{Direction: dir, Distance: b.Distance, RGB: b.RGB}
	}
	return out
}

// Sample returns the linear RGB response at world position p for receiver
// unit normal n. It uses only this resource and the supplied arguments:
// state, support and participating probes follow the established spatial
// rules, and each participant contributes its own hemispherical response.
func (r *NormalResource) Sample(p, n Vec) NormalSample {
	s := NormalSample{Position: p, Normal: n, State: "outside"}
	unit, ok := unitNormal(n)
	cell, located := locate(r.Grid, p)
	if !located {
		return s
	}
	if occupied(p, r.Blocks) {
		s.State = "solid"
		return s
	}
	s.State = "unsupported"
	var sum Vec
	for _, nbr := range corners(r.Grid, cell) {
		if nbr.index < 0 || nbr.index >= len(r.Probes) {
			continue
		}
		probe := r.Probes[nbr.index]
		if !probe.Valid || !visible(p, probe.Position, r.Blocks) {
			continue
		}
		s.Weight += nbr.weight
		if ok {
			sum = sum.add(normalEnergy(probe.incident(), unit).scale(nbr.weight))
		}
	}
	if s.Weight > 0 {
		s.State = "sampled"
		if ok {
			s.Irradiance = sum.scale(1 / s.Weight)
		}
	}
	return s
}

// EncodeNormalResource returns compact JSON for embedding or standalone use.
func EncodeNormalResource(r *NormalResource) ([]byte, error) {
	return json.Marshal(r)
}

// DecodeNormalResource parses a standalone receiver-normal resource document.
func DecodeNormalResource(data []byte) (*NormalResource, error) {
	var r NormalResource
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Schema == "" {
		return nil, fmt.Errorf("normal resource is missing its schema")
	}
	return &r, nil
}

// ReadNormalResource extracts the receiver-normal resource from a downloaded
// artifact ZIP (layout.json) or from standalone resource JSON. It needs only
// the bytes the caller already downloaded.
func ReadNormalResource(data []byte) (*NormalResource, error) {
	if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
		return normalResourceFromZip(zr)
	}
	if r, err := NormalResourceFromLayoutJSON(data); err == nil {
		return r, nil
	}
	if r, err := DecodeNormalResource(data); err == nil {
		return r, nil
	}
	return nil, fmt.Errorf("normal resource not found in download")
}

// NormalResourceFromLayoutJSON reads the resource from an encoded layout.json
// document (or a layout-like object that wraps one).
func NormalResourceFromLayoutJSON(raw []byte) (*NormalResource, error) {
	var doc struct {
		Normal         *NormalResource `json:"NormalResource"`
		ProbeResource  *NormalResource `json:"ProbeResource"`
		NormalProbeSet *NormalResource `json:"NormalProbes"`
		Offline        *NormalResource `json:"Offline"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid layout.json: %v", err)
	}
	for _, candidate := range []*NormalResource{doc.Normal, doc.ProbeResource, doc.NormalProbeSet, doc.Offline} {
		if candidate != nil && candidate.Schema == NormalResourceSchema {
			return candidate, nil
		}
	}
	if doc.Normal != nil {
		return doc.Normal, nil
	}
	return nil, fmt.Errorf("layout was built without receiver normals")
}

func normalResourceFromZip(zr *zip.Reader) (*NormalResource, error) {
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
		return NormalResourceFromLayoutJSON(raw)
	}
	return nil, fmt.Errorf("normal resource not found in archive")
}
