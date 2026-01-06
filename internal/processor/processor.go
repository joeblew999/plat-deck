// Package processor provides decksh → SVG conversion for WASM
// Based on github.com/ajstarks/deck/cmd/svgdeck
package processor

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/ajstarks/deck"
	"github.com/ajstarks/decksh"
	svg "github.com/ajstarks/svgo/float"
)

const (
	linespacing  = 1.4
	listspacing  = 2.0
	defaultColor = "rgb(127,127,127)"
	strokefmt    = "stroke-width:%.2fpx;stroke:%s;stroke-opacity:%.2f"
	fillfmt      = "fill:%s;fill-opacity:%.2f"
)

// Config holds rendering configuration
type Config struct {
	Width  int
	Height int
	// Font settings (for SVG, these are CSS font-family values)
	SansFont  string
	SerifFont string
	MonoFont  string
}

// fontmap maps generic font names to specific implementation names
var fontmap = map[string]string{}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Width:     1920,
		Height:    1080,
		SansFont:  "Helvetica, Arial, sans-serif",
		SerifFont: "Georgia, Times, serif",
		MonoFont:  "Monaco, Consolas, monospace",
	}
}

// Result holds the output of processing
type Result struct {
	Slides     [][]byte // Each slide as SVG
	SlideCount int
	Title      string
}

// ProcessDeckSH takes decksh source and returns SVG slides
func ProcessDeckSH(input []byte, cfg Config) (*Result, error) {
	// Initialize font map
	fontmap["sans"] = cfg.SansFont
	fontmap["serif"] = cfg.SerifFont
	fontmap["mono"] = cfg.MonoFont

	// Step 1: decksh → deck XML
	var deckXML bytes.Buffer
	if err := decksh.Process(&deckXML, bytes.NewReader(input)); err != nil {
		return nil, fmt.Errorf("decksh processing failed: %w", err)
	}

	// Step 2: Parse deck XML
	d, err := parseDeck(deckXML.Bytes(), cfg.Width, cfg.Height)
	if err != nil {
		return nil, fmt.Errorf("deck parsing failed: %w", err)
	}

	// Step 3: Render each slide to SVG
	result := &Result{
		Slides:     make([][]byte, len(d.Slide)),
		SlideCount: len(d.Slide),
		Title:      d.Title,
	}

	cw := float64(d.Canvas.Width)
	ch := float64(d.Canvas.Height)

	for i := range d.Slide {
		var svgBuf bytes.Buffer
		doc := svg.New(&svgBuf)
		svgslide(doc, d, i, cw, ch)
		result.Slides[i] = svgBuf.Bytes()
	}

	return result, nil
}

// parseDeck parses deck XML into structure
func parseDeck(xmlData []byte, width, height int) (*deck.Deck, error) {
	var d deck.Deck
	if err := xml.Unmarshal(xmlData, &d); err != nil {
		return nil, err
	}

	// Set canvas dimensions if not specified
	if d.Canvas.Width == 0 {
		d.Canvas.Width = width
	}
	if d.Canvas.Height == 0 {
		d.Canvas.Height = height
	}

	return &d, nil
}

// pct converts percentages to canvas measures
func pct(p float64, m float64) float64 {
	return ((p / 100.0) * m)
}

// radians converts degrees to radians
func radians(deg float64) float64 {
	return (deg * math.Pi) / 180.0
}

// polar returns the euclidian coordinates from polar coordinates
func polar(x, y, r, angle float64) (float64, float64) {
	px := (r * math.Cos(radians(angle))) + x
	py := (r * math.Sin(radians(angle))) + y
	return px, py
}

// dimen returns canvas dimensions from percentages
func dimen(w, h float64, xp, yp, sp float64) (float64, float64, float64) {
	return pct(xp, w), pct(100-yp, h), pct(sp, w)
}

// setop sets the alpha value:
// 0 == default value (opaque)
// -1 == fully transparent
// > 0 set opacity percent
func setop(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 0:
		return v / 100
	case v == 0:
		return 1
	}
	return v
}

// whitespace determines if a rune is whitespace
func whitespace(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t'
}

// fontlookup maps font aliases to implementation font names
func fontlookup(s string) string {
	font, ok := fontmap[s]
	if ok {
		return font
	}
	return fontmap["sans"]
}

