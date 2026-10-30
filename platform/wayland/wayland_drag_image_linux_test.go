//go:build linux && !android

package wayland

import (
	"image"
	"image/color"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Значок перетаскивания на Wayland, на проводе: shm-буфер с картинкой, поверхность,
// start_drag с поверхностью третьим аргументом, attach со смещением на точку
// захвата после start_drag и освобождение при конце перетаскивания.

// wlDrainFDs вычитывает всё, что окно отправило, вместе с fd (SCM_RIGHTS).
func wlDrainFDs(t *testing.T, peer *net.UnixConn) (data []byte, fds []int) {
	t.Helper()
	buf := make([]byte, 16384)
	for {
		oob := make([]byte, unix.CmsgSpace(4*4))
		peer.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		n, oobn, _, _, err := peer.ReadMsgUnix(buf, oob)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if oobn > 0 {
			cms, perr := unix.ParseSocketControlMessage(oob[:oobn])
			if perr == nil {
				for i := range cms {
					if got, rerr := unix.ParseUnixRights(&cms[i]); rerr == nil {
						fds = append(fds, got...)
					}
				}
			}
		}
		if err != nil {
			return data, fds
		}
	}
}

func wlImageSetup(t *testing.T) *wlTestConn {
	c := dragSourceSetup(t)
	c.w.gCompositorVer = 4
	return c
}

func wlDragTestImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 128, A: 128}) // премультиплицированный полупрозрачный зелёный
	img.SetRGBA(3, 1, color.RGBA{B: 200, A: 200})
	return img
}

func TestWayland_DragImage_Wire(t *testing.T) {
	c := wlImageSetup(t)
	if !c.w.DragImageSupported() {
		t.Fatal("DragImageSupported false при полном наборе глобалов")
	}
	var got []native.DragEffect
	if !c.w.BeginDragFilesImage([]string{"/tmp/a"}, native.DragEffectCopy, wlDragTestImage(), image.Pt(3, 1), func(e native.DragEffect) { got = append(got, e) }) {
		t.Fatal("BeginDragFilesImage отказал")
	}
	data, fds := wlDrainFDs(t, c.peer)
	defer func() {
		for _, fd := range fds {
			unix.Close(fd)
		}
	}()
	msgs := splitWlMsgs(t, data)

	// Пул значка: wl_shm.create_pool с fd, в памяти — BGRA премультиплицированный.
	pool, ok := findWlMsg(msgs, c.w.shmID, wlShmCreatePool)
	if !ok || len(pool.args) != 2 || pool.args[1] != 4*2*4 {
		t.Fatalf("create_pool: %+v", pool)
	}
	if len(fds) != 1 {
		t.Fatalf("передано fd: %d, ждал 1", len(fds))
	}
	mem, err := unix.Mmap(fds[0], 0, 4*2*4, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		t.Fatalf("mmap пула значка: %v", err)
	}
	defer unix.Munmap(mem)
	px := func(x, y int) [4]byte { o := y*16 + x*4; return [4]byte{mem[o], mem[o+1], mem[o+2], mem[o+3]} }
	if px(0, 0) != [4]byte{0, 0, 255, 255} { // B,G,R,A красного
		t.Errorf("пиксель (0,0) в буфере = %v, ждал красный BGRA {0 0 255 255}", px(0, 0))
	}
	if px(1, 0) != [4]byte{0, 128, 0, 128} {
		t.Errorf("пиксель (1,0) в буфере = %v, ждал {0 128 0 128} (премультиплицированный)", px(1, 0))
	}
	if px(3, 1) != [4]byte{200, 0, 0, 200} {
		t.Errorf("пиксель (3,1) = %v", px(3, 1))
	}

	// Буфер нужного размера, формат ARGB8888; пул сразу уничтожается.
	buf, ok := findWlMsg(msgs, pool.args[0], wlShmPoolCreateBuffer)
	if !ok || len(buf.args) != 6 || buf.args[2] != 4 || buf.args[3] != 2 || buf.args[4] != 16 || buf.args[5] != wlShmFormatARGB8888 {
		t.Fatalf("create_buffer: %+v", buf)
	}
	if _, ok := findWlMsg(msgs, pool.args[0], wlShmPoolDestroy); !ok {
		t.Error("пул значка не уничтожен после create_buffer")
	}

	// Поверхность значка и её место в порядке: до start_drag — создание, после
	// — attach/damage/commit.
	surf, ok := findWlMsg(msgs, c.w.compositorID, wlCompositorCreateSurface)
	if !ok {
		t.Fatal("поверхность значка не создана")
	}
	start, ok := findWlMsg(msgs, 31, wlDataDeviceStartDrag)
	if !ok || len(start.args) != 4 || start.args[2] != surf.args[0] || start.args[3] != 55 {
		t.Fatalf("start_drag %+v: третьим аргументом ждал поверхность %d", start, surf.args[0])
	}
	idx := func(obj uint32, op uint16) int {
		for i, m := range msgs {
			if m.obj == obj && m.opcode == op {
				return i
			}
		}
		return -1
	}
	iSurf, iStart := idx(c.w.compositorID, wlCompositorCreateSurface), idx(31, wlDataDeviceStartDrag)
	iAttach, iCommit := idx(surf.args[0], wlSurfaceAttach), idx(surf.args[0], wlSurfaceCommit)
	if !(iSurf < iStart && iStart < iAttach && iAttach < iCommit) {
		t.Errorf("порядок create_surface(%d) < start_drag(%d) < attach(%d) < commit(%d) нарушен", iSurf, iStart, iAttach, iCommit)
	}
	att, _ := findWlMsg(msgs, surf.args[0], wlSurfaceAttach)
	if len(att.args) != 3 || att.args[0] != buf.args[0] || int32(att.args[1]) != -3 || int32(att.args[2]) != -1 {
		t.Errorf("attach %v: ждал буфер %d и смещение (-3,-1) — точка захвата под курсором", att.args, buf.args[0])
	}
	if dmg, ok := findWlMsg(msgs, surf.args[0], wlSurfaceDamageBuffer); !ok || dmg.args[2] != 4 || dmg.args[3] != 2 {
		t.Errorf("damage_buffer %+v", dmg)
	}

	// Конец перетаскивания освобождает поверхность и буфер.
	c.w.handleEvent(c.w.drag.id, wlDataSourceEvCancelled, nil)
	if len(got) != 1 {
		t.Fatalf("done вызван %d раз", len(got))
	}
	data, _ = wlDrainFDs(t, c.peer)
	end := splitWlMsgs(t, data)
	if _, ok := findWlMsg(end, surf.args[0], wlSurfaceDestroy); !ok {
		t.Error("поверхность значка не уничтожена")
	}
	if _, ok := findWlMsg(end, buf.args[0], wlBufferDestroy); !ok {
		t.Error("буфер значка не уничтожен")
	}
}

