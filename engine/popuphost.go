// popuphost.go — вынос popup-оверлеев (dropdown, контекстные/каскадные меню)
// в отдельные нативные окна ОС («нативный вьюпорт»).
//
// Логика попапа (hover, выбор, закрытие) остаётся в этом движке: нативное
// окно-попап — лишь вьюпорт. Движок рендерит содержимое оверлея в отдельный
// маленький буфер (renderOverlay через транслирующий DrawContext) и отдаёт его
// хосту (window.popupHost) через PopupSink. Хост создаёт/двигает/закрывает
// окна ОС и транслирует их события мыши обратно в этот движок.
//
// Без установленного sink (headless и бэкенды без поддержки окон-попапов)
// поведение прежнее до пикселя: оверлеи рисуются в основной холст.
package engine

import (
	"hash/fnv"
	"image"
	"reflect"

	"github.com/oops1/headless-gui/v3/widget"
)

// PopupFrame — кадр одного активного оверлея для нативного окна-попапа.
type PopupFrame struct {
	ID   uintptr         // стабильный ключ (указатель на виджет-оверлей)
	Rect image.Rectangle // абсолютные ЛОГИЧЕСКИЕ координаты холста (могут выходить за холст)
	Img  *image.RGBA     // отрендеренный контент (ФИЗИЧЕСКИЕ пиксели, Rect.Size()×scale)
}

// popupSig — сигнатура одного попапа для детекта изменений (состав/Rect/контент).
type popupSig struct {
	id   uintptr
	rect image.Rectangle
	hash uint64
}

// popupItem — найденный в дереве хостируемый оверлей (до рендера).
type popupItem struct {
	id   uintptr
	rect image.Rectangle
	od   widget.OverlayDrawer
}

// popupEntry — межкадровый кэш одного оверлея: канвас, готовая картинка и её хэш.
type popupEntry struct {
	canvas  *Canvas
	src     *Canvas // канвас движка, из которого клонирован
	fontRev uint64
	rect    image.Rectangle
	img     *image.RGBA
	hash    uint64
	seen    uint64
}

// SetPopupSink регистрирует хост popup-оверлеев. sink вызывается из рендер-цикла
// со всеми активными оверлеями текущего кадра (пустой slice — закрыть все окна).
// nil снимает хост (оверлеи снова рисуются в холст).
//
// Дополнительно выставляет глобальный флаг widget.SetPopupsHosted — при нём
// popup-меню не клэмпятся в границы канваса (экранным позиционированием
// занимается хост).
func (e *Engine) SetPopupSink(sink func(frames []PopupFrame)) {
	e.popupMu.Lock()
	e.popupSink = sink
	e.lastPopupSig = nil
	e.popupCache = nil
	e.popupMu.Unlock()
	widget.SetPopupsHosted(sink != nil)
}

func (e *Engine) getPopupSink() func([]PopupFrame) {
	e.popupMu.Lock()
	defer e.popupMu.Unlock()
	return e.popupSink
}

// renderPopups отдаёт sink изменившиеся оверлеи (под frameMu).
// Перерисовывается только оверлей с новым Rect или в damage.
func (e *Engine) renderPopups(canvas *Canvas, root widget.Widget, modals []widget.ModalWidget,
	damage image.Rectangle, dirtyAll bool) {
	sink := e.getPopupSink()
	if sink == nil {
		return
	}

	items := e.popupItems[:0]
	if root != nil {
		items = collectPopups(items, root, 0)
	}
	for _, m := range modals {
		if m.IsModal() {
			items = collectPopups(items, m, 0)
		}
	}
	e.popupItems = items

	sig := e.popupSigBu[:0]

	e.popupMu.Lock()
	if e.popupCache == nil {
		e.popupCache = make(map[uintptr]*popupEntry)
	}
	e.popupGen++
	gen := e.popupGen
	for _, it := range items {
		ent := e.popupCache[it.id]
		if ent == nil {
			ent = &popupEntry{}
			e.popupCache[it.id] = ent
		}
		ent.seen = gen
		if ent.img == nil || ent.rect != it.rect || dirtyAll || canvas.sRect(it.rect).Overlaps(damage) {
			oc := ent.overlayCanvas(canvas, it.rect)
			ent.img = renderOverlayInto(oc, it.od, it.rect)
			ent.hash = hashRGBA(ent.img)
			ent.rect = it.rect
		}
		sig = append(sig, popupSig{id: it.id, rect: it.rect, hash: ent.hash})
	}
	for id, ent := range e.popupCache {
		if ent.seen != gen {
			delete(e.popupCache, id)
		}
	}
	e.popupSigBu = sig

	changed := !samePopupSig(e.lastPopupSig, sig)
	var frames []PopupFrame
	if changed {
		e.lastPopupSig = append([]popupSig(nil), sig...)
		frames = make([]PopupFrame, len(items))
		for i, it := range items {
			frames[i] = PopupFrame{ID: it.id, Rect: it.rect, Img: e.popupCache[it.id].img}
		}
	}
	e.popupMu.Unlock()

	if changed {
		sink(frames)
	}
}

