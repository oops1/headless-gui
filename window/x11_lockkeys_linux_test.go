//go:build linux && !android

package window

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// Клавиши-переключатели X11 на проводе: окно крутит настоящий цикл событий над
// net.Pipe, на другом конце — поддельный X-сервер с расширением XKEYBOARD (или
// без него), индикаторами клавиатуры и зафиксированными модификаторами.

const (
	lkXkbMajor = 140
	lkXkbFirst = 90
)

// lkIndicator — индикатор клавиатуры: имя и модификатор, чьё «зафиксировано»
// его зажигает (0 — горит сам по себе, сменить нельзя).
type lkIndicator struct {
	name string
	mod  uint8
}

// lkXServer — игрушечный X-сервер: запоминает запросы, отвечает на нужные и
// хранит зафиксированные модификаторы.
type lkXServer struct {
	srv net.Conn
	out chan []byte

	mu   sync.Mutex
	reqs [][]byte

	xkb        bool          // расширение XKEYBOARD есть
	xkbOld     bool          // и отказывается от версии 1.0
	indicators []lkIndicator // индикатор с номером i; пустое имя — без имени
	locked     uint8         // зафиксированные модификаторы
	extraMask  uint16        // что-то ещё в ядерной маске (кнопка мыши, нажатый Shift)
	modMap     []byte        // GetModifierMapping: 8 групп по perMod
	perMod     int
	latchErr   bool // LatchLockState отвергается ошибкой
}

func lkDefaultModMap() ([]byte, int) {
	return []byte{
		50, 62, // Shift
		66, 0, // Lock
		37, 105, // Control
		64, 108, // Mod1
		77, 0, // Mod2: Num_Lock
		0, 0, // Mod3
		133, 134, // Mod4
		203, 92, // Mod5
	}, 2
}

func lkDefaultIndicators() []lkIndicator {
	return []lkIndicator{
		{"Caps Lock", x11LockMask},
		{"Num Lock", x11Mod2Mask},
		{"Scroll Lock", 0},
	}
}

// lkAtom — атом имени на этом сервере (0 — имени он не знает).
func (s *lkXServer) lkAtom(name string) uint32 {
	for i, ind := range s.indicators {
		if ind.name == name {
			return uint32(200 + i)
		}
	}
	return 0
}

func (s *lkXServer) requests(op, minor byte) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]byte
	for _, r := range s.reqs {
		if r[0] == op && (minor == 255 || r[1] == minor) {
			out = append(out, r)
		}
	}
	return out
}

func (s *lkXServer) setLocked(m uint8) {
	s.mu.Lock()
	s.locked = m
	s.mu.Unlock()
}

func (s *lkXServer) lockedNow() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked
}

func (s *lkXServer) read() {
	for {
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(s.srv, hdr); err != nil {
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[2:4])) * 4
		if n < 4 {
			return
		}
		body := make([]byte, n-4)
		if _, err := io.ReadFull(s.srv, body); err != nil {
			return
		}
		req := append(hdr, body...)
		s.mu.Lock()
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		s.handle(req)
	}
}

func lkReply(extraWords int) []byte {
	r := make([]byte, 32+4*extraWords)
	r[0] = 1
	binary.LittleEndian.PutUint32(r[4:8], uint32(extraWords))
	return r
}

func (s *lkXServer) handle(req []byte) {
	le := binary.LittleEndian
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case req[0] == 98: // QueryExtension
		r := lkReply(0)
		nameLen := int(le.Uint16(req[4:6]))
		if s.xkb && string(req[8:8+nameLen]) == "XKEYBOARD" {
			r[8], r[9], r[10] = 1, lkXkbMajor, lkXkbFirst
		}
		s.out <- r
	case req[0] == 16: // InternAtom
		r := lkReply(0)
		nameLen := int(le.Uint16(req[4:6]))
		le.PutUint32(r[8:12], s.lkAtom(string(req[8:8+nameLen])))
		s.out <- r
	case req[0] == 43: // GetInputFocus
		s.out <- lkReply(0)
	case req[0] == 101: // GetKeyboardMapping: по символу на клавишу, все пустые
		count := int(req[5])
		r := lkReply(count)
		r[1] = 1
		s.out <- r
	case req[0] == 25: // SendEvent: окно разбудило само себя
		s.out <- append([]byte(nil), req[12:44]...)
	case req[0] == 119: // GetModifierMapping
		r := lkReply(2 * s.perMod)
		r[1] = byte(s.perMod)
		copy(r[32:], s.modMap)
		s.out <- r
	case req[0] == 38: // QueryPointer
		r := lkReply(0)
		le.PutUint16(r[24:26], uint16(s.locked)|s.extraMask)
		s.out <- r
	case req[0] == lkXkbMajor && s.xkb:
		s.handleXkb(req)
	}
}

