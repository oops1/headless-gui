package engine

// toplayers.go — слои поверх дерева на пути команд (ADR-0007, задача 4а.4).
//
// Оверлеи в холсте, затемнение и диалог модального окна, его оверлеи и
// подсказка записываются рекордером в список кадра следом за деревом корня —
// в том порядке, в каком прямой путь рисует их в холст. Дальше они — команды:
// их растрят воркеры, их тайлы хэшируются (ADR-0006), слой размытия меню
// идёт через кэш с ореолом.
//
// Переходы оверлеев (overlaytrans.go) смешивают снимки «под» и «с»
// попиксельно — командой это не выразить. При включённых переходах обход
// записывает оверлеи, пока не встретит оверлей под присмотром (или владельца,
// погасившего такой оверлей); с этого места всё, что рисуется дальше в
// порядке слоёв — остаток оверлеев слоя, модальные окна выше, подсказка, —
// откладывается (postItems) и после растра рисуется, как прежде, в окно back
// холста над объединением их мест (postRect). Порядок слоёв сохранён: что
// ниже точки переключения — в списке команд, что выше — поверх растра.

import (
	"image"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/core/input"
	"github.com/oops1/headless-gui/v3/core/overlay"
	"github.com/oops1/headless-gui/v3/widget"
)

// postKind — что кадр рисует в холст после растра.
type postKind uint8

const (
	postOverlay  postKind = iota // оверлей слоя (ovtVisit: под присмотром или мимо него)
	postClosed                   // владелец погасил оверлей под присмотром (ovtVisitClosed)
	postLayerEnd                 // конец обхода слоя (ovtLayerEnd)
	postModal                    // модальное окно целиком: затемнение, диалог, его оверлеи
	postTooltip                  // плашка подсказки
)

// postItem — отложенный слой.
type postItem struct {
	kind  postKind
	w     widget.Widget
	od    widget.OverlayDrawer
	off   image.Point
	layer widget.Widget
	tr    *overlayTrans
	m     widget.ModalWidget
}

// SetSingleBuffer включает или выключает один буфер кадра (ADR-0007): back
// холста движка — окно над front под то, что кадр рисует в холст сам, тайлы
// растра воркеры сравнивают с front из своих окон. По умолчанию включён.
// Выключенный — окно back всегда размером с кадр: прежняя модель двух
// буферов. Картинка на обоих путях одна и та же.
func (e *Engine) SetSingleBuffer(on bool) {
	e.single.Store(on)
	e.Invalidate()
}

// SingleBuffer сообщает, включён ли один буфер кадра.
func (e *Engine) SingleBuffer() bool { return e.single.Load() }

// recordTop записывает слои поверх дерева в список кадра (рекордер открыт
// recordFrame, дерево корня уже записано) или откладывает их до конца растра.
func (e *Engine) recordTop(rec *hgdraw.Recorder, canvas *Canvas, root widget.Widget, modals []widget.ModalWidget, hosted, on bool) {
	e.clearPost()
	e.topLayer(root, rec, canvas, hosted, on)
	for _, m := range modals {
		if !m.IsModal() {
			continue
		}
		if e.postMode {
			e.addPost(postItem{kind: postModal, m: m, layer: m}, canvas.Frame())
			continue
		}
		// Затемнение фона (логические координаты) и диалог — как на прямом
		// пути, только командами; диалог удерживает список, как корень.
		if dim := m.DimColor(); dim.A > 0 {
			lw, lh := canvas.LogicalSize()
			rec.FillRectAlpha(0, 0, lw, lh, dim)
		}
		widget.DrawTraced(m, rec)
		e.topLayer(m, rec, canvas, hosted, on)
	}
	tt := e.tooltipPlan(canvas, root)
	e.postTooltip = tt
	if tt.OK {
		if e.postMode {
			e.addPost(postItem{kind: postTooltip}, canvas.PhysRect(tt.Rect))
		} else {
			e.drawTooltipBox(rec, tt.Text, tt.Rect)
		}
	}
}

// clearPost забывает отложенные слои прошлого кадра (и ссылки на виджеты).
func (e *Engine) clearPost() {
	for i := range e.post {
		e.post[i] = postItem{}
	}
	e.post = e.post[:0]
	e.postRect = image.Rectangle{}
	e.postMode = false
	e.postTooltip = overlay.TooltipPlan{}
}

// addPost откладывает слой и расширяет место после растра на его место
// (физическое).
func (e *Engine) addPost(it postItem, place image.Rectangle) {
	e.post = append(e.post, it)
	e.postRect = e.postRect.Union(place)
}

// topLayer записывает или откладывает оверлеи слоя layer (корень или
// модальное окно) — тот же обход, что drawOverlayLayer.
func (e *Engine) topLayer(layer widget.Widget, rec *hgdraw.Recorder, canvas *Canvas, hosted, on bool) {
	if on {
		e.ovtSetLayer(layer)
	}
	e.topWalk(layer, rec, canvas, hosted, on, layer, image.Point{}, 0)
	switch {
	case e.postMode:
		e.addPost(postItem{kind: postLayerEnd, layer: layer}, image.Rectangle{})
	case on:
		// Ни одного оверлея под присмотром в слое не встретилось: переходы
		// слоя, чьи владельцы не встретились, уходят сразу — как после обхода
		// на прямом пути.
		e.ovtLayerEnd(layer)
	}
}

