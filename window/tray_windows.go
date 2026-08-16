//go:build windows

package window

// tray_windows.go — Win32-реализация иконки в трее (Shell_NotifyIcon),
// balloon-уведомлений (NIF_INFO) и превью окна в панели задач/Aero Peek
// (WM_PRINTCLIENT + опциональный iconic-путь DWM). Чистый Go, без CGO.
//
// Методы приватного интерфейса trayHost (см. tray.go) вызываются на UI-потоке
// окна. Обработка callback- и DWM-сообщений — в wndProc (native_windows.go),
// делегирует методам ниже.

import (
	"image"
	"os"
	"unsafe"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

// ─── Дополнительные Win32/Shell/DWM константы ────────────────────────────────

const (
	// Shell_NotifyIcon: dwMessage
	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	// NOTIFYICONDATA.uFlags
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	// dwInfoFlags (значок balloon)
	niifNone    = 0x00000000
	niifInfo    = 0x00000001
	niifWarning = 0x00000002
	niifError   = 0x00000003

	// Callback-сообщение иконки трея (WM_APP занят InvokeOnUIThread — берём +1).
	wmTrayCallback = wmApp + 1 // 0x8001

	// Превью в панели задач.
	wmPrint       = 0x0317
	wmPrintClient = 0x0318

	// Iconic-представление (DWM).
	wmDwmSendIconicThumbnail         = 0x0323
	wmDwmSendIconicLivePreviewBitmap = 0x0326

	dwmwaForceIconicRepresentation = 7
	dwmwaHasIconicBitmap           = 10

	// GetSystemMetrics indices
	smCXSmIcon = 49
	smCYSmIcon = 50

	// ShowWindow
	swHide = 0
)

// ─── Дополнительные Win32-процедуры ──────────────────────────────────────────

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procShellNotifyIconW              = shell32.NewProc("Shell_NotifyIconW")
	procCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")
	procGetCursorPos                  = user32.NewProc("GetCursorPos")
	procCreateBitmap                  = gdi32.NewProc("CreateBitmap")
	procCreateDIBSection              = gdi32.NewProc("CreateDIBSection")
	procDeleteObject                  = gdi32.NewProc("DeleteObject")
	procDwmSetIconicThumbnail         = dwmapi.NewProc("DwmSetIconicThumbnail")
	procDwmSetIconicLivePreviewBitmap = dwmapi.NewProc("DwmSetIconicLivePreviewBitmap")
)

// ─── Win32 структуры ─────────────────────────────────────────────────────────

// notifyIconDataW — NOTIFYICONDATAW (модерн-версия, Vista+).
type notifyIconDataW struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32 // union: uTimeout / uVersion
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

// iconInfo — ICONINFO для CreateIconIndirect.
type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

// ─── trayHost: иконка в трее ─────────────────────────────────────────────────

// baseNotifyData заполняет общую часть NOTIFYICONDATAW (окно, uID, cbSize).
func (w *Win32Window) baseNotifyData() notifyIconDataW {
	return notifyIconDataW{
		CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:   w.hwnd,
		UID:    1,
	}
}

// setTrayIcon добавляет/обновляет иконку в трее.
func (w *Win32Window) setTrayIcon(icon image.Image, tooltip string) error {
	if w.hwnd == 0 {
		return errTrayUnsupported
	}
	hicon := w.imageToHICON(icon)

	nid := w.baseNotifyData()
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmTrayCallback
	nid.HIcon = hicon
	copyUTF16(nid.SzTip[:], tooltip)

	msg := uintptr(nimAdd)
	if w.trayAdded {
		msg = nimModify
	}
	ret, _, _ := procShellNotifyIconW.Call(msg, uintptr(unsafe.Pointer(&nid)))

	// Уничтожаем прежнюю иконку после успешной замены.
	if w.trayHIcon != 0 {
		procDestroyIcon.Call(uintptr(w.trayHIcon))
	}
	w.trayHIcon = hicon
	if ret == 0 {
		return errTrayUnsupported
	}
	w.trayAdded = true
	return nil
}

// removeTrayIcon убирает иконку из трея.
func (w *Win32Window) removeTrayIcon() {
	if w.hwnd == 0 || !w.trayAdded {
		return
	}
	nid := w.baseNotifyData()
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	if w.trayHIcon != 0 {
		procDestroyIcon.Call(uintptr(w.trayHIcon))
		w.trayHIcon = 0
	}
	w.trayAdded = false
}

// setTrayClickHandler / setBalloonClickHandler регистрируют колбэки.
func (w *Win32Window) setTrayClickHandler(fn func(button int, doubleClick bool)) {
	w.onTrayClick = fn
}
func (w *Win32Window) setBalloonClickHandler(fn func()) { w.onBalloonClick = fn }

