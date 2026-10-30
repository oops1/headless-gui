//go:build linux && !android

package x11

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/clipimage"
	"github.com/oops1/headless-gui/v4/internal/dib"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Изображение в буфере обмена X11.
//
// Две оснастки. Для отдачи (мы владелец) хватает записи всего, что окно
// отправило серверу (x11Captured), — просителя играют сами тесты, подавая
// PropertyNotify. Для чтения (владелец чужой) нужен собеседник, который
// отвечает: fakeX играет и сервер, и владельца буфера по настоящему
// протоколу, а окно крутит обычный цикл событий.

const (
	atomPNGT  = 110 // image/png
	atomBMPT  = 111 // image/bmp
	atomINCRT = 112 // INCR
	atomATOMT = 4   // предопределённый ATOM
)

func newX11ImageWindow(t *testing.T) (*X11Window, *x11Captured) {
	w, capt := newX11TestWindow(t)
	w.atomImagePNG, w.atomImageBMP, w.atomIncr = atomPNGT, atomBMPT, atomINCRT
	w.atomClipProp = 108
	return w, capt
}

// snapshot режет записанное на запросы, не закрывая соединение; неполный
// хвост пропускает.
func (c *x11Captured) snapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out [][]byte
	b := c.data
	for len(b) >= 4 {
		size := int(binary.LittleEndian.Uint16(b[2:4])) * 4
		if size < 4 || size > len(b) {
			break
		}
		out = append(out, append([]byte(nil), b[:size]...))
		b = b[size:]
	}
	return out
}

