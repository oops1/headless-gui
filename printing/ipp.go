// ipp.go — сборка и разбор сообщений IPP (RFC 8010/8011). Чистая логика без
// сети и без платформенных вызовов: байты на входе, байты на выходе.
//
// Зачем свой IPP. Пакет WinLine ставится без зависимостей: lp и lpr в нём нет, а
// библиотека libcups — это CGO, которого в движке нет. При этом CUPS
// принимает задания по обычному HTTP, а IPP — несложный двоичный формат, так что
// запрос собирается руками в десятки строк.
//
// Формат сообщения (RFC 8010, раздел 3.1):
//
//	версия          2 байта (major, minor)
//	операция/статус 2 байта
//	request-id      4 байта
//	группы атрибутов: тег группы (1 байт), затем атрибуты:
//	    тег значения (1), длина имени (2), имя, длина значения (2), значение
//	    (дополнительные значения того же атрибута — с пустым именем)
//	тег конца атрибутов 0x03
//	данные документа (для Print-Job)
package printing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
)

// Коды операций IPP.
const (
	ippOpPrintJob      uint16 = 0x0002
	ippOpGetPrinterAtt uint16 = 0x000B
	ippOpCUPSGetDef    uint16 = 0x4001
	ippOpCUPSGetPrint  uint16 = 0x4002
)

// Коды статуса, которые нам важны.
const (
	ippStatusOK                 uint16 = 0x0000
	ippStatusOKSubstituted      uint16 = 0x0001 // успех, часть атрибутов проигнорирована
	ippStatusOKConflicting      uint16 = 0x0002
	ippStatusClientNotFound     uint16 = 0x0406
	ippStatusClientNotAuthn     uint16 = 0x0402
	ippStatusClientNotAuthz     uint16 = 0x0403
	ippStatusErrNotAccepting    uint16 = 0x0506
	ippStatusClientBadRequest   uint16 = 0x0400
	ippStatusClientForbidden    uint16 = 0x0401
	ippStatusClientNotPossible  uint16 = 0x0404
	ippStatusClientDocFormat    uint16 = 0x040A
	ippStatusClientAttrs        uint16 = 0x040B
	ippStatusClientTooLarge     uint16 = 0x0408
	ippStatusServerInternal     uint16 = 0x0500
	ippStatusServerUnsupportedO uint16 = 0x0501
	ippStatusServerUnavailable  uint16 = 0x0502
	ippStatusServerBusy         uint16 = 0x0507
)

// Теги групп атрибутов (RFC 8010, 3.5.1).
const (
	ippTagOperation byte = 0x01
	ippTagJob       byte = 0x02
	ippTagEnd       byte = 0x03
	ippTagPrinter   byte = 0x04
)

// Теги значений (RFC 8010, 3.5.2). Всё, что ≥ 0x10, — тег значения.
const (
	ippTagInteger   byte = 0x21
	ippTagBoolean   byte = 0x22
	ippTagEnum      byte = 0x23
	ippTagText      byte = 0x41 // textWithoutLanguage
	ippTagName      byte = 0x42 // nameWithoutLanguage
	ippTagKeyword   byte = 0x44
	ippTagURI       byte = 0x45
	ippTagCharset   byte = 0x47
	ippTagLanguage  byte = 0x48
	ippTagMimeMedia byte = 0x49
)

// ippValue — одно значение атрибута: тег и сырые байты.
type ippValue struct {
	Tag  byte
	Data []byte
}

// ippAttribute — именованный атрибут с одним или несколькими значениями.
type ippAttribute struct {
	Name   string
	Values []ippValue
}

// ippGroup — группа атрибутов под одним тегом (операции, задания, принтера).
// В ответе CUPS-Get-Printers групп принтера много — по одной на принтер.
type ippGroup struct {
	Tag   byte
	Attrs []ippAttribute
}

// ippMessage — разобранное или собираемое сообщение. Code — код операции в
// запросе и код статуса в ответе: на проводе это одно и то же поле.
type ippMessage struct {
	Major, Minor byte
	Code         uint16
	RequestID    uint32
	Groups       []ippGroup
}