// showBalloon показывает системное balloon-уведомление. Требует установленной
// иконки трея.
func (w *Win32Window) showBalloon(title, text string, infoFlag uint32) error {
	if w.hwnd == 0 || !w.trayAdded {
		return errTrayUnsupported
	}
	nid := w.baseNotifyData()
	nid.UFlags = nifInfo
	nid.DwInfoFlags = infoFlag
	copyUTF16(nid.SzInfo[:], text)
	copyUTF16(nid.SzInfoTitle[:], title)
	ret, _, _ := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return errTrayUnsupported
	}
	return nil
}

// cursorScreenPos возвращает позицию курсора (экранные координаты).
func (w *Win32Window) cursorScreenPos() (int, int) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

// hideToTray прячет окно (SW_HIDE — исчезает из панели задач).
func (w *Win32Window) hideToTray() {
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swHide))
	}
}

// restoreFromTray показывает окно и выводит на передний план.
func (w *Win32Window) restoreFromTray() {
	if w.hwnd == 0 {
		return
	}
	procShowWindow.Call(uintptr(w.hwnd), uintptr(swShow))
	procSetForegroundWindow.Call(uintptr(w.hwnd))
}

// ─── Обработка сообщений (вызывается из wndProc) ─────────────────────────────

// handleTrayCallback разбирает callback-сообщение иконки трея.
func (w *Win32Window) handleTrayCallback(lparam uintptr) {
	button, dbl, balloon, ok := trayEventToClick(uint32(lparam))
	if !ok {
		return
	}
	if balloon {
		if w.onBalloonClick != nil {
			w.onBalloonClick()
		}
		return
	}
	if w.onTrayClick != nil {
		w.onTrayClick(button, dbl)
	}
}

// handlePrintClient блитит кэш последнего кадра в переданный HDC (WM_PRINTCLIENT/
// WM_PRINT) — иначе превью в панели задач и Aero Peek чёрные. Кадр == клиентская
// область, блит 1:1.
func (w *Win32Window) handlePrintClient(hdc uintptr) {
	if hdc == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.frameBuf) == 0 || w.bufW <= 0 || w.bufH <= 0 {
		return
	}
	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(w.bufW),
			BiHeight:      -int32(w.bufH), // negative = top-down (буфер не перевёрнут)
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}
	procStretchDIBits.Call(
		hdc,
		0, 0, uintptr(w.bufW), uintptr(w.bufH),
		0, 0, uintptr(w.bufW), uintptr(w.bufH),
		uintptr(unsafe.Pointer(&w.frameBuf[0])),
		uintptr(unsafe.Pointer(&bi)),
		uintptr(dibRgbColors),
		uintptr(srccopy),
	)
}

// maybeEnableIconic включает iconic-представление окна (DWM), если задан
// HEADLESS_GUI_ICONIC_PREVIEW=1. Тогда DWM запрашивает миниатюру/лайв-превью
// через WM_DWMSEND* — мы отдаём их из кэша кадра. По умолчанию выключено:
// достаточно WM_PRINTCLIENT.
func (w *Win32Window) maybeEnableIconic() {
	if os.Getenv("HEADLESS_GUI_ICONIC_PREVIEW") != "1" || w.hwnd == 0 {
		return
	}
	tru := int32(1)
	procDwmSetWindowAttribute.Call(uintptr(w.hwnd), uintptr(dwmwaForceIconicRepresentation),
		uintptr(unsafe.Pointer(&tru)), unsafe.Sizeof(tru))
	procDwmSetWindowAttribute.Call(uintptr(w.hwnd), uintptr(dwmwaHasIconicBitmap),
		uintptr(unsafe.Pointer(&tru)), unsafe.Sizeof(tru))
	w.iconicEnabled = true
}

// handleIconicThumbnail отдаёт DWM миниатюру из кэша кадра (WM_DWMSENDICONIC-
// THUMBNAIL): HIWORD(lParam)=maxW, LOWORD(lParam)=maxH. Возвращает true, если
// обработано.
func (w *Win32Window) handleIconicThumbnail(lparam uintptr) bool {
	if !w.iconicEnabled {
		return false
	}
	maxW := int((lparam >> 16) & 0xFFFF)
	maxH := int(lparam & 0xFFFF)
	src := w.frameToRGBA()
	if src == nil || maxW <= 0 || maxH <= 0 {
		return false
	}
	tw, th := fitAspect(src.Bounds().Dx(), src.Bounds().Dy(), maxW, maxH)
	hbmp := scaledDIB(src, tw, th)
	if hbmp == 0 {
		return false
	}
	procDwmSetIconicThumbnail.Call(uintptr(w.hwnd), hbmp, 0)
	procDeleteObject.Call(hbmp)
	return true
}

// handleIconicLivePreview отдаёт DWM полноразмерный кадр (Aero Peek).
func (w *Win32Window) handleIconicLivePreview() bool {
	if !w.iconicEnabled {
		return false
	}
	src := w.frameToRGBA()
	if src == nil {
		return false
	}
	hbmp := scaledDIB(src, src.Bounds().Dx(), src.Bounds().Dy())
	if hbmp == 0 {
		return false
	}
	procDwmSetIconicLivePreviewBitmap.Call(uintptr(w.hwnd), hbmp, 0, 0)
	procDeleteObject.Call(hbmp)
	return true
}

