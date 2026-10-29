//go:build linux && !android

package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"reflect"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Виртуальные файлы в буфере Linux: типы объявлены сразу, пути приложение даёт
// только когда их запросили. Компоновщик и X-сервер подменены.

// virtApp — «приложение»: Materialize считает вызовы и отдаёт пути.
type virtApp struct {
	calls    atomic.Int32
	fail     atomic.Bool
	released chan struct{}
	files    []widget.VirtualFile
}

func newVirtApp() *virtApp {
	return &virtApp{released: make(chan struct{}, 4), files: []widget.VirtualFile{
		{Name: "Отчёт.txt", Size: 3},
		{Name: "Папка", IsDir: true},
		{Name: "Папка/в.txt", Size: 1},
	}}
}

func (a *virtApp) opts() widget.VirtualFilesOptions {
	return widget.VirtualFilesOptions{
		Materialize: func(i int) (string, error) {
			a.calls.Add(1)
			if a.fail.Load() {
				return "", errors.New("не скачалось")
			}
			return "/home/u/Загрузки/" + a.files[i].Name, nil
		},
		Release: func() { a.released <- struct{}{} },
	}
}

func (a *virtApp) waitRelease(t *testing.T, why string) {
	t.Helper()
	select {
	case <-a.released:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: WithRelease не вызван", why)
	}
}

func (a *virtApp) noRelease(t *testing.T, why string) {
	t.Helper()
	select {
	case <-a.released:
		t.Fatalf("%s: WithRelease вызван", why)
	case <-time.After(30 * time.Millisecond):
	}
}

const (
	virtWantURIs  = "file:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\r\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9F%D0%B0%D0%BF%D0%BA%D0%B0\r\n"
	virtWantGnome = "copy\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9F%D0%B0%D0%BF%D0%BA%D0%B0"
	virtWantNames = "Отчёт.txt\nПапка\nПапка/в.txt"
)

// ─── Wayland ────────────────────────────────────────────────────────────────

// wlVirtSend играет получателя: источник получает send с descriptor'ом.
func wlVirtSend(t *testing.T, c *wlTestConn, mime string) string {
	t.Helper()
	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(wr.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	wr.Close()
	c.w.clip.handleSourceSend(c.w.clip.source, mime, fd)
	r.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, _ := io.ReadAll(r)
	r.Close()
	return string(got)
}

// SetVirtualFiles объявляет типы списка файлов и текст, но приложение не
// трогает: ни при объявлении, ни на текстовый запрос.
func TestWlClipboard_VirtualOffersAndIsLazy(t *testing.T) {
	c := newWlFilesClipboard(t)
	app := newVirtApp()
	if err := c.w.clip.SetVirtualFiles(app.files, nil, app.opts()); err != nil {
		t.Fatal(err)
	}
	raw := c.read(t)
	for _, mime := range []string{mimeTextUriList, mimeGnomeCopiedFiles, mimeTextUTF8} {
		if !bytes.Contains(raw, []byte(mime+"\x00")) {
			t.Errorf("%s не объявлен", mime)
		}
	}
	if got := wlVirtSend(t, c, mimeTextUTF8); got != virtWantNames {
		t.Errorf("текст: %q", got)
	}
	if app.calls.Load() != 0 {
		t.Fatalf("Materialize вызван (%d) без запроса списка файлов", app.calls.Load())
	}

	// Запросили файлы — тогда и только тогда приложение даёт пути.
	if got := wlVirtSend(t, c, mimeTextUriList); got != virtWantURIs {
		t.Errorf("text/uri-list: %q", got)
	}
	if app.calls.Load() != 2 { // верхний уровень: файл и папка, вложенное не спрашивают
		t.Errorf("Materialize вызван %d раз, ждал 2", app.calls.Load())
	}
	if got := wlVirtSend(t, c, mimeGnomeCopiedFiles); got != virtWantGnome {
		t.Errorf("gnome-copied-files: %q", got)
	}
	if app.calls.Load() != 2 {
		t.Errorf("второй формат повторно спросил приложение (%d)", app.calls.Load())
	}
	app.noRelease(t, "пока буфер наш")
}

// Отказ приложения — пустой ответ (вставляющий поймёт «файлов нет»).
func TestWlClipboard_VirtualFailureGivesEmptyAnswer(t *testing.T) {
	c := newWlFilesClipboard(t)
	app := newVirtApp()
	app.fail.Store(true)
	c.w.clip.SetVirtualFiles(app.files, nil, app.opts())
	if got := wlVirtSend(t, c, mimeTextUriList); got != "" {
		t.Errorf("при отказе отдано %q", got)
	}
	app.fail.Store(false)
	if got := wlVirtSend(t, c, mimeTextUriList); got != virtWantURIs {
		t.Errorf("повторная вставка после отказа: %q", got)
	}
}

// Без Materialize файлов не предложить: отказ, буфер не тронут, на провод
// ничего не уходит.
func TestWlClipboard_VirtualNeedsMaterialize(t *testing.T) {
	c := newWlFilesClipboard(t)
	c.w.clip.SetText("до")
	c.read(t)
	err := c.w.clip.SetVirtualFiles(newVirtApp().files, func(int) (io.ReadCloser, error) { return nil, nil }, widget.VirtualFilesOptions{})
	if !errors.Is(err, widget.ErrClipboardVirtualNoSource) {
		t.Errorf("без Materialize: %v", err)
	}
	if c.w.clip.GetText() != "до" {
		t.Error("отказ изменил буфер")
	}
	c.peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, err := c.peer.Read(make([]byte, 64)); err == nil && n > 0 {
		t.Errorf("при отказе на провод ушло %d байт", n)
	}
	// Каталог верхнего уровня не объявлен.
	err = c.w.clip.SetVirtualFiles([]widget.VirtualFile{{Name: "д/ф"}}, nil, newVirtApp().opts())
	if !errors.Is(err, widget.ErrVirtualFileName) {
		t.Errorf("каталог не объявлен: %v", err)
	}
	// Нет data device / serial'а — тоже отказ, а не молчание.
	c2 := newWlTestWindow(t)
	if err := c2.w.clip.SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, widget.ErrClipboardVirtualUnsupported) {
		t.Errorf("без data device: %v", err)
	}
	c3 := newWlTestWindow(t)
	c3.w.dataDevMgrID, c3.w.dataDeviceID = 30, 31
	if err := c3.w.clip.SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, widget.ErrClipboardVirtualUnsupported) {
		t.Errorf("без serial'а: %v", err)
	}
}

