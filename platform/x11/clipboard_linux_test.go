//go:build linux && !android

package x11

import (
	"encoding/binary"
	"testing"
)

// Владея буфером, отвечаем из своей копии — без похода в сервер.
func TestX11Clipboard_OwnedText(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "abc\n", true

	if got := c.GetText(); got != "abc\n" {
		t.Errorf("GetText = %q, ждал «abc\\n»", got)
	}
}

// SelectionClear забирает владение: дальше текст у нового владельца.
func TestX11Clipboard_SelectionClear(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "наш", true

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 100) // selection = CLIPBOARD
	c.handleClear(buf)

	if c.owns || c.owned != "" {
		t.Error("владение не снято")
	}
}

// Чужое выделение (например, PRIMARY) наш буфер не трогает.
func TestX11Clipboard_ClearOfOtherSelectionIgnored(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "наш", true

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 101) // другое выделение
	c.handleClear(buf)

	if !c.owns {
		t.Error("чужое SelectionClear сняло наше владение")
	}
}

// SelectionNotify чужого выделения не наш: его разберёт XDND.
func TestX11Clipboard_NotifyOfOtherSelectionNotOurs(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 102) // XdndSelection, например
	if c.handleNotify(buf) {
		t.Error("буфер обмена забрал чужое SelectionNotify")
	}
}
