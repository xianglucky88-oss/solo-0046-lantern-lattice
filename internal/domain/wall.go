package domain

import "math"

// wallBasis is the orthonormal frame of a thin wall. A point p maps to local
// coordinates (U, V, D) = ((p-Center)·r, (p-Center)·u, (p-Center)·n), where D
// is the signed wall distance, positive toward the +Side grid.
//
// The zero value is inactive: it never blocks and never marks a slot solid,
// which lets a legacy request without a wall reuse the same code paths.
type wallBasis struct {
	active  bool
	center  Vec
	n, u, r Vec
	half    [2]float64
	openMin [2]float64
	openMax [2]float64
}

func newWallBasis(w Wall) (wallBasis, bool) {
	if w.Normal.length() == 0 || w.Up.length() == 0 {
		return wallBasis{}, false
	}
	n := w.Normal.scale(1 / w.Normal.length())
	up := w.Up.sub(n.scale(w.Up.dot(n)))
	if up.length() == 0 {
		return wallBasis{}, false
	}
	u := up.scale(1 / up.length())
	return wallBasis{
		active:  true,
		center:  w.Center,
		n:       n,
		u:       u,
		r:       u.cross(n),
		half:    w.Half,
		openMin: w.Opening.Min,
		openMax: w.Opening.Max,
	}, true
}

func (w wallBasis) signedDistance(p Vec) float64 { return p.sub(w.center).dot(w.n) }

// tolerance scales with the largest coordinate so that plane tests stay stable
// for scenes near the spatial limit while remaining far below the smallest
// permitted grid step (0.01).
func (w wallBasis) tolerance(points ...Vec) float64 {
	scale := math.Max(1, w.center.maxAbs())
	for _, p := range points {
		scale = math.Max(scale, p.maxAbs())
	}
	return math.Max(1e-9, 1e-12*scale)
}

func (w wallBasis) local(p Vec) (float64, float64) {
	q := p.sub(w.center)
	return q.dot(w.r), q.dot(w.u)
}

func (w wallBasis) inRectangle(u, v, eps float64) bool {
	return math.Abs(u) <= w.half[0]+eps && math.Abs(v) <= w.half[1]+eps
}

// strictOpening reports whether (u,v) lies strictly inside the rectangular
// hole. The hole edge is wall, so points within eps of an edge are excluded.
func (w wallBasis) strictOpening(u, v, eps float64) bool {
	return u > w.openMin[0]+eps && u < w.openMax[0]-eps &&
		v > w.openMin[1]+eps && v < w.openMax[1]-eps
}

// solidAt reports whether a point on the wall plane is part of the closed wall:
// inside the wall rectangle and not strictly inside the hole.
func (w wallBasis) solidAt(p Vec, eps float64) bool {
	u, v := w.local(p)
	if !w.inRectangle(u, v, eps) {
		return false
	}
	return !w.strictOpening(u, v, eps)
}

// onClosedWall reports whether p lies on the wall's solid surface.
func (w wallBasis) onClosedWall(p Vec) bool {
	if !w.active {
		return false
	}
	eps := w.tolerance(p)
	if math.Abs(w.signedDistance(p)) > eps {
		return false
	}
	return w.solidAt(p, eps)
}

// blocks reports whether the finite segment from p to q touches the closed
// (thickness-less) wall. Endpoints strictly inside the hole are passable, and a
// coplanar segment passes only where it stays strictly inside the hole.
func (w wallBasis) blocks(p, q Vec) bool {
	if !w.active {
		return false
	}
	eps := w.tolerance(p, q)
	dp := w.signedDistance(p)
	dq := w.signedDistance(q)
	onP := math.Abs(dp) <= eps
	onQ := math.Abs(dq) <= eps

	if onP && onQ {
		return w.coplanarBlocks(p, q, eps)
	}

	t, crosses := 0.0, false
	switch {
	case dp*dq < 0:
		t, crosses = dp/(dp-dq), true
	case onP:
		t, crosses = 0, true
	case onQ:
		t, crosses = 1, true
	}
	if !crosses {
		return false
	}
	x := p.add(q.sub(p).scale(t))
	return w.solidAt(x, eps)
}

// coplanarBlocks clips the segment onto the wall rectangle in local (U,V)
// space. The segment passes only when both ends of the clipped span lie
// strictly inside the hole; any contact with the solid frame blocks it.
func (w wallBasis) coplanarBlocks(p, q Vec, eps float64) bool {
	u0, v0 := w.local(p)
	u1, v1 := w.local(q)
	lo, hi := 0.0, 1.0
	if !clipAxis(u0, u1, w.half[0], &lo, &hi) || !clipAxis(v0, v1, w.half[1], &lo, &hi) {
		return false
	}
	if lo > hi {
		return false
	}
	uLo, vLo := u0+(u1-u0)*lo, v0+(v1-v0)*lo
	uHi, vHi := u0+(u1-u0)*hi, v0+(v1-v0)*hi
	return !(w.strictOpening(uLo, vLo, eps) && w.strictOpening(uHi, vHi, eps))
}

// clipAxis narrows [lo,hi] to the parameter range where the linear coordinate
// c0+(c1-c0)t stays within [-h,h]. It returns false when the segment misses
// the slab entirely.
func clipAxis(c0, c1, h float64, lo, hi *float64) bool {
	if c0 == c1 {
		return math.Abs(c0) <= h
	}
	a := (-h - c0) / (c1 - c0)
	b := (h - c0) / (c1 - c0)
	if a > b {
		a, b = b, a
	}
	*lo = math.Max(*lo, a)
	*hi = math.Min(*hi, b)
	return *lo <= *hi
}
