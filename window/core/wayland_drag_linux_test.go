//go:build linux && !android

package core

import (
	"encoding/binary"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// v3.34: перетаскивание файлов на Wayland — события приёмника и источник.
// «Композитор» — тест на другом конце настоящего unix-сокета.

// enterBody — тело wl_data_device.enter: serial, surface, x, y (fixed 24.8), offer.
func enterBody(serial, surface uint32, x, y int, offer uint32) []byte {
	b := make([]byte, 20)
	binary.LittleEndian.PutUint32(b[0:4], serial)
	binary.LittleEndian.PutUint32(b[4:8], surface)
	binary.LittleEndian.PutUint32(b[8:12], uint32(int32(x<<8)))
	binary.LittleEndian.PutUint32(b[12:16], uint32(int32(y<<8)))
	binary.LittleEndian.PutUint32(b[16:20], offer)
	return b
}

// motionBody — тело wl_data_device.motion: time, x, y.
func motionBody(x, y int) []byte {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[4:8], uint32(int32(x<<8)))
	binary.LittleEndian.PutUint32(b[8:12], uint32(int32(y<<8)))
	return b
}

// dragSetup готовит окно с data_device (v3) и offer, предлагающим файлы.
func dragSetup(t *testing.T) (*wlTestConn, uint32) {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.dataDevVersion = 3
	const offer = 44
	c.w.offerSet(offer, true) // text/uri-list предложен
	return c, offer
}

// wlAcceptMime разбирает accept: serial и тип ("" — отказ).
func wlAcceptMime(t *testing.T, m wlWireMsg) (uint32, string) {
	t.Helper()
	if len(m.args) < 2 {
		t.Fatalf("короткий accept: %v", m.args)
	}
	if m.args[1] == 0 {
		return m.args[0], ""
	}
	raw := make([]byte, 0, 4*len(m.args))
	for _, a := range m.args[2:] {
		raw = binary.LittleEndian.AppendUint32(raw, a)
	}
	return m.args[0], string(raw[:m.args[1]-1])
}

func TestWayland_DragTargetEnterMotionLeave(t *testing.T) {
	c, offer := dragSetup(t)
	var log []string
	accept := true
	c.w.SetOnDragEnter(func(x, y int) { log = append(log, "enter") })
	c.w.SetOnDragOver(func(x, y int) bool {
		log = append(log, "over", itoa(x), itoa(y))
		return accept
	})
	c.w.SetOnDragLeave(func() { log = append(log, "leave") })

	c.w.handleDataDevice(wlDataDeviceEvEnter, enterBody(9, 11, 100, 50, offer))
	msgs := splitWlMsgs(t, c.read(t))
	acc, ok := findWlMsg(msgs, offer, wlDataOfferAccept)
	if !ok {
		t.Fatalf("accept не отправлен: %+v", msgs)
	}
	if serial, mime := wlAcceptMime(t, acc); serial != 9 || mime != native.MimeTextURIList {
		t.Errorf("accept(serial=%d, %q), ждал (9, text/uri-list)", serial, mime)
	}
	if _, ok := findWlMsg(msgs, offer, wlDataOfferSetActions); !ok {
		t.Error("set_actions не отправлен (v3)")
	}

	// Движение с тем же ответом: компоновщика не дёргаем.
	c.w.handleDataDevice(wlDataDeviceEvMotion, motionBody(110, 60))
	c.peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if n, err := c.peer.Read(make([]byte, 64)); err == nil && n > 0 {
		t.Errorf("повторный accept при том же ответе (%d байт)", n)
	}

	// Приложение отказало — accept с null-строкой.
	accept = false
	c.w.handleDataDevice(wlDataDeviceEvMotion, motionBody(300, 60))
	msgs = splitWlMsgs(t, c.read(t))
	acc, ok = findWlMsg(msgs, offer, wlDataOfferAccept)
	if !ok {
		t.Fatalf("отказ не отправлен: %+v", msgs)
	}
	if _, mime := wlAcceptMime(t, acc); mime != "" {
		t.Errorf("при отказе accept с типом %q", mime)
	}

	c.w.handleDataDevice(wlDataDeviceEvLeave, nil)
	want := []string{"enter", "over", "100", "50", "over", "110", "60", "over", "300", "60", "leave"}
	if !reflect.DeepEqual(log, want) {
		t.Errorf("события %v, ждал %v", log, want)
	}
	msgs = splitWlMsgs(t, c.read(t))
	if _, ok := findWlMsg(msgs, offer, wlDataOfferDestroy); !ok {
		t.Error("offer не уничтожен после leave")
	}
}

