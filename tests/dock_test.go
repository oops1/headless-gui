package tests

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// dockPanelColor — нейтральный цвет для панелей-заглушек содержимого.
var dockPanelColor = color.RGBA{R: 40, G: 40, B: 44, A: 255}

// newDockMgr собирает DockManager заданного размера с документным центром.
func newDockMgr(w, h int) (*widget.DockManager, *widget.Panel) {
	m := widget.NewDockManager()
	center := widget.NewPanel(dockPanelColor)
	m.SetCenter(center)
	m.SetBounds(image.Rect(0, 0, w, h))
	return m, center
}

func newPane(id, title string) *widget.DockPane {
	return widget.NewDockPane(id, title, widget.NewPanel(dockPanelColor))
}

// ─── Раскладка сторон + центра ──────────────────────────────────────────────

func TestDock_LayoutSidesAndCenter(t *testing.T) {
	m, center := newDockMgr(400, 300)

	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	// Left полной высоты, размер по умолчанию 200, кромка 6 → центр справа.
	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Fatalf("Left pane bounds = %v, want (0,0,200,300)", got)
	}
	if got := center.Bounds(); got != image.Rect(206, 0, 400, 300) {
		t.Fatalf("center bounds = %v, want (206,0,400,300)", got)
	}

	// Добавляем Bottom: он ложится между левым регионом и правым краем.
	pB := newPane("b", "Bottom")
	m.AddPane(pB, widget.DockBottom)

	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Errorf("Left pane after Bottom = %v, want полной высоты (0,0,200,300)", got)
	}
	if got := pB.Bounds(); got != image.Rect(206, 100, 400, 300) {
		t.Errorf("Bottom pane bounds = %v, want (206,100,400,300)", got)
	}
	if got := center.Bounds(); got != image.Rect(206, 0, 400, 94) {
		t.Errorf("center bounds after Bottom = %v, want (206,0,400,94)", got)
	}
}

// ─── Ресайз кромкой с MinSize ───────────────────────────────────────────────

func TestDock_GutterResizeWithMin(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	// Кромка Left при size=200 находится в x∈[200,206]. Тянем вправо до 300.
	eng.SendMouseButton(203, 150, widget.MouseLeft, true)
	eng.SendMouseMove(300, 150)
	eng.SendMouseButton(300, 150, widget.MouseLeft, false)
	if got := pL.Bounds().Dx(); got != 300 {
		t.Fatalf("Left width after resize = %d, want 300", got)
	}
	if got := m.SideSize(widget.DockLeft); got != 300 {
		t.Fatalf("SideSize(Left) = %d, want 300", got)
	}

	// Тянем далеко влево — клэмп по MinSideSize (60). Кромка теперь x∈[300,306].
	eng.SendMouseButton(303, 150, widget.MouseLeft, true)
	eng.SendMouseMove(5, 150)
	eng.SendMouseButton(5, 150, widget.MouseLeft, false)
	if got := pL.Bounds().Dx(); got != 60 {
		t.Fatalf("Left width clamped = %d, want 60 (MinSideSize)", got)
	}
}

// ─── Drag панели на направляющую → смена стороны ────────────────────────────

func TestDock_DragToGuideChangesSide(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	// Захват за титлбар (вне кнопок) и перенос на правую направляющую (~366,150).
	eng.SendMouseButton(20, 10, widget.MouseLeft, true)
	eng.SendMouseMove(200, 150)
	eng.SendMouseMove(366, 150)
	eng.SendMouseButton(366, 150, widget.MouseLeft, false)

	if pL.State() != widget.PaneDocked {
		t.Fatalf("state = %v, want docked", pL.State())
	}
	if pL.Side() != widget.DockRight {
		t.Fatalf("side = %v, want Right(3)", pL.Side())
	}
	// Панель должна оказаться в правом регионе (правый край = край менеджера).
	if got := pL.Bounds().Max.X; got != 400 {
		t.Errorf("docked-right pane Max.X = %d, want 400", got)
	}
}

