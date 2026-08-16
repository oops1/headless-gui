package datagrid

// binding_test.go — регрессия находок аудита безопасности по биндингу:
//   SEC-1  — {Binding Save} не должен выполнять команду на модели;
//   SEC-2  — TwoWay не пишет мимо типа и не зовёт не-сеттеры;
//   SEC-3  — рефлексия не роняет процесс (неэкспортированные поля, чужие ключи);
//   SEC-13 — StringFormat из разметки не раскрывает внутренности модели.

import (
	"strings"
	"testing"
)

// ─── Модели ────────────────────────────────────────────────────────────────

// sideVM — модель с методами-командами (побочный эффект) и геттерами.
type sideVM struct {
	Calls    int
	name     string
	password string
	Age      int
	Label    string
}

func (v *sideVM) Save()                 { v.Calls++ }                   // без результата
func (v *sideVM) Reset() error          { v.Calls++; return nil }       // только error
func (v *sideVM) Drop(n int) string     { v.Calls++; return "dropped" } // с аргументом
func (v *sideVM) Log(a ...string) int   { v.Calls++; return len(a) }    // variadic
func (v *sideVM) GetName() string       { return v.name }               // геттер Get+Prop
func (v *sideVM) Total() int            { return 42 }                   // геттер по имени
func (v *sideVM) Pair() (string, error) { return "ok", nil }            // геттер с error
func (v *sideVM) SetName(s string)      { v.name = s }
func (v *sideVM) SetAge(a int)          { v.Age = a }
func (v *sideVM) SetLabel(a, b string)  { v.Calls++; v.Label = a + b } // НЕ сеттер: 2 аргумента

// markerVM — модель с явным белым списком методов.
type markerVM struct{ Calls int }

func (v *markerVM) BindableMethods() []string { return []string{"Total", "Save"} }
func (v *markerVM) Total() int                { return 7 }
func (v *markerVM) GetName() string           { return "hidden" }
func (v *markerVM) Save()                     { v.Calls++ }

// nestedVM — вложенный путь до модели с геттером.
type nestedVM struct{ Inner *sideVM }

// ─── SEC-1: методы-команды через биндинг не вызываются ─────────────────────

func TestGetPropertyValue_NoSideEffectMethods(t *testing.T) {
	for _, path := range []string{"Save", "Reset", "Drop", "Log"} {
		vm := &sideVM{}
		v, ok := GetPropertyValue(vm, path)
		if ok {
			t.Errorf("{Binding %s}: резолв удался (%v), а метод-команду звать нельзя", path, v)
		}
		if vm.Calls != 0 {
			t.Errorf("{Binding %s}: метод ВЫЗВАН (Calls=%d) — побочный эффект из разметки", path, vm.Calls)
		}
	}
}

func TestGetPropertyValue_GettersStillWork(t *testing.T) {
	vm := &sideVM{name: "Аня"}
	if v, ok := GetPropertyValue(vm, "Name"); !ok || v != "Аня" {
		t.Errorf(`{Binding Name} = %v, %v; want "Аня", true (фолбэк Get+Prop)`, v, ok)
	}
	if v, ok := GetPropertyValue(vm, "Total"); !ok || v != 42 {
		t.Errorf("{Binding Total} = %v, %v; want 42, true (одноимённый геттер)", v, ok)
	}
	if v, ok := GetPropertyValue(vm, "Pair"); !ok || v != "ok" {
		t.Errorf("{Binding Pair} = %v, %v; want \"ok\", true (геттер с error)", v, ok)
	}
}

func TestGetPropertyValue_GetterOnNestedPath(t *testing.T) {
	vm := &nestedVM{Inner: &sideVM{name: "Боб"}}
	if v, ok := GetPropertyValue(vm, "Inner.Name"); !ok || v != "Боб" {
		t.Errorf(`{Binding Inner.Name} = %v, %v; want "Боб", true`, v, ok)
	}
}

func TestStrictBindingMethods(t *testing.T) {
	defer SetStrictBindingMethods(false)
	if StrictBindingMethods() {
		t.Fatal("строгий режим должен быть выключен по умолчанию")
	}
	SetStrictBindingMethods(true)
	if !StrictBindingMethods() {
		t.Fatal("SetStrictBindingMethods(true) не сработал")
	}
	vm := &sideVM{name: "Аня"}
	if v, ok := GetPropertyValue(vm, "Name"); ok {
		t.Errorf("строгий режим: {Binding Name} = %v, а без маркера методы запрещены", v)
	}
	if ok := SetPropertyValue(vm, "Name", "Боб"); ok || vm.name != "Аня" {
		t.Errorf("строгий режим: сеттер вызван (ok=%v, name=%q)", ok, vm.name)
	}
}

