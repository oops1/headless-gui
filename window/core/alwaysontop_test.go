package core

import "testing"

type aotNative struct {
	NativeWindow
	on    bool
	calls []bool
}

func (a *aotNative) SetAlwaysOnTop(on bool) bool { a.on = on; a.calls = append(a.calls, on); return true }
func (a *aotNative) IsAlwaysOnTop() bool        { return a.on }

// Просьба «поверх всех», сделанная до Run, применяется при создании окна;
// после — идёт в бэкенд сразу. Бэкенд без поддержки — false.
func TestAlwaysOnTop_BeforeRunAndAfter(t *testing.T) {
	win := &Window{}
	if !win.SetAlwaysOnTop(true) {
		t.Skip("на этой ОС режим не поддерживается")
	}
	be := &aotNative{}
	win.native = be
	win.setupAlwaysOnTop()
	if len(be.calls) != 1 || !be.calls[0] || !win.IsAlwaysOnTop() {
		t.Fatalf("просьба до Run: вызовы %v", be.calls)
	}
	win.setupAlwaysOnTop() // применяется один раз
	if len(be.calls) != 1 {
		t.Fatalf("повторное применение: %v", be.calls)
	}
	win.SetAlwaysOnTop(false)
	if win.IsAlwaysOnTop() {
		t.Fatal("после SetAlwaysOnTop(false) окно поверх всех")
	}
	plain := &Window{}
	plain.native = struct{ NativeWindow }{}
	if plain.SetAlwaysOnTop(true) || plain.IsAlwaysOnTop() {
		t.Error("бэкенд без поддержки ответил true")
	}
}
