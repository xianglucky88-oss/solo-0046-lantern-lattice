package domain

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"math"
	"testing"
)

func demoRequest() Request {
	return Request{
		Grid:     Grid{Step: 1, Size: [3]int{3, 2, 2}},
		Exposure: 3,
		Lights: []Light{{
			Position: Vec{-1, 0, 0},
			RGB:      Vec{1, 0.55, 0.2},
			Radius:   6,
			Power:    4,
		}},
		Blocks: []Box{
			{Min: Vec{0.8, -0.2, -0.2}, Max: Vec{1.2, 0.2, 0.2}},
			{Min: Vec{1.4, -0.2, -0.2}, Max: Vec{1.6, 1.2, 1.2}},
		},
		Samples: []Vec{{0, 0, 0}},
	}
}

func readZipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

func decodeLayout(t *testing.T, data []byte) Metadata {
	t.Helper()
	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLegacyBuildUnchanged(t *testing.T) {
	pkg, err := Build(demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, pkg)
	if len(entries) != 2 {
		t.Fatalf("legacy package has %d entries", len(entries))
	}
	if _, ok := entries["probes.png"]; !ok {
		t.Fatal("legacy package missing probes.png")
	}
	meta := decodeLayout(t, entries["layout.json"])
	if meta.Schema != schemaV1 || len(meta.Tiles) != 0 || len(meta.Pages) != 0 {
		t.Fatalf("unexpected legacy metadata schema=%s", meta.Schema)
	}
	frame, err := ImportArchive(pkg)
	if err != nil {
		t.Fatalf("legacy import rejected: %v", err)
	}
	if frame.Schema != schemaV1 {
		t.Fatalf("imported schema %s", frame.Schema)
	}
}

func TestTiledBuildRoundTrip(t *testing.T) {
	req := demoRequest()
	maxW, maxH := 2, 2
	req.MaxTextureWidth, req.MaxTextureHeight = &maxW, &maxH
	pkg, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	entries := readZipEntries(t, pkg)
	meta := decodeLayout(t, entries["layout.json"])
	if meta.Schema != schemaV2 {
		t.Fatalf("schema %s", meta.Schema)
	}
	if len(meta.Pages) == 0 || len(meta.Tiles) == 0 {
		t.Fatal("tiled package declares no pages/tiles")
	}
	for _, page := range meta.Pages {
		raw, ok := entries[page.File]
		if !ok {
			t.Fatalf("missing declared page %s", page.File)
		}
		im, err := decodePagePNG(raw)
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dx() > maxW || im.Bounds().Dy() > maxH {
			t.Fatalf("page %s exceeds limits", page.File)
		}
	}
	if _, exists := entries["probes.png"]; exists {
		t.Fatal("tiled package must not embed the full strip")
	}
	frame, err := ImportArchive(pkg)
	if err != nil {
		t.Fatalf("tiled import rejected: %v", err)
	}

	legacyPkg, err := Build(demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	legacyFrame, err := ImportArchive(legacyPkg)
	if err != nil {
		t.Fatal(err)
	}
	if !atlasEqual(frame.logical, legacyFrame.logical) {
		t.Fatal("tiled import did not reconstruct the legacy logical texture")
	}
}

func atlasEqual(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.NRGBAAt(x, y) != b.NRGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func mutatePackage(t *testing.T, pkg []byte, mutate func(meta *Metadata, entries map[string][]byte)) []byte {
	t.Helper()
	entries := readZipEntries(t, pkg)
	meta := decodeLayout(t, entries["layout.json"])
	mutate(&meta, entries)
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	entries["layout.json"] = raw
	files := make([]archiveFile, 0, len(entries))
	for name, data := range entries {
		files = append(files, archiveFile{name: name, data: data})
	}
	out, err := writeZip(files)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestImportRejectsDefects(t *testing.T) {
	req := demoRequest()
	maxW, maxH := 3, 3
	req.MaxTextureWidth, req.MaxTextureHeight = &maxW, &maxH
	pkg, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing declared page", func(t *testing.T) {
		bad := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
			delete(entries, meta.Pages[0].File)
		})
		if _, err := ImportArchive(bad); err == nil {
			t.Fatal("missing page was accepted")
		}
	})

	t.Run("tile out of bounds", func(t *testing.T) {
		bad := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
			meta.Tiles[0].Target.X = meta.Pages[meta.Tiles[0].Page].Width
		})
		if _, err := ImportArchive(bad); err == nil {
			t.Fatal("out-of-bounds tile was accepted")
		}
	})

	t.Run("coverage hole", func(t *testing.T) {
		bad := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
			// Shrink the first fragment on both sides; its last logical
			// pixel loses every source.
			meta.Tiles[0].Source.W--
			meta.Tiles[0].Target.W--
		})
		if _, err := ImportArchive(bad); err == nil {
			t.Fatal("coverage hole was accepted")
		}
	})

	t.Run("conflicting duplicate pixel", func(t *testing.T) {
		req := demoRequest()
		roomW, roomH := 4, 4
		req.MaxTextureWidth, req.MaxTextureHeight = &roomW, &roomH
		roomyPkg, err := Build(req)
		if err != nil {
			t.Fatal(err)
		}
		bad := mutatePackage(t, roomyPkg, func(meta *Metadata, entries map[string][]byte) {
			dup := meta.Tiles[0]
			dup.Target.X++ // free texel inside the 4-wide page
			meta.Tiles = append([]TileRef{dup}, meta.Tiles...)
			pageFile := meta.Pages[dup.Page].File
			im := mustDecodePNG(t, entries[pageFile])
			im.SetNRGBA(dup.Target.X, dup.Target.Y, color.NRGBA{R: 200, G: 1, B: 2, A: 255})
			entries[pageFile] = mustEncodePNG(t, im)
		})
		if _, err := ImportArchive(bad); err == nil {
			t.Fatal("conflicting duplicate was accepted")
		}
	})

	t.Run("duplicate pixel with identical encoding is accepted", func(t *testing.T) {
		bad := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
			dup := meta.Tiles[0]
			meta.Tiles = append([]TileRef{dup}, meta.Tiles...)
		})
		if _, err := ImportArchive(bad); err != nil {
			t.Fatalf("identical duplicate rejected: %v", err)
		}
	})

	t.Run("actual page size disagrees", func(t *testing.T) {
		bad := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
			meta.Pages[0].Width++
		})
		if _, err := ImportArchive(bad); err == nil {
			t.Fatal("wrong declared page size was accepted")
		}
	})

	t.Run("undeclared file rejected", func(t *testing.T) {
		entries := readZipEntries(t, pkg)
		entries["pages/rogue.txt"] = []byte("not a page")
		files := make([]archiveFile, 0, len(entries))
		for name, data := range entries {
			files = append(files, archiveFile{name: name, data: data})
		}
		out, err := writeZip(files)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ImportArchive(out); err == nil {
			t.Fatal("undeclared file was accepted")
		}
	})

	t.Run("garbage archive rejected", func(t *testing.T) {
		if _, err := ImportArchive([]byte("not a zip at all")); err == nil {
			t.Fatal("garbage was accepted")
		}
	})

	t.Run("grayscale png rejected", func(t *testing.T) {
		entries := readZipEntries(t, pkg)
		meta := decodeLayout(t, entries["layout.json"])
		page := meta.Pages[0]
		gray := grayscalePNG(page.Width, page.Height)
		entries[page.File] = gray
		files := make([]archiveFile, 0, len(entries))
		for name, data := range entries {
			files = append(files, archiveFile{name: name, data: data})
		}
		out, err := writeZip(files)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ImportArchive(out); err == nil {
			t.Fatal("grayscale page was accepted")
		}
	})
}

