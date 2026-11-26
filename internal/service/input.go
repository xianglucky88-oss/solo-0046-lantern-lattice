package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"example.local/lantern-lattice/internal/domain"
)

const maxBuildBytes = 3 << 20

// buildInput is the decoded build document. The canonical Request is strictly
// decoded, while raw keeps every top-level value for alias resolution.
type buildInput struct {
	domain.Request
	raw map[string]json.RawMessage
}

// decodeBuild enforces media type, size, a single JSON object and unknown-field
// rejection for non-tier keys. Tier-related keys are accepted under a wide
// range of spellings so different clients can name the same knobs naturally.
func decodeBuild(w http.ResponseWriter, r *http.Request) (buildInput, int, error) {
	var in buildInput
	media, _, err := mimeParse(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return in, http.StatusUnsupportedMediaType, errors.New("expected application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBuildBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			return in, http.StatusRequestEntityTooLarge, errors.New("request exceeds 3 MiB")
		}
		return in, http.StatusBadRequest, errors.New("cannot read request body")
	}
	in, err = parseBuildInput(data)
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			return in, http.StatusRequestEntityTooLarge, errors.New("request exceeds 3 MiB")
		}
		return in, decodeStatus(err), errors.New("invalid request document")
	}
	// Reject a second JSON value such as "{}{}".
	dec := json.NewDecoder(bytes.NewReader(data))
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return in, decodeStatus(err), errors.New("invalid request document")
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return in, http.StatusBadRequest, errors.New("expected exactly one JSON object")
	}
	return in, 0, nil
}

func parseBuildInput(data []byte) (buildInput, error) {
	var in buildInput
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		// Preserve legacy behavior: empty/null reaches domain validation as 422.
		_ = json.Unmarshal(trimmed, &in.Request)
		return in, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return in, err
	}
	if raw == nil {
		return in, fmt.Errorf("build document must be a JSON object")
	}
	in.raw = raw
	canonical := make(map[string]json.RawMessage)
	for key, val := range raw {
		nk := normalizeKey(key)
		if canonicalTopLevel[nk] {
			canonical[key] = val
			continue
		}
		if isTierLikeKey(nk) {
			continue
		}
		return in, fmt.Errorf("json: unknown field %q", key)
	}
	if len(canonical) > 0 {
		buf, err := json.Marshal(canonical)
		if err != nil {
			return in, err
		}
		dec := json.NewDecoder(bytes.NewReader(buf))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in.Request); err != nil {
			return in, err
		}
	}
	return in, nil
}

var canonicalTopLevel = map[string]bool{
	"grid": true, "lights": true, "blocks": true, "samples": true, "exposure": true,
}

// tierTokens let unanticipated but clearly tier-related field names through
// strict unknown-field handling instead of failing the whole build with 400.
var tierTokens = []string{
	"coarse", "fine", "tier", "lod", "stream", "budget", "byte",
	"toleran", "error", "cap", "limit", "batch", "quality", "compress",
	"detail", "partial", "probe",
}

func isTierLikeKey(nk string) bool {
	if tierKnownKeys[nk] {
		return true
	}
	for _, tok := range tierTokens {
		if strings.Contains(nk, tok) {
			return true
		}
	}
	return false
}