// Перетаскивание без файлов (текст): приложению ничего, компоновщику — отказ.
func TestWayland_DragTargetNonFilesIgnored(t *testing.T) {
	c, _ := dragSetup(t)
	const textOffer = 45
	c.w.offerSetText(textOffer) // text/plain, без uri-list
	n := 0
	c.w.SetOnDragEnter(func(x, y int) { n++ })
	c.w.SetOnDragOver(func(x, y int) bool { n++; return true })
	c.w.SetOnDragLeave(func() { n++ })

	c.w.handleDataDevice(wlDataDeviceEvEnter, enterBody(3, 11, 1, 1, textOffer))
	c.w.handleDataDevice(wlDataDeviceEvMotion, motionBody(2, 2))
	c.w.handleDataDevice(wlDataDeviceEvLeave, nil)
	if n != 0 {
		t.Errorf("приложение получило %d событий для не-файлов", n)
	}
	msgs := splitWlMsgs(t, c.read(t))
	acc, ok := findWlMsg(msgs, textOffer, wlDataOfferAccept)
	if !ok {
		t.Fatal("для текста должен уйти отказ")
	}
	if _, mime := wlAcceptMime(t, acc); mime != "" {
		t.Errorf("текст принят как %q", mime)
	}
}

// Без колбэков файлы принимаются везде — как до v3.34.
func TestWayland_DragTargetAcceptsWithoutCallbacks(t *testing.T) {
	c, offer := dragSetup(t)
	c.w.handleDataDevice(wlDataDeviceEvEnter, enterBody(5, 11, 1, 1, offer))
	msgs := splitWlMsgs(t, c.read(t))
	acc, ok := findWlMsg(msgs, offer, wlDataOfferAccept)
	if !ok {
		t.Fatal("accept не отправлен")
	}
	if _, mime := wlAcceptMime(t, acc); mime != native.MimeTextURIList {
		t.Errorf("accept %q", mime)
	}
}

// Сброс: данные читаются из канала, приложению — пути, leave после drop не шлётся.
func TestWayland_DragTargetDrop(t *testing.T) {
	c, offer := dragSetup(t)
	var log []string
	dropped := make(chan []string, 1)
	c.w.SetOnDragEnter(func(x, y int) { log = append(log, "enter") })
	c.w.SetOnDragLeave(func() { log = append(log, "leave") })
	c.w.SetOnFilesDropped(func(p []string, x, y int) { dropped <- p })

	c.w.handleDataDevice(wlDataDeviceEvEnter, enterBody(5, 11, 40, 30, offer))
	c.read(t)
	c.w.handleDataDevice(wlDataDeviceEvDrop, nil)
	c.w.handleDataDevice(wlDataDeviceEvLeave, nil) // компоновщик шлёт leave и после drop

	// receive(mime, fd): забираем fd и пишем в него список файлов.
	fd := recvFD(t, c)
	f := os.NewFile(uintptr(fd), "pipe")
	io.WriteString(f, "file:///tmp/a.png\r\nfile:///tmp/%D1%84.jpg\r\n")
	f.Close()

	select {
	case p := <-dropped:
		if want := []string{"/tmp/a.png", "/tmp/ф.jpg"}; !reflect.DeepEqual(p, want) {
			t.Errorf("сброшено %q, ждал %q", p, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetOnFilesDropped не вызван")
	}
	if !reflect.DeepEqual(log, []string{"enter"}) {
		t.Errorf("события %v: после drop leave приходить не должен", log)
	}
}

// Сброс, на который приложение ответило отказом, данных не читает.
func TestWayland_DragTargetDropRejectedNotReceived(t *testing.T) {
	c, offer := dragSetup(t)
	c.w.SetOnDragOver(func(x, y int) bool { return false })
	called := false
	c.w.SetOnFilesDropped(func(p []string, x, y int) { called = true })

	c.w.handleDataDevice(wlDataDeviceEvEnter, enterBody(5, 11, 1, 1, offer))
	c.w.handleDataDevice(wlDataDeviceEvDrop, nil)
	msgs := splitWlMsgs(t, c.read(t))
	if _, ok := findWlMsg(msgs, offer, wlDataOfferReceive); ok {
		t.Error("данные запрошены, хотя приложение отказало")
	}
	if _, ok := findWlMsg(msgs, offer, wlDataOfferDestroy); !ok {
		t.Error("offer не уничтожен")
	}
	if called {
		t.Error("SetOnFilesDropped вызван при отказе")
	}
}

// recvFD забирает fd, пришедший вместе с запросом (SCM_RIGHTS).
func recvFD(t *testing.T, c *wlTestConn) int {
	t.Helper()
	buf := make([]byte, 256)
	oob := make([]byte, unix.CmsgSpace(4))
	c.peer.SetReadDeadline(time.Now().Add(time.Second))
	_, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
	if err != nil {
		t.Fatalf("ничего не пришло: %v", err)
	}
	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) == 0 {
		t.Fatalf("fd не передан: %v", err)
	}
	fds, err := unix.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) == 0 {
		t.Fatalf("fd не разобран: %v", err)
	}
	return fds[0]
}

