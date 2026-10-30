// Package overlay — слои поверх дерева виджетов, которые живут вне кадра
// холста или поверх него (ADR-0011, задача 7.4): хостовые попапы (меню и
// списки в собственных окнах ОС) с ареной холста-заготовки, кэшем картинок и
// переходами появления и исчезновения; транслирующий контекст рисования
// оверлея в отдельный буфер; хост нативных модальных окон и подписчики их
// закрытия; всплывающая подсказка.
//
// Пакет не знает о движке: всё, что ему нужно от сцены и цикла (верхнее
// модальное окно, повреждение, поколение кадра, менеджер тем, включены ли
// переходы оверлеев и в какую сторону сдвигать), он берёт через Host; корень
// и модальные окна кадра приходят параметрами RenderPopups и DrawTooltip.
// Движок (engine.Engine) создаёт Manager в конструкторе и отдаёт ему свои
// публичные методы оверлеев (SetPopupSink, SetPopupFade, SetModalHost,
// AddOnModalClosed, SetTooltipsEnabled и другие); типы PopupFrame,
// PopupTransition и ModalHost движок держит алиасами — открытый API не
// меняется. Модальные окна в холсте (ShowModal, CloseModal) и переходы
// оверлеев в холсте остались в движке.
//
// Замки. Все замки пакета листовые (ADR-0011): popupMu, hostMu, ttMu. Под
// ними не берут другой замок; Host под ними зовут только методы без замков
// (поколение кадра, менеджер тем) — проверка в lockorder_test.go. Рендер
// попапов (RenderPopups) идёт на горутине кадра под frameMu движка.
package overlay

