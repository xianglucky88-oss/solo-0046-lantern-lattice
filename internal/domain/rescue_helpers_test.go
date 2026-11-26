package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"testing"
)

func readLayout(t *testing.T, raw []byte) Metadata {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var layout Metadata
	found := false
	for _, f := range zr.File {
		if f.Name != "layout.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open layout: %v", err)
		}
		if err := json.NewDecoder(rc).Decode(&layout); err != nil {
			t.Fatalf("decode layout: %v", err)
		}
		rc.Close()
		found = true
	}
	if !found {
		t.Fatal("layout.json missing")
	}
	return layout
}

func pixelOpaque(t *testing.T, raw []byte, g Grid, x, y, z int) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != "probes.png" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open texture: %v", err)
		}
		im, err := png.Decode(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("decode texture: %v", err)
		}
		px, py := pixel(g, x, y, z, 0)
		_, _, _, a := im.At(px, py).RGBA()
		return a == 0xffff
	}
	t.Fatal("probes.png missing")
	return false
}
