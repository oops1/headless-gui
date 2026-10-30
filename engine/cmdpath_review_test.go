package engine

// cmdpath_review_test.go — случаи из состязательного ревью 4а.1: тема
// неактивной вкладки, дети мимо Children(), паника прямого виджета,
// ничейное повреждение на кадре шагов переходов, клип на выходе ребёнка,
// кегль темы.

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

func TestCmdPath_ThemeReachesInactiveTab(t *testing.T) {
	testreset.Globals(t)
	eng := New(400, 300, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 400, 300))
	scope := widget.NewThemeScope(widget.LightTheme())
	scope.SetBounds(image.Rect(0, 0, 400, 300))
	tc := widget.NewTabControl()
	tc.SetBounds(image.Rect(10, 10, 390, 290))
	a := newCmdLabel("вкладка A", image.Rect(20, 60, 300, 84))
	b := newCmdLabel("вкладка B", image.Rect(20, 60, 300, 84))
	pa := newCmdPanel(color.RGBA{R: 60, G: 60, B: 60, A: 255}, image.Rect(10, 50, 390, 290))
	pb := newCmdPanel(color.RGBA{R: 60, G: 60, B: 60, A: 255}, image.Rect(10, 50, 390, 290))
	pa.AddChild(a)
	pb.AddChild(b)
	tc.AddTab("A", pa)
	tc.AddTab("B", pb)
	scope.AddChild(tc)
	root.AddChild(scope)
	eng.SetRoot(root)
	tc.SetActive(1)
	eng.RenderOnce()
	tc.SetActive(0)
	eng.RenderOnce()
	scope.SetTheme(widget.DarkTheme())
	eng.RenderOnce()
	wb := b.draws
	tc.SetActive(1)
	img := eng.RenderOnce()
	if b.draws != wb+1 {
		t.Errorf("содержимое вкладки B после смены темы не перезаписано: Draw %d→%d", wb, b.draws)
	}
	eng.SetCommandPath(false)
	ref := eng.RenderOnce()
	if !bytes.Equal(img.Pix, ref.Pix) {
		t.Error("кадр вкладки B отличается от прямого пути — старая тема")
	}
}

// cmdExtraHolder — контейнер с детьми мимо Children(), объявляющий их.
type cmdExtraHolder struct{ cmdHolder }

func (h *cmdExtraHolder) ExtraChildren() []widget.Widget { return h.items }

func TestCmdPath_ContainerInvalidateReachesExtraChildren(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 100, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 100))
	h := &cmdExtraHolder{}
	h.SetBounds(image.Rect(10, 10, 290, 90))
	a := newCmdLabel("один", image.Rect(20, 20, 140, 44))
	h.items = []widget.Widget{a}
	root.AddChild(h)
	eng.SetRoot(root)
	eng.RenderOnce()
	wa := a.draws
	h.Invalidate() // поддерево — и дети мимо Children()
	eng.RenderOnce()
	if a.draws != wa+1 {
		t.Errorf("Invalidate контейнера не дошёл до ребёнка мимо Children(): %d→%d", wa, a.draws)
	}
}

// cmdDirectPanicker — прямой виджет, падающий в Draw заданное число раз.
type cmdDirectPanicker struct {
	widget.Base
	boom int
}

func (p *cmdDirectPanicker) DrawDirect() bool { return true }
func (p *cmdDirectPanicker) Draw(ctx widget.DrawContext) {
	if p.boom > 0 {
		p.boom--
		panic("cmdDirectPanicker: boom")
	}
	r := p.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{G: 200, A: 255})
}

func TestCmdPath_DirectDrawerPanicIsAttributed(t *testing.T) {
	testreset.Globals(t)
	eng := New(200, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	eng.SetFailurePolicy(FailureIsolate)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 200, 200))
	p := &cmdDirectPanicker{boom: 2}
	p.SetBounds(image.Rect(20, 20, 120, 120))
	root.AddChild(p)
	eng.SetRoot(root)
	eng.RenderOnce() // паника при воспроизведении: учёт включается
	eng.RenderOnce() // виновник назван, помечен сломанным
	if !widget.IsBroken(p) {
		t.Fatal("прямой виджет после второй паники не помечен сломанным")
	}
	img := eng.RenderOnce()
	if got := img.RGBAAt(40, 70); got != (color.RGBA{R: 96, G: 96, B: 96, A: 255}) {
		t.Errorf("на месте сломанного прямого виджета %v, ждали заглушку", got)
	}
	widget.ClearBroken(p)
	img = eng.RenderOnce()
	if got := img.RGBAAt(40, 70); got != (color.RGBA{G: 200, A: 255}) {
		t.Errorf("после ClearBroken прямой виджет не рисуется сам: %v", got)
	}
}

