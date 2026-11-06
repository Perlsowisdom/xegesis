package export

import (
	"fmt"
	"math"
	"strings"
)

// The stroke engine, the same algorithm as frontend/src/editor/inkengine.ts so HTML and PDF draw the
// shape the screen draws: a centripetal Catmull-Rom spline through the pen points, as cubic Béziers,
// offset on both sides by the width at each point, with round ends. Keep the two in step.

const (
	inkStep     = 2.5 // longest chord before a spline segment is split further
	inkCapSteps = 8
)

type inkSample struct{ x, y, r float64 }

func clamp01(v float64) float64 { return math.Min(1, math.Max(0, v)) }

func pressureWidth(p float64) float64 { return 0.4 + 0.85*math.Pow(clamp01(p), 0.9) }

func speedWidth(step float64) float64 { return 1.2 - 0.7*clamp01(step/24) }

// inkRadii is the half-width at every point, from pressure when the device gave it, else from speed.
func inkRadii(pts [][3]float64, size float64, flat bool) []float64 {
	n := len(pts)
	real := false
	for _, q := range pts {
		if q[2] > 0 {
			real = true
		}
	}
	f := make([]float64, n)
	for i := range f {
		switch {
		case flat:
			f[i] = 1
		case real:
			p := pts[i][2]
			if p <= 0 {
				p = 0.5
			}
			f[i] = pressureWidth(p)
		default:
			a, b := pts[max(0, i-1)], pts[min(n-1, i+1)]
			d := 2.0
			if i == 0 || i == n-1 {
				d = 1
			}
			f[i] = speedWidth(math.Hypot(b[0]-a[0], b[1]-a[1]) / d)
		}
	}
	if !flat {
		passes := 4
		if real {
			passes = 1
		}
		for ; passes > 0; passes-- {
			g := append([]float64(nil), f...)
			for i := 1; i < n-1; i++ {
				g[i] = 0.25*f[i-1] + 0.5*f[i] + 0.25*f[i+1]
			}
			f = g
		}
	}
	for i := range f {
		f[i] = f[i] * size / 2
	}
	return f
}

// inkBezier is the control points of the Catmull-Rom segment p1 to p2 between neighbours p0 and p3.
func inkBezier(p0, p1, p2, p3 [2]float64) (c1, c2 [2]float64) {
	d1 := math.Sqrt(math.Hypot(p1[0]-p0[0], p1[1]-p0[1]))
	d2 := math.Sqrt(math.Hypot(p2[0]-p1[0], p2[1]-p1[1]))
	d3 := math.Sqrt(math.Hypot(p3[0]-p2[0], p3[1]-p2[1]))
	ctl := func(a, b, c, da, db float64) float64 {
		return (da*da*c - db*db*a + (2*da*da+3*da*db+db*db)*b) / (3 * da * (da + db))
	}
	for k := 0; k < 2; k++ {
		if d1 > 1e-9 && d2 > 1e-9 {
			c1[k] = ctl(p0[k], p1[k], p2[k], d1, d2)
		} else {
			c1[k] = p1[k] + (p2[k]-p1[k])/3
		}
		if d3 > 1e-9 && d2 > 1e-9 {
			c2[k] = ctl(p3[k], p2[k], p1[k], d3, d2)
		} else {
			c2[k] = p2[k] - (p2[k]-p1[k])/3
		}
	}
	return
}

func inkCentreline(pts [][3]float64, r []float64) []inkSample {
	n := len(pts)
	out := []inkSample{{pts[0][0], pts[0][1], r[0]}}
	xy := func(i int) [2]float64 { return [2]float64{pts[i][0], pts[i][1]} }
	for i := 0; i < n-1; i++ {
		p1, p2 := xy(i), xy(i+1)
		p0 := [2]float64{2*p1[0] - p2[0], 2*p1[1] - p2[1]}
		if i > 0 {
			p0 = xy(i - 1)
		}
		p3 := [2]float64{2*p2[0] - p1[0], 2*p2[1] - p1[1]}
		if i+2 < n {
			p3 = xy(i + 2)
		}
		c1, c2 := inkBezier(p0, p1, p2, p3)
		steps := int(math.Min(32, math.Max(1, math.Ceil(math.Hypot(p2[0]-p1[0], p2[1]-p1[1])/inkStep))))
		for s := 1; s <= steps; s++ {
			t := float64(s) / float64(steps)
			u := 1 - t
			a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
			out = append(out, inkSample{
				x: a*p1[0] + b*c1[0] + c*c2[0] + d*p2[0],
				y: a*p1[1] + b*c1[1] + c*c2[1] + d*p2[1],
				r: r[i] + (r[i+1]-r[i])*t,
			})
		}
	}
	return out
}

func inkArc(x, y, r, from, sweep float64, out [][2]float64) [][2]float64 {
	for k := 1; k < inkCapSteps; k++ {
		a := from + sweep*float64(k)/inkCapSteps
		out = append(out, [2]float64{x + r*math.Cos(a), y + r*math.Sin(a)})
	}
	return out
}

func inkPolygon(s []inkSample) [][2]float64 {
	var q []inkSample
	for _, p := range s {
		if l := len(q) - 1; l < 0 || math.Hypot(p.x-q[l].x, p.y-q[l].y) > 1e-6 {
			q = append(q, p)
		}
	}
	if len(q) < 2 {
		p := s[0]
		if len(q) == 1 {
			p = q[0]
		}
		return inkArc(p.x, p.y, p.r, 0, 2*math.Pi, [][2]float64{{p.x + p.r, p.y}})
	}
	n := len(q)
	left := make([][2]float64, n)
	right := make([][2]float64, n)
	ang := make([]float64, n)
	for i := range q {
		a, b := q[max(0, i-1)], q[min(n-1, i+1)]
		th := math.Atan2(b.y-a.y, b.x-a.x)
		ang[i] = th
		nx, ny := -math.Sin(th), math.Cos(th)
		left[i] = [2]float64{q[i].x + nx*q[i].r, q[i].y + ny*q[i].r}
		right[i] = [2]float64{q[i].x - nx*q[i].r, q[i].y - ny*q[i].r}
	}
	out := append([][2]float64(nil), left...)
	e := q[n-1]
	out = inkArc(e.x, e.y, e.r, ang[n-1]+math.Pi/2, -math.Pi, out)
	for i := n - 1; i >= 0; i-- {
		out = append(out, right[i])
	}
	return inkArc(q[0].x, q[0].y, q[0].r, ang[0]-math.Pi/2, -math.Pi, out)
}

// outline is the closed polygon of the stroke in page units.
func (k InkStroke) outline() [][2]float64 {
	if len(k.Points) == 0 {
		return nil
	}
	pts := k.Points
	if len(pts) == 1 {
		pts = [][3]float64{pts[0], pts[0]}
	}
	return inkPolygon(inkCentreline(pts, inkRadii(pts, k.size(), k.Tool == "hl")))
}

// path is the outline as SVG path data.
func (k InkStroke) path() string {
	o := k.outline()
	if len(o) < 3 {
		return ""
	}
	var b strings.Builder
	f := func(v float64) float64 { return math.Round(v*100) / 100 }
	fmt.Fprintf(&b, "M%g,%g", f(o[0][0]), f(o[0][1]))
	for _, p := range o[1:] {
		fmt.Fprintf(&b, "L%g,%g", f(p[0]), f(p[1]))
	}
	b.WriteString("Z")
	return b.String()
}
