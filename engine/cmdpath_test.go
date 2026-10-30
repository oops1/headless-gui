package engine

// cmdpath_test.go — удержание списков команд (ADR-0004, приёмка 2): кто
// перезаписывается после какого события, пометки из другой горутины,
// паника, второй движок, ничейное повреждение.

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// cmdLabel — лейбл, считающий свои Draw.
type cmdLabel struct {
	*widget.Label
	draws int
}

func newCmdLabel(text string, r image.Rectangle) *cmdLabel {
	l := &cmdLabel{Label: widget.NewLabel(text, color.RGBA{R: 230, G: 230, B: 230, A: 255})}
	l.SetBounds(r)
	return l
}

func (l *cmdLabel) Draw(ctx widget.DrawContext) { l.draws++; l.Label.Draw(ctx) }

// cmdPanel — панель, считающая свои Draw.
type cmdPanel struct {
	*widget.Panel
	draws int
}

func newCmdPanel(bg color.RGBA, r image.Rectangle) *cmdPanel {
	p := &cmdPanel{Panel: widget.NewPanel(bg)}
	p.ShowHeader = false
	p.SetBounds(r)
	return p
}

func (p *cmdPanel) Draw(ctx widget.DrawContext) { p.draws++; p.Panel.Draw(ctx) }

var cmdBG = color.RGBA{R: 30, G: 30, B: 34, A: 255}

// cmdGrid — движок с панелью из cols×rows лейблов, первый кадр нарисован.
func cmdGrid(w, h, cols, rows int) (*Engine, *cmdPanel, []*cmdLabel) {
	eng := New(w, h, 20)
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, w, h))
	var labels []*cmdLabel
	cellW, cellH := w/cols, 24
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			l := newCmdLabel(fmt.Sprintf("ряд %d столбец %d", r, c),
				image.Rect(c*cellW+4, r*cellH+4, (c+1)*cellW-4, (r+1)*cellH))
			root.AddChild(l)
			labels = append(labels, l)
		}
	}
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, root, labels
}

func sumDraws(ls []*cmdLabel) int {
	n := 0
	for _, l := range ls {
		n += l.draws
	}
	return n
}

// regionChanged сообщает, отличаются ли кадры в области r.
func regionChanged(a, b *image.RGBA, r image.Rectangle) bool {
	r = r.Intersect(a.Rect).Intersect(b.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return true
			}
		}
	}
	return false
}

func TestCmdPath_SetTextDrawsOneLabel(t *testing.T) {
	testreset.Globals(t)
	eng, root, labels := cmdGrid(1280, 800, 6, 30)
	defer eng.Stop()
	if !eng.CommandPath() {
		t.Fatal("путь команд должен быть включён по умолчанию")
	}
	before := eng.RenderOnce() // кадр без повреждения: всё заново, точка отсчёта
	total, rootBefore, one := sumDraws(labels), root.draws, labels[77].draws

	labels[77].SetText("изменено")
	after := eng.RenderOnce()

	if d := sumDraws(labels) - total; d != 1 || labels[77].draws != one+1 {
		t.Errorf("после SetText одного лейбла нарисовано %d лейблов (нужный: %d)", d, labels[77].draws-one)
	}
	if root.draws != rootBefore {
		t.Errorf("контейнер перезаписан %d раз, его команды не менялись", root.draws-rootBefore)
	}
	if !regionChanged(before, after, labels[77].Bounds()) {
		t.Error("новая подпись не попала в кадр")
	}
	st := eng.Stats()
	if st.Recorded == 0 || st.Retained == 0 {
		t.Errorf("Stats: Recorded=%d Retained=%d", st.Recorded, st.Retained)
	}
}

// cmdScroll — прокрутка, считающая свои Draw.
type cmdScroll struct {
	*widget.ScrollView
	draws int
}

func (s *cmdScroll) Draw(ctx widget.DrawContext) { s.draws++; s.ScrollView.Draw(ctx) }

