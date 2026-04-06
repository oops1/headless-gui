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
| `ToggleSwitch` | ToggleSwitch | `Content`, `IsOn` |
| `ScrollViewer` | ScrollView | `ContentHeight`, `Background` |
| `ListView`, `ListBox` | ListView | `Items`, `SelectedIndex`, `ItemHeight`, child `<ListViewItem>` |
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
| `Separator`, `Line`, `Rectangle` | Separator | `Background` |

Common attributes: `Name`/`x:Name`, `Left`/`Canvas.Left`, `Top`/`Canvas.Top`, `Width`, `Height`, `Grid.Row`, `Grid.Column`, `Grid.RowSpan`, `Grid.ColumnSpan`.

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
```

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
go run ./cmd/guiview     # interactive demo with modal XAML windows
go run ./cmd/griddemo    # Grid layout
go run ./cmd/smartgit    # SmartGit-like UI

# Windows binary without console
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```
