package widget

import (
	"testing"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

type isRow struct {
	Name  string
	Price string
}

type isNode struct {
	Name     string
	Children *dgridPkg.ObservableCollection
}

type isVM struct {
	dgridPkg.PropertyNotifier
	Rows  *dgridPkg.ObservableCollection
	Roots *dgridPkg.ObservableCollection
	Title string
}

const itemsSourceXAML = `<Canvas Width="600" Height="400">
  <TextBlock Name="title" Text="{Binding Title}"/>
  <DataGrid Name="grid" Width="300" Height="200" ItemsSource="{Binding Rows}" AutoGenerateColumns="False">
    <DataGrid.Columns>
      <DataGridTextColumn Header="Name" Binding="{Binding Name}" Width="150"/>
      <DataGridTextColumn Header="Price" Binding="{Binding Price}" Width="150"/>
    </DataGrid.Columns>
  </DataGrid>
  <TreeView Name="tree" Left="300" Width="300" Height="200" ItemsSource="{Binding Roots}">
    <TreeView.ItemTemplate>
      <HierarchicalDataTemplate ItemsSource="{Binding Children}">
        <TextBlock Text="{Binding Name}"/>
      </HierarchicalDataTemplate>
    </TreeView.ItemTemplate>
  </TreeView>
</Canvas>`

// ItemsSource="{Binding …}" у таблицы и дерева подаёт саму коллекцию, а
// шаблон дерева из разметки находит детей и имена узлов. Раньше таблица из
// такой разметки была пустой, а у шаблона пути детей и заголовка
// затирались пустотой ещё при загрузке.
func TestXAMLItemsSource_GridAndTree(t *testing.T) {
	leaf := dgridPkg.NewObservableCollectionFrom([]interface{}{
		&isNode{Name: "RichText"}, &isNode{Name: "TextBox"},
	})
	vm := &isVM{
		Rows: dgridPkg.NewObservableCollectionFrom([]interface{}{
			&isRow{Name: "a", Price: "1"}, &isRow{Name: "b", Price: "2"},
		}),
		Roots: dgridPkg.NewObservableCollectionFrom([]interface{}{
			&isNode{Name: "widget", Children: leaf},
		}),
		Title: "t",
	}
	_, reg, scope, err := LoadUIFromXAMLBindings([]byte(itemsSourceXAML), vm)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Dispose()

	grid := reg["grid"].(*DataGridWidget)
	if grid.Grid.ItemsSource() != vm.Rows {
		t.Fatalf("таблица не получила коллекцию из привязки")
	}

	tree := reg["tree"].(*TreeViewWidget)
	roots := tree.Tree.Roots()
	if len(roots) != 1 || roots[0].Header != "widget" {
		t.Fatalf("корни дерева: %d, %+v", len(roots), roots)
	}
	kids := roots[0].Children
	if len(kids) != 2 || kids[0].Header != "RichText" {
		t.Fatalf("дети узла из {Binding Children}: %d", len(kids))
	}

	// Изменение другого свойства модели не переставляет источник таблицы
	// заново (иначе каждое изменение сбрасывало бы выделение и сортировку).
	grid.Grid.SetSelectedIndex(1)
	vm.Title = "u"
	vm.NotifyPropertyChanged(vm, "Title")
	if grid.Grid.SelectedItem() != vm.Rows.Get(1) {
		t.Errorf("выделение таблицы сброшено посторонним изменением модели")
	}

	// Новая коллекция в модели — новая в таблице.
	vm.Rows = dgridPkg.NewObservableCollectionFrom([]interface{}{&isRow{Name: "c"}})
	vm.NotifyPropertyChanged(vm, "Rows")
	if grid.Grid.ItemsSource() != vm.Rows {
		t.Errorf("таблица не сменила источник вслед за моделью")
	}
}
