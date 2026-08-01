package window

import (
	"bytes"
	"reflect"
	"testing"
)

// TestDBusSigOf — вывод сигнатуры по значению Go.
func TestDBusSigOf(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{byte(1), "y"},
		{true, "b"},
		{int16(-1), "n"},
		{uint16(1), "q"},
		{int32(-1), "i"},
		{uint32(1), "u"},
		{int64(-1), "x"},
		{uint64(1), "t"},
		{1.5, "d"},
		{"s", "s"},
		{dbusObjectPath("/a"), "o"},
		{dbusSignature("sv"), "g"},
		{dbusVariant{Val: "x"}, "v"},
		{[]string{"a"}, "as"},
		{[]string(nil), "as"},
		{map[string]dbusVariant{}, "a{sv}"},
		{map[string]string{}, "a{ss}"},
		{dbusStruct{Fields: []any{"s", uint32(1)}}, "(su)"},
		{dbusArray{ElemSig: "(yv)"}, "a(yv)"},
		{[]byte{1, 2}, "ay"},
		{struct {
			A string
			B uint32
		}{}, "(su)"},
	}
	for _, c := range cases {
		got, err := dbusSigOf(c.val)
		if err != nil {
			t.Fatalf("dbusSigOf(%#v): %v", c.val, err)
		}
		if got != c.want {
			t.Errorf("dbusSigOf(%#v) = %q, want %q", c.val, got, c.want)
		}
	}
}

