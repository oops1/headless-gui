//go:build windows

package clipboard

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// fmtsTestPut очищает буфер и кладёт указанные форматы одним открытием.
func fmtsTestPut(t *testing.T, formats map[uintptr][]byte) {
	t.Helper()
	if !openClipboardRetry() {
		t.Skip("буфер обмена занят другим процессом")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	for f, d := range formats {
		setClipboardBytes(f, d)
	}
}

// Вопрос о форматах не открывает буфер: пока другой «процесс» (здесь — мы
// сами) держит его открытым, ответ всё равно мгновенный и верный.
func TestWinClipboard_FormatsWithoutOpening(t *testing.T) {
	c := needSysClipboard(t)

	c.SetFiles([]string{`C:\Users\u\Отчёт.docx`}, false)
	got := c.ClipboardFormats()
	if !got.Has(base.ClipboardFormatFiles) || !got.Has(base.ClipboardFormatText) || got.Has(base.ClipboardFormatImage) {
		t.Errorf("после SetFiles: %v", got)
	}

	c.SetText("просто текст")
	if got := c.ClipboardFormats(); got != base.ClipboardFormatText {
		t.Errorf("после SetText: %v", got)
	}

	// Буфер открыт у нас самих — OpenClipboard чужому уже отказал бы, а
	// IsClipboardFormatAvailable отвечает.
	c.SetFiles([]string{`C:\a`}, true)
	if !openClipboardRetry() {
		t.Skip("буфер обмена занят другим процессом")
	}
	held := c.ClipboardFormats()
	procCloseClipboard.Call()
	if !held.Has(base.ClipboardFormatFiles) {
		t.Errorf("при открытом буфере: %v", held)
	}
}

// Свой вид у каждого формата: HTML, картинка, «обещанные» файлы без CF_HDROP.
func TestWinClipboard_FormatsKinds(t *testing.T) {
	c := needSysClipboard(t)

	c.SetHTML("<b>x</b>", "x")
	if got := c.ClipboardFormats(); got != base.ClipboardFormatText|base.ClipboardFormatHTML {
		t.Errorf("HTML: %v", got)
	}

	if err := c.SetImage(winTestImage()); err != nil {
		t.Fatal(err)
	}
	if got := c.ClipboardFormats(); !got.Has(base.ClipboardFormatImage) || got.Has(base.ClipboardFormatFiles) {
		t.Errorf("картинка: %v", got)
	}

	// Обещанные файлы: только FileGroupDescriptorW, списка CF_HDROP нет.
	fmtsTestPut(t, map[uintptr][]byte{fmtsFileDescFormat(): make([]byte, 8)})
	got := c.ClipboardFormats()
	if !got.Has(base.ClipboardFormatFiles) {
		t.Errorf("FileGroupDescriptorW не признан файлами: %v", got)
	}

	// Пустой буфер.
	fmtsTestPut(t, nil)
	if got := c.ClipboardFormats(); !got.Empty() {
		t.Errorf("пустой буфер: %v", got)
	}
}

// Через публичный API: widget.ClipboardHasFiles смотрит в системный провайдер.
func TestWinClipboard_HasFilesViaPublicAPI(t *testing.T) {
	c := needSysClipboard(t)
	old := base.GetClipboardProvider()
	defer base.SetClipboardProvider(old)
	base.SetClipboardProvider(c)

	c.SetFiles([]string{`C:\a`}, false)
	if !base.ClipboardHasFiles() {
		t.Error("ClipboardHasFiles = false при файлах в буфере")
	}
	c.SetText("x")
	if base.ClipboardHasFiles() {
		t.Error("ClipboardHasFiles = true при тексте")
	}
}
