// dbus.go — D-Bus «с нуля»: wire-формат сообщений на чистом Go.
//
// Правило проекта — zero new deps: готовую библиотеку (godbus) тянуть нельзя,
// поэтому протокол реализован здесь целиком, как это уже сделано для X11 и
// Wayland (голый сокет + разбор байтов). Файл содержит только КОДИРОВАНИЕ —
// он платформенно-независим и компилируется везде, чтобы маршалинг покрывался
// обычными unit-тестами (в т.ч. на Windows). Транспорт (unix-сокет, SASL,
// маршрутизация ответов) живёт в dbus_conn_linux.go.
//
// Поддержан полный набор типов D-Bus, нужный уведомлениям и AT-SPI:
// y b n q i u x t d s o g v h, массивы, словари a{..}, структуры (..).
// Модель значений в Go:
//
//	byte→uint8, boolean→bool, int16/uint16/int32/uint32/int64/uint64,
//	double→float64, string→string, object path→dbusObjectPath,
//	signature→dbusSignature, variant→dbusVariant, struct→dbusStruct,
//	массив→любой Go-слайс или dbusArray (когда тип элемента задаётся явно),
//	словарь→любая Go-карта.
//
// Порядок байт всегда little-endian ('l'): D-Bus разрешает выбирать его
// отправителю, а все машины, на которых нас запускают, LE.
package window

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// ─── Типы значений ───────────────────────────────────────────────────────────

// dbusObjectPath — путь объекта D-Bus (тип 'o'), например /org/a11y/atspi/accessible/root.
type dbusObjectPath string

// dbusSignature — сигнатура типа D-Bus (тип 'g').
type dbusSignature string

// dbusVariant — значение с сигнатурой (тип 'v'). Пустая Sig → выводится из Val.
type dbusVariant struct {
	Sig string
	Val any
}

// dbusStruct — структура D-Bus (тип '(...)'), поля в порядке объявления.
type dbusStruct struct {
	Fields []any
}

// dbusArray — массив с ЯВНО заданным типом элемента. Нужен там, где Go-слайс
// не годится: пустой массив вариантов, массив структур разной формы и т.п.
type dbusArray struct {
	ElemSig string
	Items   []any
}

// dbusDictEntry — пара словаря при ДЕкодировании массива a{..} с не-строковым
// ключом (строковые ключи собираются в map[string]any).
type dbusDictEntry struct {
	Key, Val any
}

// dbusUnixFD — индекс файлового дескриптора (тип 'h'). Мы дескрипторы не
// передаём, но должны уметь их пропускать в чужих сообщениях.
type dbusUnixFD uint32

// ─── Сигнатуры ───────────────────────────────────────────────────────────────

// dbusSigOf выводит сигнатуру D-Bus по значению Go.
func dbusSigOf(v any) (string, error) {
	switch t := v.(type) {
	case dbusVariant:
		return "v", nil
	case dbusStruct:
		var sb strings.Builder
		sb.WriteByte('(')
		for _, f := range t.Fields {
			s, err := dbusSigOf(f)
			if err != nil {
				return "", err
			}
			sb.WriteString(s)
		}
		sb.WriteByte(')')
		return sb.String(), nil
	case dbusArray:
		if t.ElemSig == "" {
			return "", errors.New("dbus: dbusArray без ElemSig")
		}
		return "a" + t.ElemSig, nil
	}
	if v == nil {
		return "", errors.New("dbus: nil без типа")
	}
	return dbusSigOfType(reflect.TypeOf(v))
}