// colorNumbers returns a list of numbers from a comma separated list,
// in the form of xxx(n1, n2, n3), after removing tabs and spaces.
func colorNumbers(s string) []string {
	return strings.Split(strings.NewReplacer(" ", "", "\t", "").Replace(s[4:len(s)-1]), ",")
}

// hsv2rgb converts hsv(h (0-360), s (0-100), v (0-100)) to rgb
func hsv2rgb(h, s, v float64) (int, int, int) {
	s /= 100
	v /= 100
	if s > 1 || v > 1 {
		return 0, 0, 0
	}
	h = math.Mod(h, 360)
	c := v * s
	section := h / 60
	x := c * (1 - math.Abs(math.Mod(section, 2)-1))

	var r, g, b float64
	switch {
	case section >= 0 && section <= 1:
		r, g, b = c, x, 0
	case section > 1 && section <= 2:
		r, g, b = x, c, 0
	case section > 2 && section <= 3:
		r, g, b = 0, c, x
	case section > 3 && section <= 4:
		r, g, b = 0, x, c
	case section > 4 && section <= 5:
		r, g, b = x, 0, c
	case section > 5 && section <= 6:
		r, g, b = c, 0, x
	default:
		return 0, 0, 0
	}
	m := v - c
	r += m
	g += m
	b += m
	return int(r * 255), int(g * 255), int(b * 255)
}

// h2r converts hsv to rgb colors
func h2r(s string) string {
	var red, green, blue int
	v := colorNumbers(s)
	if len(v) == 3 {
		hue, _ := strconv.ParseFloat(v[0], 64)
		sat, _ := strconv.ParseFloat(v[1], 64)
		value, _ := strconv.ParseFloat(v[2], 64)
		red, green, blue = hsv2rgb(hue, sat, value)
	}
	return fmt.Sprintf("rgb(%d,%d,%d)", red, green, blue)
}

// svgcolor returns a color spec;
// if hsv, convert to rgb, otherwise pass unchanged
func svgcolor(color string) string {
	if strings.HasPrefix(color, "hsv(") && strings.HasSuffix(color, ")") && len(color) > 5 {
		color = h2r(color)
	}
	return color
}

// strokeop stroke a color at the specified opacity
func strokeop(sw float64, color string, opacity float64) string {
	return fmt.Sprintf(strokefmt, sw, svgcolor(color), setop(opacity))
}

// fillop fills with the specified color and opacity
func fillop(color string, opacity float64) string {
	return fmt.Sprintf(fillfmt, svgcolor(color), setop(opacity))
}

// bullet draws a bullet
func bullet(doc *svg.SVG, x, y, size float64, color string) {
	rs := size / 2
	doc.Circle(x-size, y-(rs*2)/3, rs/2, "fill:"+svgcolor(color))
}

// background places a colored rectangle
func background(doc *svg.SVG, w, h float64, color string) {
	dorect(doc, 0, 0, w, h, svgcolor(color), 0)
}

// doline draws a line
func doline(doc *svg.SVG, xp1, yp1, xp2, yp2, sw float64, color string, opacity float64) {
	doc.Line(xp1, yp1, xp2, yp2, strokeop(sw, color, opacity))
}

// doarc draws an arc
func doarc(doc *svg.SVG, x, y, w, h, a1, a2, sw float64, color string, opacity float64) {
	sx, sy := polar(x, y, w, -a1)
	ex, ey := polar(x, y, h, -a2)
	large := a2-a1 >= 180
	doc.Arc(sx, sy, w, h, 0, large, false, ex, ey, "fill:none;"+strokeop(sw, color, opacity))
}

// docurve draws a bezier curve
func docurve(doc *svg.SVG, xp1, yp1, xp2, yp2, xp3, yp3, sw float64, color string, opacity float64) {
	doc.Qbez(xp1, yp1, xp2, yp2, xp3, yp3, "fill:none;"+strokeop(sw, color, opacity))
}

// dorect draws a rectangle
func dorect(doc *svg.SVG, x, y, w, h float64, color string, opacity float64) {
	doc.Rect(x, y, w, h, fillop(color, opacity))
}

// doellipse draws an ellipse
func doellipse(doc *svg.SVG, x, y, w, h float64, color string, opacity float64) {
	doc.Ellipse(x, y, w, h, fillop(color, opacity))
}

