//go:build linux && !android

package x11

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func newX11FormatsFake(t *testing.T) (*x11Clipboard, *fakeX) {
	t.Helper()
	w, c, f := newFakeX(t)
	w.atomTextUriList, w.atomGnomeCopied = 105, 106
	// Договариваться с XFixes фальшивый сервер не умеет: пока не проверяем
	// подписку, живём по сроку (так же, как на сервере без XFixes).
	c.fmts.watch.Store(x11FixesUnavailable)
	return c, f
}

// Один вопрос — TARGETS; данные файлов не запрашиваются.
func TestX11Clipboard_FormatsAsksOnlyTargets(t *testing.T) {
	c, f := newX11FormatsFake(t)
	f.onConvert = ownerWith([]uint32{101, 102, 31, 105, 106}, map[uint32][]byte{105: []byte("file:///a\r\n")})
	if got := c.ClipboardFormats(); got != fmtsFiles|fmtsText {
		t.Errorf("ClipboardFormats = %v", got)
	}
	if asked := f.asked(); len(asked) != 1 || asked[0] != 101 {
		t.Errorf("спрошено %v, ждал один TARGETS", asked)
	}
}

// Текст и картинка — файлов нет.
func TestX11Clipboard_FormatsKinds(t *testing.T) {
	c, f := newX11FormatsFake(t)
	f.onConvert = ownerWith([]uint32{101, 102, 104}, nil)
	if got := c.ClipboardFormats(); got != fmtsText|fmtsHTML {
		t.Errorf("текст и HTML: %v", got)
	}
	c.fmts.invalidate()
	f.onConvert = ownerWith([]uint32{101, atomPNGT}, nil)
	if got := c.ClipboardFormats(); got != fmtsImage {
		t.Errorf("картинка: %v", got)
	}
	c.fmts.invalidate()
	f.onConvert = ownerWith([]uint32{101, 102}, nil)
	if got := c.ClipboardFormats(); got != fmtsText {
		t.Errorf("текст: %v", got)
	}
}

// Ответ запоминается: повторный вопрос идёт из памяти, смена владельца
// (SelectionClear, XFixes) его сбрасывает.
func TestX11Clipboard_FormatsCached(t *testing.T) {
	c, f := newX11FormatsFake(t)
	f.onConvert = ownerWith([]uint32{101, 105}, nil)
	for i := 0; i < 5; i++ {
		if got := c.ClipboardFormats(); got != fmtsFiles {
			t.Fatalf("вызов %d: %v", i, got)
		}
	}
	if n := len(f.asked()); n != 1 {
		t.Errorf("владельца спросили %d раз, ждал один", n)
	}

	c.fmts.invalidate() // сменился владелец
	f.onConvert = ownerWith([]uint32{101, 102}, nil)
	if got := c.ClipboardFormats(); got != fmtsText {
		t.Errorf("после смены владельца: %v", got)
	}
	if n := len(f.asked()); n != 2 {
		t.Errorf("после смены владельца спрошено %d раз, ждал два", n)
	}

	// Без XFixes память не вечная: по истечении срока спросим снова.
	c.fmts.mu.Lock()
	c.fmts.at = time.Now().Add(-time.Minute)
	c.fmts.mu.Unlock()
	f.onConvert = ownerWith([]uint32{101, 105}, nil)
	if got := c.ClipboardFormats(); got != fmtsFiles {
		t.Errorf("после срока: %v", got)
	}
}

// Молчащий владелец стоит не больше срока одного вопроса, а повторный вопрос
// об одном и том же владельце уже мгновенный: вызов при каждом изменении
// буфера не платит срок каждый раз.
func TestX11Clipboard_FormatsSilentOwner(t *testing.T) {
	c, f := newX11FormatsFake(t)
	f.onConvert = func(*fakeX, uint32, uint32) {} // молчит

	start := time.Now()
	if got := c.ClipboardFormats(); got != 0 {
		t.Errorf("молчащий владелец: %v", got)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("вопрос занял %v", d)
	}
	start = time.Now()
	for i := 0; i < 20; i++ {
		c.ClipboardFormats()
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("повторные вопросы заняли %v: молчание не запомнено", d)
	}
	if n := len(f.asked()); n != 1 {
		t.Errorf("молчащего спросили %d раз", n)
	}
}

