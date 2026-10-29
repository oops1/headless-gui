//go:build linux && !android

package core

import (
	"encoding/binary"
	"image"
)

// x11argb_linux.go — окно-попап X11 с альфа-каналом (раскладка запросов и
// разбор visual'ов — в x11argb.go).
//
// Кадр оверлея — картинка движка: цвет в ней УЖЕ умножен на альфу (весь back
// холста premultiplied, см. core/raster/canvas.go), а прозрачные точки — нули.
// Композитор ждёт от окна ARGB ровно это: перестановка R и B с обрезкой
// канала альфой (swapRBRowPremul — то же, что у Wayland-попапа,
// convRectBGRAPremul). Повторное умножение на альфу затемнило бы
// полупрозрачные края, а точка с цветом больше альфы дала бы пересвет.

// x11CreateArgbPopupWindow создаёт override-redirect попап с глубиной 32 и ARGB
// visual'ом экрана. false — окно не создано (на экране нет такого visual'а
// либо сервер отверг запрос): вызывающий создаёт обычное 24-битное, как раньше.
//
// Ответа на CreateColormap/CreateWindow нет, а BadMatch приходит ошибкой
// позже, поэтому после запросов делаем круговой обмен (GetInputFocus) и
// смотрим, не пришла ли ошибка: на соединении ещё нет ни окна на экране, ни
// событий — читать напрямую безопасно (тот же приём, что у x11ShmInit).
func (w *X11Window) x11CreateArgbPopupWindow(width, height int, eventMask uint32) bool {
	if w.screen.ArgbVisual == 0 {
		return false
	}
	cmap := w.x11GenID()
	w.x11Send(x11CreateColormapRequest(cmap, w.screen.Root, w.screen.ArgbVisual))
	mask, values := x11ArgbPopupAttrs(eventMask, cmap)
	w.x11CreateWindowVisual(w.wid, w.screen.Root, 0, 0,
		uint16(width), uint16(height), 0,
		x11ArgbDepth, w.screen.ArgbVisual, mask, values)
	if w.x11SyncErrors() > 0 {
		// Откат: colormap освобождаем (если он всё же создан — иначе сервер
		// ответит ещё одной ошибкой, которую никто не читает, но и вреда нет),
		// окно создаст вызывающий под новым ID.
		w.x11Send(x11FreeColormapRequest(cmap))
		w.wid = w.x11GenID()
		return false
	}
	w.colormap = cmap
	w.argb = true
	return true
}

// x11FreeArgbColormap освобождает colormap окна с альфой (при закрытии).
// Окно и ресурсы клиента сервер снимает сам при закрытии соединения; явный
// запрос нужен для порядка и чтобы не держать colormap до его конца.
func (w *X11Window) x11FreeArgbColormap() {
	if w.colormap == 0 {
		return
	}
	cmap := w.colormap
	w.colormap = 0
	w.x11Send(x11FreeColormapRequest(cmap))
}

// imageDepth — глубина данных PutImage/ShmPutImage: у окна с альфой 32,
// у остальных 24 (глубина корневого visual'а).
func (w *X11Window) imageDepth() byte {
	if w.argb {
		return x11ArgbDepth
	}
	return 24
}

// exposePopup восстанавливает область попапа по Expose из последнего кадра.
// У окна без альфы сервер при открытии области заливает её фоном, у окна с
// альфой — прозрачным: без перерисовки меню «пропадало» бы кусками.
func (w *X11Window) exposePopup(r image.Rectangle) {
	if img := w.popupImg.Load(); img != nil {
		w.BlitRGBADirty(img, r)
	}
}

// x11SyncErrors шлёт GetInputFocus и читает пакеты до его ответа, считая
// ошибки, пришедшие раньше (сервер доставляет ошибки более ранних запросов
// прежде ответа на поздний). Безопасно только пока на соединении нет
// посторонних событий — см. x11SyncCheckError, которому нужен ровно один
// возможный пакет; здесь ошибок может быть несколько (colormap и окно).
func (w *X11Window) x11SyncErrors() int {
	req := make([]byte, 4)
	req[0] = 43 // GetInputFocus
	binary.LittleEndian.PutUint16(req[2:4], 1)
	w.x11Send(req)

	errs := 0
	pkt := make([]byte, 32)
	for {
		if !w.readFull(pkt) {
			return errs
		}
		switch pkt[0] {
		case 0:
			errs++
		case 1:
			return errs
		}
	}
}
