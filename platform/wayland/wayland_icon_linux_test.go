//go:build linux && !android

package wayland

import (
	"errors"
	"image"
	"image/color"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Значок окна на Wayland (xdg-toplevel-icon-v1). Компоновщик подменён парой
// сокетов; file descriptor пула приходит в служебных данных сообщения.

const (
	testIconMgrName = 50 // имя глобала в реестре
	testIconRegID   = 2

	// Опкоды из xdg-toplevel-icon-v1.xml — буквально, независимо от констант
	// в коде: опечатка в них иначе прошла бы тест.
	tIconCreate    = 1 // xdg_toplevel_icon_manager_v1.create_icon
	tIconSet       = 2 // xdg_toplevel_icon_manager_v1.set_icon
	tIconDestroy   = 0 // xdg_toplevel_icon_v1.destroy
	tIconAddBuffer = 2 // xdg_toplevel_icon_v1.add_buffer
)

// readWithFDs читает всё, что окно отправило, вместе с принятыми
// descriptor'ами.
func readWithFDs(t *testing.T, c *wlTestConn) (raw []byte, fds []int) {
	t.Helper()
	// Потоковый сокет не склеивает сообщения с descriptor'ом с соседними за
	// одно чтение, поэтому читаем, пока не наступит тишина.
	wait := time.Second
	for {
		c.peer.SetReadDeadline(time.Now().Add(wait))
		buf, oob := make([]byte, 8192), make([]byte, 256)
		n, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
		if err != nil {
			if len(raw) == 0 {
				t.Fatalf("композитор ничего не получил: %v", err)
			}
			return raw, fds
		}
		raw = append(raw, buf[:n]...)
		if oobn > 0 {
			if msgs, err := syscall.ParseSocketControlMessage(oob[:oobn]); err == nil {
				for i := range msgs {
					if f, err := syscall.ParseUnixRights(&msgs[i]); err == nil {
						fds = append(fds, f...)
					}
				}
			}
		}
		wait = 60 * time.Millisecond
	}
}

func newWlIconWindow(t *testing.T) *wlTestConn {
	c := newWlTestWindow(t)
	c.w.registryID = testIconRegID
	c.w.gToplevelIcon = testIconMgrName
	return c
}

// iconPicture — квадратный значок с полупрозрачной точкой.
func iconPicture(side int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	img.SetNRGBA(0, 0, color.NRGBA{200, 100, 50, 128})
	img.SetNRGBA(side-1, side-1, color.NRGBA{10, 20, 30, 255})
	return img
}

// Без глобала — прежняя ошибка, и в сокет ничего не уходит.
func TestWayland_SetIcon_NoGlobalUnsupported(t *testing.T) {
	c := newWlTestWindow(t)
	err := c.w.SetIcon([]image.Image{iconPicture(16)})
	if !errors.Is(err, native.ErrIconUnsupported) {
		t.Fatalf("ждал ErrIconUnsupported, получил %v", err)
	}
	c.peer.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n > 0 {
		t.Errorf("без глобала отправлено %d байт", n)
	}
}

// С глобалом: bind менеджера, create_icon, пул с descriptor'ом, по буферу на
// размер и add_buffer, set_icon на toplevel окна, destroy значка — в таком
// порядке и с такими опкодами. Пиксели — BGRA с премультиплицированной альфой.
func TestWayland_SetIcon_Wire(t *testing.T) {
	c := newWlIconWindow(t)
	wide := image.NewNRGBA(image.Rect(0, 0, 20, 10)) // неквадратный: вписывается в 20×20
	wide.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 255})
	if err := c.w.SetIcon([]image.Image{iconPicture(16), wide}); err != nil {
		t.Fatal(err)
	}
	raw, fds := readWithFDs(t, c)
	msgs := splitWlMsgs(t, raw)

	bind, ok := findWlMsg(msgs, testIconRegID, wlRegistryBind)
	if !ok || bind.args[0] != testIconMgrName {
		t.Fatalf("bind менеджера не отправлен: %+v", msgs)
	}
	mgr := bind.args[len(bind.args)-1]
	if got := string(raw[bytesIndex(raw, "xdg_toplevel_icon_manager_v1"):][:28]); got != "xdg_toplevel_icon_manager_v1" {
		t.Errorf("интерфейс в bind: %q", got)
	}

	create, ok := findWlMsg(msgs, mgr, tIconCreate)
	if !ok || len(create.args) != 1 {
		t.Fatalf("create_icon: %+v", msgs)
	}
	icon := create.args[0]

	pool, ok := findWlMsg(msgs, c.w.shmID, wlShmCreatePool)
	if !ok || len(fds) != 1 {
		t.Fatalf("пул не создан или без descriptor'а: %v, fds=%v", ok, fds)
	}
	if want := uint32(16*16*4 + 20*20*4); pool.args[1] != want {
		t.Errorf("размер пула %d, ждал %d", pool.args[1], want)
	}

	// Буферы: 16×16 со смещением 0 и 20×20 следом; формат ARGB8888.
	var bufs []wlWireMsg
	for _, m := range msgs {
		if m.obj == pool.args[0] && m.opcode == wlShmPoolCreateBuffer {
			bufs = append(bufs, m)
		}
	}
	if len(bufs) != 2 {
		t.Fatalf("буферов %d, ждал 2", len(bufs))
	}
	for i, want := range [][5]uint32{{0, 16, 16, 64, wlShmFormatARGB8888}, {16 * 16 * 4, 20, 20, 80, wlShmFormatARGB8888}} {
		got := [5]uint32{bufs[i].args[1], bufs[i].args[2], bufs[i].args[3], bufs[i].args[4], bufs[i].args[5]}
		if got != want {
			t.Errorf("буфер %d: %v, ждал %v", i, got, want)
		}
	}

	// add_buffer на каждый буфер, scale = 1.
	adds := 0
	for _, m := range msgs {
		if m.obj == icon && m.opcode == tIconAddBuffer {
			if m.args[0] != bufs[adds].args[0] || m.args[1] != 1 {
				t.Errorf("add_buffer %d: %v", adds, m.args)
			}
			adds++
		}
	}
	if adds != 2 {
		t.Errorf("add_buffer: %d, ждал 2", adds)
	}

	set, ok := findWlMsg(msgs, mgr, tIconSet)
	if !ok || set.args[0] != c.w.toplevelID || set.args[1] != icon {
		t.Fatalf("set_icon: %v %v (toplevel %d, icon %d)", set.args, ok, c.w.toplevelID, icon)
	}
	if _, ok := findWlMsg(msgs, icon, tIconDestroy); !ok {
		t.Error("объект значка не уничтожен после set_icon")
	}
	// Порядок: add_buffer'ы и set_icon идут после create_icon, destroy — последним.
	idx := func(obj uint32, op uint16) int {
		for i, m := range msgs {
			if m.obj == obj && m.opcode == op {
				return i
			}
		}
		return -1
	}
	if !(idx(mgr, tIconCreate) < idx(icon, tIconAddBuffer) &&
		idx(icon, tIconAddBuffer) < idx(mgr, tIconSet) &&
		idx(mgr, tIconSet) < idx(icon, tIconDestroy)) {
		t.Errorf("нарушен порядок запросов: %+v", msgs)
	}

	// Пиксели в пуле: BGRA, цвет умножен на альфу.
	mem, err := syscall.Mmap(fds[0], 0, 16*16*4+20*20*4, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Munmap(mem)
	defer syscall.Close(fds[0])
	// (0,0) первого: {200,100,50,128} → премульт ≈ {100,50,25}, BGRA = 25,50,100,128
	px := mem[0:4]
	if absDiff(px[0], 25) > 1 || absDiff(px[1], 50) > 1 || absDiff(px[2], 100) > 1 || px[3] != 128 {
		t.Errorf("пиксель (0,0) = %v, ждал BGRA ≈ [25 50 100 128]", px)
	}
	// Последний пиксель первого — непрозрачный {10,20,30} → BGRA 30,20,10,255.
	last := mem[(16*16-1)*4 : 16*16*4]
	if last[0] != 30 || last[1] != 20 || last[2] != 10 || last[3] != 255 {
		t.Errorf("последний пиксель первого = %v", last)
	}
	// Второй (20×10 в 20×20): картинка по центру, верхние 5 строк прозрачны, белая
	// точка — в строке 5.
	second := mem[16*16*4:]
	if second[3] != 0 {
		t.Errorf("верхний левый угол квадрата не прозрачен: %v", second[:4])
	}
	if p := second[(5*20)*4 : (5*20)*4+4]; p[0] != 255 || p[3] != 255 {
		t.Errorf("белая точка не в центрированной позиции: %v", p)
	}
}