// ─── Источник ────────────────────────────────────────────────────────────────

// dragSourceSetup готовит окно к BeginDragFiles: кнопка зажата, serial есть.
func dragSourceSetup(t *testing.T) *wlTestConn {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.dataDevVersion = 3
	c.w.ptrButtonSerial.Store(55)
	c.w.ptrButtons.Store(1)
	return c
}

func TestWayland_BeginDragFiles_Wire(t *testing.T) {
	c := dragSourceSetup(t)
	released := false
	c.w.SetOnMouseButton(func(x, y, id int, pressed bool) {
		if id == 0 && !pressed {
			released = true
		}
	})
	var got []native.DragEffect
	if !c.w.BeginDragFiles([]string{"/tmp/a b.png"}, native.DragEffectCopy|native.DragEffectMove|native.DragEffectLink, func(e native.DragEffect) { got = append(got, e) }) {
		t.Fatal("BeginDragFiles отказал")
	}
	msgs := splitWlMsgs(t, c.read(t))

	create, ok := findWlMsg(msgs, 30, wlDataDevMgrCreateDataSource)
	if !ok || len(create.args) != 1 {
		t.Fatalf("create_data_source не отправлен: %+v", msgs)
	}
	src := create.args[0]
	if offer, ok := findWlMsg(msgs, src, wlDataSourceOffer); !ok {
		t.Error("offer(text/uri-list) не отправлен")
	} else if len(offer.args) < 2 {
		t.Errorf("offer: %v", offer.args)
	}
	acts, ok := findWlMsg(msgs, src, wlDataSourceSetActions)
	if !ok || len(acts.args) != 1 || acts.args[0] != wlDndActionCopy|wlDndActionMove {
		t.Errorf("set_actions: %+v (ждал copy|move, ссылка в протоколе отсутствует)", acts)
	}
	start, ok := findWlMsg(msgs, 31, wlDataDeviceStartDrag)
	if !ok {
		t.Fatalf("start_drag не отправлен: %+v", msgs)
	}
	// source, origin (поверхность окна), icon (нет), serial нажатия.
	if want := []uint32{src, 11, 0, 55}; !reflect.DeepEqual(start.args, want) {
		t.Errorf("start_drag %v, ждал %v", start.args, want)
	}
	// Порядок: создан и наполнен до start_drag.
	idx := func(obj uint32, op uint16) int {
		for i, m := range msgs {
			if m.obj == obj && m.opcode == op {
				return i
			}
		}
		return -1
	}
	if !(idx(30, wlDataDevMgrCreateDataSource) < idx(src, wlDataSourceOffer) && idx(src, wlDataSourceOffer) < idx(31, wlDataDeviceStartDrag)) {
		t.Error("нарушен порядок create → offer → start_drag")
	}
	if !released {
		t.Error("кнопка в движке не отпущена: компоновщик забрал нажатие")
	}
	if len(got) != 0 {
		t.Errorf("done вызван до конца перетаскивания: %v", got)
	}

	// Одно перетаскивание за раз.
	if c.w.BeginDragFiles([]string{"/tmp/x"}, native.DragEffectCopy, func(native.DragEffect) {}) {
		t.Error("второе перетаскивание принято, пока идёт первое")
	}
}

func TestWayland_BeginDragFiles_Refusals(t *testing.T) {
	noop := func(native.DragEffect) {}
	files := []string{"/tmp/a"}
	cases := []struct {
		name string
		mut  func(w *WaylandWindow)
		eff  native.DragEffect
		path []string
	}{
		{"нет serial'а нажатия", func(w *WaylandWindow) { w.ptrButtonSerial.Store(0) }, native.DragEffectCopy, files},
		{"кнопка не зажата", func(w *WaylandWindow) { w.ptrButtons.Store(0) }, native.DragEffectCopy, files},
		{"нет wl_data_device", func(w *WaylandWindow) { w.dataDeviceID = 0 }, native.DragEffectCopy, files},
		{"менеджер старше v3", func(w *WaylandWindow) { w.dataDevVersion = 2 }, native.DragEffectCopy, files},
		{"указатель на попапе", func(w *WaylandWindow) { w.ptrOnPopup = true }, native.DragEffectCopy, files},
		{"только ссылка: в протоколе её нет", func(w *WaylandWindow) {}, native.DragEffectLink, files},
		{"пустой список", func(w *WaylandWindow) {}, native.DragEffectCopy, nil},
	}
	for _, tc := range cases {
		c := dragSourceSetup(t)
		tc.mut(c.w)
		if c.w.BeginDragFiles(tc.path, tc.eff, noop) {
			t.Errorf("%s: перетаскивание начато", tc.name)
		}
		c.peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		if n, err := c.peer.Read(make([]byte, 64)); err == nil && n > 0 {
			t.Errorf("%s: на провод ушло %d байт", tc.name, n)
		}
	}
}

