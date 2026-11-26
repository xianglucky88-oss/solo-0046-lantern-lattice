package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// VersionPrefix marks content-addressed light-version identities.
const VersionPrefix = "rv_"

// fingerprint is the canonical state whose digest is the light version.
type fingerprint struct {
	Schema   string        `json:"schema"`
	Grid     Grid          `json:"grid"`
	Blocks   []Box         `json:"blocks"`
	Samples  []Vec         `json:"samples"`
	Exposure float64       `json:"exposure"`
	Lights   []StableLight `json:"lights"`
}

// VersionID derives the stable, deterministic light-version identity of a
// revision. Re-sending the same continuation against the same base yields the
// same identity; distinct light states never collide except by SHA-256 itself.
func VersionID(rev Revision) (string, error) {
	doc, err := json.Marshal(fingerprint{
		Schema:   "lantern-revision/v1",
		Grid:     rev.Grid,
		Blocks:   rev.Blocks,
		Samples:  rev.Samples,
		Exposure: rev.Exposure,
		Lights:   rev.Lights,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(doc)
	return VersionPrefix + hex.EncodeToString(sum[:]), nil
}

// Digest returns the SHA-256 content digest of arbitrary bytes.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