func TestCmdPath_ForceSurvivesStepFrame(t *testing.T) {
	testreset.Globals(t)
	eng := New(200, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 200, 200))
	bl := &cmdBlinker{}
	bl.SetBounds(image.Rect(40, 40, 80, 80))
	root.AddChild(bl)
	eng.SetRoot(root)
	eng.RenderOnce()

	// Только ничейное повреждение (без damage) и кадр шагов попапов: он
	// выходит до записи дерева — повреждение обязано дожить до следующего.
	bl.on = true
	eng.InvalidateRect(bl.Bounds())
	eng.consumeDamage() // damage снят, ничейное повреждение осталось
	eng.ov.RequestPopupStep()
	eng.renderFrame()
	eng.damageRect(bl.Bounds())
	img := eng.RenderOnce()
	if got := img.RGBAAt(60, 60); got != (color.RGBA{G: 255, A: 255}) {
		t.Errorf("ничейное повреждение потеряно на кадре шагов: пиксель %v", got)
	}
}

// cmdClipLeaver — ребёнок, который снимает отсечение в конце Draw (как
// DataGrid и TreeView), пока leave == true.
type cmdClipLeaver struct {
	widget.Base
	leave bool
}

func (c *cmdClipLeaver) Draw(ctx widget.DrawContext) {
	r := c.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{B: 200, A: 255})
	if c.leave {
		ctx.ClearClip()
	}
}

// cmdClipScene — контейнер с клипом, ребёнок-снимающий и брат шире клипа.
func cmdClipScene() (*Engine, *cmdClipper, *cmdClipLeaver, *cmdPanel) {
	eng := New(300, 200, 20)
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 200))
	cl := &cmdClipper{inset: 40}
	cl.SetBounds(image.Rect(0, 0, 300, 200))
	leaver := &cmdClipLeaver{leave: true}
	leaver.SetBounds(image.Rect(60, 60, 100, 100))
	wide := newCmdPanel(color.RGBA{R: 200, A: 255}, image.Rect(0, 0, 300, 30))
	cl.AddChild(leaver)
	cl.AddChild(wide)
	root.AddChild(cl)
	eng.SetRoot(root)
	return eng, cl, leaver, wide
}

func TestCmdPath_ChildExitClipKept(t *testing.T) {
	testreset.Globals(t)
	eng, cl, leaver, wide := cmdClipScene()
	defer eng.Stop()
	ref, rcl, rleaver, rwide := cmdClipScene()
	defer ref.Stop()
	ref.SetCommandPath(false)
	first := eng.RenderOnce()
	if !bytes.Equal(first.Pix, ref.RenderOnce().Pix) {
		t.Fatal("первый кадр отличается от прямого пути")
	}
	if first.RGBAAt(10, 10) != (color.RGBA{R: 200, A: 255}) {
		t.Fatal("брат должен рисоваться без клипа после снявшего его ребёнка")
	}
	// Контейнер перезаписывается, ребёнок удержан: брат по-прежнему без клипа.
	cl.MarkDirty()
	cl.InvalidateArea(cl.Bounds())
	rcl.MarkDirty()
	rcl.InvalidateArea(rcl.Bounds())
	second := eng.RenderOnce()
	if !bytes.Equal(first.Pix, second.Pix) {
		t.Error("после перезаписи контейнера с удержанным ребёнком кадр изменился: клип на выходе ребёнка потерян")
	}
	if !bytes.Equal(second.Pix, ref.RenderOnce().Pix) {
		t.Error("второй кадр отличается от прямого пути")
	}
	// Ребёнок перестал снимать клип: список контейнера устарел, брат
	// перезаписывается под клипом — как на прямом пути (область брата
	// заявлена, иначе её пиксели не перерисовал бы ни один путь).
	leaver.leave = false
	leaver.Invalidate()
	eng.damageRect(wide.Bounds())
	rleaver.leave = false
	rleaver.Invalidate()
	ref.damageRect(rwide.Bounds())
	third := eng.RenderOnce()
	if !bytes.Equal(third.Pix, ref.RenderOnce().Pix) {
		t.Error("третий кадр отличается от прямого пути: клип брата после смены выхода ребёнка")
	}
	if third.RGBAAt(10, 10) == (color.RGBA{R: 200, A: 255}) {
		t.Error("брат всё ещё без клипа, хотя ребёнок клип больше не снимает")
	}
}

func TestCmdPath_IdentityThemeFontSize(t *testing.T) {
	testreset.Globals(t)
	withTheme(t, "Win10 Dark", func(th *widget.Theme) {
		widget.SetDefaultFontSize(9)
		p := cmdSampler(th)
		defer p.closeAll()
		cmdCompare(t, p, "кегль темы 9")
	})
}
