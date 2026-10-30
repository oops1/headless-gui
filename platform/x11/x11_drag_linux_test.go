//go:build linux && !android

package x11

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// v3.34: перетаскивание файлов на X11 — приёмник (события) и источник XDND.
// «X-сервер» — тест на другом конце net.Pipe: читает запросы, на захват
// указателя и клавиатуры отвечает успехом (или отказом).

const (
	tAtomTargets  = 101
	tAtomEnter    = 200
	tAtomPosition = 201
	tAtomStatus   = 202
	tAtomLeave    = 203
	tAtomDrop     = 204
	tAtomFinished = 205
	tAtomSel      = 206
	tAtomCopy     = 208
	tAtomUriList  = 209
	tAtomMove     = 210
	tAtomLink     = 211
	tAtomBegin    = 212
	tAtomAware    = 213
)

// x11Fake — поддельный X-сервер: записывает запросы и отвечает на захваты.
type x11Fake struct {
	mu         sync.Mutex
	reqs       [][]byte
	grabStatus byte // ответ на GrabPointer: 0 — успех
	// reply — свой ответ на запрос (nil — не отвечать); для запросов с ответом.
	reply func(req []byte) []byte
}

func newX11DragWindow(t *testing.T) (*X11Window, *x11Fake) {
	t.Helper()
	client, server := net.Pipe()
	f := &x11Fake{}
	go func() {
		for {
			hdr := make([]byte, 4)
			if _, err := readAll(server, hdr); err != nil {
				return
			}
			n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
			if n < 4 {
				return
			}
			req := make([]byte, n)
			copy(req, hdr)
			if n > 4 {
				if _, err := readAll(server, req[4:]); err != nil {
					return
				}
			}
			f.mu.Lock()
			f.reqs = append(f.reqs, req)
			status, custom := f.grabStatus, f.reply
			f.mu.Unlock()
			if custom != nil {
				if rep := custom(req); rep != nil {
					server.Write(rep)
					continue
				}
			}
			switch req[0] {
			case 26: // GrabPointer
				rep := make([]byte, 32)
				rep[0], rep[1] = 1, status
				server.Write(rep)
			case 31: // GrabKeyboard
				rep := make([]byte, 32)
				rep[0] = 1
				server.Write(rep)
			}
		}
	}()
	w := &X11Window{
		conn: client, wid: 1, rootWin: 2,
		atomTargets: tAtomTargets, atomXdndEnter: tAtomEnter, atomXdndPosition: tAtomPosition,
		atomXdndStatus: tAtomStatus, atomXdndLeave: tAtomLeave, atomXdndDrop: tAtomDrop,
		atomXdndFinished: tAtomFinished, atomXdndSelection: tAtomSel, atomXdndActionCopy: tAtomCopy,
		atomTextUriList: tAtomUriList, atomXdndActionMove: tAtomMove, atomXdndActionLink: tAtomLink,
		atomDragBegin: tAtomBegin, atomXdndAware: tAtomAware,
	}
	w.posX, w.posY, w.posCached = 100, 50, true
	t.Cleanup(func() { client.Close(); server.Close() })
	return w, f
}

func readAll(c net.Conn, b []byte) (int, error) {
	got := 0
	for got < len(b) {
		n, err := c.Read(b[got:])
		got += n
		if err != nil {
			return got, err
		}
	}
	return got, nil
}

// find ждёт запрос, удовлетворяющий pred (сервер дописывает его чуть позже,
// чем клиентская Write возвращает управление).
func (f *x11Fake) find(pred func([]byte) bool) []byte {
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		for _, r := range f.reqs {
			if pred(r) {
				f.mu.Unlock()
				return r
			}
		}
		f.mu.Unlock()
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// settle даёт серверу дочитать всё отправленное.
func (f *x11Fake) settle() { time.Sleep(30 * time.Millisecond) }

func (f *x11Fake) count(pred func([]byte) bool) int {
	f.settle()
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if pred(r) {
			n++
		}
	}
	return n
}

