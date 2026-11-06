package export

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Handwriting in a note is a fenced block of vector strokes (frontend/src/editor/ink.ts):
//
//	```ink
//	{"v":2,"h":260,"s":[{"t":"pen","c":"ink","w":2,"p":[[12.53,30,0],[14.01,33.5,0.612]]}]}
//	```
//
// The page is inkW units wide whatever its size on paper. HTML and PDF draw the strokes; Word has
// no place for them yet, and Markdown keeps the block as it was written, so nothing is lost.

const inkW = 1000.0

type Ink struct {
	Raw     string // the block as stored, for Markdown
	H       float64
	Strokes []InkStroke
}

type InkStroke struct {
	Tool   string // "pen" or "hl" (highlighter)
	Colour string
	Width  int
	Points [][3]float64 // x, y, pressure 0 to 1 (0 when the device gave none)
}

var inkHex = map[string]string{
	"ink": "#2a2622", "yellow": "#d9b43c", "green": "#5f9e55", "blue": "#4f7fbf", "pink": "#c4607f", "purple": "#8b6bbd",
}

func (k InkStroke) size() float64 {
	if k.Tool == "hl" {
		return []float64{14, 24, 40}[k.Width-1]
	}
	return []float64{2.5, 5, 10}[k.Width-1]
}

func (k InkStroke) opacity() float64 {
	if k.Tool == "hl" {
		return 0.38
	}
	return 1
}

// parseInk reads one Markdown block as ink; nil when it is not a well-formed ink block.
func parseInk(lines []string) *Ink {
	if len(lines) != 3 || lines[0] != "```ink" || lines[2] != "```" {
		return nil
	}
	var j struct {
		V int `json:"v"`
		H float64
		S []struct {
			T string       `json:"t"`
			C string       `json:"c"`
			W int          `json:"w"`
			P [][3]float64 `json:"p"`
		} `json:"s"`
	}
	j.H = -1
	var raw struct {
		H *float64 `json:"h"`
	}
	if json.Unmarshal([]byte(lines[1]), &j) != nil || json.Unmarshal([]byte(lines[1]), &raw) != nil || raw.H == nil || (j.V != 1 && j.V != 2) {
		return nil
	}
	ink := &Ink{Raw: strings.Join(lines, "\n"), H: math.Min(8000, math.Max(200, *raw.H))}
	for _, s := range j.S {
		if (s.T != "pen" && s.T != "hl") || inkHex[s.C] == "" || s.W < 1 || s.W > 3 {
			return nil
		}
		if j.V == 1 { // the first format wrote pressure 0 to 100
			for i := range s.P {
				s.P[i][2] /= 100
			}
		}
		ink.Strokes = append(ink.Strokes, InkStroke{Tool: s.T, Colour: s.C, Width: s.W, Points: s.P})
	}
	return ink
}

// svg draws the page as an inline SVG, in the colours of the light theme.
func (n *Ink) svg() string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" width="100%%" role="img" aria-label="Handwriting">`, inkW, n.H)
	for _, k := range n.Strokes {
		col := inkHex[k.Colour]
		op := ""
		if k.opacity() < 1 {
			op = fmt.Sprintf(` fill-opacity="%g"`, k.opacity())
		}
		if d := k.path(); d != "" {
			fmt.Fprintf(&b, `<path d="%s" fill="%s"%s/>`, d, col, op)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// rgb is the stroke's colour for a PDF, a highlighter blended with the white of the paper.
func (k InkStroke) rgb() [3]float64 {
	h := inkHex[k.Colour]
	var c [3]float64
	for i := range c {
		var v int
		fmt.Sscanf(h[1+2*i:3+2*i], "%02x", &v)
		c[i] = float64(v) / 255
		if op := k.opacity(); op < 1 {
			c[i] = c[i]*op + (1 - op)
		}
	}
	return c
}
