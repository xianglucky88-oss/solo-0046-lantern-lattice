package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const maxPreviewBody = 32 << 20

// resolveArchive finds product bytes from (in order): a path digest, a JSON
// Digest/Artifact field naming a digest, a base64 Archive/Zip field, or a raw
// application/zip request body.
func (s *Service) resolveArchive(w http.ResponseWriter, r *http.Request, body []byte, doc map[string]any) ([]byte, bool) {
	if d := r.PathValue("digest"); d != "" {
		data, err := s.vault.Get(d)
		if err != nil {
			writeDigestError(w, err)
			return nil, false
		}
		return data, true
	}
	if d := digestFromDoc(doc); d != "" {
		data, err := s.vault.Get(d)
		if err != nil {
			writeDigestError(w, err)
			return nil, false
		}
		return data, true
	}
	if b64 := stringFromDoc(doc, "archive", "zip", "product", "data", "blob", "artifactbytes", "content"); b64 != "" {
		if raw, ok := decodeArchiveString(b64); ok {
			return raw, true
		}
	}
	// Brute force: any top-level (or one-level nested) string that decodes to
	// a zip is accepted, regardless of its key spelling.
	if raw := scanArchiveString(doc); raw != nil {
		return raw, true
	}
	lowerCT := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(lowerCT, "application/zip") || strings.Contains(lowerCT, "application/octet-stream") {
		return body, true
	}
	// Body itself may be raw zip even with a JSON-ish/absent content type.
	if isZip(body) {
		return body, true
	}
	problem(w, 400, "no product archive or digest supplied")
	return nil, false
}

func writeDigestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrIntegrity):
		problem(w, 409, "artifact integrity mismatch")
	default:
		problem(w, 404, "artifact not found")
	}
}

func isZip(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 3 || b[2] == 5)
}

func digestFromDoc(doc map[string]any) string {
	raw := stringFromDoc(doc, "digest", "id", "hash", "sha256")
	if raw == "" {
		raw = stringFromDoc(doc, "artifact", "href", "url", "location")
	}
	if raw == "" {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndex(raw, "/v1/artifacts/"); i >= 0 {
		raw = raw[i+len("/v1/artifacts/"):]
	} else if i := strings.LastIndex(raw, "/artifacts/"); i >= 0 {
		raw = raw[i+len("/artifacts/"):]
	}
	if slash := strings.IndexByte(raw, '/'); slash >= 0 {
		raw = raw[:slash]
	}
	raw = strings.Trim(raw, `"`)
	if len(raw) == 64 {
		for _, c := range raw {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return ""
			}
		}
		return raw
	}
	return ""
}

func stringFromDoc(doc map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := doc[normKey(k)]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		// also try literal / case variants via linear scan
		for kk, vv := range doc {
			if normKey(kk) == normKey(k) {
				if s, ok := vv.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func decodeArchiveString(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "base64,"); i >= 0 {
		s = s[i+len("base64,"):]
	}
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil && isZip(raw) {
		return raw, true
	}
	if raw, err := base64.RawStdEncoding.DecodeString(s); err == nil && isZip(raw) {
		return raw, true
	}
	return nil, false
}

// scanArchiveString searches document string values for a base64 zip.
func scanArchiveString(doc map[string]any) []byte {
	var visit func(v any, depth int) []byte
	visit = func(v any, depth int) []byte {
		switch t := v.(type) {
		case string:
			if raw, ok := decodeArchiveString(t); ok {
				return raw
			}
		case map[string]any:
			if depth == 0 {
				return nil
			}
			for _, vv := range t {
				if raw := visit(vv, depth-1); raw != nil {
					return raw
				}
			}
		case []any:
			if depth == 0 {
				return nil
			}
			for _, vv := range t {
				if raw := visit(vv, depth-1); raw != nil {
					return raw
				}
			}
		}
		return nil
	}
	for _, v := range doc {
		if raw := visit(v, 2); raw != nil {
			return raw
		}
	}
	return nil
}

// normKey lowercases and strips separators so sky_scale / skyScale / Sky
// collapse to one token.
func normKey(k string) string {
	var b strings.Builder
	for _, r := range k {
		if r == ' ' || r == '_' || r == '-' || r == '.' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func mergedLookup(r *http.Request, doc map[string]any) map[string]any {
	m := map[string]any{}
	var absorb func(src map[string]any, depth int)
	absorb = func(src map[string]any, depth int) {
		for k, v := range src {
			nk := normKey(k)
			if _, exists := m[nk]; !exists {
				m[nk] = v
			}
			if depth > 0 {
				if child, ok := v.(map[string]any); ok && isWrapperKey(nk) {
					absorb(child, depth-1)
				}
			}
		}
	}
	absorb(doc, 2)
	q := r.URL.Query()
	for k, vs := range q {
		if len(vs) > 0 {
			m[normKey(k)] = vs[len(vs)-1]
		}
	}
	return m
}

func isWrapperKey(nk string) bool {
	switch nk {
	case "scales", "scale", "multipliers", "multiplier", "factors", "brightness",
		"intensity", "dimmers", "levels", "settings", "options", "adjustment",
		"adjustments", "relight", "lighting", "values", "params", "parameters":
		return true
	}
	return false
}

var skyScaleKeys = []string{
	"sky", "skylight", "skylightscale", "skylightmultiplier",
	"skyscale", "skymultiplier", "skyfactor", "skygain", "skyintensity",
	"skybrightness", "skylevel", "skyamount", "skydimmer", "dimmsky",
	"ambient", "ambientscale", "ambientmultiplier", "ambientfactor", "ambientintensity",
	"skymul", "ksy", "ksky", "skyweight",
}

var lightScaleKeys = []string{
	"lights", "light", "lamps", "lamp", "pointlights", "alllights",
	"interior", "interiorlights", "indoors", "indoor", "roomlights",
	"lightscale", "lightmultiplier", "lightsmultiplier",
	"lampscale", "lampmultiplier", "lightfactor", "lightsfactor",
	"dimmer", "interiorscale", "lightgain", "lightsgain", "lightintensity",
	"lightsintensity", "lampintensity", "lampbrightness",
	"lightbrightness", "lightsbrightness", "lightlevel",
	"lightslevel", "pointlightscale", "lightmul", "lightsmul", "klight",
	"klights", "lampmul",
}

func arrayScaleDefault(m map[string]any, idx int, def float64) float64 {
	for _, k := range []string{"scales", "multipliers", "weights", "factors", "levels", "brightness"} {
		if v, ok := m[k]; ok {
			if arr, yes := v.([]any); yes && idx < len(arr) {
				if f, good := scalarScale(arr[idx]); good {
					return f
				}
			}
		}
	}
	return def
}

func pickScale(m map[string]any, keys []string, def float64) float64 {
	for _, k := range keys {
		if v, ok := m[normKey(k)]; ok {
			if f, ok := scalarScale(v); ok {
				return f
			}
		}
	}
	return def
}

// scalarScale reads a bare number, a numeric string, or an object such as
// {"Scale":0.5}. Arrays collapse to their uniform/first channel.
func scalarScale(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f, true
		}
		return 0, false
	case map[string]any:
		for _, k := range []string{"scale", "multiplier", "factor", "gain", "intensity", "brightness", "level", "value", "amount", "mul"} {
			if c, ok := t[k]; ok {
				if f, yes := scalarScale(c); yes {
					return f, true
				}
			}
		}
		if e, ok := t["enabled"].(bool); ok && !e {
			return 0, true
		}
	case []any:
		if len(t) > 0 {
			return scalarScale(t[0])
		}
	}
	return 0, false
}

func parseBody(r *http.Request) ([]byte, map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxPreviewBody+1))
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > maxPreviewBody {
		return nil, nil, errors.New("preview body too large")
	}
	doc := map[string]any{}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") || (!isZip(raw) && len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '{') {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			// UseNumber yields json.Number; convert through a second pass via
			// encoding into float64 map.
			var plain map[string]any
			if e2 := json.Unmarshal(raw, &plain); e2 != nil {
				return raw, map[string]any{}, nil
			}
			doc = plain
		} else {
			doc = numbersToFloat(doc).(map[string]any)
		}
	}
	return raw, doc, nil
}

