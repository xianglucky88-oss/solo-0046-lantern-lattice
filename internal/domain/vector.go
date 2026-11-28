package domain

import "math"

type Vec [3]float64

func (a Vec) add(b Vec) Vec       { return Vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec) sub(b Vec) Vec       { return a.add(b.scale(-1)) }
func (a Vec) scale(v float64) Vec { return Vec{a[0] * v, a[1] * v, a[2] * v} }
func (a Vec) dot(b Vec) float64   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a Vec) cross(b Vec) Vec {
	return Vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (a Vec) length() float64 { return math.Sqrt(a.dot(a)) }
func (a Vec) maxAbs() float64 {
	return math.Max(math.Abs(a[0]), math.Max(math.Abs(a[1]), math.Abs(a[2])))
}
