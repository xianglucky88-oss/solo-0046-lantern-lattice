package domain

func attenuation(distance, radius, power float64) float64 {
	if distance >= radius {
		return 0
	}
	t := 1 - distance/radius
	return power * t * t / (1 + distance*distance)
}
