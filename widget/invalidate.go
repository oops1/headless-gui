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
	"sync/atomic"
)

// Модель уведомлений — ШИРОКОВЕЩАТЕЛЬНЫЙ РЕЕСТР (а не «последний движок
// выигрывает»). Каждый живой движок регистрирует себя через RegisterUINotifier
// при создании и снимает регистрацию в Stop. notifyUIChanged/notifyRectChanged
// рассылают уведомление ВСЕМ зарегистрированным приёмникам.
//
// Зачем: при ДВУХ и более живых движках (главное окно + hosted-диалог/попап,
// а в перспективе — немодальные floating-панели) инвалидации виджетов первого
// движка не должны «утекать» во второй. Прежняя схема с единственным
// глобальным колбэком отдавала все уведомления последнему созданному движку;
// для модалок это маскировалось блокировкой родителя, для немодальных окон —
// прямой блокер.
//
// Стоимость широковещания: лишний Invalidate в «чужом» движке безопасен и
// дёшев — on-demand рендер сделает кадр, но diff по неизменившейся области
// вернёт ПУСТОЙ набор тайлов, и кадр не будет эмитирован. Приёмников на
// процесс единицы (движки), поэтому обход реестра — микроскопические накладные.
//
// Потокобезопасность: реестр под RWMutex. Рассылка идёт под RLock; КОНТРАКТ
// приёмника — его колбэк (engine.Invalidate/InvalidateRect) НЕ должен повторно
// входить в реестр (Register/Unregister) или звать notify* (иначе возможен
// самодедлок на RWMutex). Реальные приёмники этого не делают: они лишь
// выставляют атомарный damage движка.

// uiReceiver — один зарегистрированный приёмник (обычно движок). full —
// полная инвалидация; rect — точечная (nil → деградирует до full).
type uiReceiver struct {
	handle uint64
	full   func()
	rect   func(image.Rectangle)
}

var (
	uiNotifyMu sync.RWMutex
	// uiReceivers — реестр живых приёмников (широковещание).
	uiReceivers []uiReceiver
	uiHandleSeq uint64
	// Внешний «слот» обратной совместимости для SetUIChangeNotifier/
	// SetUIRectChangeNotifier. Живёт ПАРАЛЛЕЛЬНО реестру: тесты подменяют
	// нотификатор, не создавая движок (например tests/datagrid_invalidate_test).
	// Вызывается ПОСЛЕ реестра. Семантика «последний выигрывает».
	uiExtNotify     func()
	uiExtRectNotify func(image.Rectangle)
)

// RegisterUINotifier регистрирует приёмник уведомлений об изменениях UI и
// возвращает дескриптор для UnregisterUINotifier. Движок вызывает это при
// создании (engine.New). Несколько живых движков сосуществуют — уведомления
// рассылаются всем (широковещание). fnFull — полная инвалидация (аналог
// прежнего SetUIChangeNotifier); fnRect — точечная (аналог прежнего
// SetUIRectChangeNotifier, может быть nil).
func RegisterUINotifier(fnFull func(), fnRect func(image.Rectangle)) uint64 {
	uiNotifyMu.Lock()
	uiHandleSeq++
	h := uiHandleSeq
	uiReceivers = append(uiReceivers, uiReceiver{handle: h, full: fnFull, rect: fnRect})
	uiNotifyMu.Unlock()
	return h
}

// UnregisterUINotifier снимает регистрацию приёмника по дескриптору.
// Идемпотентно: повторный вызов или неизвестный/нулевой дескриптор — no-op.
// Движок вызывает это в Stop; teardown hosted-диалога/попапа гарантированно
// зовёт eng.Stop, поэтому короткоживущие движки не текут в реестре.
func UnregisterUINotifier(handle uint64) {
	if handle == 0 {
		return
	}
	uiNotifyMu.Lock()
	for i := range uiReceivers {
		if uiReceivers[i].handle == handle {
			uiReceivers = append(uiReceivers[:i], uiReceivers[i+1:]...)
			break
		}
	}
	uiNotifyMu.Unlock()
}