// Владелец, который уже допрашивается (например, читают список файлов на 10
// минут), не получает второго вопроса: вызов возвращается сразу с последним
// известным ответом.
func TestX11Clipboard_FormatsDoesNotWaitBehindBusyOwner(t *testing.T) {
	c, f := newX11FormatsFake(t)
	f.onConvert = ownerWith([]uint32{101, 105}, nil)
	if got := c.ClipboardFormats(); got != fmtsFiles {
		t.Fatalf("первый вопрос: %v", got)
	}

	// Запрос чтения файлов «в пути» и память устарела.
	c.mu.Lock()
	c.wait = &x11Wait{ch: make(chan []byte, 1), progress: make(chan struct{}, 1)}
	c.mu.Unlock()
	c.fmts.mu.Lock()
	c.fmts.at = time.Now().Add(-time.Minute)
	c.fmts.mu.Unlock()
	before := len(f.asked())

	start := time.Now()
	if got := c.ClipboardFormats(); got != fmtsFiles {
		t.Errorf("при занятом владельце: %v, ждал последний известный ответ", got)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("вызов ждал занятого владельца")
	}
	if len(f.asked()) != before {
		t.Error("второй запрос ушёл владельцу")
	}

	// И без известного ответа — пусто, но тоже сразу.
	c.fmts.invalidate()
	if got := c.ClipboardFormats(); got != 0 {
		t.Errorf("без известного ответа: %v", got)
	}
}

// Свой буфер: из своей копии, владельца не спрашиваем.
func TestX11Clipboard_FormatsOwned(t *testing.T) {
	c, f := newX11FormatsFake(t)
	c.SetFiles([]string{"/a"}, false)
	if got := c.ClipboardFormats(); got != fmtsFiles|fmtsText {
		t.Errorf("свои файлы: %v", got)
	}
	c.SetText("x")
	if got := c.ClipboardFormats(); got != fmtsText {
		t.Errorf("свой текст: %v", got)
	}
	if len(f.asked()) != 0 {
		t.Error("свой буфер ходил к владельцу")
	}
}

func TestX11FormatsFromTargets(t *testing.T) {
	a := x11FormatAtoms{text: []uint32{102, 31, 103}, html: 104, png: 110, bmp: 111, uriList: 105, gnome: 106}
	if got := x11FormatsFromTargets(atomsBytes(101, 102, 105), a); got != fmtsText|fmtsFiles {
		t.Errorf("текст и файлы: %v", got)
	}
	if got := x11FormatsFromTargets(atomsBytes(106), a); got != fmtsFiles {
		t.Errorf("gnome: %v", got)
	}
	if got := x11FormatsFromTargets(atomsBytes(104, 111), a); got != fmtsHTML|fmtsImage {
		t.Errorf("HTML и BMP: %v", got)
	}
	if got := x11FormatsFromTargets(atomsBytes(999, 0), a); got != 0 {
		t.Errorf("чужие цели: %v", got)
	}
	// Атом, которого у окна нет (0), не совпадает с нулевой целью.
	if got := x11FormatsFromTargets(atomsBytes(0, 0), x11FormatAtoms{}); got != 0 {
		t.Errorf("нулевые атомы: %v", got)
	}
	if got := x11FormatsFromTargets([]byte{1, 2, 3}, a); got != 0 {
		t.Errorf("обрубок: %v", got)
	}
}

func TestX11FixesWire(t *testing.T) {
	r := x11FixesSelectSelectionInputRequest(130, 7, 100, 5)
	if len(r) != 16 || r[0] != 130 || r[1] != 2 || binary.LittleEndian.Uint16(r[2:]) != 4 ||
		binary.LittleEndian.Uint32(r[4:]) != 7 || binary.LittleEndian.Uint32(r[8:]) != 100 ||
		binary.LittleEndian.Uint32(r[12:]) != 5 {
		t.Errorf("SelectSelectionInput: % x", r)
	}
	q := x11FixesQueryVersionRequest(130)
	if len(q) != 12 || q[0] != 130 || q[1] != 0 || binary.LittleEndian.Uint16(q[2:]) != 3 ||
		binary.LittleEndian.Uint32(q[4:]) != 5 {
		t.Errorf("QueryVersion: % x", q)
	}
	ev := make([]byte, 32)
	ev[0] = 87 | 0x80 // событие, посланное SendEvent
	binary.LittleEndian.PutUint32(ev[12:], 100)
	if ok, sel := x11FixesSelectionEvent(ev, 87); !ok || sel != 100 {
		t.Errorf("событие XFixes: %v %v", ok, sel)
	}
	if ok, _ := x11FixesSelectionEvent(ev, 88); ok {
		t.Error("чужой код события принят")
	}
	if ok, _ := x11FixesSelectionEvent(ev, 0); ok {
		t.Error("без расширения событий XFixes нет")
	}
}

