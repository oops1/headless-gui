package window

// pointerlock.go — захват указателя и относительное движение мыши
// (Window.SetPointerLock): общая часть всех бэкендов.
//
// Для чего. Игра, CAD или удалённый рабочий стол с относительной мышью (RDP
// TS_FP_RELPOINTER_EVENT) хотят не «где указатель», а «на сколько он сдвинулся»:
// без этого указатель упирается в край окна и дальше не едет. В режиме захвата
// указатель прячется и не покидает окно, а приложение получает сдвиги — единицами
// устройства, без ускорения ОС.
//
// Как приложение узнаёт о сдвигах. Выбран колбэк Window.SetOnRelativeMotion, а
// не интерфейс «RelativeMotionHandler» у корня дерева виджетов. Приложению,
// которому нужен захват, виджеты как адресат не нужны: оно и так держит окно
// (оттуда же зовёт SetPointerLock) и пересылает сдвиг по сети или в модель
// игры. Корневой интерфейс заставил бы заводить ради этого собственный корень
// и ждать, пока движок доставит событие по дереву; колбэк один и
// переставляется на лету. Сдвиги приходят на горутине движка (как обработчики
// виджетов), поэтому в колбэке можно трогать виджеты.
//
// Бэкенды: Win32 — Raw Input (WM_INPUT) и ClipCursor; Wayland —
// zwp_pointer_constraints_v1.lock_pointer с zwp_relative_pointer_v1; X11 —
// XInput2 XI_RawMotion на корневом окне и XGrabPointer. Бэкенд без поддержки
// (macOS) просто не реализует pointerLocker, и SetPointerLock возвращает false.
//
// Файл без платформенного суффикса: слияние сдвигов — чистая логика.

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
)

// Значения Raw Input (winuser.h), нужные разбору RAWINPUT.
const (
	rimTypeMouse = 0    // RIM_TYPEMOUSE
	mouseMoveAbs = 0x01 // MOUSE_MOVE_ABSOLUTE
)

// pointerLocker — нативное окно умеет захват указателя.
type pointerLocker interface {
	// SetPointerLock включает и выключает захват. true при включении —
	// запрос принят: захват действует или начнёт действовать, как только окно
	// получит фокус (Wayland — указатель); false — механизма нет или система
	// отказала. Выключение всегда успешно.
	SetPointerLock(on bool) bool
	// SetOnRelativeMotion — приёмник сдвигов (единицы устройства, дробные).
	// Зовётся с потока бэкенда, часто (сотни раз в секунду).
	SetOnRelativeMotion(fn func(dx, dy float64))
	// SetOnPointerLockChanged — захват начал или перестал действовать:
	// окно потеряло фокус (Win32, X11), компоновщик снял блокировку
	// (Wayland); вернулся фокус — захват восстановлен.
	SetOnPointerLockChanged(fn func(active bool))
}

// relMotionQueue склеивает сдвиги мыши, стоящие в очереди движка: мышь шлёт
// их по 1000 в секунду, а движку и приложению хватает суммы. Пока сброс в
// очереди, новые сдвиги только складываются. Нулевое значение готово к работе.
type relMotionQueue struct {
	mu      sync.Mutex
	dx, dy  float64
	pending bool
}

// add складывает сдвиг и, если сброса в очереди нет, ставит его через run.
// send получает накопленную сумму на горутине, исполняющей run.
func (q *relMotionQueue) add(run func(func()), dx, dy float64, send func(dx, dy float64)) {
	q.mu.Lock()
	q.dx += dx
	q.dy += dy
	if q.pending {
		q.mu.Unlock()
		return
	}
	q.pending = true
	q.mu.Unlock()
	run(func() {
		q.mu.Lock()
		dx, dy := q.dx, q.dy
		q.dx, q.dy, q.pending = 0, 0, false
		q.mu.Unlock()
		if dx != 0 || dy != 0 {
			send(dx, dy)
		}
	})
}

// pointerLockState — состояние захвата указателя поверхности.
type pointerLockState struct {
	rel relMotionQueue
	// fn — приёмник сдвигов приложения (Window.SetOnRelativeMotion).
	fn atomic.Pointer[func(widget.RelativeMotion)]
	// changed — приложение следит за началом и концом захвата.
	changed atomic.Pointer[func(active bool)]
	// active — захват сейчас действует (бэкенд сообщил): абсолютные движения
	// мыши тогда не доставляются — указатель стоит на месте, а сдвиги идут
	// отдельным потоком.
	active atomic.Bool
	// wanted — приложение просило захват и не снимало.
	wanted atomic.Bool
}

