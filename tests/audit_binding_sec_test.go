package tests

// audit_binding_sec_test.go — сквозная регрессия находок аудита по биндингу,
// через реальную загрузку XAML:
//   SEC-1  — {Binding Save} не выполняет команду на модели;
//   SEC-2  — TwoWay не пишет в неэкспортированное/несовместимое поле;
//   SEC-3  — рефлексия на пользовательской модели не роняет процесс;
//   SEC-11 — Dispose снимает подписки на коллекцию и на legacy-нотификатор;
//   SEC-13 — StringFormat из разметки не раскрывает внутренности модели.

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── Модели ────────────────────────────────────────────────────────────────

// secretVM — модель с командой, приватным полем и вложенной структурой.
type secretVM struct {
	dg.PropertyNotifier
	SaveCalls int
	password  string
	Age       int
	Inner     innerData
}

type innerData struct {
	Name  string
	Token string
}

// Save — команда с побочным эффектом. Раньше подходила биндингу по старому
// критерию «без аргументов и хотя бы один результат» и реально выполнялась.
func (v *secretVM) Save() error { v.SaveCalls++; return nil }

// Reload — команда, неотличимая от геттера по сигнатуре: её отсекает только
// строгий режим (SetStrictBindingMethods) или маркер BindableMethods.
func (v *secretVM) Reload() bool { v.SaveCalls++; return true }

// legacyNotifier — INotifyPropertyChanged старого образца: подписка без
// дескриптора (только AddPropertyChanged/RemovePropertyChanged).
type legacyNotifier struct {
	mu       sync.Mutex
	handlers []dg.PropertyChangedHandler
}

func (l *legacyNotifier) AddPropertyChanged(h dg.PropertyChangedHandler) {
	l.mu.Lock()
	l.handlers = append(l.handlers, h)
	l.mu.Unlock()
}

func (l *legacyNotifier) RemovePropertyChanged(h dg.PropertyChangedHandler) {
	if h == nil {
		return
	}
	want := reflect.ValueOf(h).Pointer()
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.handlers) - 1; i >= 0; i-- {
		if reflect.ValueOf(l.handlers[i]).Pointer() == want {
			l.handlers = append(l.handlers[:i], l.handlers[i+1:]...)
			return
		}
	}
}

func (l *legacyNotifier) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.handlers)
}

type legacyVM struct {
	legacyNotifier
	Value float64
}

// itemsVM — модель с ObservableCollection в качестве источника ItemsControl.
type itemsVM struct {
	dg.PropertyNotifier
	Items *dg.ObservableCollection
}

type itemRow struct{ Name string }

// ─── SEC-1 ─────────────────────────────────────────────────────────────────

func TestXAMLBinding_DoesNotInvokeCommand(t *testing.T) {
	vm := &secretVM{}
	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="lbl" Text="{Binding Save}"/>
	</Canvas>`

	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.Refresh()

	if vm.SaveCalls != 0 {
		t.Fatalf("{Binding Save} ВЫПОЛНИЛ команду на модели: SaveCalls=%d", vm.SaveCalls)
	}
	lbl, ok := reg["lbl"].(*widget.Label)
	if !ok {
		t.Fatal("lbl не Label")
	}
	if got := lbl.Text(); got != "" {
		t.Errorf("текст = %q, ожидалась пустая строка (метод недоступен биндингу)", got)
	}
}

// Строгий режим отсекает и те команды, что по сигнатуре выглядят геттерами.
func TestXAMLBinding_StrictModeBlocksGetterShapedCommand(t *testing.T) {
	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="lbl" Text="{Binding Reload}"/>
	</Canvas>`

	vm := &secretVM{}
	if _, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm); err != nil {
		t.Fatalf("load: %v", err)
	} else {
		scope.Refresh()
	}
	if vm.SaveCalls == 0 {
		t.Fatal("по умолчанию геттероподобный метод должен вызываться — тест бессмыслен")
	}

	dg.SetStrictBindingMethods(true)
	defer dg.SetStrictBindingMethods(false)

	strictVM := &secretVM{}
	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), strictVM)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.Refresh()
	if strictVM.SaveCalls != 0 {
		t.Errorf("строгий режим: метод вызван %d раз(а)", strictVM.SaveCalls)
	}
}

