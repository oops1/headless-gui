package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// guideCenter — центр кнопки-направляющей стороны side для менеджера w×h
// (см. dockGuideInset=34 в widget/dockguides.go).
func guideCenter(w, h int, side widget.DockSide) (int, int) {
	const inset = 34
	cx, cy := w/2, h/2
	switch side {
	case widget.DockLeft:
		return inset, cy
	case widget.DockRight:
		return w - inset, cy
	case widget.DockTop:
		return cx, inset
	case widget.DockBottom:
		return cx, h - inset
	}
	return cx, cy
}

// ─── Дефект 1: превью дропа == фактическая вставка ───────────────────────────

// TestDockPreview_MatchesDropForAllSides — ключевой тест: для КАЖДОЙ стороны
// прямоугольник превью (PreviewRect), полученный во время drag'а над
// направляющей, СОВПАДАЕТ с bounds панели после дропа. Панель падает на пустую
// сторону (единственная docked) → совпадение точное (контракт PreviewRect).
// Якоря Left/Right заставляют Top/Bottom ложиться МЕЖДУ них (VS-порядок) — там,
// где старая формула превью (на всю ширину) расходилась с раскладкой.
func TestDockPreview_MatchesDropForAllSides(t *testing.T) {
	for _, tc := range []struct {
		name string
		side widget.DockSide
	}{
		{"left", widget.DockLeft},
		{"right", widget.DockRight},
		{"top", widget.DockTop},
		{"bottom", widget.DockBottom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newDockMgr(400, 300)
			// Якоря на всех сторонах, КРОМЕ целевой (целевая остаётся пустой →
			// панель встанет единственной, bounds == регион == превью).
			if tc.side != widget.DockLeft {
				m.AddPane(newPane("aL", "AL"), widget.DockLeft)
			}
			if tc.side != widget.DockRight {
				m.AddPane(newPane("aR", "AR"), widget.DockRight)
			}
			// Перетаскиваемая панель — плавающая (drag не трогает якоря).
			p := newPane("drag", "Drag")
			m.AddPane(p, widget.DockBottom)
			p.Float()

			eng := engine.New(400, 300, 20)
			eng.SetRoot(m)

			fb := p.Bounds()
			eng.SendMouseButton(fb.Min.X+20, fb.Min.Y+10, widget.MouseLeft, true)
			eng.SendMouseMove(200, 150)
			gx, gy := guideCenter(400, 300, tc.side)
			eng.SendMouseMove(gx, gy)

			preview := m.PreviewRect(tc.side)
			if preview.Empty() {
				t.Fatalf("%s: превью пусто во время drag'а", tc.name)
			}

			eng.SendMouseButton(gx, gy, widget.MouseLeft, false)

			if p.State() != widget.PaneDocked || p.Side() != tc.side {
				t.Fatalf("%s: после дропа state=%v side=%v, want docked/%v",
					tc.name, p.State(), p.Side(), tc.side)
			}
			if got := p.Bounds(); got != preview {
				t.Fatalf("%s: превью=%v != bounds после дропа=%v (превью должно совпадать с результатом)",
					tc.name, preview, got)
			}
		})
	}
}

// TestDockPreview_StackEqualsRegionMinusTabStrip — контракт стопки: при доке на
// сторону, где уже есть docked-панель, превью подсвечивает ПОЛНЫЙ регион, а
// bounds новой (активной) панели = регион МИНУС нижняя полоса табов.
func TestDockPreview_StackEqualsRegionMinusTabStrip(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	m.AddPane(newPane("a", "A"), widget.DockLeft) // Left уже занята
	p := newPane("p", "P")
	m.AddPane(p, widget.DockRight)
	p.Float()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	fb := p.Bounds()
	eng.SendMouseButton(fb.Min.X+20, fb.Min.Y+10, widget.MouseLeft, true)
	eng.SendMouseMove(200, 150)
	gx, gy := guideCenter(400, 300, widget.DockLeft)
	eng.SendMouseMove(gx, gy)

	preview := m.PreviewRect(widget.DockLeft)
	if preview.Empty() {
		t.Fatal("превью пусто")
	}
	eng.SendMouseButton(gx, gy, widget.MouseLeft, false)

	if p.Side() != widget.DockLeft || p.State() != widget.PaneDocked {
		t.Fatalf("после дропа side=%v state=%v, want Left/docked", p.Side(), p.State())
	}
	const tabStrip = 22 // dockTabStripHeight по умолчанию
	want := image.Rect(preview.Min.X, preview.Min.Y, preview.Max.X, preview.Max.Y-tabStrip)
	if got := p.Bounds(); got != want {
		t.Fatalf("стопка: bounds=%v, want регион-минус-табстрип=%v (превью региона=%v)", got, want, preview)
	}
}

// ─── Дефект 3: кнопки 📌 pin / ✕ close на ярлыке auto-hide ───────────────────

// TestDockStrip_PinButtonDocks — клик по 📌 на ярлыке возвращает панель в док.
func TestDockStrip_PinButtonDocks(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	pinR, _, ok := m.StripButtonRects(pL)
	if !ok {
		t.Fatal("у свёрнутой панели нет ярлыка/кнопок")
	}
	cx, cy := rectCenter(pinR)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)

	if pL.State() != widget.PaneDocked {
		t.Fatalf("после клика 📌: state=%v, want docked", pL.State())
	}
	if !pL.IsPinned() {
		t.Fatal("после 📌 панель должна быть pinned")
	}
	if got := pL.Bounds(); got != image.Rect(0, 0, 200, 300) {
		t.Fatalf("после 📌 bounds=%v, want (0,0,200,300) (docked)", got)
	}
}

// TestDockStrip_CloseButtonCloses — клик по ✕ на ярлыке закрывает панель.
func TestDockStrip_CloseButtonCloses(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	_, closeR, ok := m.StripButtonRects(pL)
	if !ok {
		t.Fatal("у свёрнутой панели нет ярлыка/кнопок")
	}
	cx, cy := rectCenter(closeR)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)

	if pL.State() != widget.PaneClosed {
		t.Fatalf("после клика ✕: state=%v, want closed", pL.State())
	}
	if pL.IsVisible() {
		t.Fatal("закрытая панель не должна быть видима")
	}
}

// TestDockStrip_NameClickTogglesFlyout — клик по ИМЕНИ ярлыка (вне кнопок)
// сохраняет прежнее поведение: открывает flyout.
func TestDockStrip_NameClickTogglesFlyout(t *testing.T) {
	m, _ := newDockMgr(400, 300)
	pL := newPane("l", "Left")
	m.AddPane(pL, widget.DockLeft)
	pL.Unpin()

	eng := engine.New(400, 300, 20)
	eng.SetRoot(m)

	pinR, _, ok := m.StripButtonRects(pL)
	if !ok {
		t.Fatal("нет ярлыка/кнопок")
	}
	// Точка имени: та же X-колонка, что и кнопки, но НАД ними (у верха ярлыка).
	cx := (pinR.Min.X + pinR.Max.X) / 2
	eng.SendMouseButton(cx, 6, widget.MouseLeft, true)
	eng.SendMouseButton(cx, 6, widget.MouseLeft, false)

	if !pL.IsVisible() {
		t.Fatal("клик по имени ярлыка должен открыть flyout (панель видима)")
	}
}
