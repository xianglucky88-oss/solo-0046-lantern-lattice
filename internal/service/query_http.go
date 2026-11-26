package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"

	"example.local/lantern-lattice/internal/domain"
)

const maxQueryBytes = 16 << 20

func (s *Service) query(w http.ResponseWriter, r *http.Request) {
	pathDigest := r.PathValue("digest")
	r.Body = http.MaxBytesReader(w, r.Body, maxQueryBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		problem(w, 413, "query document too large")
		return
	}
	req := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			problem(w, 400, "invalid query document")
			return
		}
	}

	coarseRef := firstRaw(req, "coarse", "Coarse", "coarseDigest", "coarseArtifact", "coarseId",
		"coarseVolume", "coarseResource", "volume", "volumeDigest", "artifact", "digest", "id")
	if coarseRef == nil && pathDigest == "" {
		coarseRef = heuristicRef(req, "coarse")
	}
	coarseBlob, coarseDigest, err := s.resolveArtifactRef(coarseRef, pathDigest, firstRaw(req, "coarseArtifactBase64", "coarse_base64", "coarseBlobBase64"))
	if err != nil {
		problem(w, 404, "coarse tier not found: "+err.Error())
		return
	}
	doc, probes, err := domain.LoadCoarse(coarseBlob)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}

	points := parsePositions(firstRaw(req, "positions", "Positions", "points", "Points",
		"Samples", "samples", "queries", "coordinates", "locations", "queryPoints", "queryPositions", "ats"))
	if len(points) == 0 {
		// Singular forms: "position":[...] / "at":{...} / "query":[...].
		for _, k := range []string{"position", "Position", "at", "point", "query", "location"} {
			if raw := firstRaw(req, k); raw != nil {
				if v, ok := parseVec(raw); ok {
					points = []domain.Vec{v}
					break
				}
				if ps := parsePositions(raw); len(ps) > 0 {
					points = ps
					break
				}
			}
		}
	}
	if len(points) == 0 {
		points = heuristicPositions(req)
	}

	// Determine which probes have fine data available. A nil set means "all".
	var available map[int]bool
	if probeList := firstRaw(req, "fineProbes", "FineProbes", "availableProbes", "AvailableProbes", "loadedProbes", "readyProbes", "fineIndices", "indices", "probes", "loaded", "available"); probeList != nil {
		available = parseIntSet(probeList)
		if available == nil {
			available = map[int]bool{}
		}
		mergeProbeRefs(available, doc.Grid, probeList)
	}
	for _, batchRaw := range allRaw(req, "fineBatches", "FineBatches", "batches", "Batches") {
		if available == nil {
			available = map[int]bool{}
		}
		mergeIntBatches(available, batchRaw)
	}

	fineMap := map[int]domain.Lobes{}
	fineRef := firstRaw(req, "fine", "Fine", "fineDigest", "fineArtifact", "fineId")
	var fineDigest string
	if fineRef != nil {
		fineBlob, fd, err := s.resolveArtifactRef(fineRef, "", firstRaw(req, "fineArtifactBase64", "fine_base64"))
		if err != nil {
			problem(w, 404, "fine tier not found: "+err.Error())
			return
		}
		fineDigest = fd
		fdoc, err := domain.LoadFine(fineBlob)
		if err != nil {
			problem(w, 422, err.Error())
			return
		}
		fineMap = fdoc.LobesMap(available)
	}
	// Inline fine values always contribute.
	mergeInlineFine(fineMap, firstRaw(req, "fineData", "FineData", "fineValues", "FineValues", "fineLobes", "FineLobes"))
	// If an availability set was named but no values exist for an index, it
	// simply is not present in fineMap, which TieredQuery treats as missing.

	results := domain.TieredQuery(doc.Grid, doc.Blocks, probes, fineMap, points)
	items := make([]map[string]any, 0, len(results))
	for _, qr := range results {
		items = append(items, queryResultMap(qr))
	}
	resp := map[string]any{
		"results": items, "Results": items,
		"Samples": items, "samples": items,
		"queries": items, "data": items, "Data": items,
		"count": len(items), "Count": len(items),
		"coarseDigest": coarseDigest, "coarse": coarseDigest,
		"byteAccounting": doc.ByteAccounting,
	}
	if fineDigest != "" {
		resp["fineDigest"] = fineDigest
		resp["fine"] = fineDigest
	}
	writeJSON(w, 200, resp)
}

