//go:build windows

// icon_windows.go — значок окна на Win32 (WM_SETICON).

package win32

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/platform/native"
)

const (
	wmSetIcon    = 0x0080
	iconSmallIdx = 0 // значок в заголовке окна и на панели задач
	iconBigIdx   = 1 // значок в Alt+Tab и в свойствах окна
)

var (
	procSendMessageW       = user32.NewProc("SendMessageW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
)

// iconInfo — ICONINFO для CreateIconIndirect.
type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

// Проверка на этапе сборки: Window.SetIcon находит бэкенд по интерфейсу, и
// опечатка в имени метода превратилась бы в молчаливый ErrIconUnsupported.
var _ native.IconSetter = (*Win32Window)(nil)

// SetIcon ставит окну значок сообщением WM_SETICON.
//
// Значков два: маленький (16×16, заголовок и панель задач) и большой (32×32,
// Alt+Tab). Windows масштабирует недостающий сама, но делает это грубо, и
// приложение, у которого есть оба размера, должно иметь возможность отдать оба.
//
// Прежние значки удаляются ПОСЛЕ установки новых: WM_SETICON возвращает
// предыдущий хэндл, и удалить его раньше — значит на мгновение оставить окно с
// уничтоженным объектом GDI.
func (w *Win32Window) SetIcon(icons []image.Image) error {
	if w.hwnd == 0 {
		// Окна ещё нет — запоминаем, поставим при создании.
		w.pendingIcons = icons
		return nil
	}
	if len(icons) == 0 {
		w.applyIcon(iconSmallIdx, 0)
		w.applyIcon(iconBigIdx, 0)
		return nil
	}

	small := rgbaToHICON(native.IconToRGBA(native.PickIcon(icons, 16)))
	big := rgbaToHICON(native.IconToRGBA(native.PickIcon(icons, 32)))
	w.applyIcon(iconSmallIdx, small)
	w.applyIcon(iconBigIdx, big)
	return nil
}

// applyIcon ставит один из двух значков и удаляет прежний.
func (w *Win32Window) applyIcon(which int, hicon windows.Handle) {
	prev, _, _ := procSendMessageW.Call(
		uintptr(w.hwnd), wmSetIcon, uintptr(which), uintptr(hicon))
	// Удаляем только СВОЙ прежний значок: значок из ресурсов исполняемого
	// файла принадлежит модулю, и уничтожать его нельзя.
	if old := w.ownIcons[which]; old != 0 && windows.Handle(prev) == old {
		procDestroyIcon.Call(uintptr(old))
	}
	w.ownIcons[which] = hicon
}

// applyPendingIcon ставит значок, заданный до создания окна.
func (w *Win32Window) applyPendingIcon() {
	if len(w.pendingIcons) == 0 {
		return
	}
	icons := w.pendingIcons
	w.pendingIcons = nil
	_ = w.SetIcon(icons)
}

// NewIcon строит HICON из картинки — тот же, что у значка окна (32bpp цветной
// DIB с альфой + AND-маска из альфы). Точка расширения для возможностей вне
// бэкенда (трей, platform/tray); уничтожает значок вызывающий (DestroyIcon).
// 0 — не вышло.
func NewIcon(img *image.RGBA) windows.Handle { return rgbaToHICON(img) }

// rgbaToHICON создаёт HICON из RGBA (32bpp цветной DIB + 1bpp AND-маска).
func rgbaToHICON(img *image.RGBA) windows.Handle {
	return rgbaToHandle(img, true, 0, 0)
}

// rgbaToHandle создаёт HICON (icon=true) или HCURSOR (icon=false, с горячей
// точкой hotX, hotY) из RGBA: 32bpp цветной DIB с альфой + 1bpp AND-маска.
func rgbaToHandle(img *image.RGBA, icon bool, hotX, hotY uint32) windows.Handle {
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
	color := native.IconColorBuffer(img)
	copy(unsafe.Slice((*byte)(bits), len(color)), color)

	// 1bpp AND-маска из альфы.
	mask, _ := iconMaskBuffer(img, 128)
	hbmMask, _, _ := procCreateBitmap.Call(uintptr(cw), uintptr(ch), 1, 1,
		uintptr(unsafe.Pointer(&mask[0])))

	ii := iconInfo{
		HbmMask:  windows.Handle(hbmMask),
		HbmColor: windows.Handle(hbmColor),
	}
	if icon {
		ii.FIcon = 1
	} else {
		ii.XHotspot, ii.YHotspot = hotX, hotY
	}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	procDeleteObject.Call(hbmColor)
	if hbmMask != 0 {
		procDeleteObject.Call(hbmMask)
	}
	return windows.Handle(hicon)
}
