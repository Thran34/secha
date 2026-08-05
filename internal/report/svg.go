package report

import (
	"fmt"
	"html"
	"strings"
)

const fontStack = "system-ui, -apple-system, 'Segoe UI', sans-serif"

type canvas struct {
	b             strings.Builder
	width, height int
}

func newCanvas(width, height int, p Palette) *canvas {
	c := &canvas{width: width, height: height}
	fmt.Fprintf(&c.b,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img">`,
		width, height, width, height)
	fmt.Fprintf(&c.b, `<rect width="%d" height="%d" fill="%s"/>`, width, height, p.Surface)
	return c
}

func (c *canvas) done() string {
	return c.b.String() + "</svg>"
}

func (c *canvas) title(text, subtitle string, p Palette) {
	c.text(24, 30, text, 15, 600, p.TextPrimary, "start")
	if subtitle != "" {
		c.text(24, 50, subtitle, 12, 400, p.TextSecondary, "start")
	}
}

func (c *canvas) text(x, y float64, s string, size, weight int, fill, anchor string) {
	fmt.Fprintf(&c.b,
		`<text x="%.1f" y="%.1f" font-family="%s" font-size="%d" font-weight="%d" fill="%s" text-anchor="%s">%s</text>`,
		x, y, fontStack, size, weight, fill, anchor, html.EscapeString(s))
}

func (c *canvas) tabularText(x, y float64, s string, size, weight int, fill, anchor string) {
	fmt.Fprintf(&c.b,
		`<text x="%.1f" y="%.1f" font-family="%s" font-size="%d" font-weight="%d" fill="%s" text-anchor="%s" style="font-variant-numeric:tabular-nums">%s</text>`,
		x, y, fontStack, size, weight, fill, anchor, html.EscapeString(s))
}

func (c *canvas) line(x1, y1, x2, y2 float64, stroke string, width float64) {
	fmt.Fprintf(&c.b,
		`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f" stroke-linecap="round"/>`,
		x1, y1, x2, y2, stroke, width)
}

func (c *canvas) hairline(x1, y1, x2, y2 float64, stroke string) {
	fmt.Fprintf(&c.b,
		`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`,
		x1, y1, x2, y2, stroke)
}

func (c *canvas) dot(x, y, r float64, fill, ring string) {
	fmt.Fprintf(&c.b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="%s" stroke-width="2"/>`,
		x, y, r, fill, ring)
}

func (c *canvas) swatch(x, y float64, fill string) {
	fmt.Fprintf(&c.b, `<rect x="%.1f" y="%.1f" width="10" height="10" rx="2" fill="%s"/>`, x, y, fill)
}

func (c *canvas) barRight(x, y, w, h, r float64, fill string) {
	if w <= 0 {
		return
	}
	if w < r {
		r = w
	}
	fmt.Fprintf(&c.b,
		`<path d="M %.1f %.1f H %.1f a %.1f %.1f 0 0 1 %.1f %.1f V %.1f a %.1f %.1f 0 0 1 -%.1f %.1f H %.1f Z" fill="%s"/>`,
		x, y, x+w-r, r, r, r, r, y+h-r, r, r, r, r, x, fill)
}
