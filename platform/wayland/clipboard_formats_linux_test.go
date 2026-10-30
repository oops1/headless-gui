//go:build linux && !android

package wayland

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// «Какие виды содержимого в буфере» (base.ClipboardFormats): без чтения
// данных и без долгого ожидания владельца.

const (
	fmtsText  = base.ClipboardFormatText
	fmtsHTML  = base.ClipboardFormatHTML
	fmtsImage = base.ClipboardFormatImage
	fmtsFiles = base.ClipboardFormatFiles
)

// ─── Wayland ────────────────────────────────────────────────────────────────

// Виды берутся из типов offer'а, пришедших событиями: канала нет, владелец не
// спрашивается — ответ мгновенный.
func TestWlClipboard_FormatsFromOffer(t *testing.T) {
	c := newWlTestWindow(t)
	if got := c.w.clip.ClipboardFormats(); got != 0 {
		t.Errorf("пустой буфер: %v", got)
	}

	c.w.offerSet(60, true)
	c.w.offerSetText(60)
	c.w.clip.handleSelection(60)
	start := time.Now()
	if got := c.w.clip.ClipboardFormats(); got != fmtsFiles|fmtsText {
		t.Errorf("uri-list и текст: %v", got)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Error("ответ не мгновенный")
	}

	c.w.offerSetGnomeFiles(61)
	c.w.clip.handleSelection(61)
	if got := c.w.clip.ClipboardFormats(); got != fmtsFiles {
		t.Errorf("gnome-copied-files: %v", got)
	}

	c.w.offerSetText(62)
	c.w.offerSetHTML(62, native.MimeTextHTML)
	c.w.clip.handleSelection(62)
	if got := c.w.clip.ClipboardFormats(); got != fmtsText|fmtsHTML {
		t.Errorf("текст и HTML: %v", got)
	}

	c.w.offerSetImage(63, "image/png")
	c.w.clip.handleSelection(63)
	if got := c.w.clip.ClipboardFormats(); got != fmtsImage {
		t.Errorf("картинка: %v", got)
	}

	c.w.clip.handleSelection(0)
	if got := c.w.clip.ClipboardFormats(); got != 0 {
		t.Errorf("буфер опустел: %v", got)
	}
}

// Пока владеем сами — виды по своему содержимому; чужой selection владение
// забирает.
func TestWlClipboard_FormatsOwned(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID, c.w.dataDeviceID = 30, 31
	c.w.inputSerial.Store(5)

	c.w.clip.SetFiles([]string{"/a", "/b"}, false)
	if got := c.w.clip.ClipboardFormats(); got != fmtsFiles|fmtsText {
		t.Errorf("свои файлы: %v", got)
	}
	c.w.clip.SetText("x")
	if got := c.w.clip.ClipboardFormats(); got != fmtsText {
		t.Errorf("свой текст: %v", got)
	}
	c.w.offerSetText(64)
	c.w.clip.handleSelection(64)
	if got := c.w.clip.ClipboardFormats(); got != fmtsText {
		t.Errorf("чужой текст: %v", got)
	}
}