func queryResultMap(qr domain.QueryResult) map[string]any {
	neighbors := make([]map[string]any, 0, len(qr.Neighbors))
	requiredFine := make([]int, 0, len(qr.Neighbors))
	for _, n := range qr.Neighbors {
		neighbors = append(neighbors, map[string]any{
			"index": n.Index, "Index": n.Index,
			"weight": n.Weight, "Weight": n.Weight,
			"fine": n.Fine, "Fine": n.Fine,
		})
		requiredFine = append(requiredFine, n.Index)
	}
	irr := qr.Irradiance
	return map[string]any{
		"position": qr.Position, "Position": qr.Position,
		"state": qr.State, "State": qr.State,
		"weight": qr.Weight, "Weight": qr.Weight,
		"support": qr.Weight, "Support": qr.Weight,
		"Irradiance": irr, "irradiance": irr,
		"tier": qr.Tier, "Tier": qr.Tier,
		"neighbors": neighbors, "Neighbors": neighbors,
		"requiredFineProbes": requiredFine, "RequiredFineProbes": requiredFine,
	}
}

// ---- flexible document helpers -------------------------------------------

func firstRaw(m map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
		if v, ok := lookupRaw(m, normalizeKey(k)); ok {
			return v
		}
	}
	return nil
}

func allRaw(m map[string]json.RawMessage, keys ...string) []json.RawMessage {
	var out []json.RawMessage
	for _, k := range keys {
		if v := firstRaw(m, k); v != nil {
			out = append(out, v)
		}
	}
	return out
}

// heuristicPositions finds a position list under an unanticipated key name.
// It only accepts nested arrays / arrays of objects so scalar index lists are
// not mistaken for coordinates.
func heuristicPositions(m map[string]json.RawMessage) []domain.Vec {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		raw := m[k]
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			continue
		}
		var elems []json.RawMessage
		if json.Unmarshal(raw, &elems) != nil || len(elems) == 0 {
			continue
		}
		first := bytes.TrimSpace(elems[0])
		nested := len(first) > 0 && (first[0] == '[' || first[0] == '{')
		if !nested {
			continue
		}
		ps := parsePositions(raw)
		if len(ps) > 0 {
			return ps
		}
	}
	// Singular position under a position-ish key.
	for _, k := range keys {
		nk := normalizeKey(k)
		if strings.Contains(nk, "posit") || nk == "at" || strings.Contains(nk, "coord") ||
			strings.Contains(nk, "locat") || strings.Contains(nk, "point") {
			if v, ok := parseVec(m[k]); ok {
				return []domain.Vec{v}
			}
		}
	}
	return nil
}

// heuristicRef finds a digest/path raw value under a key containing token.
func heuristicRef(m map[string]json.RawMessage, token string) json.RawMessage {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(normalizeKey(k), token) {
			var s string
			if json.Unmarshal(m[k], &s) == nil && s != "" {
				return m[k]
			}
			var obj map[string]json.RawMessage
			if json.Unmarshal(m[k], &obj) == nil && obj != nil {
				return m[k]
			}
		}
	}
	return nil
}