// ─── Drop мимо → floating, drag floating, возврат доком ─────────────────────

func TestDock_DropFloatingThenRedock(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	// Тащим в центр (мимо направляющих) → floating.
	eng.SendMouseButton(20, 10, widget.MouseLeft, true)
	eng.SendMouseMove(300, 150)
	eng.SendMouseButton(300, 150, widget.MouseLeft, false)
	if pL.State() != widget.PaneFloating {
		t.Fatalf("state = %v, want floating", pL.State())
	}
	fb := pL.Bounds()
	if fb.Empty() {
		t.Fatalf("floating bounds пусты")
	}

	// Тащим плавающую панель за титлбар на ЛЕВУЮ направляющую (~34,150) → dock.
	tbx := fb.Min.X + 20
	tby := fb.Min.Y + 10
	eng.SendMouseButton(tbx, tby, widget.MouseLeft, true)
	eng.SendMouseMove(120, 150)
	eng.SendMouseMove(34, 150)
	eng.SendMouseButton(34, 150, widget.MouseLeft, false)

	if pL.State() != widget.PaneDocked {
		t.Fatalf("state после возврата = %v, want docked", pL.State())
	}
	if pL.Side() != widget.DockLeft {
		t.Fatalf("side после возврата = %v, want Left(0)", pL.Side())
	}
}

// ─── Стопка из двух панелей: переключение табов ─────────────────────────────

func TestDock_StackTabSwitch(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pA := newPane("a", "A")
	pB := newPane("b", "B")
	m.AddPane(pA, widget.DockLeft)
	m.AddPane(pB, widget.DockLeft)

	// Последняя добавленная — активна.
	if !pB.IsVisible() || pA.IsVisible() {
		t.Fatalf("после стека активна должна быть B: A.vis=%v B.vis=%v", pA.IsVisible(), pB.IsVisible())
	}

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	// Кликаем таб A (нижняя полоса региона Left, первый слот).
	eng.SendMouseButton(10, 289, widget.MouseLeft, true)
	eng.SendMouseButton(10, 289, widget.MouseLeft, false)

	if !pA.IsVisible() || pB.IsVisible() {
		t.Fatalf("после клика по табу A активна должна быть A: A.vis=%v B.vis=%v", pA.IsVisible(), pB.IsVisible())
	}
}

// ─── Pin/Unpin + выезд по клику на ярлык ────────────────────────────────────

func TestDock_UnpinFlyoutDismiss(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	pL.Unpin()
	if pL.State() != widget.PaneAutoHidden {
		t.Fatalf("state = %v, want autohidden", pL.State())
	}
	if pL.IsPinned() {
		t.Fatalf("IsPinned = true, want false после Unpin")
	}
	if pL.IsVisible() {
		t.Fatalf("свёрнутая панель не должна быть видима до выезда")
	}

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	// Клик по ярлыку на левой полоске (у самого края) — панель выезжает.
	eng.SendMouseButton(10, 12, widget.MouseLeft, true)
	eng.SendMouseButton(10, 12, widget.MouseLeft, false)
	if !pL.IsVisible() {
		t.Fatalf("после клика по ярлыку панель должна выехать (быть видимой)")
	}

	// Уводим мышь далеко от flyout — панель прячется.
	eng.SendMouseMove(390, 290)
	if pL.IsVisible() {
		t.Fatalf("после ухода мыши flyout должен спрятаться")
	}

	// Возврат в приколотое состояние.
	pL.Pin()
	if pL.State() != widget.PaneDocked || !pL.IsPinned() {
		t.Fatalf("после Pin: state=%v pinned=%v", pL.State(), pL.IsPinned())
	}
	if !pL.IsVisible() {
		t.Fatalf("после Pin панель должна быть видима")
	}
}

// ─── Close / Show ───────────────────────────────────────────────────────────

