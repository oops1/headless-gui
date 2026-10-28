package widget

// retain.go — удержание списков команд у виджетов (ADR-0004, задача 4а.1).
//
// Когда контекст отрисовки — рекордер команд (core/draw.Recorder), DrawTraced
// не зовёт Draw виджета, чей список команд ещё верен: в список родителя
// кладётся ссылка на удержанный список. Верность списка (И1) складывается из
// меток виджета и окружения записи:
//
//   - drawDirty — собственные команды устарели (Invalidate, SetBounds, смена
//     состава детей, сломанность); subtreeDirty — устарел кто-то ниже:
//     контейнер сам не рисуется, рекордер идёт по ссылкам его списка;
//   - refIn — список родителя, в котором виджет числится (ссылкой или
//     решением «пропущен»). Пометка поднимается к родителю как subtreeDirty,
//     только пока родитель держит именно этот список; иначе в текущем списке
//     родителя виджета нет (его держат мимо DrawChildren, он был скрыт,
//     прокручен за край) — и родитель помечается dirty, чтобы перезаписаться
//     и снова решить, рисовать ли его;
//   - recParent — контейнер, в чей список попала ссылка при последней
//     записи: путь наверх для детей, которых держат мимо AddChild (значки
//     TrayGroup), — у них нет parent.
//
// Метки атомарные: ставятся из любой горутины (сеттеры виджетов зовут
// Invalidate откуда угодно), снимаются только горутиной кадра, и снимаются
// ДО Draw (Swap): Invalidate, пришедший во время записи, остаётся на
// следующий кадр, а не теряется. Паника в Draw возвращает метку
// (Recorder.Abort), список публикуется только после нормального возврата.
// Поддерево Invalidate не обходит на горутине вызывающего (дети меняются на
// горутине кадра, обход гонялся бы с AddChild): ставится treeDirty, а
// раскрывает его горутина кадра при визите, по Children() и ExtraChildren.
// InvalidateSubtree и смена темы области обходят поддерево сразу — их зовут с
// горутины движка.

import (
	"image"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
)

// DirectDrawer — см. draw.DirectDrawer: виджет, которого нельзя записать
// (ExternalSurface); при воспроизведении его Draw зовётся прямо в холст.
type DirectDrawer = hgdraw.DirectDrawer

// ListHolder — см. draw.ListHolder; его реализует всякий виджет с Base.
type ListHolder = hgdraw.ListHolder

var _ ListHolder = (*retainProbe)(nil)

// retainProbe — проверка типа: Base с Draw удовлетворяет ListHolder.
type retainProbe struct{ Base }

func (*retainProbe) Draw(DrawContext) {}

// ExtraChildrenLister — контейнер, который рисует (или будет рисовать)
// виджеты, не входящие в Children(): содержимое неактивных вкладок
// TabControl, значки TrayGroup. Пометка поддерева (Invalidate контейнера,
// InvalidateSubtree, смена темы) доходит и до них — иначе их списки команд
// пережили бы изменение и воспроизвелись устаревшими.
type ExtraChildrenLister interface {
	ExtraChildren() []Widget
}

// CmdList — удержанный список команд виджета (nil — не записывался).
func (b *Base) CmdList() *hgdraw.List { return b.cmdList.Load() }

// SetCmdList публикует список команд виджета. Зовёт рекордер после
// нормального возврата Draw.
func (b *Base) SetCmdList(l *hgdraw.List) { b.cmdList.Store(l) }

// NoteRef запоминает список родителя, в котором виджет числится.
func (b *Base) NoteRef(parent *hgdraw.List) { b.refIn.Store(parent) }

// MarkDirty помечает собственные команды виджета устаревшими и поднимает
// пометку по предкам: ближайший кадр вызовет его Draw заново. Для программы,
// которая изменила то, по чему виджет рисует, не трогая его сеттеров, и
// заявила область сама (Engine.InvalidateRect заявленную область и так
// перезаписывает — этот вызов точнее: один виджет, а не всё под областью).
func (b *Base) MarkDirty() { b.markDirty() }

// SetDirtyOnChildInvalidate объявляет, что виджет рисует по состоянию своих
// потомков (рамка фокуса панели задач поверх кнопок): Invalidate потомка
// помечает его dirty, а не subtreeDirty, и его собственные команды
// перезаписываются вместе с потомком.
func (b *Base) SetDirtyOnChildInvalidate(v bool) { b.dirtyOnChild = v }

