package widget

// docksize_test.go — регресс на GG-1: желаемый Size стороны DockManager,
// заданный ДО первой раскладки (как это делает XAML-загрузчик — см.
// buildXAMLDockManager в xaml_containers.go, который вызывает SetSideSize
// сразу после AddPane, а SetBounds самого DockManager может к этому моменту
// ещё нести пустые границы, если реальный размер выставляет только
// engine.SetRoot), должен дожить до первой раскладки, а не схлопнуться до
// MinSideSize. См. также clampSideSize/SetSideSize в dockmanager.go.

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"
)

var docksizeTestColor = color.RGBA{R: 40, G: 40, B: 44, A: 255}

func newDockSizeMgr() (*DockManager, *Panel) {
	m := NewDockManager()
	center := NewPanel(docksizeTestColor)
	m.SetCenter(center)
	return m, center
}

func newDockSizePane(id, title string) *DockPane {
	return NewDockPane(id, title, NewPanel(docksizeTestColor))
}

// TestDockSideSize_DesiredSurvivesFirstLayout — размер, заданный SetSideSize
// ДО того, как менеджер узнал свои границы (типичная точка входа —
// XAML: <DockPane Size="260"> вызывает SetSideSize раньше engine.SetRoot),
// должен применяться к первой же реальной раскладке, а не обрезаться до
// MinSideSize из-за клэмпа "от нулевых границ".
func TestDockSideSize_DesiredSurvivesFirstLayout(t *testing.T) {
	m, _ := newDockSizeMgr()
	pL := newDockSizePane("l", "Left")
	m.AddPane(pL, DockLeft) // layout() дергается уже здесь, но границы ещё пусты

	// Желаемый размер — до первого SetBounds (границы менеджера пока (0,0,0,0)).
	m.SetSideSize(DockLeft, 260)

	// Первая реальная раскладка — как engine.SetRoot.
	m.SetBounds(image.Rect(0, 0, 600, 400))

	if got := pL.Bounds().Dx(); got != 260 {
		t.Fatalf("ширина Left после первой раскладки = %d, want 260 (желаемый Size не должен схлопнуться до MinSideSize)", got)
	}
	if got := m.SideSize(DockLeft); got != 260 {
		t.Fatalf("SideSize(Left) = %d, want 260", got)
	}
}

// TestDockSideSize_MultiplePanesTakeMax — несколько панелей на одной стороне,
// каждая со своим Size (в XAML это несколько <DockPane Side="Left" Size="...">,
// каждая дёргает SetSideSize), должны дать МАКСИМУМ из заданных размеров, а не
// последнее по порядку значение — порядок панелей в разметке не должен решать
// итоговую ширину стороны.
func TestDockSideSize_MultiplePanesTakeMax(t *testing.T) {
	t.Run("больший размер задан вторым", func(t *testing.T) {
		m, _ := newDockSizeMgr()
		pA := newDockSizePane("a", "A")
		pB := newDockSizePane("b", "B")
		m.AddPane(pA, DockLeft)
		m.AddPane(pB, DockLeft) // стопка на той же стороне

		m.SetSideSize(DockLeft, 150) // как будто у A: Size="150"
		m.SetSideSize(DockLeft, 220) // как будто у B: Size="220"

		m.SetBounds(image.Rect(0, 0, 600, 400))

		if got := m.SideSize(DockLeft); got != 220 {
			t.Fatalf("SideSize(Left) = %d, want max(150,220)=220", got)
		}
		// Активная панель стопки (последняя добавленная — см. AddPane) получает
		// всю ширину региона.
		if got := pB.Bounds().Dx(); got != 220 {
			t.Fatalf("ширина активной панели стопки = %d, want 220", got)
		}
	})

	t.Run("больший размер задан первым", func(t *testing.T) {
		m, _ := newDockSizeMgr()
		pA := newDockSizePane("a", "A")
		pB := newDockSizePane("b", "B")
		m.AddPane(pA, DockLeft)
		m.AddPane(pB, DockLeft)

		m.SetSideSize(DockLeft, 220) // как будто у A: Size="220"
		m.SetSideSize(DockLeft, 150) // как будто у B: Size="150"

		m.SetBounds(image.Rect(0, 0, 600, 400))

		if got := pB.Bounds().Dx(); got != 220 {
			t.Fatalf("ширина активной панели стопки = %d, want max(220,150)=220", got)
		}
		if got := m.SideSize(DockLeft); got != 220 {
			t.Fatalf("SideSize(Left) = %d, want max(220,150)=220 (порядок панелей не должен влиять)", got)
		}
	})
}

