// Package datagrid — DataGrid и система Data Binding, совместимая с WPF.
//
// Binding связывает свойство UI-элемента с полем модели данных.
// Поддерживает:
//   - Path: путь к свойству через точку ("User.Name")
//   - Mode: OneWay, TwoWay, OneTime
//   - Converter: IValueConverter для преобразования значений
//   - StringFormat: форматирование строкового представления
package datagrid

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

// ─── Binding Mode ──────────────────────────────────────────────────────────

// BindingMode определяет направление привязки данных.
type BindingMode int

const (
	// OneWay — модель → UI (по умолчанию).
	OneWay BindingMode = iota
	// TwoWay — модель ↔ UI.
	TwoWay
	// OneTime — однократное чтение при привязке.
	OneTime
)

// ─── IValueConverter ───────────────────────────────────────────────────────

// IValueConverter преобразует значение при биндинге (WPF IValueConverter).
type IValueConverter interface {
	// Convert преобразует значение модели для отображения в UI.
	Convert(value interface{}) interface{}
	// ConvertBack преобразует значение UI обратно для записи в модель.
	ConvertBack(value interface{}) interface{}
}

// ─── Binding ───────────────────────────────────────────────────────────────

// Binding описывает привязку свойства UI к свойству модели.
type Binding struct {
	// Path — путь к свойству через точку, например "User.Name".
	Path string
	// Mode — направление привязки (OneWay, TwoWay, OneTime).
	Mode BindingMode
	// Converter — опциональный конвертер значений.
	Converter IValueConverter
	// StringFormat — формат строки, например "%.2f" или WPF-стиль "{0:F2}".
	// Применяется через SafeFormat: формат считается ДАННЫМИ разметки, а не
	// доверенным форматом из кода.
	StringFormat string
}

// ─── Политика вызова методов из биндинга (аудит SEC-1) ─────────────────────

// BindableMethods — необязательный маркер модели, явно перечисляющей имена
// методов, которые разрешено вызывать из биндинга.
//
// Если DataContext (или объект на текущем сегменте пути) реализует этот
// интерфейс, работает ТОЛЬКО белый список: любой метод вне списка недоступен
// биндингу, независимо от режима строгости. В списке можно указать как имя
// самого метода ("GetUsername", "SetUsername"), так и имя свойства из
// разметки ("Username") — тогда разрешены обе формы.
//
// Пример:
//
//	func (v *VM) BindableMethods() []string { return []string{"Username", "Total"} }
type BindableMethods interface {
	BindableMethods() []string
}

// strictBindingMethods — глобальный переключатель строгого режима.
// По умолчанию false, см. SetStrictBindingMethods.
var strictBindingMethods atomic.Bool

// SetStrictBindingMethods включает строгий режим вызова методов из биндинга.
//
// Выключен (по умолчанию): у типов БЕЗ маркера BindableMethods разрешён вызов
// методов «геттерной» формы (см. isGetterShape) — это поведение, на котором
// держатся существующие модели с приватными полями и методами Get<Prop>().
//
// Включён: методы вызываются ТОЛЬКО у типов с маркером BindableMethods и
// только из его списка. Рекомендуется приложениям, где разметка (XAML) может
// прийти из недоверенного источника: там `{Binding Save}` — это возможность
// выполнить чужой код, а не опечатка.
//
// В ЛЮБОМ режиме запрещены методы «побочной» формы: с аргументами, variadic,
// без результата и возвращающие только error. Именно они и есть команды
// (Save/Delete/Reset), ради которых и заведено ограничение.
func SetStrictBindingMethods(strict bool) { strictBindingMethods.Store(strict) }

