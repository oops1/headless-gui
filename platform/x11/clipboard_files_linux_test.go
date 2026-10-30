//go:build linux && !android

package x11

import (
	"bytes"
	"reflect"
	"testing"
)

func newX11FilesWindow(t *testing.T) (*X11Window, *x11Captured) {
	w, capt := newX11TestWindow(t)
	w.atomTextUriList, w.atomGnomeCopied = 105, 106
	return w, capt
}

// В TARGETS — цели файлов, на запросы — свои тела; у текста целей файлов нет.
func TestX11Clipboard_FilesTargetsAndBodies(t *testing.T) {
	w, capt := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	c.SetFiles([]string{"/home/u/a b", "/tmp/c"}, true)
	c.handleRequest(selectionRequest(9, 100, 101, 200)) // TARGETS
	c.handleRequest(selectionRequest(9, 100, 105, 201)) // text/uri-list
	c.handleRequest(selectionRequest(9, 100, 106, 202)) // gnome
	c.handleRequest(selectionRequest(9, 100, 102, 203)) // UTF8_STRING
	reqs := capt.requests(t, w)

	_, targets, _ := changeProperty(reqs, 200)
	seen := map[uint32]bool{}
	for i := 0; i+4 <= len(targets); i += 4 {
		seen[uint32(targets[i])|uint32(targets[i+1])<<8|uint32(targets[i+2])<<16|uint32(targets[i+3])<<24] = true
	}
	if !seen[105] || !seen[106] {
		t.Errorf("цели файлов в TARGETS: %v", seen)
	}
	if _, d, _ := changeProperty(reqs, 201); string(d) != "file:///home/u/a%20b\r\nfile:///tmp/c\r\n" {
		t.Errorf("uri-list: %q", d)
	}
	if _, d, _ := changeProperty(reqs, 202); string(d) != "cut\nfile:///home/u/a%20b\nfile:///tmp/c" {
		t.Errorf("gnome: %q", d)
	}
	if _, d, _ := changeProperty(reqs, 203); string(d) != "/home/u/a b\n/tmp/c" {
		t.Errorf("текст: %q", d)
	}

	w2, capt2 := newX11FilesWindow(t)
	c2 := newX11Clipboard(w2)
	c2.SetText("x")
	c2.handleRequest(selectionRequest(9, 100, 101, 200))
	_, targets, _ = changeProperty(capt2.requests(t, w2), 200)
	if bytes.Contains(targets, []byte{105, 0, 0, 0}) || bytes.Contains(targets, []byte{106, 0, 0, 0}) {
		t.Error("цели файлов объявлены для простого текста")
	}
}

// Пока владеем — свой список; текст и потеря владения его вытесняют.
func TestX11Clipboard_GetFilesOwned(t *testing.T) {
	w, _ := newX11FilesWindow(t)
	c := newX11Clipboard(w)
	c.SetFiles([]string{"/a"}, false)
	if p, cut, ok := c.GetFiles(); !ok || cut || !reflect.DeepEqual(p, []string{"/a"}) {
		t.Errorf("GetFiles = %v %v %v", p, cut, ok)
	}
	c.SetText("x")
	if _, _, ok := c.GetFiles(); ok {
		t.Error("после SetText остались файлы")
	}
	c.SetFiles([]string{"/a"}, false)
	clear := make([]byte, 32)
	clear[12] = 100 // selection = CLIPBOARD
	c.handleClear(clear)
	// Сам GetFiles здесь пошёл бы спрашивать нового владельца (в тесте его
	// нет) — проверяем, что своя копия сброшена.
	c.mu.Lock()
	files, owns := c.ownedFiles, c.owns
	c.mu.Unlock()
	if owns || files != nil {
		t.Errorf("после SelectionClear: owns=%v files=%v", owns, files)
	}
}
