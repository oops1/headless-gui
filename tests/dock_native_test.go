package tests

// dock_native_test.go — фаза 2 докинг-панелей: аддитивные хуки виджет-слоя, на
// которые опирается window.dockFloatHost (отрыв DockPane в нативное окно ОС).
// Живой Win32/X11 E2E (отрыв/drag/возврат) — вне юнитов; здесь проверяем
// контракт хуков в headless.

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── OnPaneAdded: вызывается в AddPane ──────────────────────────────────────

func TestDockNative_OnPaneAddedFires(t *testing.T) {
	m, _ := newDockMgr(400, 300)

	var got []*widget.DockPane
	m.OnPaneAdded = func(p *widget.DockPane) { got = append(got, p) }

	pA := newPane("a", "A")
	pB := newPane("b", "B")
	m.AddPane(pA, widget.DockLeft)
	m.AddPane(pB, widget.DockRight)

	if len(got) != 2 || got[0] != pA || got[1] != pB {
		t.Fatalf("OnPaneAdded должен вызваться на каждую AddPane в порядке; got=%v", got)
	}
}

// ─── OnFloatNative: перехватывает Float вместо виджетного floating ──────────

func TestDockNative_OnFloatNativeIntercepts(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	var calls int
	pL.OnFloatNative = func(p *widget.DockPane) {
		if p != pL {
			t.Errorf("OnFloatNative получил чужую панель: %v", p)
		}
		calls++
	}

	pL.Float()

	if calls != 1 {
		t.Fatalf("OnFloatNative вызван %d раз, ожидалось 1", calls)
	}
	if pL.State() != widget.PaneFloating {
		t.Fatalf("state = %v, want floating", pL.State())
	}
	// Панель НЕ включена в виджетный floating (не рисуется в главном холсте):
	// её bounds остаются прежними (докнутый регион), менеджер её не раскладывает.
	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Fatalf("bounds панели изменились при нативном float: %v, want докнутый (0,0,200,300)", got)
	}
}

// Без хука Float даёт обычный виджетный floating (фаза 1) — контроль фолбэка.
func TestDockNative_NoHookWidgetFloating(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	pL.Float() // OnFloatNative не задан

	if pL.State() != widget.PaneFloating {
		t.Fatalf("state = %v, want floating", pL.State())
	}
	if pL.Bounds().Empty() {
		t.Fatalf("виджетный floating должен задать непустые bounds")
	}
}

// ─── OnDragMove: титлбар двигает «окно», а не докает ────────────────────────

func TestDockNative_OnDragMoveIntercepts(t *testing.T) {
	// Панель как корень собственного движка (имитация вторичного окна).
	p := newPane("f", "Float")
	p.SetBounds(image.Rect(0, 0, 300, 200))

	var dxSum, dySum, calls int
	p.OnDragMove = func(dx, dy int) {
		dxSum += dx
		dySum += dy
		calls++
	}

	eng := engine.New(300, 200, 20)
	eng.SetRoot(p)

	// Захват за титлбар (вне кнопок) и перенос — каждый шаг шлёт дельту.
	eng.SendMouseButton(20, 10, widget.MouseLeft, true)
	eng.SendMouseMove(40, 25) // +20,+15
	eng.SendMouseMove(55, 30) // +15,+5 (dragLast не обновляется → дельта от точки захвата)
	eng.SendMouseButton(55, 30, widget.MouseLeft, false)

	if calls == 0 {
		t.Fatalf("OnDragMove не вызван ни разу")
	}
	// dragLast НЕ обновляется (координаты относительны окну): дельты считаются
	// от точки захвата (20,10). Шаги: (40,25)->(+20,+15), (55,30)->(+35,+20).
	if dxSum != 55 || dySum != 35 {
		t.Fatalf("накопленная дельта = (%d,%d), want (55,35)", dxSum, dySum)
	}
	// Панель без менеджера и в нативном режиме не докается — состояние прежнее.
	if p.State() != widget.PaneDocked {
		t.Fatalf("state = %v, want docked (drag не должен докать/флоатить)", p.State())
	}
}

// ─── OnDragMove отключает виджетный ресайз краёв ────────────────────────────

func TestDockNative_OnDragMoveDisablesEdgeResize(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Float() // виджетный floating: state = PaneFloating, есть bounds

	b := pL.Bounds()
	// Точка у левого края (зона ресайза плавающей панели).
	ex := b.Min.X + 1
	ey := (b.Min.Y + b.Max.Y) / 2

	if pL.Cursor(ex, ey) == widget.CursorArrow {
		t.Fatalf("без OnDragMove у края floating-панели ожидался resize-курсор")
	}

	pL.OnDragMove = func(dx, dy int) {}
	if pL.Cursor(ex, ey) != widget.CursorArrow {
		t.Fatalf("с OnDragMove виджетный ресайз краёв должен быть отключён (курсор Arrow)")
	}
}
