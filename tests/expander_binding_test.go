package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// expanderModel — тестовая модель для Expander-биндинга.
type expanderModel struct {
	datagrid.PropertyNotifier
	Title string
	Open  bool
}

func TestExpander_HeaderAndIsExpandedBinding(t *testing.T) {
	m := &expanderModel{Title: "Секция", Open: true}

	const xaml = `<Canvas xmlns="clr">
		<Expander Name="ex" Header="{Binding Title}" IsExpanded="{Binding Open}"/>
	</Canvas>`

	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = scope
	ex := reg["ex"].(*widget.Expander)

	// Начальные значения из модели.
	if ex.Header != "Секция" {
		t.Fatalf("начальный Header = %q, want Секция", ex.Header)
	}
	if !ex.IsExpanded {
		t.Fatalf("начальный IsExpanded = false, want true")
	}

	// Смена модели + уведомление → виджет обновился.
	m.Title = "Новый раздел"
	m.Open = false
	m.NotifyPropertyChanged(m, "Title")
	m.NotifyPropertyChanged(m, "Open")

	if ex.Header != "Новый раздел" {
		t.Fatalf("после Notify: Header = %q, want \"Новый раздел\"", ex.Header)
	}
	if ex.IsExpanded {
		t.Fatalf("после Notify: IsExpanded = true, want false")
	}
}

func TestExpander_ElementNameBinding(t *testing.T) {
	const xaml = `<Canvas xmlns="clr">
		<Expander Name="ex" IsExpanded="False"/>
		<TextBlock Name="tb" Text="{Binding IsExpanded, ElementName=ex}"/>
	</Canvas>`

	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(xaml), nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ex := reg["ex"].(*widget.Expander)
	tb := reg["tb"].(*widget.Label)

	// Начальное значение: ex.IsExpanded == false → текст "false".
	if tb.Text() != "false" {
		t.Fatalf("начальный текст = %q, want \"false\"", tb.Text())
	}

	// SetExpanded → текст должен обновиться через ElementName-биндинг.
	ex.SetExpanded(true)
	if tb.Text() != "true" {
		t.Fatalf("после SetExpanded(true): текст = %q, want \"true\"", tb.Text())
	}
}