// dopoly draws a polygon
func dopoly(doc *svg.SVG, xc, yc string, cw, ch float64, color string, opacity float64) {
	xs := strings.Split(xc, " ")
	ys := strings.Split(yc, " ")
	if len(xs) != len(ys) {
		return
	}
	if len(xs) < 3 || len(ys) < 3 {
		return
	}
	px := make([]float64, len(xs))
	py := make([]float64, len(xs))
	for i := 0; i < len(xs); i++ {
		x, err := strconv.ParseFloat(xs[i], 64)
		if err != nil {
			px[i] = 0
		} else {
			px[i] = pct(x, cw)
		}
		y, err := strconv.ParseFloat(ys[i], 64)
		if err != nil {
			py[i] = 0
		} else {
			py[i] = pct(100-y, ch)
		}
	}
	doc.Polygon(px, py, fillop(color, opacity))
}

// textalign returns the SVG text alignment operator
func textalign(s string) string {
	switch s {
	case "center", "middle", "mid", "c":
		return "middle"
	case "left", "start", "l":
		return "start"
	case "right", "end", "e":
		return "end"
	}
	return "start"
}

// showtext places fully attributed text at the specified location
func showtext(doc *svg.SVG, x, y float64, s string, fs float64, font, color, align string) {
	doc.Text(x, y, s, `xml:space="preserve"`, fmt.Sprintf("fill:%s;font-size:%.2fpx;font-family:%s;text-anchor:%s", svgcolor(color), fs, fontlookup(font), textalign(align)))
}

// dotext places text elements on the canvas according to type
func dotext(doc *svg.SVG, cw, x, y, fs, wp, rotation, ls float64, tdata, font, align, ttype, color string, opacity float64) {
	ls *= fs
	td := strings.Split(tdata, "\n")
	if rotation > 0 {
		doc.RotateTranslate(x, y, rotation)
	}
	var tw float64
	if ttype == "code" {
		font = "mono"
		ch := float64(len(td)) * ls
		tw = cw - x - 20
		dorect(doc, x-fs, y-fs, tw, ch, "rgb(240,240,240)", opacity)
	}
	if ttype == "block" {
		if wp == 0 {
			tw = cw / 2
		} else {
			tw = (cw * (wp / 100.0))
		}
		textwrap(doc, x, y, tw, fs, ls, tdata, font, color, opacity)
	} else {
		for _, t := range td {
			showtext(doc, x, y, t, fs, font, color, align)
			y += ls
		}
	}
	if rotation > 0 {
		doc.Gend()
	}
}

// textwrap draws text at location, wrapping at the specified width
func textwrap(doc *svg.SVG, x, y, w, fs float64, leading float64, s, font, color string, opacity float64) {
	doc.Gstyle(fmt.Sprintf("fill-opacity:%.2f;fill:%s;font-family:%s;font-size:%.2fpx", setop(opacity), svgcolor(color), fontlookup(font), fs))
	words := strings.FieldsFunc(s, whitespace)
	xp := x
	yp := y
	var line string
	for _, s := range words {
		if s == "\\n" {
			yp += leading
			continue
		}
		line += s + " "
		if fs*float64(len(line))*0.65 > (w + x) {
			doc.Text(xp, yp, line)
			yp += leading
			line = ""
		}
	}
	if len(line) > 0 {
		doc.Text(xp, yp, line)
	}
	doc.Gend()
}

// dolist places lists on the canvas
func dolist(doc *svg.SVG, x, y, fs, rotation, lwidth, spacing float64, tlist []deck.ListItem, font, ltype, align, color string, opacity float64) {
	if font == "" {
		font = "sans"
	}
	doc.Gstyle(fmt.Sprintf("fill-opacity:%.2f;fill:%s;font-family:%s;font-size:%.2fpx", setop(opacity), svgcolor(color), fontlookup(font), fs))
	if ltype == "bullet" {
		x += fs
	}
	ls := spacing * fs
	var t string
	for i, tl := range tlist {
		if ltype == "number" {
			t = fmt.Sprintf("%d. ", i+1) + tl.ListText
		} else {
			t = tl.ListText
		}
		if ltype == "bullet" {
			bullet(doc, x, y, fs, color)
		}
		lifmt := fmt.Sprintf("fill-opacity:%.2f;", setop(tl.Opacity))
		if len(tl.Color) > 0 {
			lifmt += "fill:" + tl.Color
		}
		if len(tl.Font) > 0 {
			lifmt += ";font-family:" + tl.Font
		}
		if align == "center" || align == "c" {
			lifmt += ";text-anchor:middle"
		}
		if len(lifmt) > 0 {
			doc.Text(x, y, t, `xml:space="preserve"`, lifmt)
		} else {
			doc.Text(x, y, t, `xml:space="preserve"`)
		}
		y += ls
	}
	doc.Gend()
}