func TestReorderedTilesImportIdentically(t *testing.T) {
	req := demoRequest()
	maxW, maxH := 2, 2
	req.MaxTextureWidth, req.MaxTextureHeight = &maxW, &maxH
	pkg, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := mutatePackage(t, pkg, func(meta *Metadata, entries map[string][]byte) {
		meta.Tiles[0], meta.Tiles[1] = meta.Tiles[1], meta.Tiles[0]
	})
	a, err := ImportArchive(pkg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ImportArchive(shuffled)
	if err != nil {
		t.Fatalf("reordered package rejected: %v", err)
	}
	if !atlasEqual(a.logical, b.logical) {
		t.Fatal("tile order changed the logical texture")
	}
}

func TestLimitValidation(t *testing.T) {
	req := demoRequest()
	small := 1
	req.MaxTextureWidth = &small
	if _, err := Build(req); err == nil {
		t.Fatal("width below 2 accepted")
	}
	height := 2
	req.MaxTextureWidth, req.MaxTextureHeight = nil, &height
	if _, err := Build(req); err == nil {
		t.Fatal("single limit accepted")
	}
}

func TestSamplerMatchesLegacy(t *testing.T) {
	legacy, err := ImportArchive(mustBuild(t, demoRequest()))
	if err != nil {
		t.Fatal(err)
	}
	req := demoRequest()
	maxW, maxH := 2, 2
	req.MaxTextureWidth, req.MaxTextureHeight = &maxW, &maxH
	tiled, err := ImportArchive(mustBuild(t, req))
	if err != nil {
		t.Fatal(err)
	}
	for face := 0; face < 6; face++ {
		for depth := 0; depth < req.Grid.Size[2]; depth++ {
			for _, q := range [][2]float64{{0, 0}, {0.3, 0.7}, {0.5, 0.5}, {1, 1}, {0.99, 0.01}} {
				a, err := legacy.Sample(face, depth, q[0], q[1])
				if err != nil {
					t.Fatal(err)
				}
				b, err := tiled.Sample(face, depth, q[0], q[1])
				if err != nil {
					t.Fatal(err)
				}
				if a.Available != b.Available || math.Abs(a.Weight-b.Weight) > 1e-12 ||
					math.Abs(a.R-b.R) > 1e-9 || math.Abs(a.G-b.G) > 1e-9 || math.Abs(a.B-b.B) > 1e-9 {
					t.Fatalf("sample mismatch face=%d depth=%d q=%v legacy=%+v tiled=%+v", face, depth, q, a, b)
				}
			}
		}
	}
}

func TestSamplerWeights(t *testing.T) {
	frame, err := ImportArchive(mustBuild(t, demoRequest()))
	if err != nil {
		t.Fatal(err)
	}
	// Invalid probe region: probe (1,0,0) is blocked; find a face/layer/u/v
	// whose four footprint texels are all transparent and expect unavailable.
	var foundUnavailable bool
	for face := 0; face < 6; face++ {
		for depth := 0; depth < frame.Grid.Size[2]; depth++ {
			allTransparent := true
			ox := face * frame.Grid.Size[0]
			oy := depth * frame.Grid.Size[1]
			for _, p := range [][2]int{{1, 0}, {2, 0}, {1, 1}, {2, 1}} {
				if frame.logical.NRGBAAt(ox+p[0], oy+p[1]).A != 0 {
					allTransparent = false
				}
			}
			if !allTransparent {
				continue
			}
			// u mapping: texel x=1 center is u = 1/(X-1).
			u := 1.0 / float64(frame.Grid.Size[0]-1)
			v := 0.0
			res, err := frame.Sample(face, depth, u, v)
			if err != nil {
				t.Fatal(err)
			}
			if res.Available || res.Weight != 0 {
				t.Fatalf("transparent footprint returned %+v", res)
			}
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Skip("demo scene has no fully transparent bilinear footprint")
	}

	// Valid black still contributes weight: build a synthetic frame with an
	// opaque black texel and transparent neighbors.
	synthetic := syntheticFrame(t)
	res, err := synthetic.Sample(0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available || res.Weight != 1 || res.R != 0 || res.G != 0 || res.B != 0 {
		t.Fatalf("black texel must count with full weight, got %+v", res)
	}

	if _, err := synthetic.Sample(6, 0, 0, 0); err == nil {
		t.Fatal("invalid face accepted")
	}
	if _, err := synthetic.Sample(0, 99, 0, 0); err == nil {
		t.Fatal("invalid depth accepted")
	}
	if _, err := synthetic.Sample(0, 0, 1.5, 0); err == nil {
		t.Fatal("out-of-range u accepted")
	}
	if _, err := synthetic.Sample(0, 0, math.NaN(), 0); err == nil {
		t.Fatal("NaN u accepted")
	}
}

func syntheticFrame(t *testing.T) *Frame {
	t.Helper()
	g := Grid{Step: 1, Size: [3]int{2, 2, 2}}
	meta := Metadata{Schema: schemaV1, Grid: g, Axes: axisNames, TextureWidth: 12, TextureHeight: 4}
	layout, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewNRGBA(image.Rect(0, 0, 12, 4))
	im.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 255}) // valid black at face0, layer0
	pngBytes := mustEncodePNG(t, im)
	out, err := writeZip([]archiveFile{
		{name: "layout.json", data: layout},
		{name: "probes.png", data: pngBytes},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := ImportArchive(out)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func mustBuild(t *testing.T, r Request) []byte {
	t.Helper()
	out, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustDecodePNG(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	im, err := decodePagePNG(data)
	if err != nil {
		t.Fatal(err)
	}
	return asNRGBA(im)
}

func mustEncodePNG(t *testing.T, im image.Image) []byte {
	t.Helper()
	out, err := encodePNG(im)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// grayscalePNG hand-rolls an 8-bit grayscale (color type 0) PNG.
func grayscalePNG(w, h int) []byte {
	chunk := func(kind string, payload []byte) []byte {
		out := []byte{}
		out = binary.BigEndian.AppendUint32(out, uint32(len(payload)))
		out = append(out, kind...)
		out = append(out, payload...)
		sum := crc32.ChecksumIEEE(append([]byte(kind), payload...))
		return binary.BigEndian.AppendUint32(out, sum)
	}
	raw := make([]byte, 0, h*(w+1))
	for y := 0; y < h; y++ {
		raw = append(raw, 0)
		for x := 0; x < w; x++ {
			raw = append(raw, uint8((x+y)%256))
		}
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(h))
	ihdr[8], ihdr[9] = 8, 0
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	b.Write(chunk("IHDR", ihdr))
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write(raw)
	zw.Close()
	b.Write(chunk("IDAT", zbuf.Bytes()))
	b.Write(chunk("IEND", nil))
	return b.Bytes()
}
