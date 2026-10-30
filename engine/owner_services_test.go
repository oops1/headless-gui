package engine

// owner_services_test.go — службы владельца дерева (ADR-0008, приёмка 5.2):
// виджет меряет текст шрифтами и масштабом движка своего дерева (окно на
// мониторе с другим DPI), движок вне экрана — тоже; объявление переноса
// доходит до движка дерева и до наблюдателей, но не до другого движка;
// анимация виджета шагает циклом его движка; <text> в SVG — шрифтами движка
// дерева.

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

const svcText = "Пример строки для замера"

// svcScene — движок с корнем и лейблом; dpi > 0 — SetDPI.
func svcScene(t *testing.T, dpi float64, offscreen bool) (*Engine, *widget.Panel, *widget.Label) {
	t.Helper()
	var eng *Engine
	if offscreen {
		eng = NewOffscreen(400, 300, 20)
	} else {
		eng = New(400, 300, 20)
	}
	t.Cleanup(eng.Stop)
	if dpi > 0 {
		eng.SetDPI(dpi)
	}
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	l := widget.NewLabel(svcText, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	l.SetBounds(image.Rect(10, 10, 390, 40))
	root.AddChild(l)
	eng.SetRoot(root)
	return eng, root, l
}

// Окно на мониторе с другим DPI: у каждого дерева свой измеритель, хотя
// глобальный MeasureUIText отвечает последним созданным движком.
func TestOwnerServices_MeasureUsesOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, _, la := svcScene(t, 0, false)
	b, _, lb := svcScene(t, 144, false)
	const pt = 12.0
	wantA, wantB := a.canvas.MeasureText(svcText, pt), b.canvas.MeasureText(svcText, pt)
	if wantA == wantB {
		t.Fatalf("сцена не различает движки: %d = %d", wantA, wantB)
	}
	if got := widget.MeasureTextFor(la, svcText, pt); got != wantA {
		t.Errorf("лейбл A измерен %d, холст A — %d", got, wantA)
	}
	if got := widget.MeasureTextFor(lb, svcText, pt); got != wantB {
		t.Errorf("лейбл B измерен %d, холст B — %d", got, wantB)
	}
	if got := widget.MeasureUIText(svcText, pt); got != wantB {
		t.Errorf("глобальный замер %d, ждали последний движок (B) %d", got, wantB)
	}
	// Раскладка самого виджета — тоже своим движком.
	wa, _ := la.DesiredSize()
	wb, _ := lb.DesiredSize()
	if wa == wb {
		t.Errorf("DesiredSize лейблов в разных движках совпал: %d", wa)
	}
	// Шрифт по имени и метрики.
	if got, want := widget.MeasureTextFontFor(la, svcText, pt, widget.BuiltinFontBold), a.canvas.MeasureTextFont(svcText, pt, widget.BuiltinFontBold); got != want {
		t.Errorf("жирный в A: %d, холст %d", got, want)
	}
	if fm := widget.FontMetricsFor(lb, pt, ""); fm.Ascent == 0 || fm.Height <= fm.Ascent {
		t.Errorf("метрики B: %+v", fm)
	}
	// Смена масштаба B: замер идёт по новому холсту, память сброшена.
	b.SetScale(2)
	if got, want := widget.MeasureTextFor(lb, svcText, pt), b.canvas.MeasureText(svcText, pt); got != want {
		t.Errorf("после SetScale B: %d, холст %d", got, want)
	}
}

// Движок вне экрана глобальным измерителем не становится, но свои деревья
// меряет сам.
func TestOwnerServices_OffscreenTreeMeasuresWithOwnCanvas(t *testing.T) {
	testreset.Globals(t)
	_, _, _ = svcScene(t, 0, false)
	off, _, lo := svcScene(t, 144, true)
	const pt = 12.0
	if got, want := widget.MeasureTextFor(lo, svcText, pt), off.canvas.MeasureText(svcText, pt); got != want {
		t.Errorf("дерево offscreen измерено %d, его холст — %d", got, want)
	}
}

