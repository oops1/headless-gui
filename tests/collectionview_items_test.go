package tests

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// VM с CollectionView в качестве источника ItemsControl.
type cvVM struct {
	datagrid.PropertyNotifier
	People *widget.CollectionView
}

const cvItemsXAML = `<Canvas xmlns="clr">
	<ItemsControl Name="lst" ItemsSource="{Binding People}">
		<ItemsControl.ItemTemplate>
			<DataTemplate>
				<TextBlock Text="{Binding Name}"/>
			</DataTemplate>
		</ItemsControl.ItemTemplate>
	</ItemsControl>
</Canvas>`

func TestCollectionView_ItemsControlLiveRebuild(t *testing.T) {
	view := widget.NewCollectionView(sampleOC())
	vm := &cvVM{People: view}

	_, reg, _, err := widget.LoadUIFromXAMLBindings([]byte(cvItemsXAML), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	panel := reg["lst"]
	if panel == nil {
		t.Fatal("lst panel not found")
	}
	// Изначально 4 элемента.
	if n := len(panel.Children()); n != 4 {
		t.Fatalf("initial children = %d, want 4", n)
	}

	// Фильтр оставляет только взрослых (Charlie, Bob) → панель должна перестроиться.
	view.SetFilter(func(it interface{}) bool { return it.(*person).Age >= 18 })

	ok := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(panel.Children()) == 2 {
			ok = true
			break
		}
		time.Sleep(3 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("after filter children = %d, want 2", len(panel.Children()))
	}
}
