package window

import "testing"

// Окно borderless: заголовок и кнопки рисует приложение, система о них не
// знает. Пока она не знала, окно не прилипало к краям экрана и не
// раскладывалось по половинам, а наведение на кнопку «развернуть» не
// показывало макеты привязки Windows 11 — их показывают только над зоной,
// объявленной кнопкой развёртывания.

// hitStub — нативное окно, принимающее колбэк зон.
type hitStub struct {
	NativeWindow
	fn func(x, y int) HitArea
}

func (h *hitStub) SetHitTest(fn func(x, y int) HitArea) { h.fn = fn }

func TestWindow_SetHitTest(t *testing.T) {
	st := &hitStub{}
	win := &Window{}
	win.scale = 1
	win.native = st

	win.SetHitTest(func(x, y int) HitArea {
		if y < 32 {
			return HitCaption
		}
		return HitClient
	})
	if st.fn == nil {
		t.Fatal("колбэк не дошёл до бэкенда")
	}
	if got := st.fn(100, 10); got != HitCaption {
		t.Errorf("в полосе заголовка %v, ждал HitCaption", got)
	}
	if got := st.fn(100, 100); got != HitClient {
		t.Errorf("в содержимом %v, ждал HitClient", got)
	}
}

// Колбэк считает зоны в логических пикселях — в той же сетке, в которой
// приложение рисует. Перевод из физических делает окно: бэкенд масштаба не
// знает.
func TestWindow_HitTestScaled(t *testing.T) {
	st := &hitStub{}
	win := &Window{}
	win.scale = 2
	win.native = st

	var seen []int
	win.SetHitTest(func(x, y int) HitArea {
		seen = append(seen, x, y)
		return HitClient
	})
	st.fn(200, 60) // физические пиксели от системы

	if len(seen) != 2 || seen[0] != 100 || seen[1] != 30 {
		t.Errorf("колбэк получил %v, ждал логические 100 и 30", seen)
	}
}

// nil снимает колбэк: окно возвращается к прежнему поведению.
func TestWindow_HitTestCleared(t *testing.T) {
	st := &hitStub{}
	win := &Window{}
	win.scale = 1
	win.native = st

	win.SetHitTest(func(x, y int) HitArea { return HitCaption })
	win.SetHitTest(nil)
	if st.fn != nil {
		t.Error("колбэк остался после сброса")
	}
}

// Бэкенд без этой способности (X11, Wayland, macOS) вызов игнорирует — и не
// падает.
func TestWindow_HitTestUnsupportedBackend(t *testing.T) {
	win := &Window{}
	win.scale = 1
	win.native = &plainStub{}
	win.SetHitTest(func(x, y int) HitArea { return HitCaption })
}