func parsePositions(raw json.RawMessage) []domain.Vec {
	if raw == nil {
		return nil
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(raw, &raws); err != nil {
		// Maybe a single position rather than an array.
		if v, ok := parseVec(raw); ok {
			return []domain.Vec{v}
		}
		return nil
	}
	out := make([]domain.Vec, 0, len(raws))
	for _, item := range raws {
		if v, ok := parseVec(item); ok {
			out = append(out, v)
		}
	}
	if len(out) == 0 && len(raws) > 0 {
		// A bare [x, y, z] decoded as three scalar RawMessages.
		if v, ok := parseVec(raw); ok {
			return []domain.Vec{v}
		}
	}
	return out
}

func parseVec(raw json.RawMessage) (domain.Vec, bool) {
	var arr [3]float64
	if err := json.Unmarshal(raw, &arr); err == nil {
		return domain.Vec(arr), true
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err == nil {
		x := firstFloat(m, "x", "X", "0")
		y := firstFloat(m, "y", "Y", "1")
		z := firstFloat(m, "z", "Z", "2")
		return domain.Vec{x, y, z}, true
	}
	return domain.Vec{}, false
}

func firstFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return 0
}

// parseIntSet parses [0,2] or [[x,y,z],...] or [{"X":..}] into linear indices
// only when a Grid mapping is available; coordinate forms are deferred and
// currently ignored unless they are plain ints. For coordinate forms callers
// needing conversion should use parseProbeRefs with the grid.
func parseIntSet(raw json.RawMessage) map[int]bool {
	if raw == nil {
		return nil
	}
	var ints []int
	if err := json.Unmarshal(raw, &ints); err == nil {
		m := make(map[int]bool, len(ints))
		for _, i := range ints {
			m[i] = true
		}
		return m
	}
	var mixed []json.RawMessage
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return nil
	}
	m := map[int]bool{}
	for _, e := range mixed {
		var i int
		if json.Unmarshal(e, &i) == nil {
			m[i] = true
			continue
		}
		var ref struct {
			Index *int `json:"Index"`
		}
		if json.Unmarshal(e, &ref) == nil && ref.Index != nil {
			m[*ref.Index] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func mergeIntBatches(set map[int]bool, raw json.RawMessage) {
	if set == nil || raw == nil {
		return
	}
	var batches [][]int
	if err := json.Unmarshal(raw, &batches); err == nil {
		for _, b := range batches {
			for _, i := range b {
				set[i] = true
			}
		}
	}
}

// mergeProbeRefs resolves [x,y,z] / {X,Y,Z} probe references to linear indices.
func mergeProbeRefs(set map[int]bool, g domain.Grid, raw json.RawMessage) {
	if raw == nil || set == nil {
		return
	}
	var mixed []json.RawMessage
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return
	}
	for _, e := range mixed {
		var coords [3]float64
		if err := json.Unmarshal(e, &coords); err == nil {
			ix := [3]int{}
			ok := true
			for a := 0; a < 3; a++ {
				if coords[a] != math.Trunc(coords[a]) {
					ok = false
					break
				}
				ix[a] = int(coords[a])
				if ix[a] < 0 || ix[a] >= g.Size[a] {
					ok = false
					break
				}
			}
			if ok {
				set[g.ProbeIndex(ix[0], ix[1], ix[2])] = true
			}
			continue
		}
		var m map[string]float64
		if err := json.Unmarshal(e, &m); err == nil {
			x := firstFloat(m, "x", "X")
			y := firstFloat(m, "y", "Y")
			z := firstFloat(m, "z", "Z")
			ix := [3]int{int(x), int(y), int(z)}
			if x == float64(ix[0]) && y == float64(ix[1]) && z == float64(ix[2]) &&
				ix[0] >= 0 && ix[0] < g.Size[0] && ix[1] >= 0 && ix[1] < g.Size[1] && ix[2] >= 0 && ix[2] < g.Size[2] {
				set[g.ProbeIndex(ix[0], ix[1], ix[2])] = true
			}
		}
	}
}

func mergeInlineFine(fine map[int]domain.Lobes, raw json.RawMessage) {
	if raw == nil {
		return
	}
	// Array form: [{"Index":3,"Irradiance":[...]}]. A single Pascal tag also
	// matches lowercase/camel spellings via case-insensitive decoding.
	var arr []struct {
		Index      int          `json:"Index"`
		Irradiance domain.Lobes `json:"Irradiance"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, e := range arr {
			fine[e.Index] = e.Irradiance
		}
		return
	}
	// Object form: {"3": [[...]]}
	var obj map[string]domain.Lobes
	if err := json.Unmarshal(raw, &obj); err == nil {
		for k, v := range obj {
			if i, err := parseIntKey(k); err == nil {
				fine[i] = v
			}
		}
	}
}

func parseIntKey(k string) (int, error) {
	n := 0
	for _, r := range k {
		if r < '0' || r > '9' {
			return 0, errBadKey
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errBadKey = badKeyError{}

type badKeyError struct{}

func (badKeyError) Error() string { return "bad index key" }

// resolveArtifactRef turns a digest, path, URL or object into blob bytes.
func (s *Service) resolveArtifactRef(ref json.RawMessage, pathDigest string, b64 json.RawMessage) ([]byte, string, error) {
	if b64 != nil {
		var encoded string
		if err := json.Unmarshal(b64, &encoded); err == nil && encoded != "" {
			blob, err := base64.StdEncoding.DecodeString(encoded)
			return blob, "inline", err
		}
	}
	digest := pathDigest
	if ref != nil {
		var text string
		if err := json.Unmarshal(ref, &text); err == nil {
			digest = digestFromRef(text)
		} else {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(ref, &obj); err == nil {
				for _, k := range []string{"digest", "Digest", "sha256", "artifact", "Artifact", "path", "url", "href"} {
					if v, ok := obj[k]; ok {
						var t string
						if json.Unmarshal(v, &t) == nil {
							digest = digestFromRef(t)
							break
						}
					}
				}
			}
		}
	}
	if digest == "" {
		return nil, "", errNoCoarse
	}
	blob, err := s.vault.Get(digest)
	if err != nil {
		return nil, digest, err
	}
	return blob, digest, nil
}

var errNoCoarse = refError("no coarse artifact reference")

type refError string

func (e refError) Error() string { return string(e) }

func digestFromRef(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/artifacts/"); i >= 0 {
		s = s[i+len("/artifacts/"):]
	} else if strings.HasPrefix(s, "/v1/artifacts/") {
		s = strings.TrimPrefix(s, "/v1/artifacts/")
	}
	if j := strings.IndexAny(s, "?#/"); j >= 0 {
		s = s[:j]
	}
	return s
}
