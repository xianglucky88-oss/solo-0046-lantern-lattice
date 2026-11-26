package service

import "net/http"

// Receipt is the response body of successful build and continuation calls.
type Receipt struct {
	Digest   string     `json:"digest"`
	Bytes    int        `json:"bytes"`
	Artifact string     `json:"artifact"`
	Version  string     `json:"version,omitempty"`
	Lights   []LightRef `json:"lights,omitempty"`
	Resolved [][3]int   `json:"resolved,omitempty"`
	Occluded [][3]int   `json:"occlusion_only,omitempty"`
	Patch    string     `json:"patch,omitempty"`
	Base     string     `json:"base,omitempty"`
}

// LightRef exposes a light's stable identity and its list position.
type LightRef struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
}

func (s *Service) build(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		problem(w, 503, "compiler busy")
		return
	}
	input, status, err := decode(w, r)
	if err != nil {
		problem(w, status, err.Error())
		return
	}
	zipBytes, err := compile(input)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	if r.Context().Err() != nil {
		return
	}
	digest, created, err := s.vault.Put(zipBytes)
	if err != nil {
		problem(w, 500, "cannot save artifact")
		return
	}
	location := "/v1/artifacts/" + digest
	w.Header().Set("Location", location)
	status = 200
	if created {
		status = 201
	}
	receipt := Receipt{Digest: digest, Bytes: len(zipBytes), Artifact: location}
	if input.Resumable {
		row, perr := s.persistRoot(input, zipBytes, digest)
		if perr != nil {
			problem(w, 500, "cannot save version")
			return
		}
		receipt.Version = row.Version
		receipt.Lights = row.Lights
		receipt.Patch = ""
	}
	writeJSON(w, status, receipt)
}