func bytesIndex(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return 0
}

func absDiff(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}

// Пустой список снимает значок: set_icon с пустым объектом, без пула.
func TestWayland_SetIcon_EmptyClears(t *testing.T) {
	c := newWlIconWindow(t)
	if err := c.w.SetIcon(nil); err != nil {
		t.Fatal(err)
	}
	raw, fds := readWithFDs(t, c)
	msgs := splitWlMsgs(t, raw)
	bind, _ := findWlMsg(msgs, testIconRegID, wlRegistryBind)
	mgr := bind.args[len(bind.args)-1]
	set, ok := findWlMsg(msgs, mgr, tIconSet)
	if !ok || set.args[0] != c.w.toplevelID || set.args[1] != 0 {
		t.Errorf("set_icon: %v %v, ждал пустой значок", set.args, ok)
	}
	if len(fds) != 0 {
		t.Error("для снятия значка создан пул")
	}
}

// Смена значка освобождает буферы и пул прежнего; менеджер привязывается один раз.
func TestWayland_SetIcon_ReplaceReleasesOld(t *testing.T) {
	c := newWlIconWindow(t)
	if err := c.w.SetIcon([]image.Image{iconPicture(16)}); err != nil {
		t.Fatal(err)
	}
	raw, fds := readWithFDs(t, c)
	for _, fd := range fds {
		syscall.Close(fd)
	}
	first := splitWlMsgs(t, raw)
	pool1, _ := findWlMsg(first, c.w.shmID, wlShmCreatePool)
	var buf1 uint32
	for _, m := range first {
		if m.obj == pool1.args[0] && m.opcode == wlShmPoolCreateBuffer {
			buf1 = m.args[0]
		}
	}

	if err := c.w.SetIcon([]image.Image{iconPicture(24)}); err != nil {
		t.Fatal(err)
	}
	raw, fds = readWithFDs(t, c)
	for _, fd := range fds {
		syscall.Close(fd)
	}
	second := splitWlMsgs(t, raw)
	if _, ok := findWlMsg(second, testIconRegID, wlRegistryBind); ok {
		t.Error("менеджер привязан повторно")
	}
	if _, ok := findWlMsg(second, buf1, wlBufferDestroy); !ok {
		t.Error("буфер прежнего значка не уничтожен")
	}
	if _, ok := findWlMsg(second, pool1.args[0], wlShmPoolDestroy); !ok {
		t.Error("пул прежнего значка не уничтожен")
	}
}

