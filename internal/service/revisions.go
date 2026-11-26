package service

import (
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io"
	"mime"
	"net/http"
)

// wireRevision is the JSON document accepted by POST /v1/revisions.
type wireRevision struct {
	Base       string        `json:"base"`
	Operations []wireLightOp `json:"operations"`
	Budget     int           `json:"budget"`
}

type wireLightOp struct {
	Action string        `json:"action"`
	ID     string        `json:"id"`
	Light  *domain.Light `json:"light,omitempty"`
}

func decodeRevision(w http.ResponseWriter, r *http.Request) (domain.RevisionRequest, int, error) {
	var wire wireRevision
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return domain.RevisionRequest{}, 415, errors.New("expected application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return domain.RevisionRequest{}, decodeStatus(err), errors.New("invalid request document")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return domain.RevisionRequest{}, 400, errors.New("expected exactly one JSON object")
	}
	if wire.Base == "" {
		return domain.RevisionRequest{}, 400, errors.New("base version is required")
	}
	ops := make([]domain.LightOperation, 0, len(wire.Operations))
	for _, op := range wire.Operations {
		ops = append(ops, domain.LightOperation{Action: op.Action, ID: op.ID, Light: op.Light})
	}
	return domain.RevisionRequest{Base: wire.Base, Operations: ops, Budget: wire.Budget}, 0, nil
}

func (s *Service) revisions(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		problem(w, 503, "compiler busy")
		return
	}
	req, status, err := decodeRevision(w, r)
	if err != nil {
		problem(w, status, err.Error())
		return
	}
	rec, err := s.loadVersion(req.Base)
	if errors.Is(err, errMissing) {
		problem(w, 404, "base version not found")
		return
	}
	if err != nil {
		problem(w, 409, "base version record damaged")
		return
	}
	updated, sets, err := domain.Apply(rec.Rev, req)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	id, err := domain.VersionID(updated)
	if err != nil {
		problem(w, 500, "cannot derive version")
		return
	}
	// Idempotent replay: the same continuation already produced this version.
	if existing, lerr := s.loadVersion(id); lerr == nil {
		writeJSON(w, 200, s.existingReceipt(existing))
		return
	} else if !errors.Is(lerr, errMissing) {
		problem(w, 409, "version record damaged")
		return
	}
	zipBytes, err := domain.Assemble(updated.Request())
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	digest, _, err := s.vault.Put(zipBytes)
	if err != nil {
		problem(w, 500, "cannot save artifact")
		return
	}
	newRec := versionRecord{Version: id, Digest: digest, Base: rec.Version, Rev: updated}
	if err := s.saveVersion(newRec); err != nil {
		problem(w, 500, "cannot save version")
		return
	}
	patch, err := domain.BuildPatch(rec.Rev, updated, rec.Digest, rec.Version, id, digest, sets)
	if err != nil {
		problem(w, 500, "cannot build patch")
		return
	}
	patchBytes, err := domain.EncodePatch(patch)
	if err != nil {
		problem(w, 500, "cannot encode patch")
		return
	}
	if _, err := s.vault.PutNamed(patchFile(id), patchBytes); err != nil {
		problem(w, 500, "cannot save patch")
		return
	}
	row := Receipt{
		Digest:   digest,
		Bytes:    len(zipBytes),
		Artifact: "/v1/artifacts/" + digest,
		Version:  id,
		Base:     rec.Version,
		Lights:   lightRefs(updated),
		Resolved: patch.Resolved,
		Occluded: patch.OcclusionOnly,
		Patch:    "/v1/patches/" + id,
	}
	// A distinct light state may still bake to identical ZIP bytes; the
	// version itself is always new, so a first success returns 201.
	w.Header().Set("Location", "/v1/artifacts/"+digest)
	writeJSON(w, 201, row)
}

func (s *Service) existingReceipt(rec versionRecord) Receipt {
	row := Receipt{
		Digest:   rec.Digest,
		Artifact: "/v1/artifacts/" + rec.Digest,
		Version:  rec.Version,
		Base:     rec.Base,
		Lights:   lightRefs(rec.Rev),
	}
	if data, err := s.vault.Get(rec.Digest); err == nil {
		row.Bytes = len(data)
	}
	if data, err := s.vault.ReadNamed(patchFile(rec.Version)); err == nil {
		if p, perr := domain.DecodePatch(data); perr == nil {
			row.Patch = "/v1/patches/" + rec.Version
			row.Resolved = p.Resolved
			row.Occluded = p.OcclusionOnly
		}
	}
	return row
}
