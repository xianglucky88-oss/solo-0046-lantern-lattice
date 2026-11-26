package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// LoadCoarse opens a coarse-tier zip and returns the document together with
// materialized probes (positions, validity and approximate lobes). The probes
// plus Grid/Blocks are sufficient for offline TieredQuery without lights/fine.
func LoadCoarse(blob []byte) (*CoarseDocument, []Probe, error) {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, nil, fmt.Errorf("coarse tier is not a zip: %w", err)
	}
	file := findZipEntry(zr, CoarseEntry)
	if file == nil {
		return nil, nil, fmt.Errorf("coarse tier missing %s", CoarseEntry)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, err
	}
	var doc CoarseDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("coarse tier decode: %w", err)
	}
	if doc.Schema != CoarseSchema {
		return nil, nil, fmt.Errorf("unexpected coarse schema %q", doc.Schema)
	}
	probes, err := doc.RuntimeProbes()
	if err != nil {
		return nil, nil, err
	}
	return &doc, probes, nil
}

// RuntimeProbes decodes the compact lighting payload and rebuilds the full
// canonical-index probe slice. Validity is recomputed from the packed boxes,
// exactly as during baking.
func (d *CoarseDocument) RuntimeProbes() ([]Probe, error) {
	g := d.Grid
	total := g.count()
	probes := make([]Probe, total)
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				i := g.index(x, y, z)
				p := g.position(x, y, z)
				probes[i] = Probe{Position: p, Valid: !occupied(p, d.Blocks)}
			}
		}
	}

	if d.FaceCount == 0 || d.Encoding.Mode == "empty-v1" {
		return probes, nil
	}

	colorBytes, err := base64.StdEncoding.DecodeString(d.Encoding.ColorsBase64)
	if err != nil {
		return nil, fmt.Errorf("coarse colors base64: %w", err)
	}
	lookupBytes, err := base64.StdEncoding.DecodeString(d.Encoding.LookupBase64)
	if err != nil {
		return nil, fmt.Errorf("coarse lookup base64: %w", err)
	}

	faces := make([]Vec, d.FaceCount)
	switch d.Encoding.Mode {
	case "exact-float64-v1":
		if len(colorBytes) != 24*d.FaceCount {
			return nil, fmt.Errorf("exact coarse payload length %d, want %d", len(colorBytes), 24*d.FaceCount)
		}
		for ch := 0; ch < 3; ch++ {
			for f := 0; f < d.FaceCount; f++ {
				off := 8 * (ch*d.FaceCount + f)
				faces[f][ch] = math.Float64frombits(binary.LittleEndian.Uint64(colorBytes[off : off+8]))
			}
		}

	case "direct-codes-v1", "palette-codes-v1":
		widths := d.Encoding.CodeWidths
		if len(widths) != 3 || len(d.Encoding.Scale) != 3 {
			return nil, fmt.Errorf("coarse code descriptor incomplete")
		}
		scale := d.Encoding.Scale

		if d.Encoding.Mode == "direct-codes-v1" {
			off := 0
			for ch := 0; ch < 3; ch++ {
				w := widths[ch]
				plane := d.FaceCount * w
				if off+plane > len(colorBytes) {
					return nil, fmt.Errorf("coarse direct colors truncated")
				}
				for f := 0; f < d.FaceCount; f++ {
					faces[f][ch] = float64(getUint(colorBytes, off+f*w, w)) * scale[ch]
				}
				off += plane
			}
		} else {
			iw := d.Encoding.IndexWidth
			if iw <= 0 {
				return nil, fmt.Errorf("coarse palette missing index width")
			}
			if len(lookupBytes) != d.FaceCount*iw {
				return nil, fmt.Errorf("coarse lookup length %d, want %d", len(lookupBytes), d.FaceCount*iw)
			}
			K := 0
			for ch := 0; ch < 3; ch++ {
				if widths[ch] <= 0 {
					return nil, fmt.Errorf("coarse palette bad code width")
				}
				K = len(colorBytes) / (widths[0] + widths[1] + widths[2])
			}
			palette := make([][3]float64, K)
			off := 0
			codes := make([][]uint64, 3)
			for ch := 0; ch < 3; ch++ {
				w := widths[ch]
				codes[ch] = make([]uint64, K)
				for k := 0; k < K; k++ {
					codes[ch][k] = getUint(colorBytes, off, w)
					off += w
				}
			}
			if off != len(colorBytes) {
				return nil, fmt.Errorf("coarse palette color length mismatch")
			}
			for k := 0; k < K; k++ {
				for ch := 0; ch < 3; ch++ {
					palette[k][ch] = float64(codes[ch][k]) * scale[ch]
				}
			}
			for f := 0; f < d.FaceCount; f++ {
				idx := int(getUint(lookupBytes, f*iw, iw))
				if idx < 0 || idx >= K {
					return nil, fmt.Errorf("coarse lookup index %d out of range %d", idx, K)
				}
				faces[f] = Vec(palette[idx])
			}
		}

	default:
		return nil, fmt.Errorf("unknown coarse encoding %q", d.Encoding.Mode)
	}

	// Scatter dense face colors back into canonical probe indices.
	f := 0
	for i := range probes {
		if !probes[i].Valid {
			continue
		}
		for face := 0; face < 6; face++ {
			probes[i].Irradiance[face] = faces[f]
			f++
		}
	}
	if f != d.FaceCount {
		return nil, fmt.Errorf("coarse face count mismatch: decoded %d of %d", f, d.FaceCount)
	}
	return probes, nil
}
