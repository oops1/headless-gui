package svg

import (
	"image/color"
	"strings"
	"testing"
)

// iconSrc собирает иконку из n залитых путей (fill-rule по флагу).
func iconSrc(n int, evenOdd bool) string {
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 24 24">`)
	rule := ""
	if evenOdd {
		rule = ` fill-rule="evenodd"`
	}
	for i := 0; i < n; i++ {
		x := 1 + i%5*4
		y := 1 + i/5*4
		b.WriteString(`<path fill="#ffffff"` + rule + ` d="M`)
		b.WriteString(itoa(x) + " " + itoa(y) + "h3v3h-3z")
		b.WriteString(`M` + itoa(x+1) + " " + itoa(y+1) + `h1v1h-1z"/>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Растеризация без even-odd: полноразмерных буферов быть не должно.
func BenchmarkRasterizeNonzero(b *testing.B) {
	doc, err := Parse([]byte(iconSrc(12, false)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.Rasterize(64, 64, color.RGBA{255, 255, 255, 255}, false)
	}
}

func BenchmarkRasterizeEvenOdd(b *testing.B) {
	doc, err := Parse([]byte(iconSrc(12, true)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.Rasterize(64, 64, color.RGBA{255, 255, 255, 255}, false)
	}
}