func (s *lkXServer) handleXkb(req []byte) {
	le := binary.LittleEndian
	switch req[1] {
	case xkbMinorUseExtension:
		r := lkReply(0)
		if !s.xkbOld {
			r[1] = 1
		}
		le.PutUint16(r[8:10], 1)
		s.out <- r
	case xkbMinorGetNames:
		var present uint32
		var atoms []uint32
		for i, ind := range s.indicators {
			if ind.name != "" {
				present |= 1 << i
				atoms = append(atoms, uint32(200+i))
			}
		}
		r := lkReply(len(atoms))
		le.PutUint32(r[8:12], le.Uint32(req[8:12]))
		le.PutUint32(r[20:24], present)
		for i, a := range atoms {
			le.PutUint32(r[32+4*i:], a)
		}
		s.out <- r
	case xkbMinorGetIndicatorState:
		var state uint32
		for i, ind := range s.indicators {
			if ind.mod != 0 && s.locked&ind.mod != 0 {
				state |= 1 << i
			}
		}
		r := lkReply(0)
		le.PutUint32(r[8:12], state)
		s.out <- r
	case xkbMinorGetState:
		r := lkReply(0)
		r[11] = s.locked
		s.out <- r
	case xkbMinorLatchLockState:
		if s.latchErr {
			e := make([]byte, 32) // ошибка вместо ответа: тип 0, код BadMatch
			e[1] = 8
			s.out <- e
			return
		}
		affect, locks := req[6], req[7]
		s.locked = s.locked&^affect | locks&affect
	}
}

// lkX11Window поднимает окно с цепким циклом событий над поддельным сервером.
func lkX11Window(t *testing.T, cfg func(*lkXServer)) (*X11Window, *lkXServer) {
	t.Helper()
	client, server := net.Pipe()
	s := &lkXServer{srv: server, out: make(chan []byte, 4096), xkb: true, indicators: lkDefaultIndicators()}
	s.modMap, s.perMod = lkDefaultModMap()
	if cfg != nil {
		cfg(s)
	}

	// Таблица символов окна: Num_Lock на коде 77, Scroll_Lock на 78, Caps_Lock на 66.
	const minKey, perKey = 8, 2
	syms := make([]uint32, (255-minKey+1)*perKey)
	put := func(code int, sym uint32) { syms[(code-minKey)*perKey] = sym }
	put(66, 0xFFE5)
	put(77, keysymNumLock)
	put(78, keysymScrollLock)

	w := &X11Window{
		conn: client, wid: 1, rootWin: 5, atomTargets: 101,
		minKeycode: minKey, symsPerCode: perKey, keysyms: syms,
	}
	w.clip = newX11Clipboard(w)

	go func() {
		for b := range s.out {
			if _, err := server.Write(b); err != nil {
				return
			}
		}
	}()
	go s.read()
	loopDone := make(chan struct{})
	go func() { w.RunEventLoop(); close(loopDone) }()
	t.Cleanup(func() {
		w.closed.Store(true)
		client.Close()
		server.Close()
		select {
		case <-loopDone:
		case <-time.After(3 * time.Second):
			t.Error("цикл событий не завершился")
		}
	})
	lkWaitUntil(t, "запуск цикла событий", func() bool { return w.lk.looping.Load() })
	return w, s
}

func lkWaitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Индикаторы читаются по именам, а не по номерам: на этом сервере «Num Lock»
// стоит под номером 0.
func TestX11LockKeysWire_ReadsIndicatorsByName(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) {
		s.indicators = []lkIndicator{{"Num Lock", x11Mod2Mask}, {"", 0}, {"Caps Lock", x11LockMask}}
	})
	if got, ok := w.LockKeys(); !ok || got != 0 {
		t.Fatalf("всё выключено: %v, %v", got, ok)
	}
	s.setLocked(x11Mod2Mask)
	if got, ok := w.LockKeys(); !ok || got != NumLock {
		t.Errorf("Num: %v, %v", got, ok)
	}
	s.setLocked(x11LockMask | x11Mod2Mask)
	if got, ok := w.LockKeys(); !ok || got != NumLock|CapsLock {
		t.Errorf("Num и Caps: %v, %v", got, ok)
	}
	// Для чтения хватает индикаторов: состояние модификаторов не запрашивалось.
	if n := len(s.requests(lkXkbMajor, xkbMinorGetState)); n != 0 {
		t.Errorf("XkbGetState звали %d раз, хватало индикаторов", n)
	}
	if n := len(s.requests(lkXkbMajor, xkbMinorGetIndicatorState)); n != 3 {
		t.Errorf("XkbGetIndicatorState звали %d раз, ждал по одному на вопрос", n)
	}
}

