//go:build linux && !android

package window

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Оформленный текст (text/html) в буфере обмена Wayland и X11. Настоящий
// системный буфер не трогаем: компоновщик и X-сервер подменены парой сокетов.

// ─── Wayland ────────────────────────────────────────────────────────────────

// SetHTML объявляет text/html рядом с текстовыми типами, SetText — нет.
func TestWlClipboard_SetHTMLOffersTextHTML(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(77)

	c.w.clip.SetHTML("<b>x</b>", "x")
	raw := c.read(t)
	if !bytes.Contains(raw, []byte(mimeTextHTML+"\x00")) {
		t.Error("text/html не объявлен при SetHTML")
	}
	if !bytes.Contains(raw, []byte(mimeTextUTF8)) {
		t.Error("простой текст перестал объявляться рядом с HTML")
	}

	c.w.clip.SetText("x")
	if bytes.Contains(c.read(t), []byte(mimeTextHTML)) {
		t.Error("text/html объявлен для простого текста")
	}
}

// На запрос text/html отдаётся разметка, на остальное — простой текст.
func TestWlClipboard_SourceSendPicksByMime(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(5)
	c.w.clip.SetHTML("<b>Привет</b>", "Привет")
	src := c.w.clip.source

	for _, tc := range []struct{ mime, want string }{
		{mimeTextHTML, "<b>Привет</b>"},
		{"text/html;charset=utf-8", "<b>Привет</b>"},
		{mimeTextUTF8, "Привет"},
		{mimeUTF8Str, "Привет"},
	} {
		r, wr, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		fd, err := syscall.Dup(int(wr.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		wr.Close()
		c.w.clip.handleSourceSend(src, tc.mime, fd) // закрывает fd сам
		r.SetReadDeadline(time.Now().Add(time.Second))
		got, _ := io.ReadAll(r)
		r.Close()
		if string(got) != tc.want {
			t.Errorf("на %q отдано %q, ждал %q", tc.mime, got, tc.want)
		}
	}
}

// Пока владеем сами, HTML берётся из своей копии; SetText и чужой selection
// оформление вытесняют.
func TestWlClipboard_GetHTMLOwned(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(5)

	c.w.clip.SetHTML("<i>a</i>", "a")
	if h, ok := c.w.clip.GetHTML(); !ok || h != "<i>a</i>" {
		t.Errorf("GetHTML = %q, %v", h, ok)
	}
	c.w.clip.SetText("b")
	if h, ok := c.w.clip.GetHTML(); ok {
		t.Errorf("после SetText остался HTML %q", h)
	}

	c.w.clip.SetHTML("<i>a</i>", "a")
	c.w.clip.handleSelection(44) // чужой offer без типов
	if h, ok := c.w.clip.GetHTML(); ok {
		t.Errorf("после чужого selection остался HTML %q", h)
	}
}

// Чужой буфер без text/html — честное «HTML нет», без запроса владельцу.
func TestWlClipboard_GetHTMLForeignWithoutHTML(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSetText(50)
	c.w.clip.handleSelection(50)
	start := time.Now()
	if h, ok := c.w.clip.GetHTML(); ok || h != "" {
		t.Errorf("GetHTML = %q, %v при буфере с одним текстом", h, ok)
	}
	if time.Since(start) > clipboardReadTimeout/2 {
		t.Error("GetHTML ждал владельца, хотя HTML не объявлен")
	}
}

// Чужой HTML читается через receive с запросом именно объявленного типа.
// Компоновщик в тесте — peer: принимает запрос с descriptor'ом и пишет в него
// разметку в UTF-16 с BOM, как делают некоторые приложения.
func TestWlClipboard_GetHTMLForeignRoundTrip(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSet(50, false)
	c.w.offerSetText(50)
	c.w.offerSetHTML(50, mimeTextHTML)
	c.w.clip.handleSelection(50)

	html := "<p>Привет, мир</p>"
	var asked string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf, oob := make([]byte, 256), make([]byte, 64)
		c.peer.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
		if err != nil || n < 8 {
			return
		}
		asked, _ = wlString(buf[8:n], 0)
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil || len(msgs) == 0 {
			return
		}
		fds, err := syscall.ParseUnixRights(&msgs[0])
		if err != nil || len(fds) == 0 {
			return
		}
		f := os.NewFile(uintptr(fds[0]), "peer-write")
		defer f.Close()
		u := []byte{0xFF, 0xFE}
		for _, r := range html {
			u = append(u, byte(r), byte(r>>8)) // тест ограничен BMP
		}
		f.Write(u)
	}()

	got, ok := c.w.clip.GetHTML()
	wg.Wait()
	if asked != mimeTextHTML {
		t.Errorf("запрошен тип %q, ждал %q", asked, mimeTextHTML)
	}
	if !ok || got != html {
		t.Errorf("GetHTML = %q, %v, ждал %q", got, ok, html)
	}
}

func TestOfferHTMLMimeTracking(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSetHTML(60, "text/html;charset=utf-8")
	if got := c.w.offerHTMLMime(60); got != "text/html;charset=utf-8" {
		t.Errorf("тип = %q", got)
	}
	c.w.offerSetHTML(60, mimeTextHTML) // голый вариант вытесняет
	if got := c.w.offerHTMLMime(60); got != mimeTextHTML {
		t.Errorf("тип = %q, ждал голый text/html", got)
	}
	c.w.offerDelete(60)
	if c.w.offerHTMLMime(60) != "" {
		t.Error("offer удалён, а тип остался")
	}
}

// ─── X11 ────────────────────────────────────────────────────────────────────

// x11Captured собирает всё, что окно отправило серверу.
type x11Captured struct {
	mu   sync.Mutex
	data []byte
	done chan struct{}
}

func newX11TestWindow(t *testing.T) (*X11Window, *x11Captured) {
	t.Helper()
	client, server := net.Pipe()
	capt := &x11Captured{done: make(chan struct{})}
	go func() {
		defer close(capt.done)
		buf := make([]byte, 4096)
		for {
			n, err := server.Read(buf)
			capt.mu.Lock()
			capt.data = append(capt.data, buf[:n]...)
			capt.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	w := &X11Window{
		conn: client, wid: 1,
		atomClipboard: 100, atomTargets: 101, atomUTF8String: 102, atomText: 103, atomTextHTML: 104,
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	return w, capt
}

// requests останавливает приём и режет поток на запросы протокола X11.
func (c *x11Captured) requests(t *testing.T, w *X11Window) [][]byte {
	t.Helper()
	w.conn.Close()
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	var out [][]byte
	b := c.data
	for len(b) >= 4 {
		size := int(binary.LittleEndian.Uint16(b[2:4])) * 4
		if size < 4 || size > len(b) {
			t.Fatalf("битая длина запроса: %d при %d байтах", size, len(b))
		}
		out = append(out, b[:size])
		b = b[size:]
	}
	return out
}

// selectionRequest собирает событие SelectionRequest для handleRequest.
func selectionRequest(requestor, selection, target, property uint32) []byte {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], requestor)
	binary.LittleEndian.PutUint32(buf[16:20], selection)
	binary.LittleEndian.PutUint32(buf[20:24], target)
	binary.LittleEndian.PutUint32(buf[24:28], property)
	return buf
}

// changeProperty ищет среди запросов ChangeProperty (opcode 18) со свойством
// property и возвращает тип и данные.
func changeProperty(reqs [][]byte, property uint32) (typ uint32, data []byte, ok bool) {
	for _, r := range reqs {
		if r[0] == 18 && binary.LittleEndian.Uint32(r[8:12]) == property {
			typ = binary.LittleEndian.Uint32(r[12:16])
			n := int(binary.LittleEndian.Uint32(r[20:24]))
			if r[16] == 32 {
				n *= 4
			}
			return typ, r[24 : 24+n], true
		}
	}
	return 0, nil, false
}

// В TARGETS рядом с текстовыми целями появляется text/html — но только когда
// есть что отдать.
func TestX11Clipboard_TargetsListHTML(t *testing.T) {
	for _, withHTML := range []bool{true, false} {
		w, capt := newX11TestWindow(t)
		c := newX11Clipboard(w)
		if withHTML {
			c.SetHTML("<b>x</b>", "x")
		} else {
			c.SetText("x")
		}
		c.handleRequest(selectionRequest(9, 100, 101, 200))

		typ, data, ok := changeProperty(capt.requests(t, w), 200)
		if !ok || typ != 4 {
			t.Fatalf("TARGETS не отправлен: %v %v", typ, ok)
		}
		has := false
		for i := 0; i+4 <= len(data); i += 4 {
			if binary.LittleEndian.Uint32(data[i:]) == 104 {
				has = true
			}
		}
		if has != withHTML {
			t.Errorf("withHTML=%v: text/html в TARGETS = %v", withHTML, has)
		}
	}
}

// На запрос text/html отвечаем разметкой, на текстовый — текстом.
func TestX11Clipboard_RequestHTMLAndText(t *testing.T) {
	w, capt := newX11TestWindow(t)
	c := newX11Clipboard(w)
	c.SetHTML("<b>Привет</b>", "Привет")
	c.handleRequest(selectionRequest(9, 100, 104, 200)) // text/html
	c.handleRequest(selectionRequest(9, 100, 102, 201)) // UTF8_STRING

	reqs := capt.requests(t, w)
	if _, d, ok := changeProperty(reqs, 200); !ok || string(d) != "<b>Привет</b>" {
		t.Errorf("на text/html отдано %q, %v", d, ok)
	}
	if _, d, ok := changeProperty(reqs, 201); !ok || string(d) != "Привет" {
		t.Errorf("на UTF8_STRING отдано %q, %v", d, ok)
	}
}

// Без оформленной версии на text/html отвечаем отказом (property = None):
// проситель поймёт, что HTML нет.
func TestX11Clipboard_RequestHTMLRefusedForPlain(t *testing.T) {
	w, capt := newX11TestWindow(t)
	c := newX11Clipboard(w)
	c.SetText("x")
	c.handleRequest(selectionRequest(9, 100, 104, 200))

	var notify []byte
	for _, r := range capt.requests(t, w) {
		if r[0] == 25 { // SendEvent
			notify = r
		}
	}
	if notify == nil {
		t.Fatal("ответ SelectionNotify не отправлен")
	}
	// Событие начинается со смещения 12; property — байты 20:24 события.
	if prop := binary.LittleEndian.Uint32(notify[12+20 : 12+24]); prop != 0 {
		t.Errorf("property = %d, ждал 0 (отказ)", prop)
	}
}

// Пока владеем, HTML берётся из своей копии; SetText и потеря владения
// оформление вытесняют.
func TestX11Clipboard_GetHTMLOwned(t *testing.T) {
	w, _ := newX11TestWindow(t)
	c := newX11Clipboard(w)
	c.SetHTML("<i>a</i>", "a")
	if h, ok := c.GetHTML(); !ok || h != "<i>a</i>" {
		t.Errorf("GetHTML = %q, %v", h, ok)
	}
	if got := c.GetText(); got != "a" {
		t.Errorf("GetText = %q рядом с HTML", got)
	}
	c.SetText("b")
	if h, ok := c.GetHTML(); ok {
		t.Errorf("после SetText остался HTML %q", h)
	}

	c.SetHTML("<i>a</i>", "a")
	clr := make([]byte, 32)
	binary.LittleEndian.PutUint32(clr[12:16], 100)
	c.handleClear(clr)
	if c.ownedHTML != "" {
		t.Error("оформление не снято при потере владения")
	}
}
