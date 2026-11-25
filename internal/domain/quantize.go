package domain

import "math"

func quantize(energy, exposure float64) uint8 {
	mapped := -math.Expm1(-energy * exposure)
	if mapped >= 1 {
		return 255
	}
	return uint8(math.Round(255 * mapped))
}
