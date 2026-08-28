# headless-gui — Developer Guide

## Overview

`headless-gui` is an off-screen GUI engine written in Go. It renders widgets into an RGBA buffer and outputs only changed 64x64 px tiles (delta compression). It does not depend on any window system — output is pluggable (RDP, WebSocket, native window).

```
headless-gui/
  engine/              render loop, canvas, events, fonts
  widget/              widgets, themes, XAML loader, Grid layout
    treeview/          TreeView core (model, templates, rendering, input)
    datagrid/          DataGrid core (ObservableCollection, PropertyNotifier)
  output/              Frame / DirtyTile types
  window/              native window Win32/Cocoa/X11 (separate go.mod, CGO-free)
  cmd/
    showcase/          full widget showcase (all widgets + live animation)
    guiview/           interactive demo with modal windows
    griddemo/          Grid layout demo
    smartgit/          SmartGit-like UI (Window + Menu + TreeView + DataGrid)
  assets/ui/           XAML layouts (demo.xaml, grid_demo.xaml, showcase.xaml)
  gui/                 XAML files for RDP UI (login, block, error dialogs)
  tests/               unit tests
```

---

## Quick Start

```go
import (
    "image"
    "image/color"
    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
)

eng := engine.New(1920, 1080, 30)   // width, height, FPS

root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
root.SetBounds(image.Rect(0, 0, 1920, 1080))

btn := widget.NewWin10AccentButton("Login")
btn.SetBounds(image.Rect(860, 500, 1060, 540))
btn.OnClick = func() { fmt.Println("Clicked!") }
root.AddChild(btn)

eng.SetRoot(root)
eng.Start()
defer eng.Stop()

for frame := range eng.Frames() {
    for _, tile := range frame.Tiles {
        sendToClient(tile)  // tile.X, tile.Y, tile.W, tile.H, tile.Data
    }
}
```

---

## Engine (engine.Engine)

```go
eng := engine.New(width, height, fps)

// Root and styling
eng.SetRoot(w widget.Widget)
eng.SetTheme(t *widget.Theme)
eng.SetBackgroundFile(path string)    // PNG/JPEG
eng.SetResolution(width, height int)  // change on the fly

// Fonts
eng.RegisterFont(name string, ttf []byte)
eng.RegisterFontFile(name, path string)
eng.SetDPI(dpi float64)              // default 96

// Lifecycle
eng.Start()
eng.Stop()                            // closes Frames() channel
eng.Frames() <-chan output.Frame
eng.CanvasSize() (w, h int)
eng.SaveFrames(dir string)            // debug: save PNG frames to disk

// Input
eng.SetFocus(w widget.Widget)
eng.SendKeyEvent(e widget.KeyEvent)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
eng.SendMouseMove(x, y int)

// Modal dialogs
eng.ShowModal(m widget.ModalWidget)
eng.CloseModal(m widget.ModalWidget)
```

`output.Frame` contains `Seq uint64`, `Timestamp time.Time`, and `[]DirtyTile{X, Y, W, H int; Data []byte}`.

---

## Widgets

Every widget embeds `widget.Base`, which implements `SetBounds`, `AddChild`, `Children`, and Grid attached properties (`GridRow`, `GridColumn`, `GridRowSpan`, `GridColSpan`).

```go
w.SetBounds(image.Rect(x, y, x+w, y+h))  // required before first frame
parent.AddChild(child)
```

### Window

Root element for native OS window. Replaces Canvas/Panel as root when working with native window.

```go
// XAML loading (recommended)
root, reg, _ := widget.LoadUIFromXAMLFile("ui/app.xaml")
eng.SetRoot(root)

// Programmatically
ww := widget.NewWindow()
ww.Title = "My Application"
ww.TitleStyle = widget.WindowTitleWin  // or WindowTitleMac
ww.Resize = widget.ResizeModeCanResize
```

In XAML:

```xml
<Window Title="Application" Width="1100" Height="700"
        TitleStyle="Win" ResizeMode="CanResize" Background="#1E1E1E">
    <DockPanel>
        <Menu DockPanel.Dock="Top">...</Menu>
        <Grid>...</Grid>
    </DockPanel>
</Window>
```

Title bar styles:
- `WindowTitleWin` — Windows: text on left, buttons ─ □ × on right
- `WindowTitleMac` — macOS: traffic lights ● ● ● on left, centered text

Resize modes: `CanResize`, `NoResize`, `CanMinimize`.

### Panel

Container with background, border, rounded corners, background image, and built-in window title bar.

```go
p := widget.NewPanel(color.RGBA{R: 45, G: 45, B: 65, A: 255})
p.ShowBorder    = true
p.BorderColor   = color.RGBA{...}
p.CornerRadius  = 8
p.UseAlpha      = true

widget.NewWin10Panel()  // standard semi-transparent dark panel
```

**Background image** — loaded via XAML attribute `BackgroundImage="pam.png"` (path relative to XAML file). The image is scaled to fit the panel. Supports PNG and JPEG.

**Title bar:**

```go
p.Caption      = "My Application"
p.ShowHeader   = true           // default true
p.MacStyle     = false          // false=Windows, true=macOS
p.HeaderHeight = 38             // default 32px
p.OnClose      = func() { ... } // close button callback
```

Windows style: dark bar, left-aligned text, decorative buttons on the right. macOS style: traffic lights on the left, centered text.

### Grid

WPF-compatible grid layout with three sizing modes: Pixel, Star (proportional), Auto (content-based).

```go
g := widget.NewGrid()
g.RowDefs = []widget.GridDefinition{
    {Mode: widget.GridSizePixel, Value: 48},  // 48px
    {Mode: widget.GridSizeStar,  Value: 1},   // *
    {Mode: widget.GridSizePixel, Value: 40},  // 40px
}
g.ColDefs = []widget.GridDefinition{
    {Mode: widget.GridSizePixel, Value: 200}, // 200px
    {Mode: widget.GridSizeStar,  Value: 1},   // *
}
g.ShowGridLines = true  // debug mode
```

Children specify their cell via attached properties:

```go
label.SetGridProps(row, col, rowSpan, colSpan)
// or in XAML: Grid.Row="1" Grid.Column="0" Grid.ColumnSpan="2"
```

In XAML:

```xml
<Grid Width="800" Height="500" ShowGridLines="True">
    <Grid.RowDefinitions>
        <RowDefinition Height="48"/>
        <RowDefinition Height="*"/>
        <RowDefinition Height="40"/>
    </Grid.RowDefinitions>
    <Grid.ColumnDefinitions>
        <ColumnDefinition Width="200"/>
        <ColumnDefinition Width="*"/>
    </Grid.ColumnDefinitions>

    <Label Grid.Row="0" Grid.Column="0" Grid.ColumnSpan="2"
           Text="Header" Foreground="White" Background="#0078D4"/>
    <Button Grid.Row="2" Grid.Column="1" Content="OK" Style="Accent"/>
</Grid>
```

### Label

```go
lbl := widget.NewWin10Label("Text")
lbl := widget.NewLabel("Text", color.RGBA{...})

lbl.SetText("new text")  // thread-safe
lbl.Text() string
lbl.WrapText = true       // word wrap by width
lbl.FontSize = 14.0
```

In XAML: `TextWrapping="Wrap"`, `FontSize="14"`.

### Button

```go
btn := widget.NewButton("Text")
btn := widget.NewWin10AccentButton("OK")  // blue accent, primary action

btn.OnClick   = func() { ... }
btn.HoverBG   = color.RGBA{...}  // hover color
btn.PressedBG = color.RGBA{...}  // pressed color
```

In XAML: `HoverBG="#C42B1C"`, `PressedBG="#A01E14"`, `Background`, `Foreground`, `BorderBrush`.

### TextInput

```go
inp := widget.NewTextInput("placeholder...")

inp.SetText("value")
inp.GetText() string

inp.OnEnter  = func() { ... }
inp.OnChange = func(text string) { ... }
```

Keyboard: Backspace, Delete, arrows, Home, End. Shift+arrows for selection. Ctrl+A/C/X/V for clipboard.

### PasswordBox

```go
inp := widget.NewPasswordInput("Enter password...")
```

In XAML: `<PasswordBox Placeholder="Password..."/>`.

### Dropdown

```go
dd := widget.NewDropdown("Item 1", "Item 2", "Item 3")

dd.SetSelected(idx int)
dd.Selected() int
dd.SelectedText() string
dd.OnChange = func(idx int, text string) { ... }
```

In XAML — two variants:

```xml
<ComboBox Items="RDP,VNC,SSH" SelectedIndex="0"/>

<ComboBox>
    <ComboBoxItem Content="Administrator"/>
    <ComboBoxItem Content="Operator"/>
</ComboBox>
```

### CheckBox

```go
cb := widget.NewCheckBox("Remember me")

cb.SetChecked(true)
cb.IsChecked() bool
cb.OnChange = func(checked bool) { ... }
```

### RadioButton

```go
rb1 := widget.NewRadioButton("Option A", "myGroup")
rb2 := widget.NewRadioButton("Option B", "myGroup")

rb1.SetSelected(true)  // rb2 is automatically deselected
rb1.IsSelected() bool
rb1.OnChange = func(selected bool) { ... }
rb1.RemoveFromGroup()  // on destruction
```

### ToggleSwitch

```go
ts := widget.NewToggleSwitch("Dark Theme")

ts.SetOn(true)
ts.IsOn() bool
ts.OnChange = func(on bool) { ... }
```

### ProgressBar

```go
pb := widget.NewProgressBar()
pb.SetValue(0.75)   // [0.0, 1.0], thread-safe
pb.Value() float64
```

In XAML: `<ProgressBar Value="0.65" Foreground="#A6E3A1"/>`.

### Slider

```go
s := widget.NewSlider()            // [0.0, 1.0]
s := widget.NewSliderRange(0, 100) // custom range

s.SetValue(0.5)
s.Value() float64
s.OnChange = func(value float64) { ... }
```