func newFmtsXServer(t *testing.T, fixesVersion uint32, hasFixes bool) (*x11Clipboard, *fmtsXServer) {
	t.Helper()
	client, server := net.Pipe()
	w := &X11Window{
		conn: client, wid: 1,
		atomClipboard: 100, atomTargets: 101, atomUTF8String: 102, atomText: 103, atomTextHTML: 104,
		atomClipProp: 108, atomTextUriList: 105, atomGnomeCopied: 106,
	}
	c := newX11Clipboard(w)
	w.clip = c
	s := &fmtsXServer{srv: server, out: make(chan []byte, 4096), props: map[uint32][]byte{}}
	go func() {
		for b := range s.out {
			if _, err := server.Write(b); err != nil {
				return
			}
		}
	}()
	go s.read(fixesVersion, hasFixes)
	go w.RunEventLoop()
	t.Cleanup(func() {
		w.closed.Store(true)
		client.Close()
		server.Close()
	})
	// Цикл событий должен работать: подписку на XFixes договаривает он.
	fmtsWaitFor(t, "запуск цикла событий", func() bool { return c.fmts.looping.Load() })
	return c, s
}

// Первый вопрос подписывает окно на смену владельца CLIPBOARD (XFixes), после
// чего ответ живёт до самого события о смене, а не по сроку.
func TestX11Clipboard_FormatsSubscribesToOwnerChanges(t *testing.T) {
	c, s := newFmtsXServer(t, 5, true)
	s.setTargets(101, 102, 105)

	if got := c.ClipboardFormats(); got != fmtsText|fmtsFiles {
		s.mu.Lock()
		for _, r := range s.reqs {
			t.Logf("запрос %d/%d", r[0], r[1])
		}
		s.mu.Unlock()
		t.Fatalf("ClipboardFormats = %v", got)
	}
	fmtsWaitFor(t, "подписка XFixes", func() bool { return c.fmts.watch.Load() == x11FixesActive })

	sel := s.requests(fmtsFixesMajor, 2)
	if len(sel) != 1 {
		t.Fatalf("SelectSelectionInput отправлен %d раз", len(sel))
	}
	if binary.LittleEndian.Uint32(sel[0][4:]) != 1 || binary.LittleEndian.Uint32(sel[0][8:]) != 100 ||
		binary.LittleEndian.Uint32(sel[0][12:]) != 7 {
		t.Errorf("SelectSelectionInput: % x (окно, выделение, маска)", sel[0])
	}

	// Подписка сбросила ответ, полученный до неё; следующий запоминается
	// надолго: срок на него уже не действует.
	if got := c.ClipboardFormats(); got != fmtsText|fmtsFiles {
		t.Fatalf("после подписки: %v", got)
	}
	asked := s.askedCount()
	c.fmts.mu.Lock()
	c.fmts.at = time.Now().Add(-time.Hour)
	c.fmts.mu.Unlock()
	for i := 0; i < 3; i++ {
		c.ClipboardFormats()
	}
	if s.askedCount() != asked {
		t.Error("при XFixes ответ пересчитывался по сроку")
	}

	// Владелец сменился: сервер присылает событие XFixes — память сброшена.
	s.setTargets(101, 104)
	ev := make([]byte, 32)
	ev[0] = fmtsFixesFirst
	binary.LittleEndian.PutUint32(ev[12:], 100)
	s.out <- ev
	fmtsWaitFor(t, "сброс памяти", func() bool {
		_, ok := c.fmts.lookup(time.Now())
		return !ok
	})
	if got := c.ClipboardFormats(); got != fmtsHTML {
		t.Errorf("после смены владельца: %v", got)
	}

	// Событие про другое выделение (PRIMARY) память не трогает.
	asked = s.askedCount()
	ev2 := make([]byte, 32)
	ev2[0] = fmtsFixesFirst
	binary.LittleEndian.PutUint32(ev2[12:], 999)
	s.out <- ev2
	time.Sleep(50 * time.Millisecond)
	c.ClipboardFormats()
	if s.askedCount() != asked {
		t.Error("событие чужого выделения сбросило память")
	}
}