// StrictBindingMethods сообщает текущий режим (см. SetStrictBindingMethods).
func StrictBindingMethods() bool { return strictBindingMethods.Load() }

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// isGetterShape — сигнатура «геттера»: без аргументов, не variadic, ровно один
// полезный результат (допускается второй результат типа error).
//
// Метод без результатов или с единственным результатом error значения не даёт
// и биндингу бесполезен — зато почти наверняка что-то делает (Save() error).
func isGetterShape(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Func || t.IsVariadic() || t.NumIn() != 0 {
		return false
	}
	switch t.NumOut() {
	case 1:
		return !t.Out(0).Implements(errorType)
	case 2:
		return !t.Out(0).Implements(errorType) && t.Out(1).Implements(errorType)
	}
	return false
}

// isSetterShape — сигнатура «сеттера»: ровно один аргумент, не variadic,
// без результата или с единственным результатом error.
func isSetterShape(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Func || t.IsVariadic() || t.NumIn() != 1 {
		return false
	}
	switch t.NumOut() {
	case 0:
		return true
	case 1:
		return t.Out(0).Implements(errorType)
	}
	return false
}

// bindableList достаёт белый список методов с первого приёмника, который его
// объявил. Приёмников два (до и после разыменования), потому что маркер может
// быть объявлен на указателе, а сам метод — на значении: иначе список можно
// было бы обойти, подобрав форму приёмника.
func bindableList(recvs ...reflect.Value) ([]string, bool) {
	for _, v := range recvs {
		if v.IsValid() && v.CanInterface() {
			if bm, ok := v.Interface().(BindableMethods); ok {
				return bm.BindableMethods(), true
			}
		}
	}
	return nil, false
}

// methodPermitted — политика доступа (без учёта сигнатуры): белый список
// BindableMethods, иначе глобальный режим строгости.
func methodPermitted(allow []string, hasList bool, name, part string) bool {
	if hasList {
		for _, a := range allow {
			if a == name || a == part {
				return true
			}
		}
		return false // маркер объявлен — работает только белый список
	}
	return !strictBindingMethods.Load()
}

// bindableGetter ищет метод-геттер для сегмента пути part: сначала одноимённый
// (part), затем Get+part. Приёмники пробуются оба — owner (значение ДО
// разыменования, видит методы с указательным приёмником) и recv.
func bindableGetter(owner, recv reflect.Value, part string) (reflect.Value, bool) {
	if part == "" {
		return reflect.Value{}, false
	}
	allow, hasList := bindableList(owner, recv)
	for _, r := range [...]reflect.Value{owner, recv} {
		if !r.IsValid() {
			continue
		}
		for _, name := range [...]string{part, "Get" + part} {
			m := r.MethodByName(name)
			// CanInterface==false — значение получено через неэкспортированное
			// поле; вызов такого метода паникует (аудит SEC-3).
			if !m.IsValid() || !m.CanInterface() {
				continue
			}
			if isGetterShape(m.Type()) && methodPermitted(allow, hasList, name, part) {
				return m, true
			}
		}
	}
	return reflect.Value{}, false
}

// bindableSetter ищет метод Set<part> сеттерной формы и готовит аргумент.
// Возвращает (метод, аргумент, ok) — вызывать можно только при ok.
func bindableSetter(owner, recv reflect.Value, part string, value interface{}) (reflect.Value, reflect.Value, bool) {
	none := reflect.Value{}
	if part == "" {
		return none, none, false
	}
	name := "Set" + part
	allow, hasList := bindableList(owner, recv)
	if !methodPermitted(allow, hasList, name, part) {
		return none, none, false
	}
	for _, r := range [...]reflect.Value{owner, recv} {
		if !r.IsValid() {
			continue
		}
		m := r.MethodByName(name)
		if !m.IsValid() || !m.CanInterface() || !isSetterShape(m.Type()) {
			continue
		}
		arg, ok := bindingConvert(reflect.ValueOf(value), m.Type().In(0))
		if !ok {
			continue // тип не подходит — вызов бы паниковал (SEC-2/3)
		}
		return m, arg, true
	}
	return none, none, false
}

// ─── PropertyAccessor ──────────────────────────────────────────────────────

