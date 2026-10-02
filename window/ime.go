package window

// ime.go — связь редактора метода ввода с движком.
//
// Набираемое («композиция») приходит от системы в окно, а показать его должно
// поле ввода — то есть виджет под фокусом. Здесь эти два конца соединяются.
//
// Оба интерфейса необязательные: бэкенд без поддержки IME (X11, macOS)
// колбэков не получает, движок без приёма композиции не трогается, и всё
// ведёт себя как раньше — до поля доходят только готовые символы.

import "image"

// imeBackend — бэкенд, умеющий редактор метода ввода.
type imeBackend interface {
	SetOnIMEComposition(fn func(text string, caret int))
	SetOnIMECommit(fn func(text string))
	// SetIMECaretProvider — источник места каретки в физических пикселях
	// клиентской области: по нему система ставит окно кандидатов.
	SetIMECaretProvider(fn func() (image.Rectangle, bool))
}

// imeSink — движок, принимающий незавершённый ввод (реализует *engine.Engine).
type imeSink interface {
	SendComposition(text string, caret int)
	CommitComposition(text string)
	CaretRect() (image.Rectangle, bool)
}

// setupIME подключает композицию бэкенда к движку.
func (win *Window) setupIME() {
	be, ok := win.native.(imeBackend)
	if !ok {
		return
	}
	sink, ok := win.eng.(imeSink)
	if !ok {
		return
	}
	be.SetOnIMEComposition(sink.SendComposition)
	be.SetOnIMECommit(sink.CommitComposition)
	be.SetIMECaretProvider(sink.CaretRect)
}
