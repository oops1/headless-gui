package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Alt не доходил до приложения никак: WM_SYSKEYDOWN не обрабатывался, ModAlt
// не выставлялся, F10 терялся. А главное — не было способа узнать жест «Alt
// нажали и отпустили впустую», которым в Windows открывают строку меню.
//
// Распознаётся он в surface, одинаково для всех трёх бэкендов, поэтому и
// проверяется здесь, без окна ОС.

// altRecorder — движок-заглушка: запоминает клавиатурные события.
//
// Post не реализует намеренно: без него surface доставляет события прямо, и
// тест читает их сразу, не гоняя очередь.
type altRecorder struct {
	keys []widget.KeyEvent
}

func (r *altRecorder) Frames() <-chan output.Frame                                    { return nil }
func (r *altRecorder) CanvasSize() (int, int)                                         { return 0, 0 }
func (r *altRecorder) Root() widget.Widget                                            { return nil }
func (r *altRecorder) SendMouseMove(x, y int)                                         {}
func (r *altRecorder) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {}
func (r *altRecorder) CursorAt(x, y int) widget.Cursor                                { return widget.CursorArrow }
func (r *altRecorder) SendKeyEvent(e widget.KeyEvent)                                 { r.keys = append(r.keys, e) }

// newAltSurface — surface с записывающим движком.
func newAltSurface() (*surface, *altRecorder) {
	rec := &altRecorder{}
	return &surface{eng: rec}, rec
}

func TestAltTap_OpensMenu(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEvent(VK_ALT, true)
	if len(rec.keys) != 0 {
		t.Fatalf("удержание Alt само по себе дало событие: %+v", rec.keys)
	}
	s.keyEvent(VK_ALT, false)

	if len(rec.keys) != 2 {
		t.Fatalf("событий %d, ждал нажатие и отпускание KeyAlt: %+v", len(rec.keys), rec.keys)
	}
	if rec.keys[0].Code != widget.KeyAlt || !rec.keys[0].Pressed {
		t.Errorf("первое событие %+v, ждал нажатие KeyAlt", rec.keys[0])
	}
	if rec.keys[1].Code != widget.KeyAlt || rec.keys[1].Pressed {
		t.Errorf("второе событие %+v, ждал отпускание KeyAlt", rec.keys[1])
	}
}

// Alt+буква — это сочетание, а не жест меню: KeyAlt приходить не должен.
func TestAltTap_NotAfterCombination(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEvent(VK_ALT, true)
	s.keyEvent(VK_F, true) // Alt+F
	s.keyEvent(VK_F, false)
	s.keyEvent(VK_ALT, false)

	for _, e := range rec.keys {
		if e.Code == widget.KeyAlt {
			t.Fatalf("после сочетания пришёл KeyAlt: %+v", rec.keys)
		}
	}
	// Сама буква пришла, и с пометкой Alt.
	found := false
	for _, e := range rec.keys {
		if e.Code == widget.KeyF && e.Pressed {
			found = true
			if e.Mod&widget.ModAlt == 0 {
				t.Error("Alt+F пришла без ModAlt")
			}
		}
	}
	if !found {
		t.Errorf("буква F не дошла: %+v", rec.keys)
	}
}

// Автоповтор удерживаемого Alt жест не отменяет и не множит: человек просто
// держит клавишу.
func TestAltTap_SurvivesAutoRepeat(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEventRepeat(VK_ALT, true, false)
	s.keyEventRepeat(VK_ALT, true, true) // автоповтор
	s.keyEventRepeat(VK_ALT, true, true)
	s.keyEvent(VK_ALT, false)

	alts := 0
	for _, e := range rec.keys {
		if e.Code == widget.KeyAlt && e.Pressed {
			alts++
		}
	}
	if alts != 1 {
		t.Errorf("жест Alt пришёл %d раз, ждал один: %+v", alts, rec.keys)
	}
}

// Второй Alt подряд снова открывает меню: жест не одноразовый.
func TestAltTap_Repeatable(t *testing.T) {
	s, rec := newAltSurface()

	for i := 0; i < 2; i++ {
		s.keyEvent(VK_ALT, true)
		s.keyEvent(VK_ALT, false)
	}
	alts := 0
	for _, e := range rec.keys {
		if e.Code == widget.KeyAlt && e.Pressed {
			alts++
		}
	}
	if alts != 2 {
		t.Errorf("жест Alt пришёл %d раз, ждал два", alts)
	}
}

// F10 — обычная клавиша с собственным кодом: приложение открывает по нему
// меню само.
func TestF10_ReachesApplication(t *testing.T) {
	s, rec := newAltSurface()

	s.keyEvent(VK_F10, true)
	if len(rec.keys) == 0 || rec.keys[0].Code != widget.KeyF10 {
		t.Fatalf("F10 не дошла: %+v", rec.keys)
	}
}
