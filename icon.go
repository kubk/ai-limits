package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"time"

	"ai-limits/limits"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// badgeText is the time to the soonest reset of any limit, as "13h" or
// "45m", when that is less than a day away; "" otherwise.
func badgeText(windows map[string][]limits.Window, now time.Time) string {
	var soonest time.Duration
	for _, ws := range windows {
		for _, w := range ws {
			if w.ResetsAt.IsZero() {
				continue
			}
			if left := w.ResetsAt.Sub(now); left > 0 && (soonest == 0 || left < soonest) {
				soonest = left
			}
		}
	}
	switch {
	case soonest <= 0 || soonest >= 24*time.Hour:
		return ""
	case soonest >= time.Hour:
		return fmt.Sprintf("%dh", int(soonest/time.Hour))
	default:
		return fmt.Sprintf("%dm", max(1, int(soonest/time.Minute)))
	}
}

// badgeFace is Helvetica Neue Bold, close to the menu bar's own text, or
// Go Bold where the system font is missing.
var badgeFace = func() font.Face {
	const size = 26 // px at 2x
	var f *opentype.Font
	if b, err := os.ReadFile("/System/Library/Fonts/HelveticaNeue.ttc"); err == nil {
		if c, err := opentype.ParseCollection(b); err == nil {
			for i := range c.NumFonts() {
				g, err := c.Font(i)
				if err != nil {
					continue
				}
				if name, _ := g.Name(nil, sfnt.NameIDFull); name == "Helvetica Neue Bold" {
					f = g
					break
				}
			}
		}
	}
	if f == nil {
		var err error
		if f, err = opentype.Parse(gobold.TTF); err != nil {
			panic(err)
		}
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return face
}()

// trayIcon draws three bars of different lengths, black on transparent
// for macOS to tint: the three meters of the popover. A badge, a pill with
// the text cut out of it, sits over the bottom right of the bars.
func trayIcon(badge string) []byte {
	const s = 36 // 18pt at 2x
	ink := image.NewUniform(color.NRGBA{A: 255})

	// The badge's size, to widen the icon when it sticks out.
	const bh, pad = 32, 7
	ink0, _ := font.BoundString(badgeFace, badge) // the text's ink
	tw := (ink0.Max.X - ink0.Min.X).Ceil()
	th := (ink0.Max.Y - ink0.Min.Y).Ceil()
	bw := max(bh, tw+2*pad)
	width := s
	if badge != "" {
		width = max(s, 22+bw)
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, s))
	for i, w := range []int{28, 20, 24} {
		y := 7 + i*9
		draw.Draw(img, image.Rect(4, y, 4+w, y+5), ink, image.Point{}, draw.Src)
	}
	if badge == "" {
		return encode(img)
	}

	box := image.Rect(width-bw, s-bh, width, s)
	// A transparent gap around the badge, so it reads apart from the bars.
	clear := image.NewUniform(color.Transparent)
	draw.DrawMask(img, box.Inset(-2), clear, image.Point{}, &pill{box.Inset(-2)}, box.Inset(-2).Min, draw.Src)
	draw.DrawMask(img, box, ink, image.Point{}, &pill{box}, box.Min, draw.Over)

	// The text, knocked out of the pill.
	text := image.NewAlpha(img.Bounds())
	d := font.Drawer{Dst: text, Src: image.Opaque, Face: badgeFace}
	// The ink centered in the pill, not the line box: digits have no
	// descenders.
	d.Dot = fixed.P(box.Min.X+(bw-tw)/2, box.Min.Y+(bh-th)/2).Sub(ink0.Min)
	d.DrawString(badge)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			c.A = uint8(uint32(c.A) * (255 - uint32(text.AlphaAt(x, y).A)) / 255)
			img.SetNRGBA(x, y, c)
		}
	}
	return encode(img)
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// pill is a mask: r with fully rounded ends, antialiased.
type pill struct{ r image.Rectangle }

func (p *pill) ColorModel() color.Model { return color.AlphaModel }
func (p *pill) Bounds() image.Rectangle { return p.r }
func (p *pill) At(x, y int) color.Color {
	r := float64(p.r.Dy()) / 2
	// Distance from the pixel's center to the pill's spine.
	px, py := float64(x)+0.5, float64(y)+0.5
	cx := min(max(px, float64(p.r.Min.X)+r), float64(p.r.Max.X)-r)
	cy := float64(p.r.Min.Y) + r
	dx, dy := px-cx, py-cy
	dist := math.Sqrt(dx*dx+dy*dy) - r
	a := min(1, max(0, 0.5-dist))
	return color.Alpha{uint8(a * 255)}
}