// dbusSigOfType выводит сигнатуру по типу Go — работает и для пустых
// слайсов/карт, где значение не подсказывает тип элемента.
func dbusSigOfType(t reflect.Type) (string, error) {
	switch t {
	case reflect.TypeOf(dbusObjectPath("")):
		return "o", nil
	case reflect.TypeOf(dbusSignature("")):
		return "g", nil
	case reflect.TypeOf(dbusVariant{}):
		return "v", nil
	case reflect.TypeOf(dbusUnixFD(0)):
		return "h", nil
	}
	switch t.Kind() {
	case reflect.Uint8:
		return "y", nil
	case reflect.Bool:
		return "b", nil
	case reflect.Int16:
		return "n", nil
	case reflect.Uint16:
		return "q", nil
	case reflect.Int32:
		return "i", nil
	case reflect.Uint32:
		return "u", nil
	case reflect.Int64:
		return "x", nil
	case reflect.Uint64:
		return "t", nil
	case reflect.Float64:
		return "d", nil
	case reflect.String:
		return "s", nil
	case reflect.Slice, reflect.Array:
		es, err := dbusSigOfType(t.Elem())
		if err != nil {
			return "", err
		}
		return "a" + es, nil
	case reflect.Map:
		ks, err := dbusSigOfType(t.Key())
		if err != nil {
			return "", err
		}
		vs, err := dbusSigOfType(t.Elem())
		if err != nil {
			return "", err
		}
		return "a{" + ks + vs + "}", nil
	case reflect.Struct:
		var sb strings.Builder
		sb.WriteByte('(')
		for i := 0; i < t.NumField(); i++ {
			fs, err := dbusSigOfType(t.Field(i).Type)
			if err != nil {
				return "", err
			}
			sb.WriteString(fs)
		}
		sb.WriteByte(')')
		return sb.String(), nil
	case reflect.Interface:
		return "", errors.New("dbus: тип элемента не выводится из interface{} — используйте dbusArray")
	}
	return "", fmt.Errorf("dbus: неподдерживаемый тип %s", t)
}

// dbusSigNext возвращает индекс ПОСЛЕ первого полного типа в sig[i:].
// Например для "a{sv}i" при i=0 вернёт 5.
func dbusSigNext(sig string, i int) (int, error) {
	if i >= len(sig) {
		return 0, errors.New("dbus: обрыв сигнатуры")
	}
	switch sig[i] {
	case 'y', 'b', 'n', 'q', 'i', 'u', 'x', 't', 'd', 's', 'o', 'g', 'v', 'h':
		return i + 1, nil
	case 'a':
		return dbusSigNext(sig, i+1)
	case '(', '{':
		open, close := sig[i], byte(')')
		if open == '{' {
			close = '}'
		}
		depth := 0
		for j := i; j < len(sig); j++ {
			switch sig[j] {
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return j + 1, nil
				}
			}
		}
		return 0, errors.New("dbus: незакрытая скобка в сигнатуре")
	}
	return 0, fmt.Errorf("dbus: неизвестный код типа %q", sig[i])
}

// dbusSigSplit разбивает сигнатуру на список полных типов.
func dbusSigSplit(sig string) ([]string, error) {
	var out []string
	for i := 0; i < len(sig); {
		j, err := dbusSigNext(sig, i)
		if err != nil {
			return nil, err
		}
		out = append(out, sig[i:j])
		i = j
	}
	return out, nil
}

// dbusAlignOf — выравнивание значения указанного типа (в байтах).
func dbusAlignOf(code byte) int {
	switch code {
	case 'y', 'g', 'v':
		return 1
	case 'n', 'q':
		return 2
	case 'b', 'i', 'u', 's', 'o', 'a', 'h':
		return 4
	case 'x', 't', 'd', '(', '{':
		return 8
	}
	return 1
}

// ─── Кодирование ─────────────────────────────────────────────────────────────

// dbusEnc — буфер кодирования. Выравнивание считается от начала буфера;
// тело сообщения всегда начинается со смещения, кратного 8, поэтому его
// можно кодировать отдельным буфером с нуля.
type dbusEnc struct{ buf []byte }

func (e *dbusEnc) align(n int) {
	for len(e.buf)%n != 0 {
		e.buf = append(e.buf, 0)
	}
}

func (e *dbusEnc) putByte(b byte)  { e.buf = append(e.buf, b) }
func (e *dbusEnc) putU16(v uint16) { e.align(2); e.buf = binary.LittleEndian.AppendUint16(e.buf, v) }
func (e *dbusEnc) putU32(v uint32) { e.align(4); e.buf = binary.LittleEndian.AppendUint32(e.buf, v) }
func (e *dbusEnc) putU64(v uint64) { e.align(8); e.buf = binary.LittleEndian.AppendUint64(e.buf, v) }

