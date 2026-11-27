package domain

import (
	"encoding/json"
	"fmt"
	"math"
)

// SkyColors holds one linear RGB sky colour for each of the six fixed axes,
// aligned with axisNames/axes: +X, -X, +Y, -Y, +Z, -Z.
type SkyColors [6]Vec

// rayBlocked reports whether the infinite ray p+t*dir (t>=0, dir is a unit
// axis vector) intersects the closed axis-aligned box b. Tangential contact
// (near == far, or a zero component resting on a box face) counts as a hit,
// matching the closed-box convention used by intersects.
func rayBlocked(p, dir Vec, b Box) bool {
	near, far := 0.0, math.Inf(1)
	for axis := range 3 {
		if dir[axis] == 0 {
			if p[axis] < b.Min[axis] || p[axis] > b.Max[axis] {
				return false
			}
			continue
		}
		lo := (b.Min[axis] - p[axis]) / dir[axis]
		hi := (b.Max[axis] - p[axis]) / dir[axis]
		if lo > hi {
			lo, hi = hi, lo
		}
		if lo > near {
			near = lo
		}
		if hi < far {
			far = hi
		}
		if near > far {
			return false
		}
	}
	return true
}

// skyOcclusion returns, per axis, whether the sky ray is blocked by any box.
func skyOcclusion(p Vec, blocks []Box) [6]bool {
	var blocked [6]bool
	for i, dir := range axes {
		for _, b := range blocks {
			if rayBlocked(p, dir, b) {
				blocked[i] = true
				break
			}
		}
	}
	return blocked
}

// skyLobes assigns the axis sky colour to every unobstructed direction and
// zero to every direction whose far ray meets a solid box.
func skyLobes(p Vec, sky SkyColors, blocks []Box) (Lobes, int) {
	var out Lobes
	blocked := skyOcclusion(p, blocks)
	count := 0
	for i := range axes {
		if blocked[i] {
			count++
			continue
		}
		out[i] = sky[i]
	}
	return out, count
}

// SkyError marks a malformed sky document. BadRequest maps to HTTP 400;
// otherwise the document is understood but outside the domain (422).
type SkyError struct {
	BadRequest bool
	Message    string
}

func (e *SkyError) Error() string { return e.Message }

// ParseSky accepts the many wire shapes used to describe six axis colours:
// a six element array of RGB triples, an object with a Colors/Colours/Axes/
// Directions/Values field, an axis-name keyed object, or a single colour
// replicated to all six axes. A null/empty document reports set=false.
func ParseSky(raw json.RawMessage) (SkyColors, bool, error) {
	var out SkyColors
	trim := trimJSON(raw)
	if len(trim) == 0 || string(trim) == "null" {
		return out, false, nil
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return out, false, &SkyError{BadRequest: true, Message: "sky must be a colour set"}
	}

	cols, ok, err := extractSkyColors(doc)
	if err != nil {
		return out, false, err
	}
	if !ok {
		return out, false, nil
	}
	if len(cols) == 0 {
		return out, false, nil
	}
	switch len(cols) {
	case 6:
		for i := range 6 {
			out[i] = cols[i]
		}
	case 1:
		for i := range 6 {
			out[i] = cols[0]
		}
	case 3:
		// Three colours are interpreted as + axes then mirrored to - axes.
		for a := range 3 {
			out[2*a] = cols[a]
			out[2*a+1] = cols[a]
		}
	default:
		return out, false, &SkyError{Message: "sky needs one colour for each of six axes"}
	}
	for i, c := range out {
		for ch, v := range c {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e4 {
				return out, false, &SkyError{Message: fmt.Sprintf("sky axis %d channel %d outside linear range", i, ch)}
			}
		}
	}
	return out, true, nil
}

func trimJSON(raw json.RawMessage) []byte {
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\n' || raw[0] == '\r' || raw[0] == '\t') {
		raw = raw[1:]
	}
	return raw
}

