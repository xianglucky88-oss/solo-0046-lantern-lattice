package service

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
)

// fineBatch serves exact linear probe lighting for a requested set of probes.
// POST body: {"digest":"...","probes":[0,2]} (aliases accepted);
// GET: ?probes=0,2 or ?all=1. The digest normally comes from the path and may
// also be supplied in the body for path-less batch routes.
func (s *Service) fineBatch(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	var body map[string]json.RawMessage
	if r.Method == http.MethodPost && r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxQueryBytes)).Decode(&body)
	}
	if digest == "" && body != nil {
		if ref := firstRaw(body, "digest", "Digest", "fine", "fineDigest", "artifact"); ref != nil {
			var text string
			if json.Unmarshal(ref, &text) == nil {
				digest = digestFromRef(text)
			}
		}
	}
	blob, err := s.vault.Get(digest)
	if err != nil {
		downloadProblem(w, err)
		return
	}
	doc, err := domain.LoadFine(blob)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}

	selected := map[int]bool{}
	all := false
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if q.Get("all") == "1" || q.Get("all") == "true" {
			all = true
		}
		selectorGiven := false
		for _, key := range []string{"probes", "indices", "probe", "index"} {
			if v := q.Get(key); v != "" {
				selectorGiven = true
				for _, part := range strings.Split(v, ",") {
					if n, perr := strconv.Atoi(strings.TrimSpace(part)); perr == nil {
						selected[n] = true
					}
				}
			}
		}
		if !selectorGiven && q.Get("all") != "0" && q.Get("all") != "false" {
			all = true
		}
	} else if body != nil {
		selector := firstRaw(body, "probes", "Probes", "indices", "Indices", "fineProbes", "batch", "Batch")
		if selector != nil {
			if parsed := parseIntSet(selector); parsed != nil {
				selected = parsed
			}
			mergeProbeRefs(selected, doc.Grid, selector)
		}
		for _, br := range allRaw(body, "batches", "Batches", "fineBatches") {
			var batches [][]int
			if json.Unmarshal(br, &batches) == nil {
				for _, b := range batches {
					for _, i := range b {
						selected[i] = true
					}
				}
			}
		}
		if allRaw(body, "all", "All") != nil {
			all = true
		} else if selector == nil && len(allRaw(body, "batches", "Batches", "fineBatches")) == 0 {
			// No selector: return the whole fine volume.
			all = true
		}
	}

	items := []map[string]any{}
	for _, p := range doc.Probes {
		if !all && (selected == nil || !selected[p.Index]) {
			continue
		}
		items = append(items, fineProbeMap(p))
	}
	writeJSON(w, 200, map[string]any{
		"digest": digest, "Digest": digest,
		"probes": items, "Probes": items,
		"count": len(items), "Count": len(items),
		"schema": domain.FineSchema,
	})
}

// fineProbe serves one probe's exact linear lighting.
func (s *Service) fineProbe(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	indexText := r.PathValue("index")
	idx, err := strconv.Atoi(indexText)
	if err != nil {
		problem(w, 400, "invalid probe index")
		return
	}
	blob, err := s.vault.Get(digest)
	if err != nil {
		downloadProblem(w, err)
		return
	}
	doc, err := domain.LoadFine(blob)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	if idx < 0 || idx >= len(doc.Probes) {
		problem(w, 404, "probe index out of range")
		return
	}
	writeJSON(w, 200, fineProbeMap(doc.Probes[idx]))
}

func fineProbeMap(p domain.FineProbe) map[string]any {
	return map[string]any{
		"index": p.Index, "Index": p.Index,
		"valid": p.Valid, "Valid": p.Valid,
		"position": p.Position, "Position": p.Position,
		"irradiance": p.Irradiance, "Irradiance": p.Irradiance,
	}
}

// downloadProblem maps vault retrieval errors to HTTP status codes.
func downloadProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrDigest), errors.Is(err, fs.ErrNotExist):
		problem(w, http.StatusNotFound, "artifact not found")
	case errors.Is(err, vault.ErrIntegrity):
		problem(w, http.StatusConflict, "artifact integrity mismatch")
	default:
		problem(w, http.StatusNotFound, "artifact not found")
	}
}
