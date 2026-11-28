package service

import (
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"example.local/lantern-lattice/internal/vault"
	"io"
	"io/fs"
	"net/http"
)

// normalRequest asks for one receiver-normal readout. The resource may be
// supplied inline as the downloaded layout.json (Layout/Resource/Artifact), or
// referenced by an archived digest. Position and Normal are the only geometry
// the engine needs once the resource is available.
type normalRequest struct {
	Layout   json.RawMessage `json:"Layout"`
	Resource json.RawMessage `json:"Resource"`
	Artifact string          `json:"Artifact"`
	Digest   string          `json:"Digest"`
	Position domain.Vec      `json:"Position"`
	Normal   domain.Vec      `json:"Normal"`
}

func (s *Service) sampleNormal(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		problem(w, 413, "request exceeds readout limit")
		return
	}
	var req normalRequest
	if err := json.Unmarshal(body, &req); err != nil {
		problem(w, 400, "invalid normal readout request")
		return
	}
	resource, err := s.normalResource(req)
	if err != nil {
		status := 400
		if errors.Is(err, errNoNormalResource) {
			status = 409
		}
		problem(w, status, err.Error())
		return
	}
	result := resource.Sample(req.Position, req.Normal)
	writeJSON(w, 200, result)
}

var errNoNormalResource = errors.New("artifact does not contain a receiver-normal resource")

func (s *Service) normalResource(req normalRequest) (*domain.NormalResource, error) {
	for _, raw := range []json.RawMessage{req.Layout, req.Resource} {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		if res, err := domain.NormalResourceFromLayoutJSON(raw); err == nil {
			return res, nil
		}
		if res, err := domain.DecodeNormalResource(raw); err == nil {
			return res, nil
		}
	}
	digest := req.Digest
	if digest == "" {
		digest = req.Artifact
	}
	if digest == "" {
		return nil, errors.New("missing downloaded receiver-normal resource")
	}
	if len(digest) > len("/v1/artifacts/") && digest[:len("/v1/artifacts/")] == "/v1/artifacts/" {
		digest = digest[len("/v1/artifacts/"):]
	}
	data, err := s.vault.Get(digest)
	switch {
	case errors.Is(err, vault.ErrDigest), errors.Is(err, fs.ErrNotExist):
		return nil, errors.New("artifact not found")
	case errors.Is(err, vault.ErrIntegrity):
		return nil, errors.New("artifact integrity mismatch")
	case err != nil:
		return nil, errors.New("cannot read artifact")
	}
	res, err := domain.ReadNormalResource(data)
	if err != nil {
		return nil, errNoNormalResource
	}
	return res, nil
}
