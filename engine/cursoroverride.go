// cursoroverride.go — принудительная форма курсора поверх курсора виджетов.
//
// Курсор до сих пор определялся только тем, что лежит под мышью, и менялся
// только после её движения. Приложению этого мало: просмотрщик изображений
// прячет курсор после паузы без движения мыши (мыши-то как раз никто не
// двигает), а на время перетаскивания показывает «сжатую руку» над любым
// виджетом, в том числе над тем, что курсор задаёт сам.
//
// SetCursorOverride задаёт такую форму поверх всего дерева; окно узнаёт о ней
// через SetOnCursorOverrideChanged и применяет сразу, не дожидаясь движения.
package engine

import (
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
)

// cursorOverride — состояние принудительного курсора. Нулевое значение готово
// к работе: override не задан. Замок собственный: вызвать SetCursorOverride
// могут с любой горутины (таймер бездействия), а CursorAt читает его на
// горутине движка.
type cursorOverride struct {
	mu       sync.Mutex
	c        widget.Cursor
	set      bool
	onChange func()
}

// SetCursorOverride задаёт форму курсора, которая перекрывает курсор любого
// виджета под мышью, пока не будет снята ClearCursorOverride.
//
// Форма применяется к окну немедленно — без ожидания движения мыши: так можно
// спрятать курсор (widget.CursorNone) по таймеру бездействия. Безопасен для
// вызова с любой горутины. Повторный вызов с той же формой окно не беспокоит.
//
// Форма действует и над вынесенными попапами (меню и выпадающие списки в
// отдельных окнах ОС): окно движка (window.Run) переносит её на каждый попап,
// в том числе открытый уже после вызова. Снятая — возвращает попапам стрелку.
func (e *Engine) SetCursorOverride(c widget.Cursor) {
	o := &e.cursorOvr
	o.mu.Lock()
	changed := !o.set || o.c != c
	o.c, o.set = c, true
	fn := o.onChange
	o.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// ClearCursorOverride снимает принудительный курсор: форму снова определяет
// виджет под мышью. Окно возвращает её сразу, по последнему положению мыши.
func (e *Engine) ClearCursorOverride() {
	o := &e.cursorOvr
	o.mu.Lock()
	changed := o.set
	o.c, o.set = widget.CursorArrow, false
	fn := o.onChange
	o.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// CursorOverride возвращает принудительную форму курсора и признак, что она
// задана.
func (e *Engine) CursorOverride() (widget.Cursor, bool) {
	o := &e.cursorOvr
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.c, o.set
}

// SetOnCursorOverrideChanged подключает уведомление о том, что принудительный
// курсор задан, сменён или снят. Его ставит окно (window.Run): по уведомлению
// оно заново спрашивает CursorAt и отдаёт форму ОС. fn зовётся на горутине
// того, кто менял override, вне замков движка; nil снимает уведомление.
func (e *Engine) SetOnCursorOverrideChanged(fn func()) {
	o := &e.cursorOvr
	o.mu.Lock()
	o.onChange = fn
	o.mu.Unlock()
}
