package draw

// record_test.go — рекордер и воспроизведение на заглушках (ADR-0004):
// структура списков, удержание, обход, отмена и воспроизведение по
// повреждению. Тождество кадра холсту проверяют тесты движка.

import (
	"image"
	"image/color"
	"testing"
)

// stubMeasurer — измеритель с простой геометрией: 8 px на руну, кегль 12.
type stubMeasurer struct{ w, h int }

func (m stubMeasurer) MeasureText(text string, sizePt float64) int { return 8 * len([]rune(text)) }
func (m stubMeasurer) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return 8 * len([]rune(text))
}
func (m stubMeasurer) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (m stubMeasurer) FontMetrics(fontName string, sizePt float64) (int, int, int) {
	return 10, 3, 2
}
func (m stubMeasurer) HasGlyph(r rune) bool { return true }
func (m stubMeasurer) TextBounds(text string, x, y int, sizePt float64, fontName string) image.Rectangle {
	return image.Rect(x, y-12, x+8*len([]rune(text)), y+6)
}
func (m stubMeasurer) LogicalClip(r image.Rectangle) image.Rectangle {
	return r.Intersect(image.Rect(0, 0, m.w, m.h))
}
func (m stubMeasurer) LogicalSize() (int, int)              { return m.w, m.h }
func (m stubMeasurer) Scale() float64                       { return 1 }
func (m stubMeasurer) HasWallpaper() bool                   { return false }
func (m stubMeasurer) OcclusionEnabled() bool               { return true }
func (m stubMeasurer) InvalidateImageCache(src image.Image) {}

// stubHolder — холдер с настраиваемым Draw.
type stubHolder struct {
	name  string
	list  *List
	refIn *List
	dirty int
	draw  func(ctx DrawContext)
	draws int
}

func (h *stubHolder) Draw(ctx DrawContext) {
	h.draws++
	if h.draw != nil {
		h.draw(ctx)
	}
}
func (h *stubHolder) CmdList() *List       { return h.list }
func (h *stubHolder) SetCmdList(l *List)   { h.list = l }
func (h *stubHolder) MarkDirty()           { h.dirty++ }
func (h *stubHolder) NoteRef(parent *List) { h.refIn = parent }

var red = color.RGBA{R: 255, A: 255}

// op — одна записанная операция заглушки контекста.
type op struct {
	name string
	r    image.Rectangle
}

// stubCtx — контекст, запоминающий вызовы.
type stubCtx struct {
	ops  []op
	clip image.Rectangle
	has  bool
}