// TestDockSideSize_StillClampedPastAvailable — желаемый размер, ощутимо
// превышающий доступное место менеджера, всё равно ограничивается при первой
// реальной раскладке. Фиксируем ТЕКУЩИЙ предел: ext - gutterSize - dockCenterMin
// (а не «половина менеджера» — так и было устроено в clampSideSizeFor до этой
// правки, менять формулу задача не просит).
func TestDockSideSize_StillClampedPastAvailable(t *testing.T) {
	m, _ := newDockSizeMgr()
	pL := newDockSizePane("l", "Left")
	m.AddPane(pL, DockLeft)

	m.SetSideSize(DockLeft, 1000) // явно больше половины и вообще всего менеджера

	m.SetBounds(image.Rect(0, 0, 300, 300))

	// gutterSize=6 (дефолт), dockCenterMin=40 → maxS = 300-6-40 = 254.
	const wantClamped = 300 - dockGutterSize - dockCenterMin
	if got := pL.Bounds().Dx(); got != wantClamped {
		t.Fatalf("ширина Left = %d, want %d (клэмп по доступному месту менеджера)", got, wantClamped)
	}
	// Желаемый размер (как и при простом сжатии менеджера, см.
	// TestDock_ManagerResizePreservesSideSize в tests/dock_test.go) остаётся
	// в SideSize непорезанным — это persisted-желание, а не текущая ширина.
	if got := m.SideSize(DockLeft); got != 1000 {
		t.Fatalf("SideSize(Left) = %d, want 1000 (желаемый размер не перезаписывается клэмпом раскладки)", got)
	}
}

// TestDockSideSize_SetAfterLayoutStillOverwrites — регресс: когда границы
// менеджера УЖЕ известны (обычная работа приложения после первой раскладки),
// SetSideSize остаётся обычным сеттером — последний вызов побеждает, даже
// если он МЕНЬШЕ предыдущего. Иначе явное уменьшение стороны (например, из
// RestoreLayout или ручного кода приложения) было бы невозможно.
func TestDockSideSize_SetAfterLayoutStillOverwrites(t *testing.T) {
	m, _ := newDockSizeMgr()
	pL := newDockSizePane("l", "Left")
	m.AddPane(pL, DockLeft)
	m.SetBounds(image.Rect(0, 0, 600, 400)) // границы уже известны

	m.SetSideSize(DockLeft, 300)
	if got := pL.Bounds().Dx(); got != 300 {
		t.Fatalf("ширина Left после SetSideSize(300) = %d, want 300", got)
	}

	m.SetSideSize(DockLeft, 150) // явное уменьшение — должно сработать как раньше
	if got := pL.Bounds().Dx(); got != 150 {
		t.Fatalf("ширина Left после SetSideSize(150) = %d, want 150 (последний вызов побеждает, без max с прошлым 300)", got)
	}
	if got := m.SideSize(DockLeft); got != 150 {
		t.Fatalf("SideSize(Left) = %d, want 150", got)
	}
}

// TestDockSideSize_RestoreLayoutOverridesPreLayoutDesired — желаемый размер,
// пришедший ДО первой раскладки (как в TestDockSideSize_DesiredSurvivesFirstLayout),
// не должен мешать последующему RestoreLayout: сохранённая раскладка,
// восстановленная ПОСЛЕ первой реальной раскладки, обязана победить.
func TestDockSideSize_RestoreLayoutOverridesPreLayoutDesired(t *testing.T) {
	m, _ := newDockSizeMgr()
	pL := newDockSizePane("l", "Left")
	m.AddPane(pL, DockLeft)

	m.SetSideSize(DockLeft, 260) // «желаемый из XAML», до первой раскладки
	m.SetBounds(image.Rect(0, 0, 600, 400))
	if got := pL.Bounds().Dx(); got != 260 {
		t.Fatalf("ширина Left до restore = %d, want 260", got)
	}

	saved := m.SaveLayout()
	var dl map[string]interface{}
	if err := json.Unmarshal(saved, &dl); err != nil {
		t.Fatalf("json.Unmarshal(saved): %v", err)
	}
	sizes, ok := dl["sizes"].([]interface{})
	if !ok || len(sizes) != 4 {
		t.Fatalf("неожиданный формат sizes в SaveLayout: %#v", dl["sizes"])
	}
	sizes[0] = float64(150) // sizes[0] = Left (см. шапку dockmanager.go)
	mutated, err := json.Marshal(dl)
	if err != nil {
		t.Fatalf("json.Marshal(mutated): %v", err)
	}

	if err := m.RestoreLayout(mutated); err != nil {
		t.Fatalf("RestoreLayout: %v", err)
	}
	if got := m.SideSize(DockLeft); got != 150 {
		t.Fatalf("SideSize(Left) после RestoreLayout = %d, want 150 (восстановленный размер не должен затираться желаемым из XAML)", got)
	}
	if got := pL.Bounds().Dx(); got != 150 {
		t.Fatalf("ширина Left после RestoreLayout = %d, want 150", got)
	}
}
