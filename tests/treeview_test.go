// Package tests — тесты TreeView и XAML-загрузки TreeView.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── TreeNode unit ──────────────────────────────────────────────────────────

func TestTreeNode_Basics(t *testing.T) {
	root := widget.NewTreeNode("Root")
	if root.Text != "Root" {
		t.Fatalf("Text = %q, want Root", root.Text)
	}
	if root.HasChildren() {
		t.Fatal("new node should have no children")
	}

	child := widget.NewTreeNode("Child")
	root.AddChild(child)
	if !root.HasChildren() {
		t.Fatal("expected HasChildren=true after AddChild")
	}
	if len(root.Children) != 1 {
		t.Fatalf("Children len = %d, want 1", len(root.Children))
	}
}

// ─── TreeView unit ──────────────────────────────────────────────────────────

func TestTreeView_NewDefaults(t *testing.T) {
	tv := widget.NewTreeView()
	if tv.ItemHeight != 22 {
		t.Fatalf("ItemHeight = %d, want 22", tv.ItemHeight)
	}
	if tv.IndentSize != 18 {
		t.Fatalf("IndentSize = %d, want 18", tv.IndentSize)
	}
	if tv.SelectedNode() != nil {
		t.Fatal("expected nil SelectedNode on new TreeView")
	}
}

func TestTreeView_AddRoot(t *testing.T) {
	tv := widget.NewTreeView()
	tv.AddRoot(widget.NewTreeNode("A"))
	tv.AddRoot(widget.NewTreeNode("B"))
	if len(tv.Roots) != 2 {
		t.Fatalf("Roots len = %d, want 2", len(tv.Roots))
	}
}