// putStr кодирует string/object path: uint32-длина + байты + NUL.
func (e *dbusEnc) putStr(s string) {
	e.putU32(uint32(len(s)))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
}

// putSig кодирует signature: byte-длина + байты + NUL.
func (e *dbusEnc) putSig(s string) {
	e.buf = append(e.buf, byte(len(s)))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
}

// encode кодирует одно значение (тип выводится из значения).
func (e *dbusEnc) encode(v any) error {
	sig, err := dbusSigOf(v)
	if err != nil {
		return err
	}
	return e.encodeAs(sig, v)
}

// encodeAs кодирует значение по ЗАДАННОЙ сигнатуре: так числовые литералы
// (int) приводятся к нужному типу, а пустые массивы получают верный тип элемента.
func (e *dbusEnc) encodeAs(sig string, v any) error {
	if sig == "" {
		return errors.New("dbus: пустая сигнатура")
	}
	switch sig[0] {
	case 'y':
		b, err := dbusToUint(v)
		if err != nil {
			return err
		}
		e.putByte(byte(b))
	case 'b':
		bv, ok := v.(bool)
		if !ok {
			return fmt.Errorf("dbus: ожидался bool, получен %T", v)
		}
		var u uint32
		if bv {
			u = 1
		}
		e.putU32(u)
	case 'n':
		i, err := dbusToInt(v)
		if err != nil {
			return err
		}
		e.putU16(uint16(int16(i)))
	case 'q':
		u, err := dbusToUint(v)
		if err != nil {
			return err
		}
		e.putU16(uint16(u))
	case 'i':
		i, err := dbusToInt(v)
		if err != nil {
			return err
		}
		e.putU32(uint32(int32(i)))
	case 'u', 'h':
		u, err := dbusToUint(v)
		if err != nil {
			return err
		}
		e.putU32(uint32(u))
	case 'x':
		i, err := dbusToInt(v)
		if err != nil {
			return err
		}
		e.putU64(uint64(i))
	case 't':
		u, err := dbusToUint(v)
		if err != nil {
			return err
		}
		e.putU64(u)
	case 'd':
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("dbus: ожидался float64, получен %T", v)
		}
		e.putU64(math.Float64bits(f))
	case 's', 'o':
		s, err := dbusToString(v)
		if err != nil {
			return err
		}
		e.putStr(s)
	case 'g':
		s, err := dbusToString(v)
		if err != nil {
			return err
		}
		e.putSig(s)
	case 'v':
		return e.encodeVariant(v)
	case 'a':
		return e.encodeArray(sig, v)
	case '(':
		return e.encodeStruct(sig, v)
	default:
		return fmt.Errorf("dbus: нечего кодировать для %q", sig)
	}
	return nil
}

func (e *dbusEnc) encodeVariant(v any) error {
	var sig string
	var val any
	switch t := v.(type) {
	case dbusVariant:
		sig, val = t.Sig, t.Val
		if sig == "" {
			s, err := dbusSigOf(t.Val)
			if err != nil {
				return err
			}
			sig = s
		}
	default:
		s, err := dbusSigOf(v)
		if err != nil {
			return err
		}
		sig, val = s, v
	}
	e.putSig(sig)
	return e.encodeAs(sig, val)
}

