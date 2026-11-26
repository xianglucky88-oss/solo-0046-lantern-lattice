package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const FineSchema = "lantern-fine/v1"
const FineEntry = "fine.json"

// FineProbe is the exact linear six-direction lighting of one grid probe.
type FineProbe struct {
	Index      int   `json:"Index"`
	Valid      bool  `json:"Valid"`
	Position   Vec   `json:"Position"`
	Irradiance Lobes `json:"Irradiance"`
}

// FineDocument is the separately downloadable fine tier. Values are the
// original float64 bake energies encoded as shortest round-trippable JSON.
type FineDocument struct {
	Schema string      `json:"Schema"`
	Grid   Grid        `json:"Grid"`
	Probes []FineProbe `json:"Probes"`
}

func buildFineDocument(r Request, probes []Probe) FineDocument {
	items := make([]FineProbe, len(probes))
	for i, p := range probes {
		items[i] = FineProbe{Index: i, Valid: p.Valid, Position: p.Position, Irradiance: p.Irradiance}
	}
	return FineDocument{Schema: FineSchema, Grid: r.Grid, Probes: items}
}

func packFine(r Request, probes []Probe) ([]byte, *FineDocument, error) {
	doc := buildFineDocument(r, probes)
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	blob, err := packEntries([]zipEntry{{FineEntry, raw}})
	return blob, &doc, err
}

// LoadFine opens a fine-tier zip and returns the document.
func LoadFine(blob []byte) (*FineDocument, error) {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return nil, fmt.Errorf("fine tier is not a zip: %w", err)
	}
	file := findZipEntry(zr, FineEntry)
	if file == nil {
		return nil, fmt.Errorf("fine tier missing %s", FineEntry)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var doc FineDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("fine tier decode: %w", err)
	}
	if doc.Schema != FineSchema {
		return nil, fmt.Errorf("unexpected fine schema %q", doc.Schema)
	}
	return &doc, nil
}

// LobesMap returns exact lobes keyed by canonical probe index. Invalid probes
// are omitted. If indices is non-empty, only those indices are included.
func (d *FineDocument) LobesMap(indices map[int]bool) map[int]Lobes {
	out := make(map[int]Lobes)
	for _, p := range d.Probes {
		if !p.Valid {
			continue
		}
		if indices != nil && !indices[p.Index] {
			continue
		}
		out[p.Index] = p.Irradiance
	}
	return out
}

func findZipEntry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}