// svgslide makes one slide per SVG page
func svgslide(doc *svg.SVG, d *deck.Deck, n int, cw, ch float64) {
	if n < 0 || n > len(d.Slide)-1 {
		return
	}
	var x, y, fs float64

	doc.Start(cw, ch)
	slide := d.Slide[n]

	// set background, if specified
	if len(slide.Bg) > 0 {
		background(doc, cw, ch, slide.Bg)
	}
	// set gradient background, if specified
	if len(slide.Gradcolor1) > 0 && len(slide.Gradcolor2) > 0 {
		oc := []svg.Offcolor{
			{Offset: 0, Color: slide.Gradcolor1, Opacity: 1.0},
			{Offset: 100, Color: slide.Gradcolor2, Opacity: 1.0},
		}
		doc.Def()
		doc.LinearGradient("slidegrad", 0, 0, 0, 100, oc)
		doc.DefEnd()
		doc.Rect(0, 0, cw, ch, "fill:url(#slidegrad)")
	}
	// set the default foreground
	if slide.Fg == "" {
		slide.Fg = "black"
	}

	// Draw layers in standard order
	layers := []string{"image", "rect", "ellipse", "curve", "arc", "line", "poly", "text", "list"}

	for _, layer := range layers {
		switch layer {
		case "image":
			for _, im := range slide.Image {
				x, y, _ = dimen(cw, ch, im.Xp, im.Yp, 0)
				iw, ih := float64(im.Width), float64(im.Height)

				if im.Scale > 0 {
					iw *= (im.Scale / 100)
					ih *= (im.Scale / 100)
				}
				// scale the image to fit the canvas width
				if im.Autoscale == "on" && iw < cw {
					ih = (cw / iw) * ih
					iw = cw
				}

				midx := iw / 2
				midy := ih / 2
				doc.Image(x-midx, y-midy, int(iw), int(ih), im.Name)
				if len(im.Caption) > 0 {
					capsize := deck.Pwidth(im.Sp, cw, pct(2.0, cw))
					if im.Font == "" {
						im.Font = "sans"
					}
					if im.Color == "" {
						im.Color = slide.Fg
					}
					if im.Align == "" {
						im.Align = "center"
					}
					showtext(doc, x, y+midy+(capsize*2), im.Caption, capsize, im.Font, im.Color, im.Align)
				}
			}

		case "rect":
			for _, rect := range slide.Rect {
				x, y, _ := dimen(cw, ch, rect.Xp, rect.Yp, 0)
				var w, h float64
				w = pct(rect.Wp, cw)
				if rect.Hr == 0 {
					h = pct(rect.Hp, ch)
				} else {
					h = pct(rect.Hr, w)
				}
				if rect.Color == "" {
					rect.Color = defaultColor
				}
				dorect(doc, x-(w/2), y-(h/2), w, h, rect.Color, rect.Opacity)
			}

		case "ellipse":
			for _, ellipse := range slide.Ellipse {
				x, y, _ := dimen(cw, ch, ellipse.Xp, ellipse.Yp, 0)
				var w, h float64
				w = pct(ellipse.Wp, cw)
				if ellipse.Hr == 0 {
					h = pct(ellipse.Hp, ch)
				} else {
					h = pct(ellipse.Hr, w)
				}
				if ellipse.Color == "" {
					ellipse.Color = defaultColor
				}
				doellipse(doc, x, y, w/2, h/2, ellipse.Color, ellipse.Opacity)
			}

		case "curve":
			for _, curve := range slide.Curve {
				if curve.Color == "" {
					curve.Color = defaultColor
				}
				x1, y1, sw := dimen(cw, ch, curve.Xp1, curve.Yp1, curve.Sp)
				x2, y2, _ := dimen(cw, ch, curve.Xp2, curve.Yp2, 0)
				x3, y3, _ := dimen(cw, ch, curve.Xp3, curve.Yp3, 0)
				if sw == 0 {
					sw = 2.0
				}
				docurve(doc, x1, y1, x2, y2, x3, y3, sw, curve.Color, curve.Opacity)
			}

		case "arc":
			for _, arc := range slide.Arc {
				if arc.Color == "" {
					arc.Color = defaultColor
				}
				x, y, sw := dimen(cw, ch, arc.Xp, arc.Yp, arc.Sp)
				w := pct(arc.Wp, cw)
				h := pct(arc.Hp, cw)
				if sw == 0 {
					sw = 2.0
				}
				doarc(doc, x, y, w/2, h/2, arc.A1, arc.A2, sw, arc.Color, arc.Opacity)
			}

		case "line":
			for _, line := range slide.Line {
				if line.Color == "" {
					line.Color = defaultColor
				}
				x1, y1, sw := dimen(cw, ch, line.Xp1, line.Yp1, line.Sp)
				x2, y2, _ := dimen(cw, ch, line.Xp2, line.Yp2, 0)
				if sw == 0 {
					sw = 2.0
				}
				doline(doc, x1, y1, x2, y2, sw, line.Color, line.Opacity)
			}

		case "poly":
			for _, poly := range slide.Polygon {
				if poly.Color == "" {
					poly.Color = defaultColor
				}
				dopoly(doc, poly.XC, poly.YC, cw, ch, poly.Color, poly.Opacity)
			}

		case "text":
			var tdata string
			for _, t := range slide.Text {
				if t.Color == "" {
					t.Color = slide.Fg
				}
				if t.Font == "" {
					t.Font = "sans"
				}
				if t.File != "" {
					tdata = t.File // Note: file reading not supported in WASM
				} else {
					tdata = t.Tdata
				}
				if t.Lp == 0 {
					t.Lp = linespacing
				}
				x, y, fs = dimen(cw, ch, t.Xp, t.Yp, t.Sp)
				dotext(doc, cw, x, y, fs, t.Wp, t.Rotation, t.Lp, tdata, t.Font, t.Align, t.Type, t.Color, t.Opacity)
			}

		case "list":
			for _, l := range slide.List {
				if l.Color == "" {
					l.Color = slide.Fg
				}
				if l.Lp == 0 {
					l.Lp = listspacing
				}
				x, y, fs = dimen(cw, ch, l.Xp, l.Yp, l.Sp)
				dolist(doc, x, y, fs, l.Wp, l.Rotation, l.Lp, l.Li, l.Font, l.Type, l.Align, l.Color, l.Opacity)
			}
		}
	}

	doc.End()
}

