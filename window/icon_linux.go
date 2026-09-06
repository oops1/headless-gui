//go:build linux

// icon_linux.go — значок окна на X11 (_NET_WM_ICON).
//
// Свойство не выставлялось вовсе, поэтому под Linux у окна не было значка в
// принципе — сколько бы ресурсов ни лежало в сборке: ресурсы Windows
// оконному менеджеру X11 ни о чём не говорят.
package window

import "image"

// atomCardinal — предопределённый атом CARDINAL (X11 protocol, §Atoms).
// Предопределённый, поэтому InternAtom не нужен.
const atomCardinal = 6

// setIcon выставляет _NET_WM_ICON.
//
// Формат свойства (EWMH): подряд идут значки, каждый — ширина, высота и
// width×height точек ARGB, по 32 бита на число. Несколько размеров в одном
// свойстве — это норма, оконный менеджер выбирает подходящий под место сам:
// один для заголовка, другой для панели задач, третий для Alt+Tab.
func (w *X11Window) setIcon(icons []image.Image) error {
	if w.wid == 0 {
		w.pendingIcons = icons
		return nil
	}
	if w.atomNetWMIcon == 0 {
		w.atomNetWMIcon = w.x11InternAtom("_NET_WM_ICON")
	}
	if w.atomNetWMIcon == 0 {
		return ErrIconUnsupported
	}
	w.x11ChangeProperty(w.wid, w.atomNetWMIcon, atomCardinal, 32, netWMIconData(icons))
	return nil
}

// applyPendingIcon выставляет значок, заданный до создания окна.
func (w *X11Window) applyPendingIcon() {
	if len(w.pendingIcons) == 0 {
		return
	}
	icons := w.pendingIcons
	w.pendingIcons = nil
	_ = w.setIcon(icons)
}