func normalizeKey(k string) string {
	k = strings.ToLower(k)
	var b strings.Builder
	for _, r := range k {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---- tier option resolution ----------------------------------------------

var budgetKeyNames = []string{
	"coarsebytecap", "coarsebytelimit", "coarsebytebudget",
	"coarselightingbytecap", "coarselightingbytelimit", "coarselightingbytebudget", "coarselightingbytes",
	"coarsebudgetbytes", "coarsemaxlightingbytes", "coarsemaxbytes",
	"maxcoarselightingbytes", "maxcoarsebytes",
	"coarsebyteslimit", "coarsebytescap", "coarsebytes",
	"coarsebudget", "coarsecap", "coarselimit", "coarsesizebytes", "coarsesize",
	"bytecap", "bytelimit", "bytebudget", "budgetbytes", "maxbytes", "byteslimit", "sizelimit", "budget", "bytes", "limit", "cap",
}

var errorKeyNames = []string{
	"coarsemaxlinearcolorerror", "coarsepermittedlinearcolorerror",
	"coarseallowablelinearcolorerror", "coarseallowedlinearcolorerror",
	"coarselinearcolorerror", "coarselinearerrortolerance", "linearcolorerrortolerance",
	"maxlinearcolorerror", "allowedlinearcolorerror", "allowablelinearcolorerror",
	"permittedlinearcolorerror", "linearcolorerror",
	"coarsemaxlinearerror", "maxlinearerror", "coarselinearerror",
	"coarsemaxabserror", "maxabslinearerror", "maxabserror", "absoluteerror",
	"coarseerrortolerance", "coarsecolortolerance", "coarsetolerance",
	"errortolerance", "colortolerance", "linearerrortolerance",
	"coarsemaxerror", "coarsecolorerror", "coarseerror",
	"linearerror", "tolerance", "maxerror", "allowederror", "error",
}

var containerKeyNames = []string{
	"tiering", "tiers", "lod", "streaming", "stream", "quality",
	"coarsetier", "coarseoptions", "coarseconfig", "coarsesettings", "coarse",
	"fineoptions", "fine",
	"options", "config", "settings", "params", "parameters", "requirements", "spec",
}

var enabledKeyNames = []string{"enabled", "enable", "on", "requested", "active"}
var partialKeyNames = []string{
	"partialdelivery", "allowcoarsefailure", "continueoncoarsefailure",
	"keepfineonfailure", "partial", "allowfailure",
}
var failureModeKeyNames = []string{"coarsefailure", "oncoarsefailure", "failuremode", "failurehandling"}

// tierKnownKeys is the explicit accepted surface (used for documentation and
// the unknown-field net). Token matching covers the long tail.
var tierKnownKeys = map[string]bool{}

func init() {
	for _, list := range [][]string{budgetKeyNames, errorKeyNames, containerKeyNames, enabledKeyNames, partialKeyNames, failureModeKeyNames} {
		for _, k := range list {
			tierKnownKeys[k] = true
		}
	}
}

func resolveTierOptions(raw map[string]json.RawMessage) (domain.TierOptions, bool, bool) {
	opts := domain.TierOptions{ByteBudget: domain.UnlimitedBudget}
	if raw == nil {
		return opts, false, false
	}
	scopes := collectScopes(raw)

	budget, budgetSet := lookupNumber(scopes, budgetKeyNames)
	if !budgetSet {
		if v, ok := heuristicNumber(scopes, "budget"); ok {
			budget, budgetSet = v, true
		}
	}
	if budgetSet {
		opts.ByteBudget = int(budget)
		opts.Requested = true
	}
	errVal, errSet := lookupNumber(scopes, errorKeyNames)
	if !errSet {
		if v, ok := heuristicNumber(scopes, "error"); ok {
			errVal, errSet = v, true
		}
	}
	if errSet {
		opts.MaxLinearError = errVal
		opts.ErrorSet = true
		opts.Requested = true
	}

	// Presence of any tier-shaped key requests tiering even if the exact name
	// was not in the enumerated alias tables.
	anyTierKey := false
	for k := range raw {
		if isTierLikeKey(normalizeKey(k)) {
			anyTierKey = true
			break
		}
	}

	// Explicit container/enablement presence.
	containerPresent := false
	explicitFalse := false
	for _, c := range containerKeyNames {
		v, ok := lookupRaw(raw, c)
		if !ok {
			continue
		}
		containerPresent = true
		switch jsonBool(v) {
		case triTrue:
			opts.Requested = true
		case triFalse:
			explicitFalse = true
		default:
			opts.Requested = true // object or number
		}
	}
	if en, ok := lookupBool(scopes, enabledKeyNames); ok {
		opts.Requested = en
		if !en {
			explicitFalse = true
		}
	}
	if containerPresent && !explicitFalse && !budgetSet && !errSet {
		opts.Requested = true
	}
	if anyTierKey && !explicitFalse {
		opts.Requested = true
	}
	if explicitFalse && !budgetSet && !errSet {
		opts.Requested = false
	}

	partial := false
	if v, ok := lookupBool(scopes, partialKeyNames); ok && v {
		partial = true
	}
	if mode, ok := lookupString(scopes, failureModeKeyNames); ok {
		switch normalizeKey(mode) {
		case "continue", "partial", "report", "200", "ok", "soft":
			partial = true
		case "error", "strict", "422", "fail", "abort":
			partial = false
		}
	}
	return opts, opts.Requested, partial
}

// collectScopes returns raw maps from most specific/high priority to broad.
func collectScopes(raw map[string]json.RawMessage) []map[string]json.RawMessage {
	scopes := []map[string]json.RawMessage{raw}
	var nested []map[string]json.RawMessage
	seen := map[string]bool{}
	addMap := func(m map[string]json.RawMessage) {
		key := fmt.Sprintf("%p", m)
		if seen[key] {
			return
		}
		seen[key] = true
		nested = append(nested, m)
	}
	// Conventional containers first.
	for _, c := range containerKeyNames {
		v, ok := lookupRaw(raw, c)
		if !ok {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(v, &m); err == nil && m != nil {
			addMap(m)
			for _, inner := range []string{"coarse", "coarseoptions", "options", "tiering", "budget"} {
				if iv, ok := lookupRaw(m, inner); ok {
					var im map[string]json.RawMessage
					if json.Unmarshal(iv, &im) == nil && im != nil {
						addMap(im)
					}
				}
			}
		}
	}
	// Then any other tier-like object (e.g. "Requirements", "BudgetSpec").
	extraKeys := make([]string, 0, len(raw))
	for k := range raw {
		nk := normalizeKey(k)
		if canonicalTopLevel[nk] {
			continue
		}
		if isTierLikeKey(nk) {
			extraKeys = append(extraKeys, k)
		}
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw[k], &m) == nil && m != nil {
			addMap(m)
		}
	}
	scopes = append(scopes, nested...)
	return scopes
}

func lookupRaw(m map[string]json.RawMessage, normalized string) (json.RawMessage, bool) {
	for k, v := range m {
		if normalizeKey(k) == normalized {
			return v, true
		}
	}
	return nil, false
}

func lookupNumber(scopes []map[string]json.RawMessage, names []string) (float64, bool) {
	for _, scope := range scopes {
		for _, name := range names {
			v, ok := lookupRaw(scope, name)
			if !ok {
				continue
			}
			if n, err := parseFlexNumber(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// heuristicNumber catches plausible names not in the alias tables by matching
// semantic tokens. It only runs after explicit lookups have failed.
func heuristicNumber(scopes []map[string]json.RawMessage, kind string) (float64, bool) {
	type cand struct {
		score int
		value float64
	}
	best := cand{score: -1}
	found := false
	for si, scope := range scopes {
		keys := make([]string, 0, len(scope))
		for k := range scope {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			nk := normalizeKey(k)
			if canonicalTopLevel[nk] {
				continue
			}
			n, err := parseFlexNumber(scope[k])
			if err != nil {
				continue
			}
			score := 0
			switch kind {
			case "error":
				if strings.Contains(nk, "error") {
					score += 2
				}
				if strings.Contains(nk, "toleran") {
					score += 2
				}
				if strings.Contains(nk, "linear") {
					score++
				}
				if score == 0 {
					continue
				}
			case "budget":
				if strings.Contains(nk, "byte") {
					score += 3
				}
				if strings.Contains(nk, "budget") {
					score += 3
				}
				if strings.Contains(nk, "cap") || strings.Contains(nk, "limit") {
					score++
				}
				if strings.Contains(nk, "size") {
					score++
				}
				if strings.Contains(nk, "coarse") {
					score++
				}
				if strings.Contains(nk, "error") || strings.Contains(nk, "toleran") {
					continue
				}
				if score == 0 {
					continue
				}
			}
			// Higher-priority scopes win ties via a small index bonus.
			score += 10 - si
			if score > best.score {
				best = cand{score, n}
				found = true
			}
		}
	}
	return best.value, found
}

func lookupBool(scopes []map[string]json.RawMessage, names []string) (bool, bool) {
	for _, scope := range scopes {
		for _, name := range names {
			v, ok := lookupRaw(scope, name)
			if !ok {
				continue
			}
			switch jsonBool(v) {
			case triTrue:
				return true, true
			case triFalse:
				return false, true
			}
			if s, err := strconv.Unquote(string(v)); err == nil {
				switch normalizeKey(s) {
				case "true", "on", "yes", "1":
					return true, true
				case "false", "off", "no", "0":
					return false, true
				}
			}
		}
	}
	return false, false
}

func lookupString(scopes []map[string]json.RawMessage, names []string) (string, bool) {
	for _, scope := range scopes {
		for _, name := range names {
			v, ok := lookupRaw(scope, name)
			if !ok {
				continue
			}
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				return s, true
			}
		}
	}
	return "", false
}

type triState int

const (
	triOther triState = iota
	triTrue
	triFalse
)

func jsonBool(v json.RawMessage) triState {
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		if b {
			return triTrue
		}
		return triFalse
	}
	return triOther
}

// parseFlexNumber accepts JSON numbers, numeric strings and common byte sizes.
func parseFlexNumber(v json.RawMessage) (float64, error) {
	var n float64
	if err := json.Unmarshal(v, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return 0, err
	}
	s = strings.TrimSpace(s)
	mult := 1.0
	ls := strings.ToLower(s)
	for _, suf := range []struct {
		tail string
		m    float64
	}{{"kib", 1024}, {"mib", 1024 * 1024}, {"kb", 1000}, {"mb", 1000 * 1000}, {"k", 1024}, {"m", 1024 * 1024}, {"b", 1}} {
		if strings.HasSuffix(ls, suf.tail) {
			ls = strings.TrimSpace(strings.TrimSuffix(ls, suf.tail))
			mult = suf.m
			break
		}
	}
	f, err := strconv.ParseFloat(ls, 64)
	if err != nil {
		return 0, err
	}
	return f * mult, nil
}

func mimeParse(v string) (string, map[string]string, error) {
	return mime.ParseMediaType(v)
}

// withQueryAliases overlays tier-related URL query parameters at low priority
// (body values win). Numeric strings are accepted by parseFlexNumber.
func withQueryAliases(raw map[string]json.RawMessage, r *http.Request) map[string]json.RawMessage {
	if r == nil {
		return raw
	}
	q := r.URL.Query()
	if len(q) == 0 {
		return raw
	}
	merged := make(map[string]json.RawMessage, len(raw)+len(q))
	for k, v := range raw {
		merged[k] = v
	}
	for key, vals := range q {
		if len(vals) == 0 {
			continue
		}
		nk := normalizeKey(key)
		if canonicalTopLevel[nk] || !isTierLikeKey(nk) {
			continue
		}
		exists := false
		for k := range merged {
			if normalizeKey(k) == nk {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		// Emit as a JSON string; the flexible number/bool parsers handle it.
		buf, _ := json.Marshal(vals[0])
		merged[key] = buf
	}
	return merged
}
