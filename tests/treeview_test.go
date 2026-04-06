// Package tests — тесты TreeView и XAML-загрузки TreeView.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// ─── TreeViewItem unit ─────────────────────────────────────────────────────

func TestTreeViewItem_Basics(t *testing.T) {
	root := treeview.NewItem("Root")
	if root.Text != "Root" {
		t.Fatalf("Text = %q, want Root", root.Text)
	}
	if root.HasChildren() {
		t.Fatal("new node should have no children")
	}

	child := treeview.NewItem("Child")
	root.AddChild(child)
	if !root.HasChildren() {
		t.Fatal("expected HasChildren=true after AddChild")
	}
	if len(root.Children) != 1 {
		t.Fatalf("Children len = %d, want 1", len(root.Children))
	}
}

func TestTreeViewItem_Compat(t *testing.T) {
	// Проверяем обратную совместимость через widget.NewTreeNode
	root := widget.NewTreeNode("Root")
	child := widget.NewTreeNode("Child")
	root.AddChild(child)
	if root.Text != "Root" {
		t.Fatalf("Text = %q, want Root", root.Text)
	}
	if !root.HasChildren() {
		t.Fatal("expected HasChildren=true")
	}
}

func TestTreeViewItem_DisplayText(t *testing.T) {
	item := treeview.NewItem("text")
	if item.DisplayText() != "text" {
		t.Fatalf("DisplayText = %q, want text", item.DisplayText())
	}
	item.Header = "header"
	if item.DisplayText() != "header" {
		t.Fatalf("DisplayText = %q, want header", item.DisplayText())
	}
}

func TestTreeViewItem_Depth(t *testing.T) {
	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	grandchild := treeview.NewItem("Grandchild")
	root.AddChild(child)
	child.AddChild(grandchild)

	if root.Depth() != 0 {
		t.Fatalf("root depth = %d, want 0", root.Depth())
	}
	if child.Depth() != 1 {
		t.Fatalf("child depth = %d, want 1", child.Depth())
	}
	if grandchild.Depth() != 2 {
		t.Fatalf("grandchild depth = %d, want 2", grandchild.Depth())
	}
}

func TestTreeViewItem_Parent(t *testing.T) {
	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	root.AddChild(child)

	if child.Parent() != root {
		t.Fatal("child.Parent() should be root")
	}
	if root.Parent() != nil {
		t.Fatal("root.Parent() should be nil")
	}
}

func TestTreeViewItem_RemoveChild(t *testing.T) {
	root := treeview.NewItem("Root")
	child1 := treeview.NewItem("C1")
	child2 := treeview.NewItem("C2")
	root.AddChild(child1)
	root.AddChild(child2)
	root.RemoveChild(child1)

	if len(root.Children) != 1 {
		t.Fatalf("Children len = %d, want 1", len(root.Children))
	}
	if root.Children[0] != child2 {
		t.Fatal("remaining child should be C2")
	}
}

func TestTreeViewItem_ClearChildren(t *testing.T) {
	root := treeview.NewItem("Root")
	root.AddChild(treeview.NewItem("A"))
	root.AddChild(treeview.NewItem("B"))
	root.ClearChildren()
	if root.HasChildren() {
		t.Fatal("expected no children after ClearChildren")
	}
}

// ─── TreeView unit ─────────────────────────────────────────────────────────

func TestTreeView_NewDefaults(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	if tw.Tree.ItemHeight != 0 {
		t.Fatalf("ItemHeight = %d, want 0 (default)", tw.Tree.ItemHeight)
	}
	if tw.SelectedNode() != nil {
		t.Fatal("expected nil SelectedNode on new TreeView")
	}
}

func TestTreeView_AddRoot(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.AddRoot(treeview.NewItem("A"))
	tw.AddRoot(treeview.NewItem("B"))
	roots := tw.Tree.Roots()
	if len(roots) != 2 {
		t.Fatalf("Roots len = %d, want 2", len(roots))
	}
}

func TestTreeView_ClearRoots(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.AddRoot(treeview.NewItem("A"))
	tw.AddRoot(treeview.NewItem("B"))
	tw.ClearRoots()
	roots := tw.Tree.Roots()
	if len(roots) != 0 {
		t.Fatalf("Roots len = %d, want 0", len(roots))
	}
}

