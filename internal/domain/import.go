package domain

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"path"
	"strings"
)

// Frame is an offline-imported probe texture set. Pixels live in logical probe
// coordinates; the original strip never has to be rebuilt by the caller.
type Frame struct {
	Schema   string
	Grid     Grid
	Axes     [6]string
	Exposure float64
	pages    []PageSpec
	tiles    []TileRef
	logical  *image.NRGBA
}

// SampleResult is a logical bilinear footprint on one face and depth layer.
type SampleResult struct {
	Available bool
	R         float64
	G         float64
	B         float64
	Weight    float64
}

// ImportArchive reads a download package without any network access. Both the
// legacy single-strip package and the tiled multi-page package enter here.
func ImportArchive(data []byte) (*Frame, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("package is not a readable zip archive")
	}
	files := map[string]*zip.File{}
	for _, entry := range zr.File {
		name := entry.Name
		if entry.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			continue
		}
		if name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("unsafe package entry %q", name)
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("duplicate package entry %q", name)
		}
		files[name] = entry
	}
	layoutFile, ok := files["layout.json"]
	if !ok {
		return nil, errors.New("package is missing layout.json")
	}
	raw, err := readZipFile(layoutFile)
	if err != nil {
		return nil, errors.New("cannot read layout.json")
	}
	var meta Metadata
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("invalid layout.json: %v", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("layout.json must contain exactly one JSON object")
	}
	if err := validateGrid(meta.Grid); err != nil {
		return nil, err
	}
	if len(meta.Tiles) == 0 {
		return importLegacy(files, meta)
	}
	return importTiled(files, meta)
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func validateGrid(g Grid) error {
	for _, n := range g.Size {
		if n < 2 || n > 16 {
			return errors.New("layout grid axes must have 2–16 probes")
		}
	}
	return nil
}

const (
	pngTrueColor      = 2
	pngTrueColorAlpha = 6
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// inspectPNGHeader reads the IHDR chunk directly because image/png's decoded
// config does not expose bit depth or color type.
func inspectPNGHeader(data []byte) (width, height, bitDepth, colorType int, err error) {
	if len(data) < 24 || !bytes.Equal(data[:8], pngSignature) {
		return 0, 0, 0, 0, errors.New("page is not a decodable PNG")
	}
	// IHDR follows the 8-byte signature: 4-byte length, "IHDR", then payload.
	if string(data[12:16]) != "IHDR" {
		return 0, 0, 0, 0, errors.New("page PNG is missing IHDR")
	}
	payload := data[16:29]
	width = int(binary.BigEndian.Uint32(payload[0:4]))
	height = int(binary.BigEndian.Uint32(payload[4:8]))
	bitDepth = int(payload[8])
	colorType = int(payload[9])
	return width, height, bitDepth, colorType, nil
}

func decodePagePNG(data []byte) (image.Image, error) {
	width, height, bitDepth, colorType, err := inspectPNGHeader(data)
	if err != nil {
		return nil, err
	}
	// Only 8-bit true-color (RGB color type 2) or true-color with alpha (RGBA
	// color type 6) is accepted; paletted, grayscale and >8-bit encodings are
	// not part of the package format.
	if bitDepth != 8 || (colorType != pngTrueColor && colorType != pngTrueColorAlpha) {
		return nil, errors.New("page PNG must be 8-bit RGB or RGBA")
	}
	if width < 2 || height < 2 {
		return nil, errors.New("page dimensions must be at least 2x2 pixels")
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("page is not a decodable PNG")
	}
	if im.Bounds().Dx() != width || im.Bounds().Dy() != height {
		return nil, errors.New("page header and image dimensions disagree")
	}
	return im, nil
}

func importLegacy(files map[string]*zip.File, meta Metadata) (*Frame, error) {
	if meta.Schema != schemaV1 {
		return nil, fmt.Errorf("single-strip package must declare schema %s", schemaV1)
	}
	if len(meta.Pages) != 0 {
		return nil, errors.New("legacy package must not declare pages")
	}
	if len(files) != 2 {
		return nil, errors.New("legacy package must contain exactly layout.json and probes.png")
	}
	stripFile, ok := files["probes.png"]
	if !ok {
		return nil, errors.New("package is missing probes.png")
	}
	raw, err := readZipFile(stripFile)
	if err != nil {
		return nil, errors.New("cannot read probes.png")
	}
	im, err := decodePagePNG(raw)
	if err != nil {
		return nil, err
	}
	wantW, wantH := meta.Grid.Size[0]*6, meta.Grid.Size[1]*meta.Grid.Size[2]
	if im.Bounds().Dx() != wantW || im.Bounds().Dy() != wantH {
		return nil, fmt.Errorf("probes.png is %dx%d but layout requires %dx%d", im.Bounds().Dx(), im.Bounds().Dy(), wantW, wantH)
	}
	if meta.TextureWidth != wantW || meta.TextureHeight != wantH {
		return nil, errors.New("layout texture dimensions disagree with the grid")
	}
	return &Frame{
		Schema:   meta.Schema,
		Grid:     meta.Grid,
		Axes:     meta.Axes,
		Exposure: meta.Exposure,
		logical:  asNRGBA(im),
	}, nil
}

func asNRGBA(im image.Image) *image.NRGBA {
	if n, ok := im.(*image.NRGBA); ok {
		return n
	}
	out := image.NewNRGBA(im.Bounds())
	for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
		for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
			out.Set(x, y, im.At(x, y))
		}
	}
	return out
}

