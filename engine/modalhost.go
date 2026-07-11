package engine

import "github.com/oops1/headless-gui/v3/widget"

// ModalHost — хост, показывающий модальные виджеты в собственных нативных
// окнах ОС. Реализуется пакетом window для бэкендов с поддержкой owner-окон
// (Фаза 1 — только Win32). Если хост не установлен или отказывается принять
// модалку, движок показывает её в своём холсте — headless-контракт и прежнее
// in-canvas поведение (центрирование, клэмп) сохраняются без изменений.
type ModalHost interface {
	// ShowModal пытается показать модалку в собственном нативном окне.
	// true — модалка целиком у хоста (движок её не отслеживает: не добавляет
	// в стек, не инжектит capture, не центрирует).
	// false — движок показывает её в своём холсте (headless или бэкенд без
	// поддержки).
	ShowModal(m widget.ModalWidget) bool
	// CloseModal закрывает модалку, показанную хостом.
	// false — модалка не хостится (движок закрывает её сам).
	CloseModal(m widget.ModalWidget) bool
}

// SetModalHost устанавливает хост нативных модалок (nil — снять).
// Обычно вызывается пакетом window в Window.Run() при поддержке бэкенда.
func (e *Engine) SetModalHost(h ModalHost) {
	e.hostMu.Lock()
	e.modalHost = h
	e.hostMu.Unlock()
}

func (e *Engine) getModalHost() ModalHost {
	e.hostMu.Lock()
	defer e.hostMu.Unlock()
	return e.modalHost
}

// SetOnModalClosed регистрирует колбэк, вызываемый в конце CloseModal после
// фактического закрытия модалки ЭТИМ движком (т.е. когда она была в стеке
// движка, а не у хоста). Используется хостом на вторичном движке: диалог,
// закрытый своим closer'ом (✕/Escape) → secondEng.CloseModal → колбэк →
// хост уничтожает нативное окно.
func (e *Engine) SetOnModalClosed(fn func(m widget.ModalWidget)) {
	e.hostMu.Lock()
	e.onModalClosed = fn
	e.hostMu.Unlock()
}

func (e *Engine) fireOnModalClosed(m widget.ModalWidget) {
	e.hostMu.Lock()
	fn := e.onModalClosed
	e.hostMu.Unlock()
	if fn != nil {
		fn(m)
	}
}
