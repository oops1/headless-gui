package engine

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// API слоя качества текста на уровне движка: режим, наследование шрифтами,
// именованный шрифт. Пути текста холста (шейпер, повёрнутый текст, HiDPI,
// формат буфера, LCD) — в core/raster/textquality_paths_test.go, таблицы
// контраста — в core/text.

// Движок: режим выключен по умолчанию, значения нормализуются, шрифты,
// зарегистрированные позже, и запасные наследуют режим, кэш масок сбрасывается.
func TestTextQuality_EngineAPI(t *testing.T) {
	testreset.Globals(t)
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skip(err)
	}
	e := New(200, 100, 20)
	t.Cleanup(e.Stop)
	if q := e.TextQuality(); q != (TextQuality{}) {
		t.Fatalf("по умолчанию режим не нулевой: %+v", q)
	}
	e.RegisterFont("OS-early", data)
	e.canvas.AddFallbackFont(data)
	before := e.canvas.DefaultFont().Glyph(9, 'e')
	beforeMask := string(before.Mask.Pix)

	e.SetTextHinting(true)
	e.SetTextContrast(2) // обрезается до 1
	q := e.TextQuality()
	if !q.Hinting || q.Contrast != 1 || q.LCD {
		t.Fatalf("SetTextHinting/SetTextContrast: %+v", q)
	}
	e.RegisterFont("OS-late", data)
	e.canvas.AddFallbackFont(data)
	for _, name := range []string{"OS-early", "OS-late"} {
		if got := e.canvas.FontFor(name).Quality(); got != q {
			t.Errorf("шрифт %s: режим %+v, ожидался %+v", name, got, q)
		}
	}
	for i, fb := range e.canvas.Fallbacks() {
		fc := fb.Get(e.canvas.DefaultFont())
		if fc == nil || fc.Quality() != q {
			t.Errorf("запасной шрифт %d не получил режим", i)
		}
	}
	if string(e.canvas.DefaultFont().Glyph(9, 'e').Mask.Pix) == beforeMask {
		t.Error("кэш масок основного шрифта не сброшен при смене режима")
	}

	e.SetTextQuality(TextQuality{LCD: true, LCDOrder: 9})
	if got := e.TextQuality(); got.LCDOrder != LCDOrderRGB || !got.LCD || got.Hinting {
		t.Errorf("LCDOrder не нормализован: %+v", got)
	}
	e.SetTextQuality(TextQuality{})
	if string(e.canvas.DefaultFont().Glyph(9, 'e').Mask.Pix) != beforeMask {
		t.Error("после выключения маска не вернулась к прежней")
	}
	// Режим переживает смену масштаба (новый холст, шрифты общие).
	e.SetTextQuality(TextQuality{Hinting: true})
	e.SetScale(2)
	if !e.TextQuality().Hinting {
		t.Error("режим потерян после SetScale")
	}
}

// Именованные шрифты рисуются тем же слоем: DrawTextFont и шрифт по умолчанию
// дают одну картинку; строчные одного кегля на одних строках.
func TestTextQuality_NamedFontMatchesDefault(t *testing.T) {
	testreset.Globals(t)
	data, err := os.ReadFile("../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skip(err)
	}
	e := New(300, 60, 20)
	t.Cleanup(e.Stop)
	e.RegisterFont("OS", data)
	e.SetTextQuality(TextQuality{Hinting: true, Contrast: DefaultTextContrast})
	c := e.canvas
	c.FillRect(0, 0, 300, 60, color.RGBA{tqBG, tqBG, tqBG, 255})
	c.DrawTextFont("oops-hp.local Подключение", 4, 4, 9, "OS", color.RGBA{20, 20, 20, 255})
	named := c.Snapshot(image.Rect(0, 0, 300, 30))
	c.FillRect(0, 0, 300, 60, color.RGBA{tqBG, tqBG, tqBG, 255})
	fc := tqFont(t, TextQuality{Hinting: true, Contrast: DefaultTextContrast})
	d := tqRender(fc, 12, 20, "oops-hp.local Подключение")
	ref := d.Snapshot(image.Rect(0, 0, 300, 30))
	if string(named.Pix) != string(ref.Pix) {
		t.Error("именованный шрифт с режимом даёт иную картинку, чем тот же файл как основной")
	}
}
