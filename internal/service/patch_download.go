package service

import (
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io/fs"
	"net/http"
)

func (s *Service) patch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.vault.ReadNamed(patchFile(id))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		problem(w, 404, "patch not found")
		return
	case err != nil:
		problem(w, 500, "cannot read patch")
		return
	}
	// Re-validate the stored supplement before handing it out.
	if _, err := domain.DecodePatch(data); err != nil {
		problem(w, 409, "patch integrity mismatch")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+id+`"`)
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
