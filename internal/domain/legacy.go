package domain

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
)

// RecoverLegacy fills a basis from a quantized probes.png when an artifact
// predates the linear basis. The recovered energies are approximate
// inversions of the exposure curve; exact no-accumulation relighting requires
// the embedded basis produced by current builds.
func (a *Artifact) RecoverLegacy() {
	if a.HasBasis() || len(a.PNG) == 0 {
		return
	}
	im, err := png.Decode(bytes.NewReader(a.PNG))
	if err != nil {
		return
	}
	g := a.Grid
	bounds := im.Bounds()
	recs := make([]BasisProbe, 0, g.count())
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				idx := g.index(x, y, z)
				rec := BasisProbe{
					Index: idx, X: x, Y: y, Z: z,
					Position: g.position(x, y, z),
				}
				valid := false
				var light Lobes
				for face := range 6 {
					px, py := pixel(g, x, y, z, face)
					if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
						continue
					}
					c := color.NRGBAModel.Convert(im.At(px, py)).(color.NRGBA)
					if c.A == 0 {
						continue
					}
					valid = true
					light[face] = Vec{
						inverseQuantize(float64(c.R)/255.0, a.Exposure),
						inverseQuantize(float64(c.G)/255.0, a.Exposure),
						inverseQuantize(float64(c.B)/255.0, a.Exposure),
					}
				}
				rec.Valid = valid
				rec.Light = light
				rec.Lights = light
				rec.Lamp = light
				rec.Lamps = light
				rec.Direct = light
				rec.Irradiance = light
				rec.Total = light
				rec.Original = light
				recs = append(recs, rec)
			}
		}
	}
	a.Probes = recs
	a.ProbeCount = len(recs)
	a.legacy = false // recovered; rendering now has valid cells
}

func inverseQuantize(normalised, exposure float64) float64 {
	if exposure <= 0 {
		exposure = 1
	}
	m := normalised
	if m >= 0.999999 {
		m = 0.999
	}
	if m <= 0 {
		return 0
	}
	return -math.Log(1-m) / exposure
}

// RelitZip returns a product zip carrying the relit probes.png while keeping
// the original layout.json bytes.
func RelitZip(archive, relitPNG []byte) ([]byte, error) {
	layout, _, err := ReadArtifactFiles(archive)
	if err != nil {
		return nil, err
	}
	return pack(layout, relitPNG)
}
