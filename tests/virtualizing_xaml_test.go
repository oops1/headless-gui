package tests

import (
	"fmt"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

type vicVM struct {
	datagrid.PropertyNotifier
	People *widget.CollectionView
}

const vicXAML = `<Canvas xmlns="clr">
	<VirtualizingItemsControl Name="vlist" ItemHeight="24" Width="200" Height="240"
	                          ItemsSource="{Binding People}">
		<VirtualizingItemsControl.ItemTemplate>
			<DataTemplate>
				<TextBlock Text="{Binding Name}"/>
			</DataTemplate>
		</VirtualizingItemsControl.ItemTemplate>
	</VirtualizingItemsControl>
</Canvas>`

func TestVirtualizing_XAML(t *testing.T) {
	big := make([]interface{}, 5000)
	for i := range big {
		big[i] = &person{Name: fmt.Sprintf("P%d", i), Age: i % 99, City: "X"}
	}
	cv := widget.NewCollectionView(datagrid.NewObservableCollectionFrom(big))
	vm := &vicVM{People: cv}

	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(vicXAML), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	vic, _ := reg["vlist"].(*widget.VirtualizingItemsControl)
	if vic == nil {
		t.Fatal("vlist not a VirtualizingItemsControl")
	}
	if vic.ItemCount() != 5000 {
		t.Fatalf("ItemCount = %d, want 5000", vic.ItemCount())
	}
	n := len(vic.Children())
	if n == 0 {
		t.Fatal("no rows materialized")
	}
	if n > 20 {
		t.Fatalf("virtualization broken: %d widgets for 5000 items", n)
	}
	// Первый материализованный виджет должен быть текстом первого элемента.
	if lbl, ok := firstLabel(vic.Children()[0]); ok && lbl != "P0" {
		t.Fatalf("first row = %q, want P0", lbl)
	}
}

// firstLabel достаёт текст из строки-виджета (DataTemplate → Label/StackPanel).
func firstLabel(w widget.Widget) (string, bool) {
	if l, ok := w.(*widget.Label); ok {
		return l.Text(), true
	}
	for _, c := range w.Children() {
		if l, ok := c.(*widget.Label); ok {
			return l.Text(), true
		}
	}
	return "", false
}
