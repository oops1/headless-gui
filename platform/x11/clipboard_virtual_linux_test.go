//go:build linux && !android

package x11

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

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
	err := c.SetVirtualFiles(newVirtApp().files, func(int) (io.ReadCloser, error) { return nil, nil }, base.VirtualFilesOptions{})
	if !errors.Is(err, base.ErrClipboardVirtualNoSource) {
		t.Errorf("без Materialize: %v", err)
	}
	if c.GetText() != "до" {
		t.Error("отказ изменил буфер")
	}
	// Сервер не дал атомов списка файлов — отказ.
	w2, _ := newX11TestWindow(t)
	if err := newX11Clipboard(w2).SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, base.ErrClipboardVirtualUnsupported) {
		t.Errorf("без атомов: %v", err)
	}
	if err := newX11Clipboard(&X11Window{}).SetVirtualFiles(newVirtApp().files, nil, newVirtApp().opts()); !errors.Is(err, base.ErrClipboardVirtualUnsupported) {
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
	if got := c.ClipboardFormats(); !got.Has(base.ClipboardFormatFiles) {
		t.Errorf("ClipboardFormats своих обещанных файлов = %v", got)
	}
	rc, err := c.OpenVirtualFile(0)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
}

// Закрытие окна X11 снимает обещание виртуальных файлов: отдавать их
// больше некому, приложение узнаёт об этом через WithRelease.
func TestX11Clipboard_VirtualWindowCloseReleases(t *testing.T) {
	w, _ := newX11FilesWindow(t)
	w.clip = newX11Clipboard(w)
	app2 := newVirtApp()
	if err := w.clip.SetVirtualFiles(app2.files, nil, app2.opts()); err != nil {
		t.Fatal(err)
	}
	w.Close()
	app2.waitRelease(t, "закрытие окна X11")
}