// GetPropertyValue получает значение свойства по пути через точку.
// Поддерживает:
//   - Поля структуры: "Name", "Address.City" (только экспортированные)
//   - Указатели: автоматически разыменовывает *T
//   - Map: "key.subkey" (только если ключ приводим из строки)
//   - PropertyGetter интерфейс: вызывает GetProperty(name)
//   - Методы-геттеры: "Total" → Total() или GetTotal(), см. политику ниже
//
// Безопасность (аудит SEC-1/SEC-3): раньше при отсутствии поля вызывался ЛЮБОЙ
// экспортированный метод без аргументов — `{Binding Save}` в разметке выполнял
// Save() на модели. Теперь вызываются только методы «геттерной» формы
// (isGetterShape), а модель может сузить список маркером BindableMethods или
// приложение — глобально через SetStrictBindingMethods(true).
//
// Паники рефлексии (Interface() на неэкспортированном поле, MapIndex с чужим
// типом ключа, вызов метода с несовпадающими типами) не выходят наружу:
// функция перехватывает их, пишет в log и возвращает (nil, false).
func GetPropertyValue(obj interface{}, path string) (value interface{}, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("datagrid: биндинг %q прерван паникой рефлексии: %v", path, r)
			value, ok = nil, false
		}
	}()
	return getPropertyValue(obj, path)
}

func getPropertyValue(obj interface{}, path string) (interface{}, bool) {
	if obj == nil || path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := reflect.ValueOf(obj)

	for _, part := range parts {
		// owner — значение ДО разыменования: на нём ищутся методы, в том числе
		// с указательным приёмником (у разыменованной структуры их не видно).
		owner := current

		// Разыменовываем указатели
		for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
			if current.IsNil() {
				return nil, false
			}
			current = current.Elem()
		}

		switch current.Kind() {
		case reflect.Struct:
			field := current.FieldByName(part)
			// CanInterface==false — поле неэкспортированное: читать его через
			// Interface() нельзя, это паника (аудит SEC-3). Считаем «не найдено».
			if field.IsValid() && field.CanInterface() {
				current = field
				continue
			}
			m, found := bindableGetter(owner, current, part)
			if !found {
				return nil, false
			}
			result := m.Call(nil)
			if len(result) == 2 && !result[1].IsZero() {
				return nil, false // геттер вернул ошибку
			}
			current = result[0]

		case reflect.Map:
			val, found := mapLookup(current, part)
			if !found {
				return nil, false
			}
			current = val

		default:
			// Проверим интерфейс PropertyGetter
			if !current.CanInterface() {
				return nil, false
			}
			pg, isGetter := current.Interface().(PropertyGetter)
			if !isGetter {
				return nil, false
			}
			v, found := pg.GetProperty(part)
			if !found {
				return nil, false
			}
			current = reflect.ValueOf(v)
		}
	}

	if !current.IsValid() || !current.CanInterface() {
		return nil, false
	}
	return current.Interface(), true
}

// SetPropertyValue устанавливает значение свойства по пути через точку.
// Поддерживает структуры (экспортированные поля и методы Set<Name>) и map.
//
// Безопасность (аудит SEC-2/SEC-3): значение записывается только после
// проверки присваиваемости/приводимости (bindingConvert), метод Set<Name>
// вызывается только сеттерной формы (isSetterShape) и только с подходящим
// типом аргумента. Паники рефлексии перехватываются, пишутся в log,
// возвращается false.
func SetPropertyValue(obj interface{}, path string, value interface{}) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("datagrid: запись биндинга %q прервана паникой рефлексии: %v", path, r)
			ok = false
		}
	}()
	return setPropertyValue(obj, path, value)
}