// ProcessDeckXML takes deck XML and returns SVG slides (alternative entry point)
func ProcessDeckXML(xmlData []byte, cfg Config) (*Result, error) {
	// Initialize font map
	fontmap["sans"] = cfg.SansFont
	fontmap["serif"] = cfg.SerifFont
	fontmap["mono"] = cfg.MonoFont

	d, err := parseDeck(xmlData, cfg.Width, cfg.Height)
	if err != nil {
		return nil, fmt.Errorf("deck parsing failed: %w", err)
	}

	result := &Result{
		Slides:     make([][]byte, len(d.Slide)),
		SlideCount: len(d.Slide),
		Title:      d.Title,
	}

	cw := float64(d.Canvas.Width)
	ch := float64(d.Canvas.Height)

	for i := range d.Slide {
		var svgBuf bytes.Buffer
		doc := svg.New(&svgBuf)
		svgslide(doc, d, i, cw, ch)
		result.Slides[i] = svgBuf.Bytes()
	}

	return result, nil
}

// RenderSlide renders a single slide to a writer
func RenderSlide(w io.Writer, d *deck.Deck, slideIndex int) error {
	if slideIndex < 0 || slideIndex >= len(d.Slide) {
		return fmt.Errorf("slide index %d out of range", slideIndex)
	}

	cw := float64(d.Canvas.Width)
	ch := float64(d.Canvas.Height)

	doc := svg.New(w)
	svgslide(doc, d, slideIndex, cw, ch)
	return nil
}
