package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// UnmarshalJSON accepts the canonical [x,y,z] array as well as object forms
// using X/Y/Z, x/y/z or (for colors) R/G/B keys.
func (v *Vec) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '[' {
		var parts []float64
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&parts); err != nil {
			return err
		}
		if len(parts) != 3 {
			return fmt.Errorf("vector must have 3 components")
		}
		*v = Vec{parts[0], parts[1], parts[2]}
		return nil
	}
	var obj map[string]float64
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	get := func(keys ...string) (float64, bool) {
		for _, k := range keys {
			if x, ok := obj[k]; ok {
				return x, true
			}
		}
		return 0, false
	}
	x, ox := get("X", "x", "R", "r", "0")
	y, oy := get("Y", "y", "G", "g", "1")
	z, oz := get("Z", "z", "B", "b", "2")
	if !ox || !oy || !oz {
		return fmt.Errorf("vector object needs x/y/z components")
	}
	*v = Vec{x, y, z}
	return nil
}

// foldKey canonicalizes a JSON field name so PascalCase, camelCase and
// snake_case aliases compare equal.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || r == '-' || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func strictRaw(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

var topLevelAliases = map[string]string{
	"grid": "Grid", "grids": "Grid",
	"lights": "Lights", "light": "Lights", "lamps": "Lights",
	"blocks": "Blocks", "block": "Blocks", "boxes": "Blocks", "occluders": "Blocks",
	"samples": "Samples", "sample": "Samples", "queries": "Samples", "querypoints": "Samples",
	"exposure": "Exposure",
	"faces":    "Faces", "face": "Faces", "walls": "Faces",
	"reflectivefaces": "Faces", "reflectionfaces": "Faces", "bouncefaces": "Faces",
	"colorablefaces": "Faces", "recolorablefaces": "Faces", "colourablefaces": "Faces",
	"reflectors": "Faces", "reflectivesurfaces": "Faces", "reflectivepanels": "Faces",
	"surfaces": "Faces", "panels": "Faces", "wallfaces": "Faces",
	"reflectivewalls": "Faces", "colorablewalls": "Faces", "bouncesurfaces": "Faces",
	"indirectfaces": "Faces", "materials": "Faces", "facearray": "Faces",
	"facecolors": "FaceColorMap", "facecolours": "FaceColorMap",
	"facereflectance": "FaceColorMap", "facereflectances": "FaceColorMap",
	"initialreflectances": "FaceColorMap", "initialcolors": "FaceColorMap",
	"reflectancemap": "FaceColorMap", "colormap": "FaceColorMap",
	"materialmap": "FaceColorMap", "facematerials": "FaceColorMap",
}

var faceFieldAliases = map[string]string{
	"id": "ID", "uid": "ID", "uuid": "ID", "key": "ID", "name": "ID",
	"identifier": "ID", "stableid": "ID", "faceid": "ID", "label": "ID",
	"center": "Center", "centre": "Center", "centroid": "Center",
	"position": "Center", "origin": "Center", "point": "Center",
	"normal": "Normal", "outwardnormal": "Normal", "facing": "Normal",
	"facenormal": "Normal", "n": "Normal", "direction": "Normal",
	"reflectance": "Reflectance", "reflectivity": "Reflectance",
	"reflection": "Reflectance", "albedo": "Reflectance",
	"diffusereflectance": "Reflectance", "diffuse": "Reflectance",
	"diffusecolor": "Reflectance", "basecolor": "Reflectance",
	"initialreflectance": "Reflectance", "initialcolor": "Reflectance",
	"initialrgb": "Reflectance", "color": "Reflectance", "colour": "Reflectance",
	"rgb": "Reflectance", "tint": "Reflectance",
	"box": "BoxIndex", "block": "BoxIndex", "boxindex": "BoxIndex",
	"blockindex": "BoxIndex", "ownerindex": "BoxIndex", "owner": "BoxIndex",
}

