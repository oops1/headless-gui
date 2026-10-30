//go:build linux && !android

package core

import "testing"

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
	_ activationNotifier    = (*WaylandWindow)(nil)
	_ appIDSetter           = (*WaylandWindow)(nil)
	_ balloonHost           = (*WaylandWindow)(nil)
	_ childPopupCursor      = (*WaylandWindow)(nil)
	_ childPopupCursorImage = (*WaylandWindow)(nil)
	_ childPopupHost        = (*WaylandWindow)(nil)
	_ clientSizer           = (*WaylandWindow)(nil)
	_ cursorImageSetter     = (*WaylandWindow)(nil)
	_ dirtyRectBlitter      = (*WaylandWindow)(nil)
	_ dpiChangeNotifier     = (*WaylandWindow)(nil)
	_ dragTargetEvents      = (*WaylandWindow)(nil)
	_ eventPumper           = (*WaylandWindow)(nil)
	_ fileDragImageSource   = (*WaylandWindow)(nil)
	_ fileDragSource        = (*WaylandWindow)(nil)
	_ filesDropTarget       = (*WaylandWindow)(nil)
	_ foregrounder          = (*WaylandWindow)(nil)
	_ fullscreenBackend     = (*WaylandWindow)(nil)
	_ fullscreenOnBackend   = (*WaylandWindow)(nil)
	_ imeBackend            = (*WaylandWindow)(nil)
	_ initialPlaceBackend   = (*WaylandWindow)(nil)
	_ interactiveMover      = (*WaylandWindow)(nil)
	_ keyRepeatSource       = (*WaylandWindow)(nil)
	_ keyboardGrabBackend   = (*WaylandWindow)(nil)
	_ keyboardGrabQuerier   = (*WaylandWindow)(nil)
	_ lockKeysBackend       = (*WaylandWindow)(nil)
	_ lockKeysSetter        = (*WaylandWindow)(nil)
	_ lockKeysSettler       = (*WaylandWindow)(nil)
	_ minimizedBackend      = (*WaylandWindow)(nil)
	_ monitorChangeSource   = (*WaylandWindow)(nil)
	_ monitorSource         = (*WaylandWindow)(nil)
	_ nativeTrayMenu        = (*WaylandWindow)(nil)
	_ popupCursorSetter     = (*WaylandWindow)(nil)
	_ rawKeySource          = (*WaylandWindow)(nil)
	_ touchSource           = (*WaylandWindow)(nil)
	_ trayHost              = (*WaylandWindow)(nil)
	_ wheelRawSource        = (*WaylandWindow)(nil)
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
