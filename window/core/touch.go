package core

// touch.go — касания: от бэкенда окна к движку.
//
// Касания — режим по заявке. Пока окно не просило о них (SetTouchEnabled), ни
// один бэкенд их не ловит, и приложение получает от ОС то же, что получало всегда:
// Windows сама превращает касание в мышь, Wayland-компоновщик и X-сервер
// делают это для клиентов без wl_touch и без XInput2-касаний. Поэтому приложения,
// не знающие о касаниях (Проводник, Калькулятор, Блокнот, оболочка), ведут себя
// так же, как раньше, без единой правки.
//
// Окно, запросившее касания, берёт их на себя целиком, а значит, и мышь из
// касаний: бэкенд перестаёт получать её от ОС (Win32 не отдаёт WM_POINTER системе
// и мыши из касания не будет; Wayland и X11 доставляют касания как касания). Чтобы
// обычные кнопки, меню и поля ввода остались живыми под пальцем, окно делает то,
// что делала ОС, само — но только для касаний, которых не взял ни один виджет
// (engine.SendTouch вернул false): первый такой палец превращается в левую кнопку
// мыши, движение и отрыв — в движение и отпускание. Касание, взятое виджетом
// (widget.TouchHandler), в мышь не превращается вовсе. Прокрутка и длинное
// нажатие из касания ОС при этом не делает, а окно воспроизводит только его
// (см. emulateMouse): палец, пролежавший на месте дольше SetTouchLongPress, при
// отрыве даёт правую кнопку мыши (контекстное меню), как у Windows. Прокрутка
// остаётся за приложением: widget.GestureRecognizer даёт для неё Pan и Swipe.

import (
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/platform/pointer"
	"github.com/oops1/headless-gui/v3/widget"
)

// touchSource — бэкенд, умеющий касания.
type touchSource interface {
	// SetTouchEnabled включает или выключает приём касаний. Зовётся ДО Create
	// (Wayland берёт wl_touch, X11 выбирает события XInput2 при создании окна) и
	// может быть вызван позже (X11 тогда делает это в цикле событий). Выключенный бэкенд
	// не ловит касаний и не мешает ОС порождать мышь.
	SetTouchEnabled(on bool)
	// SetOnTouch подписывает приёмник касаний. X, Y — физические пиксели
	// клиентской области окна; Time — штамп ОС (ноль, если бэкенд его не знает).
	// Поток события — поток бэкенда; окно ставит его в очередь движка само.
	SetOnTouch(fn func(ev widget.TouchEvent))
}

// touchEngine — то, что окно требует от движка для касаний (реализует
// *engine.Engine). Опциональный интерфейс, как и прочие расширения EngineAPI:
// движок без него просто не получит касаний.
type touchEngine interface {
	SendTouch(ev widget.TouchEvent) bool
	CancelTouches()
}

// SetTouchEnabled включает или выключает приём касаний окном (см. описание
// режима в начале touch.go). Можно вызывать до Run и позже: на X11 включение
// после создания окна выполняется отложенно, из цикла событий (XInput2 надо
// согласовать с сервером запросами с ответом, а их безопасно делать лишь между
// событиями), поэтому касания начинаются чуть позже самого вызова.
//
// Не вызывать — не то же, что выключить: окно, чей корень сам реализует
// widget.TouchHandler, включается автоматически. Явное решение приложения
// всегда важнее.
func (win *Window) SetTouchEnabled(on bool) *Window {
	win.touchWant = &on
	if win.native == nil {
		return win
	}
	if ts, ok := win.native.(touchSource); ok {
		win.onNative(func() { ts.SetTouchEnabled(on) })
		if on {
			win.setupTouch()
		} else {
			win.post(win.cancelTouches)
		}
	}
	return win
}

// SetTouchLongPress задаёт, как долго «ничей» палец должен пролежать на месте,
// чтобы при отрыве окно превратило его в щелчок правой кнопкой мыши вместо
// левой (см. SetTouchEnabled). 0 — по умолчанию (widget.DefaultGestureConfig:
// 500 мс), отрицательное — не превращать никогда. Касаний, взятых виджетами,
// это не касается.
func (win *Window) SetTouchLongPress(d time.Duration) *Window {
	win.emuLongPress.Store(int64(d))
	return win
}

// TouchEnabled сообщает, принимает ли окно касания сейчас (или примет, когда
// появится).
func (win *Window) TouchEnabled() bool {
	if win.touchWant != nil {
		return *win.touchWant
	}
	if root := win.eng.Root(); root != nil {
		_, ok := root.(widget.TouchHandler)
		return ok
	}
	return false
}

// prepareTouch сообщает бэкенду решение до создания окна ОС.
func (win *Window) prepareTouch() {
	if !win.TouchEnabled() {
		return
	}
	if ts, ok := win.native.(touchSource); ok {
		ts.SetTouchEnabled(true)
	}
}

// setupTouch подписывает бэкенд на касания и проводит их в движок. Зовётся
// после Create; повторный вызов безвреден.
func (win *Window) setupTouch() {
	ts, ok := win.native.(touchSource)
	if !ok || !win.TouchEnabled() {
		return
	}
	if _, ok := win.eng.(touchEngine); !ok {
		return
	}
	ts.SetOnTouch(func(ev widget.TouchEvent) {
		// Событие уходит в очередь движка целиком, без склейки движений: у
		// щипка два пальца, и потерянное Moved одного исказило бы масштаб.
		win.post(func() { win.deliverTouch(ev) })
	})
}

