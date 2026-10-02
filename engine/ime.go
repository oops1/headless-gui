package engine

// ime.go — незавершённый ввод (композиция) от системы к фокусному виджету.
//
// Китайский, японский и корейский набираются не по букве: человек печатает
// слоги, система показывает список кандидатов, и только выбранный вариант
// становится текстом. До движка доходили лишь готовые символы, поэтому
// набрать иероглиф в нём было нельзя вовсе.
//
// Здесь движок принимает от окна три события — «набирается», «введено»,
// «отменено» — и передаёт их фокусному виджету. Окно взамен спрашивает, где
// стоит каретка: по этому месту система ставит окно кандидатов, иначе оно
// появляется в углу экрана, далеко от набираемого слова.

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// SendComposition передаёт фокусному виджету набираемый текст; caret —
// позиция курсора ВНУТРИ него (в рунах).
//
// Пустой текст означает «набранное убрать»: так система сообщает об отмене
// последнего слога.
func (e *Engine) SendComposition(text string, caret int) {
	e.imePost(func(c widget.IMEComposer) { c.IMESetComposition(text, caret) })
}

// CommitComposition завершает ввод: текст становится содержимым поля.
func (e *Engine) CommitComposition(text string) {
	e.imePost(func(c widget.IMEComposer) { c.IMECommit(text) })
}

// CancelComposition отменяет ввод: набранное убирается без следа.
func (e *Engine) CancelComposition() {
	e.imePost(func(c widget.IMEComposer) { c.IMECancel() })
}

// imePost выполняет действие над фокусным виджетом на горутине движка:
// события IME приходят из насоса сообщений ОС, а дерево виджетов принадлежит
// движку.
func (e *Engine) imePost(fn func(widget.IMEComposer)) {
	e.Post(func() {
		c, ok := e.focus.get().(widget.IMEComposer)
		if !ok {
			return
		}
		fn(c)
		e.Invalidate()
	})
}

// IMEActive сообщает, принимает ли фокусный виджет незавершённый ввод.
//
// По нему окно решает, включать ли ввод вообще: над кнопкой или списком
// редактор метода ввода не нужен, и держать его включённым — значит
// показывать человеку окно кандидатов там, где печатать некуда.
func (e *Engine) IMEActive() bool {
	_, ok := e.focus.get().(widget.IMEComposer)
	return ok
}

// CaretRect возвращает место каретки фокусного виджета в ФИЗИЧЕСКИХ пикселях
// окна и признак, что оно известно.
//
// Физические, а не логические: координаты уходят прямо в системный вызов
// (окно кандидатов на Windows, прямоугольник курсора на Wayland), а система
// считает в пикселях экрана.
func (e *Engine) CaretRect() (image.Rectangle, bool) {
	c, ok := e.focus.get().(widget.IMEComposer)
	if !ok {
		return image.Rectangle{}, false
	}
	r := c.IMECaretRect()
	if r.Empty() {
		return image.Rectangle{}, false
	}
	k := e.Scale()
	if k == 1 {
		return r, true
	}
	return image.Rect(
		int(float64(r.Min.X)*k+0.5), int(float64(r.Min.Y)*k+0.5),
		int(float64(r.Max.X)*k+0.5), int(float64(r.Max.Y)*k+0.5),
	), true
}