// showcaseVMShape повторяет форму модели витрины (cmd/showcase, cmd/webshowcase):
// приватное поле + GetUsername/SetUsername. Именно на этот фолбэк опирается
// assets/ui/showcase.xaml, поэтому ужесточение SEC-1 не должно его убить.
type showcaseVMShape struct {
	dg.PropertyNotifier
	username string
}

func (v *showcaseVMShape) GetUsername() string  { return v.username }
func (v *showcaseVMShape) SetUsername(s string) { v.username = s }

func TestXAMLBinding_ShowcaseGetterPatternStillWorks(t *testing.T) {
	vm := &showcaseVMShape{username: "Аня"}
	const xaml = `<Canvas xmlns="clr">
		<TextBox Name="tb" Text="{Binding Username, Mode=TwoWay}"/>
		<TextBlock Name="hi" Text="{Binding Username, StringFormat=Привет, %s! (живой OneWay)}"/>
	</Canvas>`

	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.Refresh()

	if got := reg["hi"].(*widget.Label).Text(); got != "Привет, Аня! (живой OneWay)" {
		t.Errorf("OneWay через GetUsername = %q", got)
	}
	// TwoWay пишет через SetUsername.
	reg["tb"].(*widget.TextInput).OnChange("Боб")
	if vm.username != "Боб" {
		t.Errorf("TwoWay через SetUsername не сработал: %q", vm.username)
	}
}

// ─── SEC-2 / SEC-3 ─────────────────────────────────────────────────────────

func TestXAMLBinding_TwoWayUnexportedFieldIsSafe(t *testing.T) {
	vm := &secretVM{password: "s3cret"}
	const xaml = `<Canvas xmlns="clr">
		<TextBox Name="tb" Text="{Binding password, Mode=TwoWay}"/>
	</Canvas>`

	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tb, ok := reg["tb"].(*widget.TextInput)
	if !ok {
		t.Fatal("tb не TextInput")
	}
	if tb.OnChange == nil {
		t.Fatal("TwoWay не подключил OnChange")
	}
	// Раньше здесь была паника рефлексии (Interface()/Set на приватном поле).
	tb.OnChange("подмена")

	if vm.password != "s3cret" {
		t.Errorf("приватное поле перезаписано через биндинг: %q", vm.password)
	}
}

func TestXAMLBinding_TwoWayTypeMismatchIsSafe(t *testing.T) {
	vm := &secretVM{Age: 33}
	const xaml = `<Canvas xmlns="clr">
		<TextBox Name="tb" Text="{Binding Age, Mode=TwoWay}"/>
	</Canvas>`

	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tb := reg["tb"].(*widget.TextInput)
	tb.OnChange("не число") // строка в int-поле: ни паники, ни записи

	if vm.Age != 33 {
		t.Errorf("int-поле перезаписано строкой: Age=%d", vm.Age)
	}
}

