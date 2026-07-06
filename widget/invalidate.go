// invalidate.go — мост «виджеты → движок» для рендера по запросу (on-demand).
//
// Рендер по запросу — режим по умолчанию: движок пропускает кадры, пока UI
// не изменился, и перерисовывает только damage-область. Виджеты сообщают об
// изменениях сами: Base.Invalidate()/Base.SetBounds шлют свой прямоугольник
// через notifyRectChanged (точечно), слой данных (BindingScope.Refresh,
// перестроение ItemsControl, смена языка/локали) — через notifyUIChanged
// (полная инвалидация).
//
// Сеттеры виджетов (SetText, SetValue, SetHovered, ...) уже инвалидируют
// автоматически. Прямые записи в экспортированные поля (btn.Text = "...")
// движку не видны — после них вызывайте w.Invalidate().
package widget

import (
	"image"
	"sync"
)

var (
	uiNotifyMu   sync.RWMutex
	uiNotify     func()
	uiRectNotify func(image.Rectangle)
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

// SetUIRectChangeNotifier регистрирует колбэк точечной инвалидации: виджеты
// сообщают прямоугольник изменившейся области (авто-damage). Движок ставит
// сюда свой InvalidateRect. nil снимает уведомления — тогда точечные
// уведомления деградируют до полной инвалидации через SetUIChangeNotifier.
func SetUIRectChangeNotifier(fn func(image.Rectangle)) {
	uiNotifyMu.Lock()
	uiRectNotify = fn
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

// notifyRectChanged сообщает движку об изменении конкретной области UI.
// Если rect-notifier не зарегистрирован — полная инвалидация (совместимость).
// Пустой прямоугольник игнорируется.
func notifyRectChanged(r image.Rectangle) {
	if r.Empty() {
		return
	}
	uiNotifyMu.RLock()
	fn := uiRectNotify
	full := uiNotify
	uiNotifyMu.RUnlock()
	if fn != nil {
		fn(r)
		return
	}
	if full != nil {
		full()
	}
}

// b2i переводит bool в int32 для atomic-полей состояния (0 | 1).
func b2i(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// Animated реализуется виджетами, которым нужна непрерывная перерисовка
// (мигающая каретка и т.п.), пока они в соответствующем состоянии.
// В on-demand режиме движок не пропускает кадры, пока виджет с фокусом
// возвращает NeedsAnimation() == true.
type Animated interface {
	NeedsAnimation() bool
}
