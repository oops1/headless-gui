package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Возможности редактора на настоящем движке: Tab через цепочку доставки
// клавиш, шрифт и цвет диапазонов в нарисованном кадре, полоса прокрутки.

type spanStyler func(line int, text string) []widget.Span

func (f spanStyler) LineSpans(line int, text string) []widget.Span { return f(line, text) }

// tbScene — движок с редактором и соседней кнопкой, куда уходит фокус по Tab.
func tbScene(t *testing.T, tb *widget.TextBox) (*engine.Engine, *widget.Button) {
	t.Helper()
	eng := engine.New(320, 200, 30)
	t.Cleanup(eng.Stop)
	eng.SetTooltipsEnabled(false)
	tb.SetBounds(image.Rect(10, 10, 310, 110))
	btn := widget.NewButton("Дальше")
	btn.SetBounds(image.Rect(10, 130, 110, 160))
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 320, 200))
	root.AddChild(tb)
	root.AddChild(btn)
	eng.SetRoot(root)
	eng.SetFocus(tb)
	return eng, btn
}

// Без AcceptTab Tab уводит фокус — как у приложений, собранных до появления
// поля. С AcceptTab он остаётся в тексте как символ.
func TestTextBox_TabKeyThroughEngine(t *testing.T) {
	tab := widget.KeyEvent{Code: widget.KeyTab, Pressed: true}

	tb := widget.NewTextBox("")
	eng, btn := tbScene(t, tb)
	eng.SendKeyEvent(tab)
	if got := tb.GetText(); got != "" {
		t.Errorf("без AcceptTab Tab вставил %q", got)
	}
	if !btn.IsFocused() || tb.IsFocused() {
		t.Error("без AcceptTab Tab должен был перевести фокус на кнопку")
	}

	tb2 := widget.NewTextBox("")
	tb2.AcceptTab = true
	eng2, btn2 := tbScene(t, tb2)
	eng2.SendKeyEvent(widget.KeyEvent{Rune: 'a', Pressed: true})
	eng2.SendKeyEvent(tab)
	eng2.SendKeyEvent(widget.KeyEvent{Rune: 'b', Pressed: true})
	if got := tb2.GetText(); got != "a\tb" {
		t.Errorf("с AcceptTab текст %q, ждали a<TAB>b", got)
	}
	if btn2.IsFocused() || !tb2.IsFocused() {
		t.Error("с AcceptTab фокус не должен был уйти из редактора")
	}

	// Ctrl+Tab — по-прежнему навигация.
	eng2.SendKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Mod: widget.ModCtrl, Pressed: true})
	if got := tb2.GetText(); got != "a\tb" {
		t.Errorf("Ctrl+Tab изменил текст: %q", got)
	}
}

func tbFrame(eng *engine.Engine) *image.RGBA {
	eng.RenderOnce()
	return snapshotRGBA(eng.RenderOnce())
}

// countColor — сколько пикселей кадра в окрестности цвета c.
func tbCountColor(img *image.RGBA, r image.Rectangle, c color.RGBA, tol int) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p := img.RGBAAt(x, y)
			d := func(a, b uint8) int {
				if a > b {
					return int(a - b)
				}
				return int(b - a)
			}
			if d(p.R, c.R) <= tol && d(p.G, c.G) <= tol && d(p.B, c.B) <= tol {
				n++
			}
		}
	}
	return n
}

