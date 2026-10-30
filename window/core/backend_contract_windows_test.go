//go:build windows

package core

import "github.com/oops1/headless-gui/v3/platform/native"

// backend_contract_windows_test.go — бэкенд Win32 удовлетворяет
// интерфейсам-возможностям ядра.
//
// Ядро находит возможности бэкенда утверждением типа, и интерфейс с
// неэкспортированным методом тип из другого пакета молча перестаёт
// удовлетворять: ветка кода меняется без ошибки сборки (так на 9.2 выпал
// dragImageSupported). Список — снимок на шаге 9.3; бэкенд, потерявший
// возможность, здесь не соберётся. Новая возможность бэкенда — новая строка.

var (
	_ activationNotifier  = (*Win32Window)(nil)
	_ alwaysOnTopBackend  = (*Win32Window)(nil)
	_ balloonHost         = (*Win32Window)(nil)
	_ clientSizer         = (*Win32Window)(nil)
	_ cornerRounder       = (*Win32Window)(nil)
	_ cursorImageSetter   = (*Win32Window)(nil)
	_ dirtyRectBlitter    = (*Win32Window)(nil)
	_ dpiChangeNotifier   = (*Win32Window)(nil)
	_ dragTargetEvents    = (*Win32Window)(nil)
	_ exposeNotifier      = (*Win32Window)(nil)
	_ fileDragImageSource = (*Win32Window)(nil)
	_ fileDragSource      = (*Win32Window)(nil)
	_ filesDropTarget     = (*Win32Window)(nil)
	_ foregrounder        = (*Win32Window)(nil)
	_ fullscreenBackend   = (*Win32Window)(nil)
	_ fullscreenOnBackend = (*Win32Window)(nil)
	_ hitTester           = (*Win32Window)(nil)
	_ imeBackend          = (*Win32Window)(nil)
	_ initialPlaceBackend = (*Win32Window)(nil)
	_ keyRepeatSource     = (*Win32Window)(nil)
	_ keyboardGrabBackend = (*Win32Window)(nil)
	_ keyboardGrabQuerier = (*Win32Window)(nil)
	_ localeProvider      = (*Win32Window)(nil)
	_ lockKeysBackend     = (*Win32Window)(nil)
	_ lockKeysSetter      = (*Win32Window)(nil)
	_ minimizedBackend    = (*Win32Window)(nil)
	_ monitorChangeSource = (*Win32Window)(nil)
	_ monitorSource       = (*Win32Window)(nil)
	_ ownedWindow         = (*Win32Window)(nil)
	_ popupAlphaSetter    = (*Win32Window)(nil)
	_ popupCursorSetter   = (*Win32Window)(nil)
	_ popupRegionSetter   = (*Win32Window)(nil)
	_ popupWindow         = (*Win32Window)(nil)
	_ rawKeySource        = (*Win32Window)(nil)
	_ scaleDetector       = (*Win32Window)(nil)
	_ secondaryMarker     = (*Win32Window)(nil)
	_ touchSource         = (*Win32Window)(nil)
	_ trayHost            = (*Win32Window)(nil)
	_ uiThreadInvoker     = (*Win32Window)(nil)
	_ wheelRawSource      = (*Win32Window)(nil)
	_ workAreaProvider    = (*Win32Window)(nil)
)

// Стиль окна, владелец, Z-порядок и положение (style.go, шаг 9.7).
var (
	_ native.OwnerSetter = (*Win32Window)(nil)
	_ native.Positioner  = (*Win32Window)(nil)
	_ native.Stacker     = (*Win32Window)(nil)
	_ native.Styler      = (*Win32Window)(nil)
)
