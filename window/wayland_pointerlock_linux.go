//go:build linux && !android

package window

// wayland_pointerlock_linux.go — Wayland: захват указателя
// (Window.SetPointerLock).
//
// Нужны два необязательных расширения, оба должен объявлять компоновщик:
//
//   - zwp_pointer_constraints_v1.lock_pointer — запирает указатель на месте, пока
//     поверхность в фокусе. Компоновщик сообщает об этом событием locked, а когда
//     захват слетает (окно потеряло фокус) — unlocked; захват с lifetime
//     persistent восстанавливается сам, когда фокус вернулся (и снова приходит
//     locked). Это ровно «снятие при потере фокуса и восстановление».
//   - zwp_relative_pointer_manager_v1.get_relative_pointer — даёт события
//     relative_motion со сдвигом указателя. У запертого указателя обычного
//     motion нет, а relative_motion идёт: в нём и ускоренный сдвиг (dx, dy), и
//     исходный от устройства (dx_unaccel, dy_unaccel). Берётся исходный — как
//     RAWINPUT на Win32.
//
// Нет любого из двух глобалов — SetPointerLock возвращает false. Указатель должен
// быть над окном и окно в фокусе, иначе компоновщик запрет указатель позже
// (SetPointerLock в этом случае всё равно true: запрос принят).
//
// Пока захват действует, курсор спрятан (applyCursor), а при снятии вернётся.

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
)

// Протокол zwp_pointer_constraints_v1 и zwp_relative_pointer_v1.
const (
	// zwp_pointer_constraints_v1
	zwpConstraintsDestroy     = 0
	zwpConstraintsLockPointer = 1
	// lifetime
	zwpLifetimePersistent = 2

	// zwp_locked_pointer_v1
	zwpLockedDestroy     = 0
	zwpLockedEvLocked    = 0
	zwpLockedEvUnlocked  = 1
	zwpRelMgrDestroy     = 0
	zwpRelMgrGetRelative = 1

	// zwp_relative_pointer_v1
	zwpRelativeDestroy        = 0
	zwpRelativeEvRelativeMove = 0
)

// wlPtrLock — состояние захвата указателя окна Wayland.
type wlPtrLock struct {
	mu sync.Mutex // сериализует SetPointerLock

	gConstraints uint32 // имя глобала zwp_pointer_constraints_v1 (0 — нет)
	gRelative    uint32 // имя глобала zwp_relative_pointer_manager_v1 (0 — нет)

	constraintsID uint32 // объекты после bind; под mu
	relMgrID      uint32

	lockedID atomic.Uint32 // zwp_locked_pointer_v1; 0 — захвата не просили
	relID    atomic.Uint32 // zwp_relative_pointer_v1
	active   atomic.Bool   // компоновщик подтвердил захват (locked)

	onMotion  atomic.Pointer[func(dx, dy float64)]
	onChanged atomic.Pointer[func(active bool)]
}

// SetOnRelativeMotion реализует pointerLocker.
func (w *WaylandWindow) SetOnRelativeMotion(fn func(dx, dy float64)) {
	if fn == nil {
		w.plock.onMotion.Store(nil)
		return
	}
	w.plock.onMotion.Store(&fn)
}

// SetOnPointerLockChanged реализует pointerLocker.
func (w *WaylandWindow) SetOnPointerLockChanged(fn func(active bool)) {
	if fn == nil {
		w.plock.onChanged.Store(nil)
		return
	}
	w.plock.onChanged.Store(&fn)
}

// SetPointerLock реализует pointerLocker. true — запрос отправлен компоновщику;
// захват начнётся, когда он ответит locked (событие приходит в цикл событий).
func (w *WaylandWindow) SetPointerLock(on bool) bool {
	l := &w.plock
	l.mu.Lock()
	defer l.mu.Unlock()
	if !on {
		w.unlockPointer()
		return true
	}
	if l.gConstraints == 0 || l.gRelative == 0 || w.pointerID == 0 || w.surfaceID == 0 {
		return false
	}
	if l.lockedID.Load() != 0 {
		return true // уже просили: повторный lock_pointer — ошибка протокола
	}
	if l.constraintsID == 0 {
		l.constraintsID = w.bind(l.gConstraints, "zwp_pointer_constraints_v1", 1)
	}
	if l.relMgrID == 0 {
		l.relMgrID = w.bind(l.gRelative, "zwp_relative_pointer_manager_v1", 1)
	}
	rel := w.newID()
	locked := w.newID()
	l.relID.Store(rel)
	l.lockedID.Store(locked)
	w.send(wlGetRelativePointerMsg(l.relMgrID, rel, w.pointerID), -1)
	w.send(wlLockPointerMsg(l.constraintsID, locked, w.surfaceID, w.pointerID), -1)
	return true
}

