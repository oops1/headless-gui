package keyboard

// keygrab.go — фильтр системных сочетаний при захвате клавиатуры.

import "github.com/oops1/headless-gui/v4/platform/native"

// Коды клавиш, которые различает фильтр. Совпадают с VK_* Windows и с
// платформенно-независимыми VK_* пакета; левые/правые Ctrl и Alt — отдельные
// коды, которые видит низкоуровневый хук.
const (
	grabVKLControl = 0xA2
	grabVKRControl = 0xA3
	grabVKLMenu    = 0xA4
	grabVKRMenu    = 0xA5
)

// GrabFilter решает, какие клавиши забирает захват. Чистая логика без
// обращений к системе: хук Win32 кормит её событиями, а тесты — списком
// нажатий.
//
// Состояние нужно, потому что решение зависит от истории: пока зажата Win,
// перехватывается ВСЯ клавиатура; отпускание перехваченной клавиши обязано
// быть перехвачено тоже (иначе система увидит отпускание без нажатия);
// автоповтор — это повторное нажатие уже перехваченной клавиши.
//
//facadegen:skip
type GrabFilter struct {
	win      map[int]bool // зажатые клавиши Win, которые мы забрали
	swallow  map[int]bool // перехваченные и ещё не отпущенные клавиши
	ctrlHeld bool
	altHeld  bool
}

// GrabAction — что делать с событием.
//
//facadegen:skip
type GrabAction struct {
	// Swallow — не отдавать систему, вернуть из хука 1.
	Swallow bool
	// Deliver — показать клавишу окну (KeyDown/KeyUp). Всегда вместе с Swallow.
	Deliver bool
	// Repeat — нажатие является автоповтором.
	Repeat bool
}

// event разбирает одно событие клавиши. vk — код клавиши Windows, up —
// отпускание, altFlag — признак «Alt зажат» из самого события (LLKHF_ALTDOWN),
// ctrlNow — Ctrl зажат по данным системы (когда хук его не видел), active —
// наше окно активно.
func (f *GrabFilter) Event(vk int, up, altFlag, ctrlNow, active bool) GrabAction {
	if f.swallow == nil {
		f.swallow = map[int]bool{}
		f.win = map[int]bool{}
	}
	// Ctrl и Alt ведём сами: они нужны как модификаторы сочетаний. Сами эти
	// клавиши не перехватываются — окно получает их обычным путём.
	switch vk {
	case grabVKLControl, grabVKRControl, native.VK_CONTROL:
		f.ctrlHeld = !up
	case grabVKLMenu, grabVKRMenu, native.VK_ALT:
		f.altHeld = !up
	}

	if up {
		if f.swallow[vk] {
			delete(f.swallow, vk)
			delete(f.win, vk)
			return GrabAction{Swallow: true, Deliver: true}
		}
		return GrabAction{}
	}
	if f.swallow[vk] {
		// Автоповтор перехваченной клавиши: система его уже не увидит, а
		// окну он нужен как повтор. Даже если фокус только что ушёл: иначе
		// половина удержанной комбинации утекла бы системе.
		return GrabAction{Swallow: true, Deliver: active, Repeat: true}
	}
	if !active {
		return GrabAction{}
	}

	alt := altFlag || f.altHeld
	ctrl := ctrlNow || f.ctrlHeld
	grab := false
	switch vk {
	case native.VK_LWIN, native.VK_RWIN:
		grab = true
	case native.VK_TAB:
		grab = alt // Alt+Tab
	case native.VK_ESCAPE:
		grab = alt || ctrl // Alt+Esc, Ctrl+Esc, Ctrl+Shift+Esc
	case native.VK_SNAPSHOT:
		grab = true // PrintScreen с любыми модификаторами
	case native.VK_F4:
		grab = alt // Alt+F4
	case native.VK_SPACE:
		grab = alt // Alt+Space (оконное меню системы)
	}
	if !grab && len(f.win) > 0 {
		grab = true // Win+что угодно: Win+E, Win+D, Win+R, Win+стрелки
	}
	if !grab {
		return GrabAction{}
	}
	f.swallow[vk] = true
	if vk == native.VK_LWIN || vk == native.VK_RWIN {
		f.win[vk] = true
	}
	return GrabAction{Swallow: true, Deliver: true}
}

// reset забывает всё: захват выключили или окно закрыли.
func (f *GrabFilter) Reset() {
	f.swallow, f.win = nil, nil
	f.ctrlHeld, f.altHeld = false, false
}

// held — клавиши, перехваченные и ещё не отпущенные. При выключении захвата
// окну нужно «отпустить» их самому: система отпускания уже не получит как
// перехваченное, а приложение должно знать, что клавиша больше не зажата.
func (f *GrabFilter) Held() []int {
	out := make([]int, 0, len(f.swallow))
	for vk := range f.swallow {
		out = append(out, vk)
	}
	return out
}
