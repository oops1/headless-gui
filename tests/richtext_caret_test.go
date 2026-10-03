package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Каретка RichText на настоящем движке: снимок кадра, события идут тем же
// путём, что от человека (фокус, нажатия клавиш).

var richCaretRed = color.RGBA{R: 255, A: 255}

// redInk — охватывающий прямоугольник красных пикселей (цвет каретки) в r.
func redInk(img *image.RGBA, r image.Rectangle) image.Rectangle {
	box := image.Rectangle{Min: image.Pt(1<<20, 1<<20), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 200 && c.G < 60 && c.B < 60 {
				box.Min.X, box.Min.Y = min(box.Min.X, x), min(box.Min.Y, y)
				box.Max.X, box.Max.Y = max(box.Max.X, x+1), max(box.Max.Y, y+1)
			}
		}
	}
	if box.Max.X < 0 {
		return image.Rectangle{}
	}
	return box
}

// frameRenderer — то, что тесту нужно от движка.
type frameRenderer interface {
	RenderOnce() *image.RGBA
	Invalidate()
}

// caretFrame рисует кадры, пока в них не появится каретка (она мигает и
// невидима половину периода — 530 мс), и возвращает охват красных пикселей
// последнего кадра. Пусто — каретки не было за все две секунды.
func caretFrame(t *testing.T, eng frameRenderer, area image.Rectangle) image.Rectangle {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		eng.Invalidate()
		frame := eng.RenderOnce()
		if frame == nil {
			t.Fatal("кадра нет")
		}
		if ink := redInk(frame, area); !ink.Empty() || time.Now().After(deadline) {
			return ink
		}
		time.Sleep(40 * time.Millisecond)
	}
}

// noCaretFor убеждается, что каретки нет в кадрах на протяжении ~1 с: дольше
// полного периода мигания, так что «не в той фазе» отговоркой быть не может.
func noCaretFor(t *testing.T, eng frameRenderer, area image.Rectangle, what string) {
	t.Helper()
	for i := 0; i < 4; i++ {
		eng.Invalidate()
		if ink := redInk(eng.RenderOnce(), area); !ink.Empty() {
			t.Fatalf("%s: нарисована каретка %v", what, ink)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func TestRichTextEngine_CaretDrawnOnlyInFocus(t *testing.T) {
	bounds := image.Rect(20, 20, 420, 120)
	eng, rt := richEngine(t, bounds,
		widget.RichParagraph{Runs: []widget.RichRun{{Text: "hello caret"}}},
		widget.RichParagraph{Runs: []widget.RichRun{{Text: "second"}}},
	)
	rt.CaretColor = richCaretRed
	rt.ShowCaret = true
	eng.RenderOnce()

	noCaretFor(t, eng, bounds, "без фокуса")

	// Фокус и три нажатия → каретка на смещении 3: черта в один пиксель
	// высотой в строку, ровно там, где её просит IMECaretRect.
	eng.SetFocus(rt)
	for i := 0; i < 3; i++ {
		eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
	}
	if rt.CaretPosition() != 3 {
		t.Fatalf("каретка %d, ждали 3", rt.CaretPosition())
	}
	want := rt.IMECaretRect()
	ink := caretFrame(t, eng, bounds)
	if ink.Empty() {
		t.Fatal("в фокусе каретки нет ни в одной из фаз мигания")
	}
	if ink.Dx() != 1 || ink.Min.X != want.Min.X {
		t.Errorf("каретка по x: %v, ждали столбец %d", ink, want.Min.X)
	}
	if ink.Min.Y != want.Min.Y || ink.Max.Y != want.Max.Y {
		t.Errorf("каретка по y: %d..%d, ждали %d..%d", ink.Min.Y, ink.Max.Y, want.Min.Y, want.Max.Y)
	}

	// Стрелка вниз переносит каретку на вторую строку — тот же столбец.
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	want2 := rt.IMECaretRect()
	if want2.Min.Y <= want.Min.Y {
		t.Fatalf("каретка не ушла вниз: %v → %v", want, want2)
	}
	ink = caretFrame(t, eng, bounds)
	if ink.Empty() || ink.Min.Y != want2.Min.Y || ink.Min.X != want2.Min.X {
		t.Errorf("после ↓ каретка %v, ждали %v", ink, want2)
	}

	// Снимаем фокус — каретка исчезает.
	eng.SetFocus(nil)
	noCaretFor(t, eng, bounds, "после потери фокуса")
}

// В режиме просмотра (ShowCaret == false) каретка не рисуется даже в фокусе,
// хотя по тексту ходит.
func TestRichTextEngine_ViewModeDrawsNoCaret(t *testing.T) {
	bounds := image.Rect(20, 20, 420, 120)
	eng, rt := richEngine(t, bounds, widget.RichParagraph{Runs: []widget.RichRun{{Text: "hello caret"}}})
	rt.CaretColor = richCaretRed
	eng.SetFocus(rt)
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
	noCaretFor(t, eng, bounds, "в режиме просмотра")
	if rt.CaretPosition() != 1 {
		t.Errorf("в режиме просмотра каретка не ходит: %d", rt.CaretPosition())
	}
}
