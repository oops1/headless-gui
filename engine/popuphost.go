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
	e.popupMu.Unlock()
	widget.SetPopupsHosted(sink != nil)
}

func (e *Engine) getPopupSink() func([]PopupFrame) {
	e.popupMu.Lock()
	defer e.popupMu.Unlock()
	return e.popupSink
}

// renderPopups собирает активные хостируемые оверлеи, рендерит каждый в свой
// буфер и, если состав/содержимое/Rect изменились с прошлого кадра, вызывает
// sink. Вызывается из renderFrame под frameMu (canvas стабилен).
func (e *Engine) renderPopups(canvas *Canvas, root widget.Widget, modals []widget.ModalWidget) {
	sink := e.getPopupSink()
	if sink == nil {
		return
	}

	var frames []PopupFrame
	if root != nil {
		collectPopups(root, canvas, &frames)
	}
	for _, m := range modals {
		if m.IsModal() {
			collectPopups(m, canvas, &frames)
		}
	}

	// Сигнатура набора: id+rect+хэш пикселей. Вызываем sink только при отличии.
	sig := make([]popupSig, len(frames))
	for i, f := range frames {
		sig[i] = popupSig{id: f.ID, rect: f.Rect, hash: hashRGBA(f.Img)}
	}

	e.popupMu.Lock()
	changed := !samePopupSig(e.lastPopupSig, sig)
	if changed {
		e.lastPopupSig = sig
	}
	e.popupMu.Unlock()

	if changed {
		sink(frames)
	}
}

// collectPopups рекурсивно рендерит хостируемые оверлеи дерева в буферы.
func collectPopups(w widget.Widget, canvas *Canvas, out *[]PopupFrame) {
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		if ob, ok := w.(widget.OverlayBoundsProvider); ok {
			if r := ob.OverlayBounds(); !r.Empty() {
				*out = append(*out, PopupFrame{
					ID:   widgetID(w),
					Rect: r,
					Img:  canvas.renderOverlay(od, r),
				})
			}
		}
	}
	for _, child := range w.Children() {
		collectPopups(child, canvas, out)
	}
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
// в физическом масштабе движка. Все координаты оверлея (абсолютные логические)
// транслируются на -r.Min через translatingContext. Возвращает ФИЗИЧЕСКИЙ RGBA.
func (c *Canvas) renderOverlay(od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	oc := c.cloneForSize(r.Dx(), r.Dy(), c.scale, nil)
	tc := &translatingContext{inner: oc, dx: r.Min.X, dy: r.Min.Y}
	od.DrawOverlay(tc)
	// Отдаём копию back-буфера: cloneForSize-канвас переиспользоваться не будет,
	// но копия развязывает владение с хостом (он блитит асинхронно).
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
		dismissOutside(root, nil) // keep=nil → закрыть все
	}
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		dismissOutside(m, nil)
	}
}
