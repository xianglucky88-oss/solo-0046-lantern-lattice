package service

import (
	"example.local/lantern-lattice/internal/domain"
	"net/http"
)

var positionKeys = []string{
	"positions", "position", "samples", "points", "queries", "query",
	"at", "coords", "coordinates", "locations", "location", "pts", "xyz",
}

func extractPositions(m map[string]any, r *http.Request) []domain.Vec {
	var out []domain.Vec
	for _, k := range positionKeys {
		if v, ok := m[normKey(k)]; ok {
			out = append(out, positionsFromNode(v)...)
		}
	}
	out = append(out, queryPositions(r.URL.Query())...)
	return out
}

func positionsFromNode(node any) []domain.Vec {
	switch t := node.(type) {
	case []any:
		if len(t) == 0 {
			return nil
		}
		// A bare [x,y,z].
		if _, isNum := t[0].(float64); isNum {
			if v, ok := vecFromAny(t); ok {
				return []domain.Vec{v}
			}
		}
		var out []domain.Vec
		for _, item := range t {
			switch e := item.(type) {
			case []any:
				if v, ok := vecFromAny(e); ok {
					out = append(out, v)
				}
			case map[string]any:
				if v, ok := vecFromMap(e); ok {
					out = append(out, v)
				}
			}
		}
		return out
	case map[string]any:
		if v, ok := vecFromMap(t); ok {
			return []domain.Vec{v}
		}
	}
	return nil
}

func vecFromAny(t []any) (domain.Vec, bool) {
	if len(t) != 3 {
		return domain.Vec{}, false
	}
	var v domain.Vec
	for i := range 3 {
		f, ok := t[i].(float64)
		if !ok {
			return domain.Vec{}, false
		}
		v[i] = f
	}
	return v, true
}

func vecFromMap(m map[string]any) (domain.Vec, bool) {
	for _, k := range []string{"position", "at", "coord", "coordinate", "xyz", "p", "location"} {
		if n, ok := m[k]; ok {
			if arr, yes := n.([]any); yes {
				return vecFromAny(arr)
			}
		}
	}
	x, okx := m["x"].(float64)
	y, oky := m["y"].(float64)
	z, okz := m["z"].(float64)
	if okx && oky && okz {
		return domain.Vec{x, y, z}, true
	}
	return domain.Vec{}, false
}