// До создания окна значок только запоминается и ставится после Create.
func TestWayland_SetIcon_PendingBeforeCreate(t *testing.T) {
	c := newWlIconWindow(t)
	toplevel := c.w.toplevelID
	c.w.toplevelID = 0
	if err := c.w.SetIcon([]image.Image{iconPicture(16)}); err != nil {
		t.Fatalf("до Create ждал nil, получил %v", err)
	}
	c.peer.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n > 0 {
		t.Fatalf("до Create отправлено %d байт", n)
	}

	c.w.toplevelID = toplevel
	c.w.applyPendingWlIcon()
	raw, fds := readWithFDs(t, c)
	for _, fd := range fds {
		syscall.Close(fd)
	}
	msgs := splitWlMsgs(t, raw)
	bind, ok := findWlMsg(msgs, testIconRegID, wlRegistryBind)
	if !ok {
		t.Fatal("отложенный значок не поставлен")
	}
	mgr := bind.args[len(bind.args)-1]
	if set, ok := findWlMsg(msgs, mgr, tIconSet); !ok || set.args[0] != toplevel {
		t.Errorf("set_icon: %v %v", set.args, ok)
	}
	// Повторное применение ничего не шлёт.
	c.w.applyPendingWlIcon()
	c.peer.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n > 0 {
		t.Error("отложенный значок поставлен дважды")
	}
}

// Без глобала отложенный значок молча пропадает: Create не должен падать.
func TestWayland_SetIcon_PendingWithoutGlobalIsDropped(t *testing.T) {
	c := newWlTestWindow(t)
	toplevel := c.w.toplevelID
	c.w.toplevelID = 0
	if err := c.w.SetIcon([]image.Image{iconPicture(16)}); err != nil {
		t.Fatal(err)
	}
	c.w.toplevelID = toplevel
	c.w.applyPendingWlIcon() // не паникует и ничего не шлёт
	c.peer.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n > 0 {
		t.Errorf("без глобала отправлено %d байт", n)
	}
}

// Глобал из реестра запоминается: без этого SetIcon всегда отвечал бы «не
// поддержано».
func TestWayland_RegistryRemembersIconManager(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.registryID = testIconRegID
	name := "xdg_toplevel_icon_manager_v1\x00"
	body := make([]byte, 4, 64)
	body[0] = 77 // имя глобала
	body = append(body, byte(len(name)), 0, 0, 0)
	body = append(body, name...)
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	body = append(body, 1, 0, 0, 0) // версия
	c.w.handleEvent(testIconRegID, wlRegistryEvGlobal, body)
	if c.w.gToplevelIcon != 77 {
		t.Errorf("gToplevelIcon = %d, ждал 77", c.w.gToplevelIcon)
	}
}

// Подготовка пикселей: одинаковые размеры не дублируются, пустые и слишком
// большие пропускаются.
func TestWlIconPixels_Filtering(t *testing.T) {
	sides, pix := wlIconPixels([]image.Image{
		iconPicture(16), iconPicture(16), nil,
		image.NewNRGBA(image.Rect(0, 0, 0, 0)),
		image.NewNRGBA(image.Rect(0, 0, wlIconMaxSide+1, 4)),
		iconPicture(32),
	})
	if len(sides) != 2 || sides[0] != 16 || sides[1] != 32 {
		t.Fatalf("размеры %v, ждал [16 32]", sides)
	}
	if len(pix) != (16*16+32*32)*4 {
		t.Errorf("пикселей %d байт", len(pix))
	}
	if s, p := wlIconPixels(nil); len(s) != 0 || len(p) != 0 {
		t.Error("пустой список дал данные")
	}
}