func setPropertyValue(obj interface{}, path string, value interface{}) bool {
	if obj == nil || path == "" {
		return false
	}

	parts := strings.Split(path, ".")
	current := reflect.ValueOf(obj)

	// Проходим до предпоследнего элемента пути
	for i := 0; i < len(parts)-1; i++ {
		for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
			if current.IsNil() {
				return false
			}
			current = current.Elem()
		}

		switch current.Kind() {
		case reflect.Struct:
			field := current.FieldByName(parts[i])
			if !field.IsValid() || !field.CanInterface() {
				return false
			}
			current = field
		case reflect.Map:
			val, found := mapLookup(current, parts[i])
			if !found {
				return false
			}
			current = val
		default:
			return false
		}
	}

	// Устанавливаем значение последнего поля
	lastPart := parts[len(parts)-1]
	owner := current

	for current.Kind() == reflect.Ptr || current.Kind() == reflect.Interface {
		if current.IsNil() {
			return false
		}
		current = current.Elem()
	}

	switch current.Kind() {
	case reflect.Struct:
		field := current.FieldByName(lastPart)
		if field.IsValid() && field.CanSet() {
			val, okConv := bindingConvert(reflect.ValueOf(value), field.Type())
			if !okConv {
				return false
			}
			field.Set(val)
			return true
		}
		// Поля нет либо оно неэкспортированное/неадресуемое — пробуем сеттер.
		if m, arg, found := bindableSetter(owner, current, lastPart, value); found {
			m.Call([]reflect.Value{arg})
			return true
		}
		return false

	case reflect.Map:
		return mapAssign(current, lastPart, value)
	}

	return false
}

// ─── Безопасная рефлексия ──────────────────────────────────────────────────

// mapLookup достаёт элемент map по строковому сегменту пути. MapIndex с
// ключом чужого типа паникует, поэтому тип ключа проверяется заранее (SEC-3).
func mapLookup(m reflect.Value, part string) (reflect.Value, bool) {
	if m.Kind() != reflect.Map || m.IsNil() {
		return reflect.Value{}, false
	}
	key, ok := mapKey(m.Type().Key(), part)
	if !ok {
		return reflect.Value{}, false
	}
	val := m.MapIndex(key)
	if !val.IsValid() {
		return reflect.Value{}, false
	}
	return val, true
}

// mapAssign кладёт значение в map по строковому сегменту пути, проверив и ключ,
// и тип элемента (SetMapIndex с чужим типом значения паникует — SEC-3).
func mapAssign(m reflect.Value, part string, value interface{}) bool {
	if m.Kind() != reflect.Map || m.IsNil() || !m.CanInterface() {
		return false
	}
	key, ok := mapKey(m.Type().Key(), part)
	if !ok {
		return false
	}
	val, ok := bindingConvert(reflect.ValueOf(value), m.Type().Elem())
	if !ok {
		return false
	}
	m.SetMapIndex(key, val)
	return true
}

// mapKey приводит сегмент пути к типу ключа map. Разрешены только строковые
// (в т.ч. именованные) ключи и interface{}: превращать "3" в int биндинг
// не обязан, а молча промахиваться мимо ключа — хуже, чем честный false.
func mapKey(kt reflect.Type, part string) (reflect.Value, bool) {
	kv := reflect.ValueOf(part)
	switch {
	case kv.Type().AssignableTo(kt):
		return kv, true
	case kt.Kind() == reflect.String && kv.Type().ConvertibleTo(kt):
		return kv.Convert(kt), true
	}
	return reflect.Value{}, false
}

// bindingConvert приводит значение из UI к типу приёмника в модели.
//
// Присваиваемость — всегда. Преобразование — только осмысленное: reflect
// охотно превращает число в строку (int 65 → "A"), и без этого запрета
// TwoWay-привязка Slider → string-поля молча записывала бы случайную руну
// вместо текста (аудит SEC-2). Числовые преобразования (float64 → int для
// Slider/NumericUpDown) сохранены — на них держатся существующие привязки.
func bindingConvert(val reflect.Value, dst reflect.Type) (reflect.Value, bool) {
	if dst == nil {
		return reflect.Value{}, false
	}
	if !val.IsValid() {
		// nil из UI: осмыслен только для типов, у которых есть nil.
		switch dst.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice,
			reflect.Func, reflect.Chan, reflect.UnsafePointer:
			return reflect.Zero(dst), true
		}
		return reflect.Value{}, false
	}
	if val.Type().AssignableTo(dst) {
		return val, true
	}
	if !val.Type().ConvertibleTo(dst) {
		return reflect.Value{}, false
	}
	srcStr := val.Kind() == reflect.String
	dstStr := dst.Kind() == reflect.String
	if srcStr != dstStr {
		return reflect.Value{}, false // строка ↔ не-строка: не текст, а мусор
	}
	return val.Convert(dst), true
}