// unlockPointer уничтожает захват и относительный указатель. Под l.mu.
func (w *WaylandWindow) unlockPointer() {
	l := &w.plock
	locked := l.lockedID.Swap(0)
	rel := l.relID.Swap(0)
	if locked != 0 {
		w.send(newWlMsg(locked, zwpLockedDestroy), -1)
	}
	if rel != 0 {
		w.send(newWlMsg(rel, zwpRelativeDestroy), -1)
	}
	if l.active.Swap(false) {
		w.notePtrLockActive(false)
	}
}

// wlGetRelativePointerMsg — zwp_relative_pointer_manager_v1.get_relative_pointer:
// new_id id, wl_pointer pointer.
func wlGetRelativePointerMsg(mgr, id, pointer uint32) *wlMsg {
	return newWlMsg(mgr, zwpRelMgrGetRelative).putUint(id).putUint(pointer)
}

// wlLockPointerMsg — zwp_pointer_constraints_v1.lock_pointer: new_id id,
// wl_surface surface, wl_pointer pointer, wl_region region (NULL — вся
// поверхность), uint lifetime (persistent: захват восстанавливается с фокусом).
func wlLockPointerMsg(constraints, id, surface, pointer uint32) *wlMsg {
	return newWlMsg(constraints, zwpConstraintsLockPointer).
		putUint(id).putUint(surface).putUint(pointer).putUint(0).putUint(zwpLifetimePersistent)
}

// parseRelativeMotion разбирает zwp_relative_pointer_v1.relative_motion:
// utime_hi, utime_lo, dx, dy, dx_unaccel, dy_unaccel (fixed 24.8). Возвращает
// исходный (неускоренный) сдвиг. ok=false — тело короче события.
func parseRelativeMotion(b []byte) (dx, dy float64, ok bool) {
	if len(b) < 24 {
		return 0, 0, false
	}
	fx := func(o int) float64 { return float64(int32(binary.LittleEndian.Uint32(b[o:o+4]))) / 256 }
	return fx(16), fx(20), true
}

// isPtrLockObject — объект принадлежит захвату указателя.
func (w *WaylandWindow) isPtrLockObject(obj uint32) bool {
	return obj != 0 && (obj == w.plock.lockedID.Load() || obj == w.plock.relID.Load())
}

// handlePtrLock — события zwp_locked_pointer_v1 и zwp_relative_pointer_v1
// (цикл событий).
func (w *WaylandWindow) handlePtrLock(obj uint32, opcode uint16, b []byte) {
	l := &w.plock
	switch {
	case obj == l.lockedID.Load():
		switch opcode {
		case zwpLockedEvLocked:
			if !l.active.Swap(true) {
				w.notePtrLockActive(true)
			}
		case zwpLockedEvUnlocked:
			if l.active.Swap(false) {
				w.notePtrLockActive(false)
			}
		}
	case obj == l.relID.Load() && opcode == zwpRelativeEvRelativeMove:
		if !l.active.Load() {
			return
		}
		dx, dy, ok := parseRelativeMotion(b)
		if !ok || (dx == 0 && dy == 0) {
			return
		}
		if fn := l.onMotion.Load(); fn != nil {
			(*fn)(dx, dy)
		}
	}
}

// notePtrLockActive сообщает о начале и конце захвата и перерисовывает курсор
// (при захвате он спрятан).
func (w *WaylandWindow) notePtrLockActive(active bool) {
	w.applyCursor()
	if fn := w.plock.onChanged.Load(); fn != nil {
		(*fn)(active)
	}
}

var _ pointerLocker = (*WaylandWindow)(nil)
