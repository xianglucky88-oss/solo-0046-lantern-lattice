package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// logicalAtlas builds the full probe strip at pixel (x+face*X, y+z*Y).
// Invalid probes stay transparent; valid probes are opaque, including black.
func logicalAtlas(g Grid, probes []Probe, exposure float64) *image.NRGBA {
	atlas := image.NewNRGBA(image.Rect(0, 0, g.Size[0]*6, g.Size[1]*g.Size[2]))
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				p := probes[g.index(x, y, z)]
				if !p.Valid {
					continue
				}
				for face, e := range p.Irradiance {
					px, py := pixel(g, x, y, z, face)
					atlas.SetNRGBA(px, py, color.NRGBA{R: quantize(e[0], exposure), G: quantize(e[1], exposure), B: quantize(e[2], exposure), A: 255})
				}
			}
		}
	}
	return atlas
}

func encodePNG(im image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