// PropertyGetter — интерфейс для объектов, поддерживающих динамический доступ к свойствам.
type PropertyGetter interface {
	GetProperty(name string) (interface{}, bool)
}

// ─── StringFormat ──────────────────────────────────────────────────────────

// verbSpec — один спецификатор формата, найденный в строке StringFormat.
type verbSpec struct {
	start, end int    // границы спецификатора в строке формата
	flags      string // флаги (+ - # 0 пробел)
	verb       byte   // буква глагола (0 — висячий '%' в конце)
	indexed    bool   // явный индекс аргумента %[1]d
	star       bool   // ширина/точность из аргумента (%*d)
}

// SafeFormat применяет StringFormat как ДАННЫЕ разметки, а не как доверенный
// формат из кода (аудит SEC-13).
//
// Поддерживаются два стиля:
//   - WPF: "{0}", "{0:F2}", "Цена: {0:N2} ₽" (F/N — дробное, P — проценты,
//     D — целое с ведущими нулями, X — шестнадцатеричное);
//   - Go: ровно ОДИН глагол, например "%.2f", "$%.2f", "%d лет".
//
// Что отсекается и почему:
//   - "%#v"/"%+v" печатают имена полей и тип модели — утечка внутренностей;
//   - "%T"/"%p" печатают тип и адрес — то же самое;
//   - "%*d"/"%[1]d" тянут дополнительные аргументы, которых нет;
//   - несколько глаголов дают мусор вида "%!d(MISSING)".
//
// В этих случаях формат применяется безопасно: первый глагол заменяется на
// "%v", остальные печатаются буквальным текстом. Значение показывается всегда,
// разметка не может ни уронить форматирование, ни вытянуть лишнее.
func SafeFormat(format string, v interface{}) string {
	if format == "" {
		return fmt.Sprintf("%v", v)
	}
	if s, ok := formatWPF(format, v); ok {
		return s
	}
	verbs := scanVerbs(format)
	if len(verbs) == 0 {
		// Формат без глаголов (в т.ч. чужой синтаксис) — показываем значение.
		return fmt.Sprintf("%v", v)
	}
	if len(verbs) == 1 && verbAllowed(verbs[0]) {
		return fmt.Sprintf(format, v)
	}
	return fmt.Sprintf(sanitizeFormat(format, verbs), v)
}

// scanVerbs находит все спецификаторы формата, пропуская экранированные "%%".
func scanVerbs(format string) []verbSpec {
	var out []verbSpec
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++ // экранированный процент — не глагол
			continue
		}
		vs := verbSpec{start: i}
		j := i + 1
		for j < len(format) && strings.IndexByte("+-# 0", format[j]) >= 0 {
			vs.flags += string(format[j])
			j++
		}
		if j < len(format) && format[j] == '[' { // %[1]d — явный индекс
			vs.indexed = true
			if k := strings.IndexByte(format[j:], ']'); k >= 0 {
				j += k + 1
			}
		}
		for j < len(format) && (format[j] >= '0' && format[j] <= '9' || format[j] == '.' || format[j] == '*') {
			if format[j] == '*' {
				vs.star = true
			}
			j++
		}
		if j >= len(format) {
			vs.end = len(format) // висячий '%' — глагола нет
			out = append(out, vs)
			break
		}
		vs.verb = format[j]
		vs.end = j + 1
		out = append(out, vs)
		i = j
	}
	return out
}

