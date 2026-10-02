package printing

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestIPPMarshalExactBytes — сборка проверяется по байтам, записанным вручную из
// RFC 8010, а не повторным вызовом той же логики: версия 2.0, операция
// Print-Job, request-id 7, группа операции с атрибутом-строкой и целым из двух
// значений (второе — с пустым именем), тег конца.
func TestIPPMarshalExactBytes(t *testing.T) {
	m := &ippMessage{Major: 2, Minor: 0, Code: ippOpPrintJob, RequestID: 7}
	g := m.group(ippTagOperation)
	g.add(ippTagCharset, "attributes-charset", "utf-8")
	g.Attrs = append(g.Attrs, ippAttribute{Name: "copies", Values: []ippValue{
		{ippTagInteger, []byte{0, 0, 0, 3}},
		{ippTagInteger, []byte{0, 0, 0, 5}},
	}})
	got, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := "\x02\x00" + "\x00\x02" + "\x00\x00\x00\x07" +
		"\x01" +
		"\x47\x00\x12attributes-charset\x00\x05utf-8" +
		"\x21\x00\x06copies\x00\x04\x00\x00\x00\x03" +
		"\x21\x00\x00\x00\x04\x00\x00\x00\x05" +
		"\x03"
	if string(got) != want {
		t.Errorf("байты:\n got %q\nwant %q", got, want)
	}
}

func TestIPPRequestPreamble(t *testing.T) {
	a := newIPPRequest(ippOpCUPSGetPrint)
	b := newIPPRequest(ippOpCUPSGetPrint)
	if a.RequestID == 0 || b.RequestID == 0 || a.RequestID == b.RequestID {
		t.Errorf("request-id: %d, %d (ноль зарезервирован, повторы нежелательны)", a.RequestID, b.RequestID)
	}
	if a.Major != 2 || a.Minor != 0 {
		t.Errorf("версия %d.%d", a.Major, a.Minor)
	}
	// RFC 8011, 4.1.4: сначала attributes-charset, затем attributes-natural-language.
	attrs := a.Groups[0].Attrs
	if a.Groups[0].Tag != ippTagOperation || len(attrs) != 2 ||
		attrs[0].Name != "attributes-charset" || attrs[1].Name != "attributes-natural-language" {
		t.Errorf("преамбула: %+v", a.Groups)
	}
	if attrs[0].Str() != "utf-8" || attrs[0].Values[0].Tag != ippTagCharset {
		t.Errorf("charset: %+v", attrs[0])
	}
}

