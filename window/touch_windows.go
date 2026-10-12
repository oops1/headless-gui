//go:build windows

package window

// touch_windows.go — касания на Win32: WM_POINTER* для устройств PT_TOUCH.
//
// Как не дублировать мышь. Для касания Windows порождает два потока сообщений:
// WM_POINTER* и, если их передали в DefWindowProc, «совместимую» мышь
// (WM_MOUSEMOVE, WM_LBUTTONDOWN… со специальной подписью в GetMessageExtraInfo).
// Окно, взявшее касания, делает ровно одно из двух на каждое касание:
//
//   - PT_TOUCH при включённых касаниях: обрабатывает сообщение само и НЕ зовёт
//     DefWindowProc — тогда система не порождает ни мыши, ни жестов WM_GESTURE,
//     и касание достаётся окну в единственном экземпляре;
//   - всё остальное — перо, а при включённом EnableMouseInPointer и мышь, — идёт в
//     DefWindowProc как раньше. EnableMouseInPointer НЕ включается, поэтому обычная
//     мышь по-прежнему приходит как WM_MOUSE*, а перо — мышью, и менять эти пути
//     незачем.
//
// Окно без включённых касаний (SetTouchEnabled не звали) пропускает ВСЕ
// WM_POINTER* в DefWindowProc, и приложение получает мышь из касаний так же,
// как до появления этого файла. Фильтровать мышь по подписи 0xFF515700 не
// нужно, а вредно: той же подписью помечено перо, и его мышь надо оставить.
//
// Касание, которого не взял ни один виджет, окно превращает в левую кнопку мыши
// само (touch.go): иначе под пальцем не работали бы кнопки и меню.

import (
	"image"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v3/widget"
)

const (
	wmPointerUpdate         = 0x0245
	wmPointerDown           = 0x0246
	wmPointerUp             = 0x0247
	wmPointerCaptureChanged = 0x024C

	// POINTER_INPUT_TYPE
	ptTouch = 2

	// POINTER_FLAGS (HIWORD(wParam) несёт младшие 16 бит)
	pointerFlagCanceled       = 0x00008000
	pointerFlagCaptureChanged = 0x00200000
)

var (
	procGetPointerType      = user32.NewProc("GetPointerType")
	procGetPointerTouchInfo = user32.NewProc("GetPointerTouchInfo")
)

// pointerInfo — POINTER_INFO (winuser.h). Поля с HANDLE — uintptr, чтобы
// раскладка совпала с C и на 32-, и на 64-разрядной сборке.
type pointerInfo struct {
	PointerType           uint32
	PointerID             uint32
	FrameID               uint32
	PointerFlags          uint32
	SourceDevice          uintptr
	HwndTarget            uintptr
	PtPixelLocation       point
	PtHimetricLocation    point
	PtPixelLocationRaw    point
	PtHimetricLocationRaw point
	DwTime                uint32
	HistoryCount          uint32
	InputData             int32
	DwKeyStates           uint32
	PerformanceCount      uint64
	ButtonChangeType      int32
}

// pointerTouchInfo — POINTER_TOUCH_INFO.
type pointerTouchInfo struct {
	PointerInfo  pointerInfo
	TouchFlags   uint32
	TouchMask    uint32
	RcContact    rect
	RcContactRaw rect
	Orientation  uint32
	Pressure     uint32
}

// win32TouchState — состояние касаний окна. Флаги атомарные (ставит
// приложение, читает поток окна), active трогает только поток окна.
type win32TouchState struct {
	on      atomic.Bool
	onTouch atomic.Pointer[func(ev widget.TouchEvent)]
	// active — касания, которые окно ведёт, и где каждое было в последний раз
	// (клиентские пиксели): отмена по WM_POINTERCAPTURECHANGED координат не несёт.
	active   map[uint32]image.Point
	apiKnown atomic.Int32 // 0 — не проверяли, 1 — доступно, -1 — нет
}

// SetTouchEnabled включает и выключает перехват касаний. Реализует touchSource.
func (w *Win32Window) SetTouchEnabled(on bool) {
	if on && !pointerAPIAvailable(&w.touch) {
		return
	}
	w.touch.on.Store(on)
}

