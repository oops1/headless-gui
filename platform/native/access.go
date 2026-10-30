package native

// access.go — возможность доступности (ADR-0013 §3): мост окна к службе
// доступности ОС (UI Automation на Windows, AT-SPI на Linux) живёт в
// platform/a11y и регистрируется здесь; ядро окна отдаёт ему себя через
// AccessHost.

import "github.com/oops1/headless-gui/v4/widget/base"

// AccessHost — окно ядра глазами моста доступности. Методы зовутся с любой
// горутины: потоки UIA, обработчики D-Bus, цикл событий моста.
//
//facadegen:skip
type AccessHost interface {
	// AccessibilityTree — дерево семантики окна; движок строит его на своей
	// горутине, вызывающий ждёт. nil — движок дерева не отдаёт.
	AccessibilityTree() *base.AccessNode
	// FocusAccessible, ActivateAccessible и AccessAction — действия
	// скринридера, синхронно на горутине движка; false — движок их не умеет
	// или виджет их не принял.
	FocusAccessible(w base.Widget) bool
	ActivateAccessible(w base.Widget) bool
	AccessAction(w base.Widget, id string, kind base.AccessActionKind) bool
	// PostToEngine выполняет fn на горутине движка, не дожидаясь.
	PostToEngine(fn func())
	// Native — окно ОС (положение на экране; у Win32 — и дескриптор).
	Native() NativeWindow
	// Scale — масштаб окна: логические пиксели движка → физические.
	Scale() float64
	// Title — заголовок окна (имя корня без имени).
	Title() string
	// AccessibilityForced — решение приложения (SetAccessibilityEnabled):
	// set=false — не принималось, мост решает сам.
	AccessibilityForced() (on, set bool)
}

// AccessBridge — мост доступности одного окна.
//
//facadegen:skip
type AccessBridge interface {
	// Start поднимает мост после создания окна ОС; ошибка — моста нет, окно
	// работает как прежде.
	Start() error
	// Stop снимает мост после цикла событий окна.
	Stop()
}

// AccessFactory создаёт мост для окна (nil — не для этого окна).
//
//facadegen:skip
type AccessFactory func(h AccessHost) AccessBridge

var accessSlot = newCapSlot[AccessFactory]("a11y")

// RegisterAccessibility подключает доступность (init пакета platform/a11y).
//
//facadegen:skip
func RegisterAccessibility(f AccessFactory) (restore func()) { return accessSlot.register(f) }

// Accessibility — фабрика мостов доступности; ok=false — возможность не
// подключена (window/minimal).
//
//facadegen:skip
func Accessibility() (f AccessFactory, ok bool) { return accessSlot.get() }
