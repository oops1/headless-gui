//go:build linux && !android

package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/wayland"
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
	_ balloonHost         = (*X11Window)(nil)
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
	_ nativeTrayMenu      = (*X11Window)(nil)
	_ ownedWindow         = (*X11Window)(nil)
	_ popupCursorSetter   = (*X11Window)(nil)
	_ popupPixelAlpha     = (*X11Window)(nil)
	_ popupRegionSetter   = (*X11Window)(nil)
	_ popupWindow         = (*X11Window)(nil)
	_ rawKeySource        = (*X11Window)(nil)
	_ scaleDetector       = (*X11Window)(nil)
	_ touchSource         = (*X11Window)(nil)
	_ trayHost            = (*X11Window)(nil)
	_ uiThreadInvoker     = (*X11Window)(nil)
	_ wheelRawSource      = (*X11Window)(nil)
)

var (
	_ activationNotifier    = (*wayland.WaylandWindow)(nil)
	_ appIDSetter           = (*wayland.WaylandWindow)(nil)
	_ balloonHost           = (*wayland.WaylandWindow)(nil)
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
	_ nativeTrayMenu        = (*wayland.WaylandWindow)(nil)
	_ popupCursorSetter     = (*wayland.WaylandWindow)(nil)
	_ rawKeySource          = (*wayland.WaylandWindow)(nil)
	_ touchSource           = (*wayland.WaylandWindow)(nil)
	_ trayHost              = (*wayland.WaylandWindow)(nil)
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
