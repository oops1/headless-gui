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

// Значок 48×48 без градиентов — именно этот путь не должен замедляться от
// поддержки градиентов/clipPath/mask (см. BenchmarkRasterizeGradient48).
func BenchmarkRasterizeFlat48(b *testing.B) {
	doc, err := Parse([]byte(iconSrc(12, false)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.Rasterize(48, 48, color.RGBA{255, 255, 255, 255}, false)
	}
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

// gradientIconSrc — значок в духе Adwaita: подложка с линейным градиентом,
// блик радиальным, плюс вырезка clipPath и мягкая маска.
const gradientIconSrc = `<svg viewBox="0 0 48 48" xmlns:xlink="http://www.w3.org/1999/xlink">
<defs>
<linearGradient id="a" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#9cf"/><stop offset="1" stop-color="#25d"/></linearGradient>
<radialGradient id="b" cx="0.3" cy="0.3" r="0.7"><stop offset="0" stop-color="#fff" stop-opacity="0.8"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></radialGradient>
<clipPath id="c"><rect x="4" y="4" width="40" height="40" rx="8"/></clipPath>
<mask id="m"><rect width="48" height="48" fill="url(#a)"/></mask>
</defs>
<g clip-path="url(#c)">
<rect x="4" y="4" width="40" height="40" fill="url(#a)"/>
<circle cx="16" cy="16" r="26" fill="url(#b)"/>
<rect x="8" y="30" width="32" height="10" fill="#000" mask="url(#m)"/>
</g></svg>`

func BenchmarkRasterizeGradient48(b *testing.B) {
	doc, err := Parse([]byte(gradientIconSrc))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.Rasterize(48, 48, color.RGBA{255, 255, 255, 255}, false)
	}
}

func BenchmarkParseFlat(b *testing.B) {
	src := []byte(iconSrc(12, false))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseGradient(b *testing.B) {
	src := []byte(gradientIconSrc)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}
