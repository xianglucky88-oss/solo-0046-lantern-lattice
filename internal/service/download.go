package service

import (
	"bytes"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
	"io/fs"
	"net/http"
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
	name := domain.DownloadName(data)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+digest+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
