package domain

import (
	"image"
	"testing"
)

func TestTiledAcrossGridAndLimits(t *testing.T) {
	sizes := [][3]int{
		{2, 2, 2}, {3, 2, 2}, {2, 3, 2}, {2, 2, 3},
		{4, 5, 3}, {7, 3, 2}, {16, 16, 16}, {3, 16, 2},
	}
	limits := [][2]int{{2, 2}, {2, 3}, {3, 2}, {5, 4}, {7, 2}, {16, 16}, {32, 9}}
	for _, size := range sizes {
		g := Grid{Step: 1, Size: size}
		atlas := image.NewNRGBA(image.Rect(0, 0, size[0]*6, size[1]*size[2]))
		legacyFrame := &Frame{Grid: g, logical: atlas}
		for _, limit := range limits {
			maxW, maxH := limit[0], limit[1]
			pages, tiles := buildTiled(atlas, g, maxW, maxH)
			cover := map[[2]int]bool{}
			pageByIndex := map[int]renderedPage{}
			for i, pg := range pages {
				pageByIndex[i] = pg
				if pg.spec.Width > maxW || pg.spec.Height > maxH {
					t.Fatalf("size=%v limit=%v page %d exceeds limits: %+v", size, limit, i, pg.spec)
				}
				if pg.spec.Width < 2 || pg.spec.Height < 2 {
					t.Fatalf("size=%v limit=%v page %d below 2x2", size, limit, i)
				}
			}
			for _, tile := range tiles {
				if tile.Source.W != tile.Target.W || tile.Source.H != tile.Target.H {
					t.Fatalf("size mismatch: %+v", tile)
				}
				want := faceLayerRect(g, tile.Face, tile.Depth)
				if !tile.Source.rect().In(want) {
					t.Fatalf("source outside face/layer: %+v", tile)
				}
				page := pageByIndex[tile.Page].spec
				if !tile.Target.rect().In(image.Rect(0, 0, page.Width, page.Height)) {
					t.Fatalf("target outside page: %+v page=%+v", tile, page)
				}
				for y := 0; y < tile.Source.H; y++ {
					for x := 0; x < tile.Source.W; x++ {
						cover[[2]int{tile.Source.X + x, tile.Source.Y + y}] = true
					}
				}
			}
			wantPixels := size[0] * 6 * size[1] * size[2]
			if len(cover) != wantPixels {
				t.Fatalf("size=%v limit=%v covered %d of %d logical pixels", size, limit, len(cover), wantPixels)
			}
			// Every rendered page pixel actually used by a tile must hold the
			// same encoded value as the corresponding atlas texel (all zero in
			// this synthetic atlas).
			for _, tile := range tiles {
				im := pageByIndex[tile.Page].image
				for y := 0; y < tile.Source.H; y++ {
					for x := 0; x < tile.Source.W; x++ {
						if got := im.NRGBAAt(tile.Target.X+x, tile.Target.Y+y); got != atlas.NRGBAAt(tile.Source.X+x, tile.Source.Y+y) {
							t.Fatalf("page copy mismatch on tile %+v", tile)
						}
					}
				}
			}
			_ = legacyFrame
		}
	}
}
