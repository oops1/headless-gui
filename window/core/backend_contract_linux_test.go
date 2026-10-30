//go:build linux && !android

package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/wayland"
)

// backend_contract_linux_test.go — бэкенды X11 и Wayland удовлетворяют
// интерфейсам-возможностям ядра.
//
// Ядро находит возможности бэкенда утверждением типа, и интерфейс с
// неэкспортированным методом тип из другого пакета молча перестаёт
// удовлетворять: ветка кода меняется без ошибки сборки (так на 9.2 выпал
// dragImageSupported). Список — снимок на шаге 9.3; бэкенд, потерявший
// возможность, здесь не соберётся. Новая возможность бэкенда — новая строка.

var (
	_ activationNotifier  = (*X11Window)(nil)
	_ alwaysOnTopBackend  = (*X11Window)(nil)
	_ appIDSetter         = (*X11Window)(nil)
	_ cornerRounder       = (*X11Window)(nil)
	_ cursorImageSetter   = (*X11Window)(nil)
	_ dirtyRectBlitter    = (*X11Window)(nil)
	_ dragTargetEvents    = (*X11Window)(nil)
	_ eventPumper         = (*X11Window)(nil)
	_ exposeNotifier      = (*X11Window)(nil)
	_ fileDragSource      = (*X11Window)(nil)
	_ filesDropTarget     = (*X11Window)(nil)
	_ foregrounder        = (*X11Window)(nil)
	_ fullscreenBackend   = (*X11Window)(nil)
	_ fullscreenOnBackend = (*X11Window)(nil)
	_ initialPlaceBackend = (*X11Window)(nil)
	_ interactiveMover    = (*X11Window)(nil)
	_ keyRepeatSource     = (*X11Window)(nil)
	_ keyboardGrabBackend = (*X11Window)(nil)
	_ keyboardGrabQuerier = (*X11Window)(nil)
	_ localeProvider      = (*X11Window)(nil)
	_ lockKeysBackend     = (*X11Window)(nil)
	_ lockKeysSetter      = (*X11Window)(nil)
	_ minimizedBackend    = (*X11Window)(nil)
	_ monitorChangeSource = (*X11Window)(nil)
	_ ownedWindow         = (*X11Window)(nil)
	_ popupCursorSetter   = (*X11Window)(nil)
	_ popupPixelAlpha     = (*X11Window)(nil)
	_ popupRegionSetter   = (*X11Window)(nil)
	_ popupWindow         = (*X11Window)(nil)
	_ rawKeySource        = (*X11Window)(nil)
	_ scaleDetector       = (*X11Window)(nil)
	_ touchSource         = (*X11Window)(nil)
	_ trayWindow          = (*X11Window)(nil)
	_ uiThreadInvoker     = (*X11Window)(nil)
	_ wheelRawSource      = (*X11Window)(nil)
)

// Стиль окна, владелец, Z-порядок и положение (style.go, шаг 9.7). У Wayland —
// только стиль: владельца нет (xdg_toplevel.set_parent принимает только окно
// того же соединения, а у каждого окна своё), Z-порядка и положения окна
// xdg-shell не даёт (TestBackendContract_WaylandStyleOnly).
var (
	_ native.OwnerSetter = (*X11Window)(nil)
	_ native.Positioner  = (*X11Window)(nil)
	_ native.Stacker     = (*X11Window)(nil)
	_ native.Styler      = (*X11Window)(nil)

	_ cornerRounder = (*wayland.WaylandWindow)(nil)
	_ native.Styler = (*wayland.WaylandWindow)(nil)
)

