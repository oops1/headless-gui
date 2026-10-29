//go:build windows

// icon_windows.go — значок окна на Win32 (WM_SETICON).
package core

import (
	"image"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v3/platform/native"
)

const (
	wmSetIcon    = 0x0080
	iconSmallIdx = 0 // значок в заголовке окна и на панели задач
	iconBigIdx   = 1 // значок в Alt+Tab и в свойствах окна
)

var procSendMessageW = user32.NewProc("SendMessageW")

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
