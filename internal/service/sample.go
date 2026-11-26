package service

import (
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
	"io/fs"
	"net/http"
	"strconv"
)

type sampleResponse struct {
	Available bool    `json:"Available"`
	R         float64 `json:"R"`
	G         float64 `json:"G"`
	B         float64 `json:"B"`
	Weight    float64 `json:"Weight"`
}

// sampleArtifact proves offline sampling against an archived package. The same
// package imported above can be sampled without rebuilding the probe strip.
func (s *Service) sampleArtifact(w http.ResponseWriter, r *http.Request) {
	face, err := strconv.Atoi(r.URL.Query().Get("face"))
	if err != nil {
		problem(w, 400, "face must be an integer 0–5")
		return
	}
	depth, err := strconv.Atoi(r.URL.Query().Get("depth"))
	if err != nil {
		problem(w, 400, "depth must be an integer layer")
		return
	}
	u, err := strconv.ParseFloat(r.URL.Query().Get("u"), 64)
	if err != nil {
		problem(w, 400, "u must be a fractional coordinate in [0,1]")
		return
	}
	v, err := strconv.ParseFloat(r.URL.Query().Get("v"), 64)
	if err != nil {
		problem(w, 400, "v must be a fractional coordinate in [0,1]")
		return
	}
	data, err := s.vault.Get(r.PathValue("digest"))
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
	frame, err := domain.ImportArchive(data)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	result, err := frame.Sample(face, depth, u, v)
	if err != nil {
		problem(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, sampleResponse(result))
}