func TestXAMLBinding_BadPathsDoNotPanic(t *testing.T) {
	vm := &secretVM{}
	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="a" Text="{Binding Inner.Missing.Deep}"/>
		<TextBlock Name="b" Text="{Binding Inner.token}"/>
		<TextBlock Name="c" Text="{Binding .}"/>
	</Canvas>`

	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.Refresh()
	scope.SetDataContext(vm)
	scope.SetDataContext(nil)
}

// ─── SEC-13 ────────────────────────────────────────────────────────────────

func TestXAMLBinding_StringFormatDoesNotLeakInternals(t *testing.T) {
	vm := &secretVM{Age: 33, Inner: innerData{Name: "Аня", Token: "t0ken"}}
	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="dump" Text="{Binding Inner, StringFormat=%#v}"/>
		<TextBlock Name="plus" Text="{Binding Inner, StringFormat=%+v}"/>
		<TextBlock Name="multi" Text="{Binding Age, StringFormat=%d %d}"/>
		<TextBlock Name="good" Text="{Binding Age, StringFormat=%d лет}"/>
	</Canvas>`

	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.Refresh()

	text := func(name string) string { return reg[name].(*widget.Label).Text() }

	for _, name := range []string{"dump", "plus"} {
		got := text(name)
		if strings.Contains(got, "Name:") || strings.Contains(got, "Token:") {
			t.Errorf("%s = %q: раскрыты имена полей модели", name, got)
		}
		if strings.Contains(got, "innerData") || strings.Contains(got, "tests.") {
			t.Errorf("%s = %q: раскрыт тип модели", name, got)
		}
	}
	if got := text("multi"); strings.Contains(got, "%!") {
		t.Errorf("multi = %q: мусор форматирования (%%!d(MISSING))", got)
	} else if got != "33 %d" {
		t.Errorf("multi = %q, want %q", got, "33 %d")
	}
	if got := text("good"); got != "33 лет" {
		t.Errorf("good = %q, want %q (рабочий формат сломан)", got, "33 лет")
	}
}

// ─── SEC-11 ────────────────────────────────────────────────────────────────

func TestBindingScope_DisposeUnsubscribesCollection(t *testing.T) {
	oc := dg.NewObservableCollectionFrom([]interface{}{
		&itemRow{Name: "Аня"}, &itemRow{Name: "Боб"},
	})
	vm := &itemsVM{Items: oc}
	const xaml = `<Canvas xmlns="clr">
		<ItemsControl Name="lst" ItemsSource="{Binding Items}">
			<ItemsControl.ItemTemplate>
				<DataTemplate>
					<TextBlock Text="{Binding Name}"/>
				</DataTemplate>
			</ItemsControl.ItemTemplate>
		</ItemsControl>
	</Canvas>`

	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := oc.HandlerCount(); got == 0 {
		t.Fatal("ItemsControl не подписался на коллекцию — тест бессмыслен")
	}
	if got := vm.HandlerCount(); got != 1 {
		t.Fatalf("подписок на модель %d, want 1", got)
	}

	scope.Dispose()

	if got := oc.HandlerCount(); got != 0 {
		t.Errorf("после Dispose подписок на коллекцию %d, want 0 (утечка дерева виджетов)", got)
	}
	if got := vm.HandlerCount(); got != 0 {
		t.Errorf("после Dispose подписок на модель %d, want 0", got)
	}
	scope.Dispose() // повторный вызов безопасен
}

func TestBindingScope_DisposeUnsubscribesLegacyNotifier(t *testing.T) {
	vm := &legacyVM{Value: 42}
	const xaml = `<Canvas xmlns="clr">
		<Slider Name="s" Value="{Binding Value}" Minimum="0" Maximum="100"/>
	</Canvas>`

	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := vm.count(); got != 1 {
		t.Fatalf("подписок на legacy-модель %d, want 1", got)
	}

	scope.Dispose()

	if got := vm.count(); got != 0 {
		t.Errorf("после Dispose подписок на legacy-модель %d, want 0 (SEC-11: newHandle=-1)", got)
	}
}

func TestBindingScope_SetDataContextUnsubscribesLegacy(t *testing.T) {
	a := &legacyVM{Value: 1}
	b := &legacyVM{Value: 2}
	const xaml = `<Canvas xmlns="clr">
		<Slider Name="s" Value="{Binding Value}" Minimum="0" Maximum="100"/>
	</Canvas>`

	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), a)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	scope.SetDataContext(b)

	if got := a.count(); got != 0 {
		t.Errorf("после смены контекста подписок на старую модель %d, want 0", got)
	}
	if got := b.count(); got != 1 {
		t.Errorf("подписок на новую модель %d, want 1", got)
	}
}