// topWalk — drawOverlaysAt/drawOverlaysT над рекордером.
func (e *Engine) topWalk(w widget.Widget, rec *hgdraw.Recorder, canvas *Canvas, hosted, on bool, layer widget.Widget, off image.Point, depth int) {
	if tooDeep(depth) {
		return
	}
	if od, ok := w.(widget.OverlayDrawer); ok {
		switch {
		case od.HasOverlay():
			if !hosted || !isHostedOverlay(w) {
				e.topOverlay(w, od, rec, canvas, on, layer, off)
			}
		case on && len(e.ovt.byKey) > 0:
			if tr := e.ovt.byKey[overlay.WidgetID(w)]; tr != nil {
				// Исчезающий оверлей смешивается на месте владельца в порядке
				// рисования: с него и дальше — после растра.
				e.postMode = true
				e.addPost(postItem{kind: postClosed, tr: tr, layer: layer}, tr.rest)
			}
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return
	}
	coff := off.Add(input.ContentShift(w))
	for _, child := range children {
		e.topWalk(child, rec, canvas, hosted, on, layer, coff, depth+1)
	}
}

// topOverlay записывает открытый оверлей или откладывает его.
func (e *Engine) topOverlay(w widget.Widget, od widget.OverlayDrawer, rec *hgdraw.Recorder, canvas *Canvas, on bool, layer widget.Widget, off image.Point) {
	it := postItem{kind: postOverlay, w: w, od: od, off: off, layer: layer}
	if e.postMode {
		e.addPost(it, e.overlayPlace(w, canvas, off))
		return
	}
	if on {
		if rest, managed := e.ovtRestOf(w, canvas, off); managed {
			e.postMode = true
			e.addPost(it, rest)
			return
		}
		// Оверлей мимо присмотра: записывается; его место — как у ovtDrawOther
		// (переходу, который он накрывает, шагать без перерисовки нельзя).
		r := input.OverlayHitRect(w).Sub(off).Inset(-widget.DefaultOverlayMargin())
		e.ovt.others = append(e.ovt.others, ovtOther{layer: layer, rect: canvas.PhysRect(r)})
	}
	widget.DrawOverlayTraced(w, od, widget.OffsetContext(rec, off.X, off.Y))
}

// ovtRestOf — место покоя оверлея под присмотром переходов (физическое, с
// запасом под тень) и берёт ли его присмотр: те же условия, что у ovtVisit.
func (e *Engine) ovtRestOf(w widget.Widget, canvas *Canvas, off image.Point) (image.Rectangle, bool) {
	ob, ok := w.(widget.OverlayBoundsProvider)
	if !ok || overlay.WidgetID(w) == 0 {
		return image.Rectangle{}, false
	}
	if sa, ok := w.(widget.SelfAnimatedOverlay); ok && sa.AnimatesOverlay() {
		return image.Rectangle{}, false
	}
	bounds := ob.OverlayBounds()
	if bounds.Empty() {
		return image.Rectangle{}, false
	}
	margin := widget.DefaultOverlayMargin()
	if om, ok := w.(widget.OverlayMarginer); ok {
		margin = om.OverlayMargin()
	}
	rest := canvas.PhysRect(bounds.Sub(off).Inset(-margin)).Intersect(canvas.Frame())
	if rest.Empty() {
		return image.Rectangle{}, false
	}
	return rest, true
}

// overlayPlace — место отложенного оверлея мимо присмотра: границы с запасом,
// если он их объявляет и не анимирует себя сам, иначе весь кадр (рисовать он
// может где угодно).
func (e *Engine) overlayPlace(w widget.Widget, canvas *Canvas, off image.Point) image.Rectangle {
	if rest, ok := e.ovtRestOf(w, canvas, off); ok {
		return rest
	}
	return canvas.Frame()
}

// drawPost рисует отложенные слои в холст после растра — в порядке, в каком
// их нарисовал бы прямой путь после точки переключения.
func (e *Engine) drawPost(canvas *Canvas, hosted, on bool) {
	var cur widget.Widget
	for i := range e.post {
		it := &e.post[i]
		if on && it.layer != nil && it.layer != cur && it.kind != postModal {
			e.ovtSetLayer(it.layer)
			cur = it.layer
		}
		switch it.kind {
		case postOverlay:
			if on {
				e.ovtVisit(it.w, it.od, canvas, it.layer, it.off)
			} else {
				widget.DrawOverlayTraced(it.w, it.od, widget.OffsetContext(canvas, it.off.X, it.off.Y))
			}
		case postClosed:
			e.ovtVisitClosed(it.tr, canvas)
		case postLayerEnd:
			e.ovtLayerEnd(it.layer)
		case postModal:
			m := it.m
			if dim := m.DimColor(); dim.A > 0 {
				lw, lh := canvas.LogicalSize()
				canvas.FillRectAlpha(0, 0, lw, lh, dim)
			}
			widget.DrawTraced(m, canvas)
			e.drawOverlayLayer(m, canvas, hosted, on)
			cur = nil // drawOverlayLayer перестроил byKey под m
		case postTooltip:
			e.drawTooltipBox(canvas, e.postTooltip.Text, e.postTooltip.Rect)
		}
	}
	for i := range e.post {
		e.post[i] = postItem{} // ссылки на виджеты между кадрами не держим
	}
	e.post = e.post[:0]
}
