package domain

import "image"

func texture(g Grid, probes []Probe, exposure float64) ([]byte, error) {
	return encodePNG(logicalAtlas(g, probes, exposure))
}

// faceLayerRect is the region of one (face, depth layer z) inside the logical atlas.
func faceLayerRect(g Grid, face, z int) image.Rectangle {
	return image.Rect(face*g.Size[0], z*g.Size[1], (face+1)*g.Size[0], (z+1)*g.Size[1])
}
