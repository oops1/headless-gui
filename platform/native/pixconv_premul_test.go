package native

import (
	"image"
	"testing"
)

// Буфер с альфой (Wayland ARGB8888, окно X11 глубины 32) получает картинку
// движка, а она УЖЕ premultiplied. Корректная точка проходит без изменений —
// прежнее повторное умножение давало тёмную кайму по скруглённым углам; точка
// с цветом больше альфы обрезается — иначе у компоновщика пересвет.
func TestPremulConversion(t *testing.T) {
	src := []byte{
		200, 100, 50, 255, // непрозрачная: только перестановка
		28, 28, 28, 28, // корректный сглаженный край белого — как есть
		255, 255, 255, 28, // straight-белый край — обрезается до (28,28,28)
		0, 0, 0, 0, // прозрачная
		10, 0, 0, 60, // полупрозрачная тень с примесью — как есть
	}
	want := []byte{
		50, 100, 200, 255,
		28, 28, 28, 28,
		28, 28, 28, 28,
		0, 0, 0, 0,
		0, 0, 10, 60,
	}
	row := make([]byte, len(src))
	SwapRBRowPremul(row, src)
	rect := make([]byte, len(src))
	ConvRectBGRAPremul(rect, len(src), src, len(src), image.Rect(0, 0, len(src)/4, 1))
	for i := range want {
		if row[i] != want[i] {
			t.Fatalf("SwapRBRowPremul: байт %d = %d, ждали %d (%v)", i, row[i], want[i], row)
		}
		if rect[i] != want[i] {
			t.Fatalf("ConvRectBGRAPremul: байт %d = %d, ждали %d (%v)", i, rect[i], want[i], rect)
		}
	}
}
