// gradient.go — LinearGradientBrush (упрощённый) для фонов контейнеров.
//
// Движок рисует только сплошные заливки, поэтому градиент аппроксимируется
// построчной (или поколоночной) заливкой с интерполяцией цвета между стопами.
// Поддерживается для фона Panel (включая Border, который строится как Panel-like
// через DockPanel — там градиент не применяется).
package widget

import (
	"image"
	"image/color"
	"sort"
	"strconv"
	"strings"
)

// GradientStop — опорная точка градиента (цвет на смещении 0..1).
type GradientStop struct {
	Color  color.RGBA
	Offset float64
}

// LinearGradient — линейный градиент (вертикальный по умолчанию).
type LinearGradient struct {
	Horizontal bool
	Stops      []GradientStop
}

// colorAt возвращает интерполированный цвет на позиции t (0..1).
func (g *LinearGradient) colorAt(t float64) color.RGBA {
	if len(g.Stops) == 0 {
		return color.RGBA{}
	}
	if t <= g.Stops[0].Offset {
		return g.Stops[0].Color
	}
	last := g.Stops[len(g.Stops)-1]
	if t >= last.Offset {
		return last.Color
	}
	for i := 1; i < len(g.Stops); i++ {
		a, b := g.Stops[i-1], g.Stops[i]
		if t <= b.Offset {
			span := b.Offset - a.Offset
			f := 0.0
			if span > 0 {
				f = (t - a.Offset) / span
			}
			return lerpColor(a.Color, b.Color, f)
		}
	}
	return last.Color
}

func lerpColor(a, b color.RGBA, f float64) color.RGBA {
	li := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.RGBA{R: li(a.R, b.R), G: li(a.G, b.G), B: li(a.B, b.B), A: li(a.A, b.A)}
}

// drawLinearGradient заполняет прямоугольник r градиентом g.
func drawLinearGradient(ctx DrawContext, r image.Rectangle, g *LinearGradient) {
	if g == nil || len(g.Stops) == 0 || r.Empty() {
		return
	}
	if len(g.Stops) == 1 {
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), g.Stops[0].Color)
		return
	}
	if g.Horizontal {
		w := r.Dx()
		for x := 0; x < w; x++ {
			t := float64(x) / float64(w-1)
			ctx.FillRect(r.Min.X+x, r.Min.Y, 1, r.Dy(), g.colorAt(t))
		}
		return
	}
	h := r.Dy()
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		ctx.DrawHLine(r.Min.X, r.Min.Y+y, r.Dx(), g.colorAt(t))
	}
}

// encodeGradient кодирует <LinearGradientBrush> в строку для передачи через
// атрибут: "h|v;#RRGGBBAA@offset;...". Возвращает "" если не градиент.
func encodeGradient(brush *xElement) string {
	if !strings.EqualFold(brush.Tag, "LinearGradientBrush") {
		return ""
	}
	orient := "v"
	if gradientIsHorizontal(brush.attr("StartPoint"), brush.attr("EndPoint")) {
		orient = "h"
	}
	var parts []string
	parts = append(parts, orient)
	for i := range brush.Children {
		s := &brush.Children[i]
		if !strings.EqualFold(s.Tag, "GradientStop") {
			continue
		}
		col := s.attr("Color")
		off := s.attr("Offset")
		if col == "" {
			continue
		}
		parts = append(parts, col+"@"+off)
	}
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts, ";")
}

func gradientIsHorizontal(sp, ep string) bool {
	sx, sy := parsePointXY(sp)
	ex, ey := parsePointXY(ep)
	return absf(ex-sx) > absf(ey-sy)
}

func parsePointXY(s string) (float64, float64) {
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) != 2 {
		return 0, 0
	}
	x, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	y, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return x, y
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// parseGradient разбирает строку из encodeGradient обратно в *LinearGradient.
func parseGradient(s string) *LinearGradient {
	parts := strings.Split(s, ";")
	if len(parts) < 3 {
		return nil
	}
	g := &LinearGradient{Horizontal: parts[0] == "h"}
	for _, p := range parts[1:] {
		at := strings.IndexByte(p, '@')
		colStr, offStr := p, ""
		if at >= 0 {
			colStr, offStr = p[:at], p[at+1:]
		}
		col, err := parseXAMLColor(colStr)
		if err != nil {
			continue
		}
		off, _ := strconv.ParseFloat(strings.TrimSpace(offStr), 64)
		g.Stops = append(g.Stops, GradientStop{Color: col, Offset: off})
	}
	if len(g.Stops) < 2 {
		return nil
	}
	sort.SliceStable(g.Stops, func(i, j int) bool { return g.Stops[i].Offset < g.Stops[j].Offset })
	return g
}