func TestBindableMethodsAllowlist(t *testing.T) {
	vm := &markerVM{}
	if v, ok := GetPropertyValue(vm, "Total"); !ok || v != 7 {
		t.Errorf("{Binding Total} = %v, %v; want 7, true (метод в белом списке)", v, ok)
	}
	if v, ok := GetPropertyValue(vm, "Name"); ok {
		t.Errorf("{Binding Name} = %v: метода нет в белом списке, доступ должен быть закрыт", v)
	}
	// "Save" в списке есть, но у него нет результата — форма всё равно решает.
	if v, ok := GetPropertyValue(vm, "Save"); ok || vm.Calls != 0 {
		t.Errorf("{Binding Save} = %v, ok=%v, Calls=%d: команду нельзя звать даже из белого списка", v, ok, vm.Calls)
	}
}

// ─── SEC-2: запись в модель ────────────────────────────────────────────────

func TestSetPropertyValue_Field(t *testing.T) {
	vm := &sideVM{}
	if !SetPropertyValue(vm, "Age", 33) || vm.Age != 33 {
		t.Fatalf("запись в поле Age не сработала: %d", vm.Age)
	}
}

func TestSetPropertyValue_SetterAndTypeMismatch(t *testing.T) {
	vm := &sideVM{}
	if !SetPropertyValue(vm, "Name", "Аня") || vm.name != "Аня" {
		t.Fatalf("сеттер SetName не вызван: name=%q", vm.name)
	}
	// float64 из Slider → int-аргумент: приводимо, писать можно.
	if !SetPropertyValue(vm, "Age", 7.0) || vm.Age != 7 {
		t.Fatalf("float64 → int не записан: Age=%d", vm.Age)
	}
	// Строка в числовое поле — не приводима, тихо ничего не делаем.
	if SetPropertyValue(vm, "Age", "сорок") || vm.Age != 7 {
		t.Fatalf("строка записана в int-поле: Age=%d", vm.Age)
	}
	// Число в строковое поле: reflect превратил бы 65 в "A" — запрещено.
	if SetPropertyValue(vm, "Label", 65) || vm.Label != "" {
		t.Fatalf("число записано в строковое поле как руна: Label=%q", vm.Label)
	}
}

func TestSetPropertyValue_NotASetter(t *testing.T) {
	vm := &sideVM{}
	// SetLabel(a, b string) — два аргумента, это не сеттер.
	if ok := SetPropertyValue(vm, "Label2", "x"); ok {
		t.Errorf("несуществующее свойство записано: ok=%v", ok)
	}
	if vm.Calls != 0 {
		t.Errorf("вызван метод не-сеттерной формы: Calls=%d", vm.Calls)
	}
}

// ─── SEC-3: рефлексия не паникует ──────────────────────────────────────────

func TestUnexportedFieldDoesNotPanic(t *testing.T) {
	vm := &sideVM{password: "s3cret"}
	v, ok := GetPropertyValue(vm, "password")
	if ok || v != nil {
		t.Fatalf("{Binding password} = %v, %v; неэкспортированное поле не должно читаться", v, ok)
	}
	if ok := SetPropertyValue(vm, "password", "новый"); ok || vm.password != "s3cret" {
		t.Fatalf("в неэкспортированное поле записали: ok=%v, password=%q", ok, vm.password)
	}
}

func TestMapKeyAndValueTypeMismatch(t *testing.T) {
	// Ключ не строковый — MapIndex со строкой паниковал бы.
	byInt := map[int]string{1: "один"}
	if v, ok := GetPropertyValue(byInt, "1"); ok {
		t.Errorf("map[int]string по строковому ключу вернул %v", v)
	}
	if ok := SetPropertyValue(byInt, "1", "два"); ok || byInt[1] != "один" {
		t.Errorf("запись в map[int]string по строковому ключу: ok=%v, map=%v", ok, byInt)
	}
	// Значение не того типа — SetMapIndex паниковал бы.
	byStr := map[string]int{"a": 1}
	if ok := SetPropertyValue(byStr, "a", "не число"); ok || byStr["a"] != 1 {
		t.Errorf("строка записана в map[string]int: ok=%v, map=%v", ok, byStr)
	}
	if !SetPropertyValue(byStr, "a", 2) || byStr["a"] != 2 {
		t.Errorf("корректная запись в map сломалась: %v", byStr)
	}
	// Нулевая map — SetMapIndex паникует.
	var nilMap map[string]int
	if ok := SetPropertyValue(nilMap, "a", 1); ok {
		t.Error("запись в nil-map отчиталась успехом")
	}
	// interface{}-значения — обычный путь для map[string]interface{}.
	any := map[string]interface{}{"k": 1}
	if !SetPropertyValue(any, "k", "текст") || any["k"] != "текст" {
		t.Errorf("map[string]interface{}: %v", any)
	}
}