func TestCmdPath_ScrolledChild(t *testing.T) {
	testreset.Globals(t)
	eng := New(400, 300, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 400, 300))
	sv := &cmdScroll{ScrollView: widget.NewScrollView()}
	sv.SetBounds(image.Rect(20, 20, 380, 280))
	sv.ContentHeight = 2000
	var labels []*cmdLabel
	for i := 0; i < 40; i++ {
		l := newCmdLabel(fmt.Sprintf("строка %d", i), image.Rect(30, 30+i*48, 300, 30+i*48+24))
		sv.AddChild(l)
		labels = append(labels, l)
	}
	root.AddChild(sv)
	eng.SetRoot(root)
	sv.SetScrollY(300)
	before := eng.RenderOnce()
	total, svBefore := sumDraws(labels), sv.draws

	labels[8].SetText("новая строка") // на экране: y = 30+8·48−300 = 114
	after := eng.RenderOnce()
	if d := sumDraws(labels) - total; d != 1 || labels[8].draws == 0 {
		t.Errorf("в прокрутке нарисовано %d лейблов", d)
	}
	if sv.draws != svBefore {
		t.Errorf("прокрутка перезаписана %d раз", sv.draws-svBefore)
	}
	screen := labels[8].Bounds().Sub(image.Pt(0, 300))
	if !regionChanged(before, after, screen) {
		t.Errorf("новая подпись не появилась на экране в %v", screen)
	}
	if regionChanged(before, after, labels[8].Bounds()) {
		t.Error("кадр изменился в координатах содержимого, а не экрана")
	}

	// Прокрутка — окружение детей другое: видимые перезаписываются.
	total = sumDraws(labels)
	sv.SetScrollY(348)
	eng.RenderOnce()
	if d := sumDraws(labels) - total; d < 4 {
		t.Errorf("после прокрутки перезаписано %d лейблов, ждали все видимые", d)
	}
}

// cmdHolder — контейнер, который держит детей мимо AddChild и рисует их
// через DrawTraced (как TrayGroup значки).
type cmdHolder struct {
	widget.Base
	items []widget.Widget
	draws int
}

func (h *cmdHolder) Draw(ctx widget.DrawContext) {
	h.draws++
	b := h.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{R: 60, G: 60, B: 70, A: 255})
	for _, it := range h.items {
		widget.DrawTraced(it, ctx)
	}
}

func TestCmdPath_ChildHeldOutsideChildren(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 100, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 100))
	h := &cmdHolder{}
	h.SetBounds(image.Rect(10, 10, 290, 90))
	a := newCmdLabel("один", image.Rect(20, 20, 140, 44))
	b := newCmdLabel("два", image.Rect(150, 20, 280, 44))
	h.items = []widget.Widget{a, b}
	root.AddChild(h)
	eng.SetRoot(root)
	eng.RenderOnce()
	hBefore, aBefore, bBefore := h.draws, a.draws, b.draws

	b.SetText("три")
	eng.RenderOnce()
	if b.draws != bBefore+1 || a.draws != aBefore {
		t.Errorf("значки: a %d→%d, b %d→%d", aBefore, a.draws, bBefore, b.draws)
	}
	if h.draws != hBefore {
		t.Errorf("держатель перезаписан %d раз: пометка значка не нашла родителя по записи", h.draws-hBefore)
	}

	// Значок, которого держатель перестал рисовать, помечает держателя:
	// его нет в текущем списке держателя.
	h.items = h.items[:1]
	h.MarkDirty()
	h.InvalidateArea(h.Bounds())
	eng.RenderOnce()
	hBefore = h.draws
	b.SetText("четыре")
	eng.RenderOnce()
	if h.draws != hBefore+1 {
		t.Errorf("держатель без ссылки на значок должен перезаписаться: %d→%d", hBefore, h.draws)
	}
}

