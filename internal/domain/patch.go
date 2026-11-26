package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Patch is the self-contained supplement delivered once for a continuation.
// Together with the previous receipt and the previous ZIP it reconstructs the
// exact bytes of the new ZIP offline.
type Patch struct {
	Schema        string        `json:"schema"`
	BaseVersion   string        `json:"base_version"`
	BaseDigest    string        `json:"base_digest"`
	Version       string        `json:"version"`
	Digest        string        `json:"digest"`
	Grid          Grid          `json:"grid"`
	Blocks        []Box         `json:"blocks"`
	Samples       []Vec         `json:"samples"`
	Exposure      float64       `json:"exposure"`
	Lights        []StableLight `json:"lights"`
	Resolved      [][3]int      `json:"resolved"`
	OcclusionOnly [][3]int      `json:"occlusion_only"`
}

var (
	// ErrPatchIdentity means the patch's base version does not match the
	// presented previous receipt.
	ErrPatchIdentity = errors.New("patch base version does not match receipt")
	// ErrPatchBase means the presented previous ZIP does not match the
	// digest recorded in the patch and receipt.
	ErrPatchBase = errors.New("previous artifact bytes do not match receipt")
	// ErrPatchDigest means the reconstructed artifact does not match the
	// digest promised by the patch.
	ErrPatchDigest = errors.New("rebuilt artifact digest mismatch")
	// ErrPatchFormat means the patch document cannot be decoded.
	ErrPatchFormat = errors.New("invalid patch document")
)

// BuildPatch assembles the supplement for one successful continuation.
func BuildPatch(baseRev, newRev Revision, baseDigest, baseVersion, newVersion, newDigest string, sets RevisionSets) (Patch, error) {
	p := Patch{
		Schema:        "lantern-patch/v1",
		BaseVersion:   baseVersion,
		BaseDigest:    baseDigest,
		Version:       newVersion,
		Digest:        newDigest,
		Grid:          newRev.Grid,
		Blocks:        newRev.Blocks,
		Samples:       newRev.Samples,
		Exposure:      newRev.Exposure,
		Lights:        newRev.Lights,
		Resolved:      positions(baseRev.Grid, sets.Resolved),
		OcclusionOnly: positions(baseRev.Grid, sets.OcclusionOnly),
	}
	return p, nil
}

// positions maps flat probe indices back to integer grid coordinates.
func positions(g Grid, indices []int) [][3]int {
	out := make([][3]int, 0, len(indices))
	X, Y := g.Size[0], g.Size[1]
	for _, idx := range indices {
		x := idx % X
		rest := idx / X
		out = append(out, [3]int{x, rest % Y, rest / Y})
	}
	return out
}

// EncodePatch serializes a patch as one JSON document.
func EncodePatch(p Patch) ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

// DecodePatch parses a patch document, rejecting malformed inputs.
func DecodePatch(data []byte) (Patch, error) {
	var p Patch
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Patch{}, ErrPatchFormat
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Patch{}, ErrPatchFormat
	}
	if p.Schema != "lantern-patch/v1" || p.Version == "" || p.Digest == "" || p.BaseVersion == "" || p.BaseDigest == "" {
		return Patch{}, ErrPatchFormat
	}
	return p, nil
}

// PatchReceipt is the minimal previous-receipt information needed offline.
type PatchReceipt struct {
	Digest  string `json:"digest"`
	Version string `json:"version"`
}

// ApplyPatch reconstructs the new ZIP without any server access. It refuses to
// run unless the previous light-version identity and the previous ZIP bytes
// both match the patch chain, and verifies the rebuilt ZIP against the digest
// promised in the patch.
func ApplyPatch(p Patch, receipt PatchReceipt, previousZIP []byte) ([]byte, error) {
	if receipt.Version != p.BaseVersion {
		return nil, ErrPatchIdentity
	}
	if Digest(previousZIP) != p.BaseDigest || receipt.Digest != p.BaseDigest {
		return nil, ErrPatchBase
	}
	rev := Revision{Grid: p.Grid, Blocks: p.Blocks, Samples: p.Samples, Exposure: p.Exposure, Lights: p.Lights}
	zipBytes, err := Assemble(rev.Request())
	if err != nil {
		return nil, err
	}
	if Digest(zipBytes) != p.Digest {
		return nil, ErrPatchDigest
	}
	return zipBytes, nil
}