// clientMsg — SendEvent с ClientMessage типа atom; pred по получателю.
func clientMsg(atom, dest uint32) func([]byte) bool {
	return func(r []byte) bool {
		return r[0] == 25 && len(r) >= 44 && binary.LittleEndian.Uint32(r[4:8]) == dest &&
			r[12] == 33 && binary.LittleEndian.Uint32(r[20:24]) == atom
	}
}

// clientData — data.l[i] события из SendEvent.
func clientData(r []byte, i int) uint32 { return binary.LittleEndian.Uint32(r[24+i*4 : 28+i*4]) }

func opcode(op byte) func([]byte) bool { return func(r []byte) bool { return r[0] == op } }

// cm собирает входящее ClientMessage (32 байта).
func cm(atom uint32, d [5]uint32) []byte {
	b := make([]byte, 32)
	b[0], b[1] = 33, 32
	binary.LittleEndian.PutUint32(b[8:12], atom)
	for i, v := range d {
		binary.LittleEndian.PutUint32(b[12+i*4:], v)
	}
	return b
}

// ─── Приёмник ───────────────────────────────────────────────────────────────

func TestX11_DragTargetEvents(t *testing.T) {
	w, f := newX11DragWindow(t)
	var log []string
	accept := true
	w.SetOnDragEnter(func(x, y int) { log = append(log, "enter", itoa(x), itoa(y)) })
	w.SetOnDragOver(func(x, y int) bool { log = append(log, "over"); return accept })
	w.SetOnDragLeave(func() { log = append(log, "leave") })

	const src = 77
	w.handleX11Event(cm(tAtomEnter, [5]uint32{src, 5 << 24, tAtomUriList}))
	if len(log) != 0 {
		t.Fatalf("enter сообщён приложению до первой позиции: %v", log)
	}
	// Корневая (150, 90) → клиентская (50, 40) при окне в (100, 50).
	w.handleX11Event(cm(tAtomPosition, [5]uint32{src, 0, 150<<16 | 90, 0, tAtomCopy}))
	accept = false
	w.handleX11Event(cm(tAtomPosition, [5]uint32{src, 0, 160<<16 | 90}))
	w.handleX11Event(cm(tAtomLeave, [5]uint32{src}))

	want := []string{"enter", "50", "40", "over", "over", "leave"}
	if len(log) != len(want) {
		t.Fatalf("события %v, ждал %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("события %v, ждал %v", log, want)
		}
	}
	// Статусы: принято, затем отказ.
	f.settle()
	f.mu.Lock()
	var flags []uint32
	for _, r := range f.reqs {
		if clientMsg(tAtomStatus, src)(r) {
			flags = append(flags, clientData(r, 1)&1)
		}
	}
	f.mu.Unlock()
	if len(flags) != 2 || flags[0] != 1 || flags[1] != 0 {
		t.Errorf("XdndStatus accept: %v, ждал [1 0]", flags)
	}
}

// Перетаскивание без text/uri-list приложению не показывается и не принимается.
func TestX11_DragTargetIgnoresNonFiles(t *testing.T) {
	w, f := newX11DragWindow(t)
	n := 0
	w.SetOnDragEnter(func(x, y int) { n++ })
	w.SetOnDragOver(func(x, y int) bool { n++; return true })
	w.SetOnDragLeave(func() { n++ })
	const src = 77
	w.handleX11Event(cm(tAtomEnter, [5]uint32{src, 5 << 24, 999}))
	w.handleX11Event(cm(tAtomPosition, [5]uint32{src, 0, 150<<16 | 90}))
	w.handleX11Event(cm(tAtomLeave, [5]uint32{src}))
	if n != 0 {
		t.Errorf("приложение получило %d событий для не-файлов", n)
	}
	st := f.find(clientMsg(tAtomStatus, src))
	if st == nil || clientData(st, 1)&1 != 0 {
		t.Error("для не-файлов нужен отказ в XdndStatus")
	}
}