// Scroll Lock читается по индикатору; Kana — по имени «Kana».
func TestX11LockKeysWire_ScrollAndKanaIndicators(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) {
		s.indicators = append(lkDefaultIndicators(), lkIndicator{"Kana", x11Mod2Mask << 1})
	})
	s.setLocked(x11Mod2Mask << 1)
	if got, ok := w.LockKeys(); !ok || got != KanaLock {
		t.Errorf("Kana: %v, %v", got, ok)
	}
}

// Договорённость с сервером делается один раз, а не на каждый вопрос.
func TestX11LockKeysWire_NegotiatesOnce(t *testing.T) {
	w, s := lkX11Window(t, nil)
	for i := 0; i < 3; i++ {
		if _, ok := w.LockKeys(); !ok {
			t.Fatal("LockKeys не ответил")
		}
	}
	for _, c := range []struct {
		name  string
		count int
	}{
		{"QueryExtension", len(s.requests(98, 255))},
		{"UseExtension", len(s.requests(lkXkbMajor, xkbMinorUseExtension))},
		{"GetNames", len(s.requests(lkXkbMajor, xkbMinorGetNames))},
		{"GetModifierMapping", len(s.requests(119, 255))},
	} {
		if c.count != 1 {
			t.Errorf("%s: %d запросов, ждал 1", c.name, c.count)
		}
	}
	q := s.requests(98, 255)[0]
	if string(q[8:8+int(binary.LittleEndian.Uint16(q[4:6]))]) != "XKEYBOARD" {
		t.Errorf("QueryExtension спросил %q", q[8:])
	}
}

// Смена раскладки (MappingNotify) может переименовать индикаторы: договорённость
// делается заново.
func TestX11LockKeysWire_MappingNotifyRenegotiates(t *testing.T) {
	w, s := lkX11Window(t, nil)
	if _, ok := w.LockKeys(); !ok {
		t.Fatal("LockKeys не ответил")
	}
	if n := len(s.requests(lkXkbMajor, xkbMinorGetNames)); n != 1 {
		t.Fatalf("GetNames %d", n)
	}
	// Сервер присылает MappingNotify (тип 34): окно перечитывает раскладку
	// (GetKeyboardMapping) и помечает договорённость устаревшей.
	ev := make([]byte, 32)
	ev[0] = 34
	s.out <- ev
	if _, ok := w.LockKeys(); !ok {
		t.Fatal("LockKeys после смены раскладки не ответил")
	}
	if n := len(s.requests(lkXkbMajor, xkbMinorGetNames)); n != 2 {
		t.Errorf("GetNames %d раз, ждал 2 после MappingNotify", n)
	}
}

// Нет индикатора с нужным именем — зафиксированные модификаторы (XkbGetState) по
// маске клавиши; для Num Lock маска берётся из GetModifierMapping и таблицы
// символов окна.
func TestX11LockKeysWire_FallsBackToLockedMods(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) {
		s.indicators = []lkIndicator{{"Something Else", 1}} // Caps/Num/Scroll индикаторов нет
	})
	s.setLocked(x11LockMask)
	if got, ok := w.LockKeys(); !ok || got != CapsLock {
		t.Errorf("Caps по Lock: %v, %v", got, ok)
	}
	s.setLocked(x11Mod2Mask)
	if got, ok := w.LockKeys(); !ok || got != NumLock {
		t.Errorf("Num по Mod2: %v, %v", got, ok)
	}
	// Раскладка с Num_Lock на Mod3 (код 77 переехал в третью группу).
	w2, s2 := lkX11Window(t, func(s *lkXServer) {
		s.indicators = nil
		s.modMap = []byte{50, 62, 66, 0, 37, 105, 64, 108, 0, 0, 77, 0, 133, 134, 203, 92}
	})
	s2.setLocked(1 << 5)
	if got, ok := w2.LockKeys(); !ok || got != NumLock {
		t.Errorf("Num по Mod3: %v, %v", got, ok)
	}
	s2.setLocked(x11Mod2Mask) // Mod2 теперь ничей
	if got, ok := w2.LockKeys(); !ok || got != 0 {
		t.Errorf("Mod2 при Num на Mod3: %v, %v", got, ok)
	}
}

