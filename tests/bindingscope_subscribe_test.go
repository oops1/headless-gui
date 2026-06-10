package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// subscribeModel — тестовая модель с PropertyNotifier.
type subscribeModel struct {
	datagrid.PropertyNotifier
	Value float64
}

func TestBindingScope_NoDuplicateSubscription(t *testing.T) {
	m := &subscribeModel{Value: 42}

	const xaml = `<Canvas xmlns="clr">
		<Slider Name="s" Value="{Binding Value, Mode=TwoWay}" Minimum="0" Maximum="100"/>
	</Canvas>`

	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Тот же контекст 3 раза — обработчиков должно быть ровно 1.
	scope.SetDataContext(m)
	scope.SetDataContext(m)
	scope.SetDataContext(m)
	if got := m.HandlerCount(); got != 1 {
		t.Fatalf("HandlerCount после 3x SetDataContext(m) = %d, want 1", got)
	}

	// Смена контекста: от m отписались, на m2 подписались.
	m2 := &subscribeModel{Value: 10}
	scope.SetDataContext(m2)
	if got := m.HandlerCount(); got != 0 {
		t.Fatalf("HandlerCount(m) после SetDataContext(m2) = %d, want 0", got)
	}
	if got := m2.HandlerCount(); got != 1 {
		t.Fatalf("HandlerCount(m2) после SetDataContext(m2) = %d, want 1", got)
	}
}
