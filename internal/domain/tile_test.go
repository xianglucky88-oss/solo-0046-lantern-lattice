package domain

import (
	"image"
	"testing"
)

func TestSplitAxisKeepsMinTwoPixels(t *testing.T) {
	cases := []struct {
		lo, hi, max int
	}{
		{0, 3, 2},
		{0, 5, 2},
		{0, 7, 3},
		{10, 13, 2},
	}
	for _, tc := range cases {
		cuts := splitAxis(tc.lo, tc.hi, tc.max)
		cover := map[int]bool{}
		prevEnd := tc.lo
		for _, c := range cuts {
			w := c[1] - c[0]
			if w < 2 || w > tc.max {
				t.Fatalf("cut %v for %+v violates 2..%d", c, tc, tc.max)
			}
			if c[0] != prevEnd && c[0] >= prevEnd {
				// overlapping cut allowed, but it must move forward
				if c[0] <= cuts[0][0] && len(cover) > 0 {
					t.Fatalf("non-monotonic cut %v", cuts)
				}
			}
			prevEnd = c[1]
			for x := c[0]; x < c[1]; x++ {
				cover[x] = true
			}
		}
		for x := tc.lo; x < tc.hi; x++ {
			if !cover[x] {
				t.Fatalf("axis %+v leaves pixel %d uncovered, cuts=%v", tc, x, cuts)
			}
		}
		if cuts[len(cuts)-1][1] != tc.hi || cuts[0][0] != tc.lo {
			t.Fatalf("cuts %v do not span the axis", cuts)
		}
	}
}

func TestSplitRectFitsAndCovers(t *testing.T) {
	rect := image.Rect(0, 0, 3, 2)
	parts := splitRect(rect, 2, 2)
	cover := map[[2]int]bool{}
	for _, p := range parts {
		if p.Dx() > 2 || p.Dy() > 2 || p.Dx() < 1 || p.Dy() < 1 {
			t.Fatalf("fragment %v outside 2x2 limit", p)
		}
		if !p.In(rect) {
			t.Fatalf("fragment %v escapes source %v", p, rect)
		}
		for y := p.Min.Y; y < p.Max.Y; y++ {
			for x := p.Min.X; x < p.Max.X; x++ {
				cover[[2]int{x, y}] = true
			}
		}
	}
	if len(cover) != 6 {
		t.Fatalf("coverage %v misses pixels", cover)
	}
}

func TestBuildTiledHonorsLimits(t *testing.T) {
	g := Grid{Step: 1, Size: [3]int{3, 2, 2}}
	atlas := image.NewNRGBA(image.Rect(0, 0, 18, 4))
	maxW, maxH := 2, 2
	pages, tiles := buildTiled(atlas, g, maxW, maxH)
	if len(pages) == 0 || len(tiles) == 0 {
		t.Fatal("no pages or tiles produced")
	}
	for i, pg := range pages {
		if pg.spec.Width > maxW || pg.spec.Height > maxH {
			t.Fatalf("page %d is %dx%d, exceeds limit %dx%d", i, pg.spec.Width, pg.spec.Height, maxW, maxH)
		}
		if pg.spec.Width < 2 || pg.spec.Height < 2 {
			t.Fatalf("page %d is below 2x2", i)
		}
		if pg.spec.File != pageFileName(i) {
			t.Fatalf("page file name %q", pg.spec.File)
		}
		if pg.image.Bounds().Dx() != pg.spec.Width || pg.image.Bounds().Dy() != pg.spec.Height {
			t.Fatalf("page image size disagrees with spec")
		}
	}
	for _, tile := range tiles {
		if tile.Face < 0 || tile.Face >= 6 || tile.Depth < 0 || tile.Depth >= g.Size[2] {
			t.Fatalf("tile references invalid face/layer: %+v", tile)
		}
		want := faceLayerRect(g, tile.Face, tile.Depth)
		if !tile.Source.rect().In(want) {
			t.Fatalf("source %v outside face/layer rect %v", tile.Source, want)
		}
		page := pages[tile.Page].spec
		if !tile.Target.rect().In(image.Rect(0, 0, page.Width, page.Height)) {
			t.Fatalf("target %v outside page %+v", tile.Target, page)
		}
		if tile.Source.W != tile.Target.W || tile.Source.H != tile.Target.H {
			t.Fatalf("source/target size mismatch: %+v", tile)
		}
	}
}