var ippRequestSeq atomic.Uint32

// newIPPRequest начинает запрос версии 2.0 с обязательными атрибутами операции.
//
// Порядок первых двух атрибутов — attributes-charset, затем
// attributes-natural-language — требует RFC 8011 (4.1.4): сервер вправе
// отвергнуть запрос с другим порядком. request-id не должен быть нулём (0 в
// RFC зарезервирован), поэтому счётчик стартует с единицы.
func newIPPRequest(op uint16) *ippMessage {
	id := ippRequestSeq.Add(1)
	if id == 0 { // переполнение через 2^32 запросов
		id = ippRequestSeq.Add(1)
	}
	m := &ippMessage{Major: 2, Minor: 0, Code: op, RequestID: id}
	g := m.group(ippTagOperation)
	g.add(ippTagCharset, "attributes-charset", "utf-8")
	g.add(ippTagLanguage, "attributes-natural-language", "en")
	return m
}

// group возвращает последнюю группу с таким тегом, а если её нет — добавляет.
func (m *ippMessage) group(tag byte) *ippGroup {
	for i := len(m.Groups) - 1; i >= 0; i-- {
		if m.Groups[i].Tag == tag {
			return &m.Groups[i]
		}
	}
	m.Groups = append(m.Groups, ippGroup{Tag: tag})
	return &m.Groups[len(m.Groups)-1]
}

// add добавляет строковый атрибут (текст, имя, ключевое слово, URI, кодировка …).
func (g *ippGroup) add(tag byte, name, value string) {
	g.Attrs = append(g.Attrs, ippAttribute{Name: name, Values: []ippValue{{tag, []byte(value)}}})
}

// addMulti добавляет атрибут с несколькими строковыми значениями одного типа
// (например, requested-attributes).
func (g *ippGroup) addMulti(tag byte, name string, values ...string) {
	a := ippAttribute{Name: name}
	for _, v := range values {
		a.Values = append(a.Values, ippValue{tag, []byte(v)})
	}
	g.Attrs = append(g.Attrs, a)
}

// addInt добавляет целое или перечисление (tag = ippTagInteger или ippTagEnum).
func (g *ippGroup) addInt(tag byte, name string, v int32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	g.Attrs = append(g.Attrs, ippAttribute{Name: name, Values: []ippValue{{tag, b[:]}}})
}

// addBool добавляет логический атрибут.
func (g *ippGroup) addBool(name string, v bool) {
	b := byte(0)
	if v {
		b = 1
	}
	g.Attrs = append(g.Attrs, ippAttribute{Name: name, Values: []ippValue{{ippTagBoolean, []byte{b}}}})
}

// Marshal собирает сообщение в байты (без данных документа: они дописываются
// после тега конца отдельно).
func (m *ippMessage) Marshal() ([]byte, error) {
	b := make([]byte, 0, 256)
	b = append(b, m.Major, m.Minor)
	b = binary.BigEndian.AppendUint16(b, m.Code)
	b = binary.BigEndian.AppendUint32(b, m.RequestID)
	for _, g := range m.Groups {
		if g.Tag >= 0x10 {
			return nil, fmt.Errorf("ipp: тег группы 0x%02x — это тег значения", g.Tag)
		}
		b = append(b, g.Tag)
		for _, a := range g.Attrs {
			if len(a.Values) == 0 {
				return nil, fmt.Errorf("ipp: у атрибута %q нет значений", a.Name)
			}
			if a.Name == "" || len(a.Name) > 0x7fff {
				return nil, fmt.Errorf("ipp: недопустимая длина имени атрибута (%d)", len(a.Name))
			}
			for i, v := range a.Values {
				if v.Tag < 0x10 {
					return nil, fmt.Errorf("ipp: атрибут %q: тег значения 0x%02x — это тег группы", a.Name, v.Tag)
				}
				if len(v.Data) > 0xffff {
					return nil, fmt.Errorf("ipp: атрибут %q: значение длиной %d не помещается в 16 бит", a.Name, len(v.Data))
				}
				b = append(b, v.Tag)
				// Дополнительные значения идут с пустым именем: так на проводе
				// различается «ещё одно значение» и «новый атрибут».
				if i == 0 {
					b = binary.BigEndian.AppendUint16(b, uint16(len(a.Name)))
					b = append(b, a.Name...)
				} else {
					b = binary.BigEndian.AppendUint16(b, 0)
				}
				b = binary.BigEndian.AppendUint16(b, uint16(len(v.Data)))
				b = append(b, v.Data...)
			}
		}
	}
	b = append(b, ippTagEnd)
	return b, nil
}

