package engine

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// cliptext_bench_test.go — стоимость отрисовки текста под активным клипом
// (PERF-7). Типовые сценарии: длинная строка в узком ScrollView/TextInput,
// виртуальный список, горизонтально прокрученное поле.

var benchTextCol = color.RGBA{R: 230, G: 230, B: 230, A: 255}

// benchLongLine — длинная строка (~420 рун), кириллица + латиница + цифры.
var benchLongLine = strings.Repeat("Пример длинной строки Example long string 0123456789 ", 8)

// BenchmarkDrawTextClipNarrow — строка начинается внутри клипа, но почти вся
// уходит вправо за его край (узкое поле ввода, колонка таблицы).
func BenchmarkDrawTextClipNarrow(b *testing.B) {
	eng := New(1280, 800, 20)
	c := eng.canvas
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.SetClip(image.Rect(100, 100, 340, 120))
		c.DrawTextSize(benchLongLine, 100, 100, 12, benchTextCol)
		c.ClearClip()
	}
}

// BenchmarkDrawTextClipScrolled — поле прокручено вправо: начало строки левее
// клипа, видно окно в середине (TextInput со scrollX, ScrollView по X).
func BenchmarkDrawTextClipScrolled(b *testing.B) {
	eng := New(1280, 800, 20)
	c := eng.canvas
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.SetClip(image.Rect(100, 100, 340, 120))
		c.DrawTextSize(benchLongLine, -1500, 100, 12, benchTextCol)
		c.ClearClip()
	}
}

// BenchmarkDrawTextClipFull — контрольный замер: строка целиком внутри клипа
// (быстрый путь не должен подорожать).
func BenchmarkDrawTextClipFull(b *testing.B) {
	eng := New(1280, 800, 20)
	c := eng.canvas
	short := "Пример строки Example string 0123456789"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.SetClip(image.Rect(0, 90, 1280, 130))
		c.DrawTextSize(short, 10, 100, 12, benchTextCol)
		c.ClearClip()
	}
}

// BenchmarkDrawTextNoClip — контрольный замер без клипа вовсе.
func BenchmarkDrawTextNoClip(b *testing.B) {
	eng := New(1280, 800, 20)
	c := eng.canvas
	short := "Пример строки Example string 0123456789"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawTextSize(short, 10, 100, 12, benchTextCol)
	}
}

// TestDrawTextClipHorizontalCulling_Identical — горизонтальный отсев глифов не
// меняет НИ ОДНОГО пикселя: результат отрисовки под клипом обязан совпадать с
// результатом отрисовки той же строки без отсева (эмулируется широким клипом
// и последующим сравнением только внутри узкой области).
func TestDrawTextClipHorizontalCulling_Identical(t *testing.T) {
	const w, h = 400, 60
	clip := image.Rect(120, 20, 260, 44)

	for _, x := range []int{-1200, -300, -40, 0, 130, 250, 700} {
		// Эталон: та же строка под клипом во весь холст (отсев не срабатывает).
		ref := New(w, h, 20)
		ref.canvas.SetClip(image.Rect(0, 0, w, h))
		ref.canvas.DrawTextSize(benchLongLine, x, 24, 12, benchTextCol)
		ref.canvas.ClearClip()

		got := New(w, h, 20)
		got.canvas.SetClip(clip)
		got.canvas.DrawTextSize(benchLongLine, x, 24, 12, benchTextCol)
		got.canvas.ClearClip()

		// Чистый холст — эталон для области ВНЕ клипа.
		blank := New(w, h, 20)

		for y := 0; y < h; y++ {
			for px := 0; px < w; px++ {
				o := got.canvas.back.PixOffset(px, y)
				want := blank.canvas.back.Pix[o : o+4]
				if image.Pt(px, y).In(clip) {
					want = ref.canvas.back.Pix[o : o+4]
				}
				g := got.canvas.back.Pix[o : o+4]
				if g[0] != want[0] || g[1] != want[1] || g[2] != want[2] || g[3] != want[3] {
					t.Fatalf("x=%d: пиксель (%d,%d): got %v, want %v", x, px, y, g, want)
				}
			}
		}
	}
}