func TestGetPropertyValue_NilAndGarbage(t *testing.T) {
	if _, ok := GetPropertyValue(nil, "A"); ok {
		t.Error("nil-объект вернул значение")
	}
	if _, ok := GetPropertyValue(&sideVM{}, ""); ok {
		t.Error("пустой путь вернул значение")
	}
	var nilPtr *sideVM
	if _, ok := GetPropertyValue(nilPtr, "Age"); ok {
		t.Error("nil-указатель вернул значение")
	}
	if _, ok := GetPropertyValue(&nestedVM{}, "Inner.Name"); ok {
		t.Error("путь через nil-поле вернул значение")
	}
}

// ─── SEC-13: StringFormat ──────────────────────────────────────────────────

type fmtModel struct {
	Name string
	Age  int
}

func TestSafeFormat(t *testing.T) {
	cases := []struct {
		format string
		value  interface{}
		want   string
	}{
		// Документированное поведение — сохраняется как было.
		{"%.2f", 3.14159, "3.14"},
		{"$%.2f", 12.5, "$12.50"},
		{"%d лет", 30, "30 лет"},
		{"Привет, %s!", "Аня", "Привет, Аня!"},
		{"", 5, "5"},
		// Лишние глаголы — вместо мусора %!d(MISSING) буквальный текст.
		{"%d %d", 5, "5 %d"},
		{"%v %v %v", 1, "1 %v %v"},
		{"%.2f из %d", 1.5, "1.50 из %d"},
		// WPF-стиль.
		{"{0}", 5, "5"},
		{"{0:F2}", 3.14159, "3.14"},
		{"Цена: {0:N2} ₽", 1234.5, "Цена: 1234.50 ₽"},
		{"{0:P1}", 0.256, "25.6%"},
		{"{0:D3}", 7, "007"},
	}
	for _, c := range cases {
		if got := SafeFormat(c.format, c.value); got != c.want {
			t.Errorf("SafeFormat(%q, %v) = %q, want %q", c.format, c.value, got, c.want)
		}
	}
}

func TestSafeFormat_NoInternalsLeak(t *testing.T) {
	m := fmtModel{Name: "Аня", Age: 33}
	for _, format := range []string{"%#v", "%+v", "%T", "Модель: %#v"} {
		got := SafeFormat(format, m)
		if strings.Contains(got, "Name:") || strings.Contains(got, "Age:") {
			t.Errorf("SafeFormat(%q) = %q: раскрыты имена полей модели", format, got)
		}
		if strings.Contains(got, "fmtModel") || strings.Contains(got, "datagrid.") {
			t.Errorf("SafeFormat(%q) = %q: раскрыт тип модели", format, got)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("SafeFormat(%q) = %q: мусор форматирования", format, got)
		}
	}
	// Значение всё равно показывается — формат обезврежен, а не выброшен.
	if got := SafeFormat("%#v", m); !strings.Contains(got, "Аня") {
		t.Errorf("SafeFormat(%q) = %q: значение потеряно", "%#v", got)
	}
	// Индексы и звёздочки тянут аргументы, которых нет.
	for _, format := range []string{"%[1]d", "%*d"} {
		if got := SafeFormat(format, 5); strings.Contains(got, "%!") {
			t.Errorf("SafeFormat(%q) = %q: мусор форматирования", format, got)
		}
	}
}

func TestResolveBinding_StringFormat(t *testing.T) {
	m := &fmtModel{Name: "Аня", Age: 33}
	b := &Binding{Path: "Age", StringFormat: "%d лет"}
	if got := ResolveBinding(b, m); got != "33 лет" {
		t.Errorf("ResolveBinding = %q, want %q", got, "33 лет")
	}
	b = &Binding{Path: "Name", StringFormat: "%#v"}
	if got := ResolveBinding(b, m); strings.Contains(got, "string") {
		t.Errorf("ResolveBinding с %%#v = %q: раскрыт тип", got)
	}
}