func TestCmdPath_StructuralMarks(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 200))
	box := newCmdPanel(color.RGBA{R: 50, G: 50, B: 60, A: 255}, image.Rect(10, 10, 290, 190))
	a := newCmdLabel("a", image.Rect(20, 20, 140, 44))
	b := newCmdLabel("b", image.Rect(20, 60, 140, 84))
	box.AddChild(a)
	box.AddChild(b)
	root.AddChild(box)
	eng.SetRoot(root)
	eng.RenderOnce()

	type counts struct{ root, box, a, b int }
	snap := func() counts { return counts{root.draws, box.draws, a.draws, b.draws} }
	step := func(name string, act func(), want counts) {
		t.Helper()
		was := snap()
		act()
		eng.RenderOnce()
		now := snap()
		got := counts{now.root - was.root, now.box - was.box, now.a - was.a, now.b - was.b}
		if got != want {
			t.Errorf("%s: перезаписаны {root box a b} = %+v, ждали %+v", name, got, want)
		}
	}
	step("SetBounds ребёнка", func() { a.SetBounds(image.Rect(20, 20, 150, 44)) }, counts{0, 1, 1, 0})
	c := newCmdLabel("c", image.Rect(20, 100, 140, 124))
	step("AddChild", func() { box.AddChild(c) }, counts{0, 1, 0, 0})
	step("RemoveChild", func() { box.RemoveChild(c) }, counts{0, 1, 0, 0})
	step("SetVisible(false)", func() { b.SetVisible(false) }, counts{0, 1, 0, 0})
	step("SetVisible(true)", func() { b.SetVisible(true) }, counts{0, 1, 0, 1})
	step("RaiseChild", func() { box.RaiseChild(a) }, counts{0, 1, 0, 0})
	step("Invalidate контейнера — поддерево", func() { box.Invalidate() }, counts{0, 1, 1, 1})
	step("MarkDirty — только он", func() { box.MarkDirty(); box.InvalidateArea(box.Bounds()) }, counts{0, 1, 0, 0})
	step("Engine.Invalidate — все", func() { eng.Invalidate() }, counts{1, 1, 1, 1})
	step("Engine.InvalidateRect над a — цепочка до корня и a", func() {
		eng.InvalidateRect(image.Rect(30, 30, 40, 40))
	}, counts{1, 1, 1, 0})
	step("widget.InvalidateRect мимо виджетов — то же", func() {
		widget.InvalidateRect(image.Rect(30, 70, 40, 80))
	}, counts{1, 1, 0, 1})
}

func TestCmdPath_ThemeScopeSubtree(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 200))
	scope := widget.NewThemeScope(nil)
	scope.SetBounds(image.Rect(0, 0, 300, 100))
	a := newCmdLabel("в области", image.Rect(10, 10, 200, 34))
	b := newCmdLabel("тоже", image.Rect(10, 50, 200, 74))
	scope.AddChild(a)
	scope.AddChild(b)
	c := newCmdLabel("снаружи", image.Rect(10, 120, 200, 144))
	root.AddChild(scope)
	root.AddChild(c)
	eng.SetRoot(root)
	eng.RenderOnce()
	wa, wb, wc := a.draws, b.draws, c.draws
	scope.SetTheme(widget.LightTheme())
	eng.RenderOnce()
	if a.draws != wa+1 || b.draws != wb+1 {
		t.Errorf("поддерево области: a %d→%d, b %d→%d", wa, a.draws, wb, b.draws)
	}
	if c.draws != wc {
		t.Errorf("виджет вне области перезаписан: %d→%d", wc, c.draws)
	}
}

// cmdClipper — контейнер, сужающий клип детям на inset.
type cmdClipper struct {
	widget.Base
	inset int
	draws int
}

func (c *cmdClipper) Draw(ctx widget.DrawContext) {
	c.draws++
	b := c.Bounds()
	ctx.SetClip(b.Inset(c.inset))
	c.DrawChildren(ctx)
	ctx.ClearClip()
}

func TestCmdPath_ParentClipChangeRerecordsChildren(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 200))
	cl := &cmdClipper{inset: 4}
	cl.SetBounds(image.Rect(10, 10, 290, 190))
	a := newCmdLabel("ребёнок", image.Rect(0, 0, 200, 40))
	cl.AddChild(a)
	root.AddChild(cl)
	eng.SetRoot(root)
	before := eng.RenderOnce()
	wa := a.draws

	cl.inset = 30
	cl.MarkDirty() // только контейнер: ребёнка никто не трогал
	cl.InvalidateArea(cl.Bounds())
	after := eng.RenderOnce()
	if a.draws != wa+1 {
		t.Errorf("клип родителя изменился, ребёнок не перезаписан: %d→%d", wa, a.draws)
	}
	if !regionChanged(before, after, image.Rect(14, 14, 40, 40)) {
		t.Error("новый клип не проявился в кадре")
	}
}

func TestCmdPath_OccludedChildAppearsWhenNeighborClears(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 300, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 300))
	under := newCmdPanel(color.RGBA{R: 200, A: 255}, image.Rect(50, 50, 150, 150))
	over := newCmdPanel(color.RGBA{B: 200, A: 255}, image.Rect(50, 50, 150, 150))
	root.AddChild(under)
	root.AddChild(over)
	eng.SetRoot(root)
	first := eng.RenderOnce()
	if under.draws != 0 {
		t.Fatalf("закрытая панель нарисована %d раз", under.draws)
	}
	if first.RGBAAt(100, 100) != (color.RGBA{B: 200, A: 255}) {
		t.Fatalf("пиксель под верхней панелью %v", first.RGBAAt(100, 100))
	}

	over.UseAlpha = true
	over.Background = color.RGBA{}
	over.Invalidate()
	img := eng.RenderOnce()
	if under.draws != 1 {
		t.Errorf("сосед стал прозрачным, закрытая панель нарисована %d раз", under.draws)
	}
	if got := img.RGBAAt(100, 100); got != (color.RGBA{R: 200, A: 255}) {
		t.Errorf("пиксель %v, ждали красный нижней панели", got)
	}
}

