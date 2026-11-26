package service

import (
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io"
	"mime"
	"net/http"
)

const maxImportBytes = 16 << 20

// importArchive accepts a downloaded package (legacy single-strip or tiled
// multi-page) through one entry and rejects every layout defect up front,
// before any pixel is queried.
func (s *Service) importArchive(w http.ResponseWriter, r *http.Request) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/zip" {
		problem(w, 415, "expected application/zip")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		problem(w, 413, "package exceeds 16 MiB")
		return
	}
	if err != nil {
		problem(w, 400, "cannot read request body")
		return
	}
	frame, err := domain.ImportArchive(body)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, frame.Report())
}
