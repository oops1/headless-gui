package core

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// Тему ОС каждое приложение читало у себя, и каждое по-своему: кто-то брал
// SystemUsesLightTheme вместо AppsUseLightTheme, кто-то считал любое
// непонятное значение светлой темой. Ошибки здесь стоят белого окна на тёмной
// системе, поэтому разбор значений закреплён тестами на любой машине — в
// platform/shell (реестр, портал, GTK_THEME) и platform/native (String и
// нулевое значение). Здесь — отсев повторов и события окна.

// Windows на одно переключение шлёт сообщение несколько раз, портал — сигнал
// и на акцентный цвет: без отсева приложение перекрашивало бы всё дерево
// каждый раз.
func TestThemeEdge(t *testing.T) {
	var e themeEdge
	e.reset(native.SystemThemeLight)

	steps := []struct {
		in   native.SystemTheme
		want bool
	}{
		{native.SystemThemeLight, false},   // повтор прежней
		{native.SystemThemeDark, true},     // настоящая смена
		{native.SystemThemeDark, false},    // то же сообщение ещё раз
		{native.SystemThemeUnknown, false}, // портал не ответил — не смена
		{native.SystemThemeDark, false},    // и «неизвестно» не сбросило запомненную
		{native.SystemThemeLight, true},    // обратно
	}
	for i, s := range steps {
		if got := e.update(s.in); got != s.want {
			t.Errorf("шаг %d: update(%v) = %v, ждал %v", i, s.in, got, s.want)
		}
	}
}

// Если исходная тема была неизвестна, первое же определённое значение — смена:
// приложение до этого рисовало свою тему по умолчанию.
func TestThemeEdge_FromUnknown(t *testing.T) {
	var e themeEdge
	if !e.update(native.SystemThemeDark) {
		t.Error("первая известная тема после unknown не стала сменой")
	}
}

// Колбэк приложения должен ставиться в очередь движка и звать только на
// настоящую смену; nil снимает подписку.
func TestWindowSystemThemeEvent(t *testing.T) {
	eng := &postEngine{root: widget.NewWindow("T", 100, 100)}
	win := New(eng, "T")

	var got []native.SystemTheme
	// Окна ОС ещё нет: подписка отложена до Run, а колбэк уже сохранён.
	win.SetOnSystemThemeChanged(func(t native.SystemTheme) { got = append(got, t) })
	win.sysTheme.edge.reset(native.SystemThemeLight)

	win.systemThemeEvent(native.SystemThemeLight)
	win.systemThemeEvent(native.SystemThemeDark)
	win.systemThemeEvent(native.SystemThemeDark)
	win.systemThemeEvent(native.SystemThemeUnknown)
	win.systemThemeEvent(native.SystemThemeLight)

	if len(got) != 2 || got[0] != native.SystemThemeDark || got[1] != native.SystemThemeLight {
		t.Errorf("колбэк получил %v, ждал [dark light]", got)
	}
	if eng.posts != 2 {
		t.Errorf("через Post ушло %d вызовов, ждал 2: колбэк обязан идти горутиной движка", eng.posts)
	}

	win.SetOnSystemThemeChanged(nil)
	win.systemThemeEvent(native.SystemThemeDark)
	if len(got) != 2 {
		t.Errorf("колбэк снят, но вызван: %v", got)
	}
}

// Закрытие окна без подписки и повторное снятие подписки не должны падать.
func TestWindowStopSystemThemeWatch_Idempotent(t *testing.T) {
	win := New(&postEngine{root: widget.NewWindow("T", 100, 100)}, "T")
	win.stopSystemThemeWatch()
	win.stopSystemThemeWatch()
}