// HiDPI: картинка в пикселях экрана показывается 1:1 — буферный масштаб равен
// масштабу окна, смещение точки захвата — в единицах поверхности.
func TestWayland_DragImage_HiDPI(t *testing.T) {
	c := wlImageSetup(t)
	c.w.scale.current = 2
	c.w.BeginDragFilesImage([]string{"/tmp/a"}, native.DragEffectCopy, wlDragTestImage(), image.Pt(3, 1), func(native.DragEffect) {})
	data, fds := wlDrainFDs(t, c.peer)
	for _, fd := range fds {
		unix.Close(fd)
	}
	msgs := splitWlMsgs(t, data)
	surf, _ := findWlMsg(msgs, c.w.compositorID, wlCompositorCreateSurface)
	if sc, ok := findWlMsg(msgs, surf.args[0], wlSurfaceSetBufferScale); !ok || sc.args[0] != 2 {
		t.Errorf("set_buffer_scale %+v, ждал 2", sc)
	}
	att, _ := findWlMsg(msgs, surf.args[0], wlSurfaceAttach)
	if len(att.args) != 3 || int32(att.args[1]) != -1 || int32(att.args[2]) != 0 {
		t.Errorf("attach %v: ждал смещение (-3/2, -1/2) = (-1, 0)", att.args)
	}
}

// Без значка и без поддержки start_drag идёт с icon = NULL, и ни поверхности, ни
// пула не заводится.
func TestWayland_DragImage_Absent(t *testing.T) {
	// Картинки нет.
	c := wlImageSetup(t)
	c.w.BeginDragFiles([]string{"/tmp/a"}, native.DragEffectCopy, func(native.DragEffect) {})
	data, fds := wlDrainFDs(t, c.peer)
	if len(fds) != 0 {
		t.Errorf("без значка передано fd: %d", len(fds))
	}
	msgs := splitWlMsgs(t, data)
	if start, ok := findWlMsg(msgs, 31, wlDataDeviceStartDrag); !ok || start.args[2] != 0 {
		t.Errorf("start_drag без значка: %+v", start)
	}
	if _, ok := findWlMsg(msgs, c.w.compositorID, wlCompositorCreateSurface); ok {
		t.Error("поверхность создана без значка")
	}

	// Нет wl_shm — значок невозможен, перетаскивание всё равно идёт.
	c = wlImageSetup(t)
	c.w.shmID = 0
	if c.w.DragImageSupported() {
		t.Error("DragImageSupported true без wl_shm")
	}
	if !c.w.BeginDragFilesImage([]string{"/tmp/a"}, native.DragEffectCopy, wlDragTestImage(), image.Pt(0, 0), func(native.DragEffect) {}) {
		t.Fatal("без wl_shm перетаскивание не началось")
	}
	data, _ = wlDrainFDs(t, c.peer)
	if start, ok := findWlMsg(splitWlMsgs(t, data), 31, wlDataDeviceStartDrag); !ok || start.args[2] != 0 {
		t.Errorf("start_drag без wl_shm: %+v", start)
	}
}