// IsDrawDirty сообщает, ждёт ли виджет перезаписи (для тестов).
func (b *Base) IsDrawDirty() bool { return b.drawDirty.Load() }

// markDirty — сам dirty, предки по правилу markUp.
func (b *Base) markDirty() {
	b.drawDirty.Store(true)
	b.markUp()
}

// markDirtyTree — сам dirty, поддерево — на горутине кадра (treeDirty),
// предки по правилу markUp: Base.Invalidate.
func (b *Base) markDirtyTree() {
	b.drawDirty.Store(true)
	b.treeDirty.Store(true)
	b.markUp()
}

// markChildren помечает устаревшими всех детей w — по Children() и
// ExtraChildren — с их поддеревьями.
func markChildren(w Widget, depth int) {
	for _, c := range w.Children() {
		markSubtree(c, depth+1)
	}
	if x, ok := w.(ExtraChildrenLister); ok {
		for _, c := range x.ExtraChildren() {
			markSubtree(c, depth+1)
		}
	}
}

// markDirtyAndParent — сам и родитель dirty (границы, видимость, состав
// детей меняют и то, что рисует родитель: его порядок, occlusion, клип),
// выше — по правилу markUp.
func (b *Base) markDirtyAndParent() {
	b.drawDirty.Store(true)
	if p := b.parentOrRec(); p != nil {
		p.drawDirty.Store(true)
		p.markUp()
	}
}

// parentOrRec — контейнер виджета: по AddChild, а без него — по записи.
func (b *Base) parentOrRec() *Base {
	if p := b.parent.Load(); p != nil {
		return p
	}
	return b.recParent.Load()
}

// markUp поднимает пометку по предкам: родитель, который держит виджет в
// своём текущем списке, получает subtreeDirty, остальные — dirty (см.
// шапку файла). Предел глубины — страховка от цикла в дереве.
func (b *Base) markUp() {
	c := b
	for i := 0; i < maxScreenRectDepth; i++ {
		p := c.parentOrRec()
		if p == nil {
			return
		}
		if p.dirtyOnChild || c.refIn.Load() != p.cmdList.Load() {
			p.drawDirty.Store(true)
		} else {
			p.subtreeDirty.Store(true)
		}
		c = p
	}
}

// InvalidateSubtree помечает устаревшими виджет w и всё его поддерево (по
// Children(), в том числе у виджетов без Base) и заявляет область поддерева:
// после смены того, по чему рисует каждый из них (тема области ThemeScope).
// Для виджета с Base то же делает его Invalidate.
func InvalidateSubtree(w Widget) {
	if w == nil {
		return
	}
	markSubtree(w, 0)
	r := SubtreeRect(w)
	if bp, ok := w.(baseProvider); ok {
		b := bp.baseRef()
		b.markUp()
		notifyOwnedRectChanged(screenRect(b.parent.Load(), r))
		return
	}
	notifyRectChanged(r)
}

// markWidgetDirty — MarkDirty для виджета любого типа (чужой — без меток).
func markWidgetDirty(w Widget) {
	if bp, ok := w.(baseProvider); ok {
		bp.baseRef().markDirty()
	}
}

func markSubtree(w Widget, depth int) {
	if w == nil || depth > 64 {
		return
	}
	if bp, ok := w.(baseProvider); ok {
		b := bp.baseRef()
		b.drawDirty.Store(true)
		b.treeDirty.Store(false) // поддерево помечается здесь же
	}
	markChildren(w, depth)
}

// ─── Контекст записи ─────────────────────────────────────────────────────────

// retainerOf находит рекордер под контекстом: сам рекордер или обёртка со
// сдвигом над ним (вложенные сдвиги складываются). ok=false — контекст без
// удержания: холст, чужая обёртка.
func retainerOf(ctx DrawContext) (r *hgdraw.Recorder, off image.Point, ok bool) {
	switch c := ctx.(type) {
	case *hgdraw.Recorder:
		return c, image.Point{}, true
	case *svOffsetCtx:
		return c.retainer()
	case *svOffsetCtxAA:
		return c.retainer()
	}
	return nil, image.Point{}, false
}

// retainer — рекордер под обёрткой со сдвигом и накопленный сдвиг.
func (o *svOffsetCtx) retainer() (*hgdraw.Recorder, image.Point, bool) {
	r, off, ok := retainerOf(o.inner)
	if !ok {
		return nil, image.Point{}, false
	}
	return r, off.Add(image.Pt(o.dx, o.dy)), true
}