// Сервер без XFixes (или старый): подписки нет, ответы живут по сроку, вопросы
// по-прежнему работают.
func TestX11Clipboard_FormatsWithoutXFixes(t *testing.T) {
	for name, tc := range map[string]struct {
		has bool
		ver uint32
	}{"нет расширения": {false, 0}, "версия 1": {true, 1}} {
		t.Run(name, func(t *testing.T) {
			c, s := newFmtsXServer(t, tc.ver, tc.has)
			s.setTargets(101, 105)
			if got := c.ClipboardFormats(); got != fmtsFiles {
				t.Fatalf("ClipboardFormats = %v", got)
			}
			fmtsWaitFor(t, "отказ от подписки", func() bool { return c.fmts.watch.Load() == x11FixesUnavailable })
			if n := len(s.requests(fmtsFixesMajor, 2)); n != 0 {
				t.Errorf("подписка отправлена, хотя расширение не подходит (%d)", n)
			}
			if got := c.ClipboardFormats(); got != fmtsFiles {
				t.Errorf("после отказа: %v", got)
			}
		})
	}
}

// fmtsXServer — сервер с расширением XFIXES и владельцем буфера: записывает
// запросы подписки и отвечает на TARGETS.
type fmtsXServer struct {
	srv  net.Conn
	out  chan []byte
	mu   sync.Mutex
	reqs [][]byte
	// targets — ответ владельца на TARGETS.
	targets []uint32
	asked   int
	props   map[uint32][]byte
}

const (
	fmtsFixesMajor = 130
	fmtsFixesFirst = 87
)

func (s *fmtsXServer) read(fixesVersion uint32, hasFixes bool) {
	for {
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(s.srv, hdr); err != nil {
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
		body := make([]byte, n-4)
		if _, err := io.ReadFull(s.srv, body); err != nil {
			return
		}
		req := append(hdr, body...)
		s.mu.Lock()
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		switch {
		case req[0] == 98: // QueryExtension
			r := make([]byte, 32)
			r[0] = 1
			if hasFixes {
				r[8], r[9], r[10] = 1, fmtsFixesMajor, fmtsFixesFirst
			}
			s.out <- r
		case req[0] == fmtsFixesMajor && req[1] == 0: // XFixesQueryVersion
			r := make([]byte, 32)
			r[0] = 1
			binary.LittleEndian.PutUint32(r[8:], fixesVersion)
			s.out <- r
		case req[0] == 25: // SendEvent: окно разбудило само себя
			s.out <- append([]byte(nil), req[12:44]...)
		case req[0] == 24: // ConvertSelection: requestor, selection, target, property
			target := binary.LittleEndian.Uint32(req[12:16])
			prop := binary.LittleEndian.Uint32(req[16:20])
			s.mu.Lock()
			s.asked++
			targets := s.targets
			s.props[prop] = atomsBytes(targets...)
			s.mu.Unlock()
			if target != 101 {
				s.notify(target, 0)
				continue
			}
			s.notify(target, prop)
		case req[0] == 20: // GetProperty
			prop := binary.LittleEndian.Uint32(req[8:12])
			s.mu.Lock()
			data, ok := s.props[prop]
			if req[1] != 0 {
				delete(s.props, prop)
			}
			s.mu.Unlock()
			pad := (4 - len(data)%4) % 4
			r := make([]byte, 32+len(data)+pad)
			r[0] = 1
			if ok {
				r[1] = 32
				binary.LittleEndian.PutUint32(r[4:], uint32((len(data)+pad)/4))
				binary.LittleEndian.PutUint32(r[8:], 4) // ATOM
				binary.LittleEndian.PutUint32(r[16:], uint32(len(data)/4))
				copy(r[32:], data)
			}
			s.out <- r
		}
	}
}

func (s *fmtsXServer) notify(target, prop uint32) {
	b := make([]byte, 32)
	b[0] = 31
	binary.LittleEndian.PutUint32(b[8:], 1)
	binary.LittleEndian.PutUint32(b[12:], 100)
	binary.LittleEndian.PutUint32(b[16:], target)
	binary.LittleEndian.PutUint32(b[20:], prop)
	s.out <- b
}

func (s *fmtsXServer) setTargets(t ...uint32) {
	s.mu.Lock()
	s.targets = t
	s.mu.Unlock()
}

func (s *fmtsXServer) askedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked
}

func (s *fmtsXServer) requests(opcode, minor byte) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, r := range s.reqs {
		if r[0] == opcode && (minor == 255 || r[1] == minor) {
			out = append(out, r)
		}
	}
	return out
}

func fmtsWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