func TestDock_CloseShow(t *testing.T) {
	m, center := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)

	pL.Close()
	if pL.State() != widget.PaneClosed || pL.IsVisible() {
		t.Fatalf("после Close: state=%v vis=%v", pL.State(), pL.IsVisible())
	}
	// Центр забирает всё пространство.
	if got := center.Bounds(); got != image.Rect(0, 0, 400, 300) {
		t.Fatalf("center после Close = %v, want весь холст", got)
	}

	pL.Show()
	if pL.State() != widget.PaneDocked || pL.Side() != widget.DockLeft {
		t.Fatalf("после Show: state=%v side=%v", pL.State(), pL.Side())
	}
	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Fatalf("после Show bounds = %v, want (0,0,200,300)", got)
	}
}

// ─── Save / Restore (в т.ч. незнакомый id) ──────────────────────────────────

func TestDock_SaveRestore(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pA := newPane("a", "A")
	pB := newPane("b", "B")
	m.AddPane(pA, widget.DockLeft)
	m.AddPane(pB, widget.DockRight)
	m.SetSideSize(widget.DockLeft, 150)
	pB.Float()

	saved := m.SaveLayout()

	// Меняем раскладку.
	pA.Close()
	m.SetSideSize(widget.DockLeft, 200)
	pB.Dock(widget.DockLeft)

	if err := m.RestoreLayout(saved); err != nil {
		t.Fatalf("RestoreLayout error: %v", err)
	}
	if pA.State() != widget.PaneDocked || pA.Side() != widget.DockLeft {
		t.Fatalf("A после restore: state=%v side=%v, want docked/Left", pA.State(), pA.Side())
	}
	if pB.State() != widget.PaneFloating {
		t.Fatalf("B после restore: state=%v, want floating", pB.State())
	}
	if got := m.SideSize(widget.DockLeft); got != 150 {
		t.Fatalf("SideSize(Left) после restore = %d, want 150", got)
	}

	// Незнакомый id в раскладке — игнорируется, известные применяются, без паники.
	var dl map[string]interface{}
	_ = json.Unmarshal(saved, &dl)
	dl["panes"] = append(dl["panes"].([]interface{}), map[string]interface{}{
		"id": "ghost", "state": 0, "side": 1, "active": true, "float": []int{0, 0, 0, 0},
	})
	mutated, _ := json.Marshal(dl)
	if err := m.RestoreLayout(mutated); err != nil {
		t.Fatalf("RestoreLayout(с ghost) error: %v", err)
	}
	if m.FindPane("ghost") != nil {
		t.Fatalf("незнакомый id ghost не должен появиться")
	}
	if pA.State() != widget.PaneDocked {
		t.Fatalf("A после restore с ghost: state=%v, want docked", pA.State())
	}
}

// ─── Ресайз DockManager сохраняет размеры сторон ────────────────────────────

func TestDock_ManagerResizePreservesSideSize(t *testing.T) {
	m, center := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	m.SetSideSize(widget.DockLeft, 180)

	if got := pL.Bounds().Dx(); got != 180 {
		t.Fatalf("Left width @400 = %d, want 180", got)
	}

	// Сжимаем менеджер — регион клэмпится, но сохранённый размер стороны не теряется.
	m.SetBounds(image.Rect(0, 0, 140, 300))
	if got := center.Bounds().Dx(); got < 40 {
		t.Fatalf("центр схлопнулся: width=%d, want >= 40", got)
	}
	if got := pL.Bounds().Dx(); got > 140 {
		t.Fatalf("панель шире менеджера: width=%d", got)
	}
	if got := m.SideSize(widget.DockLeft); got != 180 {
		t.Fatalf("сохранённый SideSize(Left) = %d, want 180 (не теряется)", got)
	}

	// Возвращаем размер — сторона восстанавливает исходную ширину.
	m.SetBounds(image.Rect(0, 0, 400, 300))
	if got := pL.Bounds().Dx(); got != 180 {
		t.Fatalf("Left width после восстановления = %d, want 180", got)
	}
}
