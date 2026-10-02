package tests

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Набираемое (композиция) приходит от системы в окно, а показать его должно
// поле под фокусом. До этого движок о таком вводе не знал вовсе: доходили
// только готовые символы, и набрать иероглиф было нельзя.

func imeScene(t *testing.T) (*engine.Engine, *widget.TextBox) {
	t.Helper()
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.SetBounds(image.Rect(0, 0, 400, 200))

	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(10, 10, 380, 180))
	root.AddChild(tb)

	eng := engine.New(400, 200, 30)
	eng.SetRoot(root)
	eng.SetFocus(tb)
	eng.Start()
	t.Cleanup(eng.Stop)
	return eng, tb
}

// waitText ждёт, пока в поле появится ожидаемый текст: композиция едет к
// виджету через очередь движка.
func waitText(t *testing.T, tb *widget.TextBox, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tb.GetText() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("в поле %q, ждал %q", tb.GetText(), want)
}

func TestEngineIME_CompositionReachesFocused(t *testing.T) {
	eng, tb := imeScene(t)

	eng.SendComposition("に", 1)
	waitText(t, tb, "に")

	eng.SendComposition("にほん", 3)
	waitText(t, tb, "にほん")

	eng.CommitComposition("日本")
	waitText(t, tb, "日本")
}

func TestEngineIME_Cancel(t *testing.T) {
	eng, tb := imeScene(t)

	eng.SendComposition("にほ", 2)
	waitText(t, tb, "にほ")
	eng.CancelComposition()
	waitText(t, tb, "")
}

// Над кнопкой редактор метода ввода не нужен: там печатать некуда, и
// держать его включённым значит показывать окно кандидатов впустую.
func TestEngineIME_ActiveOnlyForText(t *testing.T) {
	eng, tb := imeScene(t)

	if !eng.IMEActive() {
		t.Error("поле ввода не принимает незавершённый ввод")
	}
	if _, ok := eng.CaretRect(); !ok {
		t.Error("место каретки неизвестно — окно кандидатов встанет в углу")
	}

	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(10, 10, 100, 40))
	eng.SetFocus(btn)
	if eng.IMEActive() {
		t.Error("кнопка объявлена принимающей незавершённый ввод")
	}
	if _, ok := eng.CaretRect(); ok {
		t.Error("у кнопки нашлось место каретки")
	}
	_ = tb
}

// Место каретки уходит прямо в системный вызов, а система считает в пикселях
// экрана: на HiDPI логических координат ей мало.
func TestEngineIME_CaretRectScaled(t *testing.T) {
	eng, tb := imeScene(t)
	tb.SetCaretPosition(0)

	r1, ok := eng.CaretRect()
	if !ok {
		t.Fatal("место каретки неизвестно")
	}
	eng.SetScale(2)
	r2, ok := eng.CaretRect()
	if !ok {
		t.Fatal("после смены масштаба место каретки потерялось")
	}
	if r2.Min.X != r1.Min.X*2 || r2.Min.Y != r1.Min.Y*2 {
		t.Errorf("на масштабе 2 каретка в %v, ждал вдвое больше %v", r2, r1)
	}
}
