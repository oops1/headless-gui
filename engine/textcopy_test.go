package engine

import (
	"os"
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

func readAssetFont(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile("../assets/fonts/" + file)
	if err != nil {
		t.Skipf("%s недоступен: %v", file, err)
	}
	return data
}

// Вторичный движок получает шрифты приложения (именованный, подменённый
// встроенный жирный, запасной), шрифт по умолчанию, DPI шрифтов, слой
// качества и выбранный приложением подпиксель — и рисует текст тех же ширин.
func TestCopyTextSettingsFrom(t *testing.T) {
	testreset.Globals(t)
	ui := readAssetFont(t, "OpenSans-Regular.ttf")
	bold := readAssetFont(t, "OpenSans-Bold.ttf")
	fallback := readAssetFont(t, "DejaVuSans.ttf")

	src := New(200, 100, 30)
	defer src.Stop()
	src.RegisterFont("AppUI", ui)
	src.RegisterFont(widget.BuiltinFontBold, bold)
	src.RegisterFallbackFont(fallback)
	if !src.SetDefaultFont("AppUI") {
		t.Fatal("SetDefaultFont(AppUI) не принят")
	}
	src.SetDPI(120)
	q := TextQuality{Hinting: true, HintingX: true, LCD: true, LCDOrder: LCDOrderBGR}
	src.SetTextQuality(q)
	src.SetTextSubpixel(true)

	dst := New(100, 80, 30)
	defer dst.Stop()
	const text = "Репозиторий Правка awesome-design"
	if dst.canvas.MeasureText(text, 10) == src.canvas.MeasureText(text, 10) {
		t.Fatal("до копирования ширины уже равны — тест ничего не проверяет")
	}
	dst.CopyTextSettingsFrom(src)

	if !slices.Contains(dst.AvailableFonts(), "AppUI") {
		t.Errorf("шрифт AppUI не скопирован: %v", dst.AvailableFonts())
	}
	if a, b := src.canvas.MeasureText(text, 10), dst.canvas.MeasureText(text, 10); a != b {
		t.Errorf("шрифт по умолчанию и DPI: ширина %d, у источника %d", b, a)
	}
	if a, b := src.canvas.MeasureTextFont(text, 10, widget.BuiltinFontBold), dst.canvas.MeasureTextFont(text, 10, widget.BuiltinFontBold); a != b {
		t.Errorf("жирный: ширина %d, у источника %d", b, a)
	}
	if got := dst.TextQuality(); got != q {
		t.Errorf("слой качества %+v, ждал %+v", got, q)
	}
	if !dst.TextSubpixel() {
		t.Error("подпиксель, выбранный приложением, не скопирован")
	}
	if n := len(dst.fontOps); n != 3 {
		t.Errorf("записано шрифтов %d, ждал 3 (AppUI, жирный, запасной)", n)
	}

	// Повторная регистрация того же имени и того же запасного шрифта журнал
	// не растит; запись имени переезжает в конец (последняя регистрация).
	src.RegisterFont("AppUI", append([]byte(nil), ui...))
	src.RegisterFallbackFont(append([]byte(nil), fallback...))
	if n := len(src.fontOps); n != 3 {
		t.Errorf("после повторных регистраций записано %d шрифтов, ждал 3", n)
	}

	// Источник не меняется; копия у себя самого и у nil — без последствий.
	before := len(src.fontOps)
	src.CopyTextSettingsFrom(src)
	src.CopyTextSettingsFrom(nil)
	if len(src.fontOps) != before {
		t.Errorf("копирование в себя изменило источник: %d шрифтов", len(src.fontOps))
	}
}

// Шрифты конструктора (assets/fonts, встроенные) есть у каждого движка и не
// записываются: свежий движок вторичному ничего не передаёт, и подпиксель,
// не выбранный приложением, остаётся за темой.
func TestCopyTextSettingsFrom_Fresh(t *testing.T) {
	testreset.Globals(t)
	src := New(100, 80, 30)
	defer src.Stop()
	if n := len(src.fontOps); n != 0 {
		t.Fatalf("у свежего движка записано %d шрифтов", n)
	}
	dst := New(100, 80, 30)
	defer dst.Stop()
	dst.CopyTextSettingsFrom(src)
	if len(dst.fontOps) != 0 || dst.defaultFont != "" || dst.subpixelPinned {
		t.Errorf("свежий источник: шрифтов %d, по умолчанию %q, подпиксель закреплён %v",
			len(dst.fontOps), dst.defaultFont, dst.subpixelPinned)
	}
	if dst.TextQuality() != DefaultTextQuality() {
		t.Errorf("слой %+v, ждал по умолчанию", dst.TextQuality())
	}
}
