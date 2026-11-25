package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func texture(g Grid, probes []Probe, exposure float64) ([]byte, error) {
	im := image.NewNRGBA(image.Rect(0, 0, g.Size[0]*6, g.Size[1]*g.Size[2]))
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				p := probes[g.index(x, y, z)]
				if !p.Valid {
					continue
				}
				for i, e := range p.Irradiance {
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