// TestDBusSigSplit — разбор сигнатур на полные типы.
func TestDBusSigSplit(t *testing.T) {
	cases := []struct {
		sig  string
		want []string
	}{
		{"susssasa{sv}i", []string{"s", "u", "s", "s", "s", "as", "a{sv}", "i"}},
		{"a(yv)", []string{"a(yv)"}},
		{"(so)a{s(ii)}", []string{"(so)", "a{s(ii)}"}},
		{"aai", []string{"aai"}},
		{"", nil},
	}
	for _, c := range cases {
		got, err := dbusSigSplit(c.sig)
		if err != nil {
			t.Fatalf("dbusSigSplit(%q): %v", c.sig, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("dbusSigSplit(%q) = %v, want %v", c.sig, got, c.want)
		}
	}
	if _, err := dbusSigSplit("a{sv"); err == nil {
		t.Error("незакрытая скобка должна быть ошибкой")
	}
	if _, err := dbusSigSplit("Z"); err == nil {
		t.Error("неизвестный код типа должен быть ошибкой")
	}
}

// TestDBusEncodeBytes — точные байты для базовых типов (выравнивание, NUL).
func TestDBusEncodeBytes(t *testing.T) {
	cases := []struct {
		name string
		sig  string
		val  any
		want []byte
	}{
		{"string", "s", "foo", []byte{3, 0, 0, 0, 'f', 'o', 'o', 0}},
		{"empty string", "s", "", []byte{0, 0, 0, 0, 0}},
		{"signature", "g", dbusSignature("a{sv}"), []byte{5, 'a', '{', 's', 'v', '}', 0}},
		{"bool true", "b", true, []byte{1, 0, 0, 0}},
		{"uint32", "u", uint32(0x01020304), []byte{4, 3, 2, 1}},
		{"int32 neg", "i", int32(-2), []byte{0xFE, 0xFF, 0xFF, 0xFF}},
		{"byte", "y", byte(7), []byte{7}},
	}
	for _, c := range cases {
		e := dbusEnc{}
		if err := e.encodeAs(c.sig, c.val); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(e.buf, c.want) {
			t.Errorf("%s: %v, want %v", c.name, e.buf, c.want)
		}
	}
}

// TestDBusEncodeAlignment — байт перед int64 дополняется до границы 8,
// массив выравнивает первый элемент, а длина массива padding НЕ включает.
func TestDBusEncodeAlignment(t *testing.T) {
	e := dbusEnc{}
	if err := e.encodeAs("y", byte(1)); err != nil {
		t.Fatal(err)
	}
	if err := e.encodeAs("x", int64(-1)); err != nil {
		t.Fatal(err)
	}
	want := []byte{1, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(e.buf, want) {
		t.Fatalf("y+x = %v, want %v", e.buf, want)
	}

	// byte, затем массив int64: длина (u32) на смещении 4, элементы — с 8.
	e = dbusEnc{}
	_ = e.encodeAs("y", byte(1))
	if err := e.encodeAs("ax", []int64{1}); err != nil {
		t.Fatal(err)
	}
	if got := e.buf[4]; got != 8 {
		t.Errorf("длина массива = %d, want 8 (padding не считается)", got)
	}
	if len(e.buf) != 16 {
		t.Errorf("len = %d, want 16", len(e.buf))
	}
}

// TestDBusRoundTripNotify — тело вызова Notify (susssasa{sv}i) переживает
// кодирование и разбор без потерь.
func TestDBusRoundTripNotify(t *testing.T) {
	hints := map[string]dbusVariant{
		"urgency":       {Sig: "y", Val: byte(2)},
		"desktop-entry": {Sig: "s", Val: "headless-gui"},
	}
	msg := &dbusMessage{
		Type:        dbusTypeMethodCall,
		Serial:      42,
		Path:        "/org/freedesktop/Notifications",
		Interface:   "org.freedesktop.Notifications",
		Member:      "Notify",
		Destination: "org.freedesktop.Notifications",
		Sig:         "susssasa{sv}i",
		Body: []any{
			"headless-gui", uint32(0), "dialog-information",
			"Заголовок", "Тело уведомления",
			[]string{"default", "Открыть"},
			hints, int32(5000),
		},
	}
	raw, err := msg.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if n, ok := dbusMessageLen(raw); !ok || n != len(raw) {
		t.Fatalf("dbusMessageLen = %d/%v, want %d", n, ok, len(raw))
	}
	got, err := dbusUnmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != msg.Type || got.Serial != msg.Serial || got.Path != msg.Path ||
		got.Interface != msg.Interface || got.Member != msg.Member ||
		got.Destination != msg.Destination || got.Sig != msg.Sig {
		t.Fatalf("заголовок не совпал: %+v", got)
	}
	if len(got.Body) != 8 {
		t.Fatalf("аргументов %d, want 8", len(got.Body))
	}
	if got.Body[0] != "headless-gui" || got.Body[3] != "Заголовок" || got.Body[7] != int32(5000) {
		t.Errorf("тело: %#v", got.Body)
	}
	if as, ok := got.Body[5].([]string); !ok || len(as) != 2 || as[1] != "Открыть" {
		t.Errorf("actions: %#v", got.Body[5])
	}
	hm, ok := got.Body[6].(map[string]any)
	if !ok {
		t.Fatalf("hints: %#v", got.Body[6])
	}
	if v, ok := hm["urgency"].(dbusVariant); !ok || v.Val != byte(2) {
		t.Errorf("urgency: %#v", hm["urgency"])
	}
	if v, ok := hm["desktop-entry"].(dbusVariant); !ok || v.Val != "headless-gui" {
		t.Errorf("desktop-entry: %#v", hm["desktop-entry"])
	}
}

// TestDBusRoundTripNested — вложенные структуры/массивы структур (AT-SPI).
func TestDBusRoundTripNested(t *testing.T) {
	body := []any{
		dbusStruct{Fields: []any{"org.a11y.atspi.Registry", dbusObjectPath("/org/a11y/atspi/accessible/root")}},
		dbusArray{ElemSig: "(so)", Items: []any{
			dbusStruct{Fields: []any{":1.42", dbusObjectPath("/a/1")}},
			dbusStruct{Fields: []any{":1.42", dbusObjectPath("/a/2")}},
		}},
		dbusArray{ElemSig: "v"}, // пустой массив вариантов
	}
	msg := &dbusMessage{Type: dbusTypeMethodReturn, Serial: 7, ReplySerial: 3, Sig: "(so)a(so)av", Body: body}
	raw, err := msg.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := dbusUnmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ReplySerial != 3 {
		t.Errorf("ReplySerial = %d", got.ReplySerial)
	}
	st, ok := got.Body[0].(dbusStruct)
	if !ok || st.Fields[0] != "org.a11y.atspi.Registry" || st.Fields[1] != dbusObjectPath("/org/a11y/atspi/accessible/root") {
		t.Errorf("struct: %#v", got.Body[0])
	}
	arr, ok := got.Body[1].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("array: %#v", got.Body[1])
	}
	second := arr[1].(dbusStruct)
	if second.Fields[1] != dbusObjectPath("/a/2") {
		t.Errorf("array[1]: %#v", second)
	}
	if av, ok := got.Body[2].([]any); !ok || len(av) != 0 {
		t.Errorf("пустой массив: %#v", got.Body[2])
	}
}

// TestDBusUnmarshalTruncated — обрезанное сообщение не паникует.
func TestDBusUnmarshalTruncated(t *testing.T) {
	msg := &dbusMessage{Type: dbusTypeSignal, Serial: 1, Path: "/x", Interface: "i.f", Member: "M", Sig: "s", Body: []any{"hello"}}
	raw, err := msg.marshal()
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n < len(raw); n++ {
		if _, err := dbusUnmarshal(raw[:n]); err == nil {
			t.Fatalf("обрезка до %d байт разобралась без ошибки", n)
		}
	}
}

// TestDBusSigMismatch — несовпадение сигнатуры и числа аргументов ловится.
func TestDBusSigMismatch(t *testing.T) {
	m := &dbusMessage{Type: dbusTypeMethodCall, Serial: 1, Sig: "ss", Body: []any{"one"}}
	if _, err := m.marshal(); err == nil {
		t.Error("ожидалась ошибка: аргументов меньше, чем в сигнатуре")
	}
	e := dbusEnc{}
	if err := e.encodeAs("s", 42); err == nil {
		t.Error("ожидалась ошибка: число вместо строки")
	}
	if err := e.encodeAs("u", -1); err == nil {
		t.Error("ожидалась ошибка: отрицательное в uint32")
	}
}

// TestDBusBigEndianDecode — читаем сообщение от отправителя с BE-порядком.
func TestDBusBigEndianDecode(t *testing.T) {
	m := &dbusMessage{Type: dbusTypeSignal, Serial: 0x11223344, Path: "/p", Interface: "i.f", Member: "M", Sig: "u", Body: []any{uint32(0xAABBCCDD)}}
	raw, err := m.marshal()
	if err != nil {
		t.Fatal(err)
	}
	be := beSwap(t, raw)
	got, err := dbusUnmarshal(be)
	if err != nil {
		t.Fatalf("unmarshal BE: %v", err)
	}
	if got.Serial != 0x11223344 || got.Member != "M" || got.Body[0] != uint32(0xAABBCCDD) {
		t.Errorf("BE разобрано неверно: %+v", got)
	}
}

// beSwap переписывает LE-сообщение в BE: разбирает LE и кодирует поля заново,
// переставляя байты каждого целочисленного поля. Проще — разобрать и собрать
// вручную не выйдет, поэтому меняем байты по месту, зная раскладку.
func beSwap(t *testing.T, raw []byte) []byte {
	t.Helper()
	out := append([]byte(nil), raw...)
	out[0] = 'B'
	d := &dbusDec{buf: raw, le: true}
	// Смещения целочисленных полей: bodyLen(4), serial(8), fieldsLen(12).
	swap4 := func(off int) {
		out[off], out[off+1], out[off+2], out[off+3] = out[off+3], out[off+2], out[off+1], out[off]
	}
	swap4(4)
	swap4(8)
	swap4(12)
	// Поля заголовка: строки/сигнатуры несут u32-длины, их тоже надо перевернуть.
	d.pos = 12
	fieldsLen, err := d.u32()
	if err != nil {
		t.Fatal(err)
	}
	end := 16 + int(fieldsLen)
	d.pos = 16
	for d.pos < end {
		if err := d.align(8); err != nil {
			t.Fatal(err)
		}
		d.pos++ // код поля (byte)
		vs, err := d.sig()
		if err != nil {
			t.Fatal(err)
		}
		switch vs {
		case "s", "o":
			if err := d.align(4); err != nil {
				t.Fatal(err)
			}
			swap4(d.pos)
			n, err := d.u32()
			if err != nil {
				t.Fatal(err)
			}
			d.pos += int(n) + 1
		case "g":
			n := int(raw[d.pos])
			d.pos += n + 2
		case "u":
			if err := d.align(4); err != nil {
				t.Fatal(err)
			}
			swap4(d.pos)
			d.pos += 4
		default:
			t.Fatalf("beSwap: неучтённый тип поля %q", vs)
		}
	}
	// Тело: единственный аргумент типа u в этом тесте.
	bodyStart := (end + 7) &^ 7
	swap4(bodyStart)
	return out
}