// waitRequests ждёт, пока записанных запросов станет не меньше n.
func (c *x11Captured) waitRequests(t *testing.T, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if reqs := c.snapshot(); len(reqs) >= n {
			return reqs
		} else if time.Now().After(deadline) {
			t.Fatalf("запросов %d, ждал не меньше %d", len(reqs), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// propertyNotifyEv собирает событие PropertyNotify.
func propertyNotifyEv(window, atom uint32, state byte) []byte {
	b := make([]byte, 32)
	b[0] = 28
	binary.LittleEndian.PutUint32(b[4:], window)
	binary.LittleEndian.PutUint32(b[8:], atom)
	b[16] = state
	return b
}

// noisePicture — картинка, плохо сжимающаяся: PNG заведомо больше куска INCR.
func noisePicture(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rand.New(rand.NewSource(1)).Read(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

// requestsWith отбирает запросы с данным opcode.
func requestsWith(reqs [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, r := range reqs {
		if r[0] == opcode {
			out = append(out, r)
		}
	}
	return out
}

// propData достаёт тип и данные ChangeProperty (opcode 18).
func propData(r []byte) (typ uint32, format int, data []byte) {
	typ = binary.LittleEndian.Uint32(r[12:16])
	format = int(r[16])
	n := int(binary.LittleEndian.Uint32(r[20:24]))
	if format == 32 {
		n *= 4
	} else if format == 16 {
		n *= 2
	}
	return typ, format, r[24 : 24+n]
}

// ─── Отдача (мы владелец) ───────────────────────────────────────────────────

// В TARGETS у картинки — только сам TARGETS и image/png: текстовых целей нет, и
// наоборот, у текста нет image/png.
func TestX11Clipboard_ImageTargets(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	if err := c.SetImage(testClipPicture()); err != nil {
		t.Fatal(err)
	}
	c.handleRequest(selectionRequest(9, 100, 101, 200))
	typ, data, ok := changeProperty(capt.requests(t, w), 200)
	if !ok || typ != atomATOMT {
		t.Fatalf("TARGETS не отправлен: %v %v", typ, ok)
	}
	var got []uint32
	for i := 0; i+4 <= len(data); i += 4 {
		got = append(got, binary.LittleEndian.Uint32(data[i:]))
	}
	if len(got) != 2 || got[0] != 101 || got[1] != atomPNGT {
		t.Errorf("TARGETS картинки = %v, ждал [TARGETS image/png]", got)
	}

	w2, capt2 := newX11ImageWindow(t)
	c2 := newX11Clipboard(w2)
	c2.SetText("x")
	c2.handleRequest(selectionRequest(9, 100, 101, 200))
	_, data, _ = changeProperty(capt2.requests(t, w2), 200)
	if bytes.Contains(data, []byte{atomPNGT, 0, 0, 0}) {
		t.Error("image/png объявлен для простого текста")
	}
}

// Маленькая картинка уходит одним ChangeProperty с типом image/png; PNG
// кодируется на первый запрос, а не при SetImage.
func TestX11Clipboard_ImageServedSmall(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	src := testClipPicture()
	if err := c.SetImage(src); err != nil {
		t.Fatal(err)
	}
	if c.ownedImg.Encoded() {
		t.Fatal("PNG закодирован до запроса")
	}
	src.SetNRGBA(0, 0, color.NRGBA{1, 2, 3, 255}) // правка после копирования
	c.handleRequest(selectionRequest(9, 100, atomPNGT, 200))
	c.pending.Wait()

	reqs := capt.requests(t, w)
	typ, data, ok := changeProperty(reqs, 200)
	if !ok || typ != atomPNGT {
		t.Fatalf("ChangeProperty: тип %d, %v", typ, ok)
	}
	img, err := clipimage.DecodePNG(data)
	if err != nil {
		t.Fatalf("отдан не PNG: %v", err)
	}
	samePicture(t, img, testClipPicture())
	notify := requestsWith(reqs, 25)
	if len(notify) != 1 || binary.LittleEndian.Uint32(notify[0][12+20:12+24]) != 200 {
		t.Errorf("SelectionNotify: %v", notify)
	}
	if len(requestsWith(reqs, 2)) != 0 {
		t.Error("маленькая картинка пошла по INCR")
	}
}

// Текстовые цели у картинки — отказ (property = None).
func TestX11Clipboard_ImageRefusesText(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	c.SetImage(testClipPicture())
	c.handleRequest(selectionRequest(9, 100, 102, 200)) // UTF8_STRING
	c.handleRequest(selectionRequest(9, 100, 104, 201)) // text/html
	reqs := capt.requests(t, w)
	if _, _, ok := changeProperty(reqs, 200); ok {
		t.Error("на текст отдано содержимое")
	}
	notes := requestsWith(reqs, 25)
	if len(notes) != 2 {
		t.Fatalf("ответов %d", len(notes))
	}
	for _, n := range notes {
		if prop := binary.LittleEndian.Uint32(n[12+20 : 12+24]); prop != 0 {
			t.Errorf("property = %d, ждал отказ", prop)
		}
	}
	if s := c.GetText(); s != "" {
		t.Errorf("текст рядом с картинкой: %q", s)
	}
}

// Большая картинка идёт по INCR: подписка на окно просителя, свойство типа INCR
// с размером, SelectionNotify, затем по куску на каждое удаление и пустой
// кусок в конце; собранное — тот же PNG.
func TestX11Clipboard_ImageServedByINCR(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	src := noisePicture(300, 300)
	if err := c.SetImage(src); err != nil {
		t.Fatal(err)
	}
	c.handleRequest(selectionRequest(9, 100, atomPNGT, 200))
	c.pending.Wait()

	// Первый запрос — SetSelectionOwner от SetImage; дальше сам ответ.
	reqsAfter := func(n int) [][]byte { return capt.waitRequests(t, n+1)[1:] }
	reqs := reqsAfter(3)
	// 1. подписка на PropertyNotify просителя
	if r := reqs[0]; r[0] != 2 || binary.LittleEndian.Uint32(r[4:]) != 9 ||
		binary.LittleEndian.Uint32(r[8:]) != 0x800 || binary.LittleEndian.Uint32(r[12:]) != 0x400000 {
		t.Errorf("первый запрос не подписка на PropertyNotify: % x", reqs[0])
	}
	// 2. свойство INCR с оценкой размера
	typ, format, data := propData(reqs[1])
	if reqs[1][0] != 18 || typ != atomINCRT || format != 32 || len(data) != 4 {
		t.Fatalf("второй запрос не INCR: тип %d формат %d", typ, format)
	}
	total := int(binary.LittleEndian.Uint32(data))
	// 3. SelectionNotify
	if reqs[2][0] != 25 || binary.LittleEndian.Uint32(reqs[2][12+20:]) != 200 {
		t.Errorf("третий запрос не SelectionNotify: % x", reqs[2])
	}
	// до удаления свойства просителем ни одного куска
	if len(c.incrSends) != 1 {
		t.Fatalf("передач: %d", len(c.incrSends))
	}

	var got []byte
	n := 3
	for i := 0; ; i++ {
		if !c.handlePropertyNotify(propertyNotifyEv(9, 200, 1)) {
			t.Fatalf("Deleted #%d не принят", i)
		}
		n++
		reqs = reqsAfter(n)
		r := reqs[n-1]
		if r[0] != 18 {
			t.Fatalf("на Deleted #%d не кусок: opcode %d", i, r[0])
		}
		typ, format, chunk := propData(r)
		if typ != atomPNGT || format != 8 {
			t.Fatalf("кусок #%d: тип %d формат %d", i, typ, format)
		}
		if len(chunk) > native.X11IncrChunk {
			t.Fatalf("кусок #%d больше предела: %d", i, len(chunk))
		}
		if len(chunk) == 0 {
			break
		}
		got = append(got, chunk...)
		if i > 1000 {
			t.Fatal("передача не кончается")
		}
	}
	if len(got) != total {
		t.Errorf("собрано %d байт, обещано %d", len(got), total)
	}
	if total <= native.X11IncrChunk {
		t.Errorf("картинка не большая: %d", total)
	}
	img, err := clipimage.DecodePNG(got)
	if err != nil {
		t.Fatalf("собранное не PNG: %v", err)
	}
	samePicture(t, img, src)

	// По окончании подписка снята, а состояние забыто.
	reqs = reqsAfter(n + 1)
	if r := reqs[n]; r[0] != 2 || binary.LittleEndian.Uint32(r[12:]) != 0 {
		t.Errorf("подписка не снята: % x", r)
	}
	if len(c.incrSends) != 0 {
		t.Error("передача осталась в карте")
	}
	// Лишнее удаление уже ничего не шлёт.
	if c.handlePropertyNotify(propertyNotifyEv(9, 200, 1)) {
		t.Error("удаление после конца принято")
	}
}

// Брошенная просителем передача снимается по сроку, вместе с подпиской.
func TestX11Clipboard_INCRAbandonedIsDropped(t *testing.T) {
	old := x11IncrIdle
	x11IncrIdle = 40 * time.Millisecond
	defer func() { x11IncrIdle = old }()

	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	c.SetImage(noisePicture(300, 300))
	c.handleRequest(selectionRequest(9, 100, atomPNGT, 200))
	c.pending.Wait()
	capt.waitRequests(t, 4)

	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		left := len(c.incrSends)
		c.mu.Unlock()
		if left == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("брошенная передача не снята")
		}
		time.Sleep(5 * time.Millisecond)
	}
	reqs := capt.waitRequests(t, 5)
	if r := reqs[4]; r[0] != 2 || binary.LittleEndian.Uint32(r[12:]) != 0 {
		t.Errorf("подписка не снята: % x", r)
	}
}

// Две передачи на одно окно: подписку снимает последняя.
func TestX11Clipboard_INCRTwoOnOneWindow(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	c.SetImage(noisePicture(300, 300))
	c.handleRequest(selectionRequest(9, 100, atomPNGT, 200))
	c.handleRequest(selectionRequest(9, 100, atomPNGT, 201))
	c.pending.Wait()
	capt.waitRequests(t, 7)

	finish := func(prop uint32) {
		for i := 0; i < 1000; i++ {
			c.handlePropertyNotify(propertyNotifyEv(9, prop, 1))
			c.mu.Lock()
			_, alive := c.incrSends[x11IncrKey{9, prop}]
			c.mu.Unlock()
			if !alive {
				return
			}
		}
		t.Fatal("передача не кончается")
	}
	finish(200)
	time.Sleep(20 * time.Millisecond)
	for _, r := range requestsWith(capt.snapshot(), 2) {
		if binary.LittleEndian.Uint32(r[12:]) == 0 {
			t.Fatal("подписка снята, пока вторая передача идёт")
		}
	}
	finish(201)
	last := false
	deadline := time.Now().Add(2 * time.Second)
	for !last && time.Now().Before(deadline) {
		for _, r := range requestsWith(capt.snapshot(), 2) {
			if binary.LittleEndian.Uint32(r[12:]) == 0 {
				last = true
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !last {
		t.Error("подписка не снята после последней передачи")
	}
}

// Просителем не может быть наше же окно: подписка затёрла бы его маску событий.
func TestX11Clipboard_INCRRefusesOwnWindow(t *testing.T) {
	w, capt := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	c.SetImage(noisePicture(300, 300))
	c.handleRequest(selectionRequest(w.wid, 100, atomPNGT, 200))
	c.pending.Wait()
	reqs := capt.requests(t, w)
	if len(requestsWith(reqs, 2)) != 0 {
		t.Error("подписали собственное окно")
	}
	notes := requestsWith(reqs, 25)
	if len(notes) != 1 || binary.LittleEndian.Uint32(notes[0][12+20:12+24]) != 0 {
		t.Errorf("ждал отказ, пришло %v", notes)
	}
}

// Пока владеем — картинка из своей копии; текст, потеря владения и SelectionClear
// её вытесняют.
func TestX11Clipboard_GetImageOwned(t *testing.T) {
	w, _ := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	if err := c.SetImage(testClipPicture()); err != nil {
		t.Fatal(err)
	}
	got, ok := c.GetImage()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, got, testClipPicture())
	if _, ok := c.GetHTML(); ok {
		t.Error("HTML рядом с картинкой")
	}
	if _, _, ok := c.GetFiles(); ok {
		t.Error("файлы рядом с картинкой")
	}

	c.SetText("x")
	if _, ok := c.GetImage(); ok {
		t.Error("после SetText осталась картинка")
	}
	c.SetImage(testClipPicture())
	c.SetFiles([]string{"/a"}, false)
	if c.ownedImg != nil {
		t.Error("SetFiles не вытеснил картинку")
	}
	c.SetImage(testClipPicture())
	clr := make([]byte, 32)
	binary.LittleEndian.PutUint32(clr[12:16], 100)
	c.handleClear(clr)
	if c.ownedImg != nil || c.owns {
		t.Error("SelectionClear не снял владение картинкой")
	}
}

func TestX11Clipboard_SetImageErrors(t *testing.T) {
	w, _ := newX11ImageWindow(t)
	c := newX11Clipboard(w)
	if err := c.SetImage(nil); err != base.ErrClipboardImageEmpty {
		t.Errorf("nil: %v", err)
	}
	w.atomImagePNG = 0
	if err := c.SetImage(testClipPicture()); err != base.ErrClipboardImageUnsupported {
		t.Errorf("без атома image/png: %v", err)
	}
}

// ─── Чтение (владелец чужой): сервер и владелец на проводе ──────────────────

type fakeProp struct {
	typ    uint32
	format int
	data   []byte
}

// fakeX — сервер X11 и владелец буфера в одном лице.
type fakeX struct {
	t   *testing.T
	srv net.Conn
	out chan []byte

	mu       sync.Mutex
	props    map[uint32]fakeProp // свойства нашего окна (wid = 1)
	converts []uint32            // какие цели у владельца спрашивали
	// onConvert отвечает на ConvertSelection; nil — владелец молчит.
	onConvert func(f *fakeX, target, prop uint32)
	// incr — куски, которые владелец отдаёт по одному на каждое удаление
	// свойства incrProp (протокол INCR).
	incr     [][]byte
	incrProp uint32
	incrType uint32
	incrGap  time.Duration // пауза перед сообщением о каждом куске
}

func newFakeX(t *testing.T) (*X11Window, *x11Clipboard, *fakeX) {
	t.Helper()
	client, server := net.Pipe()
	w := &X11Window{
		conn: client, wid: 1,
		atomClipboard: 100, atomTargets: 101, atomUTF8String: 102, atomText: 103, atomTextHTML: 104,
		atomImagePNG: atomPNGT, atomImageBMP: atomBMPT, atomIncr: atomINCRT, atomClipProp: 108,
	}
	c := newX11Clipboard(w)
	w.clip = c
	f := &fakeX{t: t, srv: server, out: make(chan []byte, 4096), props: map[uint32]fakeProp{}}
	go f.writer()
	go f.reader()
	go w.RunEventLoop()
	t.Cleanup(func() {
		w.closed.Store(true)
		client.Close()
		server.Close()
	})
	return w, c, f
}

func (f *fakeX) writer() {
	for b := range f.out {
		if _, err := f.srv.Write(b); err != nil {
			return
		}
	}
}

func (f *fakeX) reader() {
	for {
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(f.srv, hdr); err != nil {
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
		body := make([]byte, n-4)
		if _, err := io.ReadFull(f.srv, body); err != nil {
			return
		}
		req := append(hdr, body...)
		switch req[0] {
		case 24: // ConvertSelection: requestor, selection, target, property, time
			target := binary.LittleEndian.Uint32(req[12:16])
			prop := binary.LittleEndian.Uint32(req[16:20])
			f.mu.Lock()
			f.converts = append(f.converts, target)
			cb := f.onConvert
			f.mu.Unlock()
			if cb != nil {
				cb(f, target, prop)
			} else {
				f.selectionNotify(target, 0) // отказ
			}
		case 20: // GetProperty: window, property
			prop := binary.LittleEndian.Uint32(req[8:12])
			del := req[1] != 0
			f.mu.Lock()
			p, ok := f.props[prop]
			if del {
				delete(f.props, prop)
			}
			f.mu.Unlock()
			f.reply(p, ok)
			if del && ok {
				f.nextIncrChunk(prop)
			}
		}
	}
}

func (f *fakeX) reply(p fakeProp, ok bool) {
	data := p.data
	pad := (4 - len(data)%4) % 4
	r := make([]byte, 32+len(data)+pad)
	r[0] = 1
	if ok {
		r[1] = byte(p.format)
		binary.LittleEndian.PutUint32(r[4:], uint32((len(data)+pad)/4))
		binary.LittleEndian.PutUint32(r[8:], p.typ)
		nitems := len(data)
		if p.format == 32 {
			nitems /= 4
		}
		binary.LittleEndian.PutUint32(r[16:], uint32(nitems))
		copy(r[32:], data)
	}
	f.out <- r
}

// nextIncrChunk кладёт следующий кусок INCR после удаления свойства.
func (f *fakeX) nextIncrChunk(prop uint32) {
	f.mu.Lock()
	if prop != f.incrProp || len(f.incr) == 0 {
		f.mu.Unlock()
		return
	}
	chunk := f.incr[0]
	f.incr = f.incr[1:]
	f.props[prop] = fakeProp{typ: f.incrType, format: 8, data: chunk}
	gap := f.incrGap
	f.mu.Unlock()
	if gap > 0 {
		time.AfterFunc(gap, func() { f.out <- propertyNotifyEv(1, prop, 0) })
		return
	}
	f.out <- propertyNotifyEv(1, prop, 0)
}

func (f *fakeX) selectionNotify(target, prop uint32) {
	b := make([]byte, 32)
	b[0] = 31
	binary.LittleEndian.PutUint32(b[8:], 1)    // requestor
	binary.LittleEndian.PutUint32(b[12:], 100) // selection = CLIPBOARD
	binary.LittleEndian.PutUint32(b[16:], target)
	binary.LittleEndian.PutUint32(b[20:], prop)
	f.out <- b
}

// answer кладёт данные в свойство, как это делает владелец, и сообщает об этом
// (сначала PropertyNotify от записи свойства, затем SelectionNotify).
func (f *fakeX) answer(target, prop, typ uint32, format int, data []byte) {
	f.mu.Lock()
	f.props[prop] = fakeProp{typ: typ, format: format, data: data}
	f.mu.Unlock()
	f.out <- propertyNotifyEv(1, prop, 0)
	f.selectionNotify(target, prop)
}

// answerINCR отвечает передачей INCR: сначала свойство-маркер с размером, затем
// куски по одному на каждое удаление. endMarker добавляет пустой кусок в конце.
func (f *fakeX) answerINCR(target, prop, typ uint32, chunks [][]byte, endMarker bool) {
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	f.mu.Lock()
	f.incr = append([][]byte(nil), chunks...)
	if endMarker {
		f.incr = append(f.incr, nil)
	}
	f.incrProp, f.incrType = prop, typ
	f.mu.Unlock()
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, uint32(total))
	f.answer(target, prop, atomINCRT, 32, sz)
}

func (f *fakeX) asked() []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint32(nil), f.converts...)
}

// ownerWith играет владельца с набором целей: TARGETS отдаётся списком, цели
// из data — данными (если значение — кусочки, то по INCR).
func ownerWith(targets []uint32, data map[uint32][]byte) func(*fakeX, uint32, uint32) {
	return func(f *fakeX, target, prop uint32) {
		if target == 101 {
			f.answer(target, prop, atomATOMT, 32, atomsBytes(targets...))
			return
		}
		if d, ok := data[target]; ok {
			f.answer(target, prop, target, 8, d)
			return
		}
		f.selectionNotify(target, 0)
	}
}

// Чужой PNG: спрашивается TARGETS, затем image/png; альфа цела.
func TestX11Clipboard_GetImageForeignPNG(t *testing.T) {
	_, c, f := newFakeX(t)
	png, _ := clipimage.EncodePNG(testClipPicture())
	f.onConvert = ownerWith([]uint32{101, 102, atomBMPT, atomPNGT}, map[uint32][]byte{atomPNGT: png, atomBMPT: []byte("не нужен")})
	img, ok := c.GetImage()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, img, testClipPicture())
	if got := f.asked(); len(got) != 2 || got[0] != 101 || got[1] != atomPNGT {
		t.Errorf("спрошено %v, ждал [TARGETS image/png]", got)
	}
}

// Только BMP: берётся он, DIB с альфой разбирается.
func TestX11Clipboard_GetImageForeignBMPFallback(t *testing.T) {
	_, c, f := newFakeX(t)
	f.onConvert = ownerWith([]uint32{101, atomBMPT}, map[uint32][]byte{atomBMPT: dib.EncodeBMP(testClipPicture())})
	img, ok := c.GetImage()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, img, testClipPicture())
	if got := f.asked(); len(got) != 2 || got[1] != atomBMPT {
		t.Errorf("спрошено %v", got)
	}
}

// В буфере текст: после TARGETS больше вопросов нет.
func TestX11Clipboard_GetImageForeignTextOnly(t *testing.T) {
	_, c, f := newFakeX(t)
	f.onConvert = ownerWith([]uint32{101, 102, 31}, map[uint32][]byte{102: []byte("текст")})
	start := time.Now()
	if _, ok := c.GetImage(); ok {
		t.Error("текст принят за картинку")
	}
	if got := f.asked(); len(got) != 1 || got[0] != 101 {
		t.Errorf("спрошено %v, ждал один TARGETS", got)
	}
	if time.Since(start) > x11ClipboardTimeout/2 {
		t.Error("ждал молчания, хотя владелец ответил")
	}
}

// Владелец, не отвечающий на TARGETS отказом, пробуется вслепую: PNG, затем BMP.
func TestX11Clipboard_GetImageForeignBlindWhenNoTargets(t *testing.T) {
	_, c, f := newFakeX(t)
	png, _ := clipimage.EncodePNG(testClipPicture())
	f.onConvert = func(f *fakeX, target, prop uint32) {
		if target == atomPNGT {
			f.answer(target, prop, target, 8, png)
			return
		}
		f.selectionNotify(target, 0)
	}
	img, ok := c.GetImage()
	if !ok {
		t.Fatal("GetImage вслепую: нет")
	}
	samePicture(t, img, testClipPicture())
	if got := f.asked(); len(got) != 2 || got[0] != 101 || got[1] != atomPNGT {
		t.Errorf("спрошено %v", got)
	}
}

// Молчащий владелец: срок — 300 мс, не больше.
func TestX11Clipboard_GetImageSilentOwner(t *testing.T) {
	_, c, f := newFakeX(t)
	f.onConvert = func(*fakeX, uint32, uint32) {} // молчит
	start := time.Now()
	if _, ok := c.GetImage(); ok {
		t.Error("молчание принято за картинку")
	}
	if d := time.Since(start); d < x11ClipboardTimeout/2 || d > 3*x11ClipboardTimeout {
		t.Errorf("ожидание заняло %v, ждал около %v", d, x11ClipboardTimeout)
	}
}

// Большая картинка от владельца по INCR собирается из кусков.
func TestX11Clipboard_GetImageForeignINCR(t *testing.T) {
	_, c, f := newFakeX(t)
	src := noisePicture(200, 200)
	png, _ := clipimage.EncodePNG(src)
	third := len(png) / 3
	chunks := [][]byte{png[:third], png[third : 2*third], png[2*third:]}
	f.onConvert = func(f *fakeX, target, prop uint32) {
		if target == 101 {
			f.answer(target, prop, atomATOMT, 32, atomsBytes(101, atomPNGT))
			return
		}
		f.answerINCR(target, prop, atomPNGT, chunks, true)
	}
	img, ok := c.GetImage()
	if !ok {
		t.Fatal("GetImage по INCR: нет")
	}
	samePicture(t, img, src)
}

// Срок INCR отсчитывается на каждый кусок: передача дольше 300 мс суммарно, но
// без пауз длиннее срока, успевает.
func TestX11Clipboard_GetImageINCRSlowButAlive(t *testing.T) {
	_, c, f := newFakeX(t)
	png, _ := clipimage.EncodePNG(testClipPicture())
	q := len(png) / 4
	f.incrGap = x11ClipboardTimeout * 2 / 3
	f.onConvert = func(f *fakeX, target, prop uint32) {
		if target == 101 {
			f.answer(target, prop, atomATOMT, 32, atomsBytes(101, atomPNGT))
			return
		}
		f.answerINCR(target, prop, atomPNGT, [][]byte{png[:q], png[q : 2*q], png[2*q : 3*q], png[3*q:]}, true)
	}
	start := time.Now()
	img, ok := c.GetImage()
	if !ok {
		t.Fatal("медленный, но живой владелец не дождался")
	}
	if time.Since(start) < x11ClipboardTimeout {
		t.Error("тест не проверил суммарную длительность дольше срока")
	}
	samePicture(t, img, testClipPicture())
}

// Оборвавшаяся на полпути передача (маркера конца нет) держит вызов не дольше
// срока на кусок.
func TestX11Clipboard_GetImageINCRStalled(t *testing.T) {
	_, c, f := newFakeX(t)
	png, _ := clipimage.EncodePNG(testClipPicture())
	f.onConvert = func(f *fakeX, target, prop uint32) {
		if target == 101 {
			f.answer(target, prop, atomATOMT, 32, atomsBytes(101, atomPNGT))
			return
		}
		f.answerINCR(target, prop, atomPNGT, [][]byte{png[:len(png)/2], png[len(png)/2:]}, false)
	}
	// Конец передачи не помечен: владелец замолкает после последнего куска.
	start := time.Now()
	if _, ok := c.GetImage(); ok {
		t.Error("передача без маркера конца принята")
	}
	if d := time.Since(start); d > 3*x11ClipboardTimeout {
		t.Errorf("оборванная передача держала вызов %v", d)
	}
}

// Предел размера: куски сверх предела — отказ, а не бесконечное накопление.
func TestX11Clipboard_INCRLimit(t *testing.T) {
	_, c, f := newFakeX(t)
	f.onConvert = func(f *fakeX, target, prop uint32) {
		f.answerINCR(target, prop, 102, [][]byte{make([]byte, 60), make([]byte, 60), make([]byte, 60)}, true)
	}
	if got := c.convertBytes(102, time.Second, 100); got != nil {
		t.Errorf("сверх предела отдано %d байт", len(got))
	}
	// В пределах — всё на месте.
	f.onConvert = func(f *fakeX, target, prop uint32) {
		f.answerINCR(target, prop, 102, [][]byte{[]byte("ab"), []byte("cd")}, true)
	}
	if got := c.convertBytes(102, time.Second, 100); string(got) != "abcd" {
		t.Errorf("INCR текста: %q", got)
	}
}

// Обычная вставка текста не сломана: ответ без INCR, и событие PropertyNotify
// от записи свойства владельцем не принимается за кусок.
func TestX11Clipboard_GetTextStillWorksWithPropertyEvents(t *testing.T) {
	_, c, f := newFakeX(t)
	f.onConvert = ownerWith([]uint32{101, 102}, map[uint32][]byte{102: []byte("привет\n")})
	if got := c.GetText(); got != "привет\n" {
		t.Errorf("GetText = %q", got)
	}
}