// Данные отдаются лениво, при send, и именно запрошенным типом.
func TestWayland_DragSourceSendWritesURIList(t *testing.T) {
	c := dragSourceSetup(t)
	c.w.BeginDragFiles([]string{"/tmp/a b.png", "/tmp/ф.jpg"}, native.DragEffectCopy, func(native.DragEffect) {})
	c.read(t)
	src := c.w.drag.id

	r, wr, _ := os.Pipe()
	c.w.dragHandleSend(native.MimeTextURIList, int(wr.Fd()))
	// dragHandleSend забрал fd как свой — файл wr закрывает он.
	data := make(chan string, 1)
	go func() { b, _ := io.ReadAll(r); data <- string(b) }()
	select {
	case s := <-data:
		want := "file:///tmp/a%20b.png\r\nfile:///tmp/%D1%84.jpg\r\n"
		if s != want {
			t.Errorf("отдано %q, ждал %q", s, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("данные не записаны в канал")
	}
	r.Close()

	// Чужой тип — пустой ответ (канал закрывается).
	r2, wr2, _ := os.Pipe()
	c.w.dragHandleSend("image/png", int(wr2.Fd()))
	if b, _ := io.ReadAll(r2); len(b) != 0 {
		t.Errorf("на чужой тип отдано %q", b)
	}
	r2.Close()
	if c.w.drag.id != src {
		t.Error("send не должен завершать перетаскивание")
	}
}

// События источника: итоговое действие и отмена доходят до done.
func TestWayland_DragSourceFinishAndCancel(t *testing.T) {
	actionBody := func(a uint32) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, a)
		return b
	}
	run := func(t *testing.T, events func(c *wlTestConn, src uint32)) (native.DragEffect, int, *wlTestConn, uint32) {
		c := dragSourceSetup(t)
		calls, last := 0, native.DragEffect(99)
		c.w.BeginDragFiles([]string{"/tmp/a"}, native.DragEffectCopy|native.DragEffectMove, func(e native.DragEffect) { calls++; last = e })
		c.read(t)
		src := c.w.drag.id
		events(c, src)
		return last, calls, c, src
	}

	t.Run("копирование", func(t *testing.T) {
		got, calls, c, src := run(t, func(c *wlTestConn, src uint32) {
			c.w.handleEvent(src, wlDataSourceEvDndDropPerformed, nil)
			c.w.handleEvent(src, wlDataSourceEvAction, actionBody(wlDndActionCopy))
			c.w.handleEvent(src, wlDataSourceEvDndFinished, nil)
		})
		if got != native.DragEffectCopy || calls != 1 {
			t.Errorf("done(%v) × %d, ждал copy × 1", got, calls)
		}
		if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), src, wlDataSourceDestroy); !ok {
			t.Error("источник не уничтожен")
		}
		if c.w.drag.id != 0 {
			t.Error("состояние источника не сброшено")
		}
	})
	t.Run("перемещение", func(t *testing.T) {
		got, _, _, _ := run(t, func(c *wlTestConn, src uint32) {
			c.w.handleEvent(src, wlDataSourceEvAction, actionBody(wlDndActionMove))
			c.w.handleEvent(src, wlDataSourceEvDndFinished, nil)
		})
		if got != native.DragEffectMove {
			t.Errorf("done(%v), ждал move", got)
		}
	})
	t.Run("отмена", func(t *testing.T) {
		got, calls, c, src := run(t, func(c *wlTestConn, src uint32) {
			c.w.handleEvent(src, wlDataSourceEvCancelled, nil)
			c.w.handleEvent(src, wlDataSourceEvCancelled, nil) // повтор безвреден
		})
		if got != 0 || calls != 1 {
			t.Errorf("done(%v) × %d, ждал 0 × 1", got, calls)
		}
		if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), src, wlDataSourceDestroy); !ok {
			t.Error("источник не уничтожен")
		}
	})
	t.Run("после конца можно начать новое", func(t *testing.T) {
		_, _, c, _ := run(t, func(c *wlTestConn, src uint32) {
			c.w.handleEvent(src, wlDataSourceEvCancelled, nil)
		})
		c.read(t)
		c.w.ptrButtons.Store(1) // новое нажатие: прежнее компоновщик забрал себе
		if !c.w.BeginDragFiles([]string{"/tmp/b"}, native.DragEffectCopy, func(native.DragEffect) {}) {
			t.Error("новое перетаскивание не началось после конца прежнего")
		}
	})
}
