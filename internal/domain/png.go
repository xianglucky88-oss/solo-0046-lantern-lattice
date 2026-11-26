package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func texture(g Grid, probes []Probe, exposure float64) ([]byte, error) {
	valid := make([]bool, len(probes))
	lower := make([]Lobes, len(probes))
	for i, p := range probes {
		valid[i] = p.Valid
		lower[i] = p.Irradiance
	}
	return renderAtlas(g, valid, lower, exposure)
}

// renderAtlas encodes the six direction strip for arbitrary recomputed lobes.
// Invalid grid cells stay fully transparent exactly like the baked texture.
func renderAtlas(g Grid, valid []bool, lobes []Lobes, exposure float64) ([]byte, error) {
	im := image.NewNRGBA(image.Rect(0, 0, g.Size[0]*6, g.Size[1]*g.Size[2]))
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				if idx >= len(valid) || !valid[idx] {
					continue
				}
				for i, e := range lobes[idx] {
					px, py := pixel(g, x, y, z, i)
					im.SetNRGBA(px, py, color.NRGBA{R: quantize(e[0], exposure), G: quantize(e[1], exposure), B: quantize(e[2], exposure), A: 255})
				}
			}
		}
	}
	var out bytes.Buffer
	err := png.Encode(&out, im)
	return out.Bytes(), err
}
