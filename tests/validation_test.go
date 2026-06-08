package tests

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// waitFor опрашивает cond до 1 с (OnChange у TextInput асинхронный — go onCh).
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// typeKeys печатает строку посимвольно с паузой — TextInput.OnChange асинхронный
// (go onCh), поэтому моментальный ввод в цикле может дать гонку порядка записи;
// человек печатает с интервалами, что воспроизводим здесь.
func typeKeys(ti *widget.TextInput, s string) {
	for _, r := range s {
		ti.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
		time.Sleep(8 * time.Millisecond)
	}
}

// vForm — модель формы с проверкой по IDataErrorInfo.
type vForm struct {
	datagrid.PropertyNotifier
	Name string
}

func (f *vForm) DataError(prop string) string {
	if prop == "Name" && len([]rune(f.Name)) < 3 {
		return "Имя должно быть не короче 3 символов"
	}
	return ""
}

const valXAML = `<Canvas xmlns="clr">
	<TextBox Name="nameBox" Text="{Binding Name, Mode=TwoWay, ValidatesOnDataErrors=True}"/>
</Canvas>`

func TestValidation_InitialInvalid(t *testing.T) {
	m := &vForm{Name: ""}
	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(valXAML), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ti, _ := reg["nameBox"].(*widget.TextInput)
	if ti == nil {
		t.Fatal("nameBox not found")
	}
	if ti.ValidationError() == "" {
		t.Fatal("expected initial validation error for empty Name")
	}
	if scope.Validate() {
		t.Fatal("scope.Validate() should be false initially")
	}
}

func TestValidation_ClearsWhenValid(t *testing.T) {
	m := &vForm{Name: ""}
	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(valXAML), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ti := reg["nameBox"].(*widget.TextInput)

	// Печатаем "John" — каждый ввод вызывает TwoWay writeBack + проверку.
	typeKeys(ti, "John")
	if !waitFor(func() bool { return ti.ValidationError() == "" }) {
		t.Fatalf("error should clear after valid input, got %q", ti.ValidationError())
	}
	if m.Name != "John" {
		t.Fatalf("model not updated: %q", m.Name)
	}
	if !scope.Validate() {
		t.Fatal("scope.Validate() should be true after valid input")
	}
}

func TestValidation_ReappearsWhenInvalid(t *testing.T) {
	m := &vForm{Name: "John"}
	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(valXAML), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ti := reg["nameBox"].(*widget.TextInput)
	if ti.ValidationError() != "" {
		t.Fatalf("should start valid, got %q", ti.ValidationError())
	}
	// Стираем до "Jo" (len 2) → снова ошибка.
	ti.OnKeyEvent(widget.KeyEvent{Code: widget.KeyBackspace, Pressed: true}) // Joh
	time.Sleep(8 * time.Millisecond)
	ti.OnKeyEvent(widget.KeyEvent{Code: widget.KeyBackspace, Pressed: true}) // Jo
	if !waitFor(func() bool { return ti.ValidationError() != "" }) {
		t.Fatal("error should reappear for short Name")
	}
}
