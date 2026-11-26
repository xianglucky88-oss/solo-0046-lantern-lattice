package domain

import (
	"fmt"
	"image"
)

// Region is an axis-aligned, half-open rectangle in pixel coordinates.
type Region struct {
	X int `json:"X"`
	Y int `json:"Y"`
	W int `json:"W"`
	H int `json:"H"`
}

func (r Region) rect() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

func regionFrom(rect image.Rectangle) Region {
	return Region{X: rect.Min.X, Y: rect.Min.Y, W: rect.Dx(), H: rect.Dy()}
}

// TileRef declares one rectangular fragment. Source locates the fragment inside
// the logical probe strip (face, depth layer and in-plane rectangle); Target
// locates the same pixels inside one package page.
type TileRef struct {
	Face   int    `json:"Face"`
	Depth  int    `json:"Depth"`
	Source Region `json:"Source"`
	Page   int    `json:"Page"`
	Target Region `json:"Target"`
}

// PageSpec declares one uploadable page texture.
type PageSpec struct {
	File   string `json:"File"`
	Width  int    `json:"Width"`
	Height int    `json:"Height"`
}

const maxTextureLimit = 1_000_000

func pageFileName(index int) string { return fmt.Sprintf("pages/%04d.png", index) }

// splitAxis partitions [lo, hi) into intervals no longer than max. Every
// interval keeps at least two pixels; when an even cut would leave a trailing
// single pixel, the last two intervals overlap instead of leaving a hole.
func splitAxis(lo, hi, max int) [][2]int {
	if hi-lo <= max {
		return [][2]int{{lo, hi}}
	}
	var out [][2]int
	start := lo
	for {
		end := start + max
		if end >= hi {
			out = append(out, [2]int{start, hi})
			return out
		}
		if hi-end < 2 {
			out = append(out, [2]int{start, end})
			start = hi - max
			continue
		}
		out = append(out, [2]int{start, end})
		start = end
	}
}

// splitRect breaks rect into fragments that each fit maxW x maxH. Fragments
// may overlap on fragment boundaries but never extend past rect.
func splitRect(rect image.Rectangle, maxW, maxH int) []image.Rectangle {
	cutsX := splitAxis(rect.Min.X, rect.Max.X, maxW)
	cutsY := splitAxis(rect.Min.Y, rect.Max.Y, maxH)
	out := make([]image.Rectangle, 0, len(cutsX)*len(cutsY))
	for _, xb := range cutsX {
		for _, yb := range cutsY {
			out = append(out, image.Rect(xb[0], yb[0], xb[1], yb[1]))
		}
	}
	return out
}

type shelfLayout struct {
	y, height, usedWidth int
}

type pageLayout struct {
	shelves []*shelfLayout
	tiles   []TileRef
}

type packer struct {
	maxW, maxH int
	pages      []*pageLayout
	page       *pageLayout
	shelf      *shelfLayout
}

func (pk *packer) newPage() {
	pk.page = &pageLayout{}
	pk.pages = append(pk.pages, pk.page)
	pk.shelf = nil
}

// place stores one fragment on the current page, opening a new shelf or page
// whenever the declared texture limits would be exceeded.
func (pk *packer) place(face, depth int, src image.Rectangle) {
	w, h := src.Dx(), src.Dy()
	if pk.page == nil {
		pk.newPage()
	}
	// A fragment taller than the current shelf needs a fresh shelf; when the
	// current page has no vertical room left, start a new page instead.
	if pk.shelf != nil {
		nextY := pk.shelf.y + pk.shelf.height
		needNewShelf := pk.shelf.usedWidth+w > pk.maxW || h > pk.shelf.height
		if needNewShelf && nextY+h > pk.maxH {
			pk.newPage()
		} else if needNewShelf {
			pk.shelf = &shelfLayout{y: nextY, height: h}
			pk.page.shelves = append(pk.page.shelves, pk.shelf)
		}
	}
	if pk.shelf == nil {
		pk.shelf = &shelfLayout{y: 0, height: h}
		pk.page.shelves = append(pk.page.shelves, pk.shelf)
	}
	x := pk.shelf.usedWidth
	pk.shelf.usedWidth += w
	pk.page.tiles = append(pk.page.tiles, TileRef{
		Face:   face,
		Depth:  depth,
		Source: regionFrom(src),
		Target: Region{X: x, Y: pk.shelf.y, W: w, H: h},
	})
}

type renderedPage struct {
	spec  PageSpec
	image *image.NRGBA
}

func copyRect(dst *image.NRGBA, target image.Rectangle, src *image.NRGBA, source image.Rectangle) {
	for y := 0; y < source.Dy(); y++ {
		for x := 0; x < source.Dx(); x++ {
			dst.SetNRGBA(target.Min.X+x, target.Min.Y+y, src.NRGBAAt(source.Min.X+x, source.Min.Y+y))
		}
	}
}

// buildTiled splits the logical atlas into per-face/layer fragments, packs the
// fragments into pages within maxW x maxH and renders the page images.
func buildTiled(atlas *image.NRGBA, g Grid, maxW, maxH int) ([]renderedPage, []TileRef) {
	pk := &packer{maxW: maxW, maxH: maxH}
	for face := range 6 {
		for z := range g.Size[2] {
			for _, part := range splitRect(faceLayerRect(g, face, z), maxW, maxH) {
				pk.place(face, z, part)
			}
		}
	}
	pages := make([]renderedPage, 0, len(pk.pages))
	refs := make([]TileRef, 0)
	for i, pg := range pk.pages {
		width, height := 0, 0
		for _, sh := range pg.shelves {
			width = max(width, sh.usedWidth)
			height = max(height, sh.y+sh.height)
		}
		im := image.NewNRGBA(image.Rect(0, 0, width, height))
		for j := range pg.tiles {
			r := pg.tiles[j]
			r.Page = i
			copyRect(im, r.Target.rect(), atlas, r.Source.rect())
			pg.tiles[j] = r
			refs = append(refs, r)
		}
		pages = append(pages, renderedPage{
			spec:  PageSpec{File: pageFileName(i), Width: width, Height: height},
			image: im,
		})
	}
	return pages, refs
}