// После сброса leave приложению не приходит; отказавшее приложение данных не получает.
func TestX11_DragTargetDropNoLeave(t *testing.T) {
	w, f := newX11DragWindow(t)
	var log []string
	w.SetOnDragEnter(func(x, y int) { log = append(log, "enter") })
	w.SetOnDragLeave(func() { log = append(log, "leave") })
	w.SetOnFilesDropped(func([]string, int, int) {})
	const src = 77
	w.handleX11Event(cm(tAtomEnter, [5]uint32{src, 5 << 24, tAtomUriList}))
	w.handleX11Event(cm(tAtomPosition, [5]uint32{src, 0, 150<<16 | 90}))
	w.handleX11Event(cm(tAtomDrop, [5]uint32{src, 0, 123}))
	w.handleX11Event(cm(tAtomLeave, [5]uint32{src}))
	if len(log) != 1 || log[0] != "enter" {
		t.Errorf("события %v: после drop leave приходить не должен", log)
	}
	if f.find(opcode(24)) == nil { // ConvertSelection: данные запрошены
		t.Error("данные не запрошены после Drop")
	}

	// Приложение отказало в последней точке: Drop данных не запрашивает.
	w2, f2 := newX11DragWindow(t)
	w2.SetOnDragOver(func(x, y int) bool { return false })
	w2.SetOnFilesDropped(func([]string, int, int) {})
	w2.handleX11Event(cm(tAtomEnter, [5]uint32{src, 5 << 24, tAtomUriList}))
	w2.handleX11Event(cm(tAtomPosition, [5]uint32{src, 0, 150<<16 | 90}))
	w2.handleX11Event(cm(tAtomDrop, [5]uint32{src, 0, 123}))
	if f2.count(opcode(24)) != 0 {
		t.Error("данные запрошены, хотя приложение отказало")
	}
	if fin := f2.find(clientMsg(tAtomFinished, src)); fin == nil || clientData(fin, 1)&1 != 0 {
		t.Error("ждал XdndFinished с отказом")
	}
}

// ─── Источник ───────────────────────────────────────────────────────────────

// motion собирает MotionNotify с корневыми координатами.
func motion(rx, ry int, state uint16) []byte {
	b := make([]byte, 32)
	b[0] = 6
	binary.LittleEndian.PutUint32(b[4:8], 4242)
	binary.LittleEndian.PutUint16(b[20:22], uint16(int16(rx)))
	binary.LittleEndian.PutUint16(b[22:24], uint16(int16(ry)))
	binary.LittleEndian.PutUint16(b[28:30], state)
	return b
}

func buttonEv(op byte, btn byte) []byte {
	b := make([]byte, 32)
	b[0], b[1] = op, btn
	binary.LittleEndian.PutUint32(b[4:8], 5555)
	return b
}

// startDrag доводит источник до состояния «ведём»: кнопка зажата, захват взят.
func startDrag(t *testing.T, w *X11Window, f *x11Fake, effect native.DragEffect, win uint32, ver int) *[]native.DragEffect {
	t.Helper()
	got := new([]native.DragEffect)
	w.btnDown.Store(1)
	w.drag.find = func(rx, ry int) (uint32, int) { return win, ver }
	if !w.BeginDragFiles([]string{"/tmp/a b.png", "/tmp/ф.jpg"}, effect, func(e native.DragEffect) { *got = append(*got, e) }) {
		t.Fatal("BeginDragFiles отказал при зажатой кнопке")
	}
	if f.find(clientMsg(tAtomBegin, 1)) == nil {
		t.Fatal("служебное сообщение насосу не отправлено")
	}
	w.handleX11Event(cm(tAtomBegin, [5]uint32{}))
	if !w.drag.active.Load() {
		t.Fatal("перетаскивание не началось после служебного сообщения")
	}
	return got
}