func TestCmdPath_UnownedDamagePicksUpFieldWrite(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 100, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 100))
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(20, 20, 160, 60))
	root.AddChild(btn)
	eng.SetRoot(root)
	before := eng.RenderOnce()

	btn.Text = "Нет, другая подпись" // мимо сеттера
	eng.InvalidateRect(btn.Bounds())
	after := eng.RenderOnce()
	if !regionChanged(before, after, btn.Bounds()) {
		t.Error("подпись, записанная в поле, не дошла до кадра после InvalidateRect")
	}
}

// cmdBlinker — виджет, рисующий по полю и заявляющий область мимо себя.
type cmdBlinker struct {
	widget.Base
	on bool
}

func (b *cmdBlinker) Draw(ctx widget.DrawContext) {
	r := b.Bounds()
	col := color.RGBA{R: 255, A: 255}
	if b.on {
		col = color.RGBA{G: 255, A: 255}
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
}

func TestCmdPath_PackageInvalidateRectAnimates(t *testing.T) {
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
	for i := 0; i < 3; i++ {
		bl.on = !bl.on
		widget.InvalidateRect(bl.Bounds())
		img := eng.RenderOnce()
		want := color.RGBA{R: 255, A: 255}
		if bl.on {
			want = color.RGBA{G: 255, A: 255}
		}
		if got := img.RGBAAt(60, 60); got != want {
			t.Fatalf("шаг %d: пиксель %v, ждали %v", i, got, want)
		}
	}
}

// cmdBlocker — лейбл, чей первый Draw ждёт сигнала: в это время другая
// горутина меняет текст.
type cmdBlocker struct {
	*widget.Label
	started chan struct{}
	release chan struct{}
	once    sync.Once
	draws   int
}

func (b *cmdBlocker) Draw(ctx widget.DrawContext) {
	b.draws++
	b.once.Do(func() {
		close(b.started)
		<-b.release
	})
	b.Label.Draw(ctx)
}

func TestCmdPath_InvalidateDuringDrawIsNotLost(t *testing.T) {
	testreset.Globals(t)
	eng := New(300, 100, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 300, 100))
	bl := &cmdBlocker{Label: widget.NewLabel("до", color.RGBA{R: 230, G: 230, B: 230, A: 255}),
		started: make(chan struct{}), release: make(chan struct{})}
	bl.SetBounds(image.Rect(20, 20, 280, 60))
	root.AddChild(bl)
	eng.SetRoot(root)

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-bl.started
		bl.SetText("после") // во время записи — пометка должна пережить кадр
		close(bl.release)
	}()
	first := eng.RenderOnce()
	<-done
	if bl.draws != 1 {
		t.Fatalf("первый кадр: %d Draw", bl.draws)
	}
	// Первый кадр уже нарисовал новый текст (Draw читает его после
	// сигнала), но пометка, поставленная во время записи, обязана дожить до
	// второго кадра: он перезаписывает лейбл ещё раз.
	second := eng.RenderOnce()
	if bl.draws != 2 {
		t.Errorf("Invalidate во время Draw потерян: второй кадр не перезаписал лейбл (%d Draw)", bl.draws)
	}
	if regionChanged(first, second, bl.Bounds()) {
		t.Error("второй кадр нарисовал другой текст")
	}
}

// cmdPanicker — виджет, падающий в Draw заданное число раз.
type cmdPanicker struct {
	widget.Base
	boom  int
	draws int
}

func (p *cmdPanicker) Draw(ctx widget.DrawContext) {
	p.draws++
	if p.boom > 0 {
		p.boom--
		panic("cmdPanicker: boom")
	}
	r := p.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{G: 200, A: 255})
}