// SetOnTouch подписывает приёмник касаний. Реализует touchSource.
func (w *Win32Window) SetOnTouch(fn func(ev widget.TouchEvent)) {
	if fn == nil {
		w.touch.onTouch.Store(nil)
		return
	}
	w.touch.onTouch.Store(&fn)
}

// pointerAPIAvailable — есть ли в системе API указателей (Windows 8+).
func pointerAPIAvailable(st *win32TouchState) bool {
	switch st.apiKnown.Load() {
	case 1:
		return true
	case -1:
		return false
	}
	ok := procGetPointerType.Find() == nil && procGetPointerTouchInfo.Find() == nil
	if ok {
		st.apiKnown.Store(1)
	} else {
		st.apiKnown.Store(-1)
	}
	return ok
}

// pointerIDFromWParam — GET_POINTERID_WPARAM.
func pointerIDFromWParam(wparam uintptr) uint32 { return uint32(wparam & 0xFFFF) }

// touchPhaseOfPointer переводит сообщение указателя и его флаги в фазу касания.
// ok=false — сообщение к касанию как таковому не относится.
func touchPhaseOfPointer(umsg uint32, flags uint32) (widget.TouchPhase, bool) {
	switch umsg {
	case wmPointerDown:
		return widget.TouchBegan, true
	case wmPointerUpdate:
		return widget.TouchMoved, true
	case wmPointerUp:
		if flags&pointerFlagCanceled != 0 {
			return widget.TouchCancelled, true
		}
		return widget.TouchEnded, true
	case wmPointerCaptureChanged:
		return widget.TouchCancelled, true
	}
	return 0, false
}

// touchEventFromPointer собирает событие касания из сведений указателя.
// cx, cy — клиентские физические пиксели.
func touchEventFromPointer(id uint32, phase widget.TouchPhase, dwTime uint32, cx, cy int) widget.TouchEvent {
	return widget.TouchEvent{
		ID:    int(id),
		X:     cx,
		Y:     cy,
		Phase: phase,
		Time:  time.Duration(dwTime) * time.Millisecond,
	}
}

// handlePointerMsg разбирает WM_POINTER*. true — сообщение обработано и в
// DefWindowProc отдавать нельзя (иначе система доставит ещё и мышь).
func (w *Win32Window) handlePointerMsg(hwnd uintptr, umsg uint32, wparam uintptr) bool {
	if !w.touch.on.Load() {
		return false
	}
	fn := w.touch.onTouch.Load()
	if fn == nil {
		return false
	}
	id := pointerIDFromWParam(wparam)

	if umsg == wmPointerCaptureChanged {
		// Касание, которое окно ещё ведёт, у нас забрали (системный жест,
		// другое окно): отрыва не будет. Для касаний, уже закончившихся, захват
		// снимается штатно, и сообщать нечего.
		last, mine := w.touch.active[id]
		if !mine {
			return false
		}
		delete(w.touch.active, id)
		(*fn)(touchEventFromPointer(id, widget.TouchCancelled, 0, last.X, last.Y))
		return true
	}

	var typ uint32
	if r, _, _ := procGetPointerType.Call(uintptr(id), uintptr(unsafe.Pointer(&typ))); r == 0 || typ != ptTouch {
		return false // перо и прочее — системе, мышь из них прежняя
	}
	var info pointerTouchInfo
	if r, _, _ := procGetPointerTouchInfo.Call(uintptr(id), uintptr(unsafe.Pointer(&info))); r == 0 {
		return false
	}
	phase, ok := touchPhaseOfPointer(umsg, info.PointerInfo.PointerFlags)
	if !ok {
		return false
	}
	if w.touch.active == nil {
		w.touch.active = make(map[uint32]image.Point)
	}
	if _, mine := w.touch.active[id]; !mine && phase != widget.TouchBegan {
		return true // касание началось до включения: чужое, мышь из него тоже не нужна
	}

	// Экранные физические пиксели -> клиентские.
	pt := info.PointerInfo.PtPixelLocation
	procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	at := image.Pt(int(pt.X), int(pt.Y))
	if phase.Terminal() {
		delete(w.touch.active, id)
	} else {
		w.touch.active[id] = at
	}
	(*fn)(touchEventFromPointer(id, phase, info.PointerInfo.DwTime, at.X, at.Y))
	return true
}
