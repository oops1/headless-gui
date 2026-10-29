//go:build windows

package core

// mouse_windows.go — Win32: уход указателя (WM_MOUSELEAVE), боковые кнопки
// мыши (WM_XBUTTON*) и формы курсора, которых нет среди IDC_*.

import (
	"unsafe"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/pointer"
)

const (
	// Боковые кнопки мыши. wParam: старшее слово — какая из них, младшее —
	// состояние остальных кнопок и модификаторов.
	wmXbuttondown   = 0x020B
	wmXbuttonup     = 0x020C
	wmXbuttondblclk = 0x020D

	// WM_MOUSELEAVE — указатель покинул клиентскую область (приходит только
	// после TrackMouseEvent(TME_LEAVE), см. platform/pointer/pointerleave.go).
	wmMouseleave = 0x02A3

	tmeLeave     = 0x00000002 // TME_LEAVE
	tmeNonclient = 0x00000010 // TME_NONCLIENT

	idcWait    = 32514
	idcCross   = 32515
	idcSizeAll = 32646

	// artCursorKey — смещение ключей cursorCache для нарисованных курсоров:
	// ключи IDC_* лежат ниже.
	artCursorKey = 1 << 16
)

var (
	procTrackMouseEvent = user32.NewProc("TrackMouseEvent")
	procWindowFromPoint = user32.NewProc("WindowFromPoint")
	procDestroyCursor   = user32.NewProc("DestroyCursor")
)

// trackMouseEvent — TRACKMOUSEEVENT.
type trackMouseEvent struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

// ─── Уход указателя ─────────────────────────────────────────────────────────

// requestTrack просит систему прислать WM_MOUSELEAVE (или WM_NCMOUSELEAVE при
// tmeNonclient). false — система отказала.
func (w *Win32Window) requestTrack(hwnd uintptr, flags uint32) bool {
	if w.trackMouse != nil {
		return w.trackMouse(hwnd, flags)
	}
	tme := trackMouseEvent{DwFlags: flags, HwndTrack: hwnd}
	tme.CbSize = uint32(unsafe.Sizeof(tme))
	r, _, _ := procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
	return r != 0
}

// clientMouseMove — WM_MOUSEMOVE: первое движение после входа взводит запрос на
// уход указателя, затем движение уходит приложению.
func (w *Win32Window) clientMouseMove(hwnd uintptr, x, y int) {
	if w.leave.NeedArm() {
		w.leave.ArmedOK(w.requestTrack(hwnd, tmeLeave))
	}
	if w.onMouseMove != nil {
		w.onMouseMove(x, y)
	}
}

// ncButtonMouseMove — WM_NCMOUSEMOVE над кнопкой заголовка, нарисованной
// приложением: подсветку ведёт приложение, а об уходе с кнопки система скажет
// только тому, кто попросил (WM_NCMOUSELEAVE).
func (w *Win32Window) ncButtonMouseMove(hwnd uintptr, x, y int) {
	if w.ncLeave.NeedArm() {
		w.ncLeave.ArmedOK(w.requestTrack(hwnd, tmeLeave|tmeNonclient))
	}
	if w.onMouseMove != nil {
		w.onMouseMove(x, y)
	}
}

// ncMouseLeft — WM_NCMOUSELEAVE: указатель ушёл с кнопки заголовка. Запрос
// одноразовый, снимаем отметку и уводим мышь за окно.
func (w *Win32Window) ncMouseLeft() {
	w.ncLeave.Left()
	if w.onMouseMove != nil {
		w.onMouseMove(pointer.Outside, pointer.Outside)
	}
}

// handleMouseLeave — WM_MOUSELEAVE: берёт положение указателя и размеры окна у
// системы и решает, настоящий ли это уход.
func (w *Win32Window) handleMouseLeave(hwnd uintptr) {
	w.leave.Left()
	var pt point
	have, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	var wr rect
	if hwnd != 0 {
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	}
	w.mouseLeft(int(pt.X), int(pt.Y), have != 0, wr)
}

// mouseLeft сообщает приложению об уходе указателя — движением за границу
// окна, как LeaveNotify на X11 и wl_pointer.leave на Wayland. (sx, sy) —
// положение указателя на экране, wr — прямоугольник окна.
//
// Переход на кнопку заголовка, которую нарисовало приложение, уходом не
// считается: см. pointer.MouseLeaveReal.
func (w *Win32Window) mouseLeft(sx, sy int, havePos bool, wr rect) {
	inside := havePos && sx >= int(wr.Left) && sx < int(wr.Right) &&
		sy >= int(wr.Top) && sy < int(wr.Bottom)
	under := native.HitClient
	if inside {
		under = w.hitAreaKind(sx-int(wr.Left), sy-int(wr.Top))
	}
	if !pointer.MouseLeaveReal(under, inside) {
		return
	}
	if w.onMouseMove != nil {
		w.onMouseMove(pointer.Outside, pointer.Outside)
	}
}

// hitAreaKind спрашивает приложение, что лежит в точке окна (физические
// пиксели). Без колбэка — обычное содержимое.
func (w *Win32Window) hitAreaKind(x, y int) native.HitArea {
	w.hitTestMu.Lock()
	fn := w.hitTest
	w.hitTestMu.Unlock()
	if fn == nil {
		return native.HitClient
	}
	return fn(x, y)
}

// ─── Боковые кнопки ─────────────────────────────────────────────────────────

