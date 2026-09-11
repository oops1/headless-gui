// title_close.go — смена заголовка окна и право остановить закрытие.
//
// Заголовок задавался один раз, в window.New, и больше не менялся: показать
// «*» при несохранённых правках было нечем. А закрытие окна — и кнопкой ×, и
// средствами ОС (Alt+F4, панель задач) — проходило молча: SetOnClose нативного
// окна всегда отвечал «можно», и спросить «Сохранить изменения?» приложение не
// успевало — правки терялись вместе с окном.
package window

import "github.com/oops1/headless-gui/v3/widget"

// poster — движок, умеющий выполнить функцию на своей горутине (engine.Post).
type poster interface {
	Post(fn func())
}

// SetTitle меняет заголовок окна — и в полосе, которую рисует движок, и там,
// где его показывает ОС (панель задач, Alt+Tab).
//
//	win.SetTitle("difftool — main.go *")
//
// Вызывать с горутины движка — из обработчика или через engine.Post, как и
// любое изменение виджетов. До Run() заголовок запоминается и уходит в окно при
// создании.
func (win *Window) SetTitle(title string) {
	win.title = title
	if root, ok := win.eng.Root().(*widget.Window); ok {
		root.SetTitle(title)
	}
	n := win.native
	if n == nil {
		return
	}
	// Заголовок окна ОС меняется на потоке его цикла сообщений: Win32 шлёт
	// WM_SETTEXT синхронно, и вызов с чужой горутины ждал бы поток, который
	// сам может ждать движок.
	if inv, ok := n.(uiThreadInvoker); ok {
		inv.InvokeOnUIThread(func() { n.SetTitle(title) })
		return
	}
	n.SetTitle(title)
}

// Title возвращает текущий заголовок окна.
func (win *Window) Title() string { return win.title }

// SetOnCloseRequest задаёт вопрос «можно ли закрывать» — перед закрытием
// кнопкой × и средствами ОС (Alt+F4, панель задач).
//
// false останавливает закрытие, и окно остаётся на экране. Решение, принятое
// позже (пользователь ответил в диалоге), приложение исполняет само — вызовом
// Close:
//
//	win.SetOnCloseRequest(func() bool {
//	    if !view.IsModified(widget.DiffLeft) {
//	        return true
//	    }
//	    askSave(func(ok bool) { if ok { win.Close() } })
//	    return false // пока не закрываемся
//	})
//
// Хук зовётся на горутине движка — там же, где обработчики виджетов, и из него
// можно сразу показать диалог. Закрытие средствами ОС для этого откладывается:
// окну ОС отвечают «пока нет», а вопрос уходит движку через Post. Спросить
// прямо в насосе событий ОС значило бы показывать диалог из чужого потока, а
// ждать там ответа — повесить окно.
//
// Close (закрытие по команде приложения) хук не спрашивает: это уже решение
// приложения. nil снимает хук — закрытие снова идёт без вопросов.
func (win *Window) SetOnCloseRequest(fn func() bool) {
	win.closeMu.Lock()
	win.onCloseRequest = fn
	win.closeMu.Unlock()
}

// requestClose — общий путь кнопки × и закрытия средствами ОС: спросить
// приложение и закрыть, если оно не против. Зовётся на горутине движка.
func (win *Window) requestClose() {
	win.closeMu.Lock()
	fn := win.onCloseRequest
	win.closeMu.Unlock()
	if fn != nil && !fn() {
		return
	}
	win.Close()
}

// hasCloseHook сообщает, задан ли вопрос перед закрытием.
func (win *Window) hasCloseHook() bool {
	win.closeMu.Lock()
	defer win.closeMu.Unlock()
	return win.onCloseRequest != nil
}

// postToEngine выполняет fn на горутине движка; движок без Post — отдельной
// горутиной, но никогда не в насосе событий ОС (см. SetOnCloseRequest).
func (win *Window) postToEngine(fn func()) {
	if p, ok := win.eng.(poster); ok {
		p.Post(fn)
		return
	}
	go fn()
}