// encodeArray кодирует массив/словарь: uint32-длина ТЕЛА (без выравнивания
// перед первым элементом) + элементы.
func (e *dbusEnc) encodeArray(sig string, v any) error {
	elemSig := sig[1:]
	if elemSig == "" {
		return errors.New("dbus: массив без типа элемента")
	}
	e.putU32(0) // место под длину
	lenPos := len(e.buf) - 4
	e.align(dbusAlignOf(elemSig[0]))
	start := len(e.buf)

	switch t := v.(type) {
	case dbusArray:
		for _, it := range t.Items {
			if err := e.encodeAs(elemSig, it); err != nil {
				return err
			}
		}
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				if err := e.encodeAs(elemSig, rv.Index(i).Interface()); err != nil {
					return err
				}
			}
		case reflect.Map:
			if elemSig[0] != '{' {
				return fmt.Errorf("dbus: карта кодируется в %q", sig)
			}
			kSig, err := dbusSigNext(elemSig, 1)
			if err != nil {
				return err
			}
			keySig := elemSig[1:kSig]
			valSig := elemSig[kSig : len(elemSig)-1]
			// Итерация карты в Go не упорядочена; D-Bus порядок пар не
			// оговаривает, но детерминированный вывод удобен для тестов.
			keys := rv.MapKeys()
			dbusSortValues(keys)
			for _, k := range keys {
				e.align(8)
				if err := e.encodeAs(keySig, k.Interface()); err != nil {
					return err
				}
				if err := e.encodeAs(valSig, rv.MapIndex(k).Interface()); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("dbus: %T не массив", v)
		}
	}

	body := len(e.buf) - start
	if body > 67108864 {
		return errors.New("dbus: массив длиннее 64 МиБ")
	}
	binary.LittleEndian.PutUint32(e.buf[lenPos:], uint32(body))
	return nil
}

func (e *dbusEnc) encodeStruct(sig string, v any) error {
	inner := sig[1 : len(sig)-1]
	fields, err := dbusSigSplit(inner)
	if err != nil {
		return err
	}
	e.align(8)
	var vals []any
	switch t := v.(type) {
	case dbusStruct:
		vals = t.Fields
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() != reflect.Struct {
			return fmt.Errorf("dbus: %T не структура", v)
		}
		for i := 0; i < rv.NumField(); i++ {
			vals = append(vals, rv.Field(i).Interface())
		}
	}
	if len(vals) != len(fields) {
		return fmt.Errorf("dbus: полей %d, а в сигнатуре %q — %d", len(vals), sig, len(fields))
	}
	for i, f := range fields {
		if err := e.encodeAs(f, vals[i]); err != nil {
			return err
		}
	}
	return nil
}

// dbusSortValues упорядочивает ключи карты (строки/числа) для стабильного вывода.
func dbusSortValues(vals []reflect.Value) {
	less := func(a, b reflect.Value) bool {
		switch a.Kind() {
		case reflect.String:
			return a.String() < b.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return a.Int() < b.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return a.Uint() < b.Uint()
		}
		return false
	}
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && less(vals[j], vals[j-1]); j-- {
			vals[j], vals[j-1] = vals[j-1], vals[j]
		}
	}
}

// dbusToUint/-Int/-String принимают любые числовые/строковые типы Go —
// вызывающему коду не нужно приводить литералы вручную.
func dbusToUint(v any) (uint64, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if rv.Int() < 0 {
			return 0, fmt.Errorf("dbus: отрицательное %d в беззнаковом поле", rv.Int())
		}
		return uint64(rv.Int()), nil
	}
	return 0, fmt.Errorf("dbus: ожидалось целое, получен %T", v)
}

func dbusToInt(v any) (int64, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint()), nil
	}
	return 0, fmt.Errorf("dbus: ожидалось целое, получен %T", v)
}

func dbusToString(v any) (string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.String {
		return rv.String(), nil
	}
	return "", fmt.Errorf("dbus: ожидалась строка, получен %T", v)
}

// ─── Декодирование ───────────────────────────────────────────────────────────

type dbusDec struct {
	buf []byte
	pos int
	le  bool // порядок байт отправителя
}