// Обещание снимается: новым содержимым, чужим selection, отменой источника,
// Cleanup и закрытием окна — и приложение узнаёт об этом ровно один раз.
func TestWlClipboard_VirtualRelease(t *testing.T) {
	setup := func() (*wlTestConn, *virtApp) {
		c := newWlFilesClipboard(t)
		app := newVirtApp()
		if err := c.w.clip.SetVirtualFiles(app.files, nil, app.opts()); err != nil {
			t.Fatal(err)
		}
		return c, app
	}

	c, app := setup()
	if got, ok := c.w.clip.GetVirtualFiles(); !ok || !reflect.DeepEqual(got, app.files) {
		t.Errorf("GetVirtualFiles = %v %v", got, ok)
	}
	c.w.clip.SetText("x")
	app.waitRelease(t, "SetText")
	if _, ok := c.w.clip.GetVirtualFiles(); ok {
		t.Error("обещание пережило SetText")
	}

	c, app = setup()
	c.w.clip.SetFiles([]string{"/a"}, false)
	app.waitRelease(t, "SetFiles")

	c, app = setup()
	c.w.clip.SetVirtualFiles(app.files, nil, app.opts()) // то же обещание заменено новым
	app.waitRelease(t, "замена обещания")

	c, app = setup()
	c.w.clip.handleSelection(44)
	app.waitRelease(t, "чужой selection")

	c, app = setup()
	c.w.clip.handleSourceCancelled(c.w.clip.source)
	app.waitRelease(t, "отмена источника")
	if _, ok := c.w.clip.GetVirtualFiles(); ok {
		t.Error("обещание пережило отмену источника")
	}

	c, app = setup()
	src := c.w.clip.source
	c.read(t)
	c.w.clip.Cleanup()
	app.waitRelease(t, "Cleanup")
	msgs := splitWlMsgs(t, c.read(t))
	if _, ok := findWlMsg(msgs, src, wlDataSourceDestroy); !ok {
		t.Error("Cleanup не уничтожил источник")
	}
	c.w.clip.Cleanup()
	app.noRelease(t, "повторный Cleanup")

	// Cleanup не трогает чужое и наше невиртуальное содержимое.
	c = newWlFilesClipboard(t)
	c.w.clip.SetFiles([]string{"/a"}, false)
	c.w.clip.Cleanup()
	if p, _, ok := c.w.clip.GetFiles(); !ok || !reflect.DeepEqual(p, []string{"/a"}) {
		t.Errorf("Cleanup задел обычные файлы: %v %v", p, ok)
	}
}