func TestCmdPath_PanicRestoresDirtyAndBrokenPlaceholder(t *testing.T) {
	testreset.Globals(t)
	eng := New(200, 200, 20)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	eng.SetFailurePolicy(FailureIsolate)
	root := newCmdPanel(cmdBG, image.Rect(0, 0, 200, 200))
	p := &cmdPanicker{boom: 2}
	p.SetBounds(image.Rect(20, 20, 120, 120))
	root.AddChild(p)
	eng.SetRoot(root)

	eng.RenderOnce() // первая паника: учёт выключен, виновник неизвестен
	if eng.Stats().Panics != 1 || !p.IsDrawDirty() {
		t.Fatalf("после паники: Panics=%d, dirty=%v", eng.Stats().Panics, p.IsDrawDirty())
	}
	eng.RenderOnce() // вторая: учёт включён, виджет помечен сломанным
	if !widget.IsBroken(p) {
		t.Fatal("после второй паники виджет не помечен сломанным")
	}
	img := eng.RenderOnce()
	// Точка вне диагоналей креста заглушки — её серая заливка.
	if got := img.RGBAAt(40, 70); got != (color.RGBA{R: 96, G: 96, B: 96, A: 255}) {
		t.Errorf("на месте сломанного виджета %v, ждали заглушку", got)
	}
	draws := p.draws
	widget.ClearBroken(p)
	if !p.IsDrawDirty() {
		t.Error("ClearBroken не пометил виджет устаревшим")
	}
	img = eng.RenderOnce()
	if p.draws != draws+1 || img.RGBAAt(40, 70) != (color.RGBA{G: 200, A: 255}) {
		t.Errorf("после ClearBroken: Draw %d→%d, пиксель %v", draws, p.draws, img.RGBAAt(40, 70))
	}
}

func TestCmdPath_SecondEngineRerecords(t *testing.T) {
	testreset.Globals(t)
	eng1, root, labels := cmdGrid(300, 100, 2, 2)
	defer eng1.Stop()
	eng2 := New(300, 100, 20)
	defer eng2.Stop()
	eng2.SetRenderOnDemand(true)
	total := sumDraws(labels)
	eng2.SetRoot(root)
	eng2.RenderOnce()
	if d := sumDraws(labels) - total; d != len(labels) {
		t.Errorf("второй движок перезаписал %d из %d лейблов", d, len(labels))
	}
	// Назад в первый: списки принадлежат второму — снова все.
	total = sumDraws(labels)
	labels[0].SetText("x")
	eng1.RenderOnce()
	if d := sumDraws(labels) - total; d != len(labels) {
		t.Errorf("первый движок после второго перезаписал %d из %d", d, len(labels))
	}
}

// cmdAnimated — виджет с фокусом, которому нужна анимация (каретка).
type cmdAnimated struct{ *cmdLabel }

func (a *cmdAnimated) NeedsAnimation() bool { return true }

func TestCmdPath_AnimatedFocusFrameIsPartial(t *testing.T) {
	testreset.Globals(t)
	eng, root, labels := cmdGrid(300, 100, 2, 2)
	defer eng.Stop()
	anim := &cmdAnimated{labels[1]}
	root.AddChild(anim)
	eng.RenderOnce()
	// Повреждение от самой смены фокуса снимаем: проверяется только кадр ради
	// анимации каретки.
	eng.SetFocus(anim)
	eng.consumeDamage()
	if !eng.animationNeeded(time.Second / 60) {
		t.Fatal("анимация фокусного виджета не замечена")
	}
	if !labels[1].IsDrawDirty() {
		t.Error("фокусный анимируемый виджет не помечен устаревшим")
	}
	regions, all := eng.consumeDamage()
	if all || len(regions) != 1 {
		t.Errorf("повреждение кадра анимации: all=%v, областей %d — кадр должен быть частичным", all, len(regions))
	}
}

func TestCmdPath_SwitchOffUsesDirectPath(t *testing.T) {
	testreset.Globals(t)
	eng, root, labels := cmdGrid(300, 100, 2, 2)
	defer eng.Stop()
	eng.SetCommandPath(false)
	eng.RenderOnce()
	total, rootBefore := sumDraws(labels), root.draws
	labels[0].SetText("прямой путь")
	eng.RenderOnce()
	if root.draws != rootBefore+1 || sumDraws(labels)-total < 1 {
		t.Errorf("прямой путь: root %d→%d, лейблов +%d", rootBefore, root.draws, sumDraws(labels)-total)
	}
	before := eng.Stats()
	eng.SetCommandPath(true)
	eng.RenderOnce()
	eng.RenderOnce()
	if st := eng.Stats(); st.Recorded == before.Recorded {
		t.Error("после включения пути команд запись не идёт")
	}
}
