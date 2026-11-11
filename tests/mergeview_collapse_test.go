package tests

import (
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// GG-96: у MergeView сворачиваются «наше», «база» и «их», итог и верх
// разворачиваются на всю высоту, доля границы читается и задаётся, и одно
// событие OnLayoutChanged сообщает о любом изменении раскладки.

// mergePaneRects — области панелей из дерева доступности по сторонам.
func mergePaneRects(t *testing.T, mv *widget.MergeView) map[widget.MergeSide]image.Rectangle {
	t.Helper()
	out := map[widget.MergeSide]image.Rectangle{}
	sides := map[string]widget.MergeSide{"0": widget.MergeOurs, "1": widget.MergeBase, "2": widget.MergeTheirs, "3": widget.MergeResult}
	for _, c := range widget.BuildAccessTree(mv, nil).Children {
		if s, ok := sides[c.ID]; ok && c.Role == widget.RoleTextInput {
			out[s] = c.Bounds
		}
	}
	if len(out) < 3 {
		t.Fatalf("в дереве доступности %d панелей", len(out))
	}
	return out
}

func mergeClick(mv *widget.MergeView, x, y int) {
	mv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	mv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
}

// mergeFoldY — высота кнопки «свернуть» в шапке верхней панели (поле контрола
// 14 + отступ шапки 4 + половина её высоты 17).
const mergeFoldY = 14 + 4 + 17

func TestMergeView_CollapsePanes(t *testing.T) {
	testreset.Globals(t)
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	eng := mergeScene(t, mv)
	if mv.PaneCollapsed(widget.MergeOurs) || mv.PaneCollapsed(widget.MergeBase) || mv.PaneCollapsed(widget.MergeTheirs) ||
		mv.ResultMaximized() || mv.SourcesMaximized() || mv.Split() != 0.58 || !mv.ShowCollapseButtons() {
		t.Fatal("по умолчанию раскладка не исходная")
	}
	before := mergePaneRects(t, mv)
	frame0 := eng.RenderOnce()

	layout := 0
	mv.OnLayoutChanged = func() { layout++ }
	mv.SetPaneCollapsed(widget.MergeBase, true)
	if !mv.PaneCollapsed(widget.MergeBase) || layout != 1 {
		t.Fatalf("SetPaneCollapsed(Base): свёрнута %v, событий %d", mv.PaneCollapsed(widget.MergeBase), layout)
	}
	p := mergePaneRects(t, mv)
	if p[widget.MergeBase].Dx() > 40 {
		t.Fatalf("свёрнутая база шириной %d", p[widget.MergeBase].Dx())
	}
	if p[widget.MergeOurs].Dx() <= before[widget.MergeOurs].Dx()+80 {
		t.Fatalf("«наше» не заняло место базы: %d, было %d", p[widget.MergeOurs].Dx(), before[widget.MergeOurs].Dx())
	}
	if sameStrip(frame0, eng.RenderOnce(), image.Rect(0, 0, 1000, 600)) {
		t.Fatal("кадр со свёрнутой базой не отличается")
	}

	// Последнюю развёрнутую свернуть нельзя.
	mv.SetPaneCollapsed(widget.MergeOurs, true)
	mv.SetPaneCollapsed(widget.MergeTheirs, true)
	if mv.PaneCollapsed(widget.MergeTheirs) {
		t.Fatal("свернулась последняя развёрнутая верхняя панель")
	}

	// Всё развернули — раскладка и кадр прежние.
	mv.SetPaneCollapsed(widget.MergeOurs, false)
	mv.SetPaneCollapsed(widget.MergeBase, false)
	if got := mergePaneRects(t, mv); got[widget.MergeOurs] != before[widget.MergeOurs] || got[widget.MergeResult] != before[widget.MergeResult] {
		t.Fatalf("после разворачивания панели %v, было %v", got, before)
	}
	if !sameStrip(frame0, eng.RenderOnce(), image.Rect(0, 0, 1000, 600)) {
		t.Fatal("после разворачивания кадр не вернулся к прежнему")
	}
}

// У свёрнутой панели кнопки решения нет, а решение из кода работает.
func TestMergeView_CollapsedResolve(t *testing.T) {
	testreset.Globals(t)
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	eng := mergeScene(t, mv)
	eng.RenderOnce()
	mv.SetPaneCollapsed(widget.MergeOurs, true)
	eng.RenderOnce()
	for x := 0; x < 1000; x += 5 {
		for y := 60; y < 300; y += 5 {
			if mv.Cursor(x, y) == widget.CursorHand && x > 60 && x < 500 {
				// Рука над верхней частью левее «их» — только полоса свёрнутой панели.
				t.Fatalf("над свёрнутой «нашей» стороной нашлась кнопка в (%d,%d)", x, y)
			}
		}
	}
	mv.ResolveAll(widget.MergeTakeOurs)
	if mv.Unresolved() != 0 || !strings.Contains(mv.Result(), "Pad: 8") {
		t.Fatalf("ResolveAll при свёрнутой стороне: нерешённых %d, итог\n%s", mv.Unresolved(), mv.Result())
	}
}

func TestMergeView_ActiveCollapsed(t *testing.T) {
	testreset.Globals(t)
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	mergeScene(t, mv)
	mv.SetActiveSide(widget.MergeTheirs)
	mv.SetPaneCollapsed(widget.MergeTheirs, true)
	if mv.ActiveSide() != widget.MergeResult {
		t.Fatalf("свернули активную панель — активна %v, ждал итог", mv.ActiveSide())
	}
	mv.SetActiveSide(widget.MergeTheirs)
	if mv.PaneCollapsed(widget.MergeTheirs) || mv.ActiveSide() != widget.MergeTheirs {
		t.Fatalf("SetActiveSide на свёрнутую: свёрнута %v, активна %v", mv.PaneCollapsed(widget.MergeTheirs), mv.ActiveSide())
	}

	// Свернули «наше» и «их», потом спрятали базу — «наше» разворачивается.
	mv.SetPaneCollapsed(widget.MergeOurs, true)
	mv.SetPaneCollapsed(widget.MergeTheirs, true)
	mv.SetShowBase(false)
	if mv.PaneCollapsed(widget.MergeOurs) {
		t.Fatal("без базы и со свёрнутыми «наше» и «их» не осталось ни одной панели")
	}
}

func TestMergeView_Maximize(t *testing.T) {
	testreset.Globals(t)
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	eng := mergeScene(t, mv)
	eng.RenderOnce()
	before := mergePaneRects(t, mv)
	layout := 0
	mv.OnLayoutChanged = func() { layout++ }

	mv.SetResultMaximized(true)
	p := mergePaneRects(t, mv)
	if !mv.ResultMaximized() || p[widget.MergeOurs].Dy() != 0 || p[widget.MergeResult].Dy() <= before[widget.MergeResult].Dy()+150 || layout != 1 {
		t.Fatalf("итог на всю высоту: верх %d (было %d), итог %d (было %d), событий %d",
			p[widget.MergeOurs].Dy(), before[widget.MergeOurs].Dy(), p[widget.MergeResult].Dy(), before[widget.MergeResult].Dy(), layout)
	}
	if mv.ActiveSide() != widget.MergeResult {
		t.Fatalf("итог на всю высоту — активна %v", mv.ActiveSide())
	}
	eng.RenderOnce()

	mv.SetSourcesMaximized(true)
	p = mergePaneRects(t, mv)
	if mv.ResultMaximized() || !mv.SourcesMaximized() || p[widget.MergeResult].Dy() != 0 || p[widget.MergeOurs].Dy() <= before[widget.MergeOurs].Dy()+150 {
		t.Fatalf("верх на всю высоту: итог %d, верх %d (было %d)", p[widget.MergeResult].Dy(), p[widget.MergeOurs].Dy(), before[widget.MergeOurs].Dy())
	}
	eng.RenderOnce()

	mv.SetSourcesMaximized(false)
	if got := mergePaneRects(t, mv); got[widget.MergeOurs] != before[widget.MergeOurs] || got[widget.MergeResult] != before[widget.MergeResult] {
		t.Fatal("после снятия развёртывания раскладка не вернулась")
	}
}

func TestMergeView_SplitAndMouse(t *testing.T) {
	testreset.Globals(t)
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	eng := mergeScene(t, mv)
	eng.RenderOnce()
	layout := 0
	mv.OnLayoutChanged = func() { layout++ }

	mv.SetSplit(0.4)
	if mv.Split() != 0.4 || layout != 1 {
		t.Fatalf("SetSplit(0.4): доля %v, событий %d", mv.Split(), layout)
	}
	mv.SetResultMaximized(true)
	mv.SetSplit(0.5)
	if mv.ResultMaximized() || mv.Split() != 0.5 {
		t.Fatal("SetSplit не снял развёртывание итога")
	}
	mv.SetSplit(0.01)
	if mv.Split() != 0.2 {
		t.Fatalf("SetSplit(0.01) не зажался: %v", mv.Split())
	}
	mv.SetSplit(0.58)

	// Перетаскивание разделителя: одно событие на отпускание.
	p := mergePaneRects(t, mv)
	sy := p[widget.MergeOurs].Max.Y + 4
	layout = 0
	mv.OnMouseButton(widget.MouseEvent{X: 300, Y: sy, Button: widget.MouseLeft, Pressed: true})
	for y := sy; y <= sy+80; y += 10 {
		mv.OnMouseMove(300, y)
	}
	if layout != 0 {
		t.Fatalf("OnLayoutChanged во время перетаскивания: %d", layout)
	}
	mv.OnMouseButton(widget.MouseEvent{X: 300, Y: sy + 80, Button: widget.MouseLeft, Pressed: false})
	if layout != 1 || mv.Split() <= 0.65 {
		t.Fatalf("после перетаскивания доля %v, событий %d", mv.Split(), layout)
	}
	// Двойной щелчок по разделителю возвращает 0.58.
	sy = mergePaneRects(t, mv)[widget.MergeOurs].Max.Y + 4
	mergeClick(mv, 300, sy)
	mergeClick(mv, 300, sy)
	if mv.Split() != 0.58 {
		t.Fatalf("двойной щелчок по разделителю: доля %v", mv.Split())
	}

	// Кнопка «свернуть» в шапке «нашей» панели и щелчок по полосе.
	eng.RenderOnce()
	ours := mergePaneRects(t, mv)[widget.MergeOurs]
	mergeClick(mv, ours.Max.X-20, mergeFoldY)
	if !mv.PaneCollapsed(widget.MergeOurs) {
		t.Fatal("кнопка «свернуть» не свернула «наше»")
	}
	strip := mergePaneRects(t, mv)[widget.MergeOurs]
	mergeClick(mv, (strip.Min.X+strip.Max.X)/2, mergeFoldY+40)
	if mv.PaneCollapsed(widget.MergeOurs) {
		t.Fatal("щелчок по полосе не развернул «наше»")
	}

	// Кнопки в шапке итога (на 23 px выше начала его кода): «итог на всю
	// высоту», повторно — назад; «верх на всю высоту» правее.
	resBtnX := 1000 - 14 - 12 - 6 - 8 - 24 - 4 - 12
	srcBtnX := 1000 - 14 - 12 - 6 - 8 - 12
	hy := func() int { return mergePaneRects(t, mv)[widget.MergeResult].Min.Y - 23 }
	mergeClick(mv, resBtnX, hy())
	if !mv.ResultMaximized() {
		t.Fatal("кнопка в шапке итога не развернула итог")
	}
	mergeClick(mv, resBtnX, hy())
	if mv.ResultMaximized() {
		t.Fatal("повторный щелчок не вернул раскладку")
	}
	mergeClick(mv, srcBtnX, hy())
	if !mv.SourcesMaximized() {
		t.Fatal("кнопка в шапке итога не развернула верх")
	}
	mergeClick(mv, srcBtnX, hy())
	if mv.SourcesMaximized() {
		t.Fatal("повторный щелчок не вернул раскладку")
	}

	mv.SetShowCollapseButtons(false)
	mergeClick(mv, ours.Max.X-20, mergeFoldY)
	if mv.PaneCollapsed(widget.MergeOurs) {
		t.Fatal("кнопки скрыты, а щелчок по их месту свернул панель")
	}
}

func TestMergeView_CollapseXAML(t *testing.T) {
	testreset.Globals(t)
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="900" Height="500">
  <MergeView x:Name="m" CollapsedPanes="Ours, Base" Split="0.4" ResultMaximized="True" ShowCollapseButtons="False"/>
</Canvas>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	mv := reg["m"].(*widget.MergeView)
	// Split снимает развёртывание, поэтому порядок в разметке: сначала Split,
	// потом ResultMaximized — как и записано.
	if !mv.PaneCollapsed(widget.MergeOurs) || !mv.PaneCollapsed(widget.MergeBase) || mv.PaneCollapsed(widget.MergeTheirs) ||
		mv.Split() != 0.4 || !mv.ResultMaximized() || mv.ShowCollapseButtons() {
		t.Fatalf("атрибуты не применились: наше %v, база %v, их %v, доля %v, итог %v, кнопки %v",
			mv.PaneCollapsed(widget.MergeOurs), mv.PaneCollapsed(widget.MergeBase), mv.PaneCollapsed(widget.MergeTheirs),
			mv.Split(), mv.ResultMaximized(), mv.ShowCollapseButtons())
	}
}