// Пределы разбора: ответ приходит из сети (хотя и от локального CUPS), и
// поддельный или повреждённый ответ не должен разрастись в память без меры.
const (
	ippMaxAttrs  = 20000
	ippMaxGroups = 2000
)

// parseIPP разбирает сообщение и возвращает его с остатком — байтами после тега
// конца атрибутов (данные документа в запросе Print-Job). Любое чтение за
// границей буфера — ошибка, а не паника: оборванный ответ — обычное дело.
func parseIPP(b []byte) (*ippMessage, []byte, error) {
	if len(b) < 9 {
		return nil, nil, errors.New("ipp: сообщение короче заголовка")
	}
	m := &ippMessage{
		Major:     b[0],
		Minor:     b[1],
		Code:      binary.BigEndian.Uint16(b[2:4]),
		RequestID: binary.BigEndian.Uint32(b[4:8]),
	}
	p := 8
	var (
		cur   *ippGroup
		last  *ippAttribute
		attrs int
	)
	for {
		if p >= len(b) {
			return nil, nil, errors.New("ipp: нет тега конца атрибутов (сообщение оборвано)")
		}
		tag := b[p]
		p++
		if tag < 0x10 { // разделитель: начало группы или конец
			if tag == ippTagEnd {
				return m, b[p:], nil
			}
			if len(m.Groups) >= ippMaxGroups {
				return nil, nil, errors.New("ipp: слишком много групп атрибутов")
			}
			m.Groups = append(m.Groups, ippGroup{Tag: tag})
			cur = &m.Groups[len(m.Groups)-1]
			last = nil
			continue
		}
		// Тег значения: имя и значение.
		if cur == nil {
			return nil, nil, errors.New("ipp: атрибут вне группы")
		}
		if p+2 > len(b) {
			return nil, nil, errors.New("ipp: оборвана длина имени")
		}
		nl := int(binary.BigEndian.Uint16(b[p:]))
		p += 2
		if p+nl+2 > len(b) {
			return nil, nil, errors.New("ipp: оборвано имя атрибута")
		}
		name := string(b[p : p+nl])
		p += nl
		vl := int(binary.BigEndian.Uint16(b[p:]))
		p += 2
		if p+vl > len(b) {
			return nil, nil, fmt.Errorf("ipp: оборвано значение атрибута %q", name)
		}
		val := ippValue{Tag: tag, Data: append([]byte(nil), b[p:p+vl]...)}
		p += vl
		if attrs++; attrs > ippMaxAttrs {
			return nil, nil, errors.New("ipp: слишком много атрибутов")
		}
		if nl == 0 {
			// Дополнительное значение предыдущего атрибута. Так же приходят
			// части коллекций (memberAttrName, endCollection): разбирать их
			// как структуру нам незачем, а как плоский поток они безвредны.
			if last == nil {
				return nil, nil, errors.New("ipp: дополнительное значение без атрибута")
			}
			last.Values = append(last.Values, val)
			continue
		}
		cur.Attrs = append(cur.Attrs, ippAttribute{Name: name, Values: []ippValue{val}})
		last = &cur.Attrs[len(cur.Attrs)-1]
	}
}