// hookPointerLock подключает приём сдвигов и смену состояния захвата от
// бэкенда к поверхности. Зовётся из setupInput один раз.
func (s *surface) hookPointerLock() {
	pl, ok := s.native.(pointerLocker)
	if !ok {
		return
	}
	pl.SetOnRelativeMotion(s.deliverRelative)
	pl.SetOnPointerLockChanged(func(active bool) {
		s.lock.active.Store(active)
		if fn := s.lock.changed.Load(); fn != nil {
			s.onEngine(func() { (*fn)(active) })
		}
	})
}

// deliverRelative ставит сдвиг в очередь движка (с потока бэкенда).
func (s *surface) deliverRelative(dx, dy float64) {
	s.lock.rel.add(s.onEngine, dx, dy, func(dx, dy float64) {
		if fn := s.lock.fn.Load(); fn != nil {
			(*fn)(widget.RelativeMotion{DX: dx, DY: dy})
		}
	})
}

// SetPointerLock включает и выключает захват указателя: указатель прячется и
// не выходит за окно, а приложение получает относительные сдвиги мыши через
// SetOnRelativeMotion — в единицах устройства, без ускорения ОС. Для игр, CAD
// и удалённых рабочих столов с относительной мышью (RDP).
//
// Возвращает true, если запрос принят: захват действует или начнёт действовать,
// как только окно получит фокус (на Wayland — как только компоновщик даст
// блокировку; указатель должен быть над окном). false — окна ещё нет (до Run),
// ОС не умеет (macOS, компоновщик без zwp_pointer_constraints_v1, X11 без
// XInput2) или отказала. Выключение возвращает true всегда.
//
// Захват снимается при потере окном фокуса и восстанавливается, когда фокус
// вернулся (SetOnPointerLockChanged сообщает об обоих переходах). Пока он
// действует, обычные движения мыши (OnMouseMove виджетов) не приходят, нажатия
// кнопок и колесо — как всегда, в центре окна. Можно звать с любой горутины.
func (win *Window) SetPointerLock(on bool) bool {
	pl, ok := win.native.(pointerLocker)
	if !ok {
		return !on
	}
	if !on {
		win.lock.wanted.Store(false)
		pl.SetPointerLock(false)
		return true
	}
	if !pl.SetPointerLock(true) {
		return false
	}
	win.lock.wanted.Store(true)
	return true
}

// PointerLocked сообщает, что захват указателя действует прямо сейчас.
func (win *Window) PointerLocked() bool { return win.lock.active.Load() }

// SetOnRelativeMotion задаёт приёмник относительных сдвигов мыши в режиме
// захвата (SetPointerLock). Сдвиги, пришедшие подряд, склеиваются в сумму;
// приёмник зовётся на горутине движка. nil — снять. Можно звать до Run.
func (win *Window) SetOnRelativeMotion(fn func(widget.RelativeMotion)) {
	if fn == nil {
		win.lock.fn.Store(nil)
		return
	}
	win.lock.fn.Store(&fn)
}

// SetOnPointerLockChanged задаёт уведомление о том, что захват начал или
// перестал действовать (потеря и возврат фокуса, отказ компоновщика). Зовётся
// на горутине движка. nil — снять.
func (win *Window) SetOnPointerLockChanged(fn func(active bool)) {
	if fn == nil {
		win.lock.changed.Store(nil)
		return
	}
	win.lock.changed.Store(&fn)
}

// parseRawMouse разбирает RAWINPUT мыши: lLastX, lLastY и признак «абсолютное
// положение» (MOUSE_MOVE_ABSOLUTE). ptrSize — размер указателя (8 или 4):
// заголовок RAWINPUTHEADER содержит два слова размером с указатель. ok=false —
// это не мышь или буфер короче структуры.
//
// Раскладка RAWMOUSE после заголовка: usFlags (2) + 2 байта выравнивания,
// ulButtons (4), ulRawButtons (4), lLastX (4), lLastY (4), ulExtraInformation (4).
func parseRawMouse(buf []byte, ptrSize int) (x, y int32, absolute, ok bool) {
	hdr := 8 + 2*ptrSize
	if len(buf) < hdr+24 {
		return 0, 0, false, false
	}
	if binary.LittleEndian.Uint32(buf[0:4]) != rimTypeMouse {
		return 0, 0, false, false
	}
	flags := binary.LittleEndian.Uint16(buf[hdr : hdr+2])
	x = int32(binary.LittleEndian.Uint32(buf[hdr+12 : hdr+16]))
	y = int32(binary.LittleEndian.Uint32(buf[hdr+16 : hdr+20]))
	return x, y, flags&mouseMoveAbs != 0, true
}