func TestX11_BeginDragFiles_Refusals(t *testing.T) {
	w, _ := newX11DragWindow(t)
	noop := func(native.DragEffect) {}
	if w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, noop) {
		t.Error("принято без зажатой кнопки")
	}
	w.btnDown.Store(1)
	if w.BeginDragFiles(nil, native.DragEffectCopy, noop) || w.BeginDragFiles([]string{"/a"}, 0, noop) ||
		w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, nil) {
		t.Error("принят пустой список / пустая маска / нет done")
	}
	if !w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, noop) {
		t.Fatal("корректный запрос отвергнут")
	}
	if w.BeginDragFiles([]string{"/b"}, native.DragEffectCopy, noop) {
		t.Error("второе перетаскивание принято, пока идёт первое")
	}
}

func TestX11_DragSource_FullDrop(t *testing.T) {
	w, f := newX11DragWindow(t)
	released := false
	w.SetOnMouseButton(func(x, y, id int, pressed bool) {
		if id == 0 && !pressed {
			released = true
		}
	})
	got := startDrag(t, w, f, native.DragEffectCopy|native.DragEffectMove, 50, 5)

	if r := f.find(opcode(22)); r == nil || binary.LittleEndian.Uint32(r[8:12]) != tAtomSel || binary.LittleEndian.Uint32(r[4:8]) != 1 {
		t.Error("окно не объявлено владельцем XdndSelection")
	}
	if f.find(opcode(26)) == nil || f.find(opcode(31)) == nil {
		t.Error("указатель/клавиатура не захвачены")
	}

	// Первое движение: Enter и Position.
	w.handleX11Event(motion(300, 200, 0))
	enter := f.find(clientMsg(tAtomEnter, 50))
	if enter == nil {
		t.Fatal("XdndEnter не отправлен")
	}
	if clientData(enter, 0) != 1 || clientData(enter, 1)>>24 != 5 || clientData(enter, 2) != tAtomUriList {
		t.Errorf("XdndEnter: источник=%d версия=%d тип=%d", clientData(enter, 0), clientData(enter, 1)>>24, clientData(enter, 2))
	}
	pos := f.find(clientMsg(tAtomPosition, 50))
	if pos == nil || clientData(pos, 2) != 300<<16|200 || clientData(pos, 4) != tAtomCopy || clientData(pos, 3) != 4242 {
		t.Fatalf("XdndPosition: %v", pos)
	}

	// Пока нет XdndStatus, новые позиции не шлём — одна ждёт очереди.
	w.handleX11Event(motion(310, 210, 0))
	w.handleX11Event(motion(320, 220, 4)) // с Ctrl; последняя запоминается
	if n := f.count(clientMsg(tAtomPosition, 50)); n != 1 {
		t.Errorf("до XdndStatus отправлено позиций: %d, ждал 1", n)
	}
	w.handleX11Event(cm(tAtomStatus, [5]uint32{50, 1, 0, 0, tAtomCopy}))
	f.settle()
	if n := f.count(clientMsg(tAtomPosition, 50)); n != 2 {
		t.Fatalf("после XdndStatus позиций: %d, ждал 2 (отложенная)", n)
	}

	// Отпускание над принявшим окном — Drop, захваты сняты.
	w.handleX11Event(buttonEv(5, 1))
	drop := f.find(clientMsg(tAtomDrop, 50))
	if drop == nil || clientData(drop, 2) != 5555 {
		t.Fatalf("XdndDrop: %v", drop)
	}
	if f.find(opcode(27)) == nil || f.find(opcode(32)) == nil {
		t.Error("захваты не сняты")
	}
	if len(*got) != 0 {
		t.Errorf("done вызван до XdndFinished: %v", *got)
	}

	// Приёмник просит данные.
	req := func(target uint32) []byte {
		b := make([]byte, 32)
		b[0] = 30
		binary.LittleEndian.PutUint32(b[4:8], 777)  // time
		binary.LittleEndian.PutUint32(b[12:16], 50) // requestor
		binary.LittleEndian.PutUint32(b[16:20], tAtomSel)
		binary.LittleEndian.PutUint32(b[20:24], target)
		binary.LittleEndian.PutUint32(b[24:28], 888) // property
		return b
	}
	w.handleX11Event(req(tAtomUriList))
	cp := f.find(func(r []byte) bool { return r[0] == 18 && len(r) > 24 }) // ChangeProperty
	if cp == nil {
		t.Fatal("данные не записаны в свойство запросчика")
	}
	body := "file:///tmp/a%20b.png\r\nfile:///tmp/%D1%84.jpg\r\n"
	if binary.LittleEndian.Uint32(cp[4:8]) != 50 || binary.LittleEndian.Uint32(cp[8:12]) != 888 ||
		binary.LittleEndian.Uint32(cp[12:16]) != tAtomUriList {
		t.Errorf("ChangeProperty адресован неверно: % x", cp[:20])
	}
	if n := int(binary.LittleEndian.Uint32(cp[20:24])); n != len(body) || string(cp[24:24+n]) != body {
		t.Errorf("тело списка: %q", cp[24:])
	}
	if sn := f.find(func(r []byte) bool { return r[0] == 25 && len(r) >= 44 && r[12] == 31 }); sn == nil ||
		binary.LittleEndian.Uint32(sn[32:36]) != 888 {
		t.Error("SelectionNotify с property не отправлен")
	}
	// TARGETS — типы, которые отдаём; чужой тип — отказ (property None).
	w.handleX11Event(req(tAtomTargets))
	w.handleX11Event(req(9999))
	f.settle()
	f.mu.Lock()
	var notifies [][]byte
	for _, r := range f.reqs {
		if r[0] == 25 && len(r) >= 44 && r[12] == 31 {
			notifies = append(notifies, r)
		}
	}
	f.mu.Unlock()
	if len(notifies) != 3 || binary.LittleEndian.Uint32(notifies[2][32:36]) != 0 {
		t.Errorf("SelectionNotify: %d шт., на чужой тип должен быть property=None", len(notifies))
	}

	// XdndFinished v5: принято, действие «переместить».
	w.handleX11Event(cm(tAtomFinished, [5]uint32{50, 1, tAtomMove}))
	if len(*got) != 1 || (*got)[0] != native.DragEffectMove {
		t.Errorf("done: %v, ждал [move]", *got)
	}
	if w.drag.active.Load() {
		t.Error("перетаскивание не завершено")
	}
	if !released {
		t.Error("кнопка в движке не отпущена")
	}
	// Владение XdndSelection снято (owner = None).
	if f.count(func(r []byte) bool {
		return r[0] == 22 && binary.LittleEndian.Uint32(r[4:8]) == 0 && binary.LittleEndian.Uint32(r[8:12]) == tAtomSel
	}) != 1 {
		t.Error("владение XdndSelection не снято")
	}
}