func TestTreeView_ClickSelect(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	root.AddChild(child)
	root.Expanded = true
	tw.AddRoot(root)

	var selected *widget.TreeNode
	tw.Tree.OnSelect = func(node *widget.TreeNode) {
		selected = node
	}

	// Click on first row (Root)
	tw.OnMouseButton(widget.MouseEvent{
		X: 50, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if selected == nil {
		t.Fatal("OnSelect not called")
	}
	if selected.Text != "Root" {
		t.Fatalf("selected = %q, want Root", selected.Text)
	}
}

func TestTreeView_ExpandCollapse(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	root.AddChild(child)
	root.Expanded = true
	tw.AddRoot(root)

	// Click on arrow zone to collapse
	tw.OnMouseButton(widget.MouseEvent{
		X: 10, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if root.Expanded {
		t.Fatal("expected root collapsed after click on arrow")
	}

	// Click again to expand
	tw.OnMouseButton(widget.MouseEvent{
		X: 10, Y: 11, Button: widget.MouseLeft, Pressed: true,
	})
	if !root.Expanded {
		t.Fatal("expected root expanded after second click")
	}
}

func TestTreeView_MouseMove_Hover(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))
	tw.AddRoot(treeview.NewItem("A"))
	tw.AddRoot(treeview.NewItem("B"))

	// Move into bounds
	tw.OnMouseMove(50, 11)
	// No panic

	// Move out of bounds
	tw.OnMouseMove(500, 500)
	// No panic
}

func TestTreeView_DrawNoPanic(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	root.Expanded = true
	child := treeview.NewItem("Child")
	root.AddChild(child)
	tw.AddRoot(root)

	eng := engine.New(300, 200, 30)
	eng.SetRoot(tw)
	eng.Start()
	<-eng.Frames()
	eng.Stop()
}

func TestTreeView_ApplyTheme(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	light := widget.LightTheme()
	tw.ApplyTheme(light)

	if tw.Tree.Theme.Background != light.WindowBG {
		t.Fatalf("Background = %v, want %v", tw.Tree.Theme.Background, light.WindowBG)
	}
}

func TestTreeView_Keyboard(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	a := treeview.NewItem("A")
	b := treeview.NewItem("B")
	c := treeview.NewItem("C")
	tw.AddRoot(a)
	tw.AddRoot(b)
	tw.AddRoot(c)

	// Select first item
	tw.Tree.SetSelectedItem(a)

	// Press Down arrow
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	if tw.Tree.SelectedItem() != b {
		t.Fatalf("after Down: selected = %q, want B", tw.Tree.SelectedItem().Text)
	}

	// Press Down again
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	if tw.Tree.SelectedItem() != c {
		t.Fatalf("after Down: selected = %q, want C", tw.Tree.SelectedItem().Text)
	}

	// Press Up
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUp, Pressed: true})
	if tw.Tree.SelectedItem() != b {
		t.Fatalf("after Up: selected = %q, want B", tw.Tree.SelectedItem().Text)
	}

	// Home
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyHome, Pressed: true})
	if tw.Tree.SelectedItem() != a {
		t.Fatalf("after Home: selected = %q, want A", tw.Tree.SelectedItem().Text)
	}

	// End
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnd, Pressed: true})
	if tw.Tree.SelectedItem() != c {
		t.Fatalf("after End: selected = %q, want C", tw.Tree.SelectedItem().Text)
	}
}

func TestTreeView_KeyboardExpandCollapse(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	root.AddChild(child)
	tw.AddRoot(root)

	tw.Tree.SetSelectedItem(root)

	// Right → expand
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
	if !root.Expanded {
		t.Fatal("expected root expanded after Right")
	}

	// Left → collapse
	tw.OnKeyEvent(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true})
	if root.Expanded {
		t.Fatal("expected root collapsed after Left")
	}
}

func TestTreeView_ScrollBy(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 44)) // 2 rows visible at 22px each

	for i := 0; i < 10; i++ {
		tw.AddRoot(treeview.NewItem("Item"))
	}

	tw.ScrollBy(44)
	// No panic, scroll should be clamped
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

	tw, ok := reg["tree"].(*widget.TreeViewWidget)
	if !ok {
		t.Fatalf("tree not found or wrong type: %T", reg["tree"])
	}

	roots := tw.Tree.Roots()
	if len(roots) != 1 {
		t.Fatalf("Roots len = %d, want 1", len(roots))
	}

	root := roots[0]
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

	tw := reg["tree"].(*widget.TreeViewWidget)
	if tw.Tree.Theme.Background.R != 255 || tw.Tree.Theme.Background.G != 0 {
		t.Fatalf("Background = %v, expected red", tw.Tree.Theme.Background)
	}
	if tw.Tree.Theme.Foreground.G != 255 || tw.Tree.Theme.Foreground.R != 0 {
		t.Fatalf("Foreground = %v, expected green", tw.Tree.Theme.Foreground)
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

	win, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("root type = %T, want *widget.Window", root)
	}

	if len(win.Children()) == 0 {
		t.Fatal("Window has no children")
	}

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
		case *widget.TreeViewWidget:
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
