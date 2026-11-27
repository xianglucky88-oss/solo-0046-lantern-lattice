package service

import (
	"bytes"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
	"io/fs"
	"net/http"
	"net/url"
	"time"
)

func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	digest := r.PathValue("digest")
	data, err := s.vault.Get(digest)
	switch {
	case errors.Is(err, vault.ErrDigest), errors.Is(err, fs.ErrNotExist):
		problem(w, 404, "artifact not found")
		return
	case errors.Is(err, vault.ErrIntegrity):
		problem(w, 409, "artifact integrity mismatch")
		return
	case err != nil:
		problem(w, 500, "cannot read artifact")
		return
	}

	// A preview-aware query (multipliers or format) renders the relit
	// texture instead of the immutable stock zip. Plain downloads, ETags and
	// range requests carry no such query and keep the original behaviour.
	if previewQueryActive(r.URL.Query()) {
		s.handlePreview(true).ServeHTTP(w, r)
		return
	}

	w.Header().Set("Content-Disposition", `attachment; filename="`+domain.Filename+`"`)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+digest+`"`)
	http.ServeContent(w, r, domain.Filename, time.Time{}, bytes.NewReader(data))
}

// previewQueryActive reports whether the artifact URL asks for relighting.
func previewQueryActive(q url.Values) bool {
	scales := map[string]bool{}
	for _, k := range skyScaleKeys {
		scales[normKey(k)] = true
	}
	for _, k := range lightScaleKeys {
		scales[normKey(k)] = true
	}
	for k := range q {
		nk := normKey(k)
		if nk == "format" || nk == "png" || nk == "raw" || scales[nk] {
			return true
		}
	}
	return false
}