func (c *stubCtx) log(name string, r image.Rectangle) { c.ops = append(c.ops, op{name, r}) }
func (c *stubCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.log("fill", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.log("fillA", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.log("round", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) DrawBorder(x, y, w, h int, col color.RGBA) {
	c.log("border", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	c.log("rborder", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) SetPixel(x, y int, col color.RGBA) { c.log("px", image.Rect(x, y, x+1, y+1)) }
func (c *stubCtx) DrawHLine(x, y, length int, col color.RGBA) {
	c.log("h", image.Rect(x, y, x+length, y+1))
}
func (c *stubCtx) DrawVLine(x, y, length int, col color.RGBA) {
	c.log("v", image.Rect(x, y, x+1, y+length))
}
func (c *stubCtx) DrawImage(src image.Image, x, y int) { c.log("img", image.Rect(x, y, x, y)) }
func (c *stubCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.log("imgS", image.Rect(x, y, x+w, y+h))
}
func (c *stubCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.log("text", image.Rect(x, y, x, y))
}
func (c *stubCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.log("text", image.Rect(x, y, x, y))
}
func (c *stubCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.log("text", image.Rect(x, y, x, y))
}
func (c *stubCtx) MeasureText(text string, sizePt float64) int { return 8 * len(text) }
func (c *stubCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return 8 * len(text)
}
func (c *stubCtx) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (c *stubCtx) SetClip(r image.Rectangle) {
	c.log("clip", r)
	c.clip, c.has = r, true
}
func (c *stubCtx) ClearClip() {
	c.log("noclip", image.Rectangle{})
	c.has = false
}
func (c *stubCtx) Clip() image.Rectangle {
	if c.has {
		return c.clip
	}
	return image.Rect(0, 0, 200, 200)
}

func (c *stubCtx) count(name string) int {
	n := 0
	for _, o := range c.ops {
		if o.name == name {
			n++
		}
	}
	return n
}

// fillHolder — холдер, рисующий один прямоугольник.
func fillHolder(name string, r image.Rectangle) *stubHolder {
	return &stubHolder{name: name, draw: func(ctx DrawContext) {
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), red)
	}}
}

// recordChild — запись ребёнка под рекордером: BeginRecord, Draw, EndRecord.
func recordChild(r *Recorder, h *stubHolder, off image.Point) {
	r.BeginRecord(h, off)
	h.Draw(r)
	r.EndRecord(h)
}

func TestRecorder_RecordKeepAndWalk(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	root := &stubHolder{name: "root"}
	a := fillHolder("a", image.Rect(10, 10, 50, 50))
	b := fillHolder("b", image.Rect(100, 100, 150, 150))
	root.draw = func(ctx DrawContext) {
		ctx.FillRect(0, 0, 200, 200, red)
		recordChild(r, a, image.Point{})
		recordChild(r, b, image.Point{})
	}

	// Кадр 1: всё записано.
	r.BeginFrame(m, true, nil)
	recordChild(r, root, image.Point{})
	l, exit := r.EndFrame()
	if l == nil || l != root.list {
		t.Fatal("список корня не опубликован")
	}
	if !exit.NoClip {
		t.Errorf("клип на выходе: %+v, ждали снятый", exit)
	}
	if got := l.Len(); got != 3 {
		t.Fatalf("команд у корня %d, ждали 3 (заливка и две ссылки)", got)
	}
	if !l.HasRef(a) || !l.HasRef(b) {
		t.Error("ссылки на детей не записаны")
	}
	if a.refIn != l || b.refIn != l {
		t.Error("NoteRef: у детей не список корня")
	}
	if r.Recorded != 5 || r.Retained != 0 { // a, b и корень (заливка + две ссылки)
		t.Errorf("Recorded=%d Retained=%d, ждали 5/0", r.Recorded, r.Retained)
	}
	if want := image.Rect(0, 0, 200, 200); l.BBox != want {
		t.Errorf("охват корня %v, ждали %v", l.BBox, want)
	}
	if a.list.BBox != (image.Rect(10, 10, 50, 50)) {
		t.Errorf("охват a %v", a.list.BBox)
	}

	// Кадр 2: ничего не менялось — списки верны, Keep берёт их.
	r.BeginFrame(m, true, nil)
	if !r.CanReuse(root, image.Rect(0, 0, 200, 200), image.Point{}) {
		t.Fatal("список корня должен быть верен")
	}
	r.Keep(root, image.Point{})
	l2, _ := r.EndFrame()
	if l2 != l {
		t.Error("Keep корня должен вернуть тот же список")
	}
	if r.Retained != 3 || r.Recorded != 0 {
		t.Errorf("Recorded=%d Retained=%d, ждали 0/3", r.Recorded, r.Retained)
	}

	// Кадр 3: обход удержанного корня, b перезаписывается шире.
	b.draw = func(ctx DrawContext) { ctx.FillRect(100, 100, 90, 90, red) }
	r.BeginFrame(m, true, nil)
	visited := 0
	stale := r.WalkRetained(root, func(child ListHolder, off image.Point) {
		visited++
		if child == ListHolder(b) {
			recordChild(r, b, off)
			return
		}
		if !r.CanReuse(child, image.Rect(10, 10, 50, 50), off) {
			t.Error("список a должен быть верен при обходе")
		}
		r.Keep(child, off)
	})
	r.Keep(root, image.Point{})
	r.EndFrame()
	if visited != 2 || stale {
		t.Errorf("обход: посещено %d, список корня устарел %v (клип на выходе b не менялся)", visited, stale)
	}
	if b.list.Len() != 1 || b.list.BBox != image.Rect(100, 100, 190, 190) {
		t.Errorf("новый список b: %d команд, охват %v", b.list.Len(), b.list.BBox)
	}
	if l.Len() != 3 {
		t.Errorf("обход добавил команды в список корня: %d", l.Len())
	}
	if !l.BBox.Eq(image.Rect(0, 0, 200, 200)) {
		t.Errorf("охват корня после обхода %v", l.BBox)
	}
	if b.refIn != l {
		t.Error("после перезаписи в обходе b должен числиться в списке корня")
	}
}

func TestRecorder_CanReuseConditions(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	a := fillHolder("a", image.Rect(10, 10, 50, 50))
	r.BeginFrame(m, true, nil)
	r.SetClip(image.Rect(0, 0, 100, 100))
	recordChild(r, a, image.Point{})
	r.EndFrame()
	bounds := image.Rect(10, 10, 50, 50)

	check := func(name string, want bool, off image.Point, retain bool, force []image.Rectangle, clip image.Rectangle) {
		t.Helper()
		r.BeginFrame(m, retain, force)
		if clip.Empty() {
			r.ClearClip()
		} else {
			r.SetClip(clip)
		}
		if got := r.CanReuse(a, bounds, off); got != want {
			t.Errorf("%s: CanReuse=%v, ждали %v", name, got, want)
		}
		r.EndFrame()
	}
	same := image.Rect(0, 0, 100, 100)
	check("то же окружение", true, image.Point{}, true, nil, same)
	check("другой клип", false, image.Point{}, true, nil, image.Rect(0, 0, 90, 100))
	check("снятый клип", false, image.Point{}, true, nil, image.Rectangle{})
	check("другой сдвиг", false, image.Pt(0, 5), true, nil, same)
	check("ничейное повреждение рядом", true, image.Point{}, true, []image.Rectangle{image.Rect(60, 60, 70, 70)}, same)
	check("ничейное повреждение задевает", false, image.Point{}, true, []image.Rectangle{image.Rect(40, 40, 70, 70)}, same)
	check("без удержания", false, image.Point{}, false, nil, same)
	r.Invalidate()
	check("после Invalidate", false, image.Point{}, true, nil, same)
	// Другой рекордер — другой движок.
	r2 := NewRecorder()
	r2.BeginFrame(m, true, nil)
	r2.SetClip(image.Rect(0, 0, 100, 100))
	if r2.CanReuse(a, bounds, image.Point{}) {
		t.Error("список чужого рекордера принят")
	}
}

func TestRecorder_AbortMarksOpenHolders(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	root := &stubHolder{name: "root"}
	a := &stubHolder{name: "a"}
	r.BeginFrame(m, true, nil)
	r.BeginRecord(root, image.Point{})
	r.FillRect(0, 0, 10, 10, red)
	r.BeginRecord(a, image.Point{})
	r.Abort()
	if root.dirty != 1 || a.dirty != 1 {
		t.Errorf("Abort: dirty root=%d a=%d, ждали 1/1", root.dirty, a.dirty)
	}
	if root.list != nil || a.list != nil {
		t.Error("Abort опубликовал список")
	}
	if l, _ := r.EndFrame(); l != nil {
		t.Error("после Abort у кадра не должно быть списка")
	}
}

func TestRecorder_OpacityAndSkips(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	root := &stubHolder{name: "root"}
	a := fillHolder("a", image.Rect(10, 10, 50, 50))
	hidden := &stubHolder{name: "hidden"}
	root.draw = func(ctx DrawContext) {
		ctx.FillRect(0, 0, 200, 200, red)
		r.DrawWithOpacity(image.Rect(0, 0, 100, 100), 0.5, func() {
			ctx.FillRect(5, 5, 10, 10, red)
			recordChild(r, a, image.Point{})
		})
		r.NoteSkip(hidden, true)
	}
	r.BeginFrame(m, true, nil)
	recordChild(r, root, image.Point{})
	l, _ := r.EndFrame()
	if l.Len() != 2 || l.Cmds[1].Op != OpOpacity || l.Cmds[1].Ext == nil || l.Cmds[1].Ext.Sub == nil {
		t.Fatalf("ждали заливку и группу прозрачности, получили %d команд", l.Len())
	}
	sub := l.Cmds[1].Ext.Sub
	if sub.Len() != 2 || !sub.HasRef(a) {
		t.Errorf("группа: %d команд, ссылка на a: %v", sub.Len(), sub.HasRef(a))
	}
	if a.refIn != l {
		t.Error("ребёнок в группе числится не в списке холдера")
	}
	if !l.OcclusionSkipped() || hidden.refIn != l {
		t.Error("NoteSkip не отмечен в списке или у ребёнка")
	}
	// Обход заходит в группу; перезапись внутри обхода не добавляет ссылок
	// в группу (цель записи при обходе — пустая).
	r.BeginFrame(m, true, nil)
	n := 0
	r.WalkRetained(root, func(child ListHolder, off image.Point) {
		n++
		recordChild(r, child.(*stubHolder), off)
	})
	r.EndFrame()
	if n != 1 || sub.Len() != 2 || l.Len() != 2 {
		t.Errorf("обход группы: посещено %d, в группе %d, у корня %d", n, sub.Len(), l.Len())
	}
}

func TestRecorder_CommandsClippedAndDropped(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	h := &stubHolder{name: "h"}
	r.BeginFrame(m, true, nil)
	r.BeginRecord(h, image.Point{})
	r.SetClip(image.Rect(0, 0, 50, 50))
	r.FillRect(40, 40, 100, 100, red)           // обрезается клипом
	r.FillRect(60, 60, 10, 10, red)             // целиком вне клипа — отбрасывается
	r.FillRect(0, 0, 10, 10, color.RGBA{})      // прозрачная — отбрасывается
	r.DrawTextFont("", 5, 20, 12, "", red)      // пустой текст — отбрасывается
	r.DrawTextFont("ab", 5, 20, 12, "", red)    // охват от измерителя
	r.SetRoundClip(image.Rect(0, 0, 30, 30), 4) // клип сужается до 30×30
	r.FillRect(0, 0, 100, 100, red)             // обрезается скруглённым клипом
	r.EndRecord(h)
	l, exit := r.EndFrame()
	if l.Len() != 3 {
		t.Fatalf("команд %d, ждали 3", l.Len())
	}
	if l.Cmds[0].BBox != image.Rect(40, 40, 50, 50) {
		t.Errorf("охват обрезанной заливки %v", l.Cmds[0].BBox)
	}
	if l.Cmds[1].Op != OpText || l.Cmds[1].BBox != image.Rect(5, 8, 21, 26) {
		t.Errorf("текст: %v %v", l.Cmds[1].Op, l.Cmds[1].BBox)
	}
	if l.Cmds[2].BBox != image.Rect(0, 0, 30, 30) || l.Cmds[2].Clip.Round != image.Rect(0, 0, 30, 30) {
		t.Errorf("скруглённый клип: охват %v, клип %+v", l.Cmds[2].BBox, l.Cmds[2].Clip)
	}
	if exit.NoClip || exit.Clip != image.Rect(0, 0, 30, 30) || exit.RoundR != 4 {
		t.Errorf("клип на выходе %+v", exit)
	}
}

func TestReplay_DamageClipAndDirect(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	direct := &stubHolder{name: "direct"}
	root := &stubHolder{name: "root"}
	root.draw = func(ctx DrawContext) {
		ctx.SetClip(image.Rect(0, 0, 100, 100))
		ctx.FillRect(10, 10, 20, 20, red)
		ctx.FillRect(50, 50, 20, 20, red) // тот же клип — без повторной установки
		ctx.ClearClip()
		ctx.FillRect(150, 150, 20, 20, red)
		r.Direct(direct, image.Rect(120, 10, 140, 30), image.Pt(0, 5))
	}
	r.BeginFrame(m, true, nil)
	recordChild(r, root, image.Point{})
	l, exit := r.EndFrame()

	// Полное воспроизведение: клип ставится при смене, прямой виджет
	// рисуется через сдвиг, на выходе — клип рекордера.
	var offs []image.Point
	ctx := &stubCtx{}
	Replay(ctx, l, ReplayOpts{Exit: exit, Offset: func(c DrawContext, off image.Point) DrawContext {
		offs = append(offs, off)
		return c
	}})
	if got := ctx.count("fill"); got != 3 {
		t.Errorf("заливок %d, ждали 3", got)
	}
	if got := ctx.count("clip"); got != 1 {
		t.Errorf("установок клипа %d, ждали 1 (две команды под одним клипом)", got)
	}
	if direct.draws != 1 || len(offs) != 1 || offs[0] != image.Pt(0, 5) {
		t.Errorf("прямой виджет: draws=%d, сдвиги %v", direct.draws, offs)
	}
	if last := ctx.ops[len(ctx.ops)-1]; last.name != "noclip" {
		t.Errorf("последняя операция %q, ждали снятие клипа на выходе", last.name)
	}

	// Повреждение задевает только вторую заливку: первая и третья, прямой
	// виджет — пропущены; клип всё равно ставится явно.
	ctx = &stubCtx{}
	direct.draws = 0
	Replay(ctx, l, ReplayOpts{Damage: []image.Rectangle{image.Rect(55, 55, 60, 60)}, Exit: exit})
	if got := ctx.count("fill"); got != 1 || ctx.ops[1].r != image.Rect(50, 50, 70, 70) {
		t.Errorf("по повреждению: %v", ctx.ops)
	}
	if ctx.ops[0].name != "clip" || direct.draws != 0 {
		t.Errorf("по повреждению: первая операция %q, прямых %d", ctx.ops[0].name, direct.draws)
	}
	// Повреждение вне охвата списка — только клип выхода.
	ctx = &stubCtx{}
	Replay(ctx, l, ReplayOpts{Damage: []image.Rectangle{image.Rect(190, 190, 199, 199)}, Exit: exit})
	if len(ctx.ops) != 1 || ctx.ops[0].name != "noclip" {
		t.Errorf("вне охвата: %v", ctx.ops)
	}
}

func TestRecorder_ChildRefResolvesCurrentList(t *testing.T) {
	// Контейнер удержан, ребёнок перезаписан: воспроизведение ссылки берёт
	// новый список ребёнка.
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	a := fillHolder("a", image.Rect(10, 10, 20, 20))
	root := &stubHolder{name: "root", draw: func(ctx DrawContext) { recordChild(r, a, image.Point{}) }}
	r.BeginFrame(m, true, nil)
	recordChild(r, root, image.Point{})
	l, _ := r.EndFrame()
	a.draw = func(ctx DrawContext) { ctx.FillRect(30, 30, 10, 10, red) }
	r.BeginFrame(m, true, nil)
	r.WalkRetained(root, func(child ListHolder, off image.Point) { recordChild(r, child.(*stubHolder), off) })
	r.Keep(root, image.Point{})
	r.EndFrame()
	ctx := &stubCtx{}
	Replay(ctx, l, ReplayOpts{})
	if ctx.count("fill") != 1 || ctx.ops[1].r != image.Rect(30, 30, 40, 40) {
		t.Errorf("ссылка воспроизвела не новый список: %v", ctx.ops)
	}
}