// verbAllowed — можно ли отдать этот спецификатор в fmt.Sprintf как есть.
func verbAllowed(vs verbSpec) bool {
	if vs.verb == 0 || vs.indexed || vs.star {
		return false
	}
	if strings.ContainsAny(vs.flags, "#+") {
		return false // %#v / %+v раскрывают тип и имена полей модели
	}
	switch vs.verb {
	case 'T', 'p':
		return false // тип и адрес в памяти — не данные для UI
	}
	return vs.verb >= 'a' && vs.verb <= 'z' || vs.verb >= 'A' && vs.verb <= 'Z'
}

// sanitizeFormat перестраивает формат так, чтобы у него был ровно один
// безопасный глагол: первый (или "%v", если он запрещён), остальные —
// экранируются и печатаются буквально.
func sanitizeFormat(format string, verbs []verbSpec) string {
	var b strings.Builder
	prev := 0
	for i, vs := range verbs {
		b.WriteString(format[prev:vs.start])
		switch {
		case i > 0:
			b.WriteString("%%") // литеральный '%'
			b.WriteString(format[vs.start+1 : vs.end])
		case verbAllowed(vs):
			b.WriteString(format[vs.start:vs.end])
		default:
			b.WriteString("%v")
		}
		prev = vs.end
	}
	b.WriteString(format[prev:])
	return b.String()
}

// formatWPF применяет WPF-стиль StringFormat: "{0}" / "{0:F2}".
// Возвращает ok=false, если плейсхолдера нет — тогда формат разбирается как Go.
func formatWPF(format string, v interface{}) (string, bool) {
	i := strings.Index(format, "{0")
	if i < 0 {
		return "", false
	}
	j := strings.IndexByte(format[i:], '}')
	if j < 0 {
		return "", false
	}
	j += i
	spec := format[i+2 : j]
	if spec != "" && !strings.HasPrefix(spec, ":") {
		return "", false // "{0abc}" — не наш синтаксис
	}
	return format[:i] + wpfValue(strings.TrimPrefix(spec, ":"), v) + format[j+1:], true
}

// wpfValue форматирует значение по WPF-спецификатору (F2, N0, P1, D3, X).
// Незнакомый спецификатор или нечисловое значение → "%v".
func wpfValue(spec string, v interface{}) string {
	if spec == "" {
		return fmt.Sprintf("%v", v)
	}
	f, isNum := toFloat(v)
	digits := -1
	if n, err := strconv.Atoi(spec[1:]); err == nil && n >= 0 {
		digits = n
	}
	if isNum {
		switch spec[0] {
		case 'F', 'f', 'N', 'n':
			if digits < 0 {
				digits = 2
			}
			return strconv.FormatFloat(f, 'f', digits, 64)
		case 'P', 'p':
			if digits < 0 {
				digits = 2
			}
			return strconv.FormatFloat(f*100, 'f', digits, 64) + "%"
		case 'D', 'd':
			s := strconv.FormatInt(int64(f), 10)
			neg := strings.HasPrefix(s, "-")
			s = strings.TrimPrefix(s, "-")
			for len(s) < digits {
				s = "0" + s
			}
			if neg {
				s = "-" + s
			}
			return s
		case 'X':
			return fmt.Sprintf("%X", int64(f))
		case 'x':
			return fmt.Sprintf("%x", int64(f))
		}
	}
	return fmt.Sprintf("%v", v)
}

// toFloat приводит числовое значение модели к float64.
func toFloat(v interface{}) (float64, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return 0, false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	}
	return 0, false
}

// ─── Resolve Binding ───────────────────────────────────────────────────────

// ResolveBinding получает значение из объекта по binding-пути.
// Применяет Converter и StringFormat (через SafeFormat).
func ResolveBinding(b *Binding, item interface{}) string {
	if b == nil || item == nil {
		return ""
	}

	val, ok := GetPropertyValue(item, b.Path)
	if !ok {
		return ""
	}

	// Применяем конвертер
	if b.Converter != nil {
		val = b.Converter.Convert(val)
	}

	// Применяем StringFormat
	if b.StringFormat != "" {
		return SafeFormat(b.StringFormat, val)
	}

	return fmt.Sprintf("%v", val)
}
