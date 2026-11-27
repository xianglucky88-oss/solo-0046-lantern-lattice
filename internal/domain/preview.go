package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoBasis is returned when an artifact carries no full-precision relighting
// basis (legacy product). Callers may fall back to quantized PNG recovery.
var ErrNoBasis = errors.New("artifact has no linear preview basis")

// Artifact is the in-memory view of a downloaded product used by the
// artifact-only preview tool. It never needs the original build request.
type Artifact struct {
	Basis
	Layout []byte
	PNG    []byte
	legacy bool
}

// ReadArtifact opens a product zip and locates its linear basis. Products
// produced by this implementation always carry a canonical "Preview" basis
// when sky was enabled; nearby container keys are accepted too.
func ReadArtifact(zipBytes []byte) (*Artifact, error) {
	layoutRaw, pngRaw, err := ReadArtifactFiles(zipBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid product archive: %w", err)
	}
	var env layoutEnvelope
	if err := json.Unmarshal(layoutRaw, &env); err != nil {
		return nil, fmt.Errorf("invalid layout.json: %w", err)
	}
	a := &Artifact{Layout: layoutRaw, PNG: pngRaw}

	for _, candidate := range []json.RawMessage{env.Preview, env.Relight, env.Lighting, env.Basis, env.Rig, env.Base} {
		if len(trimJSON(candidate)) == 0 || string(trimJSON(candidate)) == "null" {
			continue
		}
		var b Basis
		if err := json.Unmarshal(candidate, &b); err == nil && len(b.Probes) > 0 {
			a.Basis = b
			return a, nil
		}
	}

	// Top-level Probes array (extended metadata) fallback.
	if len(trimJSON(env.Probes)) > 0 {
		var recs []BasisProbe
		if err := json.Unmarshal(env.Probes, &recs); err == nil && len(recs) > 0 {
			a.Basis = Basis{
				Schema:     PreviewSchema,
				Grid:       env.Grid,
				Axes:       axisNames,
				Exposure:   env.Exposure,
				Width:      env.Grid.Size[0] * 6,
				Height:     env.Grid.Size[1] * env.Grid.Size[2],
				Blocks:     env.Blocks,
				ProbeCount: len(recs),
				Probes:     recs,
			}
			return a, nil
		}
	}

	// Legacy artifact: only aggregate layout fields are available. Sampling
	// geometry still works if Blocks happen to be present; exact group
	// separation does not.
	a.legacy = true
	a.Grid = env.Grid
	a.Exposure = env.Exposure
	a.Blocks = env.Blocks
	a.Axes = axisNames
	a.Width = env.Grid.Size[0] * 6
	a.Height = env.Grid.Size[1] * env.Grid.Size[2]
	return a, ErrNoBasis
}

// HasBasis reports whether exact group relighting is possible.
func (a *Artifact) HasBasis() bool { return !a.legacy && len(a.Probes) > 0 }

// relitProbes rebuilds grid probes with linear lobes
//
//	L' = lightScale * originalLight + skyScale * originalSky
//
// Both scales apply to the stored full-precision originals, so repeated
// adjustments never feed a quantized result back in.
func (a *Artifact) relitProbes(skyScale, lightScale float64) []Probe {
	skyScale = ScaleClamp(skyScale)
	lightScale = ScaleClamp(lightScale)
	g := a.Grid
	probes := make([]Probe, g.count())
	for i := range probes {
		probes[i].Position = g.position(0, 0, 0) // overwritten for known cells below
	}
	for _, rec := range a.Probes {
		idx := rec.Index
		if idx < 0 || idx >= len(probes) {
			if rec.X >= 0 && rec.Y >= 0 && rec.Z >= 0 {
				idx = g.index(rec.X, rec.Y, rec.Z)
			}
		}
		if idx < 0 || idx >= len(probes) {
			continue
		}
		lamp := rec.Light
		sky := rec.Sky
		// Tolerate records that only populated alias keys.
		if isZeroLobes(lamp) {
			lamp = firstNonZeroLobes(rec.Lights, rec.Lamp, rec.Lamps, rec.Direct)
		}
		if isZeroLobes(sky) {
			sky = firstNonZeroLobes(rec.SkyLight, rec.Ambient)
		}
		p := Probe{Position: rec.Position, Valid: rec.Valid, Lamp: lamp, Sky: sky}
		if p.Position == (Vec{}) {
			x, y, z := rec.X, rec.Y, rec.Z
			p.Position = g.position(x, y, z)
		}
		for f := range 6 {
			p.Irradiance[f] = lamp[f].scale(lightScale).add(sky[f].scale(skyScale))
		}
		probes[idx] = p
	}
	return probes
}