func toVec(v any) (Vec, bool) {
	xs, ok := v.([]any)
	if !ok || len(xs) != 3 {
		return Vec{}, false
	}
	var p Vec
	for i := range 3 {
		f, ok := number(xs[i])
		if !ok {
			return Vec{}, false
		}
		p[i] = f
	}
	return p, true
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// colorFromNode reads an RGB triple from an array or from common object keys.
func colorFromNode(v any) (Vec, bool) {
	if p, ok := toVec(v); ok {
		return p, true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Vec{}, false
	}
	for _, k := range []string{"RGB", "Rgb", "rgb", "Color", "colour", "Colour", "Albedo", "Value", "value", "Channels", "channels", "Emission", "emission"} {
		if node, yes := m[k]; yes {
			if p, ok := toVec(node); ok {
				return p, true
			}
		}
	}
	// Object shaped {"R":r,"G":g,"B":b}.
	r, ok1 := number(m["R"])
	g, ok2 := number(m["G"])
	b, ok3 := number(m["B"])
	if !ok1 {
		r, _ = number(m["r"])
	}
	if !ok2 {
		g, _ = number(m["g"])
	}
	if !ok3 {
		b, _ = number(m["b"])
	}
	if ok1 || ok2 || ok3 {
		return Vec{r, g, b}, true
	}
	return Vec{}, false
}

func extractSkyColors(doc any) ([]Vec, bool, error) {
	switch node := doc.(type) {
	case float64:
		return []Vec{{node, node, node}}, true, nil
	case []any:
		if len(node) == 0 {
			return nil, true, nil
		}
		// A bare [r,g,b] is one linear RGB colour replicated to all axes.
		if len(node) == 3 {
			if c, ok := toVec(node); ok {
				return []Vec{c}, true, nil
			}
		}
		// Six scalars => six grey axis colours.
		if _, scalar := node[0].(float64); scalar {
			allScalar := true
			var grey []Vec
			for _, item := range node {
				f, ok := number(item)
				if !ok {
					allScalar = false
					break
				}
				grey = append(grey, Vec{f, f, f})
			}
			if allScalar {
				return grey, true, nil
			}
		}
		var cols []Vec
		for _, item := range node {
			c, ok := colorFromNode(item)
			if !ok {
				return nil, false, &SkyError{BadRequest: true, Message: "invalid sky colour entry"}
			}
			cols = append(cols, c)
		}
		return cols, true, nil
	case map[string]any:
		// Wrapper keys carrying the actual colour set.
		for _, k := range []string{"Colors", "colors", "Colours", "colours", "Axes", "axes", "Directions", "directions", "Values", "values", "Sky", "sky", "SkyColors", "Irradiance", "Probes"} {
			if inner, ok := node[k]; ok {
				cols, _, err := extractSkyColors(inner)
				if err != nil {
					return nil, false, err
				}
				if len(cols) > 0 {
					return cols, true, nil
				}
			}
		}
		// Axis-name keyed document.
		if cols, ok := axisKeyedColors(node); ok {
			return cols, true, nil
		}
		// Single RGB-like object.
		if c, ok := colorFromNode(node); ok {
			return []Vec{c}, true, nil
		}
		return nil, false, &SkyError{BadRequest: true, Message: "unrecognised sky document"}
	default:
		return nil, false, &SkyError{BadRequest: true, Message: "unrecognised sky document"}
	}
}

var axisKeyIndex = map[string]int{
	"+x": 0, "x+": 0, "px": 0, "posx": 0, "right": 0, "east": 0, "0": 0,
	"-x": 1, "x-": 1, "nx": 1, "negx": 1, "left": 1, "west": 1, "1": 1,
	"+y": 2, "y+": 2, "py": 2, "posy": 2, "up": 2, "top": 2, "2": 2,
	"-y": 3, "y-": 3, "ny": 3, "negy": 3, "down": 3, "bottom": 3, "3": 3,
	"+z": 4, "z+": 4, "pz": 4, "posz": 4, "forward": 4, "front": 4, "south": 4, "4": 4,
	"-z": 5, "z-": 5, "nz": 5, "negz": 5, "back": 5, "backward": 5, "rear": 5, "north": 5, "5": 5,
}

func axisKeyedColors(m map[string]any) ([]Vec, bool) {
	var slots [6]Vec
	seen := [6]bool{}
	anySeen := false
	for k, v := range m {
		idx, ok := axisKeyIndex[normalizeKey(k)]
		if !ok {
			continue
		}
		c, ok := colorFromNode(v)
		if !ok {
			continue
		}
		slots[idx] = c
		seen[idx] = true
		anySeen = true
	}
	if !anySeen {
		return nil, false
	}
	cols := make([]Vec, 6)
	for i := range 6 {
		if seen[i] {
			cols[i] = slots[i]
		}
	}
	return cols, true
}

func normalizeKey(k string) string {
	out := make([]rune, 0, len(k))
	for _, r := range k {
		if r == ' ' || r == '_' || r == '-' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

// ScaleClamp confines a preview multiplier to its legal [0,1] range. NaN is
// treated as zero so malformed arithmetic never reaches the encoder.
func ScaleClamp(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
