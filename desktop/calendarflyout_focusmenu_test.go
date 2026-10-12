package desktop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню выбора длительности у кнопки «▶ Фокусировка» (v3.34).

func w11RightClick(w interface {
	OnMouseButton(widget.MouseEvent) bool
}, x, y int) {
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight, Pressed: true})
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight})
}

func focusMenuTexts(w *w11Fix) (texts []string, checked []bool) {
	for _, it := range w.cal.focusMenu.Items() {
		texts = append(texts, it.Text)
		checked = append(checked, it.Checked)
	}
	return
}

// Правый щелчок по кнопке открывает меню 15/30/45/60/90 минут с отметкой на текущей;
// выбор пункта уходит в SetDuration.
func TestFocusMenu_RightClickOpensAndChooses(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	l := w.cal.w11Layout(w.cal.rect())
	x, y := pointIn(l.faction)
	w11RightClick(w.cal, x, y)
	if !w.cal.FocusMenuOpen() {
		t.Fatal("правый щелчок по «Фокусировке» не открыл меню")
	}
	texts, checked := focusMenuTexts(w)
	want := []string{"15 мин", "30 мин", "45 мин", "1 ч", "1 ч 30 мин"}
	if len(texts) != len(want) {
		t.Fatalf("пункты %v, ждали %v", texts, want)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("пункт %d: %q, ждали %q", i, texts[i], want[i])
		}
		if checked[i] != (i == 1) {
			t.Errorf("отметка пункта %q: %v (текущая длительность 30 мин)", texts[i], checked[i])
		}
	}
	// Меню над кнопкой: ниже неё панель задач.
	if r := w.cal.focusMenu.OverlayBounds(); r.Empty() || r.Max.Y > l.faction.Min.Y {
		t.Errorf("меню %v не над кнопкой %v", r, l.faction)
	}
	// Выбор мышью: середина меню — третий пункт из пяти.
	r := w.cal.focusMenu.OverlayBounds()
	cx, cy := pointIn(r)
	w.cal.focusMenu.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})
	w.cal.focusMenu.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft})
	if got := w.fs.State().Duration; got != 45*time.Minute {
		t.Errorf("длительность после выбора %v, ждали 45 мин", got)
	}
	if w.cal.FocusMenuOpen() {
		t.Error("меню осталось открытым после выбора")
	}
	if !w.cal.IsOpen() {
		t.Error("выбор пункта закрыл календарь")
	}
	if w.fs.Started != 0 {
		t.Error("выбор длительности запустил сеанс")
	}
}

// Меню с клавиатуры: клавиша меню и Shift+F10 на кнопке, стрелки и Enter, Esc
// закрывает сначала меню, потом календарь.
func TestFocusMenu_Keyboard(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	w.cal.SetFocused(true)
	key := func(code widget.KeyCode, mod widget.KeyMod) {
		w.cal.OnKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	}

	// Кнопка не в фокусе — клавиша меню ничего не делает.
	key(widget.KeyMenu, 0)
	if w.cal.FocusMenuOpen() {
		t.Fatal("меню открылось без фокуса на кнопке")
	}
	for i := 0; i < 12; i++ {
		w.cal.mu.Lock()
		on := w.cal.focusStop == calStopAction
		w.cal.mu.Unlock()
		if on {
			break
		}
		key(widget.KeyTab, 0)
	}
	key(widget.KeyF10, widget.ModShift)
	if !w.cal.FocusMenuOpen() {
		t.Fatal("Shift+F10 на кнопке не открыл меню")
	}
	// Esc закрывает только меню.
	key(widget.KeyEscape, 0)
	if w.cal.FocusMenuOpen() || !w.cal.IsOpen() {
		t.Fatalf("Esc: меню открыто=%v, календарь открыт=%v", w.cal.FocusMenuOpen(), w.cal.IsOpen())
	}
	key(widget.KeyMenu, 0)
	if !w.cal.FocusMenuOpen() {
		t.Fatal("клавиша меню не открыла меню")
	}
	// Стрелка вниз встаёт на первый пункт, Enter выбирает его.
	key(widget.KeyDown, 0)
	key(widget.KeyEnter, 0)
	if w.cal.FocusMenuOpen() {
		t.Error("Enter не закрыл меню")
	}
	if d := w.fs.State().Duration; d != 15*time.Minute {
		t.Errorf("Down+Enter выбрали %v, ждали первый пункт (15 мин)", d)
	}
	// Пока меню открыто, клавиши идут ему, а не календарю: Tab не уводит фокус.
	key(widget.KeyMenu, 0)
	key(widget.KeyTab, 0)
	w.cal.mu.Lock()
	still := w.cal.focusStop == calStopAction
	w.cal.mu.Unlock()
	if !still {
		t.Error("Tab при открытом меню сдвинул фокус календаря")
	}
	w.cal.DismissOnEscape()
	if w.cal.FocusMenuOpen() || !w.cal.IsOpen() {
		t.Error("DismissOnEscape при открытом меню должен закрыть только меню")
	}
	w.cal.DismissOnEscape()
	if w.cal.IsOpen() {
		t.Error("второй Esc не закрыл календарь")
	}
}