// Вставка внутри программы: GetFiles отдаёт те же пути, что чужому вставляющему.
func TestWlClipboard_VirtualGetFilesOwned(t *testing.T) {
	c := newWlFilesClipboard(t)
	app := newVirtApp()
	c.w.clip.SetVirtualFiles(app.files, func(i int) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("abc"))), nil }, app.opts())
	p, cut, ok := c.w.clip.GetFiles()
	if !ok || cut || len(p) != 2 || p[0] != "/home/u/Загрузки/Отчёт.txt" {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}
	if got := c.w.clip.ClipboardFormats(); !got.Has(widget.ClipboardFormatFiles) {
		t.Errorf("ClipboardFormats своих обещанных файлов = %v", got)
	}
	rc, err := c.w.clip.OpenVirtualFile(0)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "abc" {
		t.Errorf("OpenVirtualFile: %q", b)
	}
	if _, err := c.w.clip.OpenVirtualFile(1); !errors.Is(err, widget.ErrVirtualFileNotFound) {
		t.Errorf("каталог: %v", err)
	}
}

// ─── X11 ────────────────────────────────────────────────────────────────────

// X11: цели файлов в TARGETS есть, на запрос списка — ответ из горутины с
// путями приложения; текстовая цель приложения не трогает.
func TestX11Clipboard_VirtualTargetsAndLazyBodies(t *testing.T) {
	w, capt := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	app := newVirtApp()
	if err := c.SetVirtualFiles(app.files, nil, app.opts()); err != nil {
		t.Fatal(err)
	}
	c.handleRequest(selectionRequest(9, 100, 101, 200)) // TARGETS
	c.handleRequest(selectionRequest(9, 100, 102, 203)) // UTF8_STRING
	if app.calls.Load() != 0 {
		t.Fatalf("Materialize вызван (%d) без запроса списка файлов", app.calls.Load())
	}
	c.handleRequest(selectionRequest(9, 100, 105, 201)) // text/uri-list
	c.handleRequest(selectionRequest(9, 100, 106, 202)) // gnome
	c.pending.Wait()
	reqs := capt.requests(t, w)

	_, targets, _ := changeProperty(reqs, 200)
	seen := map[uint32]bool{}
	for i := 0; i+4 <= len(targets); i += 4 {
		seen[binary.LittleEndian.Uint32(targets[i:])] = true
	}
	if !seen[105] || !seen[106] {
		t.Errorf("цели файлов в TARGETS: %v", seen)
	}
	if _, d, _ := changeProperty(reqs, 203); string(d) != virtWantNames {
		t.Errorf("текст: %q", d)
	}
	if _, d, _ := changeProperty(reqs, 201); string(d) != virtWantURIs {
		t.Errorf("uri-list: %q", d)
	}
	if _, d, _ := changeProperty(reqs, 202); string(d) != virtWantGnome {
		t.Errorf("gnome: %q", d)
	}
	if app.calls.Load() != 2 {
		t.Errorf("Materialize вызван %d раз, ждал 2 (верхний уровень, один обход на оба формата)", app.calls.Load())
	}
}

// Отказ приложения — SelectionNotify со свойством None.
func TestX11Clipboard_VirtualFailureRefuses(t *testing.T) {
	w, capt := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	app := newVirtApp()
	app.fail.Store(true)
	c.SetVirtualFiles(app.files, nil, app.opts())
	c.handleRequest(selectionRequest(9, 100, 105, 201))
	c.pending.Wait()
	var notify []byte
	for _, r := range capt.requests(t, w) {
		if r[0] == 25 {
			notify = r
		}
	}
	if notify == nil {
		t.Fatal("ответ SelectionNotify не отправлен")
	}
	if prop := binary.LittleEndian.Uint32(notify[12+20 : 12+24]); prop != 0 {
		t.Errorf("property = %d, ждал 0 (отказ)", prop)
	}
}

