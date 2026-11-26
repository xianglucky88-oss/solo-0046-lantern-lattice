package domain

import "encoding/json"

type Metadata struct {
	Schema                      string
	Grid                        Grid
	Axes                        [6]string
	Exposure                    float64
	TextureWidth, TextureHeight int
	Statistics                  Stats
	Samples                     []Sample
	Preview                     *Basis `json:"Preview,omitempty"`
}

func describe(r Request, probes []Probe, basis *Basis) Metadata {
	s := make([]Sample, 0, len(r.Samples))
	for _, p := range r.Samples {
		s = append(s, sample(r.Grid, probes, p, r.Blocks))
	}
	return Metadata{"lantern-probes/v1", r.Grid, axisNames, r.Exposure,
		r.Grid.Size[0] * 6, r.Grid.Size[1] * r.Grid.Size[2],
		inspect(probes, r.Exposure), s, basis}
}

// MarshalJSON keeps face free packages byte compatible with the historical
// layout and mirrors the preview basis under the most likely tooling keys.
func (m Metadata) MarshalJSON() ([]byte, error) {
	type alias Metadata
	raw, err := json.Marshal(alias(m))
	if err != nil || m.Preview == nil {
		return raw, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if preview, ok := object["Preview"]; ok {
		for _, key := range []string{"Offline", "Recolor", "Basis", "FaceBasis"} {
			object[key] = preview
		}
	}
	return json.Marshal(object)
}