// Идущий сеанс, чужие места и закрытие календаря.
func TestFocusMenu_Guards(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.open()
	l := w.cal.w11Layout(w.cal.rect())

	// Правый щелчок мимо кнопки (по «+») — меню нет.
	x, y := pointIn(l.fmore)
	w11RightClick(w.cal, x, y)
	if w.cal.FocusMenuOpen() {
		t.Error("меню открылось над «+»")
	}
	// Идёт сеанс — длительность не выбирают.
	w.fs.Start()
	x, y = pointIn(w.cal.w11Layout(w.cal.rect()).faction)
	w11RightClick(w.cal, x, y)
	if w.cal.FocusMenuOpen() {
		t.Error("меню открылось у идущего сеанса")
	}
	w.fs.Stop()

	// Закрытие календаря закрывает и меню.
	w11RightClick(w.cal, x, y)
	if !w.cal.FocusMenuOpen() {
		t.Fatal("меню не открылось")
	}
	w.cal.Close()
	if w.cal.FocusMenuOpen() {
		t.Error("меню пережило закрытие календаря")
	}

	// Календарь без сеанса: открывать нечего.
	w.cal.Open(w.anchor)
	w.cal.Settle()
	w.cal.SetFocusSession(nil)
	if w.cal.OpenFocusDurationMenu() {
		t.Error("меню открылось без сеанса")
	}
}

// Одна кнопка (FocusSingleButton) меню тоже получает; свои длительности сжимаются
// пределами сеанса.
func TestFocusMenu_SingleButtonAndCustomDurations(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, false)
	w.cal.FocusSingleButton = true
	w.cal.FocusDurations = []time.Duration{2 * time.Minute, 25 * time.Minute, 10 * time.Hour}
	w.open()
	x, y := pointIn(w.cal.w11Layout(w.cal.rect()).faction)
	w11RightClick(w.cal, x, y)
	if !w.cal.FocusMenuOpen() {
		t.Fatal("при одной кнопке меню не открылось")
	}
	items := w.cal.focusMenu.Items()
	if len(items) != 3 {
		t.Fatalf("пунктов %d, ждали 3", len(items))
	}
	items[0].OnClick()
	if d := w.fs.State().Duration; d != FocusMinDuration {
		t.Errorf("2 мин сжалось до %v, ждали %v", d, FocusMinDuration)
	}
	items[2].OnClick()
	if d := w.fs.State().Duration; d != FocusMaxDuration {
		t.Errorf("10 ч сжалось до %v, ждали %v", d, FocusMaxDuration)
	}
	items[1].OnClick()
	if d := w.fs.State().Duration; d != 25*time.Minute {
		t.Errorf("25 мин: %v", d)
	}
}