// Без XKB — ядерный протокол: маска QueryPointer. Нажатые кнопки мыши и Shift в
// ней есть, но переключателями не считаются.
func TestX11LockKeysWire_CoreProtocolFallback(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) { s.xkb = false })
	s.extraMask = 0x0101 // Shift и кнопка 1
	if got, ok := w.LockKeys(); !ok || got != 0 {
		t.Errorf("пусто: %v, %v", got, ok)
	}
	s.setLocked(x11LockMask | x11Mod2Mask)
	if got, ok := w.LockKeys(); !ok || got != CapsLock|NumLock {
		t.Errorf("Caps и Num: %v, %v", got, ok)
	}
	if n := len(s.requests(38, 255)); n != 2 {
		t.Errorf("QueryPointer %d раз, ждал 2", n)
	}
	if q := s.requests(38, 255)[0]; binary.LittleEndian.Uint32(q[4:8]) != 5 {
		t.Errorf("QueryPointer спросил окно %d, ждал корневое 5", binary.LittleEndian.Uint32(q[4:8]))
	}
	// Менять нечем.
	if w.SetLockKeys(ScrollLock) {
		t.Error("без XKB SetLockKeys выставил Scroll Lock")
	}
	if len(s.requests(lkXkbMajor, xkbMinorLatchLockState)) != 0 {
		t.Error("без XKB ушёл LatchLockState")
	}
	if !w.SetLockKeys(CapsLock | NumLock) {
		t.Error("без XKB совпадающий набор признан неудачей")
	}
}

// XKB есть, но не соглашается на версию 1.0 — как будто его нет.
func TestX11LockKeysWire_OldXkbIsIgnored(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) { s.xkbOld = true })
	s.setLocked(x11LockMask)
	if got, ok := w.LockKeys(); !ok || got != CapsLock {
		t.Errorf("LockKeys = %v, %v", got, ok)
	}
	if len(s.requests(lkXkbMajor, xkbMinorGetIndicatorState)) != 0 {
		t.Error("после отказа UseExtension всё равно спрашивали индикаторы")
	}
}

// SetLockKeys(NumLock): уходит XkbLatchLockState только по Mod2 и не трогает Caps.
func TestX11LockKeysWire_SetNumLockLeavesCaps(t *testing.T) {
	w, s := lkX11Window(t, nil)
	s.setLocked(x11LockMask) // Caps включён, Num выключен
	if !w.SetLockKeys(CapsLock | NumLock) {
		t.Fatal("SetLockKeys отказал")
	}
	reqs := s.requests(lkXkbMajor, xkbMinorLatchLockState)
	if len(reqs) != 1 {
		t.Fatalf("LatchLockState %d раз", len(reqs))
	}
	if r := reqs[0]; r[6] != x11Mod2Mask || r[7] != x11Mod2Mask {
		t.Errorf("affect=%#x locks=%#x, ждал только Mod2", r[6], r[7])
	}
	if got := s.lockedNow(); got != x11LockMask|x11Mod2Mask {
		t.Errorf("зафиксировано %#x, ждал Lock+Mod2", got)
	}
	if got, _ := w.LockKeys(); got != CapsLock|NumLock {
		t.Errorf("LockKeys после: %v", got)
	}
}

// Выключение и сразу несколько клавиш.
func TestX11LockKeysWire_SetOffAndMany(t *testing.T) {
	w, s := lkX11Window(t, nil)
	s.setLocked(x11LockMask | x11Mod2Mask)
	if !w.SetLockKeys(NumLock) {
		t.Fatal("выключить Caps не вышло")
	}
	if got := s.lockedNow(); got != x11Mod2Mask {
		t.Errorf("зафиксировано %#x, ждал только Mod2", got)
	}
	r := s.requests(lkXkbMajor, xkbMinorLatchLockState)[0]
	if r[6] != x11LockMask || r[7] != 0 {
		t.Errorf("affect=%#x locks=%#x, ждал выключение Lock", r[6], r[7])
	}
	// Сразу все: Caps включить, Num выключить.
	if !w.SetLockKeys(CapsLock) {
		t.Fatal("поменять местами не вышло")
	}
	if got := s.lockedNow(); got != x11LockMask {
		t.Errorf("зафиксировано %#x", got)
	}
	// Уже как просили — запросов нет.
	n := len(s.requests(lkXkbMajor, xkbMinorLatchLockState))
	if !w.SetLockKeys(CapsLock) {
		t.Error("совпадающий набор признан неудачей")
	}
	if len(s.requests(lkXkbMajor, xkbMinorLatchLockState)) != n {
		t.Error("для совпадающего набора ушёл ещё один LatchLockState")
	}
}

