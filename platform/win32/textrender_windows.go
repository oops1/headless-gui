package win32

// textrender_windows.go — настройка сглаживания текста Windows (ClearType) и
// уведомления о её смене и о подключении удалённого сеанса.

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/platform/native"
)

const (
	spiGetFontSmoothing            = 0x004A // SPI_GETFONTSMOOTHING
	spiSetFontSmoothing            = 0x004B // SPI_SETFONTSMOOTHING
	spiGetFontSmoothingType        = 0x200A // SPI_GETFONTSMOOTHINGTYPE
	spiSetFontSmoothingType        = 0x200B // SPI_SETFONTSMOOTHINGTYPE
	spiSetFontSmoothingContrast    = 0x200D // SPI_SETFONTSMOOTHINGCONTRAST
	spiGetFontSmoothingOrientation = 0x2012 // SPI_GETFONTSMOOTHINGORIENTATION
	spiSetFontSmoothingOrientation = 0x2013 // SPI_SETFONTSMOOTHINGORIENTATION
	feFontSmoothingClearType       = 2      // FE_FONTSMOOTHINGCLEARTYPE
	feFontSmoothingOrientationBGR  = 0      // FE_FONTSMOOTHINGORIENTATIONBGR
	smRemoteSession                = 0x1000 // SM_REMOTESESSION
	wmWtsSessionChange             = 0x02B1 // WM_WTSSESSION_CHANGE
	notifyForThisSession           = 0      // NOTIFY_FOR_THIS_SESSION
)

var (
	wtsapi32                             = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
)

// readTextRendering читает настройку сглаживания текста. Переменная — тесты
// подставляют свою.
var readTextRendering = func() native.TextRendering {
	spi := func(action uintptr) (uint32, bool) {
		var v uint32
		r, _, _ := procSystemParametersInfoW.Call(action, 0, uintptr(unsafe.Pointer(&v)), 0)
		return v, r != 0
	}
	on, ok := spi(spiGetFontSmoothing)
	if !ok {
		return native.TextRendering{}
	}
	tr := native.TextRendering{Known: true, Remote: getSystemMetrics(smRemoteSession) != 0}
	if on == 0 {
		return tr
	}
	if typ, ok := spi(spiGetFontSmoothingType); ok {
		tr.Subpixel = typ == feFontSmoothingClearType
	}
	if o, ok := spi(spiGetFontSmoothingOrientation); ok {
		tr.BGR = o == feFontSmoothingOrientationBGR
	}
	return tr
}

// textSmoothingSetting — WM_SETTINGCHANGE про сглаживание текста: его шлёт
// «Настройка текста ClearType» и параметры быстродействия системы.
func textSmoothingSetting(wparam uintptr) bool {
	switch wparam {
	case spiSetFontSmoothing, spiSetFontSmoothingType, spiSetFontSmoothingContrast, spiSetFontSmoothingOrientation:
		return true
	}
	return false
}

// TextRendering — настройка сглаживания текста Windows сейчас: ClearType или
// серое, порядок подпикселей, удалённый ли сеанс.
func (w *Win32Window) TextRendering() native.TextRendering { return readTextRendering() }

// SetOnTextRenderingChanged подписывает на смену сглаживания текста
// (WM_SETTINGCHANGE) и на подключение и отключение удалённого сеанса
// (WM_WTSSESSION_CHANGE: человек зашёл по RDP в уже открытый сеанс или
// вернулся за свой монитор). fn зовётся на потоке окна; nil снимает подписку.
// До создания окна запоминается, уведомления о сеансе включаются с окном.
func (w *Win32Window) SetOnTextRenderingChanged(fn func()) {
	if fn == nil {
		w.onTextRendering.Store(nil)
	} else {
		w.onTextRendering.Store(&fn)
	}
	w.syncSessionNotify()
}

// syncSessionNotify включает уведомления о сеансе, пока есть подписка и окно,
// и выключает без подписки.
func (w *Win32Window) syncSessionNotify() {
	if w.hwnd == 0 || w.isPopup {
		return
	}
	want := w.onTextRendering.Load() != nil
	if want == w.wtsNotify.Load() {
		return
	}
	if want {
		if r, _, _ := procWTSRegisterSessionNotification.Call(uintptr(w.hwnd), notifyForThisSession); r != 0 {
			w.wtsNotify.Store(true)
		}
		return
	}
	procWTSUnRegisterSessionNotification.Call(uintptr(w.hwnd))
	w.wtsNotify.Store(false)
}

// noteTextRendering зовёт подписчика на смену сглаживания текста.
func (w *Win32Window) noteTextRendering() {
	if fn := w.onTextRendering.Load(); fn != nil {
		(*fn)()
	}
}
