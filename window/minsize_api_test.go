package window

import "testing"

// Минимальный размер окна из приложения — вторая половина GG-18.
//
// Бэкенды ограничение соблюдают (Win32 WM_GETMINMAXINFO, X11 WM_NORMAL_HINTS,
// Wayland xdg_toplevel.set_min_size), а задать его снаружи было нечем: минимум
// брался ТОЛЬКО из MinWidth/MinHeight корневого widget.Window и применялся
// ровно один раз внутри Run.

// minSpy — бэкенд, запоминающий последний присланный минимум.
type minSpy struct {
	NativeWindow
	w, h int
	n    int
}

func (m *minSpy) SetMinSize(width, height int) { m.w, m.h, m.n = width, height, m.n+1 }

// До Run вызов откладывается, при создании окна применяется.
func TestWindowMinSize_BeforeRunIsRemembered(t *testing.T) {
	win := &Window{}
	win.SetMinSize(800, 600)

	if w, h := win.MinSize(); w != 800 || h != 600 {
		t.Fatalf("запомнено %dx%d", w, h)
	}

	be := &minSpy{}
	win.native, win.scale = be, 1
	win.applyMinSize()

	if be.n != 1 || be.w != 800 || be.h != 600 {
		t.Errorf("бэкенду досталось %dx%d за %d вызовов", be.w, be.h, be.n)
	}
}

// Во время работы вызов уходит бэкенду сразу.
func TestWindowMinSize_AppliedAtRuntime(t *testing.T) {
	be := &minSpy{}
	win := &Window{}
	win.native, win.scale = be, 1

	win.SetMinSize(400, 300)
	if be.w != 400 || be.h != 300 {
		t.Errorf("бэкенду досталось %dx%d", be.w, be.h)
	}

	win.SetMinSize(640, 480) // минимум можно менять на лету
	if be.w != 640 || be.h != 480 {
		t.Errorf("после второго вызова %dx%d", be.w, be.h)
	}
}

// Логические пиксели умножаются на масштаб: бэкенд принимает физические.
func TestWindowMinSize_ScaledToPhysical(t *testing.T) {
	be := &minSpy{}
	win := &Window{}
	win.native, win.scale = be, 1.5

	win.SetMinSize(800, 600)
	if be.w != 1200 || be.h != 900 {
		t.Errorf("при масштабе 1.5 бэкенду досталось %dx%d, ждали 1200x900", be.w, be.h)
	}
}

// Ноль остаётся нулём: это «ограничения нет», и округление не должно
// превращать его в единицу.
func TestWindowMinSize_ZeroStaysZero(t *testing.T) {
	be := &minSpy{}
	win := &Window{}
	win.native, win.scale = be, 1.25

	win.SetMinSize(0, 600)
	if be.w != 0 {
		t.Errorf("ноль по ширине превратился в %d", be.w)
	}
	if be.h != 750 {
		t.Errorf("высота %d, ждали 750", be.h)
	}
}

// Смена DPI пересчитывает минимум: физическое значение, посчитанное однажды,
// на другом мониторе означало бы другой размер на глаз.
func TestWindowMinSize_RecomputedOnScaleChange(t *testing.T) {
	be := &minSpy{}
	win := &Window{}
	win.native, win.scale = be, 1
	win.SetMinSize(800, 600)

	win.scale = 2
	win.applyMinSize()

	if be.w != 1600 || be.h != 1200 {
		t.Errorf("после смены масштаба %dx%d, ждали 1600x1200", be.w, be.h)
	}
}

// Значения из разметки берутся, только если явного вызова не было: явный
// конкретнее.
func TestWindowMinSize_ExplicitBeatsMarkup(t *testing.T) {
	win := &Window{}
	win.SetMinSize(1024, 768)
	win.pickupWidgetMinSize(800, 600)

	if w, h := win.MinSize(); w != 1024 || h != 768 {
		t.Errorf("разметка перебила явный вызов: %dx%d", w, h)
	}

	// Без явного вызова разметка работает как прежде.
	other := &Window{}
	other.pickupWidgetMinSize(800, 600)
	if w, h := other.MinSize(); w != 800 || h != 600 {
		t.Errorf("из разметки взято %dx%d", w, h)
	}
}

// Ничего не задано — бэкенд не трогаем.
func TestWindowMinSize_NothingSetIsNoOp(t *testing.T) {
	be := &minSpy{}
	win := &Window{}
	win.native, win.scale = be, 1
	win.pickupWidgetMinSize(0, 0)
	win.applyMinSize()

	if be.n != 0 {
		t.Errorf("бэкенд получил %d вызовов без заданного минимума", be.n)
	}
}