// Scroll Lock без модификатора переключить нечем: честный false, остальное
// приведено.
func TestX11LockKeysWire_SetScrollWithoutModifier(t *testing.T) {
	w, s := lkX11Window(t, nil)
	if w.SetLockKeys(ScrollLock | NumLock) {
		t.Error("Scroll Lock без модификатора признан выставленным")
	}
	if got := s.lockedNow(); got != x11Mod2Mask {
		t.Errorf("Num Lock не приведён: зафиксировано %#x", got)
	}
}

// Scroll Lock, висящий на Mod3, переключается как все.
func TestX11LockKeysWire_SetScrollOnMod3(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) {
		s.modMap = []byte{50, 62, 66, 0, 37, 105, 64, 108, 77, 0, 78, 0, 133, 134, 203, 92}
		s.indicators = []lkIndicator{{"Scroll Lock", 1 << 5}}
	})
	if !w.SetLockKeys(ScrollLock) {
		t.Fatal("SetLockKeys(Scroll) отказал")
	}
	if got := s.lockedNow(); got != 1<<5 {
		t.Errorf("зафиксировано %#x, ждал Mod3", got)
	}
	if got, _ := w.LockKeys(); got != ScrollLock {
		t.Errorf("LockKeys = %v", got)
	}
}

// Сервер отверг XkbLatchLockState ошибкой — false, и ответ следующего запроса не
// сбит чужой ошибкой.
func TestX11LockKeysWire_SetRejectedByServer(t *testing.T) {
	w, s := lkX11Window(t, func(s *lkXServer) { s.latchErr = true })
	if w.SetLockKeys(CapsLock) {
		t.Error("ошибка сервера, а SetLockKeys ответил true")
	}
	s.mu.Lock()
	s.latchErr = false
	s.mu.Unlock()
	s.setLocked(x11Mod2Mask)
	if got, ok := w.LockKeys(); !ok || got != NumLock {
		t.Errorf("после ошибки LockKeys = %v, %v", got, ok)
	}
}

// Цикл событий не крутится (окно не запущено или закрыто): вопрос не виснет.
func TestX11LockKeysWire_NoLoop(t *testing.T) {
	w := &X11Window{wid: 1}
	start := time.Now()
	if k, ok := w.LockKeys(); ok || k != 0 {
		t.Errorf("LockKeys без цикла = %v, %v", k, ok)
	}
	if w.SetLockKeys(CapsLock) {
		t.Error("SetLockKeys без цикла ответил true")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("без цикла вопрос ждал %v", time.Since(start))
	}
}

// Сервер молчит: вопрос заканчивается по сроку, а цикл остаётся живым.
func TestX11LockKeysWire_ServerSilentTimesOut(t *testing.T) {
	client, server := net.Pipe()
	w := &X11Window{conn: client, wid: 1, rootWin: 5, atomTargets: 101}
	w.clip = newX11Clipboard(w)
	go func() { // сервер читает и молчит
		buf := make([]byte, 4096)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()
	loopDone := make(chan struct{})
	go func() { w.RunEventLoop(); close(loopDone) }()
	t.Cleanup(func() {
		w.closed.Store(true)
		client.Close()
		server.Close()
		<-loopDone
	})
	lkWaitUntil(t, "запуск цикла событий", func() bool { return w.lk.looping.Load() })

	done := make(chan bool, 1)
	go func() { _, ok := w.LockKeys(); done <- ok }()
	select {
	case ok := <-done:
		if ok {
			t.Error("молчащий сервер дал ответ")
		}
	case <-time.After(x11LockTimeout + 2*time.Second):
		t.Fatal("LockKeys завис на молчащем сервере")
	}
}

// Окно закрылось, пока вопрос ждал: ждущий отпускается сразу, а не по сроку.
func TestX11LockKeysWire_LoopEndReleasesWaiters(t *testing.T) {
	w := &X11Window{wid: 1}
	w.lk.looping.Store(true)
	job := &x11LockJob{done: make(chan x11LockResult, 1)}
	w.lk.jobs = append(w.lk.jobs, job)
	w.lkFailAll()
	select {
	case r := <-job.done:
		if r.ok {
			t.Error("отпущенный ждущий получил успех")
		}
	default:
		t.Error("ждущий не отпущен")
	}
}
