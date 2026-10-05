package tests

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки для просмотра глазами: окно с заголовком, подписью и кнопкой под
// профилем Windows 11 (10 pt) и Windows 10 (8,5 pt из Fonts[default]).
//
// Запуск: GOLDEN_OUT=<папка> go test ./tests -run DefaultFontShots -count=1
func TestDefaultFontShots(t *testing.T) {
	dir := os.Getenv("GOLDEN_OUT")
	if dir == "" {
		t.Skip("снимки делаются только при заданном GOLDEN_OUT")
	}
	for _, profile := range []string{theme.ProfileWindows11, theme.ProfileWindows10} {
		resetDefaultFont(t)
		m := fontTestManager(t)
		eng := engine.New(360, 160, 30)
		if err := eng.SetThemeProfile(m, profile); err != nil {
			t.Fatal(err)
		}
		win := widget.NewWindow("Заголовок окна приложения", 320, 120)
		win.SetBounds(image.Rect(20, 16, 340, 144))
		lbl := widget.NewLabel("Подпись без своего кегля", color.RGBA{A: 255})
		lbl.SetBounds(image.Rect(40, 60, 300, 80))
		win.AddChild(lbl)
		eng.SetRoot(win)

		f, err := os.Create(filepath.Join(dir, "defaultfont_"+profile+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, eng.RenderOnce()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