// touchEmu — палец, превращённый окном в мышь. Только горутина движка.
type touchEmu struct {
	active bool
	id     int
	// x0, y0 — где палец коснулся; moved — он уходил дальше порога тапа (и
	// тогда не «лежал», что бы ни было потом); evT/wall — когда коснулся: по
	// часам события, если бэкенд их знает, иначе по часам прихода.
	x0, y0 int
	moved  bool
	evT    time.Duration
	wall   time.Time
}

// emuNowTime — часы прихода событий; подменяются в тестах.
func (s *surface) emuNowTime() time.Time {
	if s.emuNow != nil {
		return s.emuNow()
	}
	return time.Now()
}

// emuLongPressDuration — сколько палец должен пролежать, чтобы это стало
// долгим нажатием (<= 0 — никогда).
func (s *surface) emuLongPressDuration() time.Duration {
	if d := time.Duration(s.emuLongPress.Load()); d != 0 {
		return d
	}
	return widget.DefaultGestureConfig().LongPressDuration
}

// emuSlop — порог тапа в физических пикселях окна.
func (s *surface) emuSlop() float64 {
	k := s.scale
	if k <= 0 {
		k = 1
	}
	return float64(widget.DefaultGestureConfig().TapSlop) * k
}

// emuIsLongPress — палец, отрываемый в (x, y) в момент ev, всё это время лежал.
func (s *surface) emuIsLongPress(ev widget.TouchEvent) bool {
	d := s.emuLongPressDuration()
	if d <= 0 || s.emu.moved {
		return false
	}
	if math.Hypot(float64(ev.X-s.emu.x0), float64(ev.Y-s.emu.y0)) > s.emuSlop() {
		return false
	}
	held := s.emuNowTime().Sub(s.emu.wall)
	if s.emu.evT > 0 && ev.Time > 0 {
		held = ev.Time - s.emu.evT
	}
	return held >= d
}

// deliverTouch — касание на горутине движка: виджетам, а если никто не взял —
// в мышь (см. начало файла).
func (s *surface) deliverTouch(ev widget.TouchEvent) {
	te, ok := s.eng.(touchEngine)
	if !ok {
		return
	}
	ev.X, ev.Y = s.toContent(ev.X, ev.Y)
	if te.SendTouch(ev) {
		return
	}
	s.emulateMouse(ev)
}

// emulateMouse превращает «ничьё» касание в левую кнопку мыши. Мышь одна,
// поэтому следим за первым пальцем; остальные «ничьи» пальцы игнорируются.
//
// Долгое нажатие (палец лёг и не сдвинулся дальше порога тапа, SetTouchLongPress)
// при отрыве становится правым щелчком, как у Windows: действие по отрыву, а не
// по истечении срока, поэтому палец, который долго целится в ползунок или
// выделение и потом тянет, остаётся обычной левой кнопкой. Левая кнопка к
// отрыву уже нажата (отклик нажатия нужен сразу), и её отпускают вне окна —
// тем же приёмом, что при отмене, чтобы кнопка или пункт не сработали, — а затем
// в точке отрыва давят и отпускают правую.
func (s *surface) emulateMouse(ev widget.TouchEvent) {
	switch ev.Phase {
	case widget.TouchBegan:
		if s.emu.active {
			return
		}
		s.emu = touchEmu{active: true, id: ev.ID, x0: ev.X, y0: ev.Y, evT: ev.Time, wall: s.emuNowTime()}
		s.eng.SendMouseMove(ev.X, ev.Y)
		s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseLeft, true)
	case widget.TouchMoved:
		if s.emu.active && s.emu.id == ev.ID {
			if math.Hypot(float64(ev.X-s.emu.x0), float64(ev.Y-s.emu.y0)) > s.emuSlop() {
				s.emu.moved = true
			}
			s.eng.SendMouseMove(ev.X, ev.Y)
		}
	case widget.TouchEnded:
		if s.emu.active && s.emu.id == ev.ID {
			long := s.emuIsLongPress(ev)
			if long {
				s.releaseEmulatedMouse()
				s.eng.SendMouseMove(ev.X, ev.Y)
				s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseRight, true)
				s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseRight, false)
				return
			}
			s.emu = touchEmu{}
			s.eng.SendMouseMove(ev.X, ev.Y)
			s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseLeft, false)
		}
	case widget.TouchCancelled:
		if s.emu.active && s.emu.id == ev.ID {
			s.releaseEmulatedMouse()
		}
	}
}

// releaseEmulatedMouse отпускает эмулируемую кнопку вне окна: движение за край
// снимает наведение, а отпускание там не засчитывается кликом ни кнопке, ни
// ссылке.
func (s *surface) releaseEmulatedMouse() {
	s.emu = touchEmu{}
	s.eng.SendMouseMove(pointer.Outside, pointer.Outside)
	s.eng.SendMouseButton(pointer.Outside, pointer.Outside, widget.MouseLeft, false)
}

// cancelTouches отменяет все касания: владельцам-виджетам уходит Cancelled, а
// эмулируемая мышь отпускается. Горутина движка.
func (s *surface) cancelTouches() {
	if te, ok := s.eng.(touchEngine); ok {
		te.CancelTouches()
	}
	if s.emu.active {
		s.releaseEmulatedMouse()
	}
}
