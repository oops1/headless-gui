package window

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
// нажатие из касания (контекстное меню) ОС при этом не делает — они остаются за
// приложением: widget.GestureRecognizer даёт для них Pan и Swipe.

import "github.com/oops1/headless-gui/v3/widget"

// touchSource — бэкенд, умеющий касания.
type touchSource interface {
	// SetTouchEnabled включает или выключает приём касаний. Зовётся ДО Create
	// (бэкенду нужно знать заранее: Wayland берёт wl_touch, X11 выбирает события
	// XInput2 при создании окна) и может быть вызван позже. Выключенный бэкенд
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
// режима в начале touch.go). Вызывать до Run; на Windows и Wayland можно и
// позже, на X11 включение после создания окна не действует (XInput2 надо
// согласовать с сервером при создании), а выключение — действует.
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
func (s *surface) emulateMouse(ev widget.TouchEvent) {
	switch ev.Phase {
	case widget.TouchBegan:
		if s.emu.active {
			return
		}
		s.emu = touchEmu{active: true, id: ev.ID}
		s.eng.SendMouseMove(ev.X, ev.Y)
		s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseLeft, true)
	case widget.TouchMoved:
		if s.emu.active && s.emu.id == ev.ID {
			s.eng.SendMouseMove(ev.X, ev.Y)
		}
	case widget.TouchEnded:
		if s.emu.active && s.emu.id == ev.ID {
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
	s.eng.SendMouseMove(pointerOutside, pointerOutside)
	s.eng.SendMouseButton(pointerOutside, pointerOutside, widget.MouseLeft, false)
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