func TestX11_DragSource_RejectedAndEsc(t *testing.T) {
	// Приёмник отказал в XdndStatus: при отпускании — Leave, а не Drop.
	w, f := newX11DragWindow(t)
	got := startDrag(t, w, f, native.DragEffectCopy, 50, 5)
	w.handleX11Event(motion(10, 10, 0))
	w.handleX11Event(cm(tAtomStatus, [5]uint32{50, 0, 0, 0, 0}))
	w.handleX11Event(buttonEv(5, 1))
	if f.find(clientMsg(tAtomLeave, 50)) == nil {
		t.Error("XdndLeave не отправлен")
	}
	if f.count(clientMsg(tAtomDrop, 50)) != 0 {
		t.Error("XdndDrop отправлен, хотя приёмник не принял")
	}
	if len(*got) != 1 || (*got)[0] != 0 {
		t.Errorf("done: %v, ждал [0]", *got)
	}

	// Esc отменяет: Leave и итог 0. Код 9 — Escape.
	w, f = newX11DragWindow(t)
	got = startDrag(t, w, f, native.DragEffectCopy, 50, 5)
	w.handleX11Event(motion(10, 10, 0))
	esc := make([]byte, 32)
	esc[0], esc[1] = 2, 9
	w.handleX11Event(esc)
	if f.find(clientMsg(tAtomLeave, 50)) == nil || len(*got) != 1 || (*got)[0] != 0 {
		t.Errorf("Esc: Leave=%v done=%v", f.find(clientMsg(tAtomLeave, 50)) != nil, *got)
	}
	if w.drag.active.Load() {
		t.Error("после Esc перетаскивание всё ещё активно")
	}
}

