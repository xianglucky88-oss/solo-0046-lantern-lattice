package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Request struct {
	Grid     Grid
	Lights   []Light
	Blocks   []Box
	Samples  []Vec
	Normals  []Vec `json:"Normals,omitempty"`
	Exposure float64
}
type Grid struct {
	Origin Vec
	Step   float64
	Size   [3]int
}
type Light struct {
	Position, RGB Vec
	Radius, Power float64
}
type Box struct{ Min, Max Vec }

// normalsEnabled reports whether the optional receiver-normal capability was
// requested. A nil Normals slice is the historical document and keeps the
// legacy six-direction result byte for byte. An explicitly supplied (even
// empty) list opts in.
func (r Request) normalsEnabled() bool { return r.Normals != nil }

// UnmarshalJSON keeps the established strict top-level contract while adding
// the optional receiver normals. Besides the canonical parallel "Normals"
// array, norms may also be embedded in "Samples" entries as
// {"Position":[...],"Normal":[...]}, or fused into six-element rows.
func (r *Request) UnmarshalJSON(data []byte) error {
	*r = Request{}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return err
	}
	normalsSet := false
	var separateNormals []Vec
	for key, raw := range fields {
		switch canonicalKey(key) {
		case "grid":
			if err := strictDecode(raw, &r.Grid); err != nil {
				return err
			}
		case "lights":
			if err := strictDecode(raw, &r.Lights); err != nil {
				return err
			}
		case "blocks":
			if err := strictDecode(raw, &r.Blocks); err != nil {
				return err
			}
		case "samples":
			positions, embedded, err := decodeSamples(raw)
			if err != nil {
				return err
			}
			r.Samples = positions
			if embedded != nil {
				r.Normals = embedded
			}
		case "normals":
			normalsSet = true
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				separateNormals = nil
				normalsSet = false
				continue
			}
			if err := json.Unmarshal(raw, &separateNormals); err != nil {
				return err
			}
		case "exposure":
			if err := json.Unmarshal(raw, &r.Exposure); err != nil {
				return err
			}
		default:
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	if normalsSet {
		r.Normals = separateNormals
		if r.Normals == nil {
			// "Normals": [] with no samples still opts in.
			r.Normals = []Vec{}
		}
	}
	return nil
}

func canonicalKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	switch b.String() {
	case "grid", "lights", "blocks", "samples", "exposure":
		return b.String()
	case "normals", "samplenormals", "receivernormals", "receivernormal",
		"querynormals", "surfacenormals", "directions", "normal":
		return "normals"
	}
	return b.String()
}

func strictDecode(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

// decodeSamples accepts the canonical position array and also object entries
// carrying an optional receiver normal, or a fused six-component row.
func decodeSamples(raw json.RawMessage) ([]Vec, []Vec, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, nil, err
	}
	positions := make([]Vec, 0, len(items))
	normals := make([]Vec, len(items))
	anyNormal := false
	for i, item := range items {
		item = bytes.TrimSpace(item)
		if len(item) == 0 {
			return nil, nil, fmt.Errorf("sample %d is empty", i)
		}
		switch item[0] {
		case '[':
			var parts []float64
			if err := json.Unmarshal(item, &parts); err != nil {
				return nil, nil, err
			}
			switch len(parts) {
			case 3:
				positions = append(positions, Vec{parts[0], parts[1], parts[2]})
			case 6:
				positions = append(positions, Vec{parts[0], parts[1], parts[2]})
				normals[i] = Vec{parts[3], parts[4], parts[5]}
				anyNormal = true
			default:
				return nil, nil, fmt.Errorf("sample %d must have 3 or 6 components", i)
			}
		case '{':
			pos, nrm, hasNormal, err := decodeSampleObject(item)
			if err != nil {
				return nil, nil, err
			}
			positions = append(positions, pos)
			if hasNormal {
				normals[i] = nrm
				anyNormal = true
			}
		default:
			return nil, nil, fmt.Errorf("sample %d must be an array or object", i)
		}
	}
	if anyNormal {
		return positions, normals, nil
	}
	return positions, nil, nil
}

func decodeSampleObject(raw json.RawMessage) (Vec, Vec, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Vec{}, Vec{}, false, err
	}
	var pos, nrm Vec
	hasPos, hasNormal := false, false
	for key, value := range fields {
		switch canonicalKey(key) {
		case "position", "point", "p", "query", "sample":
			if err := json.Unmarshal(value, &pos); err != nil {
				return Vec{}, Vec{}, false, err
			}
			hasPos = true
		case "normals":
			if err := json.Unmarshal(value, &nrm); err != nil {
				return Vec{}, Vec{}, false, err
			}
			hasNormal = true
		default:
			return Vec{}, Vec{}, false, fmt.Errorf("json: unknown field %q", key)
		}
	}
	if !hasPos {
		return Vec{}, Vec{}, false, fmt.Errorf("sample object needs Position")
	}
	return pos, nrm, hasNormal, nil
}