func TestX11Clipboard_VirtualNeedsMaterialize(t *testing.T) {
	w, _ := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	c.SetText("до")
	err := c.SetVirtualFiles(newVirtApp().files, func(int) (io.ReadCloser, error) { return nil, nil }, widget.VirtualFilesOptions{})
	if !errors.Is(err, widget.ErrClipboardVirtualNoSource) {
		t.Errorf("без Materialize: %v", err)
	}
	if c.GetText() != "до" {
		t.Error("отказ изменил буфер")
	}
	// Сервер не дал атомов списка файлов — отказ.
	w2, _ := newX11TestWindow(t)
	if err := newX11Clipboard(w2).SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, widget.ErrClipboardVirtualUnsupported) {
		t.Errorf("без атомов: %v", err)
	}
	if err := newX11Clipboard(&X11Window{}).SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, widget.ErrClipboardVirtualUnsupported) {
		t.Errorf("без окна: %v", err)
	}
}

func TestX11Clipboard_VirtualRelease(t *testing.T) {
	setup := func() (*x11Clipboard, *virtApp) {
		w, _ := newX11FilesWindow(t)
		c := newX11Clipboard(w)
		app := newVirtApp()
		if err := c.SetVirtualFiles(app.files, nil, app.opts()); err != nil {
			t.Fatal(err)
		}
		return c, app
	}

	c, app := setup()
	if got, ok := c.GetVirtualFiles(); !ok || !reflect.DeepEqual(got, app.files) {
		t.Errorf("GetVirtualFiles = %v %v", got, ok)
	}
	c.SetText("x")
	app.waitRelease(t, "SetText")

	c, app = setup()
	c.SetFiles([]string{"/a"}, false)
	app.waitRelease(t, "SetFiles")

	c, app = setup()
	clear := make([]byte, 32)
	clear[12] = 100 // selection = CLIPBOARD
	c.handleClear(clear)
	app.waitRelease(t, "SelectionClear")
	if _, ok := c.GetVirtualFiles(); ok {
		t.Error("обещание пережило SelectionClear")
	}

	c, app = setup()
	c.Cleanup()
	app.waitRelease(t, "Cleanup")
	if c.owns {
		t.Error("после Cleanup мы всё ещё владелец")
	}
	c.Cleanup()
	app.noRelease(t, "повторный Cleanup")

	// Cleanup не трогает обычное содержимое.
	w, _ := newX11FilesWindow(t)
	c2 := newX11Clipboard(w)
	c2.SetFiles([]string{"/a"}, false)
	c2.Cleanup()
	if p, _, ok := c2.GetFiles(); !ok || !reflect.DeepEqual(p, []string{"/a"}) {
		t.Errorf("Cleanup задел обычные файлы: %v %v", p, ok)
	}
}

func TestX11Clipboard_VirtualGetFilesOwned(t *testing.T) {
	w, _ := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	app := newVirtApp()
	c.SetVirtualFiles(app.files, func(int) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader([]byte("abc"))), nil }, app.opts())
	p, cut, ok := c.GetFiles()
	if !ok || cut || len(p) != 2 || p[1] != "/home/u/Загрузки/Папка" {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}
	if got := c.ClipboardFormats(); !got.Has(widget.ClipboardFormatFiles) {
		t.Errorf("ClipboardFormats своих обещанных файлов = %v", got)
	}
	rc, err := c.OpenVirtualFile(0)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
}

// Закрытие окна снимает обещание: отдавать файлы больше некому, приложение
// узнаёт об этом через WithRelease.
func TestVirtualFiles_WindowCloseReleases(t *testing.T) {
	c := newWlFilesClipboard(t)
	app := newVirtApp()
	if err := c.w.clip.SetVirtualFiles(app.files, nil, app.opts()); err != nil {
		t.Fatal(err)
	}
	c.w.Close()
	app.waitRelease(t, "закрытие окна Wayland")

	w, _ := newX11FilesWindow(t)
	w.clip = newX11Clipboard(w)
	app2 := newVirtApp()
	if err := w.clip.SetVirtualFiles(app2.files, nil, app2.opts()); err != nil {
		t.Fatal(err)
	}
	w.Close()
	app2.waitRelease(t, "закрытие окна X11")
}