// holderBase — Base холдера (nil — чужая реализация).
func holderBase(h ListHolder) *Base {
	if bp, ok := h.(baseProvider); ok {
		return bp.baseRef()
	}
	return nil
}

// drawRetained — DrawTraced под рекордером: решает по меткам и окружению,
// брать ли удержанный список виджета, идти ли по его ссылкам или звать
// Draw заново (И1).
func drawRetained(w Widget, h ListHolder, b *Base, r *hgdraw.Recorder, off image.Point, ctx DrawContext) {
	if p := r.Parent(); p != nil {
		if pb := holderBase(p); pb != nil {
			b.recParent.Store(pb)
		}
	}
	// Invalidate пометил поддерево отложенно: раскрыть здесь, на горутине
	// кадра, где дети не меняются (до снятия меток и до любой ветки).
	if b.treeDirty.Swap(false) {
		markChildren(w, 0)
	}
	if d, ok := w.(DirectDrawer); ok && d.DrawDirect() {
		b.drawDirty.Store(false)
		b.subtreeDirty.Store(false)
		r.Direct(h, SubtreeRect(w), off)
		return
	}
	// Охват нужен только ничейному повреждению: без него его не считаем.
	var bounds image.Rectangle
	if r.HasForce() {
		bounds = SubtreeRect(w)
	}
	// Метки снимаются до Draw: пришедшие во время записи — на следующий кадр.
	dirty := b.drawDirty.Swap(false)
	sub := b.subtreeDirty.Swap(false)
	if !dirty && !r.CanReuse(h, bounds, off) {
		dirty = true
	}
	if !dirty && sub {
		// Контейнер цел, внизу кто-то устарел: по ссылкам его списка, каждого
		// ребёнка — в окружении его записи. Контейнер, пропускавший детей по
		// occlusion, решал по непрозрачным областям соседей: если у
		// перезаписанного ребёнка она изменилась (стекло вместо сплошного
		// фона), решение устарело — контейнер перезаписывается сам.
		occ := h.CmdList().OcclusionSkipped()
		opaqueChanged := false
		stale := r.WalkRetained(h, func(child ListHolder, coff image.Point) {
			cw, ok := child.(Widget)
			if !ok {
				return
			}
			cb := holderBase(child)
			// Чистый ребёнок в удержанном списке: ссылка на него уже там,
			// делать нечего — ни визита, ни Keep.
			if cb != nil && !cb.drawDirty.Load() && !cb.subtreeDirty.Load() &&
				(!r.HasForce() || !r.ForceHits(SubtreeRect(cw), coff)) {
				return
			}
			cctx := ctx
			if coff != off {
				cctx = OffsetContext(r, coff.X, coff.Y)
			}
			DrawTraced(cw, cctx)
			if occ && cb != nil && opaqueSig(cw) != cb.opaqueSig.Load() {
				opaqueChanged = true
			}
		})
		// Устарел: ребёнок оставляет на выходе другой клип (под ним лежит
		// всё, что записано после) или сменил непрозрачную область.
		if stale || opaqueChanged {
			dirty = true
		}
	}
	if r.ParentOcclusionSkipped() {
		b.opaqueSig.Store(opaqueSig(w))
	}
	if !dirty {
		r.Keep(h, off)
		return
	}
	r.BeginRecord(h, off)
	if drawSlowPath.Load() > 0 {
		drawTracedSlow(w, ctx)
	} else {
		w.Draw(ctx)
	}
	r.EndRecord(h)
}

// opaqueSig — подпись непрозрачной области виджета (OpaqueRegioner): по ней
// контейнер, пропускающий закрытых детей, узнаёт, что решение устарело.
func opaqueSig(w Widget) uint64 {
	o, ok := w.(OpaqueRegioner)
	if !ok {
		return 0
	}
	const offset, prime = 14695981039346656037, 1099511628211
	h := uint64(offset)
	for _, r := range o.OpaqueRegion() {
		for _, v := range [4]int{r.Min.X, r.Min.Y, r.Max.X, r.Max.Y} {
			h = (h ^ uint64(int64(v))) * prime
		}
	}
	return h
}

// noteSkipped сообщает рекордеру (если контекст — он), что контейнер не
// рисует ребёнка в этом кадре.
func noteSkipped(ctx DrawContext, child Widget, occlusion bool) {
	r, _, ok := retainerOf(ctx)
	if !ok {
		return
	}
	if h, ok := child.(ListHolder); ok {
		r.NoteSkip(h, occlusion)
	}
}