Keyboard: arrows for 5% step, Shift+arrows for 1% step, Home/End for min/max.

### TabControl

```go
tc := widget.NewTabControl(
    widget.TabItem{Header: "General",   Content: generalPanel},
    widget.TabItem{Header: "Settings",  Content: settingsPanel},
)

tc.AddTab("More", anotherPanel)
tc.SetActive(0)
tc.Active() int
tc.TabCount() int
tc.OnTabChange = func(index int, header string) { ... }
```

In XAML:

```xml
<TabControl SelectedIndex="0">
    <TabItem Header="General">
        <Canvas Width="600" Height="368">
            <Label Left="10" Top="10" Text="Content"/>
        </Canvas>
    </TabItem>
</TabControl>
```

### ScrollView

```go
sv := widget.NewScrollView()
sv.ContentHeight = 2000

sv.AddChild(longPanel)
sv.ScrollY() int
sv.SetScrollY(100)
sv.ScrollBy(50)
```

### ListView

```go
lv := widget.NewListView("Item 1", "Item 2", "Item 3")

lv.AddItem("More")
lv.Clear()
lv.SetSelected(0)
lv.Selected() int        // -1 if no selection
lv.SelectedText() string
lv.OnSelect = func(index int, text string) { ... }
```

In XAML:

```xml
<ListView>
    <ListViewItem Content="Entry 1"/>
    <ListViewItem Content="Entry 2"/>
</ListView>
```

### Image

```go
img := widget.NewImageWidget()
img.SetSource("assets/logo.png")  // PNG or JPEG
img.SetImage(myImage)             // image.Image directly
img.Stretch = widget.ImageStretchFill     // stretch to fill (default)
              widget.ImageStretchUniform  // fit preserving aspect ratio
              widget.ImageStretchNone     // original size
```

### PopupMenu

Context / popup menu. Renders as an overlay on top of the entire UI.

```go
menu := widget.NewPopupMenu()
menu.AddItem("Copy", func() { /* ... */ })
menu.AddItem("Paste", func() { /* ... */ })
menu.AddSeparator()
menu.AddItem("Delete", func() { /* ... */ })

menu.OnSelect = func(idx int, text string) {
    log.Printf("Selected: %s", text)
}

menu.Show(x, y)          // show at coordinates
menu.ShowBelow(button)    // show below a widget
menu.ShowRight(widget)    // show to the right of a widget
menu.Close()              // close
```

XAML:

```xml
<PopupMenu Name="ctxMenu">
    <MenuItem Text="Copy"/>
    <MenuItem Text="Paste"/>
    <MenuItem Separator="True"/>
    <MenuItem Text="Disabled item" Disabled="True"/>
    <MenuItem Text="Delete"/>
</PopupMenu>
```

Closes on click outside or Escape. Arrow keys and Enter for keyboard navigation.

### MenuBar

Horizontal menu bar (classic Windows-style). Each top-level item opens a PopupMenu with sub-items. When hovering over an adjacent item, the submenu automatically switches.

```go
menu := widget.NewMenuBar()
menu.AddMenu("File",
    widget.MenuItem{Text: "New"},
    widget.MenuItem{Text: "Open"},
    widget.MenuItem{Separator: true},
    widget.MenuItem{Text: "Exit"},
)
menu.AddMenu("Edit",
    widget.MenuItem{Text: "Copy"},
    widget.MenuItem{Text: "Paste"},
)

menu.OnSelect = func(topIdx, subIdx int, text string) {
    log.Printf("Menu: %s", text)
}
```

XAML:

```xml
<Menu Name="mainMenu" Left="0" Top="0" Width="800" Height="28">
    <MenuItem Header="File">
        <MenuItem Text="New"/>
        <MenuItem Text="Open"/>
        <MenuItem Separator="True"/>
        <MenuItem Text="Exit"/>
    </MenuItem>
    <MenuItem Header="Edit">
        <MenuItem Text="Copy"/>
        <MenuItem Text="Paste"/>
    </MenuItem>
</Menu>
```

Cascading submenus (nested MenuItem):

```xml
<Menu Name="mainMenu">
    <MenuItem Header="Settings">
        <MenuItem Header="Theme">
            <MenuItem Header="Dark"/>
            <MenuItem Header="Light"/>
        </MenuItem>
    </MenuItem>
</Menu>
```

Items with nested submenus display an arrow ▸ on the right. On hover, the child menu opens.

Navigation: Left/Right switches sections, Up/Down/Enter for sub-items, Right to enter cascading submenu, Left to exit, Escape to close.

### TreeView

WPF-compatible hierarchical tree with virtualization, HierarchicalDataTemplate, icons, and keyboard navigation. Architecture: core logic in `widget/treeview/`, wrapper `widget.TreeViewWidget`.

```go
tw := widget.NewTreeViewWidget()
tw.SetBounds(image.Rect(0, 0, 300, 500))

// Create nodes
root := widget.NewTreeNode("Root")
child1 := widget.NewTreeNode("Branch 1")
child2 := widget.NewTreeNode("Branch 2")
leaf := widget.NewTreeNode("Leaf")

child1.AddChild(leaf)
root.AddChild(child1)
root.AddChild(child2)
root.Expanded = true

tw.Tree.AddRoot(root)
```

Node properties (TreeViewItem / TreeNode):

```go
item.Text       // text
item.Header     // WPF alias for Text
item.Icon       // image.Image (icon before text)
item.Expanded   // expanded state
item.IsSelected // selected state
item.IsEnabled  // enabled state
item.Tag        // arbitrary data
item.DataContext // data object for binding
item.Children   // children []*TreeViewItem
```

Node methods:

```go
item.AddChild(child)
item.InsertChild(idx, child)
item.RemoveChild(child)
item.RemoveChildAt(idx)
item.ClearChildren()
item.HasChildren() bool
item.Parent() *TreeViewItem
item.Depth() int
item.DisplayText() string  // Header → Text → fmt.Sprint(DataContext)
```

TreeView properties (via `tw.Tree`):

```go
tw.Tree.ItemHeight       // row height (px), default 22
tw.Tree.IndentSize       // indent per level (px), default 18
tw.Tree.FontSize         // font size, default 10
tw.Tree.IconSize         // icon size (px), default 16
tw.Tree.IsReadOnly       // read-only mode
tw.Tree.ShowIndentGuides // show hierarchy lines
```

Tree management:

```go
tw.Tree.AddRoot(item)
tw.Tree.SetRoots(items)
tw.Tree.ClearRoots()
tw.Tree.Roots() []*TreeViewItem
tw.Tree.SelectedItem() *TreeViewItem
tw.Tree.SetSelectedItem(item)
tw.Tree.ExpandItem(item)
tw.Tree.CollapseItem(item)
tw.Tree.ToggleExpand(item)
```

Events:

```go
tw.Tree.OnSelect = func(item *treeview.TreeViewItem) { ... }

tw.Tree.OnSelectedItemChanged = func(e treeview.SelectedItemChangedEvent) {
    // e.OldItem, e.NewItem
}
tw.Tree.OnExpanded = func(e treeview.ExpandedEvent) { ... }
tw.Tree.OnCollapsed = func(e treeview.CollapsedEvent) { ... }
tw.Tree.OnItemInvoked = func(e treeview.ItemInvokedEvent) { ... } // double-click
```

Data Binding with HierarchicalDataTemplate:

```go
import "github.com/oops1/headless-gui/v3/widget/treeview"

tmpl := &treeview.HierarchicalDataTemplate{
    ItemsSourcePath: "Children",
    HeaderPath:      "Name",
    IconPath:        "Icon",
}
tw.Tree.SetItemTemplate(tmpl)

// ObservableCollection
coll := datagrid.NewObservableCollection()
coll.Add(myDataObject)
tw.Tree.SetItemsSource(coll)
```

Keyboard: ↑/↓ navigation, ←/→ collapse/expand and parent/child traversal, Home/End, PageUp/PageDown, Enter/Space toggle + invoke.

Mouse: click to select, double-click to expand/collapse, click arrow zone to toggle.

In XAML:

```xml
<TreeView Name="tree" Width="300" Height="500"
          IndentSize="20" ShowIndentGuides="True">
    <TreeViewItem Header="Root" IsExpanded="True">
        <TreeViewItem Header="Branch 1">
            <TreeViewItem Header="Leaf"/>
        </TreeViewItem>
        <TreeViewItem Header="Branch 2"/>
    </TreeViewItem>
</TreeView>
```

With HierarchicalDataTemplate:

```xml
<TreeView Name="tree" Width="300" Height="500">
    <TreeView.ItemTemplate>
        <HierarchicalDataTemplate ItemsSource="{Binding Children}">
            <StackPanel Orientation="Horizontal">
                <Image Source="{Binding Icon}" Width="16" Height="16"/>
                <TextBlock Text="{Binding Name}"/>
            </StackPanel>
        </HierarchicalDataTemplate>
    </TreeView.ItemTemplate>
</TreeView>
```

Virtualization: only visible rows are rendered. Supports 10,000+ nodes.

### DataGrid

WPF-compatible data table with columns, sorting, cell editing, column resizing, and virtualization. Architecture: core logic in `widget/datagrid/`, wrapper `widget.DataGridWidget`.

```go
dg := widget.NewDataGridWidget()
dg.SetBounds(image.Rect(0, 0, 800, 400))

// Add columns
dg.Grid.AddColumn(datagrid.NewTextColumn("Name", "Name"))
dg.Grid.AddColumn(datagrid.NewTextColumn("Age", "Age"))
dg.Grid.AddColumn(datagrid.NewCheckBoxColumn("Active", "IsActive"))

// Data source
coll := datagrid.NewObservableCollection()
coll.Add(&User{Name: "Alice", Age: 30, IsActive: true})
coll.Add(&User{Name: "Bob", Age: 25, IsActive: false})
dg.Grid.SetItemsSource(coll)
```

Column types:

