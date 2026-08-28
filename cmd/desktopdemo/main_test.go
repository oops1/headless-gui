package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
)

// Демонстрация собирается и рисуется под каждой темой.
//
// Демонстрационная программа — первое, что смотрит новый человек, и первое,
// что ломается незаметно: она не участвует в тестах, компилируется отдельно
// и падает на запуске. Здесь собирается ровно та же сцена, что показывает
// main, и по кадру на каждую тему проверяется, что она вообще рисуется.
func TestDemoScene_RendersUnderEveryTheme(t *testing.T) {
	eng := engine.New(screenW, screenH, 60)
	sc := buildDesktop(eng)
	defer sc.close()
	eng.SetRoot(sc.root)

	for i, th := range themeOrder {
		img := eng.RenderOnce()
		if img == nil {
			t.Fatalf("%s: кадр не отрисован", th.label)
		}
		// Панель задач стоит у нижнего края и отличается от обоев над ней.
		if !bottomStripDiffers(img) {
			t.Errorf("%s: панель задач не отличается от фона", th.label)
		}
		saveIfAsked(t, img, th.profile)

		// Следующая тема — тем же способом, что и кнопка в демонстрации.
		if i+1 < len(themeOrder) {
			sc.apply(i + 1)
			eng.Invalidate()
		}
	}
}

// bottomStripDiffers — нижняя полоса кадра отличается от полосы над ней.
func bottomStripDiffers(img *image.RGBA) bool {
	b := img.Bounds()
	y1 := b.Max.Y - 4
	y2 := b.Max.Y - 80 // выше самой высокой панели (macOS — 64)
	if y2 <= b.Min.Y {
		return false
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		if img.RGBAAt(x, y1) != img.RGBAAt(x, y2) {
			return true
		}
	}
	return false
}

func saveIfAsked(t *testing.T, img *image.RGBA, name string) {
	dir := os.Getenv("GOLDEN_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, "demo_"+name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