// Цвет диапазона доходит до кадра, а текст без Styler рисуется цветом виджета.
func TestTextBox_StylerColorReachesFrame(t *testing.T) {
	green := color.RGBA{G: 255, A: 255}
	tb := widget.NewTextBox("")
	tb.SetText("highlighted text")
	eng, _ := tbScene(t, tb)

	area := image.Rect(10, 10, 310, 110)
	before := tbCountColor(tbFrame(eng), area, green, 40)

	tb.SetStyler(spanStyler(func(line int, text string) []widget.Span {
		return []widget.Span{{From: 0, To: 11, Style: widget.Style{Color: green}}}
	}))
	after := tbCountColor(tbFrame(eng), area, green, 40)
	if after <= before+20 {
		t.Fatalf("зелёных пикселей было %d, стало %d — цвет диапазона не нарисован", before, after)
	}

	// Фон диапазона — прямоугольник, а не штрихи букв.
	bg := color.RGBA{R: 200, G: 0, B: 200, A: 255}
	tb.SetStyler(spanStyler(func(line int, text string) []widget.Span {
		return []widget.Span{{From: 0, To: 4, Style: widget.Style{BG: bg}}}
	}))
	if n := tbCountColor(tbFrame(eng), area, bg, 0); n < 100 {
		t.Errorf("фон диапазона занял всего %d пикселей", n)
	}
}

// Шрифт и его начертание меняют кадр и раскладку: жирный шрифт шире обычного.
func TestTextBox_FontNameThroughEngine(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.Wrap = false
	tb.SetText("Repository Repository")
	eng, _ := tbScene(t, tb)
	plain := tbFrame(eng)
	tb.SetCaretPosition(0)
	tb.SetCaretPosition(len("Repository Repository"))
	plainEnd := tb.IMECaretRect().Min.X

	tb.FontName = widget.BuiltinFontBold
	tb.SetCaretPosition(0)
	tb.SetCaretPosition(len("Repository Repository"))
	boldEnd := tb.IMECaretRect().Min.X
	bold := tbFrame(eng)

	if boldEnd <= plainEnd {
		t.Errorf("конец строки жирным шрифтом на %d, обычным на %d — раскладка не сменила шрифт", boldEnd, plainEnd)
	}
	same := true
	for i := range plain.Pix {
		if plain.Pix[i] != bold.Pix[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("кадр не изменился после смены шрифта")
	}
}

// Табуляция на настоящем движке: текст после неё стоит правее, чем при одном
// пробеле, и ровно на табстопе.
func TestTextBox_TabStopsThroughEngine(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.Wrap = false
	tb.TabSize = 4
	tb.SetText("a\tb")
	tbScene(t, tb)

	sp := widget.MeasureUIText(" ", widget.DefaultFontSizePt)
	tb.SetCaretPosition(2)
	x := tb.IMECaretRect().Min.X - 10 - tb.PaddingX
	if want := 4 * sp; x != want {
		t.Errorf("каретка после «a»+Tab на %d, ждали табстоп %d (4 пробела по %d)", x, want, sp)
	}
}

// Полоса прокрутки на настоящем движке: у длинной строки без переноса она
// видна и ползунок едет за мышью.
func TestTextBox_HBarThroughEngine(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.Wrap = false
	long := ""
	for i := 0; i < 40; i++ {
		long += "очень длинная строка "
	}
	tb.SetText(long)
	eng, _ := tbScene(t, tb)
	img := tbFrame(eng)

	// Полоса — в нижних десяти пикселях редактора, внутри рамки: там
	// появляется то, что не фон поля.
	const barH = 10
	bar := image.Rect(12, 110-barH, 306, 109)
	if n := bar.Dx()*bar.Dy() - tbCountColor(img, bar, tb.Background, 0); n < 20 {
		t.Fatal("у длинной строки без переноса не нарисован ползунок полосы")
	}

	// Тянем ползунок вправо — текст уезжает, каретка остаётся на месте.
	tb.SetCaretPosition(0)
	y := 110 - barH + 4
	eng.SendMouseButton(30, y, widget.MouseLeft, true)
	eng.SendMouseMove(200, y)
	eng.SendMouseButton(200, y, widget.MouseLeft, false)
	moved := tbFrame(eng)
	same := true
	for i := range img.Pix {
		if img.Pix[i] != moved.Pix[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("перетаскивание ползунка не изменило кадр")
	}
	if tb.CaretPosition() != 0 {
		t.Errorf("перетаскивание ползунка сдвинуло каретку в %d", tb.CaretPosition())
	}
}
