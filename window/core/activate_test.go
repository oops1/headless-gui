package core

import (
	"testing"
)

// Вывод окна на передний план (E14): общий слой без платформы. Платформенные
// части проверяются в activate_*_test.go.

// actSpy — бэкенд с активацией и свёрнутостью.
type actSpy struct {
	fakeCloseNative
	ok        bool
	activated int
	tokens    []string
	minimized bool
	onMin     func(bool)
}

func (a *actSpy) Activate() bool { a.activated++; return a.ok }
func (a *actSpy) ActivateWithToken(t string) bool {
	a.tokens = append(a.tokens, t)
	return a.ok
}
func (a *actSpy) IsMinimized() bool                   { return a.minimized }
func (a *actSpy) SetOnMinimizedChanged(fn func(bool)) { a.onMin = fn }

// Бэкенд без активации: «не умею», а не молчаливая успешность.
func TestActivate_UnsupportedBackend(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	if win.Activate() || win.ActivateWithToken("tok") {
		t.Error("бэкенд без активации ответил true")
	}
	if win.IsMinimized() {
		t.Error("IsMinimized true без поддержки")
	}
}

// До Run бэкенда нет: false, без паники.
func TestActivate_BeforeRun(t *testing.T) {
	win := New(&postEngine{}, "x")
	if win.Activate() || win.ActivateWithToken("tok") || win.IsMinimized() {
		t.Error("до Run ответ должен быть false")
	}
	win.SetOnMinimizedChanged(func(bool) {}) // не должно падать
}

// Вызовы доходят до бэкенда вместе с токеном, ответ возвращается как есть.
func TestActivate_DelegatesToBackend(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	spy := &actSpy{ok: true}
	win.native = spy
	if !win.Activate() || spy.activated != 1 {
		t.Errorf("Activate: %d вызовов бэкенда", spy.activated)
	}
	if !win.ActivateWithToken("abc_TIME42") || len(spy.tokens) != 1 || spy.tokens[0] != "abc_TIME42" {
		t.Errorf("токен до бэкенда не дошёл: %v", spy.tokens)
	}
	spy.ok = false
	if win.Activate() || win.ActivateWithToken("x") {
		t.Error("отказ бэкенда не дошёл до приложения")
	}
	spy.minimized = true
	if !win.IsMinimized() {
		t.Error("IsMinimized не прочитал состояние бэкенда")
	}
}

// Колбэк свёрнутости подписывается при создании окна и получает смены.
func TestActivate_MinimizedCallback(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	var got []bool
	win.SetOnMinimizedChanged(func(m bool) { got = append(got, m) }) // до Run
	spy := &actSpy{ok: true}
	win.native = spy
	win.setupActivate()
	if spy.onMin == nil {
		t.Fatal("бэкенду не подписан приёмник смены")
	}
	spy.onMin(true)
	spy.onMin(false)
	if len(got) != 2 || !got[0] || got[1] {
		t.Errorf("колбэк получил %v, ждал [true false]", got)
	}
}

// Метка времени из стартового идентификатора (DESKTOP_STARTUP_ID).
func TestStartupIDTime(t *testing.T) {
	for _, tc := range []struct {
		id string
		t  uint32
		ok bool
	}{
		{"sh-1234-host_TIME123456", 123456, true},
		{"x_TIME1", 1, true},
		{"a_TIME99_TIME4294967295", 4294967295, true}, // берётся последняя
		{"abc_TIME77 extra", 77, true},                // хвост после числа не мешает
		{"", 0, false},
		{"no-time-here", 0, false},
		{"id_TIME", 0, false},           // нет числа
		{"id_TIMEabc", 0, false},        // не число
		{"id_TIME0", 0, false},          // 0 — CurrentTime, не метка
		{"id_TIME4294967296", 0, false}, // не помещается в 32 бита
		{"id_TIME12345678901", 0, false},
	} {
		got, ok := startupIDTime(tc.id)
		if got != tc.t || ok != tc.ok {
			t.Errorf("startupIDTime(%q) = %d, %v; ждал %d, %v", tc.id, got, ok, tc.t, tc.ok)
		}
	}
}

// Токен забирается из окружения и стирается из него. В Wayland-сессии первой
// берётся XDG_ACTIVATION_TOKEN, в X11 — DESKTOP_STARTUP_ID; нужной нет —
// другая; обе переменные стираются всегда.
func TestTakeActivationToken(t *testing.T) {
	for _, tc := range []struct {
		name         string
		xdg, startup string
		preferXDG    bool
		want         string
	}{
		{"wayland: оба", "xdg-tok", "x11-id_TIME5", true, "xdg-tok"},
		{"x11: оба", "xdg-tok", "x11-id_TIME5", false, "x11-id_TIME5"},
		{"wayland: только startup", "", "x11-id_TIME5", true, "x11-id_TIME5"},
		{"x11: только xdg", "xdg-tok", "", false, "xdg-tok"},
		{"нет токенов", "", "", true, ""},
	} {
		env := map[string]string{}
		if tc.xdg != "" {
			env[envXDGActivationToken] = tc.xdg
		}
		if tc.startup != "" {
			env[envDesktopStartupID] = tc.startup
		}
		got := takeActivationToken(
			func(k string) string { return env[k] },
			func(k string) error { delete(env, k); return nil },
			tc.preferXDG)
		if got != tc.want {
			t.Errorf("%s: токен %q, ждал %q", tc.name, got, tc.want)
		}
		if len(env) != 0 {
			t.Errorf("%s: окружение не очищено: %v", tc.name, env)
		}
	}
}

// То же на настоящем окружении процесса: после TakeActivationToken переменных
// нет, второй вызов даёт пустую строку.
func TestTakeActivationToken_RealEnv(t *testing.T) {
	t.Setenv(envXDGActivationToken, "real-xdg")
	t.Setenv(envDesktopStartupID, "real-startup_TIME9")
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("HEADLESS_GUI_X11", "")
	if got := TakeActivationToken(); got != "real-xdg" {
		t.Errorf("токен %q, ждал real-xdg", got)
	}
	if got := TakeActivationToken(); got != "" {
		t.Errorf("повторный вызов вернул %q: токен не стёрт", got)
	}
	// Принудительный X11 меняет предпочтение.
	t.Setenv(envXDGActivationToken, "real-xdg")
	t.Setenv(envDesktopStartupID, "real-startup_TIME9")
	t.Setenv("HEADLESS_GUI_X11", "1")
	if got := TakeActivationToken(); got != "real-startup_TIME9" {
		t.Errorf("при HEADLESS_GUI_X11=1 токен %q, ждал real-startup_TIME9", got)
	}
}
