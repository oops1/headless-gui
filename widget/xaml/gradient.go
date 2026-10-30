package xaml

// gradient.go — LinearGradientBrush в разметке: кодирование в строку
// ресурса и разбор обратно в *base.LinearGradient.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// encodeGradient кодирует <LinearGradientBrush> в строку для передачи через
// атрибут: "h|v;#RRGGBBAA@offset;...". Возвращает "" если не градиент.
func encodeGradient(brush *Element) string {
	if !strings.EqualFold(brush.Tag, "LinearGradientBrush") {
		return ""
	}
	orient := "v"
	if gradientIsHorizontal(brush.Attr("StartPoint"), brush.Attr("EndPoint")) {
		orient = "h"
	}
	var parts []string
	parts = append(parts, orient)
	for i := range brush.Children {
		s := &brush.Children[i]
		if !strings.EqualFold(s.Tag, "GradientStop") {
			continue
		}
		col := s.Attr("Color")
		off := s.Attr("Offset")
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
func parseGradient(s string) *base.LinearGradient {
	parts := strings.Split(s, ";")
	if len(parts) < 3 {
		return nil
	}
	g := &base.LinearGradient{Horizontal: parts[0] == "h"}
	for _, p := range parts[1:] {
		at := strings.IndexByte(p, '@')
		colStr, offStr := p, ""
		if at >= 0 {
			colStr, offStr = p[:at], p[at+1:]
		}
		col, err := ParseColor(colStr)
		if err != nil {
			continue
		}
		off, _ := strconv.ParseFloat(strings.TrimSpace(offStr), 64)
		g.Stops = append(g.Stops, base.GradientStop{Color: col, Offset: off})
	}
	if len(g.Stops) < 2 {
		return nil
	}
	sort.SliceStable(g.Stops, func(i, j int) bool { return g.Stops[i].Offset < g.Stops[j].Offset })
	return g
}
