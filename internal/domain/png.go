package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// encodeAtlas paints six lobes per grid probe into the canonical strip atlas.
// valid[i]==false leaves the probe's six pixels transparent. The exposure
// mapping is the original bake mapping and is shared by normal and preview
// textures.
func encodeAtlas(g Grid, valid []bool, lobes []Lobes, exposure float64) ([]byte, error) {
	im := image.NewNRGBA(image.Rect(0, 0, g.Size[0]*6, g.Size[1]*g.Size[2]))
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				if !valid[idx] {
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

func texture(g Grid, probes []Probe, exposure float64) ([]byte, error) {
	valid := make([]bool, len(probes))
	lobes := make([]Lobes, len(probes))
	for i, p := range probes {
		valid[i] = p.Valid
		lobes[i] = p.Irradiance
	}
	return encodeAtlas(g, valid, lobes, exposure)
}
