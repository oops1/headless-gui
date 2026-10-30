package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// golden_scene_test.go — пиксельные эталоны сцены (ADR-0002, 2.8): окно
// программы (ExternalSurface) и окно оболочки (widget.Window с тенью) в
// одном Z-порядке, панель задач поверх обоих, тень на стыке с окном
// программы.

// goldenSceneBuffer — детерминированная «картинка программы»: диагональные
// полосы, чтобы на снимке были видны и границы, и тень поверх.
func goldenSceneBuffer(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(((x + y) / 12 % 2) * 60)
			img.SetRGBA(x, y, color.RGBA{R: 40 + v, G: 120 + v, B: 200 - v, A: 255})
		}
	}
	return img
}

// goldenScene собирает сцену; raise — окно программы поднято над окном
// оболочки.
func goldenScene(t *testing.T, raise bool) *image.RGBA {
	t.Helper()
	eng := New(480, 320, 30)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{R: 225, G: 228, B: 232, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 480, 320))

	windows := widget.NewPanel(color.RGBA{})
	windows.ShowHeader = false
	windows.SetBounds(image.Rect(0, 0, 480, 320))
	root.AddChild(windows)

	surf := widget.NewExternalSurface()
	surf.SetBounds(image.Rect(30, 30, 270, 300))
	surf.SetBuffer(goldenSceneBuffer(240, 270), true)
	windows.AddChild(surf)

	win := widget.NewWindow("Shell", 200, 210)
	// Стиль заголовка — явно: по умолчанию он по ОС, и на macOS (кнопки
	// слева, заголовок по центру) кадр разошёлся бы с эталоном.
	win.TitleStyle = widget.WindowTitleWin
	win.CornerRadius = 8
	win.SetShadow(testShadow)
	win.SetBounds(image.Rect(210, 90, 410, 300))
	windows.AddChild(win)

	if raise {
		windows.RaiseChild(surf)
	}

	bar := widget.NewPanel(color.RGBA{R: 32, G: 36, B: 44, A: 255})
	bar.ShowHeader = false
	bar.SetBounds(image.Rect(0, 280, 480, 320))
	root.AddChild(bar)

	eng.SetRoot(root)
	return eng.RenderOnce()
}

func TestGoldenScene_WindowOverSurface(t *testing.T) {
	testreset.Globals(t)
	goldenCompare(t, "scene_window_over_surface", goldenScene(t, false))
}

func TestGoldenScene_SurfaceOverWindow(t *testing.T) {
	testreset.Globals(t)
	goldenCompare(t, "scene_surface_over_window", goldenScene(t, true))
}