```go
// Text column — displays and edits string values
datagrid.NewTextColumn("Header", "BindingPath")

// CheckBox column — displays bool as a checkbox
datagrid.NewCheckBoxColumn("Active", "IsActive")

// Template column — custom cell rendering
datagrid.NewTemplateColumn("Actions", func(cdc datagrid.CellDrawContext) {
    // draw via cdc.DrawCtx...
})
```

Column widths (WPF-style):

```go
col.SetWidth(datagrid.StarWidth(1))    // proportional (*)
col.SetWidth(datagrid.StarWidth(2))    // double weight (2*)
col.SetWidth(datagrid.PixelWidth(150)) // fixed 150px
col.SetWidth(datagrid.AutoWidth())     // fit content
```

DataGrid properties (via `dg.Grid`):

```go
dg.Grid.AutoGenerateColumns  // auto-generate columns from data structure
dg.Grid.IsReadOnly           // read-only mode
dg.Grid.CanUserSortColumns   // sort by header click (default true)
dg.Grid.CanUserResizeColumns // resize column widths (default true)
dg.Grid.SelectionMode        // SelectionSingle or SelectionExtended
dg.Grid.RowHeight            // row height (default 28px)
dg.Grid.HeaderHeight         // header height (default 30px)
dg.Grid.FontSize             // font size (default 10)
```

Data management:

```go
dg.Grid.SetItemsSource(coll)           // set data source
dg.Grid.ItemsSource()                  // get ObservableCollection
dg.Grid.SelectedItem() interface{}     // current selected item
dg.Grid.SelectedItems() []interface{}  // all selected (Extended)
dg.Grid.SetSelectedIndex(idx)          // select row by index
```

ObservableCollection — collection with change notifications:

```go
coll := datagrid.NewObservableCollection()
coll.Add(item)            // append
coll.Insert(idx, item)    // insert at index
coll.RemoveAt(idx)        // remove by index
coll.Set(idx, item)       // replace
coll.Clear()              // clear all
coll.Count() int          // count
coll.Get(idx) interface{} // get by index

coll.AddCollectionChanged(func(e datagrid.CollectionChangedEvent) {
    // e.Action: CollectionAdd, CollectionRemove, CollectionReplace, CollectionReset
})
```

Data Binding — property binding for data objects:

```go
// Binding with Path, Converter, StringFormat
b := &datagrid.Binding{
    Path:         "User.Name",        // nested paths via dot
    Mode:         datagrid.TwoWay,    // OneWay, TwoWay, OneTime
    StringFormat: "%.2f",             // output format (optional)
}

// IValueConverter — value transformation
type MyConverter struct{}
func (c *MyConverter) Convert(value interface{}) interface{} { ... }
func (c *MyConverter) ConvertBack(value interface{}) interface{} { ... }
```

INotifyPropertyChanged — property change notifications:

```go
type User struct {
    datagrid.PropertyNotifier
    name string
}

func (u *User) SetName(name string) {
    u.name = name
    u.NotifyPropertyChanged(u, "Name")
}
```

Events:

```go
dg.Grid.OnSelectionChanged = func(e datagrid.SelectionChangedEvent) {
    // e.SelectedIndex, e.SelectedItem
}
dg.Grid.OnSorting = func(e *datagrid.SortingEvent) {
    // e.Column, e.Direction; e.Handled = true to prevent default
}
dg.Grid.OnCellEditEnding = func(e *datagrid.CellEditEndingEvent) {
    // e.RowIndex, e.Column, e.Item, e.NewValue; e.Cancel = true to cancel
}
dg.Grid.OnRowEditEnding = func(rowIndex int, item interface{}) { ... }
```

Keyboard: ↑/↓/←/→ navigation, Home/End, PageUp/PageDown, Tab/Shift+Tab between cells, Enter to start/commit editing, Escape to cancel, Ctrl+A select all (Extended).

Mouse: click to select, double-click to edit, drag column edge to resize, click header to sort.

In XAML:

```xml
<DataGrid Name="grid" Width="800" Height="400"
          AutoGenerateColumns="False"
          CanUserSortColumns="True"
          CanUserResizeColumns="True"
          SelectionMode="Extended"
          IsReadOnly="False"
          RowHeight="28" HeaderHeight="30">
    <DataGrid.Columns>
        <DataGridTextColumn Header="Name"
                           Binding="{Binding Name}" Width="*"/>
        <DataGridTextColumn Header="Age"
                           Binding="{Binding Age}" Width="100"/>
        <DataGridCheckBoxColumn Header="Active"
                               Binding="{Binding IsActive}" Width="60"/>
    </DataGrid.Columns>
</DataGrid>
```

Binding formats: `{Binding Name}`, `{Binding Path=User.Name}`, `"Name"` (no braces).

Width formats: `"*"`, `"2*"`, `"Auto"`, `"150"` (pixels).

### Separator

In XAML: `<Separator Width="400" Height="1" Background="#FF0000"/>`.

### MessageBox

```go
mb := widget.NewMessageBox(eng)

mb.Show("Error", "Something went wrong")                    // OK
mb.ShowYesNo("Exit", "Exit without saving?", callback)       // Yes/No
mb.ShowYesNoCancel("Save", "Save changes?", callback)        // Yes/No/Cancel
```

---

## Input

### Mouse

```go
eng.SendMouseMove(x, y int)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
// btn: widget.MouseLeft | widget.MouseRight | widget.MouseMiddle
```

The engine performs hit-testing and dispatches the event to the appropriate widget. On left click, focus automatically transfers to the `Focusable` widget under the cursor.

### Keyboard

```go
eng.SendKeyEvent(widget.KeyEvent{
    Code:    widget.KeyLeft,
    Rune:    'A',               // for character input (Code = KeyUnknown)
    Mod:     widget.ModCtrl | widget.ModShift,
    Pressed: true,
})
```

Key codes: `KeyBackspace, KeyEnter, KeyEscape, KeyTab, KeySpace, KeyLeft/Right/Up/Down, KeyHome, KeyEnd, KeyDelete, KeyA/C/V/X/Z`.

Modifiers: `ModShift, ModCtrl, ModAlt, ModMeta`.

---

## Themes

```go
eng.SetTheme(widget.DarkTheme())   // Windows 10 Dark (default)
eng.SetTheme(widget.LightTheme())  // Windows 10 Light

// Custom theme
t := widget.DarkTheme()
t.Accent = color.RGBA{R: 200, G: 50, B: 50, A: 255}
eng.SetTheme(t)
```

`SetTheme` applies colors to all existing widgets via `ApplyTheme(t)` and updates global defaults for newly created widgets.

The theme contains 80+ color tokens, grouped by widget:

- Window/panels: `WindowBG`, `PanelBG`, `TitleBG`, `TitleText`, `Border`, `ShadowColor`
- Buttons: `BtnBG`, `BtnHoverBG`, `BtnPressedBG`, `BtnText`, `BtnBorder`
- Text inputs: `InputBG`, `InputText`, `InputFocus`, `InputCaret`, `InputPlaceholder`
- Dropdown/PopupMenu: `DropBG`, `DropText`, `DropBorder`
- TreeView: `TreeText`, `TreeArrow`
- ListView/ScrollView: `ListItemHover`, `ListItemSelect`, `ScrollTrackBG`, `ScrollThumbBG`
- Dialog: `DialogBG`, `DialogTitleBG`, `DialogDim`
- GridSplitter: `SplitterBG`, `SplitterHoverBG`
- StatusBar: `StatusBarBG`, `StatusBarText`
- DataGrid header: `HeaderBG`, `HeaderText`
- System: `Accent`, `Disabled`, `Scrollbar`

---

## XAML

The engine reads standard WPF XAML. Files are compatible with Blend / Visual Studio.

### Loading

```go
root, named, err := widget.LoadUIFromXAMLFile("gui/window.xaml")
if err != nil { log.Fatal(err) }

// Find widget by Name / x:Name
loginBtn := named["btnLogin"].(*widget.Button)
loginBtn.OnClick = func() { ... }

eng.SetRoot(root)
```

Also available: `LoadUIFromXAML(data []byte)` and `LoadUIFromXAMLWithBase(data, baseDir)` for loading from memory.

### Coordinates

Child element coordinates are **relative** (standard WPF Canvas behavior):

```
root Canvas (0,0)
  +-- Border mainWin (Left=100, Top=50)       -> absolute: (100, 50)
       +-- Label (Left=10, Top=5)             -> absolute: (110, 55)
```

For Grid children, coordinates are set by the grid via `Grid.Row` / `Grid.Column` — `Left` and `Top` attributes are ignored.

### XAML Element Reference

