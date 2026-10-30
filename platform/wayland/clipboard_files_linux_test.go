//go:build linux && !android

package wayland

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Список файлов в буфере обмена Wayland. Компоновщик подменён парой сокетов.

func newWlFilesClipboard(t *testing.T) *wlTestConn {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(9)
	return c
}

// SetFiles объявляет оба типа списка файлов и текст; SetText — ни одного.
func TestWlClipboard_SetFilesOffersFileTypes(t *testing.T) {
	c := newWlFilesClipboard(t)
	c.w.clip.SetFiles([]string{"/tmp/a"}, false)
	raw := c.read(t)
	for _, mime := range []string{native.MimeTextURIList, native.MimeGnomeCopiedFiles, mimeTextUTF8} {
		if !bytes.Contains(raw, []byte(mime+"\x00")) {
			t.Errorf("%s не объявлен при SetFiles", mime)
		}
	}
	c.w.clip.SetText("x")
	raw = c.read(t)
	if bytes.Contains(raw, []byte(native.MimeTextURIList)) || bytes.Contains(raw, []byte(native.MimeGnomeCopiedFiles)) {
		t.Error("типы файлов объявлены для простого текста")
	}
}

// На каждый тип — своё тело.
func TestWlClipboard_SourceSendFiles(t *testing.T) {
	c := newWlFilesClipboard(t)
	c.w.clip.SetFiles([]string{"/home/u/Отчёт.txt", "/tmp/b"}, true)
	src := c.w.clip.source
	for _, tc := range []struct{ mime, want string }{
		{native.MimeTextURIList, "file:///home/u/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\r\nfile:///tmp/b\r\n"},
		{native.MimeGnomeCopiedFiles, "cut\nfile:///home/u/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\nfile:///tmp/b"},
		{mimeTextUTF8, "/home/u/Отчёт.txt\n/tmp/b"},
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
		c.w.clip.handleSourceSend(src, tc.mime, fd)
		r.SetReadDeadline(time.Now().Add(time.Second))
		got, _ := io.ReadAll(r)
		r.Close()
		if string(got) != tc.want {
			t.Errorf("на %q отдано %q, ждал %q", tc.mime, got, tc.want)
		}
	}
}

// Пока владеем — свой список; текст и чужой selection его вытесняют.
func TestWlClipboard_GetFilesOwned(t *testing.T) {
	c := newWlFilesClipboard(t)
	c.w.clip.SetFiles([]string{"/a", "/b"}, true)
	if p, cut, ok := c.w.clip.GetFiles(); !ok || !cut || !reflect.DeepEqual(p, []string{"/a", "/b"}) {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}
	if s := c.w.clip.GetText(); s != "/a\n/b" {
		t.Errorf("текст файлов: %q", s)
	}
	c.w.clip.SetText("x")
	if _, _, ok := c.w.clip.GetFiles(); ok {
		t.Error("после SetText остались файлы")
	}
	c.w.clip.SetFiles([]string{"/a"}, false)
	c.w.clip.handleSelection(44)
	if _, _, ok := c.w.clip.GetFiles(); ok {
		t.Error("после чужого selection остались файлы")
	}
}

// fakeOwner играет компоновщика: принимает receive с descriptor'ом, называет
// запрошенный тип и пишет body после паузы.
func fakeOwner(t *testing.T, c *wlTestConn, delay time.Duration, body func(mime string) string) (asked *[]string, wait func()) {
	var mu sync.Mutex
	var got []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			buf, oob := make([]byte, 256), make([]byte, 64)
			c.peer.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
			if err != nil || n < 8 {
				return
			}
			mime, _ := wlString(buf[8:n], 0)
			mu.Lock()
			got = append(got, mime)
			mu.Unlock()
			msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
			if err != nil || len(msgs) == 0 {
				continue
			}
			fds, err := syscall.ParseUnixRights(&msgs[0])
			if err != nil || len(fds) == 0 {
				continue
			}
			f := os.NewFile(uintptr(fds[0]), "peer-write")
			time.Sleep(delay)
			f.Write([]byte(body(mime)))
			f.Close()
		}
	}()
	return &got, func() { c.peer.SetReadDeadline(time.Now()); wg.Wait() }
}

// Чужие файлы: спрашивается gnome-формат (там cut), и чтение ждёт дольше
// короткого тайм-аута текста — владелец на удалённом рабочем столе отдаёт
// список, когда файлы скачаны.
func TestWlClipboard_GetFilesForeignWaitsForSlowOwner(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSet(50, true) // text/uri-list
	c.w.offerSetGnomeFiles(50)
	c.w.offerSetText(50)
	c.w.clip.handleSelection(50)

	asked, wait := fakeOwner(t, c, 2*clipboardReadTimeout, func(string) string {
		return "cut\nfile:///home/u/a%20b\nfile:///srv/c"
	})
	p, cut, ok := c.w.clip.GetFiles()
	wait()
	if !ok || !cut || !reflect.DeepEqual(p, []string{"/home/u/a b", "/srv/c"}) {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}
	if len(*asked) != 1 || (*asked)[0] != native.MimeGnomeCopiedFiles {
		t.Errorf("запрошено %v, ждали один %s", *asked, native.MimeGnomeCopiedFiles)
	}
}

// Только uri-list — берётся он; ссылка в списке — не файлы.
func TestWlClipboard_GetFilesForeignURIList(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSet(51, true)
	c.w.clip.handleSelection(51)
	_, wait := fakeOwner(t, c, 0, func(string) string { return "file:///tmp/a\r\n" })
	p, cut, ok := c.w.clip.GetFiles()
	wait()
	if !ok || cut || !reflect.DeepEqual(p, []string{"/tmp/a"}) {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}

	c2 := newWlTestWindow(t)
	c2.w.offerSet(52, true)
	c2.w.clip.handleSelection(52)
	_, wait2 := fakeOwner(t, c2, 0, func(string) string { return "file:///tmp/a\r\nhttps://example.com/\r\n" })
	if p, _, ok := c2.w.clip.GetFiles(); ok {
		t.Errorf("список со ссылкой принят как файлы: %v", p)
	}
	wait2()
}

// В буфере текст — файлов нет, владельца не спрашиваем.
func TestWlClipboard_GetFilesForeignTextOnly(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSet(53, false)
	c.w.offerSetText(53)
	c.w.clip.handleSelection(53)
	start := time.Now()
	if p, _, ok := c.w.clip.GetFiles(); ok {
		t.Errorf("текст принят как файлы: %v", p)
	}
	if time.Since(start) > clipboardReadTimeout/2 {
		t.Error("GetFiles спрашивал владельца, хотя файлы не предложены")
	}
}