// overlayCanvas отдаёт кэшированный канвас оверлея (или создаёт новый),
// очищенный под новый рендер.
func (ent *popupEntry) overlayCanvas(src *Canvas, r image.Rectangle) *Canvas {
	oc := ent.canvas
	if oc == nil || ent.src != src || ent.fontRev != src.fontRev ||
		oc.logicalW != r.Dx() || oc.logicalH != r.Dy() || oc.scale != src.scale {
		oc = src.cloneForSize(r.Dx(), r.Dy(), src.scale, nil)
		ent.canvas, ent.src, ent.fontRev = oc, src, src.fontRev
		return oc
	}
	clear(oc.back.Pix)
	oc.hasClip = false
	oc.hasBase = false
	return oc
}

// collectPopups рекурсивно собирает хостируемые оверлеи дерева (без рендера).
func collectPopups(out []popupItem, w widget.Widget, depth int) []popupItem {
	if tooDeep(depth) {
		return out
	}
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		if ob, ok := w.(widget.OverlayBoundsProvider); ok {
			if r := ob.OverlayBounds(); !r.Empty() {
				out = append(out, popupItem{id: widgetID(w), rect: r, od: od})
			}
		}
	}
	for _, child := range w.Children() {
		out = collectPopups(out, child, depth+1)
	}
	return out
}

// widgetID возвращает стабильный ключ виджета (указатель на его структуру).
func widgetID(w widget.Widget) uintptr {
	v := reflect.ValueOf(w)
	if v.Kind() == reflect.Ptr {
		return v.Pointer()
	}
	return 0
}

// renderOverlay рендерит оверлей od в отдельный буфер размером с r (логический),
// в физическом масштабе движка. Возвращает ФИЗИЧЕСКИЙ RGBA.
func (c *Canvas) renderOverlay(od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	return renderOverlayInto(c.cloneForSize(r.Dx(), r.Dy(), c.scale, nil), od, r)
}

// renderOverlayInto рисует оверлей в подготовленный канвас oc. Координаты
// оверлея (абсолютные логические) транслируются на -r.Min.
func renderOverlayInto(oc *Canvas, od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	tc := &translatingContext{inner: oc, dx: r.Min.X, dy: r.Min.Y}
	od.DrawOverlay(tc)
	// Копия развязывает владение с хостом: он блитит асинхронно, а канвас
	// оверлея переиспользуется следующими кадрами.
	out := image.NewRGBA(oc.back.Rect)
	copy(out.Pix, oc.back.Pix)
	return out
}

// hashRGBA — быстрый хэш пикселей (детект изменения содержимого попапа).
func hashRGBA(img *image.RGBA) uint64 {
	if img == nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(img.Pix)
	return h.Sum64()
}

// samePopupSig сравнивает две сигнатуры набора попапов (порядок стабилен —
// обход дерева детерминирован).
func samePopupSig(a, b []popupSig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ─── CloseAllOverlays ────────────────────────────────────────────────────────

// CloseAllOverlays закрывает все Dismissable-оверлеи (dropdown/меню) в дереве и
// в модалках. Используется хостом при деактивации окна-носителя (клик в другое
// приложение): системные меню в этом случае закрываются.
func (e *Engine) CloseAllOverlays() {
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root != nil {
		// keep=nil и точка вне любого экрана: закрыть ВСЕ, включая панели,
		// которые считают своей площадь соседки по группе.
		dismissOutside(root, nil, widget.CursorNowhere, widget.CursorNowhere)
	}
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		dismissOutside(m, nil, widget.CursorNowhere, widget.CursorNowhere)
	}
}
