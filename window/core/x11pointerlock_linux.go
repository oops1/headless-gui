//go:build linux && !android

package core

// x11pointerlock_linux.go — X11: захват указателя окна (Window.SetPointerLock).
// Протокол — в x11pointerlock.go.
//
// Просьба приходит с произвольной горутины, а согласование XInput2 и
// XGrabPointer — запросы с ответом, которые можно читать только в цикле событий
// (соединение читает он один). Поэтому SetPointerLock кладёт просьбу, будит цикл
// (x11WakeRequest) и ждёт результата; цикл исполняет её между событиями
// (x11SyncPointerLock), как отложенные касания и курсор-картинку.
//
// Фокус. FocusOut снимает захват с сервера (иначе окно, потерявшее фокус, держало
// бы указатель), FocusIn возвращает. Сами обработчики фокуса лишь отмечают
// перемену: они могут вызываться из чтения чужого ответа, где вложенный запрос
// недопустим; действие — в x11SyncPointerLock.

import (
	"sync"
	"sync/atomic"
	"time"
)

// x11LockReq — просьба SetPointerLock, ждущая цикла событий.
type x11LockReq struct {
	on   bool
	done chan bool
}

// x11PtrLock — состояние захвата указателя окна X11.
type x11PtrLock struct {
	mu  sync.Mutex
	req *x11LockReq // просьба, ещё не исполненная

	// Состояние ниже трогает только цикл событий.
	want     bool // приложение просило захват и не снимало
	active   bool // указатель сейчас захвачен
	selected bool // подписка XI_RawMotion на корневое окно действует
	warp     bool // XInput2 нет: сдвиг считается возвратом указателя в центр
	xiMajor  byte // код расширения XInputExtension

	looping    atomic.Bool // цикл событий читает соединение и его можно будить
	blurred    atomic.Bool // окно потеряло фокус (FocusOut); пока FocusIn не было — считаем в фокусе
	focusDirty atomic.Bool // фокус менялся с прошлого x11SyncPointerLock
	// activeFlag зеркалит active для чтения с других горутин (тесты, PointerLocked).
	activeFlag atomic.Bool

	onMotion  atomic.Pointer[func(dx, dy float64)]
	onChanged atomic.Pointer[func(active bool)]
}

// SetOnRelativeMotion реализует pointerLocker.
func (w *X11Window) SetOnRelativeMotion(fn func(dx, dy float64)) {
	if fn == nil {
		w.plock.onMotion.Store(nil)
		return
	}
	w.plock.onMotion.Store(&fn)
}

// SetOnPointerLockChanged реализует pointerLocker.
func (w *X11Window) SetOnPointerLockChanged(fn func(active bool)) {
	if fn == nil {
		w.plock.onChanged.Store(nil)
		return
	}
	w.plock.onChanged.Store(&fn)
}

// SetPointerLock реализует pointerLocker. Ждёт, пока цикл событий исполнит
// просьбу; без запущенного цикла (до RunEventLoop) включить захват нельзя.
func (w *X11Window) SetPointerLock(on bool) bool {
	l := &w.plock
	if w.wid == 0 || !l.looping.Load() {
		return !on
	}
	req := &x11LockReq{on: on, done: make(chan bool, 1)}
	l.mu.Lock()
	l.req = req
	l.mu.Unlock()
	w.x11Send(x11WakeRequest(w.wid))
	select {
	case ok := <-req.done:
		return ok
	case <-time.After(2 * time.Second):
		return !on
	}
}

// x11SyncPointerLock исполняет просьбу о захвате и перемену фокуса. Зовётся
// циклом событий между событиями.
func (w *X11Window) x11SyncPointerLock() {
	l := &w.plock
	l.mu.Lock()
	req := l.req
	l.req = nil
	l.mu.Unlock()
	if req != nil {
		req.done <- w.x11ApplyLock(req.on)
	}
	if l.focusDirty.Swap(false) && l.want {
		if l.blurred.Load() {
			w.x11ReleaseGrab()
		} else {
			w.x11EngageLock()
		}
	}
}

// x11ApplyLock включает и выключает захват. Цикл событий.
func (w *X11Window) x11ApplyLock(on bool) bool {
	l := &w.plock
	if !on {
		l.want = false
		w.x11ReleaseGrab()
		w.x11DeselectRaw()
		return true
	}
	if !l.selected && !l.warp {
		if !w.x11SelectRaw() {
			l.warp = true // XInput2 нет — запасной путь
		}
	}
	l.want = true
	if l.blurred.Load() {
		return true // окно без фокуса: захват включится с фокусом
	}
	if !w.x11EngageLock() {
		l.want = false
		w.x11DeselectRaw()
		return false
	}
	return true
}

