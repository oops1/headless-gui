package widget

import (
	"reflect"
	"testing"
)

func TestDropFiles_RoundTrip(t *testing.T) {
	paths := []string{`C:\Users\u\Отчёт.docx`, `D:\a b\c.txt`, `\\server\share\😀.png`}
	b := buildDropFiles(paths)
	got, ok := parseDropFiles(b)
	if !ok || !reflect.DeepEqual(got, paths) {
		t.Fatalf("разбор: %v %v", got, ok)
	}
	// Заголовок: смещение 20, fWide = 1, список кончается двумя нулями.
	n := len(b)
	if b[0] != 20 || b[16] != 1 || b[n-1]|b[n-2]|b[n-3]|b[n-4] != 0 {
		t.Errorf("заголовок или конец списка: % x … % x", b[:20], b[n-4:])
	}
}

func TestDropFiles_RejectsBroken(t *testing.T) {
	good := buildDropFiles([]string{`C:\a`})
	ansi := append([]byte(nil), good...)
	ansi[16] = 0 // fWide = 0
	far := append([]byte(nil), good...)
	far[0] = 0xFF // смещение за концом блока
	for name, b := range map[string][]byte{
		"короткий": good[:10], "ANSI": ansi, "смещение": far,
		"пусто": buildDropFiles(nil),
	} {
		if p, ok := parseDropFiles(b); ok {
			t.Errorf("%s: принят %v", name, p)
		}
	}
	// Без завершающих нулей — не читаем за край, последний путь не теряем.
	short := good[:len(good)-4]
	if p, ok := parseDropFiles(short); !ok || !reflect.DeepEqual(p, []string{`C:\a`}) {
		t.Errorf("без конца списка: %v %v", p, ok)
	}
}

func TestDropEffect(t *testing.T) {
	if dropEffectIsCut(dropEffectBytes(false)) || !dropEffectIsCut(dropEffectBytes(true)) {
		t.Error("Preferred DropEffect: copy/cut перепутаны")
	}
	if dropEffectIsCut([]byte{2}) {
		t.Error("короткий блок принят")
	}
}

// Провайдер без файлов: ClipboardSetFiles кладёт пути текстом, ClipboardFiles
// честно отвечает «не файлы».
type textOnlyClipboard struct{ s string }

func (c *textOnlyClipboard) GetText() string  { return c.s }
func (c *textOnlyClipboard) SetText(s string) { c.s = s }

func TestClipboardFiles_Fallbacks(t *testing.T) {
	prev := GetClipboardProvider()
	defer SetClipboardProvider(prev)

	to := &textOnlyClipboard{}
	SetClipboardProvider(to)
	ClipboardSetFiles([]string{"/a", "/b"}, true)
	if to.s != "/a\n/b" {
		t.Errorf("запасной текст: %q", to.s)
	}
	if _, _, ok := ClipboardFiles(); ok {
		t.Error("провайдер без файлов ответил файлами")
	}

	UseMemoryClipboard()
	ClipboardSetFiles([]string{"/a", "/b"}, true)
	p, cut, ok := ClipboardFiles()
	if !ok || !cut || !reflect.DeepEqual(p, []string{"/a", "/b"}) || ClipboardGetText() != "/a\n/b" {
		t.Errorf("в памяти: %v %v %v %q", p, cut, ok, ClipboardGetText())
	}
	ClipboardSetText("x")
	if _, _, ok := ClipboardFiles(); ok {
		t.Error("текст не вытеснил файлы")
	}
}