// Диалог до показа владельца не имеет — замер глобальный; после ShowModal —
// движком, где показан.
func TestOwnerServices_DialogMeasuresAfterShow(t *testing.T) {
	testreset.Globals(t)
	a, _, _ := svcScene(t, 144, false)
	_, _, _ = svcScene(t, 0, false) // последний — глобальный измеритель
	d := widget.NewDialog("Заголовок", 300, 200)
	const pt = 12.0
	if got, want := widget.MeasureTextFor(d, svcText, pt), widget.MeasureUIText(svcText, pt); got != want {
		t.Errorf("диалог до показа: %d, глобальный %d", got, want)
	}
	a.ShowModal(d)
	if got, want := widget.MeasureTextFor(d, svcText, pt), a.canvas.MeasureText(svcText, pt); got != want {
		t.Errorf("диалог в A: %d, холст A %d", got, want)
	}
}

// Перенос окна в дереве A доходит до A и до наблюдателя, но не до B.
func TestOwnerServices_MoveReachesOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, ra, _ := svcScene(t, 0, false)
	b, _, _ := svcScene(t, 0, false)
	win := widget.NewWindow("Окно", 300, 200)
	win.SetBounds(image.Rect(60, 60, 360, 260))
	ra.AddChild(win)
	a.takeMoves()
	b.takeMoves()
	var seen []widget.MoveNotice
	h := widget.RegisterMoveSink(func(n widget.MoveNotice) { seen = append(seen, n) })
	defer widget.UnregisterMoveSink(h)

	// Тащим за заголовок (как tests/moves_test.go).
	win.OnMouseButton(widget.MouseEvent{X: 160, Y: 70, Button: widget.MouseLeft, Pressed: true})
	win.OnMouseMove(190, 100)

	if len(seen) == 0 {
		t.Fatal("перетаскивание не объявило переноса")
	}
	if got := a.takeMoves(); len(got) == 0 {
		t.Error("перенос окна не дошёл до движка его дерева")
	}
	if got := b.takeMoves(); len(got) != 0 {
		t.Errorf("перенос окна дерева A дошёл до B: %d", len(got))
	}
}

// Анимация виджета дерева B (цикл B запущен), заведённая с чужой горутины,
// шагает циклом B: цикл A её не берёт. Без запущенного цикла ключа нет — её
// забирает работающий цикл, как прежде.
func TestOwnerServices_AnimationStepsOnOwnLoop(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	a, _, _ := svcScene(t, 0, false)
	b, _, lb := svcScene(t, 0, false)
	b.Start()
	var ticks atomic.Int32
	widget.AnimateOwned(lb, "svc", 50*time.Millisecond, nil, func(float64) { ticks.Add(1) })
	// Цикл A (шаг вручную) анимацию B не берёт, сколько бы ни шагал.
	now := time.Now()
	for i := 0; i < 5; i++ {
		widget.StepAnimationsFor(a, now.Add(time.Duration(i)*10*time.Millisecond))
	}
	deadline := time.Now().Add(2 * time.Second)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ticks.Load() == 0 {
		t.Fatal("цикл B не шагнул анимацию своего виджета")
	}
	// Остановленный движок снимает анимации своего цикла.
	ticks.Store(0)
	widget.AnimateOwned(lb, "svc2", time.Hour, nil, func(float64) { ticks.Add(1) })
	b.Stop()
	widget.StepAnimationsFor(a, time.Now())
	widget.StepAnimationsFor(a, time.Now().Add(time.Second))
	if n := ticks.Load(); n > 1 {
		t.Fatalf("анимацию остановленного движка шагает цикл A: %d тиков", n)
	}
}

// <text> в SVG иконки дерева — шрифтами движка этого дерева.
func TestOwnerServices_SVGTextRasterizerOfOwner(t *testing.T) {
	testreset.Globals(t)
	_, _, _ = svcScene(t, 0, false)
	b, rb, _ := svcScene(t, 0, false)
	ic := widget.NewSVGIcon()
	ic.SetBounds(image.Rect(0, 0, 32, 32))
	rb.AddChild(ic)
	if tr := widget.TextRasterizerFor(ic); tr != (svgTextBridge{b}) {
		t.Fatalf("растеризатор иконки %T %v, ждали мост движка B", tr, tr)
	}
	if err := ic.SetSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><text x="2" y="20" font-size="12">Ab</text></svg>`)); err != nil {
		t.Fatal(err)
	}
	orphan := widget.NewSVGIcon()
	if widget.TextRasterizerFor(orphan) != nil {
		t.Fatal("иконка без дерева получила растеризатор владельца")
	}
}
