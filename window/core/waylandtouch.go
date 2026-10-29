package core

// waylandtouch.go — касания Wayland (wl_touch): разбор событий и их перевод в
// widget.TouchEvent.
//
// Файл БЕЗ платформенного суффикса — по той же причине, что и waylandwire.go:
// проверять раскладку байтов и логику событий можно и нужно на любой машине, а не
// только в Wayland-сессии. Сокет и привязка объектов — в wayland_touch_linux.go.
//
// Как устроены события wl_touch. Касание начинает down (с поверхностью, на
// которую пришёлся палец), дальше идут motion, закончит up или cancel. Координаты
// есть у down и motion, а у up их нет, поэтому последнюю точку помним сами. События
// одного момента группирует frame; мы доставляем каждое событие сразу, в порядке
// прихода, — для приёмника результат тот же, а задержки на ожидание frame нет.
//
// Площадь контакта (wl_touch версии 6). События shape (большая и малая оси
// эллипса) и orientation (угол большой оси) приходят ПОСЛЕ down или motion своего
// момента и перед frame: они относятся к пальцам этого кадра, а не к
// предыдущему событию. Поэтому, если компоновщик умеет версию 6, события не
// доставляются сразу, а копятся до frame и уходят с добавленными осями; у версий
// ниже frame по-прежнему не ждём. Давления в wl_touch нет вовсе — Pressure
// остаётся нулём.
//
// Координаты — fixed 24.8 в ПОВЕРХНОСТНЫХ единицах; движок живёт в пикселях
// буфера, поэтому умножаем на масштаб поверхности (как размеры окна, только в
// обратную сторону: configure делает fromSurface).
//
// Попапы (xdg_popup — меню и выпадающие списки) — отдельные поверхности, и палец,
// упавший на попап, принадлежит ему весь свой путь. Попапы управляются мышью, а не
// касаниями, поэтому первый палец на попапе превращается в движение и левую кнопку
// мыши ТОГО ЖЕ попапа (через те же приёмники, что и у указателя), а остальные
// пальцы на попапе игнорируются. Отмена отпускает кнопку вне попапа, чтобы пункт
// меню не сработал от прерванного касания.

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Значения протокола wl_seat / wl_touch.
const (
	wlSeatGetTouch = 2 // wl_seat.get_touch(new_id)
	seatCapTouch   = 4 // wl_seat.capability.touch

	wlTouchEvDown   = 0
	wlTouchEvUp     = 1
	wlTouchEvMotion = 2
	wlTouchEvFrame  = 3
	wlTouchEvCancel = 4
	// События версии 6.
	wlTouchEvShape       = 5 // id, major(fixed), minor(fixed)
	wlTouchEvOrientation = 6 // id, orientation(fixed, градусы)

	wlTouchRelease = 0 // wl_touch.release (версия 3)

	// wlSeatTouchShapeVersion — версия wl_seat (а с ней и wl_touch), с которой
	// у касаний есть shape и orientation.
	wlSeatTouchShapeVersion = 6
	wlSeatRelease           = 3 // wl_seat.release (версия 5)
)

// wlFixedToFloat переводит wl_fixed (24.8) в число.
func wlFixedToFloat(v int32) float64 { return float64(v) / 256 }

// wlTouchDown — разобранное wl_touch.down.
type wlTouchDown struct {
	Serial, Time, Surface uint32
	ID                    int32
	X, Y                  float64 // поверхностные единицы
}

// parseWlTouchDown разбирает down: serial, time, surface, id, x, y.
func parseWlTouchDown(b []byte) (wlTouchDown, bool) {
	if len(b) < 24 {
		return wlTouchDown{}, false
	}
	return wlTouchDown{
		Serial:  binary.LittleEndian.Uint32(b[0:4]),
		Time:    binary.LittleEndian.Uint32(b[4:8]),
		Surface: binary.LittleEndian.Uint32(b[8:12]),
		ID:      int32(binary.LittleEndian.Uint32(b[12:16])),
		X:       wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[16:20]))),
		Y:       wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[20:24]))),
	}, true
}

// parseWlTouchUp разбирает up: serial, time, id.
func parseWlTouchUp(b []byte) (serial, t uint32, id int32, ok bool) {
	if len(b) < 12 {
		return 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint32(b[4:8]),
		int32(binary.LittleEndian.Uint32(b[8:12])), true
}

// parseWlTouchMotion разбирает motion: time, id, x, y.
func parseWlTouchMotion(b []byte) (t uint32, id int32, x, y float64, ok bool) {
	if len(b) < 16 {
		return 0, 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(b[0:4]), int32(binary.LittleEndian.Uint32(b[4:8])),
		wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[8:12]))),
		wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[12:16]))), true
}

