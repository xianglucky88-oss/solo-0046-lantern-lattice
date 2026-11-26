package domain

import (
	"encoding/json"
	"fmt"
)

// TierBundle is the result of a bake: the unchanged original product plus
// optional separately-deliverable coarse and fine archives.
type TierBundle struct {
	Original  []byte
	Coarse    []byte
	Fine      []byte
	CoarseDoc *CoarseDocument
}

// Build produces only the original product. It preserves the legacy entrypoint.
func Build(r Request) ([]byte, error) {
	bundle, err := BuildTiered(r, TierOptions{})
	if err != nil {
		return nil, err
	}
	return bundle.Original, nil
}

// BuildTiered validates and bakes the scene, always producing the original
// archive and, when opts.Requested, the coarse and fine archives.
func BuildTiered(r Request, opts TierOptions) (*TierBundle, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	if opts.Requested {
		if opts.ByteBudget < -1 {
			return nil, fmt.Errorf("coarse byte budget must be non-negative")
		}
		if opts.ErrorSet && (!isFinite(opts.MaxLinearError) || opts.MaxLinearError < 0) {
			return nil, fmt.Errorf("coarse linear error tolerance must be non-negative")
		}
		if !opts.ErrorSet {
			opts.MaxLinearError = DefaultCoarseError
		}
	}

	probes := bake(r)
	metadata, err := json.MarshalIndent(describe(r, probes), "", "  ")
	if err != nil {
		return nil, err
	}
	atlas, err := texture(r.Grid, probes, r.Exposure)
	if err != nil {
		return nil, err
	}
	original, err := pack(metadata, atlas)
	if err != nil {
		return nil, err
	}
	bundle := &TierBundle{Original: original}
	if !opts.Requested {
		return bundle, nil
	}

	fineBlob, _, err := packFine(r, probes)
	if err != nil {
		return bundle, err
	}
	bundle.Fine = fineBlob

	coarseDoc, err := encodeCoarse(r, probes, opts)
	if err != nil {
		// Keep Original and Fine on the bundle so callers may deliver them
		// while reporting the coarse-tier failure explicitly.
		return bundle, err
	}
	coarseRaw, err := json.MarshalIndent(coarseDoc, "", "  ")
	if err != nil {
		return bundle, err
	}
	coarseBlob, err := packEntries([]zipEntry{{CoarseEntry, coarseRaw}})
	if err != nil {
		return bundle, err
	}
	bundle.Coarse = coarseBlob
	bundle.CoarseDoc = coarseDoc
	return bundle, nil
}

// DefaultCoarseError is used when tiering is requested without an explicit
// linear error tolerance.
const DefaultCoarseError = 0.05

func isFinite(v float64) bool {
	return v == v && v > -1e300 && v < 1e300
}