var (
	_ activationNotifier    = (*wayland.WaylandWindow)(nil)
	_ appIDSetter           = (*wayland.WaylandWindow)(nil)
	_ childPopupCursor      = (*wayland.WaylandWindow)(nil)
	_ childPopupCursorImage = (*wayland.WaylandWindow)(nil)
	_ childPopupHost        = (*wayland.WaylandWindow)(nil)
	_ clientSizer           = (*wayland.WaylandWindow)(nil)
	_ cursorImageSetter     = (*wayland.WaylandWindow)(nil)
	_ dirtyRectBlitter      = (*wayland.WaylandWindow)(nil)
	_ dpiChangeNotifier     = (*wayland.WaylandWindow)(nil)
	_ dragTargetEvents      = (*wayland.WaylandWindow)(nil)
	_ eventPumper           = (*wayland.WaylandWindow)(nil)
	_ fileDragImageSource   = (*wayland.WaylandWindow)(nil)
	_ fileDragSource        = (*wayland.WaylandWindow)(nil)
	_ filesDropTarget       = (*wayland.WaylandWindow)(nil)
	_ foregrounder          = (*wayland.WaylandWindow)(nil)
	_ fullscreenBackend     = (*wayland.WaylandWindow)(nil)
	_ fullscreenOnBackend   = (*wayland.WaylandWindow)(nil)
	_ imeBackend            = (*wayland.WaylandWindow)(nil)
	_ initialPlaceBackend   = (*wayland.WaylandWindow)(nil)
	_ interactiveMover      = (*wayland.WaylandWindow)(nil)
	_ keyRepeatSource       = (*wayland.WaylandWindow)(nil)
	_ keyboardGrabBackend   = (*wayland.WaylandWindow)(nil)
	_ keyboardGrabQuerier   = (*wayland.WaylandWindow)(nil)
	_ lockKeysBackend       = (*wayland.WaylandWindow)(nil)
	_ lockKeysSetter        = (*wayland.WaylandWindow)(nil)
	_ lockKeysSettler       = (*wayland.WaylandWindow)(nil)
	_ minimizedBackend      = (*wayland.WaylandWindow)(nil)
	_ monitorChangeSource   = (*wayland.WaylandWindow)(nil)
	_ monitorSource         = (*wayland.WaylandWindow)(nil)
	_ popupCursorSetter     = (*wayland.WaylandWindow)(nil)
	_ rawKeySource          = (*wayland.WaylandWindow)(nil)
	_ touchSource           = (*wayland.WaylandWindow)(nil)
	_ trayWindow            = (*wayland.WaylandWindow)(nil)
	_ wheelRawSource        = (*wayland.WaylandWindow)(nil)
)

// X11 значка перетаскивания не рисует: окно ядра на нём отвечает
// DragImageSupported() == false и перетаскивает без значка.
func TestBackendContract_X11NoDragImage(t *testing.T) {
	if _, ok := any((*X11Window)(nil)).(fileDragImageSource); ok {
		t.Error("X11Window объявил значок перетаскивания")
	}
	win := &Window{surface: surface{native: &X11Window{}, scale: 1}}
	if win.DragImageSupported() {
		t.Error("DragImageSupported true на X11")
	}
}

// Окно Wayland отвечает на владельца, Z-порядок и положение честным false:
// set_parent принимает только окно того же соединения, а у каждого окна своё.
func TestBackendContract_WaylandStyleOnly(t *testing.T) {
	var nw native.NativeWindow = &wayland.WaylandWindow{}
	if _, ok := nw.(native.OwnerSetter); ok {
		t.Error("WaylandWindow объявил владельца")
	}
	win := &Window{surface: surface{native: nw, scale: 1}}
	owner := &Window{surface: surface{native: &wayland.WaylandWindow{}, scale: 1}}
	if win.SetOwner(owner) || win.Raise() || win.Lower() || win.SetPosition(1, 1) {
		t.Error("SetOwner, Raise, Lower или SetPosition на Wayland = true")
	}
	if _, _, ok := win.Position(); ok {
		t.Error("Position на Wayland ok")
	}
	if win.SetStyle(native.Style{Tool: true}) || win.Style().Tool {
		t.Error("Tool на Wayland принят")
	}
}

// trayIconMethods и balloonMethods — методы трея, которые раньше объявляли
// бэкенды. Иконка в трее и уведомления — возможность platform/tray (9.8c):
// окно бэкенда их не объявляет, ядро их у окна и не ищет.
type (
	trayIconMethods interface {
		SetTrayIcon(image.Image, string) error
	}
	balloonMethods interface {
		ShowBalloon(string, string, uint32) error
	}
)

func TestBackendContract_LinuxNoTray(t *testing.T) {
	for name, nw := range map[string]any{"X11Window": &X11Window{}, "WaylandWindow": &wayland.WaylandWindow{}} {
		if _, ok := nw.(trayIconMethods); ok {
			t.Errorf("%s объявил SetTrayIcon", name)
		}
		if _, ok := nw.(balloonMethods); ok {
			t.Errorf("%s объявил ShowBalloon", name)
		}
	}
}
