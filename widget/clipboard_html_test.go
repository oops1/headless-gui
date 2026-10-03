package widget

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Тесты не трогают системный буфер обмена — он общий для всего рабочего стола.
// Проверяется сборка и разбор формата «HTML Format» и работа пакетных функций
// с подставным провайдером.

// headerOffset достаёт из собранных данных смещение по имени ключа.
func headerOffset(t *testing.T, data []byte, key string) int {
	t.Helper()
	for _, line := range strings.Split(string(data[:bytes.Index(data, []byte("<"))]), "\r\n") {
		if v, ok := strings.CutPrefix(line, key+":"); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				t.Fatalf("смещение %s = %q: %v", key, v, err)
			}
			return n
		}
	}
	t.Fatalf("в заголовке нет %s", key)
	return 0
}

// Смещения должны указывать на маркеры в БАЙТАХ, в том числе после кириллицы:
// каждая русская буква — два байта, и счёт в символах сдвинул бы фрагмент.
func TestBuildCFHTML_OffsetsAreBytes(t *testing.T) {
	for _, frag := range []string{
		"<b>hello</b>",
		"<h1>Заголовок</h1><p>Жирный <b>текст</b> и <a href=\"https://example.com\">ссылка</a></p>",
		"日本語 <i>😀</i>",
		"",
	} {
		data := BuildCFHTML(frag)

		sh := headerOffset(t, data, "StartHTML")
		eh := headerOffset(t, data, "EndHTML")
		sf := headerOffset(t, data, "StartFragment")
		ef := headerOffset(t, data, "EndFragment")

		if eh != len(data) {
			t.Errorf("%q: EndHTML = %d, длина данных %d", frag, eh, len(data))
		}
		if !strings.HasPrefix(string(data[sh:]), "<html>") {
			t.Errorf("%q: StartHTML не указывает на начало документа: %q", frag, data[sh:sh+10])
		}
		// Прямо перед фрагментом — маркер начала, сразу после — маркер конца.
		if !bytes.HasSuffix(data[:sf], []byte(cfHTMLStartMarker)) {
			t.Errorf("%q: StartFragment не после маркера начала", frag)
		}
		if !bytes.HasPrefix(data[ef:], []byte(cfHTMLEndMarker)) {
			t.Errorf("%q: EndFragment не перед маркером конца", frag)
		}
		if got := string(data[sf:ef]); got != frag {
			t.Errorf("фрагмент по смещениям = %q, ждал %q", got, frag)
		}
	}
}

// Заголовок обязан иметь вид, который ждут Word и браузеры: версия 0.9 и
// четыре смещения подряд.
func TestBuildCFHTML_HeaderShape(t *testing.T) {
	data := string(BuildCFHTML("<p>x</p>"))
	want := []string{"Version:0.9\r\n", "StartHTML:", "EndHTML:", "StartFragment:", "EndFragment:"}
	pos := 0
	for _, w := range want {
		i := strings.Index(data[pos:], w)
		if i < 0 {
			t.Fatalf("в заголовке нет %q (или порядок нарушен):\n%s", w, data)
		}
		pos += i + len(w)
	}
	if strings.IndexByte(data, 0) >= 0 {
		t.Error("NUL внутри данных: его добавляет тот, кто кладёт данные в буфер")
	}
}

// Круг «собрали — разобрали» возвращает исходный фрагмент побайтно.
func TestCFHTML_RoundTrip(t *testing.T) {
	for _, frag := range []string{
		"<b>hello</b>",
		"<h1>Привет, мир</h1><ul><li>раз</li><li>два</li></ul>",
		"строка\r\nс переводами\nстрок",
		"<p>эмодзи 😀 и 日本語</p>",
	} {
		got, ok := ParseCFHTML(BuildCFHTML(frag))
		if !ok || got != frag {
			t.Errorf("ParseCFHTML(BuildCFHTML(%q)) = %q, %v", frag, got, ok)
		}
		// С завершающим NUL, как лежит в системном буфере.
		got, ok = ParseCFHTML(append(BuildCFHTML(frag), 0, 0))
		if !ok || got != frag {
			t.Errorf("с NUL: %q, %v, ждал %q", got, ok, frag)
		}
	}
}

// Целый документ получает маркеры внутри тела, а не вторую обёртку.
func TestBuildCFHTML_FullDocument(t *testing.T) {
	doc := "<!DOCTYPE html><HTML><BODY class=\"x\"><p>Тело</p></BODY></HTML>"
	data := BuildCFHTML(doc)
	frag, ok := ParseCFHTML(data)
	if !ok || frag != "<p>Тело</p>" {
		t.Errorf("фрагмент = %q, %v, ждал только содержимое тела", frag, ok)
	}
	if strings.Count(string(data), "<html") > 0 && strings.Count(strings.ToLower(string(data)), "<html") != 1 {
		t.Error("документ обёрнут второй раз")
	}
}

// Маркеры, уже стоящие во входе, не должны давать два набора.
func TestBuildCFHTML_StripsForeignMarkers(t *testing.T) {
	data := BuildCFHTML("<!--StartFragment-->x<!--EndFragment-->")
	if n := strings.Count(string(data), cfHTMLStartMarker); n != 1 {
		t.Errorf("маркеров начала %d, ждал 1", n)
	}
	if got, _ := ParseCFHTML(data); got != "x" {
		t.Errorf("фрагмент = %q, ждал x", got)
	}
}