// parseWlTouchShape разбирает shape: id, major, minor (поверхностные единицы).
func parseWlTouchShape(b []byte) (id int32, major, minor float64, ok bool) {
	if len(b) < 12 {
		return 0, 0, 0, false
	}
	return int32(binary.LittleEndian.Uint32(b[0:4])),
		wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[4:8]))),
		wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[8:12]))), true
}

// parseWlTouchOrientation разбирает orientation: id, угол в градусах.
func parseWlTouchOrientation(b []byte) (id int32, deg float64, ok bool) {
	if len(b) < 8 {
		return 0, 0, false
	}
	return int32(binary.LittleEndian.Uint32(b[0:4])),
		wlFixedToFloat(int32(binary.LittleEndian.Uint32(b[4:8]))), true
}

// wlShape — последняя известная форма контакта пальца (поверхностные единицы и
// градусы); хранится до отрыва: shape и orientation приходят только когда
// меняются.
type wlShape struct {
	major, minor float64
	orient       float64
}

// wlTouchPoint — палец, известный приёмнику.
type wlTouchPoint struct {
	popup   bool    // упал на попап
	popupID uintptr // какой именно
	x, y    int     // последняя точка, физические пиксели (в системе своей поверхности)
}

// wlTouchSink переводит события wl_touch в события окна. Все методы зовёт одна
// горутина — цикл событий Wayland.
type wlTouchSink struct {
	// scale — масштаб поверхности (поверхностные единицы -> пиксели буфера).
	scale func() float64
	// popupFor — какой попап стоит за поверхностью (ok=false — это окно).
	popupFor func(surface uint32) (id uintptr, ok bool)
	// onTouch — приёмник касаний окна.
	onTouch func(ev widget.TouchEvent)
	// popup — приёмники мыши попапов.
	popup func() childPopupHandlers
	// serial запоминает serial ввода (нужен xdg_popup.grab и буферу обмена).
	serial func(uint32)

	// shapes сообщает, что wl_touch версии 6 и события надо копить до frame,
	// чтобы приложить к ним форму контакта (nil — нет).
	shapes func() bool

	points map[int32]*wlTouchPoint
	// shape — форма контакта пальцев; pend — события, ждущие frame.
	shape map[int32]wlShape
	pend  []widget.TouchEvent
	// popupPrimary — палец, превращённый в мышь попапа.
	popupPrimary    int32
	hasPopupPrimary bool
}

// px переводит поверхностную координату в физический пиксель.
func (t *wlTouchSink) px(v float64) int {
	k := 1.0
	if t.scale != nil {
		if s := t.scale(); s > 0 {
			k = s
		}
	}
	return int(math.Floor(v*k + 0.5))
}

func wlTime(ms uint32) time.Duration { return time.Duration(ms) * time.Millisecond }

// emit отдаёт событие приёмнику окна: сразу или, если компоновщик присылает
// форму контакта, после frame.
func (t *wlTouchSink) emit(id int32, ph widget.TouchPhase, ms uint32, x, y int) {
	if t.onTouch == nil {
		return
	}
	ev := widget.TouchEvent{ID: int(id), X: x, Y: y, Phase: ph, Time: wlTime(ms)}
	if t.shapes != nil && t.shapes() {
		t.pend = append(t.pend, ev)
		return
	}
	t.onTouch(ev)
}

// flush отдаёт накопленные события с формой контакта; зовётся на frame, отмене и
// выключении. Оси переводятся в пиксели буфера тем же масштабом, что и координаты.
func (t *wlTouchSink) flush() {
	pend := t.pend
	t.pend = nil
	k := 1.0
	if t.scale != nil {
		if s := t.scale(); s > 0 {
			k = s
		}
	}
	for _, ev := range pend {
		if sh, ok := t.shape[int32(ev.ID)]; ok && sh.major > 0 {
			ev.Major, ev.Minor, ev.Orientation = sh.major*k, sh.minor*k, sh.orient
		}
		if t.onTouch != nil {
			t.onTouch(ev)
		}
		if ev.Phase.Terminal() {
			delete(t.shape, int32(ev.ID))
		}
	}
}

