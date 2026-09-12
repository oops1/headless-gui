package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-58: RestoreLayout менял состояние панели напрямую и не звал
// OnStateChanged. Оторванная в нативное окно панель возвращалась в док, но
// сносит её окно как раз обёртка OnStateChanged (window/dock_host.go) — и окно
// ОС оставалось жить без содержимого чёрным прямоугольником поверх всего.

// TestDock_RestoreLayoutFiresStateChanged — возврат из плавающего состояния в
// док через RestoreLayout сообщает о смене состояния.
func TestDock_RestoreLayoutFiresStateChanged(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	p := newPane("a", "A")
	m.AddPane(p, widget.DockLeft)

	docked := m.SaveLayout() // панель в доке

	p.Float()
	if p.State() != widget.PaneFloating {
		t.Fatalf("подготовка: state=%v, want floating", p.State())
	}

	var states []widget.DockPaneState
	p.OnStateChanged = func(pp *widget.DockPane) { states = append(states, pp.State()) }

	if err := m.RestoreLayout(docked); err != nil {
		t.Fatalf("RestoreLayout: %v", err)
	}
	if len(states) != 1 || states[0] != widget.PaneDocked {
		t.Fatalf("OnStateChanged: %v, want один вызов с PaneDocked", states)
	}
	if p.State() != widget.PaneDocked {
		t.Fatalf("state после restore = %v, want docked", p.State())
	}
}

// TestDock_RestoreLayoutNoFireWhenSame — панель, чьё состояние не изменилось,
// лишнего события не получает: обработчик сноса окна принял бы его за возврат
// в док.
func TestDock_RestoreLayoutNoFireWhenSame(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	p := newPane("a", "A")
	m.AddPane(p, widget.DockLeft)
	saved := m.SaveLayout()

	n := 0
	p.OnStateChanged = func(*widget.DockPane) { n++ }
	if err := m.RestoreLayout(saved); err != nil {
		t.Fatalf("RestoreLayout: %v", err)
	}
	if n != 0 {
		t.Fatalf("OnStateChanged вызван %d раз(а) при неизменном состоянии", n)
	}
}

// TestDock_RestoreLayoutFloatsNatively — панель, СТАВШАЯ плавающей по
// восстановленной раскладке, получает нативное окно тем же хуком, что и отрыв
// мышью; у той, что плавала и до восстановления, окно уже есть — второй вызов
// создал бы дубль.
func TestDock_RestoreLayoutFloatsNatively(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	p := newPane("a", "A")
	m.AddPane(p, widget.DockLeft)

	floats := 0
	p.OnFloatNative = func(*widget.DockPane) { floats++ }

	p.Float() // отрыв мышью: хук уже сработал
	if floats != 1 {
		t.Fatalf("Float: хук вызван %d раз(а), want 1", floats)
	}
	floating := m.SaveLayout()

	p.Dock(widget.DockLeft)
	if err := m.RestoreLayout(floating); err != nil {
		t.Fatalf("RestoreLayout: %v", err)
	}
	if floats != 2 {
		t.Fatalf("после restore в плавающее: хук вызван %d раз(а), want 2", floats)
	}

	// Повторное восстановление той же раскладки: панель уже плавает — окно не
	// создаётся заново.
	if err := m.RestoreLayout(floating); err != nil {
		t.Fatalf("повторный RestoreLayout: %v", err)
	}
	if floats != 2 {
		t.Fatalf("повторный restore создал ещё окно: хук вызван %d раз(а), want 2", floats)
	}
}
