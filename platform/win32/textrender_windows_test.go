//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// Настройка сглаживания текста читается у живой системы, а подписчика будят
// только сообщения про сглаживание и про сеанс; с подпиской окно получает
// уведомления о сеансе, без неё — нет, и с окном они снимаются.
func TestWin32TextRendering(t *testing.T) {
	w := newFsWindow(t)
	tr := w.TextRendering()
	if !tr.Known {
		t.Fatal("SystemParametersInfo(SPI_GETFONTSMOOTHING) не ответил")
	}
	t.Logf("сглаживание текста: %+v", tr)

	n := 0
	w.SetOnTextRenderingChanged(func() { n++ })
	if !w.wtsNotify.Load() {
		t.Error("с подпиской уведомления о сеансе не включились")
	}
	hwnd := uintptr(w.hwnd)
	for _, spi := range []uintptr{spiSetFontSmoothing, spiSetFontSmoothingType, spiSetFontSmoothingContrast, spiSetFontSmoothingOrientation} {
		wndProc(hwnd, wmSettingchange, spi, 0)
	}
	if n != 4 {
		t.Errorf("WM_SETTINGCHANGE про сглаживание дал %d вызовов, ждал 4", n)
	}
	wndProc(hwnd, wmWtsSessionChange, 3 /* WTS_REMOTE_CONNECT */, 0)
	if n != 5 {
		t.Errorf("WM_WTSSESSION_CHANGE дал %d вызовов, ждал 5", n)
	}
	wndProc(hwnd, wmSettingchange, 0x0001 /* SPI_SETBEEP */, 0)
	wndProc(hwnd, wmSettingchange, spiSetWorkArea, 0)
	if n != 5 {
		t.Errorf("посторонний WM_SETTINGCHANGE разбудил подписчика: %d", n)
	}

	w.SetOnTextRenderingChanged(nil)
	if w.wtsNotify.Load() {
		t.Error("подписка снята, а уведомления о сеансе остались")
	}
	wndProc(hwnd, wmSettingchange, spiSetFontSmoothingType, 0)
	if n != 5 {
		t.Errorf("подписка снята, но вызов был: %d", n)
	}
}

// Подписка, заданная до создания окна, включает уведомления о сеансе вместе
// с окном.
func TestWin32TextRendering_SubscribeBeforeCreate(t *testing.T) {
	w := &Win32Window{}
	w.noQuit = true
	w.SetOnTextRenderingChanged(func() {})
	if w.wtsNotify.Load() {
		t.Fatal("уведомления о сеансе включились без окна")
	}
	if err := w.Create("tr-test", 200, 150); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(uintptr(w.hwnd)) })
	if !w.wtsNotify.Load() {
		t.Error("окно создано, подписка есть, а уведомлений о сеансе нет")
	}
}

// readTextRendering сходится с прямым ответом системы: субпиксельное — только
// при включённом сглаживании типа ClearType.
func TestWin32TextRendering_Read(t *testing.T) {
	tr := readTextRendering()
	if !tr.Known {
		t.Skip("SystemParametersInfo не ответил")
	}
	var typ uint32
	procSystemParametersInfoW.Call(spiGetFontSmoothingType, 0, uintptr(unsafe.Pointer(&typ)), 0)
	var on uint32
	procSystemParametersInfoW.Call(spiGetFontSmoothing, 0, uintptr(unsafe.Pointer(&on)), 0)
	if want := on != 0 && typ == feFontSmoothingClearType; tr.Subpixel != want {
		t.Errorf("Subpixel=%v, а сглаживание=%d тип=%d", tr.Subpixel, on, typ)
	}
}