import (
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Host — что оверлеи берут у движка.
type Host interface {
	// TopModal — верхнее модальное окно в холсте (nil — нет).
	TopModal() widget.ModalWidget
	// DamageRect — область (логическая) изменилась, дерево под ней — нет
	// (стирание подсказки).
	DamageRect(r image.Rectangle)
	// BumpGen просит кадр без области (шаг перехода попапа). Без замков.
	BumpGen()
	// ThemeManager — менеджер тем движка (токены переходов). Без замков.
	ThemeManager() *theme.Manager
	// TransitionsOn — переходы оверлеев включены (SetOverlayTransitions). Без
	// замков.
	TransitionsOn() bool
	// SlideDir — сдвиг оверлея w в начале появления (r — его экранные
	// границы, d — величина): выше точки привязки — снизу, иначе сверху.
	SlideDir(w widget.Widget, r image.Rectangle, d int) image.Point
}

// Manager — оверлеи одного движка.
type Manager struct {
	host Host

	// modalHost — опциональный хост нативных модалок (window.dialogHost на
	// Win32). Если установлен и принимает модалку, она живёт в собственном
	// нативном окне ОС, а не в холсте движка. onModalClosed — колбэк,
	// вызываемый в конце CloseModal (см. modalhost.go). hostMu защищает оба.
	modalHost     ModalHost
	onModalClosed func(widget.ModalWidget)
	// modalClosedSubs — подписчики AddOnModalClosed. Слот onModalClosed занят
	// хостом нативных окон, и приложению его не хватало: второй подписчик
	// молча вытеснял первого (GG-82).
	modalClosedSubs []modalClosedSub
	modalSubNextID  int
	hostMu          sync.Mutex

	// popupSink — опциональный хост popup-оверлеев (window.popupHost). Если
	// задан, открытые оверлеи (dropdown/меню) выносятся в собственные нативные
	// окна ОС, а не рисуются в холст. lastPopupSig — сигнатура последнего
	// набора попапов, отданного sink (для вызова sink только при изменениях).
	popupSink    func([]PopupFrame)
	lastPopupSig []popupSig
	popupCache   map[uintptr]*popupEntry // кэш картинки на оверлей
	// popupArena — общий холст-заготовка хостовых попапов (ADR-0009); только
	// горутина кадра, под frameMu движка.
	popupArena Arena
	popupGen   uint64 // поколение для отсева исчезнувших попапов
	popupMu    sync.Mutex
	// popupBytes — память хостовых попапов (Stats.PopupBytes).
	popupBytes atomic.Uint64

	// Рабочие буферы RenderPopups (только горутина кадра, под frameMu).
	popupItems []PopupItem
	popupSigBu []popupSig
	// Переходы хостовых попапов (popuptrans.go): popupFade — хост их
	// понимает (SetPopupFade); popupTicked — тик просит кадр под шаг;
	// popupClosingBu — исчезающие попапы кадра (переиспользуемый буфер).
	popupFade      atomic.Bool
	popupTicked    atomic.Bool
	popupClosingBu []uintptr

	// ── Tooltip ─────────────────────────────────────────────────────────────
	ttMu       sync.Mutex
	ttEnabled  bool          // показывать всплывающие подсказки
	ttDelay    time.Duration // задержка появления после остановки курсора
	ttMouseX   int
	ttMouseY   int
	ttLastMove time.Time
	ttHasMouse bool            // курсор хотя бы раз входил на холст
	ttShownAt  image.Rectangle // область показанной подсказки (пустая — не показана)
}

// New создаёт оверлеи движка host: подсказки включены, задержка 600 мс.
func New(host Host) *Manager {
	return &Manager{host: host, ttEnabled: true, ttDelay: 600 * time.Millisecond}
}

// PopupBytes — память хостовых попапов: заготовка арены и картинки
// открытых и исчезающих попапов (Stats.PopupBytes).
func (m *Manager) PopupBytes() uint64 { return m.popupBytes.Load() }

// ResetPopupBytes обнуляет учёт памяти попапов: хост снят, заготовка
// отпущена, кадров попапов больше нет.
func (m *Manager) ResetPopupBytes() { m.popupBytes.Store(0) }

// TakePopupTicked снимает просьбу кадра под шаг перехода попапа: true — шаг
// был, кадр доходит до RenderPopups и без повреждения.
func (m *Manager) TakePopupTicked() bool { return m.popupTicked.Swap(false) }

// ReleaseArena отпускает холст-заготовку (хост снят, формат кадра сменился).
// Горутина кадра, под frameMu движка.
func (m *Manager) ReleaseArena() { m.popupArena.Release() }

// Arena — арена попапов (для диагностики и тестов; трогать — только под
// frameMu движка).
func (m *Manager) Arena() *Arena { return &m.popupArena }

// CachedPopups — сколько оверлеев в кэше картинок (для тестов).
func (m *Manager) CachedPopups() int {
	m.popupMu.Lock()
	defer m.popupMu.Unlock()
	return len(m.popupCache)
}

// CachedImage — последняя картинка оверлея id в кэше (nil — нет; для тестов).
func (m *Manager) CachedImage(id uintptr) *image.RGBA {
	m.popupMu.Lock()
	defer m.popupMu.Unlock()
	if ent := m.popupCache[id]; ent != nil {
		return ent.img
	}
	return nil
}

// TooltipShownAt — область показанной подсказки (пустая — не показана).
func (m *Manager) TooltipShownAt() image.Rectangle {
	m.ttMu.Lock()
	defer m.ttMu.Unlock()
	return m.ttShownAt
}

// maxTreeDepth — предел глубины обходов дерева виджетов: цикл в Children()
// иначе вешает обход намертво (как у движка).
const maxTreeDepth = 512

var deepTreeOnce sync.Once

// tooDeep сообщает о превышении предела глубины; логирует один раз.
func tooDeep(depth int) bool {
	if depth < maxTreeDepth {
		return false
	}
	deepTreeOnce.Do(func() {
		log.Printf("overlay: обход дерева виджетов прерван на глубине %d — вероятен цикл в Children()", maxTreeDepth)
	})
	return true
}