| WPF Element | Widget | Key Attributes |
|---|---|---|
| `Canvas`, `Border`, `StackPanel`, `DockPanel` | Panel | `Background`, `CornerRadius`, `Caption`, `ShowHeader`, `MacStyle`, `BackgroundImage`, `BorderBrush` |
| `Grid` | Grid | `ShowGridLines`, `Grid.RowDefinitions`, `Grid.ColumnDefinitions` |
| `Label`, `TextBlock` | Label | `Text`, `Foreground`, `Background`, `TextWrapping`, `FontSize` |
| `Button`, `ToggleButton`, `RepeatButton` | Button | `Content`, `Style="Accent"`, `HoverBG`, `PressedBG`, `Background`, `Foreground`, `BorderBrush` |
| `TextBox` | TextInput | `Placeholder`, `Text`, `Foreground` |
| `PasswordBox` | TextInput (password) | `Placeholder`, `Text` |
| `ComboBox` | Dropdown | `Items`, `SelectedIndex`, child `<ComboBoxItem>` |
| `ProgressBar` | ProgressBar | `Value`, `Foreground` |
| `CheckBox` | CheckBox | `Content`, `IsChecked` |
| `RadioButton` | RadioButton | `Content`, `GroupName`, `IsChecked` |
| `TabControl` | TabControl | `SelectedIndex`, child `<TabItem Header="...">` |
| `Slider` | Slider | `Minimum`, `Maximum`, `Value` |
| `NumericUpDown`, `IntegerUpDown`, `DoubleUpDown` | NumericUpDown | `Minimum`, `Maximum`, `Increment`, `Decimals`, `Value` |
| `ToggleSwitch` | ToggleSwitch | `Content`, `IsOn` |
| `ScrollViewer` | ScrollView | `ContentHeight`, `Background` |
| `ListView`, `ListBox` | ListView | `Items`, `SelectedIndex`, `ItemHeight`, child `<ListViewItem>` |
| `VirtualizingItemsControl` | VirtualizingItemsControl | `ItemHeight`, `Buffer`, `ItemsSource`, `VirtualizingItemsControl.ItemTemplate` |
| `WrapPanel` | WrapPanel | `Spacing`, `Orientation` |
| `UniformGrid` | UniformGrid | `Rows`, `Columns`, `Spacing` |
| `GroupBox` | GroupBox | `Header` |
| `Expander` | Expander | `Header`, `IsExpanded` |
| `Ellipse`, `Rectangle`, `Line`, `Polygon`, `Polyline` | Shapes | `Fill`, `Stroke`, `StrokeThickness`, `Points`, `RadiusX` |
| `Image` | Image | `Source`, `Stretch` (Fill/Uniform/None) |
| `PopupMenu`, `ContextMenu` | PopupMenu | child `<MenuItem Text="..." Separator="True" Disabled="True"/>` |
| `Menu`, `MenuBar`, `MainMenu` | MenuBar | child `<MenuItem Header="...">` with nested `<MenuItem>` |
| `TreeView` | TreeViewWidget | `IndentSize`, `IsReadOnly`, `ShowIndentGuides`, child `<TreeViewItem>`, `<TreeView.ItemTemplate>` |
| `TreeViewItem` | TreeViewItem | `Header`, `IsExpanded`, `Icon`, `IsEnabled` |
| `HierarchicalDataTemplate` | HierarchicalDataTemplate | `ItemsSource="{Binding ...}"`, child `<StackPanel>` with `<Image>` + `<TextBlock>` |
| `DataGrid` | DataGridWidget | `AutoGenerateColumns`, `IsReadOnly`, `CanUserSortColumns`, `CanUserResizeColumns`, `SelectionMode`, `RowHeight`, `HeaderHeight` |
| `DataGridTextColumn` | DataGridTextColumn | `Header`, `Binding`, `Width`, `IsReadOnly`, `SortMemberPath` |
| `DataGridCheckBoxColumn` | DataGridCheckBoxColumn | `Header`, `Binding`, `Width`, `IsReadOnly` |
| `DataGridTemplateColumn` | DataGridTemplateColumn | `Header`, `Width` |
| `SplitPanel` | SplitPanel | `Orientation`, `Position`, `SplitterSize`, `MinFirst`, `MinSecond` (first two children = panes) |
| `SVGIcon` | SVGIcon | `Source`, `Color`, `Tint` |
| `Separator` | Separator | `Background` |
| `DockManager` | DockManager | `Background`, `NativeFloating`; children `<DockPane>`×N + one `<DockContent>` (see "Docking panels") |
| `DockPane` | DockPane | `Id`, `Title`, `Side` (Left/Top/Bottom/Right), `Size`, `State` (Docked/AutoHidden/Floating/Closed); valid only inside `<DockManager>` |
| `DockContent` | — (marker) | single child → `DockManager.SetCenter`; valid only inside `<DockManager>` |
| `Window` | Window | `Title`, `Width`, `Height`, `WindowStyle`, `ResizeMode`, `MainWindow`, `TrayIcon`, `TrayTooltip` (see "Tray from XAML") |
| `TrayMenu` | — (child of `<Window>`) | tray menu: child `<MenuItem>`/`<Separator>` (see "Tray from XAML") |

Common attributes: `Name`/`x:Name`, `Left`/`Canvas.Left`, `Top`/`Canvas.Top`, `Width`, `Height`, `Grid.Row`, `Grid.Column`, `Grid.RowSpan`, `Grid.ColumnSpan`, `ToolTip`, `Visibility`, `IsEnabled`, `TabIndex`. `{Binding ...}` and `{Loc Key}` localization work on any string attribute.

---

## Native Window (window) — Win32 / Cocoa / X11

Separate module with platform-native backends. CGO-free on all platforms (Windows: Win32 API, macOS: Cocoa via purego, Linux: X11 protocol).

```go
import "github.com/oops1/headless-gui/v3/window"

eng := engine.New(1280, 720, 30)
// ... build UI, eng.Start() ...

win := window.New(eng, "Window Title")
win.SetMaxFPS(60)
win.SetResizable(true)

if err := win.Run(); err != nil {  // blocks until window closes
    log.Fatal(err)
}
```

---

## Custom Widget

```go
type MyWidget struct {
    widget.Base                      // required
    Color color.RGBA
}

func (w *MyWidget) Draw(ctx widget.DrawContext) {
    b := w.Bounds()
    ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, w.Color)
    w.Base.DrawChildren(ctx)
}

// Optional interfaces:
func (w *MyWidget) OnMouseButton(e widget.MouseEvent) bool { ... }  // clicks
func (w *MyWidget) OnMouseMove(x, y int)                   { ... }  // hover
func (w *MyWidget) OnKeyEvent(e widget.KeyEvent)           { ... }  // keyboard
func (w *MyWidget) SetFocused(v bool)                      { ... }  // focus
func (w *MyWidget) IsFocused() bool                        { ... }
func (w *MyWidget) ApplyTheme(t *widget.Theme)             { ... }  // themes
```

### DrawContext API

```go
// Rectangles
ctx.FillRect(x, y, w, h int, col color.RGBA)
ctx.FillRectAlpha(x, y, w, h int, col color.RGBA)
ctx.FillRoundRect(x, y, w, h, r int, col color.RGBA)
ctx.DrawBorder(x, y, w, h int, col color.RGBA)
ctx.DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

// Lines
ctx.DrawHLine(x, y, length int, col color.RGBA)
ctx.DrawVLine(x, y, length int, col color.RGBA)
ctx.SetPixel(x, y int, col color.RGBA)

// Images
ctx.DrawImage(src image.Image, x, y int)
ctx.DrawImageScaled(src image.Image, x, y, w, h int)

// Text
ctx.DrawText(text string, x, y int, col color.RGBA)
ctx.DrawTextSize(text string, x, y int, pt float64, col)
ctx.DrawTextFont(text string, x, y int, pt float64, name string, col)
ctx.MeasureText(text string, pt float64) int
ctx.MeasureRunePositions(text string, pt float64) []int

// Clipping
ctx.SetClip(r image.Rectangle)
ctx.ClearClip()
ctx.Clip() image.Rectangle   // current clip rect (for nested clipping)
```

### The draw contract

Starting with v3.15.0 the engine can skip a subtree whose bounding rectangle
does not intersect any changed region (damage) — its widgets' `Draw` is
simply not called that frame. That's a big win on screens where most of the
area is static (a taskbar, a clock, a form sitting next to the one field
being typed into), but it means `Draw` stops being a safe place for anything
other than actual drawing. A widget that follows the four rules below is
unaffected by the optimization; one that doesn't will break in ways that used
to work only by accident.