func TestX11_DragSource_TargetChangeAndNoTarget(t *testing.T) {
	w, f := newX11DragWindow(t)
	cur := uint32(50)
	got := startDrag(t, w, f, native.DragEffectCopy, 50, 5)
	w.drag.find = func(rx, ry int) (uint32, int) { return cur, 5 }
	w.handleX11Event(motion(1, 1, 0))
	cur = 60
	w.handleX11Event(motion(2, 2, 0))
	if f.find(clientMsg(tAtomLeave, 50)) == nil || f.find(clientMsg(tAtomEnter, 60)) == nil {
		t.Error("смена окна: ждал XdndLeave старому и XdndEnter новому")
	}
	cur = 0
	w.handleX11Event(motion(3, 3, 0))
	if f.count(clientMsg(tAtomLeave, 60)) != 1 {
		t.Error("уход с окна без приёмника: ждал XdndLeave")
	}
	w.handleX11Event(buttonEv(5, 1)) // отпускание над пустотой
	if len(*got) != 1 || (*got)[0] != 0 {
		t.Errorf("done: %v, ждал [0]", *got)
	}
}

func TestX11_DragSource_GrabRefused(t *testing.T) {
	w, f := newX11DragWindow(t)
	f.grabStatus = 1 // AlreadyGrabbed
	var got []native.DragEffect
	w.btnDown.Store(1)
	w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, func(e native.DragEffect) { got = append(got, e) })
	w.handleX11Event(cm(tAtomBegin, [5]uint32{}))
	if len(got) != 1 || got[0] != 0 || w.drag.active.Load() {
		t.Errorf("захват отклонён: done=%v active=%v", got, w.drag.active.Load())
	}
	// После отказа можно начать заново.
	f.mu.Lock()
	f.grabStatus = 0
	f.mu.Unlock()
	w.btnDown.Store(1)
	if !w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, func(native.DragEffect) {}) {
		t.Error("новое перетаскивание не началось после отказа")
	}
}

// Приёмник, не приславший XdndFinished, не держит источник вечно.
func TestX11_DragSource_FinishTimeout(t *testing.T) {
	old := x11DragFinishTimeout
	x11DragFinishTimeout = 50 * time.Millisecond
	defer func() { x11DragFinishTimeout = old }()

	w, f := newX11DragWindow(t)
	var mu sync.Mutex
	var got []native.DragEffect
	w.btnDown.Store(1)
	w.drag.find = func(rx, ry int) (uint32, int) { return 50, 5 }
	w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, func(e native.DragEffect) { mu.Lock(); got = append(got, e); mu.Unlock() })
	w.handleX11Event(cm(tAtomBegin, [5]uint32{}))
	w.handleX11Event(motion(1, 1, 0))
	w.handleX11Event(cm(tAtomStatus, [5]uint32{50, 1, 0, 0, tAtomCopy}))
	w.handleX11Event(buttonEv(5, 1))
	if f.find(clientMsg(tAtomDrop, 50)) == nil {
		t.Fatal("Drop не отправлен")
	}
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("done после тайм-аута: %v, ждал [0]", got)
	}
}