func isZeroLobes(l Lobes) bool {
	for i := range l {
		for _, v := range l[i] {
			if v != 0 {
				return false
			}
		}
	}
	return true
}

func firstNonZeroLobes(options ...Lobes) Lobes {
	for _, o := range options {
		if !isZeroLobes(o) {
			return o
		}
	}
	return Lobes{}
}

// RenderPNG produces the relit probe atlas using the original exposure map.
func (a *Artifact) RenderPNG(skyScale, lightScale float64) ([]byte, int, int, error) {
	probes := a.relitProbes(skyScale, lightScale)
	valid := make([]bool, len(probes))
	lobes := make([]Lobes, len(probes))
	for i, p := range probes {
		valid[i] = p.Valid
		lobes[i] = p.Irradiance
	}
	pngBytes, err := encodeAtlas(a.Grid, valid, lobes, a.Exposure)
	return pngBytes, a.Width, a.Height, err
}

// LinearSample is a spatial sample with the relit total and the separated
// scaled groups for clients that want each channel independently.
type LinearSample struct {
	Sample
	SkyLobes   Lobes `json:"SkyGroup"`
	LightLobes Lobes `json:"LightGroup"`
	SkyScale   float64
	LightScale float64
}

// SamplePositions evaluates arbitrary in-volume positions against the frozen
// visible neighbours and support weights. Multipliers change colours only;
// geometry, State and Weight are invariant. With both scales zero a supported
// point stays State "sampled" with zero Irradiance (valid black).
func (a *Artifact) SamplePositions(positions []Vec, skyScale, lightScale float64) []LinearSample {
	skyScale = ScaleClamp(skyScale)
	lightScale = ScaleClamp(lightScale)
	probes := a.relitProbes(skyScale, lightScale)
	out := make([]LinearSample, 0, len(positions))
	for _, p := range positions {
		total := sample(a.Grid, probes, p, a.Blocks)
		ls := LinearSample{
			Sample:     total,
			SkyScale:   skyScale,
			LightScale: lightScale,
		}
		// Re-run the same supported-neighbour accumulation for each group so
		// the separated linear values share exactly the same support.
		cell, ok := locate(a.Grid, p)
		if ok && !occupied(p, a.Blocks) {
			var wsum float64
			var skyAcc, lampAcc Lobes
			for _, n := range corners(a.Grid, cell) {
				pr := probes[n.index]
				if !pr.Valid || !visible(p, pr.Position, a.Blocks) {
					continue
				}
				wsum += n.weight
				for f := range 6 {
					skyAcc[f] = skyAcc[f].add(pr.Sky[f].scale(n.weight))
					lampAcc[f] = lampAcc[f].add(pr.Lamp[f].scale(n.weight))
				}
			}
			if wsum > 0 {
				for f := range 6 {
					ls.SkyLobes[f] = skyAcc[f].scale(1 / wsum).scale(skyScale)
					ls.LightLobes[f] = lampAcc[f].scale(1 / wsum).scale(lightScale)
				}
			}
		}
		out = append(out, ls)
	}
	return out
}

// PreviewResult bundles everything an HTTP/CLI preview response may expose.
type PreviewResult struct {
	Artifact   *Artifact
	Probes     []Probe
	PNG        []byte
	Width      int
	Height     int
	Samples    []LinearSample
	SkyScale   float64
	LightScale float64
}

// Preview runs render + optional ad-hoc sampling from artifact bytes only.
func Preview(zipBytes []byte, skyScale, lightScale float64, positions []Vec) (*PreviewResult, error) {
	a, err := ReadArtifact(zipBytes)
	if err != nil && !errors.Is(err, ErrNoBasis) {
		return nil, err
	}
	if errors.Is(err, ErrNoBasis) {
		// Best-effort recovery of validity and quantized energies from the
		// original texture so legacy products still dim coherently.
		a.RecoverLegacy()
	}
	probes := a.relitProbes(skyScale, lightScale)
	valid := make([]bool, len(probes))
	lobes := make([]Lobes, len(probes))
	for i, p := range probes {
		valid[i] = p.Valid
		lobes[i] = p.Irradiance
	}
	pngBytes, rerr := encodeAtlas(a.Grid, valid, lobes, a.Exposure)
	if rerr != nil {
		return nil, rerr
	}
	res := &PreviewResult{
		Artifact:   a,
		Probes:     probes,
		PNG:        pngBytes,
		Width:      a.Width,
		Height:     a.Height,
		SkyScale:   ScaleClamp(skyScale),
		LightScale: ScaleClamp(lightScale),
	}
	if len(positions) > 0 {
		res.Samples = a.SamplePositions(positions, skyScale, lightScale)
	}
	return res, nil
}
