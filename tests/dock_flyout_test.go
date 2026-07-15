package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// compositeFrames собирает полное изображение холста, накладывая дельта-тайлы
// всех кадров, пока движок не «замолкнет» (нет новых кадров дольше quiet).
// Кадры on-demand приходят по мере инвалидаций; последний полный рендер задаёт
// итоговое состояние (в т.ч. призрак на финальной позиции курсора).
func compositeFrames(t *testing.T, eng *engine.Engine, w, h int) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	apply := func(f output.Frame) {
		for _, tl := range f.Tiles {
			for row := 0; row < tl.H; row++ {
				for col := 0; col < tl.W; col++ {
					si := (row*tl.W + col) * 4
					img.Set(tl.X+col, tl.Y+row, color.RGBA{
						R: tl.Data[si], G: tl.Data[si+1], B: tl.Data[si+2], A: tl.Data[si+3],
					})
				}
			}
		}
	}
	const quiet = 200 * time.Millisecond
	got := false
	timer := time.NewTimer(quiet)
	defer timer.Stop()
	deadline := time.After(4 * time.Second)
	for {
		select {
		case f := <-eng.Frames():
			apply(f)
			got = true
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quiet)
		case <-timer.C:
			if got {
				return img
			}
			timer.Reset(quiet)
		case <-deadline:
			if !got {
				t.Fatal("не получили ни одного кадра")
			}
			return img
		}
	}
}

// TestDockGhost_SnapshotFollowsCursor: во время drag'а призрак — это полупрозрачный
// СНИМОК панели, а не сплошной прямоугольник. Проверяем, что в позиции призрака
// (над центром) присутствует характерный цвет контента панели, смешанный с фоном.
func TestDockGhost_SnapshotFollowsCursor(t *testing.T) {
	blue := color.RGBA{R: 0, G: 0, B: 180, A: 255}  // центр
	green := color.RGBA{R: 0, G: 200, B: 0, A: 255} // контент панели

	m := widget.NewDockManager()
	m.SetCenter(widget.NewPanel(blue))
	m.SetBounds(image.Rect(0, 0, 400, 300))
	pL := widget.NewDockPane("l", "Left", widget.NewPanel(green))
	m.AddPane(pL, widget.DockLeft)

	eng := engine.New(400, 300, 60)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(m)
	eng.Start()
	defer eng.Stop()

	// Захват панели за титлбар (grab offset 30,10) и перенос призрака на центр.
	eng.SendMouseButton(30, 10, widget.MouseLeft, true)
	eng.SendMouseMove(150, 100)
	eng.SendMouseMove(250, 150)

	img := compositeFrames(t, eng, 400, 300)

	// Призрак: top-left (220,140), размер = размеру панели (200x300, клип по
	// холсту). Контент (зелёный) начинается ниже титлбара (+24), т.е. с y≈164.
	// Точка (250,200) — внутри контентной части призрака, над синим центром.
	px := img.RGBAAt(250, 200)
	if px.G < 100 {
		t.Fatalf("в позиции призрака нет пикселей снимка панели: %v (ждали зеленоватый блендинг)", px)
	}
	if px.B > 130 {
		t.Fatalf("призрак не полупрозрачен либо не на месте: B=%d слишком высок (%v)", px.B, px)
	}
	if px.G == green.G && px.B == 0 {
		t.Fatalf("призрак непрозрачен (не смешался с фоном): %v", px)
	}

	// Контроль: точка над центром ВНЕ призрака (y<140) — чистый синий.
	ctl := img.RGBAAt(380, 40)
	if ctl.G > 40 || ctl.B < 120 {
		t.Fatalf("контрольная точка центра не синяя: %v", ctl)
	}
}

// ─── Flyout: удержание и закрытие ───────────────────────────────────────────

// openLeftFlyout разворачивает auto-hidden левую панель кликом по её ярлыку.
func openLeftFlyout(t *testing.T, eng *engine.Engine, p *widget.DockPane) {
	t.Helper()
	eng.SendMouseButton(10, 12, widget.MouseLeft, true)
	eng.SendMouseButton(10, 12, widget.MouseLeft, false)
	if !p.IsVisible() {
		t.Fatal("flyout не выехал по клику на ярлык")
	}
}

// TestDockFlyout_HoldOverPanel: курсор, доведённый до выехавшей панели и внутри
// неё, НЕ схлопывает flyout (регресс: раньше зона удержания была только ярлыком).
func TestDockFlyout_HoldOverPanel(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)
	openLeftFlyout(t, eng, pL)

	// flyout Left: ярлык x[0,22], панель x[22,222], y[0,300].
	// Путь ярлык→панель и точки внутри панели — flyout остаётся открытым.
	for _, pt := range [][2]int{{20, 150}, {24, 150}, {120, 150}, {200, 150}, {120, 280}} {
		eng.SendMouseMove(pt[0], pt[1])
		if !pL.IsVisible() {
			t.Fatalf("flyout свернулся в точке %v внутри зоны удержания", pt)
		}
	}

	// Уход далеко за пределы union'а — сворачивается.
	eng.SendMouseMove(390, 290)
	if pL.IsVisible() {
		t.Fatal("flyout не свернулся при уходе далеко за пределы")
	}
}

