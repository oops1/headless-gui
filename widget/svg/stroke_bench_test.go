package svg

import (
	"image/color"
	"testing"
)

// strokeIconSrc — значок из обводок с кривыми: то, на чём работает StrokeJoins.
const strokeIconSrc = `<svg viewBox="0 0 48 48"><g fill="none" stroke="#333" stroke-width="3" stroke-linejoin="round" stroke-linecap="round">
<circle cx="24" cy="24" r="18"/><path d="M12 28 Q24 8 36 28 T44 30"/><path d="M8 40 H40" stroke-dasharray="4 2"/></g></svg>`

// Стоимость включённой обводки (StrokeJoins) на значке с кривыми.
func BenchmarkRasterizeStrokeJoins48(b *testing.B) {
	doc, err := Parse([]byte(strokeIconSrc))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.RasterizeWith(48, 48, color.RGBA{255, 255, 255, 255}, false, Options{StrokeJoins: true})
	}
}

// Прежняя обводка того же значка — для сравнения.
func BenchmarkRasterizeStrokeLegacy48(b *testing.B) {
	doc, err := Parse([]byte(strokeIconSrc))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.RasterizeWith(48, 48, color.RGBA{255, 255, 255, 255}, false, Options{})
	}
}
