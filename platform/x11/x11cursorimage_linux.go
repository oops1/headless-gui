//go:build linux && !android

package x11

// x11cursorimage_linux.go — X11: курсор из картинки (Engine.SetCursorImage).
// Протокол — в x11cursorimage.go.
//
// Построение курсора RENDER-ом требует узнать у сервера код расширения и
// идентификатор формата ARGB32, то есть запросов с ответом, а ответ можно читать
// только там, где соединение читает один читатель. Соединение читает цикл
// событий, поэтому SetCursorImage лишь запоминает просьбу (pending) и будит цикл
// безвредным событием (x11WakeRequest); построение и показ идут в цикле, между событиями
// (x11SyncCursorImage), как отложенная подписка на касания (x11SyncTouch).
//
// Порядок с формами курсора. SetCursor тоже может прийти с другой горутины: он
// отзывает ещё не показанную картинку, и показанным остаётся то, что вызвали
// позже. Всё состояние — под w.cursors.mu, как у форм курсора (там же
// выдаются идентификаторы ресурсов).

import (
	"sync/atomic"

	"github.com/oops1/headless-gui/v4/platform/pointer"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Состояние поддержки RENDER.
const (
	x11RenderUnknown = iota // ещё не спрашивали
	x11RenderOK             // есть, версия и формат подходят
	x11RenderNone           // нет: курсор строится 1-битным
)

// x11CursorImages — курсоры-картинки окна. Всё под w.cursors.mu, кроме looping.
type x11CursorImages struct {
	pending  *base.CursorImage          // просьба, ещё не исполненная
	cache    *pointer.CursorLRU[uint32] // готовые курсоры по ключу картинки
	shown    uint32                     // курсор, повешенный на окно сейчас (0 — нет)
	shownKey [32]byte                   // ключ картинки показанного курсора
	render   int                        // x11RenderUnknown / OK / None
	major    byte                       // код расширения RENDER
	argb     uint32                     // формат ARGB32 сервера
	verified bool                       // первый курсор RENDER прошёл проверку сервером
	created  int                        // сколько курсоров построено (тесты)

	// looping — цикл событий читает соединение и его можно будить.
	looping atomic.Bool
}

// SetCursorImage показывает курсор из картинки. Реализует cursorImageSetter.
// Безопасен из любой горутины; показ происходит в цикле событий окна.
func (w *X11Window) SetCursorImage(ci *base.CursorImage) {
	if w.wid == 0 || ci == nil {
		return
	}
	w.cursors.mu.Lock()
	im := &w.curImg
	if im.pending == nil && im.shown != 0 && im.shownKey == ci.Key {
		w.cursors.mu.Unlock()
		return // уже показана
	}
	im.pending = ci
	w.cursors.known = false // следующая форма должна дойти до сервера
	looping := im.looping.Load()
	w.cursors.mu.Unlock()
	if looping {
		w.x11Send(x11WakeRequest(w.wid)) // разбудить цикл: он исполнит просьбу
	}
}

// x11SyncCursorImage исполняет отложенную просьбу показать курсор-картинку.
// Зовётся циклом событий между событиями (и до первого чтения), где запрос с
// ответом безопасен.
func (w *X11Window) x11SyncCursorImage() {
	w.cursors.mu.Lock()
	defer w.cursors.mu.Unlock()
	im := &w.curImg
	ci := im.pending
	im.pending = nil
	if ci == nil {
		return
	}
	id := w.cursorImageLocked(ci)
	if id == 0 {
		return
	}
	im.shown, im.shownKey = id, ci.Key
	w.x11SetWindowCursor(w.wid, id)
}

// cursorImageLocked возвращает курсор картинки: из кэша либо новый. Вызывать
// под w.cursors.mu в цикле событий. 0 — построить не вышло.
func (w *X11Window) cursorImageLocked(ci *base.CursorImage) uint32 {
	im := &w.curImg
	if im.cache == nil {
		im.cache = pointer.NewCursorLRU[uint32](pointer.CursorCacheSize,
			func(id uint32) { w.x11Send(x11FreeCursorRequest(id)) },
			func(id uint32) bool { return id == im.shown })
	}
	if id, ok := im.cache.Get(ci.Key); ok {
		return id
	}
	if im.render == x11RenderUnknown {
		w.x11NegotiateRenderLocked()
	}
	id := w.x11GenID()
	if im.render == x11RenderOK {
		ids := x11RenderCursorIDs{
			cursor: id, drawable: w.wid,
			pixmap: w.x11GenID(), gc: w.x11GenID(), picture: w.x11GenID(),
		}
		for _, req := range x11RenderCursorRequests(ids, im.major, im.argb, ci) {
			w.x11Send(req)
		}
		if !im.verified {
			// Первый курсор RENDER проверяем: отказ (BadAlloc, нет поддержки
			// глубины 32) приходит ошибкой, а не ответом. Отказал — дальше
			// 1-битные.
			if !w.x11SyncCheck() {
				im.render = x11RenderNone
				id = w.x11GenID()
			} else {
				im.verified = true
			}
		}
	}
	if im.render == x11RenderNone {
		for _, req := range x11MonoImageCursorRequests(w.x11CursorIDs(id, w.wid), ci) {
			w.x11Send(req)
		}
	}
	im.created++
	im.cache.Put(ci.Key, id)
	return id
}

// x11NegotiateRenderLocked узнаёт у сервера, есть ли RENDER ≥ 0.5 и формат
// ARGB32. Любая неудача — x11RenderNone. Вызывать в цикле событий, под
// w.cursors.mu.
func (w *X11Window) x11NegotiateRenderLocked() {
	im := &w.curImg
	im.render = x11RenderNone
	reply, ok := w.x11Reply(x11QueryExtensionRequest(renderExtensionName))
	if !ok {
		return
	}
	major, _, present := parseQueryExtensionReply(reply)
	if !present {
		return
	}
	reply, ok = w.x11Reply(x11RenderQueryVersionRequest(major))
	if !ok {
		return
	}
	vmaj, vmin, ok := parseRenderVersion(reply)
	if !ok || !renderVersionHasCursor(vmaj, vmin) {
		return
	}
	reply, ok = w.x11Reply(x11RenderQueryPictFormatsRequest(major))
	if !ok {
		return
	}
	argb := parseRenderArgbFormat(reply)
	if argb == 0 {
		return
	}
	im.major, im.argb, im.render = major, argb, x11RenderOK
}
