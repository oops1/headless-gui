package widget

import (
	"image"
	"testing"
)

// GG-79: свой минимум стороны у панели и событие о смене размера стороны.
//
// У DockManager был один MinSideSize на все стороны: поднять его под вкладки
// панели справа значило запретить и журналу снизу быть ниже того же числа.

type sideEvent struct {
	side DockSide
	size int
}

func newMinSizeMgr(t *testing.T) (*DockManager, *DockPane, *DockPane, *[]sideEvent) {
	t.Helper()
	m, _ := newDockSizeMgr()
	commit := newDockSizePane("commit", "Коммит")
	commit.MinSize = 300
	journal := newDockSizePane("journal", "Журнал")
	m.AddPane(commit, DockRight)
	m.AddPane(journal, DockBottom)
	m.SetBounds(image.Rect(0, 0, 1000, 700))
	var events []sideEvent
	m.OnSideResized = func(s DockSide, size int) { events = append(events, sideEvent{s, size}) }
	return m, commit, journal, &events
}

// Минимум панели действует только на её сторону.
func TestDockMinSize_OnlyOwnSide(t *testing.T) {
	m, commit, journal, _ := newMinSizeMgr(t)
	m.SetSideSize(DockRight, 100)
	m.SetSideSize(DockBottom, 80)
	if got := commit.Bounds().Dx(); got != 300 {
		t.Fatalf("ширина стороны с MinSize=300 — %d", got)
	}
	if got := journal.Bounds().Dy(); got != 80 {
		t.Fatalf("высота нижней стороны %d, ждал 80 — минимум чужой панели её не касается", got)
	}
}

// Разделитель упирается в минимум, событие приходит одно — по отпусканию.
func TestDockMinSize_GutterStopsAndReports(t *testing.T) {
	m, commit, _, events := newMinSizeMgr(t)
	m.SetSideSize(DockRight, 400)
	g := m.gutters[int(DockRight)]
	gx, gy := g.Min.X+g.Dx()/2, g.Min.Y+g.Dy()/2

	m.OnMouseButton(MouseEvent{X: gx, Y: gy, Button: MouseLeft, Pressed: true})
	m.OnMouseMove(gx+50, gy)
	m.OnMouseMove(gx+300, gy) // тянем далеко вправо — сузить до 100 с лишним
	if len(*events) != 0 {
		t.Fatalf("событие во время перетаскивания: %v", *events)
	}
	m.OnMouseButton(MouseEvent{X: gx + 300, Y: gy, Button: MouseLeft, Pressed: false})

	if got := commit.Bounds().Dx(); got != 300 {
		t.Fatalf("разделитель не упёрся в минимум: ширина %d", got)
	}
	if len(*events) != 1 || (*events)[0] != (sideEvent{DockRight, 300}) {
		t.Fatalf("события после перетаскивания: %v, ждал одно {Right 300}", *events)
	}

	// Нажали и отпустили, не сдвинув, — события нет.
	g = m.gutters[int(DockRight)]
	gx = g.Min.X + g.Dx()/2
	m.OnMouseButton(MouseEvent{X: gx, Y: gy, Button: MouseLeft, Pressed: true})
	m.OnMouseButton(MouseEvent{X: gx, Y: gy, Button: MouseLeft, Pressed: false})
	if len(*events) != 1 {
		t.Fatalf("событие без изменения размера: %v", *events)
	}
}

// В тесном окне минимум панели уступает документной области.
func TestDockMinSize_YieldsInTightManager(t *testing.T) {
	m, _ := newDockSizeMgr()
	p := newDockSizePane("commit", "Коммит")
	p.MinSize = 300
	m.AddPane(p, DockLeft)
	m.SetBounds(image.Rect(0, 0, 250, 300))
	if got, want := p.Bounds().Dx(), 250-dockGutterSize-dockCenterMin; got != want {
		t.Fatalf("ширина в тесном окне %d, ждал %d", got, want)
	}
}

// RestoreLayout поднимает сохранённый размер до минимума и сообщает размеры
// занятых сторон.
func TestDockMinSize_RestoreLayout(t *testing.T) {
	m, _, _, events := newMinSizeMgr(t)
	m.MinSideSize = 60
	commit := m.FindPane("commit")
	commit.MinSize = 0
	m.SetSideSize(DockRight, 120)
	saved := m.SaveLayout()

	commit.SetMinSize(300)
	*events = nil
	if err := m.RestoreLayout(saved); err != nil {
		t.Fatal(err)
	}
	if got := m.SideSize(DockRight); got != 300 {
		t.Fatalf("SideSize(Right) после восстановления %d, ждал 300", got)
	}
	want := map[DockSide]bool{DockRight: true, DockBottom: true}
	if len(*events) != 2 {
		t.Fatalf("события после RestoreLayout: %v, ждал по занятым сторонам", *events)
	}
	for _, e := range *events {
		if !want[e.side] {
			t.Fatalf("событие для незанятой стороны: %v", e)
		}
	}
}

// SetMinSize перекладывает менеджер сразу.
func TestDockMinSize_SetMinSizeRelayouts(t *testing.T) {
	m, commit, _, _ := newMinSizeMgr(t)
	commit.SetMinSize(0)
	m.SetSideSize(DockRight, 120)
	if got := commit.Bounds().Dx(); got != 120 {
		t.Fatalf("без минимума ширина %d, ждал 120", got)
	}
	commit.SetMinSize(320)
	if got := commit.Bounds().Dx(); got != 320 {
		t.Fatalf("после SetMinSize(320) ширина %d", got)
	}
}

// Разметка: <DockPane MinSize="…">.
func TestDockMinSize_XAML(t *testing.T) {
	_, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="1000" Height="700">
  <DockManager Name="dm" Left="0" Top="0" Width="1000" Height="700">
    <DockPane Id="commit" Title="Коммит" Side="Right" Size="120" MinSize="300">
      <Label Text="вкладки"/>
    </DockPane>
  </DockManager>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := reg["commit"].(*DockPane)
	if !ok {
		t.Fatalf("commit — %T", reg["commit"])
	}
	if p.MinSize != 300 {
		t.Fatalf("MinSize из разметки %d", p.MinSize)
	}
	dm := reg["dm"].(*DockManager)
	dm.SetBounds(image.Rect(0, 0, 1000, 700))
	if got := p.Bounds().Dx(); got != 300 {
		t.Fatalf("ширина стороны из разметки %d, ждал 300", got)
	}
}
