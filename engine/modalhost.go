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

// modalClosedSub — подписчик закрытия модалки с дескриптором для отписки.
type modalClosedSub struct {
	id int
	fn func(widget.ModalWidget)
}

// AddOnModalClosed подписывает колбэк на закрытие модалки и возвращает
// дескриптор для RemoveOnModalClosed.
//
// SetOnModalClosed хранит ОДИН колбэк, и он занят хостом нативных окон:
// приложение, подписавшееся им, вытесняло хост или вытеснялось само (GG-82).
// Подписок может быть сколько угодно; зовутся они в порядке подписки, после
// колбэка SetOnModalClosed.
//
// Колбэк приходит и для диалога, показанного в собственном окне ОС, — в том
// числе когда его закрыли крестиком окна или Alt+F4.
func (e *Engine) AddOnModalClosed(fn func(m widget.ModalWidget)) int {
	if fn == nil {
		return 0
	}
	e.hostMu.Lock()
	defer e.hostMu.Unlock()
	e.modalSubNextID++
	id := e.modalSubNextID
	e.modalClosedSubs = append(e.modalClosedSubs, modalClosedSub{id: id, fn: fn})
	return id
}

// RemoveOnModalClosed снимает подписку по дескриптору (no-op, если нет).
func (e *Engine) RemoveOnModalClosed(id int) {
	if id == 0 {
		return
	}
	e.hostMu.Lock()
	defer e.hostMu.Unlock()
	for i, s := range e.modalClosedSubs {
		if s.id == id {
			e.modalClosedSubs = append(e.modalClosedSubs[:i], e.modalClosedSubs[i+1:]...)
			return
		}
	}
}

// NotifyModalClosed сообщает подписчикам движка, что модалка закрыта.
//
// Зовётся хостом нативных окон: диалог в собственном окне ОС закрывается мимо
// CloseModal этого движка — его закрывает вторичный движок окна, — и без этого
// приложение не узнавало о закрытии крестиком или Alt+F4 (GG-82).
func (e *Engine) NotifyModalClosed(m widget.ModalWidget) {
	e.fireOnModalClosed(m)
}

func (e *Engine) fireOnModalClosed(m widget.ModalWidget) {
	e.hostMu.Lock()
	fn := e.onModalClosed
	subs := append([]modalClosedSub(nil), e.modalClosedSubs...)
	e.hostMu.Unlock()
	if fn != nil {
		fn(m)
	}
	for _, s := range subs {
		s.fn(m)
	}
	// Событие самого диалога: приложению обычно нужен не «какая-то модалка
	// закрылась», а «этот диалог закрылся» — чтобы освободить его дерево.
	if d, ok := m.(*widget.Dialog); ok {
		if cb := d.OnClosed; cb != nil {
			cb()
		}
	}
}