// UINotifierCount возвращает число зарегистрированных приёмников. Для тестов:
// проверка отсутствия утечки реестра после Stop.
func UINotifierCount() int {
	uiNotifyMu.RLock()
	n := len(uiReceivers)
	uiNotifyMu.RUnlock()
	return n
}

// SetUIChangeNotifier задаёт ВНЕШНИЙ колбэк полной инвалидации (обратная
// совместимость). В отличие от реестра приёмников, это одиночный слот
// («последний выигрывает»), живущий ПОВЕРХ реестра и вызываемый после него.
// Движок больше сюда не регистрируется (он использует RegisterUINotifier) —
// слот предназначен для тестов/встраивания. nil снимает внешний колбэк.
func SetUIChangeNotifier(fn func()) {
	uiNotifyMu.Lock()
	uiExtNotify = fn
	uiNotifyMu.Unlock()
}

// SetUIRectChangeNotifier задаёт ВНЕШНИЙ колбэк точечной инвалидации (обратная
// совместимость, одиночный слот поверх реестра, вызывается после него). nil
// снимает внешний колбэк — тогда внешние точечные уведомления деградируют до
// внешнего SetUIChangeNotifier (если задан). На реестр приёмников не влияет.
func SetUIRectChangeNotifier(fn func(image.Rectangle)) {
	uiNotifyMu.Lock()
	uiExtRectNotify = fn
	uiNotifyMu.Unlock()
}

// ─── Ревизия метрик текста ──────────────────────────────────────────────────

// textMetricsRev растёт при каждом изменении, влияющем на ШИРИНУ текста:
// смена DPI шрифтов или HiDPI-масштаба. Кэши, зависящие от измерения строк
// (например, кэш переноса в Label), сверяются с ревизией и пересчитываются.
var textMetricsRev atomic.Uint64

// TextMetricsRev — текущая ревизия метрик текста.
func TextMetricsRev() uint64 { return textMetricsRev.Load() }

// BumpTextMetricsRev поднимает ревизию метрик текста. Вызывается движком при
// смене DPI/масштаба; приложениям обычно не нужна.
func BumpTextMetricsRev() { textMetricsRev.Add(1) }

// notifyUIChanged сообщает всем приёмникам, что содержимое UI могло измениться.
// Дёшево и потокобезопасно; no-op, если приёмников нет.
func notifyUIChanged() {
	uiNotifyMu.RLock()
	defer uiNotifyMu.RUnlock()
	for i := range uiReceivers {
		if uiReceivers[i].full != nil {
			uiReceivers[i].full()
		}
	}
	if uiExtNotify != nil {
		uiExtNotify()
	}
}

// InvalidateRect сообщает движку, что изменилась область r (в логических
// координатах холста), даже если она лежит вне границ какого-либо виджета.
//
// Base.Invalidate заявляет границы САМОГО виджета — и этого достаточно, пока
// виджет рисует внутри себя. Оверлею этого мало: меню «Пуск» рисуется далеко
// за пределами кнопки, которой оно принадлежит, и, заявив границы кнопки,
// оверлей окажется обрезан клипом частичной перерисовки. Такому виджету
// нужно заявить область своего оверлея — этой функцией.
//
// Пустой прямоугольник игнорируется.
func InvalidateRect(r image.Rectangle) { notifyRectChanged(r) }

// notifyRectChanged сообщает всем приёмникам об изменении конкретной области UI.
// Приёмник без rect-колбэка получает полную инвалидацию (совместимость).
// Пустой прямоугольник игнорируется.
func notifyRectChanged(r image.Rectangle) {
	// Границы виджета могли поменяться — прямоугольники поддеревьев,
	// посчитанные до этого, больше не годятся.
	bumpTreeGen()
	if r.Empty() {
		return
	}
	uiNotifyMu.RLock()
	defer uiNotifyMu.RUnlock()
	for i := range uiReceivers {
		if uiReceivers[i].rect != nil {
			uiReceivers[i].rect(r)
		} else if uiReceivers[i].full != nil {
			uiReceivers[i].full()
		}
	}
	if uiExtRectNotify != nil {
		uiExtRectNotify(r)
	} else if uiExtNotify != nil {
		uiExtNotify()
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