// find ищет первый атрибут с таким именем в первой группе с таким тегом.
func (m *ippMessage) find(groupTag byte, name string) *ippAttribute {
	for gi := range m.Groups {
		if m.Groups[gi].Tag != groupTag {
			continue
		}
		return m.Groups[gi].find(name)
	}
	return nil
}

// find ищет атрибут по имени в группе.
func (g *ippGroup) find(name string) *ippAttribute {
	for i := range g.Attrs {
		if g.Attrs[i].Name == name {
			return &g.Attrs[i]
		}
	}
	return nil
}

// groupsWithTag — все группы с данным тегом, по порядку.
func (m *ippMessage) groupsWithTag(tag byte) []*ippGroup {
	var out []*ippGroup
	for i := range m.Groups {
		if m.Groups[i].Tag == tag {
			out = append(out, &m.Groups[i])
		}
	}
	return out
}

// Str возвращает первое значение как строку; "" — если атрибута или значения
// нет. Для nil-атрибута безопасно: цепочки find(...).Str() не требуют проверок.
func (a *ippAttribute) Str() string {
	if a == nil || len(a.Values) == 0 {
		return ""
	}
	return string(a.Values[0].Data)
}

// Int возвращает первое значение как целое (integer, enum).
func (a *ippAttribute) Int() (int32, bool) {
	if a == nil || len(a.Values) == 0 || len(a.Values[0].Data) != 4 {
		return 0, false
	}
	return int32(binary.BigEndian.Uint32(a.Values[0].Data)), true
}

// Bool возвращает первое значение как логическое.
func (a *ippAttribute) Bool() (v, ok bool) {
	if a == nil || len(a.Values) == 0 || len(a.Values[0].Data) != 1 {
		return false, false
	}
	return a.Values[0].Data[0] != 0, true
}

// ippStatusText — название кода статуса для сообщения об ошибке. Голый
// «0x0406» пользователю ничего не говорит, а имя из RFC 8011 находится в
// поиске сразу.
func ippStatusText(code uint16) string {
	switch code {
	case ippStatusOK:
		return "successful-ok"
	case ippStatusOKSubstituted:
		return "successful-ok-ignored-or-substituted-attributes"
	case ippStatusOKConflicting:
		return "successful-ok-conflicting-attributes"
	case ippStatusClientBadRequest:
		return "client-error-bad-request"
	case ippStatusClientForbidden:
		return "client-error-forbidden"
	case ippStatusClientNotAuthn:
		return "client-error-not-authenticated"
	case ippStatusClientNotAuthz:
		return "client-error-not-authorized"
	case ippStatusClientNotPossible:
		return "client-error-not-possible"
	case ippStatusClientTooLarge:
		return "client-error-request-entity-too-large"
	case ippStatusClientDocFormat:
		return "client-error-document-format-not-supported"
	case ippStatusClientAttrs:
		return "client-error-attributes-or-values-not-supported"
	case ippStatusClientNotFound:
		return "client-error-not-found"
	case ippStatusServerInternal:
		return "server-error-internal-error"
	case ippStatusServerUnsupportedO:
		return "server-error-operation-not-supported"
	case ippStatusServerUnavailable:
		return "server-error-service-unavailable"
	case ippStatusErrNotAccepting:
		return "server-error-not-accepting-jobs"
	case ippStatusServerBusy:
		return "server-error-busy"
	}
	return fmt.Sprintf("0x%04x", code)
}

// ippError — ошибка, пришедшая в статусе ответа.
type ippError struct {
	Code    uint16
	Message string // status-message сервера, если был
}

func (e *ippError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("ipp: %s: %s", ippStatusText(e.Code), e.Message)
	}
	return "ipp: " + ippStatusText(e.Code)
}

// check превращает статус ответа в ошибку: коды до 0x00FF — успех (в том числе
// «успех с заменой атрибутов»), от 0x0400 — отказ.
func (m *ippMessage) check() error {
	if m.Code < 0x0100 {
		return nil
	}
	return &ippError{Code: m.Code, Message: m.find(ippTagOperation, "status-message").Str()}
}
