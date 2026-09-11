package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Цвет рамки главного окна — запрос GG-56.
//
// Главное окно рисовало контур XOR-инверсией фона и BorderColor не читало
// вовсе. Windows с включённым «цветом элементов в заголовках и границах окон»
// красит рамку активного окна акцентом, и окно с серо-чёрным контуром рядом с
// системными выглядело чужим.
//
// Предложенная проверка BorderColor.A > 0 не годится: движок заполняет
// BorderColor сам — в NewWindow и в ApplyTheme, — и XOR-рамка пропала бы у
// всех окон. Поэтому у рамки свой цвет: FrameColor и Theme.WindowFrame.

var accent = color.RGBA{R: 0x00, G: 0x78, B: 0xD4, A: 255}

// frameScene рисует главное окно и возвращает цвет точки на левой рамке.
func frameScene(t *testing.T, setup func(w *widget.Window)) (color.RGBA, *widget.Window) {
	t.Helper()
	w := widget.NewWindow("Go.Git", 320, 200)
	w.MainWindow = true
	w.TitleStyle = widget.WindowTitleWin
	w.CornerRadius = 0
	if setup != nil {
		setup(w)
	}
	w.SetBounds(image.Rect(0, 0, 320, 200))

	eng := engine.New(320, 200, 30)
	eng.SetRoot(w)
	eng.RenderOnce()
	img := snapshotRGBA(eng.RenderOnce())
	i := img.PixOffset(0, 120) // левая рамка, ниже заголовка
	return color.RGBA{R: img.Pix[i], G: img.Pix[i+1], B: img.Pix[i+2], A: 255}, w
}

func xorOf(c color.RGBA) color.RGBA {
	return color.RGBA{R: ^c.R, G: ^c.G, B: ^c.B, A: 255}
}

// Без заданного цвета главное окно рисуется как раньше — XOR-рамкой.
func TestFrameColor_DefaultIsXOR(t *testing.T) {
	got, w := frameScene(t, nil)
	if w.FrameColor.A != 0 {
		t.Fatalf("у окна без темы с рамкой появился цвет рамки %v", w.FrameColor)
	}
	if got == accent {
		t.Fatal("рамка окрашена акцентом без запроса")
	}
	if want := xorOf(w.Background); got != want {
		t.Errorf("рамка %v, ожидалась XOR-инверсия фона %v", got, want)
	}
}

// Свой цвет красит рамку главного окна.
func TestFrameColor_SetFrameColorPaintsMainWindow(t *testing.T) {
	got, _ := frameScene(t, func(w *widget.Window) { w.SetFrameColor(accent) })
	if got != accent {
		t.Errorf("рамка %v, задан %v", got, accent)
	}
}

// Смена темы не сбрасывает цвет, заданный приложением: акцент системы не
// должен слетать при переключении светлой и тёмной темы.
func TestFrameColor_SurvivesThemeSwitch(t *testing.T) {
	got, _ := frameScene(t, func(w *widget.Window) {
		w.SetFrameColor(accent)
		w.ApplyTheme(widget.ThemeByName("Win11 Light"))
	})
	if got != accent {
		t.Errorf("после смены темы рамка %v, задан %v", got, accent)
	}
}

// Цвет рамки — часть темы: Theme.WindowFrame красит рамку без кода в окне.
func TestFrameColor_FromTheme(t *testing.T) {
	th := widget.ThemeByName("Win10 Dark")
	th.WindowFrame = accent
	got, _ := frameScene(t, func(w *widget.Window) { w.ApplyTheme(th) })
	if got != accent {
		t.Errorf("рамка %v, в теме %v", got, accent)
	}
}

// Неактивное окно приглушает рамку, как Windows гасит акцент окна без фокуса.
func TestFrameColor_InactiveIsDimmed(t *testing.T) {
	got, _ := frameScene(t, func(w *widget.Window) {
		w.SetFrameColor(accent)
		w.SetActive(false)
	})
	if got == accent {
		t.Error("рамка неактивного окна не приглушена")
	}
	if got == (color.RGBA{A: 255}) {
		t.Error("рамка неактивного окна пропала")
	}
}

// Нулевой цвет снимает закрепление: рамка снова прежняя.
func TestFrameColor_ZeroRestoresXOR(t *testing.T) {
	got, w := frameScene(t, func(w *widget.Window) {
		w.SetFrameColor(accent)
		w.SetFrameColor(color.RGBA{})
	})
	if want := xorOf(w.Background); got != want {
		t.Errorf("после сброса рамка %v, ожидалась XOR %v", got, want)
	}
}
