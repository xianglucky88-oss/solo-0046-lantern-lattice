package service

import (
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io/fs"
)

// versionRecord is the durable record of one light version. It stores enough
// of the scene state to act as the base of another continuation after restart.
type versionRecord struct {
	Version string          `json:"version"`
	Digest  string          `json:"digest"`
	Base    string          `json:"base,omitempty"`
	Rev     domain.Revision `json:"revision"`
}

func versionFile(id string) string { return "version-" + id + ".json" }
func patchFile(id string) string   { return "patch-" + id + ".json" }

func (s *Service) loadVersion(id string) (versionRecord, error) {
	var rec versionRecord
	data, err := s.vault.ReadNamed(versionFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return rec, errMissing
	}
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, errCorrupt
	}
	if rec.Version != id || rec.Digest == "" {
		return rec, errCorrupt
	}
	return rec, nil
}

func (s *Service) saveVersion(rec versionRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.vault.PutNamed(versionFile(rec.Version), data)
	return err
}

func lightRefs(rev domain.Revision) []LightRef {
	out := make([]LightRef, 0, len(rev.Lights))
	for i, l := range rev.Lights {
		out = append(out, LightRef{ID: l.ID, Position: i})
	}
	return out
}

// persistRoot records the initial resumable baking result as the root version.
// Re-posting the same resumable request deterministically reaches the same
// version identity and never duplicates state.
func (s *Service) persistRoot(req domain.Request, zipBytes []byte, digest string) (Receipt, error) {
	rev := domain.RootRevision(req)
	id, err := domain.VersionID(rev)
	if err != nil {
		return Receipt{}, err
	}
	rec := versionRecord{Version: id, Digest: digest, Rev: rev}
	if err := s.saveVersion(rec); err != nil {
		return Receipt{}, err
	}
	return Receipt{Digest: digest, Bytes: len(zipBytes), Artifact: "/v1/artifacts/" + digest, Version: id, Lights: lightRefs(rev)}, nil
}

var (
	errMissing = errors.New("version not found")
	errCorrupt = errors.New("version record damaged")
)