// Разбор терпим к тому, как пишут формат чужие приложения.
func TestParseCFHTML_Tolerant(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"голый HTML без заголовка", "<b>raw</b>", "<b>raw</b>", true},
		{"голый HTML с NUL", "<b>raw</b>\x00", "<b>raw</b>", true},
		{"пусто", "", "", false},
		{"только NUL", "\x00\x00", "", false},
		{
			"смещение за пределами данных",
			"Version:0.9\r\nStartHTML:0000000001\r\nEndHTML:9999999999\r\nStartFragment:0000000001\r\nEndFragment:9999999999\r\n<p>x</p>",
			"", false,
		},
		{
			"конец раньше начала",
			"Version:0.9\r\nStartHTML:0000000050\r\nEndHTML:0000000040\r\nStartFragment:0000000050\r\nEndFragment:0000000040\r\n<p>x</p>",
			"", false,
		},
		{
			"отрицательные смещения",
			"Version:0.9\r\nStartHTML:-1\r\nEndHTML:-1\r\nStartFragment:-1\r\nEndFragment:-1\r\n<p>x</p>",
			"", false,
		},
	}
	for _, c := range cases {
		got, ok := ParseCFHTML([]byte(c.in))
		if got != c.want || ok != c.ok {
			t.Errorf("%s: (%q, %v), ждал (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// Смещения без ведущих нулей и переводы строк LF — вручную посчитанный пример, как его пишут
// некоторые приложения.
func TestParseCFHTML_HandWritten(t *testing.T) {
	body := "<html><body><!--StartFragment--><b>Привет</b><!--EndFragment--></body></html>"
	hdrFmt := "Version:1.0\nStartHTML:%d\nEndHTML:%d\nStartFragment:%d\nEndFragment:%d\n"
	// Длина заголовка зависит от числа цифр, поэтому считаем итеративно.
	hlen := 0
	var data string
	for i := 0; i < 5; i++ {
		sf := hlen + strings.Index(body, "<b>")
		ef := hlen + strings.Index(body, "<!--EndFragment-->")
		data = fmt.Sprintf(hdrFmt, hlen, hlen+len(body), sf, ef) + body
		hlen = len(data) - len(body)
	}
	got, ok := ParseCFHTML([]byte(data))
	if !ok || got != "<b>Привет</b>" {
		t.Errorf("ParseCFHTML = %q, %v", got, ok)
	}
}

// Если указан только документ, без фрагмента, отдаём документ.
func TestParseCFHTML_DocumentWithoutFragment(t *testing.T) {
	body := "<p>doc</p>"
	data := "Version:0.9\r\nStartHTML:0000000051\r\nEndHTML:0000000061\r\n" + body
	// Подгоняем смещение под фактическую длину заголовка.
	start := len(data) - len(body)
	data = strings.Replace(data, "0000000051", padOffset(start), 1)
	data = strings.Replace(data, "0000000061", padOffset(start+len(body)), 1)
	got, ok := ParseCFHTML([]byte(data))
	if !ok || got != body {
		t.Errorf("ParseCFHTML = %q, %v", got, ok)
	}
}

// Мусор в начале данных или обрезанный заголовок не должны паниковать.
func TestParseCFHTML_NoPanic(t *testing.T) {
	for _, in := range []string{
		"Version:", "Version:0.9", "Version:0.9\r\nStartFragment:", "Version:0.9\r\nStartFragment:5\r\nEndFragment:3",
		"Version:0.9\r\nUnknown:1\r\n<p>", "\xff\xfe\x00",
	} {
		ParseCFHTML([]byte(in))
	}
}

// ─── Пакетные функции ───────────────────────────────────────────────────────

// plainOnlyClipboard — провайдер приложения, не знающий про HTML.
type plainOnlyClipboard struct{ text string }

func (c *plainOnlyClipboard) GetText() string  { return c.text }
func (c *plainOnlyClipboard) SetText(s string) { c.text = s }

// withClipboard подставляет провайдера на время теста.
func withClipboard(t *testing.T, p ClipboardProvider) {
	t.Helper()
	old := defaultClipboard
	SetClipboardProvider(p)
	t.Cleanup(func() { defaultClipboard = old })
}

// Провайдер без HTML: в буфер уходит простой текст, а HTML честно «нет».
func TestClipboardHTML_FallbackToPlainText(t *testing.T) {
	p := &plainOnlyClipboard{}
	withClipboard(t, p)

	SetClipboardHTML("<b>жирный</b>", "жирный")
	if p.text != "жирный" {
		t.Errorf("запасной путь положил %q, ждал простой текст", p.text)
	}
	if h, ok := ClipboardHTML(); ok || h != "" {
		t.Errorf("ClipboardHTML = %q, %v у провайдера без HTML", h, ok)
	}
}

// Провайдер с HTML получает оба представления, а SetText вытесняет оформление.
func TestClipboardHTML_MemoryProvider(t *testing.T) {
	withClipboard(t, &memoryClipboard{})

	SetClipboardHTML("<h1>Заголовок</h1>", "Заголовок")
	if h, ok := ClipboardHTML(); !ok || h != "<h1>Заголовок</h1>" {
		t.Errorf("ClipboardHTML = %q, %v", h, ok)
	}
	if got := ClipboardGetText(); got != "Заголовок" {
		t.Errorf("простой текст рядом = %q", got)
	}

	ClipboardSetText("новый")
	if h, ok := ClipboardHTML(); ok {
		t.Errorf("после SetText остался HTML %q от прошлого копирования", h)
	}
}
