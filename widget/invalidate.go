// invalidate.go — мост «виджеты → движок» для рендера по запросу (on-demand).
//
// В режиме Engine.SetRenderOnDemand(true) движок пропускает кадры, пока UI не
// изменился. Изменения, инициированные самим движком (события мыши/клавиатуры,
// модалки, смена root), он отслеживает сам; изменения из слоя данных
// (BindingScope.Refresh, перестроение ItemsControl, смена языка/локали)
// сообщаются сюда через notifyUIChanged.
//
// Приложения, мутирующие виджеты напрямую (label.SetText из своей горутины),
// в on-demand режиме должны вызывать Engine.Invalidate() сами. В режиме по
// умолчанию (рендер каждый кадр) ничего этого не требуется.
package widget

import "sync"

var (
	uiNotifyMu sync.RWMutex
	uiNotify   func()
)

// SetUIChangeNotifier регистрирует колбэк, вызываемый при изменениях UI из
// слоя данных (биндинги, локализация, live-коллекции). Движок ставит сюда
// свой Invalidate. Последняя регистрация выигрывает (на процесс — один
// активный движок). nil снимает уведомления.
func SetUIChangeNotifier(fn func()) {
	uiNotifyMu.Lock()
	uiNotify = fn
	uiNotifyMu.Unlock()
}

// notifyUIChanged сообщает движку, что содержимое UI могло измениться.
// Дёшево и потокобезопасно; no-op, если notifier не зарегистрирован.
func notifyUIChanged() {
	uiNotifyMu.RLock()
	fn := uiNotify
	uiNotifyMu.RUnlock()
	if fn != nil {
		fn()
	}
}

// Animated реализуется виджетами, которым нужна непрерывная перерисовка
// (мигающая каретка и т.п.), пока они в соответствующем состоянии.
// В on-demand режиме движок не пропускает кадры, пока виджет с фокусом
// возвращает NeedsAnimation() == true.
type Animated interface {
	NeedsAnimation() bool
}
