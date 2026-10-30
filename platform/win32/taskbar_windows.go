//go:build windows

package win32

// taskbar_windows.go — окно в панели задач: превью окна в панели задач и Aero
// Peek (WM_PRINTCLIENT + опциональный iconic-путь DWM) и «спрятать в трей» —
// скрыть окно из панели задач и вернуть (HideToTray, RestoreFromTray). Иконка в
// трее и уведомления — возможность platform/tray. Чистый Go, без CGO.

import (
	"image"
	"os"
	"unsafe"

	xdraw "golang.org/x/image/draw"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// ─── Дополнительные Win32/Shell/DWM константы ────────────────────────────────

const (
	// Превью в панели задач.
	wmPrint       = 0x0317
	wmPrintClient = 0x0318

	// Iconic-представление (DWM).
	wmDwmSendIconicThumbnail         = 0x0323
	wmDwmSendIconicLivePreviewBitmap = 0x0326

	dwmwaForceIconicRepresentation = 7
	dwmwaHasIconicBitmap           = 10

	// ShowWindow
	swHide = 0
)

var (
	procDwmSetIconicThumbnail         = dwmapi.NewProc("DwmSetIconicThumbnail")
	procDwmSetIconicLivePreviewBitmap = dwmapi.NewProc("DwmSetIconicLivePreviewBitmap")
)

// ─── Спрятать в трей ─────────────────────────────────────────────────────────

// HideToTray прячет окно (SW_HIDE — исчезает из панели задач).
func (w *Win32Window) HideToTray() {
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swHide))
	}
}

// RestoreFromTray показывает окно и выводит на передний план.
func (w *Win32Window) RestoreFromTray() {
	if w.hwnd == 0 {
		return
	}
	procShowWindow.Call(uintptr(w.hwnd), uintptr(swShow))
	procSetForegroundWindow.Call(uintptr(w.hwnd))
}

// ─── Превью (вызывается из wndProc) ──────────────────────────────────────────

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
	copy(unsafe.Slice((*byte)(bits), tw*th*4), native.IconColorBuffer(dst))
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