func validateRegion(r Region, bounds image.Rectangle, label string) error {
	if r.W < 1 || r.H < 1 {
		return fmt.Errorf("%s region must be non-empty", label)
	}
	rect := r.rect()
	if !rect.In(bounds) {
		return fmt.Errorf("%s region %v falls outside %v", label, rect, bounds)
	}
	return nil
}

func importTiled(files map[string]*zip.File, meta Metadata) (*Frame, error) {
	if meta.Schema != schemaV2 {
		return nil, fmt.Errorf("tiled package must declare schema %s", schemaV2)
	}
	if len(meta.Pages) == 0 {
		return nil, errors.New("tiled layout declares no pages")
	}
	pageImages := make([]*image.NRGBA, len(meta.Pages))
	pageNames := make(map[string]int, len(meta.Pages))
	for i, page := range meta.Pages {
		if page.Width < 2 || page.Height < 2 {
			return nil, fmt.Errorf("page %d dimensions must be at least 2x2 pixels", i)
		}
		if page.File != path.Clean(page.File) || !strings.HasPrefix(page.File, "pages/") || strings.Contains(page.File, "\\") {
			return nil, fmt.Errorf("page %d has an invalid file name", i)
		}
		if prev, dup := pageNames[page.File]; dup {
			return nil, fmt.Errorf("page file %q declared by pages %d and %d", page.File, prev, i)
		}
		pageNames[page.File] = i
		f, ok := files[page.File]
		if !ok {
			return nil, fmt.Errorf("declared page %q is missing from the package", page.File)
		}
		raw, err := readZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("cannot read page %q", page.File)
		}
		im, err := decodePagePNG(raw)
		if err != nil {
			return nil, fmt.Errorf("page %q: %v", page.File, err)
		}
		if im.Bounds().Dx() != page.Width || im.Bounds().Dy() != page.Height {
			return nil, fmt.Errorf("page %q is %dx%d but layout declares %dx%d", page.File, im.Bounds().Dx(), im.Bounds().Dy(), page.Width, page.Height)
		}
		pageImages[i] = asNRGBA(im)
	}
	if len(files) != len(meta.Pages)+1 {
		return nil, errors.New("package contains files not declared by the layout")
	}

	g := meta.Grid
	atlasW, atlasH := g.Size[0]*6, g.Size[1]*g.Size[2]
	if meta.TextureWidth != atlasW || meta.TextureHeight != atlasH {
		return nil, errors.New("layout texture dimensions disagree with the grid")
	}
	atlasBounds := image.Rect(0, 0, atlasW, atlasH)
	covered := make([]bool, atlasW*atlasH)
	atlas := image.NewNRGBA(atlasBounds)

	for ti, tile := range meta.Tiles {
		if tile.Face < 0 || tile.Face >= 6 {
			return nil, fmt.Errorf("tile %d references invalid face %d", ti, tile.Face)
		}
		if tile.Depth < 0 || tile.Depth >= g.Size[2] {
			return nil, fmt.Errorf("tile %d references invalid depth layer %d", ti, tile.Depth)
		}
		if tile.Page < 0 || tile.Page >= len(meta.Pages) {
			return nil, fmt.Errorf("tile %d references missing page %d", ti, tile.Page)
		}
		page := meta.Pages[tile.Page]
		pageBounds := image.Rect(0, 0, page.Width, page.Height)
		if err := validateRegion(tile.Target, pageBounds, "target"); err != nil {
			return nil, fmt.Errorf("tile %d: %v", ti, err)
		}
		want := faceLayerRect(g, tile.Face, tile.Depth)
		if err := validateRegion(tile.Source, want, "source"); err != nil {
			return nil, fmt.Errorf("tile %d: %v", ti, err)
		}
		src, tgt := tile.Source, tile.Target
		if src.W != tgt.W || src.H != tgt.H {
			return nil, fmt.Errorf("tile %d source and target sizes disagree", ti)
		}
		im := pageImages[tile.Page]
		for y := 0; y < src.H; y++ {
			for x := 0; x < src.W; x++ {
				px := src.X + x
				py := src.Y + y
				idx := py*atlasW + px
				c := im.NRGBAAt(tgt.X+x, tgt.Y+y)
				if covered[idx] {
					if atlas.NRGBAAt(px, py) != c {
						return nil, fmt.Errorf("tile %d conflicts with an earlier tile at logical pixel (%d,%d)", ti, px, py)
					}
					continue
				}
				covered[idx] = true
				atlas.SetNRGBA(px, py, c)
			}
		}
	}

	// Every logical pixel of every face and depth layer must have a source,
	// including transparent (invalid probe) pixels.
	for idx, ok := range covered {
		if !ok {
			return nil, fmt.Errorf("logical coverage hole at pixel (%d,%d)", idx%atlasW, idx/atlasW)
		}
	}
	return &Frame{
		Schema:   meta.Schema,
		Grid:     g,
		Axes:     meta.Axes,
		Exposure: meta.Exposure,
		pages:    meta.Pages,
		tiles:    meta.Tiles,
		logical:  atlas,
	}, nil
}
