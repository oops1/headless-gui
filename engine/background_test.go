package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Фон из памяти и снятие фона.
//
// Раньше вход был один — файл, и потребитель писал уже готовое изображение на
// диск, чтобы движок прочитал его обратно; «просто тёмный стол» изображался
// растянутым однопиксельным PNG, потому что снять фон было нельзя.

// solidImage — равномерная картинка заданного цвета.
func solidImage(w, h int, col color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

func TestSetBackground_FromMemory(t *testing.T) {
	const w, h = 120, 80
	eng := New(w, h, 30)

	// Пустой корень: то, что видно, — это фон.
	root := widget.NewPanel(color.RGBA{})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 0, 0))
	eng.SetRoot(root)

	green := color.RGBA{R: 40, G: 160, B: 70, A: 255}
	if err := eng.SetBackground(solidImage(w, h, green)); err != nil {
		t.Fatalf("SetBackground: %v", err)
	}
	img := eng.RenderOnce()
	if img == nil {
		t.Fatal("кадр не отрисован")
	}
	if got := img.RGBAAt(w/2, h/2); got != green {
		t.Errorf("фон из памяти дал %v, ждали %v", got, green)
	}

	// Снятие фона возвращает чёрный — не требуя однопиксельного файла.
	eng.ClearBackground()
	img = eng.RenderOnce()
	if got := img.RGBAAt(w/2, h/2); got == green {
		t.Error("ClearBackground не снял фон")
	}
}

func TestSetBackground_RejectsEmpty(t *testing.T) {
	eng := New(40, 30, 30)
	if err := eng.SetBackground(nil); err == nil {
		t.Error("nil принят как фон — снятие фона делается ClearBackground")
	}
	if err := eng.SetBackground(image.NewRGBA(image.Rectangle{})); err == nil {
		t.Error("пустое изображение принято как фон")
	}
}