func TestIPPRoundTrip(t *testing.T) {
	m := newIPPRequest(ippOpPrintJob)
	og := m.group(ippTagOperation)
	og.add(ippTagURI, "printer-uri", "ipp://localhost:631/printers/HP%20Laser")
	og.add(ippTagName, "job-name", "Отчёт за квартал")
	og.addMulti(ippTagKeyword, "requested-attributes", "printer-name", "printer-state")
	jg := m.group(ippTagJob)
	jg.addInt(ippTagInteger, "copies", 12)
	jg.addBool("fidelity", true)
	jg.addInt(ippTagEnum, "print-quality", 4)

	raw, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	doc := []byte("%PDF-1.4 данные документа")
	back, rest, err := parseIPP(append(append([]byte(nil), raw...), doc...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rest, doc) {
		t.Errorf("остаток после тега конца: %q", rest)
	}
	if back.Major != 2 || back.Code != ippOpPrintJob || back.RequestID != m.RequestID {
		t.Errorf("заголовок: %+v", back)
	}
	if len(back.Groups) != 2 || back.Groups[0].Tag != ippTagOperation || back.Groups[1].Tag != ippTagJob {
		t.Fatalf("группы: %+v", back.Groups)
	}
	if got := back.find(ippTagOperation, "job-name").Str(); got != "Отчёт за квартал" {
		t.Errorf("job-name: %q", got)
	}
	if got := back.find(ippTagOperation, "printer-uri").Str(); got != "ipp://localhost:631/printers/HP%20Laser" {
		t.Errorf("printer-uri: %q", got)
	}
	ra := back.find(ippTagOperation, "requested-attributes")
	if ra == nil || len(ra.Values) != 2 || string(ra.Values[1].Data) != "printer-state" {
		t.Errorf("requested-attributes: %+v", ra)
	}
	if v, ok := back.find(ippTagJob, "copies").Int(); !ok || v != 12 {
		t.Errorf("copies: %v %v", v, ok)
	}
	if v, ok := back.find(ippTagJob, "fidelity").Bool(); !ok || !v {
		t.Errorf("fidelity: %v %v", v, ok)
	}
	if v, ok := back.find(ippTagJob, "print-quality").Int(); !ok || v != 4 {
		t.Errorf("print-quality: %v %v", v, ok)
	}
	// Безопасность nil-цепочек.
	if back.find(ippTagPrinter, "нет такого").Str() != "" {
		t.Error("Str у nil-атрибута не пуст")
	}
	if _, ok := back.find(ippTagJob, "нет").Int(); ok {
		t.Error("Int у nil-атрибута вернул ok")
	}
}

// TestIPPParseNeverPanics — любое обрезание корректного сообщения даёт ошибку,
// а не панику и не «успех».
func TestIPPParseNeverPanics(t *testing.T) {
	m := newIPPRequest(ippOpCUPSGetPrint)
	m.group(ippTagOperation).addMulti(ippTagKeyword, "requested-attributes", "a", "b", "c")
	m.group(ippTagPrinter).addInt(ippTagInteger, "x", 1)
	raw, _ := m.Marshal()
	for n := 0; n < len(raw); n++ {
		if _, _, err := parseIPP(raw[:n]); err == nil {
			t.Errorf("обрезано до %d из %d байт, а ошибки нет", n, len(raw))
		}
	}
	if _, _, err := parseIPP(raw); err != nil {
		t.Errorf("целое сообщение: %v", err)
	}
	// Мусор.
	for _, junk := range [][]byte{
		nil, {0}, bytes.Repeat([]byte{0xff}, 40),
		append(append([]byte(nil), raw[:8]...), 0x01, 0x47, 0xff, 0xff),   // длина имени за границей
		append(append([]byte(nil), raw[:8]...), 0x47, 0, 1, 'a', 0, 0, 3), // атрибут вне группы
	} {
		if _, _, err := parseIPP(junk); err == nil {
			t.Errorf("мусор %v принят", junk)
		}
	}
}

// TestIPPParseCollectionsFlat — вложенные коллекции (в ответах CUPS это
// media-col-database и подобное) разбираются плоским потоком и не ломают
// чтение следующих атрибутов.
func TestIPPParseCollectionsFlat(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\x02\x00\x00\x00\x00\x00\x00\x01")
	b.WriteString("\x04")
	// begCollection (0x34) с именем "media-col", пустое значение
	b.WriteString("\x34\x00\x09media-col\x00\x00")
	// memberAttrName (0x4a): пустое имя, значение — имя члена
	b.WriteString("\x4a\x00\x00\x00\x0amedia-size")
	// begCollection без имени, вложенная
	b.WriteString("\x34\x00\x00\x00\x00")
	b.WriteString("\x21\x00\x00\x00\x04\x00\x00\x52\x08")
	b.WriteString("\x37\x00\x00\x00\x00") // endCollection
	b.WriteString("\x37\x00\x00\x00\x00") // endCollection
	b.WriteString("\x42\x00\x0cprinter-name\x00\x03LJ4")
	b.WriteString("\x03")
	m, _, err := parseIPP(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := m.find(ippTagPrinter, "printer-name").Str(); got != "LJ4" {
		t.Errorf("printer-name после коллекции: %q", got)
	}
}

func TestIPPMarshalRejectsBad(t *testing.T) {
	bad := map[string]*ippMessage{
		"атрибут без значений": {Groups: []ippGroup{{Tag: ippTagOperation, Attrs: []ippAttribute{{Name: "x"}}}}},
		"пустое имя":           {Groups: []ippGroup{{Tag: ippTagOperation, Attrs: []ippAttribute{{Name: "", Values: []ippValue{{ippTagKeyword, nil}}}}}}},
		"тег группы ≥ 0x10":    {Groups: []ippGroup{{Tag: 0x45}}},
		"тег значения < 0x10":  {Groups: []ippGroup{{Tag: ippTagOperation, Attrs: []ippAttribute{{Name: "x", Values: []ippValue{{0x03, nil}}}}}}},
		"значение > 65535":     {Groups: []ippGroup{{Tag: ippTagOperation, Attrs: []ippAttribute{{Name: "x", Values: []ippValue{{ippTagText, make([]byte, 70000)}}}}}}},
	}
	for name, m := range bad {
		if _, err := m.Marshal(); err == nil {
			t.Errorf("%s: ошибки нет", name)
		}
	}
}

func TestIPPStatusCheck(t *testing.T) {
	ok := &ippMessage{Code: ippStatusOK}
	if err := ok.check(); err != nil {
		t.Errorf("successful-ok: %v", err)
	}
	if err := (&ippMessage{Code: ippStatusOKSubstituted}).check(); err != nil {
		t.Errorf("успех с заменой атрибутов — это успех: %v", err)
	}
	m := &ippMessage{Code: ippStatusClientNotFound, Groups: []ippGroup{{Tag: ippTagOperation}}}
	m.group(ippTagOperation).add(ippTagText, "status-message", "no such printer")
	err := m.check()
	var ie *ippError
	if !errors.As(err, &ie) || ie.Code != ippStatusClientNotFound {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "client-error-not-found") || !strings.Contains(err.Error(), "no such printer") {
		t.Errorf("текст ошибки: %q", err)
	}
	if got := ippStatusText(0x1234); got != "0x1234" {
		t.Errorf("неизвестный код: %q", got)
	}
}