// Приёмник XDND старше v2 XdndFinished не шлёт: итог — согласованное действие.
func TestX11_DragSource_OldTargetNoFinished(t *testing.T) {
	w, f := newX11DragWindow(t)
	got := startDrag(t, w, f, native.DragEffectCopy|native.DragEffectMove, 50, 5)
	w.drag.find = func(rx, ry int) (uint32, int) { return 50, 1 } // целевая версия 1
	w.handleX11Event(motion(1, 1, 0))
	w.drag.mu.Lock()
	w.drag.targetVer = 1
	w.drag.mu.Unlock()
	w.handleX11Event(cm(tAtomStatus, [5]uint32{50, 1, 0, 0, tAtomMove}))
	w.handleX11Event(buttonEv(5, 1))
	if f.find(clientMsg(tAtomDrop, 50)) == nil {
		t.Fatal("Drop не отправлен")
	}
	if len(*got) != 1 || (*got)[0] != native.DragEffectMove {
		t.Errorf("done: %v, ждал [move]", *got)
	}
}

// Маска кнопок ведётся из обычных событий мыши — по ней BeginDragFiles
// решает, можно ли начинать.
func TestX11_ButtonEventsFeedDragGate(t *testing.T) {
	w, _ := newX11DragWindow(t)
	noop := func(native.DragEffect) {}
	w.handleX11Event(buttonEv(4, 1)) // ButtonPress, левая
	if !w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, noop) {
		t.Fatal("после нажатия кнопки перетаскивание не принято")
	}
	w.dragFinish(0)
	w.handleX11Event(buttonEv(5, 1)) // ButtonRelease
	if w.BeginDragFiles([]string{"/a"}, native.DragEffectCopy, noop) {
		t.Error("перетаскивание принято после отпускания кнопки")
	}
}

func TestNoteButtonMask(t *testing.T) {
	w := &X11Window{}
	w.noteButton(1, true)
	w.noteButton(3, true)
	w.noteButton(4, true) // колесо в маску не идёт
	if got := w.btnDown.Load(); got != 0b101 {
		t.Errorf("маска %b, ждал 101", got)
	}
	w.noteButton(1, false)
	if got := w.btnDown.Load(); got != 0b100 {
		t.Errorf("маска %b, ждал 100", got)
	}
}

// Поиск приёмника под курсором: спуск от корня по TranslateCoordinates, окно
// с XdndAware — первое сверху (у WM с рамкой это окно клиента, а не рамка).
func TestX11_FindDndTarget(t *testing.T) {
	w, f := newX11DragWindow(t)
	// Цепочка: корень (2) → рамка (10) → клиент (11); XdndAware (v5) только у клиента.
	chain := map[uint32]uint32{2: 10, 10: 11}
	aware := uint32(11)
	f.reply = func(req []byte) []byte {
		rep := make([]byte, 32)
		rep[0] = 1
		switch req[0] {
		case 40: // TranslateCoordinates(src=root, dst=cur): потомок cur под точкой
			dst := binary.LittleEndian.Uint32(req[8:12])
			binary.LittleEndian.PutUint32(rep[8:12], chain[dst])
			return rep
		case 20: // GetProperty
			win := binary.LittleEndian.Uint32(req[4:8])
			if win != aware {
				return rep // свойства нет: type = None
			}
			rep[1] = 32
			binary.LittleEndian.PutUint32(rep[4:8], 1)  // длина значения в словах
			binary.LittleEndian.PutUint32(rep[8:12], 4) // ATOM
			binary.LittleEndian.PutUint32(rep[16:20], 1)
			return append(rep, 5, 0, 0, 0)
		}
		return nil
	}
	if win, ver := w.x11FindDndTarget(10, 20); win != 11 || ver != 5 {
		t.Errorf("приёмник (%d, v%d), ждал (11, v5)", win, ver)
	}
	aware = 0 // никто не объявил XdndAware
	if win, _ := w.x11FindDndTarget(10, 20); win != 0 {
		t.Errorf("найден приёмник %d без XdndAware", win)
	}
	chain = map[uint32]uint32{} // под точкой пусто
	aware = 11
	if win, _ := w.x11FindDndTarget(10, 20); win != 0 {
		t.Errorf("найден приёмник %d над пустотой", win)
	}
}