// x11SelectRaw согласует XInput2 и подписывает корневое окно на XI_RawMotion.
// false — расширения нет, версия мала или подписка отвергнута.
func (w *X11Window) x11SelectRaw() bool {
	l := &w.plock
	reply, ok := w.x11Reply(x11QueryExtensionRequest(xiExtensionName))
	if !ok {
		return false
	}
	major, _, present := parseQueryExtensionReply(reply)
	if !present {
		return false
	}
	// Версию просим ту же, что у касаний: повторный запрос с меньшей версией
	// сервер вправе отвергнуть.
	reply, ok = w.x11Reply(xiQueryVersionRequest(major, xiTouchMajor, xiTouchMinor))
	if !ok {
		return false
	}
	vmaj, _, ok := parseXIQueryVersionReply(reply)
	if !ok || vmaj < 2 {
		return false
	}
	w.x11Send(xiSelectEventsRequest(major, w.rootWin, xiAllMasterDevices, xiRawMotionMask()))
	if !w.x11SyncCheck() {
		return false
	}
	l.xiMajor, l.selected = major, true
	return true
}

// x11DeselectRaw снимает подписку на сырое движение.
func (w *X11Window) x11DeselectRaw() {
	l := &w.plock
	if l.selected {
		w.x11Send(xiSelectEventsRequest(l.xiMajor, w.rootWin, xiAllMasterDevices, [4]byte{}))
		l.selected = false
	}
	l.warp = false
}

// x11EngageLock захватывает указатель: XGrabPointer с привязкой к окну и
// невидимым курсором, затем возврат в центр. false — сервер отказал.
func (w *X11Window) x11EngageLock() bool {
	l := &w.plock
	if l.active {
		return true
	}
	w.cursors.mu.Lock()
	blank := w.cursorResourceLocked(wlCursorNone)
	w.cursors.mu.Unlock()
	reply, ok := w.x11Reply(x11GrabPointerRequest(w.wid, w.wid, blank))
	if !ok {
		return false
	}
	if st, ok := parseGrabPointerReply(reply); !ok || st != x11GrabSuccess {
		return false
	}
	l.active = true
	l.activeFlag.Store(true)
	w.x11Send(x11WarpPointerRequest(w.wid, w.width/2, w.height/2))
	if fn := l.onChanged.Load(); fn != nil {
		(*fn)(true)
	}
	return true
}

// x11ReleaseGrab отпускает указатель; просьба приложения (want) остаётся.
func (w *X11Window) x11ReleaseGrab() {
	l := &w.plock
	if !l.active {
		return
	}
	l.active = false
	l.activeFlag.Store(false)
	w.x11Send(x11UngrabPointerRequest())
	if fn := l.onChanged.Load(); fn != nil {
		(*fn)(false)
	}
}

// x11NotePointerFocus отмечает перемену фокуса (FocusIn/FocusOut).
func (w *X11Window) x11NotePointerFocus(in bool) {
	l := &w.plock
	l.blurred.Store(!in)
	l.focusDirty.Store(true)
}

// x11LockMotion разбирает MotionNotify при захвате и сообщает, поглощено ли оно:
// при действующем захвате абсолютные движения приложению не нужны. Цикл событий.
//
// С XInput2 указатель лишь возвращают в центр, когда он ушёл далеко. Без него
// сдвиг — отличие положения от центра, указатель возвращается после каждого
// движения.
func (w *X11Window) x11LockMotion(x, y int) bool {
	l := &w.plock
	if !l.active {
		return false
	}
	cx, cy := w.width/2, w.height/2
	if l.warp {
		if dx, dy := x11WarpDelta(x, y, cx, cy); dx != 0 || dy != 0 {
			if fn := l.onMotion.Load(); fn != nil {
				(*fn)(dx, dy)
			}
			w.x11Send(x11WarpPointerRequest(w.wid, cx, cy))
		}
		return true
	}
	if x11NeedsRecenter(x, y, w.width, w.height) {
		w.x11Send(x11WarpPointerRequest(w.wid, cx, cy))
	}
	return true
}

// x11RawMotion — GenericEvent: сырое движение мыши при захвате. true — событие
// разобрано (в том числе отброшено как не относящееся к захвату).
func (w *X11Window) x11RawMotion(buf []byte) bool {
	l := &w.plock
	if !l.selected {
		return false
	}
	dx, dy, ok := parseXIRawMotion(buf, l.xiMajor)
	if !ok {
		return false
	}
	if l.active && (dx != 0 || dy != 0) {
		if fn := l.onMotion.Load(); fn != nil {
			(*fn)(dx, dy)
		}
	}
	return true
}

var _ pointerLocker = (*X11Window)(nil)