// handleXButton разбирает WM_XBUTTONDOWN/UP/DBLCLK и отдаёт нажатие приложению
// как кнопку X1 или X2. true — сообщение обработано: по документации
// обработчик возвращает TRUE, иначе DefWindowProc превратит отпускание в
// WM_APPCOMMAND (браузерное «назад/вперёд») и пошлёт его окну.
//
// Захват мыши не берём: боковые кнопки не начинают перетаскивание.
func (w *Win32Window) handleXButton(umsg uint32, wparam, lparam uintptr) bool {
	id, ok := pointer.XButtonFromWParam(wparam)
	if !ok {
		return false
	}
	x := int(int16(lparam & 0xFFFF))
	y := int(int16((lparam >> 16) & 0xFFFF))
	if w.onMouseButton != nil {
		w.onMouseButton(x, y, id, umsg != wmXbuttonup)
	}
	return true
}

// ─── Курсор ─────────────────────────────────────────────────────────────────

// win32CursorSpec — чем показывать форму курсора движка.
type win32CursorSpec struct {
	idc    int  // стандартный курсор IDC_*
	hidden bool // курсор спрятан
	art    bool // нарисованный курсор (pointer.CursorPixels): среди IDC_* аналога нет
}

// win32CursorSpecFor переводит форму widget.Cursor в способ её показа.
//
// У Win32 нет «раскрытой» и «сжатой» руки — их рисуем сами (CreateIconIndirect,
// 24×24: больше 32×32 курсор не пройдёт через RDP). Остальные формы —
// системные курсоры, значит выглядят как в любой программе и следуют теме
// пользователя.
func win32CursorSpecFor(c int) win32CursorSpec {
	switch c {
	case pointer.CursorNone:
		return win32CursorSpec{hidden: true}
	case pointer.CursorGrab, pointer.CursorGrabbing:
		return win32CursorSpec{art: true}
	case pointer.CursorIBeam:
		return win32CursorSpec{idc: idcIBeam}
	case pointer.CursorHand:
		return win32CursorSpec{idc: idcHand}
	case pointer.CursorSizeWE:
		return win32CursorSpec{idc: idcSizeWE}
	case pointer.CursorSizeNS:
		return win32CursorSpec{idc: idcSizeNS}
	case pointer.CursorSizeNWSE:
		return win32CursorSpec{idc: idcSizeNWSE}
	case pointer.CursorSizeNESW:
		return win32CursorSpec{idc: idcSizeNESW}
	case pointer.CursorSizeAll:
		return win32CursorSpec{idc: idcSizeAll}
	case pointer.CursorCrosshair:
		return win32CursorSpec{idc: idcCross}
	case pointer.CursorWait:
		return win32CursorSpec{idc: idcWait}
	}
	return win32CursorSpec{idc: idcArrow}
}

// cursorFor возвращает HCURSOR формы c; hidden — курсор надо спрятать
// (HCURSOR 0). Загруженные курсоры кэшируются на окно.
func (w *Win32Window) cursorFor(c int) (h uintptr, hidden bool) {
	spec := win32CursorSpecFor(c)
	if spec.hidden {
		return 0, true
	}
	key := spec.idc
	if spec.art {
		key = artCursorKey + c
	}
	if w.cursorCache == nil {
		w.cursorCache = map[int]uintptr{}
	}
	if h, ok := w.cursorCache[key]; ok {
		return h, false
	}
	if spec.art {
		img, hx, hy := pointer.CursorImage(c)
		h = uintptr(rgbaToHandle(img, false, uint32(hx), uint32(hy)))
		if h == 0 { // не вышло нарисовать — ближайший системный
			idc := idcHand
			if c == pointer.CursorGrabbing {
				idc = idcSizeAll
			}
			h, _, _ = procLoadCursorW.Call(0, uintptr(idc))
		}
	} else {
		h, _, _ = procLoadCursorW.Call(0, uintptr(spec.idc))
	}
	w.cursorCache[key] = h
	return h, false
}

// setCursorMessage решает, что делать с WM_SETCURSOR: hit — младшее слово
// lParam (результат hit-test). handled=false — оставить DefWindowProc;
// иначе курсор h (0 — спрятать) ставит вызывающий. Над рамкой и кнопками
// заголовка курсор выбирает система.
func (w *Win32Window) setCursorMessage(hit uintptr) (h uintptr, handled bool) {
	if hit != htClient {
		return 0, false
	}
	if w.lock.hide.Load() { // захват указателя: курсора нет
		return 0, true
	}
	if w.cursorHidden {
		return 0, true
	}
	if w.cursorHandle != 0 {
		return w.cursorHandle, true
	}
	return 0, false
}

// pointerOverWindow — лежит ли под указателем это окно (а не соседнее).
// Курсор ставится глобально, и «применить сразу» поверх чужого окна значило бы
// прятать курсор или менять его форму, пока мышь не сдвинется.
func (w *Win32Window) pointerOverWindow() bool {
	if w.hwnd == 0 {
		return false
	}
	var pt point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return false
	}
	var r uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		// POINT по значению умещается в один 64-битный регистр.
		r, _, _ = procWindowFromPoint.Call(uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32)
	} else {
		r, _, _ = procWindowFromPoint.Call(uintptr(uint32(pt.X)), uintptr(uint32(pt.Y)))
	}
	return r == uintptr(w.hwnd)
}

// destroyArtCursors освобождает нарисованные курсоры окна.
func (w *Win32Window) destroyArtCursors() {
	for key, h := range w.cursorCache {
		if key >= artCursorKey && h != 0 {
			procDestroyCursor.Call(h)
			delete(w.cursorCache, key)
		}
	}
}