// ─── Помощники ───────────────────────────────────────────────────────────────

// imageToHICON масштабирует icon до размера маленькой системной иконки и строит
// HICON (цветной DIB с альфой + AND-маска из альфы).
func (w *Win32Window) imageToHICON(icon image.Image) windows.Handle {
	sz := getSystemMetrics(smCXSmIcon)
	szY := getSystemMetrics(smCYSmIcon)
	if sz <= 0 {
		sz = 16
	}
	if szY <= 0 {
		szY = 16
	}
	dst := image.NewRGBA(image.Rect(0, 0, sz, szY))
	if icon != nil {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), icon, icon.Bounds(), xdraw.Over, nil)
	}
	return rgbaToHICON(dst)
}

// rgbaToHICON создаёт HICON из RGBA (32bpp цветной DIB + 1bpp AND-маска).
func rgbaToHICON(img *image.RGBA) windows.Handle {
	b := img.Bounds()
	cw, ch := b.Dx(), b.Dy()
	if cw <= 0 || ch <= 0 {
		return 0
	}

	// Цветной top-down 32bpp DIB-section.
	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(cw),
			BiHeight:      -int32(ch), // negative = top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}
	var bits unsafe.Pointer
	hbmColor, _, _ := procCreateDIBSection.Call(
		0, uintptr(unsafe.Pointer(&bi)), uintptr(dibRgbColors),
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmColor == 0 || bits == nil {
		return 0
	}
	color := iconColorBuffer(img)
	copy(unsafe.Slice((*byte)(bits), len(color)), color)

	// 1bpp AND-маска из альфы.
	mask, _ := iconMaskBuffer(img, 128)
	hbmMask, _, _ := procCreateBitmap.Call(uintptr(cw), uintptr(ch), 1, 1,
		uintptr(unsafe.Pointer(&mask[0])))

	ii := iconInfo{
		FIcon:    1,
		HbmMask:  windows.Handle(hbmMask),
		HbmColor: windows.Handle(hbmColor),
	}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	procDeleteObject.Call(hbmColor)
	if hbmMask != 0 {
		procDeleteObject.Call(hbmMask)
	}
	return windows.Handle(hicon)
}

// frameToRGBA собирает top-down premultiplied RGBA из кэша кадра (frameBuf —
// top-down BGRA, см. PERF-2 в native_windows.go). nil, если кадра ещё нет.
func (w *Win32Window) frameToRGBA() *image.RGBA {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.frameBuf) == 0 || w.bufW <= 0 || w.bufH <= 0 {
		return nil
	}
	cw, ch := w.bufW, w.bufH
	img := image.NewRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		srcRow := y * cw * 4 // frameBuf top-down: порядок строк совпадает
		for x := 0; x < cw; x++ {
			si := srcRow + x*4
			di := img.PixOffset(x, y)
			img.Pix[di+0] = w.frameBuf[si+2] // R (frameBuf: B,G,R,A)
			img.Pix[di+1] = w.frameBuf[si+1] // G
			img.Pix[di+2] = w.frameBuf[si+0] // B
			img.Pix[di+3] = w.frameBuf[si+3] // A
		}
	}
	return img
}

// scaledDIB масштабирует src до tw×th и возвращает HBITMAP (top-down 32bpp,
// premultiplied — как ожидает DWM). 0 при ошибке.
func scaledDIB(src *image.RGBA, tw, th int) uintptr {
	if tw <= 0 || th <= 0 {
		return 0
	}
	dst := src
	if tw != src.Bounds().Dx() || th != src.Bounds().Dy() {
		dst = image.NewRGBA(image.Rect(0, 0, tw, th))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	}
	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(tw),
			BiHeight:      -int32(th), // top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}
	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		0, uintptr(unsafe.Pointer(&bi)), uintptr(dibRgbColors),
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		return 0
	}
	copy(unsafe.Slice((*byte)(bits), tw*th*4), iconColorBuffer(dst))
	return hbmp
}

// fitAspect вписывает w×h в maxW×maxH с сохранением пропорций.
func fitAspect(w, h, maxW, maxH int) (int, int) {
	if w <= 0 || h <= 0 {
		return maxW, maxH
	}
	tw, th := maxW, maxW*h/w
	if th > maxH {
		th = maxH
		tw = maxH * w / h
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	return tw, th
}

// copyUTF16 кодирует s в dst (UTF-16), обрезая до len(dst)-1 и завершая NUL.
func copyUTF16(dst []uint16, s string) {
	u := windows.StringToUTF16(s)
	n := len(u)
	if n > len(dst) {
		n = len(dst)
		u[n-1] = 0
	}
	copy(dst[:n], u[:n])
}