An application that hasn't been brought into compliance yet can restore the
old behavior with one line: `eng.SetSubtreeCulling(false)` turns culling off
entirely (it's on by default).

**1. `Draw` is not guaranteed every frame.** Being skipped means "what's on
screen is already correct" — a widget has no business assuming that, because
it exists in the tree, it's necessarily being drawn.

**2. Animation goes through `widget.Animate` / `widget.AnimateOwned` only,
never a frame counter or a time measurement taken inside `Draw`.** A counter
that `Draw` bumps on every call stops advancing the moment the widget falls
out of damage — the animation just freezes instead of finishing:

```go
// Bad: the animation only progresses while the engine keeps calling Draw
func (w *MyWidget) Draw(ctx widget.DrawContext) {
    w.frame++                    // stalls the instant Draw stops being called
    ...
}

// Good: ticks on its own clock, regardless of whether this frame draws the widget
widget.AnimateOwned(w, "pulse", 400*time.Millisecond, widget.EaseOutCubic, func(t float64) {
    w.phase = t
    w.Invalidate()
})
```

**3. `Draw` doesn't change widget state — it only paints.** The dangerous
case is computing layout (child positions, hit-test zones) inside `Draw` and
caching it there: skip a `Draw` call and that cache goes stale, so a click
lands on where the widget was in the last frame that actually drew, not where
it visually is now. Compute layout in `SetBounds`/`Layout`; let `Draw` only
read the finished result.

**4. Any change that affects appearance must be paired with `Invalidate()`**
(when the change is within the widget's own bounds) **or
`widget.InvalidateRect(r)`** (when it's outside them — needed by overlays and
popups, which paint somewhere other than where they sit). Skip this and the
engine has no way to know the frame is stale — and under the new scheme
that's no longer just "one extra frame of staleness," it's "this widget never
repaints again" until something else happens to touch the same region.

Separately: a widget that paints **beyond its own bounds** — an `Elevation`
shadow, an overlay, a popup — has to widen its own claimed area itself,
because the subtree bounding rectangle the engine uses to decide whether to
skip `Draw` is computed from widget `Bounds()`, not from what the widget
actually paints. A widget with a shadow poking out past its bounds is a prime
candidate for visual artifacts once culling is on.

---

## New Features

This section covers functionality added after the first revision of the guide:
data binding, styles/triggers/templates, commands, localization, validation,
`CollectionView`, virtualization, and new widgets.

### New widgets

#### NumericUpDown

Numeric spinner field (WPF Extended Toolkit style). XAML tags:
`<NumericUpDown>`, `<IntegerUpDown>`, `<DoubleUpDown>`.

```go
n := widget.NewNumericUpDown()
n.Min, n.Max, n.Step, n.Decimals = 0, 100, 0.5, 2
n.SetValue(9.99)
n.Value()                       // float64
n.OnChange = func(v float64) { ... }
```

```xml
<NumericUpDown Minimum="0" Maximum="100" Increment="1" Value="10"/>
<NumericUpDown Minimum="0" Maximum="10" Increment="0.5" Decimals="1" Value="3.5"/>
```

Interaction: ▲/▼ spinner, mouse wheel, Up/Down keys when focused, direct typing
committed on Enter. Value is always clamped to `[Min, Max]`.

#### VirtualizingItemsControl — UI virtualization

A list that materializes widgets only for the visible window (+a buffer). Handles
tens of thousands of rows.

```go
v := widget.NewVirtualizingItemsControl()
v.ItemHeight = 28
v.SetItemBuilder(func(item any, i int) widget.Widget {
    return widget.NewLabel(item.(*Person).Name, white)
})
v.SetItems(people)              // []any
v.BindCollectionView(view)      // auto-refresh from a CollectionView
```

```xml
<VirtualizingItemsControl ItemHeight="24" Width="240" Height="320"
                          ItemsSource="{Binding People}">
    <VirtualizingItemsControl.ItemTemplate>
        <DataTemplate><TextBlock Text="{Binding Name}"/></DataTemplate>
    </VirtualizingItemsControl.ItemTemplate>
</VirtualizingItemsControl>
```

Requires a fixed `ItemHeight`. Has a built-in scrollbar, mouse wheel, and thumb
dragging.

#### WrapPanel / UniformGrid

```xml
<WrapPanel Spacing="6">
    <Button Content="One" Width="84" Height="30"/>
    <Button Content="Two" Width="84" Height="30"/>
</WrapPanel>
<UniformGrid Columns="3" Spacing="6">
    <Button Content="A"/><Button Content="B"/><Button Content="C"/>
</UniformGrid>
```

`WrapPanel` flows children to the next line; `UniformGrid` lays them out in
equal-sized cells (`Rows`/`Columns`).

#### GroupBox / Expander

```xml
<GroupBox Header="Group" Width="240" Height="104">
    <StackPanel Padding="8" Spacing="6">
        <CheckBox Content="Option 1"/>
        <CheckBox Content="Option 2"/>
    </StackPanel>
</GroupBox>
<Expander Header="Expand" IsExpanded="True" Width="234" Height="104">
    <StackPanel Padding="8"><TextBlock Text="Content"/></StackPanel>
</Expander>
```

`GroupBox` is a titled border; `Expander` is a click-collapsible panel. Both
**clip their content to the inner area** so it never bleeds past the border.

#### Vector shapes

```xml
<Ellipse   Left="20" Top="20" Width="120" Height="80" Fill="#E06C75" Stroke="white" StrokeThickness="3"/>
<Rectangle Left="20" Top="20" Width="160" Height="80" Fill="#98C379" RadiusX="14"/>
<Line      X1="20" Y1="140" X2="200" Y2="200" Stroke="#E5C07B" StrokeThickness="4"/>
<Polygon   Points="280,130 340,210 220,210" Fill="#C678DD" Stroke="white"/>
<Polyline  Points="360,200 380,150 400,200" Stroke="#56B6C2" StrokeThickness="3"/>
```

Go types: `widget.Ellipse`, `widget.RectangleShape`, `widget.Line`,
`widget.Polygon`, `widget.Polyline`.

### TextBox maturity

- **MaxLength** — caps length (typing & paste): `<TextBox MaxLength="20"/>`.
- **Undo/Redo** — `Ctrl+Z` undo, `Ctrl+Y` or `Ctrl+Shift+Z` redo.
- **Double-click** — selects the word under the cursor.

### Data binding ({Binding})

Loading XAML with a DataContext makes bindings live:

```go
root, reg, scope, err := widget.LoadUIFromXAMLBindings(data, viewModel)
scope.SetDataContext(other)   // swap the source
scope.Refresh()               // force-refresh the UI
```

```xml
<TextBox  Text="{Binding Name, Mode=TwoWay}"/>
<TextBlock Text="{Binding Price, StringFormat=$%.2f}"/>
<TextBlock Text="{Binding Value, ElementName=slider1, Converter={StaticResource Pct}}"/>
```

- Modes: `OneWay` (default), `TwoWay`, `OneTime`.
- `INotifyPropertyChanged` (via `datagrid.PropertyNotifier`) refreshes the UI.
- `IValueConverter`: `widget.RegisterValueConverter("Pct", conv)`.
- `ElementName` and `RelativeSource Self` bind to another element's property.
- An `ItemsControl` with `ItemsSource="{Binding Coll}"` + `<DataTemplate>` rebuilds
  live when the `ObservableCollection` changes.

### Resources, styles, triggers, templates

```xml
<Canvas.Resources>
    <SolidColorBrush x:Key="Accent" Color="#0E639C"/>
    <Style x:Key="H1" TargetType="TextBlock">
        <Setter Property="FontSize" Value="15"/>
        <Setter Property="FontWeight" Value="Bold"/>
        <Style.Triggers>
            <DataTrigger Binding="{Binding Status}" Value="ERROR">
                <Setter Property="Foreground" Value="#F38BA8"/>
            </DataTrigger>
        </Style.Triggers>
    </Style>
    <ControlTemplate x:Key="Card">
        <Border Background="{TemplateBinding Background}" BorderBrush="#5A5A6A" BorderThickness="1">
            <ContentPresenter/>
        </Border>
    </ControlTemplate>
</Canvas.Resources>
```

Supported: `StaticResource`, implicit styles by `TargetType`, `BasedOn`,
`Trigger`/`DataTrigger`/`MultiTrigger`/`MultiDataTrigger`, `ControlTemplate` +
`ContentPresenter` + `TemplateBinding`, property-element syntax
(`<X.Background><SolidColorBrush/></X.Background>`), `LinearGradientBrush`.

### Commands and hotkeys

```go
vm.SaveCommand = widget.NewRelayCommand(func() { /* ... */ })
```

```xml
<Canvas.InputBindings>
    <KeyBinding Modifiers="Ctrl" Key="S" Command="{Binding SaveCommand}"/>
</Canvas.InputBindings>
<Button Content="Save" Command="{Binding SaveCommand}"/>
```

### Localization: UI language ≠ keyboard layout

Two **independent axes**:

| What | API | Purpose |
|---|---|---|
| UI language | `SetLanguage` / `Language` / `AddLanguageListener` | language of **labels**; drives `Tr` and `{Loc}` |
| Keyboard layout | `SetLocale` / `Locale` (+ badge/OS applier) | language used to **type** text |

An app can be in Russian while the user types English/Chinese — changing one
never changes the other.

```go
widget.RegisterStrings("EN", map[string]string{"Greeting": "Hello", "Save": "Save"})
widget.RegisterStrings("RU", map[string]string{"Greeting": "Привет", "Save": "Сохранить"})
widget.SetFallbackLanguage("EN")
widget.SetLanguage("RU")
widget.Tr("Greeting")            // "Привет" (missing → EN fallback → key itself)
widget.Trf("Count", 5)           // printf: "Count"="Items: %d" → "Items: 5"
widget.LoadStringsDir("i18n")    // ru.json, en.json → tables by file name
```

In XAML — `{Loc Key}` markup, re-applied **live** on `SetLanguage`:

```xml
<TextBlock Text="{Loc Greeting}"/>
<Button Content="{Loc Save}"/>
```

**Backward compatible:** plain strings are untouched — only `{Loc ...}` is
translated; with no tables `Tr` returns the key itself.

### Validation (IDataErrorInfo)

The `DataContext` model implements `widget.DataErrorInfo`; a binding with
`ValidatesOnDataErrors=True` queries the error text after each write and puts the
widget into an error state (red border + tooltip).

```go
func (m *Form) DataError(prop string) string { // "" == valid
    if prop == "Email" && !strings.Contains(m.Email, "@") {
        return "E-mail must contain '@'"
    }
    return ""
}
```

```xml
<TextBox Text="{Binding Email, Mode=TwoWay, ValidatesOnDataErrors=True}"/>
```

`scope.Validate() bool` re-checks every validating binding (handy before saving a
form).

### CollectionView (sort/filter/group)

```go
view := widget.NewCollectionView(people)          // *ObservableCollection or slice
view.SetFilter(func(it any) bool { return it.(*Person).Age >= 18 })
view.SetSort(
    widget.SortDescription{Property: "City"},
    widget.SortDescription{Property: "Age", Direction: widget.Descending},
)
view.SetGroup("City")
view.Items()    // current view
view.Groups()   // []CollectionViewGroup{Name, Items}
```

`ItemsControl` and `VirtualizingItemsControl` bound to a `CollectionView` rebuild
when the filter/sort/group changes.

### Tooltips, cursors, locale indicator

- `ToolTip="..."` on any widget; delay/toggle via
  `eng.SetTooltipsEnabled`/`SetTooltipDelay`.
- Mouse cursors: widgets implement `Cursor() widget.Cursor` (TextBox → I-beam,
  GridSplitter → resize); `eng.CursorAt(x, y)`.
- Keyboard-layout indicator in windows/dialogs — `ShowLocaleIndicator` property
  with a switch context menu.

### Theme presets and control styling (Win10/Win11/Win2000/Mac)

A theme is not just a palette: `Theme.Style` (`widget.ThemeStyle`) controls the
**shape** of controls:

- `ControlCorner` — corner radius of Button/TextBox/ComboBox/ProgressBar
  (0 = square; Win11 = 6, Mac = 8); an explicit XAML `CornerRadius` wins;
- `Classic3D` — Win2000 classic: square corners, raised bevel buttons (sunken
  when pressed), sunken text fields and checkboxes, a raised arrow button in
  ComboBox, a blocky ProgressBar, no hover tinting.

```go
widget.ThemeNames()            // ["Win10 Dark", ..., "Win2000", "Mac"]
eng.SetTheme(widget.ThemeByName("Win2000"))
widget.CurrentThemeStyle()     // active theme's style (for custom widgets)
```

Constructors: `Win10DarkTheme/Win10LightTheme/Win11DarkTheme/Win11LightTheme/
Win2000Theme/MacTheme` (`DarkTheme`/`LightTheme` == Win10). Mac: #007AFF accent
and a green ToggleSwitch; Win2000: silver #D4D0C8 + navy.

`SetTheme` semantics: container backgrounds follow the theme too — an opaque
`Canvas` background is repainted to `WindowBG`, `Panel` to `PanelBG` (explicit
XAML colors are replaced, consistent with all other widgets).

`window.Window.Close()` closes the native window programmatically (`Run()`
returns) — for a "File → Exit" menu item.

### Render-on-demand and invalidation

Render-on-demand is the **default mode** (since v3.5): frames render only when
the UI actually changed, and only the damaged region is redrawn and diffed
(auto-damage). Idle CPU is near zero; hover/typing cost microseconds instead
of a full frame. The previous "render every tick" behavior:
`eng.SetRenderOnDemand(false)`.

```go
eng.Invalidate()        // mark the whole frame changed (cheap, atomic)
eng.InvalidateRect(r)   // declare a changed region — both drawing and diff
                        // are limited to it
eng.RenderCount()       // frames actually rendered (diagnostics)
```

Tracked automatically: widget setters (`SetText`, `SetValue`, `SetChecked`,
hover/press/focus, `SetBounds` on move/resize — widgets self-invalidate on an
actual change), input events and focus, SetRoot/SetTheme/SetResolution/modals,
the data layer (binding Refresh, `{Loc}`, live collections, locale/language
switches), the blinking caret (`widget.Animated`) and tooltips.

IMPORTANT: direct writes to exported widget fields (`btn.Text = "..."`) are
invisible to the engine — call `btn.Invalidate()` afterwards (available on all
widgets via Base) or `eng.Invalidate()`. Custom widgets with their own visual
state must call `Invalidate()` when it changes.

Locking: a frame no longer holds the engine's main mutex — `SetRoot`/event
dispatch are never blocked by rendering; structural operations (SetResolution,
RegisterFont*, SetTheme) serialize with the frame via a dedicated internal
mutex.

### HiDPI

Widgets live in LOGICAL pixels (like WPF DIPs); frame buffers are physical
(logical × scale). Text rasterizes at the true physical size (crisper, not
stretched); rounded corners and AA shapes scale smoothly.

```go
eng.SetScale(2.0)       // 200% (or 1.25/1.5/...)
eng.CanvasSize()        // logical size (widget coordinate space)
eng.PhysicalSize()      // physical size of frames/tiles
```

`Frames()` tiles and `SendMouse*` events are physical pixels (events are
converted to logical inside the engine).

The native window (`window.Run`) detects the scale automatically: on Windows
per-monitor DPI awareness (v2) is enabled plus WM_DPICHANGED handling when
the window moves between monitors; on X11/macOS set the
`HEADLESS_GUI_SCALE=1.5` environment variable (auto-detection planned).

### Fonts

Bundled free fonts: Roboto (default), Open Sans, Inter. Auto-loaded from
`assets/fonts/`, with a system glyph fallback chain for symbols/emoji (no tofu).

```go
eng.SetDefaultFont("Inter")
eng.RegisterFontFile("Roboto", path)
eng.RegisterFontDir("my/fonts")
eng.AvailableFonts()
```

```xml
<TextBlock FontFamily="Roboto" FontSize="16"/>
```

---

### Standard dialogs (MessageBox, input, progress, files)

A complete set of modal dialogs drawn by the engine itself — they work
headless/streamed (file dialogs show the process/server filesystem), follow
the theme and are localized (`dlg.*` keys, EN/RU built in, live switching):

```go
mb := widget.NewMessageBox(eng)
mb.ShowInfo("", "Document saved.")                    // severity icon + default title
mb.ShowQuestion("", "Save changes?", func(r widget.MessageBoxResult) { ... })
id := mb.ShowInput("", "Name:", "default", validate, onResult); id.SetHint("hint")
pd := mb.ShowProgress("Copying", "file.jpg", onCancel)
pd.SetDetail("34 of 120 · 61 MB/s"); pd.SetProgress(0.28) // or SetIndeterminate(true)
mb.ShowOpenFile(widget.FileDialogOptions{Filters: ...}, func(path string, ok bool) { ... })
mb.ShowSaveFile(widget.FileDialogOptions{InitialName: "a.txt"}, ...)  // compact form
mb.ShowPickFolder(widget.FileDialogOptions{}, ...)
```

Hotkeys: Enter — default button, Escape/✕ — cancel, Ctrl+C in a MessageBox
copies its content in the Windows format (`---` separators). The Open
browser has a places sidebar (custom Places + home/drives), a clickable
breadcrumb path, Name/Size/Modified columns and extension filters.

### Multiline TextBox

`widget.NewTextBox(placeholder)` — an editor: word wrap (`Wrap=false`
gives horizontal scroll), vertical scrolling (wheel, PgUp/PgDn), selection
by mouse and Shift+navigation, Ctrl+arrows word jumps, Ctrl+Home/End,
Ctrl+A/C/X/V, Ctrl+Z/Y, double-click word select, context menu,
`ReadOnly`. In XAML it is built by
`<TextBox AcceptsReturn="True" TextWrapping="Wrap"/>` (without those
attributes the tag still creates the single-line TextInput). Layout uses
`widget.MeasureUIText`, so caret math and scrolling work headless.

### Browser viewer (output/webstream)

Any engine app can be shown in a browser with no client build:

```go
srv := webstream.New(eng)          // the sole consumer of eng.Frames()
go srv.Run()
http.ListenAndServe(":8091", srv)  // "/" — embedded viewer, "/ws" — stream
```

The WebSocket server is written from scratch (RFC 6455, zero deps).
Protocol: binary `init` (canvas size) and tile batches (per-tile PNG with
a u16 x/y/w/h header); the server keeps a composite of the screen and
sends each new client a full keyframe, then only deltas; slow clients
skip frames. Input comes back as JSON (mouse/wheel/keys; the browser's
`e.keyCode` matches `widget.KeyCode`). Demo: go run ./cmd/webshowcase (the entire widget showcase in a browser -
same markup, tabs, themes and localization as the native window, without a
single OS window) or the minimal go run ./cmd/webdemo →
http://localhost:8091.

### Native modal windows and popups (v3.10)

Since v3.10 modal dialogs and popup overlays (dropdowns, context/tray menus)
open in **their own OS windows** rather than being drawn inside the main window:

- A **modal dialog** opens as a separate window, so it can be larger than the
  main window and dragged outside it. Nothing to configure — `window.Window`
  installs the host in `Run()`. A dialog on top of a dialog (a file dialog from
  a regular one) forms a stack of windows.
- **Dropdowns and menus** open in a small window at the target point and are
  **not clipped** by the main window's edge.
- Works natively on **Windows (Win32)** and **Linux (X11)**. On **Wayland,
  macOS and headless** it falls back to the previous behavior: everything is
  drawn into the canvas (functionally identical, just within the window).

```go
dlg := widget.NewDialog("Settings", 1000, 700) // may be larger than the window
dlg.CornerRadius = 8                             // rounded dialog window corners
eng.ShowModal(dlg)
```

### Tray and notifications (Windows)

`window.Window` can drive the notification area. Methods may be called before
`Run()` (state is applied when the window is created) or from UI handlers. On
non-Windows platforms they are polite no-ops (return an error/nothing).

```go
win := window.New(eng, "My App")

// Tray icon (scaled to the system size, transparency from the alpha channel).
win.SetTrayIcon(iconImg, "My App")

// Right-click context menu (our widget.PopupMenu, shown at the cursor).
m := widget.NewPopupMenu()
m.AddItem("Show", func() { win.RestoreFromTray() })
m.AddItem("Hide", func() { win.HideToTray() })
m.AddItem("Quit", func() { win.Close() })
win.SetTrayMenu(m)

// Icon clicks (otherwise the default: double left-click restores the window).
win.SetOnTrayClick(func(btn widget.MouseButton, dbl bool) { /* ... */ })

// Balloon notification (icon by severity). Requires a tray icon to be set.
win.ShowBalloon("Done", "Task finished", widget.SeverityInfo)
win.SetOnBalloonClick(func() { win.RestoreFromTray() })

// Minimize to tray / restore.
win.HideToTray()
win.RestoreFromTray()

win.Run()
```

**Taskbar preview.** The hover thumbnail and Aero Peek show the live window
contents (they used to be black). An additional DWM iconic path is enabled with
the `HEADLESS_GUI_ICONIC_PREVIEW=1` environment variable (not needed by
default).

**Tray from XAML.** The tray icon, tooltip and menu can be declared right in the
root `<Window>` — no imperative `SetTrayIcon`/`SetTrayMenu` for the basic case:

```xml
<Window Title="My App" TrayIcon="icons/app.svg" TrayTooltip="My App">
  <TrayMenu Name="trayMenu">
    <MenuItem Text="Show"/>
    <Separator/>
    <MenuItem Text="Quit"/>
  </TrayMenu>
  <!-- …normal content… -->
</Window>
```

`TrayIcon` is a path relative to the XAML file: `.png`/`.jpg` is decoded as-is,
`.svg` is rasterized to 32×32 (the SVG's own colors are kept, `currentColor` →
theme label color; the tray is intentionally not themed). `TrayTooltip` defaults
to `Title`. `<TrayMenu>` is the single tray-menu child with `<MenuItem>` items
and `<Separator/>`; it is stored in the `Window.TrayMenu` field, not the widget
tree. Wire handlers in code: look the menu up by `Name` and use
`PopupMenu.OnSelect(idx, text)`. **Code wins:** if the app calls
`SetTrayIcon`/`SetTrayMenu` before `Run()`, the XAML declaration is not applied
(the pickup only fills unset fields). Balloon notifications, `SetOnTrayClick` and
`HideToTray` are code-only.

### SplitPanel — two panes with a splitter

`SplitPanel` holds two children (the first two `AddChild` calls — First/Second)
and lays them out on either side of a draggable bar. The position is stored as a
`0..1` fraction, so resizing the window keeps the ratio. Panels nest
(split-in-split).

```go
sp := widget.NewSplitPanel(widget.OrientationHorizontal) // left/right; Vertical — top/bottom
sp.SplitterSize = 6
sp.Position = 0.35        // fraction taken by First
sp.MinFirst, sp.MinSecond = 120, 200
sp.OnPositionChanged = func(pos float64) { /* update a position label */ }

sp.AddChild(leftPanel)    // First
sp.AddChild(rightPanel)   // Second
sp.SetBounds(image.Rect(0, 0, 800, 500))

// Collapse control (same as double-clicking the bar):
sp.Collapse(); sp.Expand(); sp.ToggleCollapse(); _ = sp.IsCollapsed()
```

Hovering the bar shows a resize cursor (`SizeWE`/`SizeNS`), an LMB drag moves the
boundary clamped by `MinFirst`/`MinSecond`, and a double-click on the bar
collapses/restores First. The bar color follows the theme
(`Theme.SplitterBG`/`SplitterHoverBG`). `SplitPanel` is registered in
`HasOwnLayout`, so nesting it inside Canvas/DockPanel does not double-shift its
children.

XAML (the first two child elements are the panes):

```xml
<SplitPanel Orientation="Horizontal" Position="0.35" SplitterSize="6"
            MinFirst="120" MinSecond="200">
  <Panel Background="#1E1E1E"/>   <!-- First -->
  <Panel Background="#252526"/>   <!-- Second -->
</SplitPanel>
```

To resize `Grid` cells, keep using `GridSplitter`.

### SVG icons

The `SVGIcon` widget draws a vector icon from an SVG subset, rasterized to its
bounds preserving aspect. Without an explicit color the icon recolors to the
theme's text color — handy for toolbars and menus.

```go
ic := widget.NewSVGIcon()          // color = Theme.LabelText until set explicitly
ic.SetSVGFile("assets/menu.svg")   // or ic.SetSVG(data []byte)
ic.SetColor(color.RGBA{0xFF, 0x33, 0x66, 0xFF}) // explicit color = currentColor
ic.SetTint(true)                   // recolor the WHOLE content to Color (monochrome)
ic.SetBounds(image.Rect(8, 8, 32, 32))
```

- `fill="currentColor"` in the SVG is replaced with the widget's `Color`.
- `Tint=true` recolors all content to `Color` (monochrome mode); `Tint=false`
  recolors only `currentColor`.
- Supported: `path` (all commands, including arcs and smooth curves),
  `rect`/`circle`/`ellipse`/`line`/`polyline`/`polygon`, group transforms,
  `fill`/`fill-rule` (nonzero + even-odd)/`fill-opacity`, the `style` attribute.
- Limitations: no gradients, `clipPath`, or `text`; stroke is a simple
  approximation.

The `widget/svg` package is also usable directly: `svg.Parse(data)` /
`svg.ParseFile(path)` → `*svg.Document` with `RasterizeCached(w, h, current, tint)`.

XAML (`Source` resolves against the XAML file's directory):

```xml
<SVGIcon Source="icons/menu.svg" Color="#FF3366" Tint="True"/>
<SVGIcon Source="icons/folder.svg"/>   <!-- no Color — theme text color -->
```

### Smooth / inertial scroll

Besides whole wheel "ticks", the engine accepts **pixel-precise deltas** — so
touchpads and precision wheels scroll smoothly.

```go
// Exact delta in physical window/frame pixels (dy>0 — down, dx>0 — right).
eng.SendMouseWheelPixels(x, y, dx, dy)
```

The event bubbles from the deepest widget to the root; the first one implementing
`OnMouseWheelPixels(x, y int, dx, dy float64) bool` and returning `true` consumes
the delta. `ScrollView` starts a flywheel of inertia (a velocity impulse decayed
on the engine clock — no goroutines); any click/press stops the fling, and in
`Classic3D` scrolling is instant. `ListView` and `TextBox` scroll per pixel with
subpixel accumulation and hand the delta to the parent at the edges. If nobody
takes the precise delta, the engine synthesizes equivalent ticks — the old
`MouseWheelUp`/`Down` path is intact.

By platform: Win32 (`WM_MOUSEWHEEL`) and Wayland (`wl_pointer.axis`) deliver exact
pixels; X11 stays on ticks (buttons 4/5), and the macOS wheel is not emitted yet.
Headless works through `SendMouseWheelPixels`.

### File drag & drop from the OS

An app can accept files dragged from Explorer/Finder into the window.

```go
win := window.New(eng, "My App")
win.SetOnFilesDropped(func(paths []string, x, y int) {
    // paths — absolute file paths; x, y — LOGICAL drop coordinates.
    for _, p := range paths { open(p) }
})
win.Run()
```

In parallel the event goes to the engine (`eng.SendFilesDropped(x, y, paths)`,
where `x,y` are physical pixels) and is delivered to the widget under the point
that implements the target interface — giving headless symmetry and testability:

```go
type FileDropTarget interface {
    OnFilesDropped(x, y int, paths []string) bool // true — consume (stop bubbling)
}
```

By platform: **Win32** (`WM_DROPFILES`) and **X11** (XDND v5) are complete;
**Wayland** (`wl_data_device`) is a skeleton that needs live-session verification;
macOS is not supported. Headless routing to `FileDropTarget` is testable via
`SendFilesDropped` without a window.

### Color emoji

The text path renders color glyphs automatically — no separate API, just put
emoji in any widget's string (`👍🎉🚀🔥`). COLRv0 (flat CPAL layers), COLRv1
(paint graph with transforms and solid fills), and CBDT/sbix (PNG bitmaps, e.g.
Noto Color Emoji) are supported. Color glyphs are cached separately from
monochrome masks.

Limitations (honestly): BMP symbols below U+1F000 stay monochrome; regional flags
(letter ligatures) are a known gap; COLRv1 gradients are approximated by their
average color. Works the same headless and in a window.

> **Licensing.** The engine bundles and ships no emoji font — glyphs come from
> the font already installed on the user's OS (Segoe UI Emoji on Windows, Apple
> Color Emoji on macOS, Noto Color Emoji on Linux if present), exactly like the
> system fallback fonts used for Arabic/Hebrew/Thai. Strings hold plain Unicode
> code points, not artwork, so the project carries no extra licensing
> obligation. If you need guaranteed emoji rendering across platforms, bundle a
> freely-licensed font (e.g. Noto Color Emoji, OFL) into your product and ship
> its license.

### Docking panels (Toolbox)

`DockManager` + `DockPane` — a Visual Studio-style docking zone: a document
area in the center (`Center`) and up to four sides (Left/Top/Bottom/Right),
each able to hold a stack of `DockPane` panels. The manager handles: resizing
a side by dragging its gutter, stack tabs (2+ panes on the same side),
auto-hide (collapsing to an edge label), and drag&dock (drag a pane by its
title bar — docking guides appear; drop on an arrow to dock, drop elsewhere
to float).

```go
mgr := widget.NewDockManager()
mgr.SetBounds(image.Rect(0, 0, 1000, 600))

tools := widget.NewDockPane("tools", "Explorer", widget.NewListView("File.txt"))
mgr.AddPane(tools, widget.DockLeft)
mgr.SetSideSize(widget.DockLeft, 220)

props := widget.NewDockPane("props", "Properties", widget.NewWin10Label("—"))
mgr.AddPane(props, widget.DockRight)

mgr.SetCenter(editor) // document area

// Pane state transitions delegate to the manager when the pane belongs to one.
tools.Unpin()  // Docked → AutoHidden (edge label)
tools.Pin()    // back
props.Float()  // Docked/AutoHidden → Floating (floats above the center)
props.Dock(widget.DockRight) // back into the stack

tools.OnStateChanged = func(p *widget.DockPane) {
    log.Println(p.Title, "→", p.State()) // docked/autohidden/floating/closed
}
```

Layout can be saved and restored (JSON, panes matched by `ID`):

```go
data := mgr.SaveLayout()
// ...
_ = mgr.RestoreLayout(data)
```

**Floating panels.** By default `Float()` turns on a widget-drawn floating
panel right inside the canvas (drag/resize with the mouse, headless-testable).
The hook `DockPane.OnFloatNative func(p *DockPane)`, if set, hands the detach
off to a native OS window (`window/**`) instead of the widget floating — as
of this writing no platform backend assigns it (work in progress); with the
hook unset, the in-canvas fallback is what runs.

XAML:

```xml
<DockManager Background="#232338">
  <DockPane Id="tools" Title="Tools" Side="Left" Size="220" State="Docked">
    <ListView><ListViewItem Content="item 1"/></ListView>
  </DockPane>
  <DockPane Id="props" Title="Properties" Side="Right" Size="200"/>
  <DockPane Title="Output" Side="Bottom" Size="120" State="AutoHidden">
    <TextBlock Text="log..."/>
  </DockPane>
  <DockContent>
    <TextBox Text="document area"/>
  </DockContent>
</DockManager>
```

`<DockManager>` also accepts `NativeFloating="True"` — a declaration of native
pane detach into separate OS windows: `window.Window.Run()` walks the tree and,
unless the app called `EnableDockFloating(dm)` itself, enables it for the first
such manager. An explicit `EnableDockFloating` call wins; with several
`NativeFloating` managers only the first is wired (the host holds one) and the
rest are logged. Headless / unsupported backends ignore the attribute (floating
stays a widget-drawn overlay in the canvas).

`<DockPane>`: `Id` (if omitted, a slug of `Title` is generated), `Title`,
`Side` (Left/Top/Bottom/Right, default Left), `Size` in px (→ `SetSideSize`
for its side — several panes on the same side just share one region, the
last `Size` wins), `State` (Docked by default; AutoHidden calls `Unpin()`
right after adding; Floating/Closed as desired). Content is the pane's first
child widget. `<DockContent>` is not a widget — a marker whose single child
becomes the center (`SetCenter`). Both tags are ignored outside
`<DockManager>`.

See the "Docking" tab in `cmd/showcase` for a working example with three
panes and save/restore-layout buttons.


### Themes as data and the desktop (v3.14.0)

The `theme/` package describes an application's looks with **data** rather than
code, and `desktop/` builds a system taskbar out of that data.

#### Theme profile

A profile is a set of flat token tables: colors, metrics, flags, fonts, icons,
animations, presenters and component styles.

```go
p := theme.NewProfile("mytheme")
p.Parent = theme.ProfileWindows11        // inherit everything, override what differs
p.SetColor("accent", theme.RGB(200, 60, 60)).
    SetMetric("taskbar.height", 44).
    SetFlag("taskbar.centered", true)
p.SetStyle("taskbutton", "", theme.StateHover, theme.StyleDelta{
    Fill: theme.C(theme.RGBA(255, 255, 255, 24)), Corner: theme.N(6),
})

m := theme.NewManager()
theme.RegisterBuiltinProfiles(m)          // Windows 11/10/2000, macOS + dark ones
m.RegisterTheme(p)
m.SetTheme("mytheme")                     // live switch, subscribers notified
```

A style is asked for by (component, part, state); a missing state falls back to
normal, a missing part to the component, a missing component to the theme's
overall look. `GetStyle` returns a pointer into the resolved table and allocates
nothing — it is safe to call from `Draw`.

States are a bitmask, but the table keeps one style per state: `Dominant()`
collapses the mask to the leading one (Disabled > Pressed > Active > Hover >
Focused). Six entries per component instead of thirty-two.

A dark variant declares **only the differences** — usually three or four colors:
style fill and text come from the flat `surface` and `text` tokens unless the
style sets them.

Themes also load from JSON, with no engine changes:

```go
res, err := theme.LoadTheme(file)   // res.Profile, res.Warnings
m.RegisterTheme(res.Profile)
```

#### Glass, shadows and rounded clipping

A theme style can ask for what used to be drawn by hand:

```go
p.SetStyle("taskbar", "", theme.StateNormal, theme.StyleDelta{
    Backdrop:  &theme.BackdropSpec{Mode: theme.BackdropBlur, Radius: 30,
                                   Tint: theme.RGBA(243, 243, 243, 200)},
    Corner:    theme.N(8),
    Elevation: theme.N(12),                       // soft shadow
    Shadow:    theme.C(theme.RGBA(0, 0, 0, 70)),
})
```

Backdrop blur takes the pixels already on the canvas, downscales them 4×, blurs
them with a separable box blur (cost independent of the radius) and puts them
back. `Canvas.SetRoundClip` clips along the rounded outline instead of its
bounding box.

#### The taskbar and its components

```go
bar := desktop.NewTaskbar(m)
bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
bar.AddItem(desktop.SlotApps, desktop.NewApplicationArea(m, catalog, windows))
bar.AddItem(desktop.SlotTray, tray)      // desktop.NewSystemTray(m)
bar.AddItem(desktop.SlotTray, desktop.NewClock(m, desktop.SystemClock{}))
bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
```

Components never touch the system themselves: data arrives through the
`WindowModel`, `AppCatalog`, `SystemStatus`, `Notifications` and `Clock`
interfaces implemented by the consumer. The engine ships fakes for all of them
(`FakeWindowModel`, `StaticAppCatalog`, `FakeSystemStatus`, `FakeNotifications`,
`FakeClock`) — tests and the demo run on those.

Flyouts — Start menu, calendar, quick settings, notification center — are drawn
as engine overlays, so they can be hosted in separate OS windows
(`engine.SetPopupSink`) and are not clipped by the shell window:

```go
menu := desktop.NewStartMenu(m, catalog)
menu.Screen = image.Rect(0, 0, w, h)
startBtn.OnClick = func() { menu.Toggle(startBtn.Bounds()) }
root.AddChild(menu)                       // must be in the tree, or the overlay is not found
```

The taskbar spans the full width, survives `SetBounds` to another resolution and
respects the canvas scale. A component short on space degrades predictably:
window buttons shrink to icons, tray icons hide behind a chevron button.

#### Presenters: a theme changes more than color

Tokens describe palette and geometry, but not shape. The macOS dock is not a
repainted button row: icons are large, centered, and the one under the cursor
grows and pushes its neighbours aside. So a profile may bring a **presenter**
with it — someone else's drawing and layout for a component it knows:

```go
p.Presenters["runningapps"] = "dock"      // in the macOS profile
```

The component stays single, its behaviour tests pass under both themes; only the
one who draws changes. Register your own with
`desktop.RegisterPresenter(name, p)`.

Demo: `go run ./cmd/desktopdemo` — five looks of the very same components,
switched by buttons without a restart.


#### Radial gradient

A linear gradient describes a transition along an axis, and the glow under a
dock icon cannot be expressed that way: there the light spreads out in a circle.

```go
p.SetStyle("dock", "", theme.StateHover, theme.StyleDelta{
    Gradient: []theme.GradientStop{
        {Pos: 0, Color: theme.RGBA(255, 255, 255, 150)},
        {Pos: 1, Color: theme.RGBA(255, 255, 255, 0)},
    },
    GradientKind:   theme.GK(theme.GradientRadial),
    GradientRadius: theme.N(1.1),     // fraction of half the longer side
})
```

Center (`GradientCenterX/Y`) and radius are fractions of the area, not pixels:
the same glow fits both a 24 pt icon and a 64 pt one. A gradient replaces the
fill when set. `widget.DrawRadialGradient` and `widget.DrawLinearGradient` are
available directly too.

#### A theme for a subtree

There used to be exactly one theme per application: `ApplyGlobalTheme` writes to
shared variables and `Engine.SetTheme` walks the whole tree. A remote-desktop
shell needs something else — the guest's window in the guest's theme next to its
own interface.

```go
scope := widget.NewThemeScope(widget.Win2000Theme())
scope.SetBounds(image.Rect(0, 0, 400, 300))
scope.AddChild(button)          // a child is themed by the scope right away
root.AddChild(scope)

eng.SetTheme(widget.DarkTheme()) // the scope stays classic
```

The scope hands its theme to its subtree and shields it from a global change:
`ApplyThemeTree` does not descend into it. Shape — bevels, corners, the classic
flag — is read from a shared variable inside `Draw`, so it is swapped for the
duration of the subtree's drawing and restored via `defer`. Scopes nest: an
inner one restores the OUTER style rather than resetting to the global one.
`NewThemeScope(nil)` is a plain container — a global theme reaches its children.


### The frame pipeline (v3.15.0)

A frame is produced and handed to a consumer; this is what the engine tells
about it and who sets the pace.

#### Subtree culling

A frame used to walk the whole tree: damage clipped at the canvas level, so
stray pixels were discarded — but the walk and the `Draw` calls happened
anyway. Now a branch that touches none of the changed areas is not drawn at
all.

```go
eng.SetSubtreeCulling(false) // back to the full walk
```

Hence the draw contract (see "Custom Widget" → "The draw contract"): `Draw` is
not guaranteed every frame. A widget that draws outside its own bounds declares
the margin:

```go
func (w *MyWidget) DrawMargin() int { return 12 } // shadow, glow
```

#### What a tile is made of

```go
frame := <-eng.Frames()
for i, tile := range frame.Tiles {
    switch frame.Regions[i].Kind {
    case output.RegionSolid: // fill with Regions[i].Color
    case output.RegionText:  // compress losslessly
    case output.RegionImage: // lossy is fine
    case output.RegionMixed: // as before
    }
}
```

The engine knows what it painted each area with; that knowledge used to be lost
on the way out, and the consumer rebuilt it with a second codec pass. The mark
accumulates per tile: an opaque full-tile fill erases whatever was under it,
text or an image over a background becomes the tile's mark, and a fill over
content yields `RegionMixed` — an honest "don't know".

#### Content that moved

```go
for _, m := range frame.Moves { // moves first, then tiles!
    blit(m.Src(), m.Rect)
}
```

Dragging a window does not change pixels, it moves them. `widget.NotifyMove(src, dst)`
declares the move; in RDP that is a pair of surface-cache commands instead of a
hundred kilobytes.

#### Who sets the pace

```go
eng.SetFrameSink(sink)          // the sink gets the frame synchronously and cannot lose it
eng.SetPacing(engine.PacingExternal)
eng.RequestFrame()              // the sink is ready
```

Under external pacing the internal ticker starts no frames (it still advances
animations). This is what vblank pacing on a local output needs — and it also
lets a consumer mutate the scene and produce the frame on one goroutine,
removing that race by construction. `Frames()` keeps working: the sink is an
alternative, not a replacement.

---

## Module Structure

```
go.mod:  module github.com/oops1/headless-gui/v3
  require golang.org/x/image

go.mod:  module github.com/oops1/headless-gui/v3/window
  require github.com/oops1/headless-gui/v3 => ../
  require github.com/ebitengine/purego, golang.org/x/sys
```

Consumer application imports the main module:

```
require github.com/oops1/headless-gui/v3 v0.x.x
```

If native window is needed:

```
require github.com/oops1/headless-gui/v3/window v0.x.x
```

For local development use `replace`:

```
replace github.com/oops1/headless-gui/v3 => ../GuiEngine
replace github.com/oops1/headless-gui/v3/window => ../GuiEngine/window
```

---

## Demo Applications

Run from the root `GuiEngine` directory:

```bash
go run ./cmd/showcase    # all widgets + live animation
go run ./cmd/desktopdemo # desktop: taskbar and live theme switching
go run ./cmd/guiview     # interactive demo with modal XAML windows
go run ./cmd/griddemo    # Grid layout
go run ./cmd/smartgit    # SmartGit-like UI
go run ./cmd/webshowcase # the whole showcase in a browser (http://localhost:8091)
go run ./cmd/webdemo     # minimal streaming example

# Windows binary without console
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```
