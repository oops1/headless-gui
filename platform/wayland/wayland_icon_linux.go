//go:build linux && !android

package wayland

// wayland_icon_linux.go — значок окна на Wayland (xdg-toplevel-icon-v1).
//
// Протокол 2024 года: компоновщик объявляет глобал xdg_toplevel_icon_manager_v1
// (KDE 6.1+, Sway, Mutter с расширением). Значок собирается из shm-буферов:
//
//	icon = manager.create_icon()
//	icon.add_buffer(buffer, scale)   // по одному на каждый размер
//	manager.set_icon(toplevel, icon)
//	icon.destroy()
//
// Буферы — квадратные ARGB8888 с ПРЕМУЛЬТИПЛИЦИРОВАННОЙ альфой. Компоновщик
// выбирает подходящий размер сам (как оконные менеджеры X11 из _NET_WM_ICON);
// пустой список снимает значок (set_icon с пустым объектом).
//
// Компоновщик без глобала (WinLine берёт значок из .desktop) получает прежнюю
// ErrIconUnsupported. Значок, заданный до создания окна, откладывается и
// ставится сразу после первого configure.

import (
	"image"
	"image/draw"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Запросы xdg_toplevel_icon_manager_v1 и xdg_toplevel_icon_v1, wl_buffer.
const (
	xdgIconMgrCreateIcon = 1
	xdgIconMgrSetIcon    = 2
	xdgIconAddBuffer     = 2
	xdgIconDestroy       = 0

	// Предел стороны значка: компоновщики берут единицы-сотни точек, а
	// огромная картинка только раздувает память пула.
	wlIconMaxSide = 1024
)

var _ native.IconSetter = (*WaylandWindow)(nil)

// wlIconState — то, что держит значок окна: объекты протокола и память пула.
type wlIconState struct {
	mu      sync.Mutex
	mgrID   uint32 // привязанный xdg_toplevel_icon_manager_v1; 0 — ещё не привязан
	pending []image.Image
	want    bool // значок задан до создания окна

	// Пул и буферы текущего значка живут, пока компоновщик не заменил его:
	// содержимое буфера он вправе читать и после set_icon.
	poolID uint32
	bufIDs []uint32
	fd     int
	data   []byte
}

// SetIcon ставит значок окна через xdg-toplevel-icon-v1.
//
// До создания окна значок только запоминается (nil): глобалы компоновщика
// становятся известны в Create. Без глобала — ErrIconUnsupported.
func (w *WaylandWindow) SetIcon(icons []image.Image) error {
	w.icon.mu.Lock()
	defer w.icon.mu.Unlock()
	if w.toplevelID == 0 {
		w.icon.pending, w.icon.want = icons, true
		return nil
	}
	return w.setIconLocked(icons)
}

// applyPendingWlIcon ставит значок, заданный до создания окна.
func (w *WaylandWindow) applyPendingWlIcon() {
	w.icon.mu.Lock()
	defer w.icon.mu.Unlock()
	if !w.icon.want {
		return
	}
	icons := w.icon.pending
	w.icon.pending, w.icon.want = nil, false
	_ = w.setIconLocked(icons)
}

func (w *WaylandWindow) setIconLocked(icons []image.Image) error {
	if w.gToplevelIcon == 0 {
		return native.ErrIconUnsupported
	}
	if w.icon.mgrID == 0 {
		w.icon.mgrID = w.bind(w.gToplevelIcon, "xdg_toplevel_icon_manager_v1", 1)
	}
	mgr := w.icon.mgrID

	if len(icons) == 0 {
		w.send(newWlMsg(mgr, xdgIconMgrSetIcon).putUint(w.toplevelID).putUint(0), -1)
		w.releaseIconBuffers()
		return nil
	}

	sides, pix := wlIconPixels(icons)
	if len(sides) == 0 {
		return native.ErrIconUnsupported
	}
	total := len(pix)
	fd, err := unix.MemfdCreate("headless-gui-icon", unix.MFD_CLOEXEC)
	if err != nil {
		return err
	}
	if err := unix.Ftruncate(fd, int64(total)); err != nil {
		unix.Close(fd)
		return err
	}
	data, err := unix.Mmap(fd, 0, total, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return err
	}
	copy(data, pix)

	poolID := w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(poolID).putInt(int32(total)), fd)
	iconID := w.newID()
	w.send(newWlMsg(mgr, xdgIconMgrCreateIcon).putUint(iconID), -1)
	var bufs []uint32
	off := 0
	for _, side := range sides {
		stride := side * 4
		id := w.newID()
		w.send(newWlMsg(poolID, wlShmPoolCreateBuffer).
			putUint(id).putInt(int32(off)).
			putInt(int32(side)).putInt(int32(side)).
			putInt(int32(stride)).putUint(wlShmFormatARGB8888), -1)
		w.send(newWlMsg(iconID, xdgIconAddBuffer).putUint(id).putInt(1), -1)
		bufs = append(bufs, id)
		off += stride * side
	}
	w.send(newWlMsg(mgr, xdgIconMgrSetIcon).putUint(w.toplevelID).putUint(iconID), -1)
	w.send(newWlMsg(iconID, xdgIconDestroy), -1)

	// Прежний значок заменён — его память больше не нужна.
	w.releaseIconBuffers()
	w.icon.poolID, w.icon.bufIDs, w.icon.fd, w.icon.data = poolID, bufs, fd, data
	return nil
}

// releaseIconBuffers освобождает пул и буферы прежнего значка.
func (w *WaylandWindow) releaseIconBuffers() {
	for _, id := range w.icon.bufIDs {
		w.send(newWlMsg(id, wlBufferDestroy), -1)
	}
	if w.icon.poolID != 0 {
		w.send(newWlMsg(w.icon.poolID, wlShmPoolDestroy), -1)
	}
	w.closeIconMemory()
	w.icon.poolID, w.icon.bufIDs = 0, nil
}

// closeIconMemory отпускает память пула значка.
func (w *WaylandWindow) closeIconMemory() {
	if w.icon.data != nil {
		unix.Munmap(w.icon.data)
		w.icon.data = nil
	}
	if w.icon.fd > 0 {
		unix.Close(w.icon.fd)
		w.icon.fd = 0
	}
}

// wlIconPixels готовит пиксели значков: квадратные ARGB8888 с премультиплицированной
// альфой (в памяти — B, G, R, A), подряд друг за другом. Неквадратная картинка
// вписывается по центру в прозрачный квадрат; второй значок того же размера и
// пустые пропускаются.
func wlIconPixels(icons []image.Image) (sides []int, pix []byte) {
	seen := map[int]bool{}
	for _, ic := range icons {
		if ic == nil || ic.Bounds().Empty() {
			continue
		}
		b := ic.Bounds()
		side := max(b.Dx(), b.Dy())
		if side > wlIconMaxSide || seen[side] {
			continue
		}
		seen[side] = true
		sq := image.NewRGBA(image.Rect(0, 0, side, side)) // премультиплицированный
		dst := image.Rect((side-b.Dx())/2, (side-b.Dy())/2, (side-b.Dx())/2+b.Dx(), (side-b.Dy())/2+b.Dy())
		draw.Draw(sq, dst, ic, b.Min, draw.Src)
		start := len(pix)
		pix = append(pix, sq.Pix...)
		for i := start; i < len(pix); i += 4 {
			pix[i], pix[i+2] = pix[i+2], pix[i] // RGBA → BGRA
		}
		sides = append(sides, side)
	}
	return sides, pix
}