// TestDockFlyout_ClickCenterDismisses: клик по центру (вне панели) сворачивает
// flyout через Dismissable-семантику (как клик мимо dropdown).
func TestDockFlyout_ClickCenterDismisses(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)
	openLeftFlyout(t, eng, pL)

	// Клик по центру (x=350, вне панели x[22,222]).
	eng.SendMouseButton(350, 150, widget.MouseLeft, true)
	eng.SendMouseButton(350, 150, widget.MouseLeft, false)
	if pL.IsVisible() {
		t.Fatal("клик по центру должен свернуть flyout (Dismissable)")
	}
	if pL.State() != widget.PaneAutoHidden {
		t.Fatalf("после dismiss панель должна остаться auto-hidden, got %v", pL.State())
	}
}

// TestDockFlyout_PinDocks: кнопка 📌 (pin) в титлбаре выехавшей панели
// возвращает её в docked-состояние и завершает flyout.
func TestDockFlyout_PinDocks(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)
	openLeftFlyout(t, eng, pL)

	// Кнопка pin — самая левая из трёх справа: pinR ≈ x[161,179], y[3,21].
	eng.SendMouseButton(170, 12, widget.MouseLeft, true)
	eng.SendMouseButton(170, 12, widget.MouseLeft, false)

	if pL.State() != widget.PaneDocked {
		t.Fatalf("после pin: state=%v, want docked", pL.State())
	}
	if !pL.IsPinned() {
		t.Fatal("после pin панель должна быть pinned")
	}
	// Docked-раскладка: панель занимает левый регион (ярлык исчез).
	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Fatalf("после pin bounds = %v, want (0,0,200,300) (docked, не flyout)", got)
	}
}

// TestDockFlyout_CloseButton: ✕ в титлбаре выехавшей панели закрывает её
// (state Closed) и завершает flyout.
func TestDockFlyout_CloseButton(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)
	openLeftFlyout(t, eng, pL)

	// closeR ≈ x[201,219], y[3,21].
	eng.SendMouseButton(210, 12, widget.MouseLeft, true)
	eng.SendMouseButton(210, 12, widget.MouseLeft, false)

	if pL.State() != widget.PaneClosed {
		t.Fatalf("после ✕: state=%v, want closed", pL.State())
	}
	if pL.IsVisible() {
		t.Fatal("закрытая панель не должна быть видима")
	}
}

// TestDockFlyout_DragGhostIsPaneNotManager: drag выехавшей (auto-hide) панели за
// титлбар даёт КОРРЕКТНЫЙ призрак — снимок самой панели (её контента), а не
// «пол-экрана» с чужими панелями. Регресс: раньше DismissAll(p) прятал панель до
// снимка, и призрак захватывал фон центра. Проверяем: (1) размер призрака ≈
// ширине панели (не менеджера); (2) в позиции призрака — контент панели
// (зелёный), а не центр (синий).
func TestDockFlyout_DragGhostIsPaneNotManager(t *testing.T) {
	blue := color.RGBA{R: 0, G: 0, B: 180, A: 255}  // центр
	green := color.RGBA{R: 0, G: 200, B: 0, A: 255} // контент панели

	m := widget.NewDockManager()
	m.SetCenter(widget.NewPanel(blue))
	m.SetBounds(image.Rect(0, 0, 400, 300))
	pL := widget.NewDockPane("l", "Left", widget.NewPanel(green))
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin() // auto-hide → ярлык у левого края

	eng := engine.New(400, 300, 60)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(m)
	eng.Start()
	defer eng.Stop()

	// Открываем flyout кликом по имени ярлыка (у верхнего края левой полоски).
	eng.SendMouseButton(10, 12, widget.MouseLeft, true)
	eng.SendMouseButton(10, 12, widget.MouseLeft, false)
	if !pL.IsVisible() {
		t.Fatal("flyout не выехал")
	}

	// Захват титлбара выехавшей панели (flyout Left: x[22,222], титлбар y[0,24]) и
	// перенос призрака на центр.
	eng.SendMouseButton(40, 10, widget.MouseLeft, true)
	eng.SendMouseMove(150, 100)
	eng.SendMouseMove(250, 150)

	img := compositeFrames(t, eng, 400, 300)

	// (1) Размер призрака ≈ панели (ширина ~200), а не менеджера (400).
	gw, gh := m.GhostSize()
	if gw <= 0 || gh <= 0 {
		t.Fatalf("призрак не захвачен: %dx%d", gw, gh)
	}
	if gw >= 320 {
		t.Fatalf("ширина призрака %d — почти как менеджер (400); ждали ≈ ширину панели (~200)", gw)
	}

	// (2) В контентной части призрака — зелёный снимок панели, а не синий центр.
	// Призрак: offX=grabDX=40-22=18, top-left=(250-18,150-10)=(232,140).
	// Точка (280,250): контент панели (ниже титлбара), над синим центром.
	px := img.RGBAAt(280, 250)
	if px.G < 100 {
		t.Fatalf("в позиции призрака нет пикселей панели: %v (ждали зелёный контент, не синий центр)", px)
	}
	if px.B > 130 {
		t.Fatalf("призрак содержит пиксели центра/чужой панели: B=%d слишком высок (%v)", px.B, px)
	}
}

// TestDockFlyout_FloatButton: кнопка float в титлбаре выехавшей панели делает
// её плавающей (state Floating) и завершает flyout.
func TestDockFlyout_FloatButton(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)
	openLeftFlyout(t, eng, pL)

	// floatR ≈ x[181,199], y[3,21].
	eng.SendMouseButton(190, 12, widget.MouseLeft, true)
	eng.SendMouseButton(190, 12, widget.MouseLeft, false)

	if pL.State() != widget.PaneFloating {
		t.Fatalf("после float: state=%v, want floating", pL.State())
	}
	if !pL.IsVisible() {
		t.Fatal("плавающая панель должна быть видима")
	}
}