// noteShape запоминает форму контакта известного пальца.
func (t *wlTouchSink) noteShape(id int32, f func(*wlShape)) {
	if t.points[id] == nil {
		return
	}
	if t.shape == nil {
		t.shape = make(map[int32]wlShape)
	}
	sh := t.shape[id]
	f(&sh)
	t.shape[id] = sh
}

// handle разбирает одно событие wl_touch.
func (t *wlTouchSink) handle(opcode uint16, b []byte) {
	switch opcode {
	case wlTouchEvDown:
		d, ok := parseWlTouchDown(b)
		if !ok {
			return
		}
		if t.serial != nil {
			t.serial(d.Serial)
		}
		if t.points == nil {
			t.points = make(map[int32]*wlTouchPoint)
		}
		p := &wlTouchPoint{x: t.px(d.X), y: t.px(d.Y)}
		t.points[d.ID] = p
		if t.popupFor != nil {
			if pid, isPopup := t.popupFor(d.Surface); isPopup {
				p.popup, p.popupID = true, pid
				if !t.hasPopupPrimary {
					t.popupPrimary, t.hasPopupPrimary = d.ID, true
					if h := t.popup(); h.Move != nil && h.Button != nil {
						h.Move(pid, p.x, p.y)
						h.Button(pid, p.x, p.y, 0, true)
					}
				}
				return
			}
		}
		t.emit(d.ID, widget.TouchBegan, d.Time, p.x, p.y)

	case wlTouchEvMotion:
		ms, id, fx, fy, ok := parseWlTouchMotion(b)
		if !ok {
			return
		}
		p := t.points[id]
		if p == nil {
			return
		}
		p.x, p.y = t.px(fx), t.px(fy)
		if p.popup {
			if t.hasPopupPrimary && t.popupPrimary == id {
				if h := t.popup(); h.Move != nil {
					h.Move(p.popupID, p.x, p.y)
				}
			}
			return
		}
		t.emit(id, widget.TouchMoved, ms, p.x, p.y)

	case wlTouchEvUp:
		serial, ms, id, ok := parseWlTouchUp(b)
		if !ok {
			return
		}
		if t.serial != nil {
			t.serial(serial)
		}
		p := t.points[id]
		if p == nil {
			return
		}
		delete(t.points, id)
		if p.popup {
			if t.hasPopupPrimary && t.popupPrimary == id {
				t.hasPopupPrimary = false
				if h := t.popup(); h.Move != nil && h.Button != nil {
					h.Move(p.popupID, p.x, p.y)
					h.Button(p.popupID, p.x, p.y, 0, false)
				}
			}
			return
		}
		t.emit(id, widget.TouchEnded, ms, p.x, p.y)

	case wlTouchEvCancel:
		t.cancelAll()

	case wlTouchEvShape:
		if id, major, minor, ok := parseWlTouchShape(b); ok {
			t.noteShape(id, func(s *wlShape) { s.major, s.minor = major, minor })
		}

	case wlTouchEvOrientation:
		if id, deg, ok := parseWlTouchOrientation(b); ok {
			t.noteShape(id, func(s *wlShape) { s.orient = deg })
		}

	case wlTouchEvFrame:
		// Группировка событий одного момента. Без формы контакта доставляем
		// сразу и ждать нечего; с формой (версия 6) события копились до сих пор.
		t.flush()
	}
}

// cancelAll отменяет все касания: компоновщик забрал их себе (системный жест,
// потеря ввода). Отпускания мы уже не получим.
func (t *wlTouchSink) cancelAll() {
	t.flush() // всё, что уже случилось, доходит до отмены
	for id, p := range t.points {
		if p.popup {
			if t.hasPopupPrimary && t.popupPrimary == id {
				if h := t.popup(); h.Move != nil && h.Button != nil {
					// Кнопка отпускается вне попапа: пункт меню не сработает.
					h.Move(p.popupID, pointerOutside, pointerOutside)
					h.Button(p.popupID, pointerOutside, pointerOutside, 0, false)
				}
			}
			continue
		}
		if t.onTouch != nil {
			t.onTouch(widget.TouchEvent{ID: int(id), X: p.x, Y: p.y, Phase: widget.TouchCancelled})
		}
	}
	t.points = nil
	t.shape = nil
	t.hasPopupPrimary = false
}

// drop молча забывает все касания (приём выключен: отменять уже некому).
func (t *wlTouchSink) drop() {
	t.points = nil
	t.shape = nil
	t.pend = nil
	t.hasPopupPrimary = false
}