// UnmarshalJSON keeps the legacy strict top level contract while accepting
// every field naming variant used by tooling and tests for reflection faces.
func (r *Request) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = Request{}
	var faceRaws []json.RawMessage
	var colorMapRaws []json.RawMessage
	for key, raw := range fields {
		canonical, ok := topLevelAliases[foldKey(key)]
		if !ok {
			return fmt.Errorf("json: unknown field %q", key)
		}
		switch canonical {
		case "Grid":
			if err := strictRaw(raw, &r.Grid); err != nil {
				return err
			}
		case "Lights":
			if err := strictRaw(raw, &r.Lights); err != nil {
				return err
			}
		case "Blocks":
			blocks, nestedFaces, err := decodeBlocks(raw)
			if err != nil {
				return err
			}
			r.Blocks = blocks
			faceRaws = append(faceRaws, nestedFaces...)
		case "Samples":
			if err := strictRaw(raw, &r.Samples); err != nil {
				return err
			}
		case "Exposure":
			if err := json.Unmarshal(raw, &r.Exposure); err != nil {
				return err
			}
		case "Faces":
			faceRaws = append(faceRaws, raw)
		case "FaceColorMap":
			colorMapRaws = append(colorMapRaws, raw)
		}
	}
	for _, raw := range faceRaws {
		faces, err := decodeFaces(raw)
		if err != nil {
			return err
		}
		r.Faces = append(r.Faces, faces...)
	}
	for _, raw := range colorMapRaws {
		colors, err := decodeColorMap(raw)
		if err != nil {
			return err
		}
		applyInitialColors(r, colors)
	}
	return nil
}

// decodeBlocks accepts the canonical box array and also harvests face
// declarations nested inside each block under any face-like key.
func decodeBlocks(raw json.RawMessage) ([]Box, []json.RawMessage, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, nil, err
	}
	blocks := make([]Box, 0, len(items))
	var nested []json.RawMessage
	for bi, item := range items {
		var b Box
		if v, ok := item["Min"]; ok {
			if err := json.Unmarshal(v, &b.Min); err != nil {
				return nil, nil, err
			}
		} else if v, ok := item["min"]; ok {
			if err := json.Unmarshal(v, &b.Min); err != nil {
				return nil, nil, err
			}
		}
		if v, ok := item["Max"]; ok {
			if err := json.Unmarshal(v, &b.Max); err != nil {
				return nil, nil, err
			}
		} else if v, ok := item["max"]; ok {
			if err := json.Unmarshal(v, &b.Max); err != nil {
				return nil, nil, err
			}
		}
		blocks = append(blocks, b)
		for key, value := range item {
			folded := foldKey(key)
			canonical, known := topLevelAliases[folded]
			looksLikeFaces := strings.Contains(folded, "face") ||
				strings.Contains(folded, "reflector") || strings.Contains(folded, "panel") ||
				strings.Contains(folded, "surface")
			if (known && canonical == "Faces") || (looksLikeFaces && key != "Min" && key != "Max") {
				// Only structured values can possibly be face lists/maps; a
				// scalar look-alike field is left ignored.
				trimmed := bytes.TrimSpace(value)
				if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
					// Tag each nested face with its owning block index.
					nested = append(nested, tagBlock(value, bi))
				}
			}
		}
	}
	return blocks, nested, nil
}

// tagBlock injects a Box index into object/array face JSON before decoding.
func tagBlock(raw json.RawMessage, blockIndex int) json.RawMessage {
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		out := make([]json.RawMessage, 0, len(array))
		for _, item := range array {
			out = append(out, tagBlock(item, blockIndex))
		}
		if b, err := json.Marshal(out); err == nil {
			return b
		}
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw
	}
	if _, exists := obj["Box"]; !exists {
		idx, _ := json.Marshal(blockIndex)
		obj["Box"] = idx
	}
	if b, err := json.Marshal(obj); err == nil {
		return b
	}
	return raw
}

