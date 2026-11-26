package service

import (
	"encoding/json"
	"net/http"

	"example.local/lantern-lattice/internal/domain"
)

// bakeResource handles direct JSON resource creation for engine/tooling use.
// The canonical delivery path remains POST /v1/build; its layout.json embeds
// the same exact resource.
func (s *Service) bakeResource(w http.ResponseWriter, r *http.Request) {
	input, status, err := decode(w, r)
	if err != nil {
		problem(w, status, err.Error())
		return
	}
	resource, err := domain.BuildResource(input)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

type instanceRequest struct {
	Resource  json.RawMessage `json:"Resource"`
	Asset     json.RawMessage `json:"Asset"`
	Prefab    json.RawMessage `json:"Prefab"`
	Cabinet   json.RawMessage `json:"Cabinet"`
	Layout    json.RawMessage `json:"Layout"`
	Artifact  string          `json:"Artifact"`
	Digest    string          `json:"Digest"`
	Placement json.RawMessage `json:"Placement"`
	Transform json.RawMessage `json:"Transform"`
	Instance  json.RawMessage `json:"Instance"`
	Pose      json.RawMessage `json:"Pose"`
	Point     domain.Vec      `json:"Point"`
	World     domain.Vec      `json:"World"`
	Position  domain.Vec      `json:"Position"`
	At        domain.Vec      `json:"At"`
}

func (s *Service) sampleInstance(w http.ResponseWriter, r *http.Request) {
	var body instanceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20))
	if err := dec.Decode(&body); err != nil {
		problem(w, http.StatusBadRequest, "invalid instance query")
		return
	}

	var resource *domain.Resource
	var err error
	for _, raw := range []json.RawMessage{body.Resource, body.Asset, body.Prefab, body.Cabinet} {
		if len(raw) == 0 {
			continue
		}
		resource, err = decodePossibleResource(raw)
		if err != nil {
			problem(w, http.StatusBadRequest, "invalid probe resource")
			return
		}
		break
	}
	if resource == nil && len(body.Layout) > 0 {
		if r, err := domain.ResourceFromLayoutJSON(body.Layout); err == nil {
			resource = r
		}
	}
	if resource == nil && (body.Artifact != "" || body.Digest != "") {
		digest := body.Artifact
		if digest == "" {
			digest = body.Digest
		}
		if len(digest) > 7 && digest[:7] == "/v1/artifacts/" {
			digest = digest[len("/v1/artifacts/"):]
		}
		data, getErr := s.vault.Get(digest)
		if getErr != nil {
			problem(w, http.StatusNotFound, "artifact resource not found")
			return
		}
		resource, err = domain.ReadResourceArchive(data)
		if err != nil {
			problem(w, http.StatusConflict, "artifact does not contain a probe resource")
			return
		}
	}
	if resource == nil {
		problem(w, http.StatusBadRequest, "missing downloaded probe resource")
		return
	}

	var placement domain.Placement
	for _, raw := range []json.RawMessage{body.Placement, body.Transform, body.Pose} {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		if err := json.Unmarshal(raw, &placement); err != nil {
			problem(w, http.StatusBadRequest, "invalid placement")
			return
		}
		break
	}
	// Instance may be either a placement or an object wrapping Placement.
	if len(body.Instance) > 0 && string(body.Instance) != "null" {
		p, err := decodePlacementMessage(body.Instance)
		if err != nil {
			problem(w, http.StatusBadRequest, "invalid instance placement")
			return
		}
		placement = p
	}

	point := firstVec(body.Point, body.World, body.Position, body.At)
	sample, err := resource.TrySample(placement, point)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sample)
}

func decodePossibleResource(raw json.RawMessage) (*domain.Resource, error) {
	if r, err := domain.DecodeResource(raw); err == nil && len(r.Probes) > 0 {
		return r, nil
	}
	if r, err := domain.ResourceFromLayoutJSON(raw); err == nil {
		return r, nil
	}
	// Preserve the JSON decode error for ordinary malformed resources.
	return domain.DecodeResource(raw)
}

func decodePlacementMessage(raw json.RawMessage) (domain.Placement, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return domain.Placement{}, err
	}
	for _, key := range []string{"Placement", "Transform", "Pose"} {
		if nested, ok := keys[key]; ok {
			var p domain.Placement
			if err := json.Unmarshal(nested, &p); err != nil {
				return domain.Placement{}, err
			}
			return p, nil
		}
	}
	var p domain.Placement
	err := json.Unmarshal(raw, &p)
	return p, err
}

func firstVec(values ...domain.Vec) domain.Vec {
	for _, v := range values {
		if v != (domain.Vec{}) {
			return v
		}
	}
	return domain.Vec{}
}
