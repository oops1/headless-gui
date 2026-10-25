package widget

import (
	"image"
	"testing"
)

// Набор видов: Has требует ВСЕ перечисленные, пустой запрос не подходит ничему.
func TestClipboardFormatSet_HasAndString(t *testing.T) {
	s := ClipboardFormatText | ClipboardFormatFiles
	if !s.Has(ClipboardFormatFiles) || !s.Has(ClipboardFormatText|ClipboardFormatFiles) {
		t.Errorf("Has не видит то, что есть: %v", s)
	}
	if s.Has(ClipboardFormatImage) || s.Has(ClipboardFormatText|ClipboardFormatHTML) {
		t.Errorf("Has видит то, чего нет: %v", s)
	}
	if s.Has(0) {
		t.Error("Has(0) должен быть ложью")
	}
	if got := s.String(); got != "text|files" {
		t.Errorf("String = %q", got)
	}
	if got := ClipboardFormatSet(0).String(); got != "-" {
		t.Errorf("String пустого = %q", got)
	}
	if !ClipboardFormatSet(0).Empty() || s.Empty() {
		t.Error("Empty")
	}
}

// Буфер в памяти: набор виден сразу после каждой записи, а новая запись
// вытесняет прежний вид (как и самo содержимое).
func TestClipboardFormats_Memory(t *testing.T) {
	old := GetClipboardProvider()
	defer SetClipboardProvider(old)
	UseMemoryClipboard()

	if got := ClipboardFormats(); !got.Empty() || ClipboardHasFiles() {
		t.Errorf("пустой буфер: %v", got)
	}
	ClipboardSetText("абв")
	if got := ClipboardFormats(); got != ClipboardFormatText || ClipboardHasFiles() {
		t.Errorf("текст: %v", got)
	}
	SetClipboardHTML("<b>x</b>", "x")
	if got := ClipboardFormats(); got != ClipboardFormatText|ClipboardFormatHTML {
		t.Errorf("HTML: %v", got)
	}
	// Файлы лежат и списком, и текстом путей.
	ClipboardSetFiles([]string{"/a", "/b"}, false)
	if got := ClipboardFormats(); got != ClipboardFormatText|ClipboardFormatFiles || !ClipboardHasFiles() {
		t.Errorf("файлы: %v", got)
	}
	if err := ClipboardSetImage(image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if got := ClipboardFormats(); got != ClipboardFormatImage || ClipboardHasFiles() {
		t.Errorf("картинка: %v", got)
	}
	ClipboardSetText("снова текст")
	if ClipboardHasFiles() || ClipboardFormats().Has(ClipboardFormatImage) {
		t.Error("текст не вытеснил прежние виды")
	}
}

// Провайдер приложения без расширения: пустой набор. Файлы НЕ читаются через
// GetFiles — тот может ждать сколько угодно.
func TestClipboardFormats_ProviderWithoutExtension(t *testing.T) {
	old := GetClipboardProvider()
	defer SetClipboardProvider(old)
	p := &fmtsFilesOnlyClipboard{}
	SetClipboardProvider(p)
	if got := ClipboardFormats(); !got.Empty() {
		t.Errorf("набор у провайдера без расширения: %v", got)
	}
	if ClipboardHasFiles() {
		t.Error("ClipboardHasFiles ответил true без расширения")
	}
	if p.getFilesCalls != 0 {
		t.Errorf("GetFiles вызван %d раз: вопрос «есть ли файлы» не должен читать", p.getFilesCalls)
	}
}

// fmtsFilesOnlyClipboard — провайдер с файлами, но без ClipboardFormatsProvider.
type fmtsFilesOnlyClipboard struct {
	textOnlyClipboard
	getFilesCalls int
}

func (c *fmtsFilesOnlyClipboard) SetFiles([]string, bool) {}
func (c *fmtsFilesOnlyClipboard) GetFiles() ([]string, bool, bool) {
	c.getFilesCalls++
	return []string{"/a"}, false, true
}

// Расширение подхватывается у любого провайдера, который его реализует.
func TestClipboardFormats_CustomProvider(t *testing.T) {
	old := GetClipboardProvider()
	defer SetClipboardProvider(old)
	SetClipboardProvider(&fmtsTestProvider{set: ClipboardFormatFiles})
	if !ClipboardHasFiles() {
		t.Error("расширение провайдера не используется")
	}
}

type fmtsTestProvider struct {
	textOnlyClipboard
	set ClipboardFormatSet
}

func (c *fmtsTestProvider) ClipboardFormats() ClipboardFormatSet { return c.set }
