package overlay

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
func (m *Manager) SetModalHost(h ModalHost) {
	m.hostMu.Lock()
	m.modalHost = h
	m.hostMu.Unlock()
}

// NativeModalHost — хост нативных модальных окон (nil — модальные окна в
// холсте).
func (m *Manager) NativeModalHost() ModalHost {
	m.hostMu.Lock()
	defer m.hostMu.Unlock()
	return m.modalHost
}

// SetOnModalClosed регистрирует колбэк, вызываемый в конце CloseModal после
// фактического закрытия модалки ЭТИМ движком (т.е. когда она была в стеке
// движка, а не у хоста). Используется хостом на вторичном движке: диалог,
// закрытый своим closer'ом (✕/Escape) → secondEng.CloseModal → колбэк →
// хост уничтожает нативное окно.
func (m *Manager) SetOnModalClosed(fn func(m widget.ModalWidget)) {
	m.hostMu.Lock()
	m.onModalClosed = fn
	m.hostMu.Unlock()
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
func (m *Manager) AddOnModalClosed(fn func(m widget.ModalWidget)) int {
	if fn == nil {
		return 0
	}
	m.hostMu.Lock()
	defer m.hostMu.Unlock()
	m.modalSubNextID++
	id := m.modalSubNextID
	m.modalClosedSubs = append(m.modalClosedSubs, modalClosedSub{id: id, fn: fn})
	return id
}

// RemoveOnModalClosed снимает подписку по дескриптору (no-op, если нет).
func (m *Manager) RemoveOnModalClosed(id int) {
	if id == 0 {
		return
	}
	m.hostMu.Lock()
	defer m.hostMu.Unlock()
	for i, s := range m.modalClosedSubs {
		if s.id == id {
			m.modalClosedSubs = append(m.modalClosedSubs[:i], m.modalClosedSubs[i+1:]...)
			return
		}
	}
}

// NotifyModalClosed сообщает подписчикам движка, что модалка закрыта.
//
// Зовётся хостом нативных окон: диалог в собственном окне ОС закрывается мимо
// CloseModal этого движка — его закрывает вторичный движок окна, — и без этого
// приложение не узнавало о закрытии крестиком или Alt+F4 (GG-82).
func (m *Manager) NotifyModalClosed(mw widget.ModalWidget) {
	m.FireModalClosed(mw)
}

// FireModalClosed сообщает о закрытии модального окна mw: слоту
// SetOnModalClosed, затем подписчикам AddOnModalClosed (замок снят до
// колбэков), затем самому окну. Зовёт движок в конце CloseModal.
func (m *Manager) FireModalClosed(mw widget.ModalWidget) {
	m.hostMu.Lock()
	fn := m.onModalClosed
	subs := append([]modalClosedSub(nil), m.modalClosedSubs...)
	m.hostMu.Unlock()
	if fn != nil {
		fn(mw)
	}
	for _, s := range subs {
		s.fn(mw)
	}
	// Событие самого диалога: приложению обычно нужен не «какая-то модалка
	// закрылась», а «этот диалог закрылся» — чтобы освободить его дерево.
	if d, ok := mw.(*widget.Dialog); ok {
		if cb := d.OnClosed; cb != nil {
			cb()
		}
	}
}
