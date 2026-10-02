package tests

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Номер нажатия в серии (двойной, тройной щелчок) считает движок — один раз
// на всё дерево. До этого двойной щелчок высматривал каждый виджет сам: своим
// полем «время прошлого клика», своим порогом в 400 мс и своей меркой «мышь
// не сдвинулась». Тройного щелчка не умел никто, а на Windows второе нажатие
// и вовсе приходило отдельным сообщением (WM_LBUTTONDBLCLK) и выглядело как
// обычное первое.

// clickSpy — виджет, запоминающий события мыши.
type clickSpy struct {
	widget.Base
	got []widget.MouseEvent
}

func (c *clickSpy) Draw(ctx widget.DrawContext) {}

func (c *clickSpy) OnMouseButton(e widget.MouseEvent) bool {
	c.got = append(c.got, e)
	return true
}

func clickSpyEngine(t *testing.T) (*engine.Engine, *clickSpy) {
	t.Helper()
	spy := &clickSpy{}
	spy.SetBounds(image.Rect(0, 0, 200, 100))
	eng := engine.New(200, 100, 30)
	eng.SetRoot(spy)
	eng.RenderOnce()
	return eng, spy
}

func press(eng *engine.Engine, x, y int) {
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func TestClickCount_DoubleAndTriple(t *testing.T) {
	eng, spy := clickSpyEngine(t)

	press(eng, 20, 20)
	press(eng, 20, 20)
	press(eng, 21, 20) // дрожь руки серию не рвёт

	var pressed []int
	for _, e := range spy.got {
		if e.Pressed {
			pressed = append(pressed, e.Clicks)
		}
	}
	want := []int{1, 2, 3}
	if len(pressed) != len(want) {
		t.Fatalf("нажатий %d: %v", len(pressed), pressed)
	}
	for i := range want {
		if pressed[i] != want[i] {
			t.Errorf("нажатие %d получило Clicks=%d, ждал %d", i+1, pressed[i], want[i])
		}
	}
}

// Отпускание несёт номер своего нажатия: обработчик release должен видеть ту
// же серию, что и press.
func TestClickCount_ReleaseKeepsNumber(t *testing.T) {
	eng, spy := clickSpyEngine(t)

	press(eng, 20, 20)
	press(eng, 20, 20)

	if len(spy.got) != 4 {
		t.Fatalf("событий %d, ждал 4", len(spy.got))
	}
	if spy.got[3].Pressed || spy.got[3].Clicks != 2 {
		t.Errorf("отпускание двойного щелчка: Pressed=%v Clicks=%d",
			spy.got[3].Pressed, spy.got[3].Clicks)
	}
}

// Серию рвёт что угодно из трёх: далёкий курсор, другая кнопка, долгая пауза.
func TestClickCount_SeriesBreaks(t *testing.T) {
	eng, spy := clickSpyEngine(t)
	eng.SetDoubleClickTime(30 * time.Millisecond)

	press(eng, 20, 20)
	press(eng, 90, 20) // далеко
	eng.SendMouseButton(90, 20, widget.MouseRight, true)
	eng.SendMouseButton(90, 20, widget.MouseRight, false)
	press(eng, 90, 20) // другая кнопка между нажатиями тоже рвёт серию
	time.Sleep(60 * time.Millisecond)
	press(eng, 90, 20) // пауза больше интервала

	for i, e := range spy.got {
		if e.Pressed && e.Clicks != 1 {
			t.Errorf("событие %d: Clicks=%d, ждал 1 — серия должна была прерваться",
				i, e.Clicks)
		}
	}
}

// Интервал движок берёт системный — тот, который человек выставил в
// параметрах мыши; окно сообщает его при запуске.
func TestClickCount_RespectsInterval(t *testing.T) {
	eng, spy := clickSpyEngine(t)
	eng.SetDoubleClickTime(40 * time.Millisecond)

	press(eng, 20, 20)
	time.Sleep(80 * time.Millisecond)
	press(eng, 20, 20)

	for _, e := range spy.got {
		if e.Pressed && e.Clicks != 1 {
			t.Fatalf("Clicks=%d при паузе вдвое больше интервала", e.Clicks)
		}
	}
}

// ─── Виджеты ───────────────────────────────────────────────────────────────

// Тройной щелчок по многострочному полю выделяет строку. Раньше третье
// нажатие сбрасывало выделение и ставило каретку.
func TestTextBox_TripleClickSelectsLine(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 300, 120))
	tb.SetText("первая строка\nвторая строка\nтретья")

	eng := engine.New(300, 120, 30)
	eng.SetRoot(tb)
	eng.RenderOnce()
	eng.SetFocus(tb)

	// Вторая строка.
	lineH := textBoxLineHeight(tb)
	y := tb.Bounds().Min.Y + lineH + lineH/2
	press(eng, 40, y)
	press(eng, 40, y)
	press(eng, 40, y)

	if got := tb.SelectedText(); got != "вторая строка" {
		t.Errorf("выделено %q, ждал «вторая строка»", got)
	}
}

// Двойной щелчок по-прежнему выделяет слово — его и считали виджеты сами.
func TestTextBox_DoubleClickSelectsWord(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 300, 120))
	tb.SetText("первая строка")

	eng := engine.New(300, 120, 30)
	eng.SetRoot(tb)
	eng.RenderOnce()
	eng.SetFocus(tb)

	y := tb.Bounds().Min.Y + textBoxLineHeight(tb)/2
	press(eng, 10, y)
	press(eng, 10, y)

	if got := tb.SelectedText(); got != "первая" {
		t.Errorf("выделено %q, ждал «первая»", got)
	}
}

// Событие может прийти и не от движка — прямым вызовом OnMouseButton. Тогда
// Clicks равен нулю, и виджет считает серию сам, как считал раньше.
func TestTextBox_CountsWithoutEngine(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, 300, 120))
	tb.SetText("первая строка")
	tb.SetFocused(true)

	y := tb.Bounds().Min.Y + textBoxLineHeight(tb)/2
	for i := 0; i < 2; i++ {
		tb.OnMouseButton(widget.MouseEvent{X: 10, Y: y, Button: widget.MouseLeft, Pressed: true})
		tb.OnMouseButton(widget.MouseEvent{X: 10, Y: y, Button: widget.MouseLeft})
	}
	if got := tb.SelectedText(); got != "первая" {
		t.Errorf("выделено %q, ждал «первая» — счётчик виджета не сработал", got)
	}
}

// Поле в одну строку: тройной щелчок выделяет всё — «строка» здесь и есть
// всё поле.
func TestTextInput_TripleClickSelectsAll(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 300, 32))
	ti.SetText("строка целиком")

	eng := engine.New(300, 32, 30)
	eng.SetRoot(ti)
	eng.RenderOnce()
	eng.SetFocus(ti)

	y := ti.Bounds().Min.Y + ti.Bounds().Dy()/2
	press(eng, 30, y)
	press(eng, 30, y)
	press(eng, 30, y)

	// Публичного «что выделено» у поля нет — проверяем по сути: набранный
	// символ заменяет выделение, значит выделено было всё.
	eng.SendKeyEvent(widget.KeyEvent{Rune: 'я', Pressed: true})
	if got := ti.GetText(); got != "я" {
		t.Errorf("после ввода поле содержит %q, ждал «я» — выделено было не всё", got)
	}
}

// textBoxLineHeight — высота визуальной строки поля. Своего метода у TextBox
// нет, но считается она из размера шрифта тем же правилом.
func textBoxLineHeight(tb *widget.TextBox) int {
	size := tb.FontSize
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	return int(size*1.6) + 3
}
