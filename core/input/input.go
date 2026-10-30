// Package input — ввод движка: доставка мыши, клавиатуры, колеса, касаний,
// IME и сброса файлов в дерево виджетов, фокус, захват мыши, владелец
// указателя, принудительный курсор и перевод координат «экран ↔ кадр
// содержимого прокрутки» (ADR-0011, задача 7.2).
//
// Пакет не знает о движке: всё, что ему нужно от сцены и цикла (корень
// дерева, модальные окна, масштаб, инвалидация, очередь Post, граница сбоев,
// подсказка), он берёт через Host. Движок (engine.Engine) создаёт Dispatcher
// в конструкторе и отдаёт ему свои публичные методы ввода (SendMouseMove,
// SendKeyEvent, SetFocus и прочие) — открытый API движка не меняется.
//
// Замки. Все замки пакета листовые (ADR-0011): focusManager.mu, capMu,
// focusLoan.mu, touchState.mu, cursorOverride.mu. Под ними не берут другой
// замок, не зовут Host и не зовут код виджетов — проверка в
// lockorder_test.go. Движок может звать Dispatcher под своим frameMu, но не
// под листовыми замками.
//
// Горутины. Методы доставки событий зовёт поток-источник ввода (под
// window.Run — горутина кадра, через Post). Поля прежней позиции курсора,
// серии нажатий и владельца указателя принадлежат ему и замка не имеют.
package input

import (
	"image"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v4/widget"
)

// Host — что ввод берёт у движка. Методы зовутся без удержанных замков
// пакета input.
type Host interface {
	// Root — корень дерева виджетов (nil — дерева нет).
	Root() widget.Widget
	// TopModal — верхнее модальное окно в холсте (nil — нет).
	TopModal() widget.ModalWidget
	// Modals — открытые модальные окна в холсте снизу вверх (копия).
	Modals() []widget.ModalWidget
	// RequestCloseModal — закрыть модальное окно так, как его закрывает Esc
	// или щелчок мимо: с проверкой CanClose и OnCancel.
	RequestCloseModal(m widget.ModalWidget)
	// Scale — HiDPI-масштаб: события приходят в физических пикселях.
	Scale() float64
	// Invalidate — полный кадр; InvalidateRect — область в логических
	// координатах экрана.
	Invalidate()
	InvalidateRect(r image.Rectangle)
	// Post ставит fn в очередь горутины кадра.
	Post(fn func())
	// InLoop — можно выполнять сразу: вызов идёт на горутине кадра или цикл
	// не запущен (очередь разбирает вызывающий).
	InLoop() bool
	// GuardInput — граница сбоев доставки события where: отложенный вызов
	// результата ловит панику обработчика (политика сбоев движка) и после
	// неё зовёт Dispatcher.AbortGestures.
	GuardInput(where string) func()
	// RecordMouse — курсор в (x, y) логических: отсчёт задержки подсказки.
	RecordMouse(x, y int)
	// InvalidateShownTooltip — стереть показанную подсказку.
	InvalidateShownTooltip()
	// CaptureManager — менеджер захвата, который движок раздаёт виджетам
	// (тот же, что при SetRoot): запасная инъекция при нажатии отдаёт его
	// же, а не Dispatcher. Виджеты достают из менеджера и другие
	// возможности движка (DiffView — Post). nil — Dispatcher.
	CaptureManager() widget.CaptureManager
}

// Dispatcher — состояние ввода одного движка и доставка событий.
type Dispatcher struct {
	host Host

	focus focusManager // текущий виджет с фокусом; свой замок
	loan  focusLoan    // фокус, взятый виджетом по просьбе (focusreq.go); свой замок

	captured widget.Widget // виджет, захвативший мышь (drag)
	capMu    sync.Mutex
	// capChain — контейнеры со сдвигом содержимого над захватившим мышь
	// виджетом (см. captureOffset); capChainOK — цепочка уже найдена. Под capMu.
	capChain   []widget.ContentOffsetter
	capChainOK bool

	// pressConsumer — виджет, поглотивший последний press ЛКМ.
	// При release: если этот виджет больше не под курсором
	// (удалён/закрыт), событие проглатывается, чтобы не пролетело
	// на виджет, оказавшийся под курсором после закрытия.
	pressConsumer widget.Widget

	// touch — владельцы активных касаний по ID (см. touch.go); свой замок.
	touch touchState

	// mouseMod — модификаторы клавиатуры, которыми помечаются события мыши
	// (см. modifiers.go). Атомарные: пишет поток окна, читает диспетчеризация.
	mouseMod atomic.Int32

	// Прежняя позиция курсора для адресного broadcastMouseMove. Доступ только
	// из потока-источника ввода (SendMouseMove), синхронизация не нужна.
	lastMoveX, lastMoveY int
	hasLastMove          bool
	// pointerOwner — верхний под указателем widget.PointerOwner, которому
	// последним отдано движение (pointerowner.go); ownerPath — рабочий буфер
	// его поиска. Только поток-источник ввода, как lastMoveX.
	pointerOwner widget.PointerOwner
	ownerPath    []HitStep

	// Серия нажатий одной кнопкой — для MouseEvent.Clicks. Доступ оттуда же,
	// из потока-источника ввода (SendMouseButton).
	clickBtn      widget.MouseButton
	clickAt       time.Time
	clickX        int
	clickY        int
	clickN        int
	dblClickDelay time.Duration // 0 — значение по умолчанию (defaultDoubleClick)

	// cursorOvr — принудительный курсор поверх курсора виджетов (cursoroverride.go); свой замок.
	cursorOvr cursorOverride
}

// New создаёт ввод движка host.
func New(host Host) *Dispatcher {
	return &Dispatcher{host: host}
}

// Focused — виджет с фокусом (nil — нет).
func (d *Dispatcher) Focused() widget.Widget { return d.focus.get() }

// Captured — виджет, захвативший мышь (nil — нет).
func (d *Dispatcher) Captured() widget.Widget { return d.getCaptured() }

// FrameOffsetOf — смещение кадра виджета w (см. frames.go): w ищется в дереве
// и в модальных окнах; не найден — нулевое.
func (d *Dispatcher) FrameOffsetOf(w widget.Widget) image.Point { return d.frameOffsetOf(w) }

// ScreenBoundsOf — границы w на экране (логические): Bounds минус кадр.
func (d *Dispatcher) ScreenBoundsOf(w widget.Widget) image.Rectangle { return d.screenBoundsOf(w) }

// AbortGestures снимает захват мыши, забывает потребителя нажатия и
// отменяет касания: после паники в обработчике события иначе мышь осталась
// бы захваченной навсегда. Фокус не трогается: он принадлежит программе.
// Зовётся границей сбоев движка (Host.GuardInput).
func (d *Dispatcher) AbortGestures() {
	d.ReleaseCapture()
	d.pressConsumer = nil
	d.CancelTouches()
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
		log.Printf("input: обход дерева виджетов прерван на глубине %d — вероятен цикл в Children()", maxTreeDepth)
	})
	return true
}

// widgetInTree — лежит ли target в поддереве node.
func widgetInTree(node, target widget.Widget) bool {
	return widgetInTreeAt(node, target, 0)
}

func widgetInTreeAt(node, target widget.Widget, depth int) bool {
	if node == nil || tooDeep(depth) {
		return false
	}
	if node == target {
		return true
	}
	for _, child := range node.Children() {
		if widgetInTreeAt(child, target, depth+1) {
			return true
		}
	}
	return false
}