func (d *dbusDec) order() binary.ByteOrder {
	if d.le {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func (d *dbusDec) align(n int) error {
	for d.pos%n != 0 {
		if d.pos >= len(d.buf) {
			return errors.New("dbus: обрыв на выравнивании")
		}
		d.pos++
	}
	return nil
}

func (d *dbusDec) take(n int) ([]byte, error) {
	if d.pos+n > len(d.buf) {
		return nil, errors.New("dbus: обрыв данных")
	}
	b := d.buf[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

func (d *dbusDec) u16() (uint16, error) {
	if err := d.align(2); err != nil {
		return 0, err
	}
	b, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return d.order().Uint16(b), nil
}

func (d *dbusDec) u32() (uint32, error) {
	if err := d.align(4); err != nil {
		return 0, err
	}
	b, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return d.order().Uint32(b), nil
}

func (d *dbusDec) u64() (uint64, error) {
	if err := d.align(8); err != nil {
		return 0, err
	}
	b, err := d.take(8)
	if err != nil {
		return 0, err
	}
	return d.order().Uint64(b), nil
}

func (d *dbusDec) str() (string, error) {
	n, err := d.u32()
	if err != nil {
		return "", err
	}
	b, err := d.take(int(n) + 1) // +NUL
	if err != nil {
		return "", err
	}
	return string(b[:n]), nil
}

func (d *dbusDec) sig() (string, error) {
	b, err := d.take(1)
	if err != nil {
		return "", err
	}
	n := int(b[0])
	s, err := d.take(n + 1)
	if err != nil {
		return "", err
	}
	return string(s[:n]), nil
}

// decode читает одно значение указанного типа.
func (d *dbusDec) decode(sig string) (any, error) {
	if sig == "" {
		return nil, errors.New("dbus: пустая сигнатура")
	}
	switch sig[0] {
	case 'y':
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return b[0], nil
	case 'b':
		u, err := d.u32()
		return u != 0, err
	case 'n':
		u, err := d.u16()
		return int16(u), err
	case 'q':
		return d.u16()
	case 'i':
		u, err := d.u32()
		return int32(u), err
	case 'u':
		return d.u32()
	case 'h':
		u, err := d.u32()
		return dbusUnixFD(u), err
	case 'x':
		u, err := d.u64()
		return int64(u), err
	case 't':
		return d.u64()
	case 'd':
		u, err := d.u64()
		return math.Float64frombits(u), err
	case 's':
		return d.str()
	case 'o':
		s, err := d.str()
		return dbusObjectPath(s), err
	case 'g':
		s, err := d.sig()
		return dbusSignature(s), err
	case 'v':
		vs, err := d.sig()
		if err != nil {
			return nil, err
		}
		val, err := d.decode(vs)
		if err != nil {
			return nil, err
		}
		return dbusVariant{Sig: vs, Val: val}, nil
	case 'a':
		return d.decodeArray(sig)
	case '(':
		return d.decodeStruct(sig)
	}
	return nil, fmt.Errorf("dbus: неизвестный код типа %q", sig[0])
}

func (d *dbusDec) decodeArray(sig string) (any, error) {
	elemSig := sig[1:]
	n, err := d.u32()
	if err != nil {
		return nil, err
	}
	if elemSig == "" {
		return nil, errors.New("dbus: массив без типа элемента")
	}
	if err := d.align(dbusAlignOf(elemSig[0])); err != nil {
		return nil, err
	}
	end := d.pos + int(n)
	if end > len(d.buf) {
		return nil, errors.New("dbus: массив выходит за границу сообщения")
	}

	// Словарь: строковые ключи собираем в map[string]any (типовой случай a{sv}),
	// иначе — список пар.
	if elemSig[0] == '{' {
		kEnd, err := dbusSigNext(elemSig, 1)
		if err != nil {
			return nil, err
		}
		keySig := elemSig[1:kEnd]
		valSig := elemSig[kEnd : len(elemSig)-1]
		strKey := keySig == "s" || keySig == "o" || keySig == "g"
		m := map[string]any{}
		var pairs []dbusDictEntry
		for d.pos < end {
			if err := d.align(8); err != nil {
				return nil, err
			}
			k, err := d.decode(keySig)
			if err != nil {
				return nil, err
			}
			v, err := d.decode(valSig)
			if err != nil {
				return nil, err
			}
			if strKey {
				m[fmt.Sprint(k)] = v
			} else {
				pairs = append(pairs, dbusDictEntry{Key: k, Val: v})
			}
		}
		d.pos = end
		if strKey {
			return m, nil
		}
		return pairs, nil
	}

	// Массив строк — самый частый случай, отдаём типизированным.
	if elemSig == "s" {
		var out []string
		for d.pos < end {
			s, err := d.str()
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		d.pos = end
		return out, nil
	}
	if elemSig == "y" {
		b, err := d.take(int(n))
		if err != nil {
			return nil, err
		}
		out := make([]byte, len(b))
		copy(out, b)
		return out, nil
	}
	out := []any{}
	for d.pos < end {
		v, err := d.decode(elemSig)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	d.pos = end
	return out, nil
}

func (d *dbusDec) decodeStruct(sig string) (any, error) {
	fields, err := dbusSigSplit(sig[1 : len(sig)-1])
	if err != nil {
		return nil, err
	}
	if err := d.align(8); err != nil {
		return nil, err
	}
	st := dbusStruct{}
	for _, f := range fields {
		v, err := d.decode(f)
		if err != nil {
			return nil, err
		}
		st.Fields = append(st.Fields, v)
	}
	return st, nil
}

// ─── Сообщения ───────────────────────────────────────────────────────────────

// Типы сообщений D-Bus.
const (
	dbusTypeMethodCall   = 1
	dbusTypeMethodReturn = 2
	dbusTypeError        = 3
	dbusTypeSignal       = 4
)

// Флаги заголовка.
const (
	dbusFlagNoReplyExpected = 0x1
	dbusFlagNoAutoStart     = 0x2
)

// Коды полей заголовка.
const (
	dbusFieldPath        = 1
	dbusFieldInterface   = 2
	dbusFieldMember      = 3
	dbusFieldErrorName   = 4
	dbusFieldReplySerial = 5
	dbusFieldDestination = 6
	dbusFieldSender      = 7
	dbusFieldSignature   = 8
	dbusFieldUnixFDs     = 9
)

// dbusMessage — сообщение D-Bus (заголовок + тело).
type dbusMessage struct {
	Type        byte
	Flags       byte
	Serial      uint32
	Path        string
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Sig         string
	Body        []any
}

// marshal собирает сообщение в байты (little-endian).
func (m *dbusMessage) marshal() ([]byte, error) {
	// Тело и его сигнатура.
	be := dbusEnc{}
	sig := m.Sig
	if sig == "" {
		var sb strings.Builder
		for _, a := range m.Body {
			s, err := dbusSigOf(a)
			if err != nil {
				return nil, err
			}
			sb.WriteString(s)
		}
		sig = sb.String()
	}
	sigs, err := dbusSigSplit(sig)
	if err != nil {
		return nil, err
	}
	if len(sigs) != len(m.Body) {
		return nil, fmt.Errorf("dbus: сигнатура %q не соответствует %d аргументам", sig, len(m.Body))
	}
	for i, s := range sigs {
		if err := be.encodeAs(s, m.Body[i]); err != nil {
			return nil, err
		}
	}

	// Поля заголовка: массив структур (yv).
	fields := dbusArray{ElemSig: "(yv)"}
	add := func(code byte, sig string, val any) {
		fields.Items = append(fields.Items, dbusStruct{Fields: []any{code, dbusVariant{Sig: sig, Val: val}}})
	}
	if m.Path != "" {
		add(dbusFieldPath, "o", dbusObjectPath(m.Path))
	}
	if m.Interface != "" {
		add(dbusFieldInterface, "s", m.Interface)
	}
	if m.Member != "" {
		add(dbusFieldMember, "s", m.Member)
	}
	if m.ErrorName != "" {
		add(dbusFieldErrorName, "s", m.ErrorName)
	}
	if m.ReplySerial != 0 {
		add(dbusFieldReplySerial, "u", m.ReplySerial)
	}
	if m.Destination != "" {
		add(dbusFieldDestination, "s", m.Destination)
	}
	if m.Sender != "" {
		add(dbusFieldSender, "s", m.Sender)
	}
	if sig != "" {
		add(dbusFieldSignature, "g", dbusSignature(sig))
	}

	he := dbusEnc{}
	he.putByte('l')
	he.putByte(m.Type)
	he.putByte(m.Flags)
	he.putByte(1) // версия протокола
	he.putU32(uint32(len(be.buf)))
	he.putU32(m.Serial)
	if err := he.encodeAs("a(yv)", fields); err != nil {
		return nil, err
	}
	he.align(8) // тело начинается с границы 8 байт
	return append(he.buf, be.buf...), nil
}

// dbusFixedHeader — размер несменяемой части заголовка (до массива полей).
const dbusFixedHeader = 16

// dbusMessageLen возвращает полную длину сообщения по первым байтам буфера
// (нужно >= 16 байт). ok=false — данных ещё мало.
func dbusMessageLen(buf []byte) (int, bool) {
	if len(buf) < dbusFixedHeader {
		return 0, false
	}
	var ord binary.ByteOrder = binary.LittleEndian
	if buf[0] == 'B' {
		ord = binary.BigEndian
	}
	bodyLen := ord.Uint32(buf[4:8])
	fieldsLen := ord.Uint32(buf[12:16])
	hdr := dbusFixedHeader + int(fieldsLen)
	hdr = (hdr + 7) &^ 7
	return hdr + int(bodyLen), true
}

// dbusUnmarshal разбирает одно сообщение целиком.
func dbusUnmarshal(buf []byte) (*dbusMessage, error) {
	if len(buf) < dbusFixedHeader {
		return nil, errors.New("dbus: сообщение короче заголовка")
	}
	d := &dbusDec{buf: buf, le: buf[0] == 'l'}
	if buf[0] != 'l' && buf[0] != 'B' {
		return nil, fmt.Errorf("dbus: неизвестный порядок байт %q", buf[0])
	}
	m := &dbusMessage{Type: buf[1], Flags: buf[2]}
	if buf[3] != 1 {
		return nil, fmt.Errorf("dbus: версия протокола %d", buf[3])
	}
	d.pos = 4
	bodyLen, err := d.u32()
	if err != nil {
		return nil, err
	}
	if m.Serial, err = d.u32(); err != nil {
		return nil, err
	}
	fv, err := d.decode("a(yv)")
	if err != nil {
		return nil, err
	}
	items, _ := fv.([]any)
	for _, it := range items {
		st, ok := it.(dbusStruct)
		if !ok || len(st.Fields) != 2 {
			continue
		}
		code, _ := st.Fields[0].(byte)
		vr, _ := st.Fields[1].(dbusVariant)
		switch code {
		case dbusFieldPath:
			m.Path = fmt.Sprint(vr.Val)
		case dbusFieldInterface:
			m.Interface, _ = vr.Val.(string)
		case dbusFieldMember:
			m.Member, _ = vr.Val.(string)
		case dbusFieldErrorName:
			m.ErrorName, _ = vr.Val.(string)
		case dbusFieldReplySerial:
			m.ReplySerial, _ = vr.Val.(uint32)
		case dbusFieldDestination:
			m.Destination, _ = vr.Val.(string)
		case dbusFieldSender:
			m.Sender, _ = vr.Val.(string)
		case dbusFieldSignature:
			m.Sig = fmt.Sprint(vr.Val)
		}
	}
	if err := d.align(8); err != nil {
		return nil, err
	}
	if d.pos+int(bodyLen) > len(buf) {
		return nil, errors.New("dbus: тело выходит за границу буфера")
	}
	if m.Sig != "" && bodyLen > 0 {
		sigs, err := dbusSigSplit(m.Sig)
		if err != nil {
			return nil, err
		}
		for _, s := range sigs {
			v, err := d.decode(s)
			if err != nil {
				return nil, fmt.Errorf("dbus: тело (%s): %w", m.Sig, err)
			}
			m.Body = append(m.Body, v)
		}
	}
	return m, nil
}

// dbusErrorText вытаскивает человекочитаемое сообщение из ответа-ошибки.
func (m *dbusMessage) errorText() string {
	if len(m.Body) > 0 {
		if s, ok := m.Body[0].(string); ok {
			return m.ErrorName + ": " + s
		}
	}
	return m.ErrorName
}