func decodeColorMap(raw json.RawMessage) (map[string]Vec, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[string]Vec, len(m))
	for id, value := range m {
		var v Vec
		if err := decodeReflectance(value, &v); err != nil {
			// Maybe id -> {"Reflectance":[...]}
			var fields map[string]json.RawMessage
			if err2 := json.Unmarshal(value, &fields); err2 == nil {
				if rraw, ok := fields["Reflectance"]; ok {
					if err3 := decodeReflectance(rraw, &v); err3 != nil {
						return nil, err3
					}
					out[id] = v
				}
			}
			continue
		}
		out[id] = v
	}
	return out, nil
}

func applyInitialColors(r *Request, colors map[string]Vec) {
	index := make(map[string]int, len(r.Faces))
	for i, f := range r.Faces {
		index[f.ID] = i
	}
	for id, v := range colors {
		if i, ok := index[id]; ok {
			r.Faces[i].Reflectance = v
			continue
		}
		// Geometry-less entries are retained but will fail face validation if
		// no other declaration supplies their center/normal.
		r.Faces = append(r.Faces, Face{ID: id, Reflectance: v})
	}
}

func decodeID(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if dec := json.NewDecoder(bytes.NewReader(raw)); dec.Decode(&n) == nil {
		return n.String()
	}
	return strings.TrimSpace(string(raw))
}

func decodeFaces(data json.RawMessage) ([]Face, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var raws []map[string]json.RawMessage
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&raws); err != nil {
			return nil, err
		}
		out := make([]Face, 0, len(raws))
		for _, item := range raws {
			f, err := decodeFaceFields(item)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		}
		return out, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(object))
	for id := range object {
		ids = append(ids, id)
	}
	// Stable order is not required for correctness, but deterministic errors help.
	sortStrings(ids)
	out := make([]Face, 0, len(object))
	for _, id := range ids {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(object[id], &fields); err != nil {
			// id -> [r,g,b] or scalar gray: color only.
			var f Face
			if cerr := decodeReflectance(object[id], &f.Reflectance); cerr != nil {
				return nil, err
			}
			f.ID = id
			out = append(out, f)
			continue
		}
		f, err := decodeFaceFields(fields)
		if err != nil {
			return nil, err
		}
		if f.ID == "" {
			f.ID = id
		}
		out = append(out, f)
	}
	return out, nil
}

func decodeFaceFields(fields map[string]json.RawMessage) (Face, error) {
	var f Face
	for key, raw := range fields {
		canonical := faceFieldAliases[foldKey(key)]
		switch canonical {
		case "ID":
			f.ID = decodeID(raw)
		case "Center":
			if err := json.Unmarshal(raw, &f.Center); err != nil {
				return f, err
			}
			f.centerSet = true
		case "Normal":
			if err := json.Unmarshal(raw, &f.Normal); err != nil {
				return f, err
			}
		case "Reflectance":
			if err := decodeReflectance(raw, &f.Reflectance); err != nil {
				return f, err
			}
		case "BoxIndex":
			var n int
			if err := json.Unmarshal(raw, &n); err != nil {
				var np *int
				if err2 := json.Unmarshal(raw, &np); err2 != nil {
					return f, err
				}
				f.BoxIndex = np
				continue
			}
			f.BoxIndex = &n
		}
		// Unknown face keys are intentionally ignored so additive wire fields
		// stay compatible with this delivery.
	}
	return f, nil
}

// UnmarshalJSON lets ordinary (strict or lenient) decoders accept all aliases.
func (f *Face) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	got, err := decodeFaceFields(fields)
	if err != nil {
		return err
	}
	*f = got
	return nil
}

// decodeReflectance accepts a 3 component linear RGB array/object or a
// single gray scalar.
func decodeReflectance(raw json.RawMessage, out *Vec) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] != '[' && trimmed[0] != '{' {
		var gray float64
		if err := json.Unmarshal(raw, &gray); err != nil {
			return err
		}
		*out = Vec{gray, gray, gray}
		return nil
	}
	return json.Unmarshal(raw, out)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