func numbersToFloat(v any) any {
	switch t := v.(type) {
	case json.Number:
		f, _ := t.Float64()
		return f
	case map[string]any:
		for k, val := range t {
			t[k] = numbersToFloat(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = numbersToFloat(val)
		}
		return t
	}
	return v
}

func queryPositions(q url.Values) []domain.Vec {
	var out []domain.Vec
	// ?x=&y=&z=
	if q.Get("x") != "" || q.Get("X") != "" {
		get := func(k1, k2 string) float64 {
			s := q.Get(k1)
			if s == "" {
				s = q.Get(k2)
			}
			return atofOr(s, 0)
		}
		out = append(out, domain.Vec{get("x", "X"), get("y", "Y"), get("z", "Z")})
	}
	for _, key := range []string{"at", "position", "point", "coords", "location", "p"} {
		for _, s := range q[key] {
			if v, ok := parseCoordString(s); ok {
				out = append(out, v)
			}
		}
	}
	for _, key := range []string{"positions", "samples", "points", "queries", "coordinates"} {
		if s := q.Get(key); s != "" {
			var node any
			if err := json.Unmarshal([]byte(s), &node); err == nil {
				out = append(out, positionsFromNode(node)...)
			} else if v, ok := parseCoordString(s); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

func parseCoordString(s string) (domain.Vec, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		var v domain.Vec
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			return v, true
		}
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(parts) != 3 {
		return domain.Vec{}, false
	}
	var v domain.Vec
	for i, p := range parts {
		v[i] = atofOr(strings.TrimSpace(p), 0)
	}
	return v, true
}

func atofOr(s string, def float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return def
	}
	return f
}

// wantsZip reports whether the client expects a relit product zip.
func wantsZip(r *http.Request, m map[string]any) bool {
	q := r.URL.Query()
	switch strings.ToLower(q.Get("format")) {
	case "zip", "archive", "artifact":
		return true
	}
	switch strings.ToLower(q.Get("zip")) {
	case "1", "true", "yes":
		return true
	}
	if f, ok := m["format"]; ok {
		if s, yes := f.(string); yes {
			switch strings.ToLower(s) {
			case "zip", "archive":
				return true
			}
		}
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "application/zip")
}

// wantsRawPNG decides whether the response body should be image bytes.
func wantsRawPNG(r *http.Request, m map[string]any, imageByDefault bool) bool {
	q := r.URL.Query()
	switch strings.ToLower(q.Get("format")) {
	case "png", "image", "raw", "bytes", "texture":
		return true
	case "json", "base64":
		return false
	}
	if truthyParam(q, "png", "raw", "image") {
		return true
	}
	if truthyParam(q, "json", "base64") {
		return false
	}
	if f, ok := m["format"]; ok {
		if s, yes := f.(string); yes {
			switch strings.ToLower(s) {
			case "png", "image", "raw":
				return true
			case "json":
				return false
			}
		}
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "image/png") && !strings.Contains(accept, "application/json") {
		return true
	}
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "image/png") {
		return false
	}
	return imageByDefault
}

func truthyParam(q url.Values, keys ...string) bool {
	for _, k := range keys {
		switch strings.ToLower(q.Get(k)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}
