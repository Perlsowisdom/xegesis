// Command genassets draws Xegesis's placeholder logo and writes every icon the Windows build and the
// Microsoft Store package need: the app icon (.ico) and the MSIX logos. Replace the drawing in
// draw() when there is a designed logo; nothing else changes.
//
//	go run ./genassets <repo root>
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var (
	navy  = color.NRGBA{0x1f, 0x3a, 0x5f, 0xff}
	cream = color.NRGBA{0xf7, 0xf3, 0xe8, 0xff}
	gold  = color.NRGBA{0xd9, 0xa4, 0x41, 0xff}
)

// segDist is the distance from (px,py) to the segment a-b.
func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// sample gives the colour at u,v in 0..1 (alpha 0 outside the rounded square when round is set).
func sample(u, v float64, full bool) color.NRGBA {
	if !full { // rounded square
		r := 0.18
		cx, cy := math.Abs(u-0.5)-(0.5-r), math.Abs(v-0.5)-(0.5-r)
		if math.Hypot(math.Max(cx, 0), math.Max(cy, 0)) > r {
			return color.NRGBA{}
		}
	}
	c := navy
	w := 0.075
	if segDist(u, v, 0.30, 0.27, 0.70, 0.67) < w || segDist(u, v, 0.70, 0.27, 0.30, 0.67) < w {
		c = cream
	}
	if math.Abs(u-0.5) < 0.22 && math.Abs(v-0.79) < 0.025 {
		c = gold
	}
	return c
}

// logo draws w by h: the mark, centred and sized to the shorter side. Wide logos get the navy
// background across the whole image.
func logo(w, h int, full bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	side := float64(min(w, h))
	const ss = 4
	for y := range h {
		for x := range w {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					u := (px-float64(w)/2)/side + 0.5
					v := (py-float64(h)/2)/side + 0.5
					var c color.NRGBA
					if u < 0 || u > 1 || v < 0 || v > 1 {
						if full {
							c = navy
						}
					} else {
						c = sample(u, v, full)
					}
					fa := float64(c.A) / 255
					r += float64(c.R) * fa
					g += float64(c.G) * fa
					b += float64(c.B) * fa
					a += fa
				}
			}
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (ss * ss))})
			}
		}
	}
	return img
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

func write(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
}

// ico packs PNG images into one .ico file.
func ico(sizes []int) []byte {
	var head, body bytes.Buffer
	binary.Write(&head, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for _, s := range sizes {
		data := pngBytes(logo(s, s, false))
		d := byte(s)
		if s >= 256 {
			d = 0
		}
		head.Write([]byte{d, d, 0, 0, 1, 0, 32, 0})
		binary.Write(&head, binary.LittleEndian, [2]uint32{uint32(len(data)), uint32(off + body.Len())})
		body.Write(data)
	}
	return append(head.Bytes(), body.Bytes()...)
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: genassets <repo root>")
	}
	root := os.Args[1]
	write(filepath.Join(root, "build", "windows", "icon.ico"), ico([]int{16, 24, 32, 48, 64, 128, 256}))
	write(filepath.Join(root, "build", "appicon.png"), pngBytes(logo(512, 512, false)))
	assets := filepath.Join(root, "build", "msix", "Assets")
	type asset struct {
		name string
		w, h int
		wide bool
	}
	list := []asset{
		{"StoreLogo", 50, 50, false}, {"StoreLogo.scale-200", 100, 100, false},
		{"Square44x44Logo", 44, 44, false}, {"Square44x44Logo.scale-200", 88, 88, false},
		{"Square44x44Logo.targetsize-24_altform-unplated", 24, 24, false},
		{"Square44x44Logo.targetsize-48_altform-unplated", 48, 48, false},
		{"Square44x44Logo.targetsize-256_altform-unplated", 256, 256, false},
		{"SmallTile", 71, 71, false}, {"SmallTile.scale-200", 142, 142, false},
		{"Square150x150Logo", 150, 150, false}, {"Square150x150Logo.scale-200", 300, 300, false},
		{"LargeTile", 310, 310, false}, {"LargeTile.scale-200", 620, 620, false},
		{"Wide310x150Logo", 310, 150, true}, {"Wide310x150Logo.scale-200", 620, 300, true},
		{"SplashScreen", 620, 300, true}, {"SplashScreen.scale-200", 1240, 600, true},
	}
	for _, a := range list {
		write(filepath.Join(assets, a.name+".png"), pngBytes(logo(a.w, a.h, a.wide)))
	}
	// Store listing logos (Partner Center asks for these as uploads).
	write(filepath.Join(root, "docs", "store", "logo-300.png"), pngBytes(logo(300, 300, false)))
	write(filepath.Join(root, "docs", "store", "logo-1080.png"), pngBytes(logo(1080, 1080, false)))
}
