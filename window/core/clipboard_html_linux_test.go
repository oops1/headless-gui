//go:build linux && !android

package core

import (
	"bytes"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
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
	if !bytes.Contains(raw, []byte(native.MimeTextHTML+"\x00")) {
		t.Error("text/html не объявлен при SetHTML")
	}
	if !bytes.Contains(raw, []byte(mimeTextUTF8)) {
		t.Error("простой текст перестал объявляться рядом с HTML")
	}

	c.w.clip.SetText("x")
	if bytes.Contains(c.read(t), []byte(native.MimeTextHTML)) {
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
		{native.MimeTextHTML, "<b>Привет</b>"},
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
	c.w.offerSetHTML(50, native.MimeTextHTML)
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
	if asked != native.MimeTextHTML {
		t.Errorf("запрошен тип %q, ждал %q", asked, native.MimeTextHTML)
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
	c.w.offerSetHTML(60, native.MimeTextHTML) // голый вариант вытесняет
	if got := c.w.offerHTMLMime(60); got != native.MimeTextHTML {
		t.Errorf("тип = %q, ждал голый text/html", got)
	}
	c.w.offerDelete(60)
	if c.w.offerHTMLMime(60) != "" {
		t.Error("offer удалён, а тип остался")
	}
}