func TestTreeView_ClickSelect(t *testing.T) {
	tv := widget.NewTreeView()
	tv.SetBounds(image.Rect(0, 0, 300, 200))

	root := widget.NewTreeNode("Root")
	child := widget.NewTreeNode("Child")
	root.AddChild(child)
	root.Expanded = true
	tv.AddRoot(root)

	var selected *widget.TreeNode
	tv.OnSelect = func(node *widget.TreeNode) {
		selected = node
	}

	// Click on first row (Root) — this also toggles expand (collapses it)
	tv.OnMouseButton(widget.MouseEvent{
		X: 50, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if selected == nil {
		t.Fatal("OnSelect not called")
	}
	if selected.Text != "Root" {
		t.Fatalf("selected = %q, want Root", selected.Text)
	}

	// Root was collapsed by click above; re-expand to see Child
	root.Expanded = true

	// Click on second row (Child) — Y = ItemHeight(22) + half = 33
	tv.OnMouseButton(widget.MouseEvent{
		X: 50, Y: 33, Button: widget.MouseLeft, Pressed: true,
	})
	if selected.Text != "Child" {
		t.Fatalf("selected = %q, want Child", selected.Text)
	}
}

func TestTreeView_ExpandCollapse(t *testing.T) {
	tv := widget.NewTreeView()
	tv.SetBounds(image.Rect(0, 0, 300, 200))

	root := widget.NewTreeNode("Root")
	child := widget.NewTreeNode("Child")
	root.AddChild(child)
	root.Expanded = true
	tv.AddRoot(root)

	// Click on root to collapse (toggle)
	tv.OnMouseButton(widget.MouseEvent{
		X: 50, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if root.Expanded {
		t.Fatal("expected root collapsed after click")
	}

	// Click again to expand
	tv.OnMouseButton(widget.MouseEvent{
		X: 50, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if !root.Expanded {
		t.Fatal("expected root expanded after second click")
	}
}

func TestTreeView_MouseMove_Hover(t *testing.T) {
	tv := widget.NewTreeView()
	tv.SetBounds(image.Rect(0, 0, 300, 200))
	tv.AddRoot(widget.NewTreeNode("A"))
	tv.AddRoot(widget.NewTreeNode("B"))

	// Move into bounds
	tv.OnMouseMove(50, 11)
	// No panic; hover should be updated internally

	// Move out of bounds
	tv.OnMouseMove(500, 500)
	// No panic
}

func TestTreeView_DrawNoPanic(t *testing.T) {
	tv := widget.NewTreeView()
	tv.SetBounds(image.Rect(0, 0, 300, 200))

	root := widget.NewTreeNode("Root")
	root.Expanded = true
	child := widget.NewTreeNode("Child")
	root.AddChild(child)
	tv.AddRoot(root)

	eng := engine.New(300, 200, 30)
	eng.SetRoot(tv)
	eng.Start()
	<-eng.Frames()
	eng.Stop()
}

func TestTreeView_ApplyTheme(t *testing.T) {
	tv := widget.NewTreeView()
	light := widget.LightTheme()
	tv.ApplyTheme(light)

	if tv.Background != light.WindowBG {
		t.Fatalf("Background = %v, want %v", tv.Background, light.WindowBG)
	}
}

// ─── TreeView XAML ──────────────────────────────────────────────────────────

func TestTreeView_XAML_Basic(t *testing.T) {
	xaml := []byte(`
<Panel Width="600" Height="400" Background="Transparent">
    <TreeView Name="tree" Background="#252526" Foreground="#CCCCCC"
              Left="0" Top="0" Width="300" Height="400">
        <TreeViewItem Header="Root" IsExpanded="True">
            <TreeViewItem Header="Child 1"/>
            <TreeViewItem Header="Child 2">
                <TreeViewItem Header="Grandchild"/>
            </TreeViewItem>
        </TreeViewItem>
    </TreeView>
</Panel>`)

	_, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	tv, ok := reg["tree"].(*widget.TreeView)
	if !ok {
		t.Fatalf("tree not found or wrong type: %T", reg["tree"])
	}
	if len(tv.Roots) != 1 {
		t.Fatalf("Roots len = %d, want 1", len(tv.Roots))
	}

	root := tv.Roots[0]
	if root.Text != "Root" {
		t.Fatalf("root.Text = %q, want Root", root.Text)
	}
	if !root.Expanded {
		t.Fatal("root should be expanded (IsExpanded=True)")
	}
	if len(root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(root.Children))
	}
	if root.Children[0].Text != "Child 1" {
		t.Fatalf("child[0] = %q, want Child 1", root.Children[0].Text)
	}

	child2 := root.Children[1]
	if child2.Text != "Child 2" {
		t.Fatalf("child[1] = %q, want Child 2", child2.Text)
	}
	if len(child2.Children) != 1 {
		t.Fatalf("child2 children = %d, want 1", len(child2.Children))
	}
	if child2.Children[0].Text != "Grandchild" {
		t.Fatalf("grandchild = %q, want Grandchild", child2.Children[0].Text)
	}
}

func TestTreeView_XAML_Colors(t *testing.T) {
	xaml := []byte(`
<Panel Width="400" Height="300" Background="Transparent">
    <TreeView Name="tree" Background="#FF0000" Foreground="#00FF00"
              Left="0" Top="0" Width="200" Height="300">
        <TreeViewItem Header="Item"/>
    </TreeView>
</Panel>`)

	_, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	tv := reg["tree"].(*widget.TreeView)
	if tv.Background.R != 255 || tv.Background.G != 0 {
		t.Fatalf("Background = %v, expected red", tv.Background)
	}
	if tv.Foreground.G != 255 || tv.Foreground.R != 0 {
		t.Fatalf("Foreground = %v, expected green", tv.Foreground)
	}
}

// ─── DataGrid XAML ──────────────────────────────────────────────────────────

func TestDataGrid_XAML_AsListView(t *testing.T) {
	xaml := []byte(`
<Panel Width="600" Height="400" Background="Transparent">
    <DataGrid Name="dg" Background="#1E1E1E"
              Left="0" Top="0" Width="600" Height="300">
        <DataGrid.Columns>
            <DataGridTextColumn Header="Name"/>
            <DataGridTextColumn Header="Age"/>
            <DataGridTextColumn Header="Email"/>
        </DataGrid.Columns>
    </DataGrid>
</Panel>`)

	_, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	lv, ok := reg["dg"].(*widget.ListView)
	if !ok {
		t.Fatalf("dg not found or wrong type: %T", reg["dg"])
	}
	// Should have at least the header row
	if lv == nil {
		t.Fatal("ListView is nil")
	}
}

// ─── SmartGit XAML full load ────────────────────────────────────────────────

func TestSmartGit_XAML_Loads(t *testing.T) {
	root, _, err := widget.LoadUIFromXAMLFile("../assets/ui/smartgit.xaml")
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFile: %v", err)
	}
	if root == nil {
		t.Fatal("root is nil")
	}

	// Window should be root
	win, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("root type = %T, want *widget.Window", root)
	}

	// Window должен содержать дочерние виджеты (DockPanel с Menu, Grid и т.д.)
	if len(win.Children()) == 0 {
		t.Fatal("Window has no children")
	}

	// Рекурсивно проверяем что дерево виджетов непустое и содержит
	// ожидаемые типы из smartgit.xaml (DockPanel, MenuBar, Grid, StackPanel, TreeView, etc.)
	var (
		hasDockPanel  bool
		hasMenuBar    bool
		hasGrid       bool
		hasStackPanel bool
		hasTreeView   bool
		hasButton     bool
		hasListView   bool
		totalWidgets  int
	)

	var walk func(w widget.Widget)
	walk = func(w widget.Widget) {
		totalWidgets++
		switch w.(type) {
		case *widget.DockPanel:
			hasDockPanel = true
		case *widget.MenuBar:
			hasMenuBar = true
		case *widget.Grid:
			hasGrid = true
		case *widget.StackPanel:
			hasStackPanel = true
		case *widget.TreeView:
			hasTreeView = true
		case *widget.Button:
			hasButton = true
		case *widget.ListView:
			hasListView = true
		}
		for _, child := range w.Children() {
			walk(child)
		}
	}
	walk(root)

	if !hasDockPanel {
		t.Error("expected DockPanel in widget tree")
	}
	if !hasMenuBar {
		t.Error("expected MenuBar in widget tree")
	}
	if !hasGrid {
		t.Error("expected Grid in widget tree")
	}
	if !hasStackPanel {
		t.Error("expected StackPanel in widget tree")
	}
	if !hasTreeView {
		t.Error("expected TreeView in widget tree")
	}
	if !hasButton {
		t.Error("expected Button in widget tree")
	}
	if !hasListView {
		t.Error("expected ListView (DataGrid) in widget tree")
	}

	t.Logf("Total widgets in tree: %d", totalWidgets)
}
