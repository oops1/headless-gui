# AI Agent Reference: headless-gui Framework

**Framework**: `github.com/oops1/headless-gui/v3`  
**Language**: Go  
**Rendering**: Off-screen to RGBA buffer with dirty tile output  
**No CGO**: Pure Go implementation  

---

## Table of Contents

1. [Quick Reference Card](#quick-reference-card)
2. [Widget Constructor Cheatsheet](#widget-constructor-cheatsheet)
3. [Engine API](#engine-api)
4. [Common Patterns](#common-patterns)
5. [XAML Tag Mapping](#xaml-tag-mapping)
6. [Event Callback Signatures](#event-callback-signatures)
7. [Interface Reference](#interface-reference)
8. [Constants and Enums](#constants-and-enums)
9. [Type Hierarchy](#type-hierarchy)
10. [Common Mistakes and Gotchas](#common-mistakes-and-gotchas)

---

## Quick Reference Card

### Module & Imports

```go
module: github.com/oops1/headless-gui/v3
go version: 1.22+

import (
    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
    "github.com/oops1/headless-gui/v3/widget/datagrid"
    "github.com/oops1/headless-gui/v3/widget/treeview"
    "github.com/oops1/headless-gui/v3/output"
    "github.com/oops1/headless-gui/v3/window"
)
```

### Bootstrap

```go
// Create engine (width, height, target FPS)
eng := engine.New(1920, 1080, 20)

// Set root widget (required before Start)
eng.SetRoot(rootWidget)

// Start rendering loop (non-blocking)
eng.Start()

// Consume frames
for frame := range eng.Frames() {
    // Handle frame.Tiles (dirty tile updates)
}

// Stop engine
eng.Stop()
```

---

## Widget Constructor Cheatsheet

All exported constructors in `widget` package. These are the ONLY way to create widgets with correct defaults.

### Basic Widgets

```go
NewButton(text string) *Button
NewWin10AccentButton(text string) *Button
NewLabel(text string, col color.RGBA) *Label
NewWin10Label(text string) *Label
NewTextInput(placeholder string) *TextInput
NewPasswordInput(placeholder string) *TextInput
NewCheckBox(text string) *CheckBox
NewRadioButton(text, group string) *RadioButton
NewToggleSwitch(text string) *ToggleSwitch
```

### Range/Selection Widgets

```go
NewSlider() *Slider                           // range [0.0, 1.0]
NewSliderRange(min, max float64) *Slider
NewProgressBar() *ProgressBar
NewProgressBarColor(fill color.RGBA) *ProgressBar
NewDropdown(items ...string) *Dropdown
NewListView(items ...string) *ListView
NewPopupMenu() *PopupMenu
```

### Container Widgets

```go
NewPanel(bg color.RGBA) *Panel
NewWin10Panel() *Panel
NewStackPanel(orient Orientation) *StackPanel
NewDockPanel() *DockPanel
NewScrollView() *ScrollView
NewGrid() *Grid
NewCanvas() *Canvas
```

### Window/Dialog Widgets

```go
NewWindow(title string, width, height int) *Window
NewDialog(title string, width, height int) *Dialog
NewConfirmDialog(title, message string, onResult func(ok bool)) *Dialog
NewMessageBox(eng ModalShower) *MessageBox
NewModalAdapter(w Widget) *ModalAdapter
NewModalAdapterWithDim(w Widget, dim color.RGBA) *ModalAdapter
```

### Menu/Tab Widgets

```go
NewMenuBar() *MenuBar
NewTabControl(tabs ...TabItem) *TabControl
```

### Specialized Widgets

```go
NewImageWidget() *ImageWidget
NewTreeViewWidget() *TreeViewWidget
NewDataGridWidget() *DataGridWidget
```

### Legacy TreeView

```go
NewTreeNode(text string) *TreeNode
NewTreeView() *TreeViewWidget  // Wrapper; use treeview.New() internally
```

---

## Engine API

### Constructor

```go
func New(width, height, fps int) *Engine
```

Creates engine with virtual canvas size and target frame rate (1–120 fps, default 20).

### Core Methods

```go
// Set root widget (required before Start, safe to call anytime)
func (e *Engine) SetRoot(w widget.Widget)

// Get current root widget
func (e *Engine) Root() widget.Widget

// Start rendering loop (non-blocking, spawns goroutine)
func (e *Engine) Start()

// Stop rendering loop (blocking, waits for completion)
func (e *Engine) Stop()

// Get read-only channel of rendered frames
func (e *Engine) Frames() <-chan output.Frame

// Get canvas dimensions in pixels
func (e *Engine) CanvasSize() (w, h int)
```

### Resolution & Appearance

```go
// Change canvas resolution (call before Start or after Stop)
// Auto-scales background image if set
func (e *Engine) SetResolution(width, height int)

// Load background image from file (PNG or JPEG)
// Automatically scales to canvas size
// Saved internally for rescaling on SetResolution
func (e *Engine) SetBackgroundFile(path string) error

// Set color theme across all widgets
func (e *Engine) SetTheme(t *widget.Theme)

// Register TTF font by name (for use in DrawTextFont)
func (e *Engine) RegisterFont(fontName string, ttfData []byte)

// Register TTF font from file
func (e *Engine) RegisterFontFile(fontName, path string) error

// Set DPI for font rendering (default 96)
// Resets font cache
func (e *Engine) SetDPI(dpi float64)
```

### Frame Output

```go
// Enable frame saving as PNG files
// Call before Start(); blocks on send (ensures all frames saved)
// Stop() waits for all PNG writes to complete
func (e *Engine) SaveFrames(dir string)
```

### Modal Dialogs

```go
// Show modal widget (auto-centers, injects CaptureManager)
func (e *Engine) ShowModal(m widget.ModalWidget)

// Close specific modal (if m==nil, closes top modal)
func (e *Engine) CloseModal(m widget.ModalWidget)
```

### Input Events (called by window backend)

```go
// Send mouse movement
func (e *Engine) SendMouseMove(x, y int)

// Send mouse button event
func (e *Engine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)

// Send keyboard event
func (e *Engine) SendKeyEvent(e widget.KeyEvent)
```

---

## Common Patterns

### 1. Basic Application with Native Window

```go
package main

import (
    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
    "github.com/oops1/headless-gui/v3/window"
)

func main() {
    // Build UI
    panel := widget.NewPanel(color.RGBA{R: 43, G: 43, B: 43, A: 255})
    btn := widget.NewButton("Click Me")
    btn.SetBounds(image.Rect(10, 10, 100, 40))
    btn.OnClick = func() { println("Clicked!") }
    panel.AddChild(btn)
    
    // Create engine
    eng := engine.New(800, 600, 30)
    eng.SetRoot(panel)
    eng.Start()
    
    // Create native window
    win := window.New(eng, "My App")
    win.SetMaxFPS(60)
    win.Run() // blocks
    
    eng.Stop()
}
```

### 2. Loading XAML and Wiring Events

```go
// Create root from XAML
root, err := widget.LoadXAML(xamlBytes)
if err != nil {
    log.Fatal(err)
}

eng := engine.New(1024, 768, 25)
eng.SetRoot(root)
eng.Start()

// Find widget by name and wire event
if btn, ok := widget.FindByName(root, "submitBtn").(*widget.Button); ok {
    btn.OnClick = func() {
        // Handle click
    }
}
```

### 3. Creating a Modal Dialog

```go
// Create dialog
dlg := widget.NewDialog("Settings", 400, 300)
dlg.SetBounds(image.Rect(0, 0, 400, 300))

// Add content
content := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
label := widget.NewWin10Label("Setting 1")
content.AddChild(label)
dlg.AddChild(content)

// Show modal (auto-centers)
eng.ShowModal(dlg)

// Close from callback
okBtn.OnClick = func() {
    eng.CloseModal(dlg)
}
```

### 4. DataGrid with ObservableCollection

```go
import (
    dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Create data source
collection := datagrid.NewObservableCollection()

// Add items
type Person struct {
    Name  string
    Age   int
    Email string
}

for _, p := range people {
    collection.Add(p)
}

// Create DataGrid
grid := widget.NewDataGridWidget()
grid.SetItemsSource(collection)

// Add columns
col1 := &dg.DataGridTextColumn{
    Header:        "Name",
    Binding:       "Name",
    Width:         150,
}
grid.Columns = append(grid.Columns, col1)

// Wire events
grid.OnSelectionChanged = func(e dg.SelectionChangedEvent) {
    if e.SelectedIndex >= 0 {
        item := e.SelectedItem
        println("Selected:", item)
    }
}

grid.OnSorting = func(e *dg.SortingEvent) {
    // Custom sort logic
}

grid.OnCellEditEnding = func(e *dg.CellEditEndingEvent) {
    // Validate/save edited value
}
```

### 5. TreeView with Items

```go
import tv "github.com/oops1/headless-gui/v3/widget/treeview"

// Create TreeView (from treeview subpackage)
tree := tv.New()
tree.SetBounds(image.Rect(0, 0, 500, 400))

// Create root nodes
root1 := &tv.TreeViewItem{
    Text: "Projects",
    Children: []*tv.TreeViewItem{
        {Text: "Project A"},
        {Text: "Project B"},
    },
}

tree.AddRoot(root1)

// Wire events
tree.OnSelectedItemChanged = func(e tv.SelectedItemChangedEvent) {
    if e.SelectedItem != nil {
        println("Selected:", e.SelectedItem.Text)
    }
}

tree.OnExpanded = func(e tv.ExpandedEvent) {
    println("Expanded:", e.Item.Text)
}

tree.OnItemInvoked = func(e tv.ItemInvokedEvent) {
    println("Double-clicked:", e.Item.Text)
}
```

### 6. Registering Custom XAML Widgets

```go
// In init() or early startup
widget.RegisterXAMLWidget("CustomChart", func(attrs widget.XAMLAttrs) (widget.Widget, error) {
    width := attrs.Attr("Width")
    height := attrs.Attr("Height")
    
    chart := createChart(width, height)
    return chart, nil
})

// Now <CustomChart> tags work in XAML
```

### 7. Applying Themes

```go
// Use built-in theme
darkTheme := widget.DefaultDarkTheme()
eng.SetTheme(darkTheme)

// Or light theme
lightTheme := widget.DefaultLightTheme()
eng.SetTheme(lightTheme)

// Or create custom theme
customTheme := &widget.Theme{
    WindowBG:   color.RGBA{R: 20, G: 20, B: 20, A: 255},
    BtnBG:      color.RGBA{R: 50, G: 50, B: 50, A: 255},
    // ... set all fields
}
eng.SetTheme(customTheme)
```

---

## XAML Tag Mapping

Mapping of XAML tags to Go types with key attributes.

| XAML Tag | Go Type | Key Attributes |
|----------|---------|-----------------|
| `<Button>` | `Button` | `Content`, `Width`, `Height`, `Background`, `Foreground` |
| `<Label>` | `Label` | `Content`, `Foreground`, `Background`, `FontSize` |
| `<TextBox>` | `TextInput` | `Text`, `PlaceholderText`, `Width`, `Height`, `AcceptsReturn`, `MaxLength` |
| `<PasswordBox>` | `TextInput` | (created with NewPasswordInput) |
| `<NumericUpDown>` (`<IntegerUpDown>`/`<DoubleUpDown>`) | `NumericUpDown` | `Minimum`, `Maximum`, `Increment`, `Decimals`, `Value` |
| `<VirtualizingItemsControl>` | `VirtualizingItemsControl` | `ItemHeight`, `Buffer`, `ItemsSource`, `ItemTemplate` |
| `<CheckBox>` | `CheckBox` | `Content`, `IsChecked`, `Foreground` |
| `<RadioButton>` | `RadioButton` | `Content`, `GroupName`, `IsChecked` |
| `<ToggleButton>` | `ToggleSwitch` | `Content`, `IsChecked` |
| `<Slider>` | `Slider` | `Minimum`, `Maximum`, `Value`, `Width` |
| `<ProgressBar>` | `ProgressBar` | `Value`, `Minimum`, `Maximum`, `Foreground` |
| `<ComboBox>` | `Dropdown` | `Items`, `SelectedIndex`, `SelectedValue` |
| `<ListBox>` | `ListView` | `Items`, `SelectedIndex`, `ItemHeight` |
| `<Image>` | `ImageWidget` | `Source` (file path) |
| `<Panel>` | `Panel` | `Background`, `Children` |
| `<StackPanel>` | `StackPanel` | `Orientation` (Horizontal/Vertical), `Spacing`, `Padding` |
| `<DockPanel>` | `DockPanel` | `Children`, `DockPanel.Dock` (attached property) |
| `<Grid>` | `Grid` | `RowDefinitions`, `ColumnDefinitions`, `Children` |
| `<Canvas>` | `Canvas` | `Width`, `Height`, `Canvas.Left`, `Canvas.Top` |
| `<ScrollViewer>` | `ScrollView` | `Content`, `Height` |
| `<TabControl>` | `TabControl` | `Items` (TabItem elements) |
| `<Window>` | `Window` | `Title`, `Width`, `Height`, `WindowStyle`, `ResizeMode` |
| `<MenuItem>` | (nested in MenuBar) | `Header`, `Items` |
| `<TreeView>` | `TreeViewWidget` | `Items`, `ItemHeight`, `ShowIndentGuides` |
| `<DataGrid>` | `DataGridWidget` | `ItemsSource`, `Columns` |

### XAML Color Values

Supported color formats:
- Named colors: `"white"`, `"black"`, `"red"`, `"green"`, `"blue"`, `"gray"`, `"transparent"`
- Hex RGB: `"#RRGGBB"` (e.g., `"#FF0000"`)
- Hex RGBA: `"#RRGGBBAA"` (e.g., `"#FF0000FF"`)

### XAML Margins

Margin syntax (WPF-compatible):
- Single value: `"10"` → all sides 10px
- Two values: `"5,10"` → horizontal 5px, vertical 10px
- Four values: `"1,2,3,4"` → left, top, right, bottom

---

## Event Callback Signatures

All event callbacks are executed in goroutines (non-blocking). Modify UI state safely (callbacks already hold necessary locks).

### Input Events

```go
// Button click (fires on RELEASE, not press)
Button.OnClick func()

// Multiple click subscribers (back-compat with OnClick).
// Returns id usable with RemoveClickHandler. Handlers fire in
// registration order, AFTER OnClick (the field).
id := Button.AddClickHandler(func() { ... })
Button.RemoveClickHandler(id)
Button.ClearClickHandlers()

// TextInput when Enter pressed (if AcceptsReturn=false)
TextInput.OnEnter func()

// TextInput on any text change
TextInput.OnChange func(text string)
```

### DataGrid row activation (NEW: A3 fix)

```go
// Fires on double-click OR Enter, regardless of IsReadOnly.
// Useful for read-only grids: open details, toggle breakpoint, etc.
dg.OnRowActivated = func(row int, item interface{}) { ... }
```

### Per-column IsReadOnly tri-state (NEW: A4 fix)

```go
col.SetReadOnly(true)        // explicit RO — overrides grid.IsReadOnly=false
col.SetReadOnly(false)       // explicit editable — overrides grid.IsReadOnly=true
col.ResetReadOnly()          // back to inheriting grid.IsReadOnly
col.IsReadOnlyExplicit()     // was IsReadOnly set explicitly?
```

XAML: `<DataGridTextColumn IsReadOnly="False" />` now overrides
`<DataGrid IsReadOnly="True">`. If the column omits `IsReadOnly`, it
inherits the grid value.

### ListView live-tail (NEW: A6 fix)

```go
lv.AutoScrollToBottom = true       // SetItems / AddItem keep scroll at bottom
                                    // if user was already at bottom
lv.PreserveScrollOnSetItems = true // keep current scrollY across SetItems
lv.ScrollToBottom()                 // force jump to end
lv.ScrollToTop()
```

### Grid Star=0 collapse (NEW: A1 fix)

```go
g.ColDefs = []widget.GridDefinition{
    {Mode: widget.GridSizeStar, Value: 0}, // collapsed (0px), not "1*"
    {Mode: widget.GridSizeStar, Value: 1},
}
```
XAML: `<ColumnDefinition Width="0*"/>` works as expected; column gets
0 px and is excluded from the star-distribution.

### Engine.SetRoot bounds preservation (NEW: A9 fix)

```go
eng.SetRoot(root)            // if root.Bounds is non-empty, KEEP it
eng.SetRootFullCanvas(root)  // legacy: always stretch to canvas
```

### Selection Events

```go
// CheckBox state change
CheckBox.OnChange func(checked bool)

// RadioButton state change
RadioButton.OnChange func(checked bool)

// ToggleSwitch state change
ToggleSwitch.OnChange func(on bool)

// Slider value change
Slider.OnChange func(value float64)

// Dropdown selection change
Dropdown.OnChange func(idx int, text string)

// ListView item selection
ListView.OnSelect func(index int, text string)

// PopupMenu item selection
PopupMenu.OnSelect func(index int, text string)

// TabControl tab change
TabControl.OnTabChange func(index int, header string)
```

### Menu Events

```go
// MenuBar submenu selection
// topIdx = top-level menu index, subIdx = submenu item index
MenuBar.OnSelect func(topIdx int, subIdx int, text string)
```

### Modal Events

```go
// Dialog: use engine.ShowModal() / engine.CloseModal()
// No direct OnClose callback (close via button handler)

// Panel.OnClose (fires on PRESS of close button in title bar)
Panel.OnClose func()

// Window.OnClose (fires on PRESS of close button in title bar)
Window.OnClose func()
```

### DataGrid Events

```go
// Selection changed
DataGrid.OnSelectionChanged func(e datagrid.SelectionChangedEvent)
// SelectionChangedEvent.SelectedIndex (int)
// SelectionChangedEvent.SelectedItem (interface{})

// Column sort requested
DataGrid.OnSorting func(e *datagrid.SortingEvent)
// SortingEvent.ColumnIndex (int)
// SortingEvent.Column (*datagrid.DataGridTextColumn)

// Cell edit completed
DataGrid.OnCellEditEnding func(e *datagrid.CellEditEndingEvent)
// CellEditEndingEvent.RowIndex, ColumnIndex (int)
// CellEditEndingEvent.NewValue (string)

// Row edit completed
DataGrid.OnRowEditEnding func(rowIndex int, item interface{})
```

### TreeView Events

```go
// Selected item changed
TreeView.OnSelectedItemChanged func(e treeview.SelectedItemChangedEvent)
// SelectedItemChangedEvent.SelectedItem (*TreeViewItem)

// Node expanded
TreeView.OnExpanded func(e treeview.ExpandedEvent)
// ExpandedEvent.Item (*TreeViewItem)

// Node collapsed
TreeView.OnCollapsed func(e treeview.CollapsedEvent)
// CollapsedEvent.Item (*TreeViewItem)

// Node double-clicked
TreeView.OnItemInvoked func(e treeview.ItemInvokedEvent)
// ItemInvokedEvent.Item (*TreeViewItem)

// Legacy: simple callback (for compatibility)
TreeView.OnSelect func(item *TreeViewItem)
```

---

## Interface Reference

### Core Widget Interface

```go
type Widget interface {
    // Draw renders widget and children to context
    Draw(ctx DrawContext)
    
    // Bounds returns bounding rectangle in absolute canvas coordinates
    Bounds() image.Rectangle
    
    // SetBounds sets position and size in absolute coordinates
    SetBounds(r image.Rectangle)
    
    // Children returns slice of child widgets
    Children() []Widget
    
    // AddChild appends child to children slice
    AddChild(w Widget)
}
```

### Draw Context (Rendering API)

```go
type DrawContext interface {
    // ── Primitives ──
    FillRect(x, y, w, h int, col color.RGBA)
    FillRectAlpha(x, y, w, h int, col color.RGBA)  // with alpha blend
    FillRoundRect(x, y, w, h, r int, col color.RGBA)
    DrawBorder(x, y, w, h int, col color.RGBA)  // 1px outline
    DrawRoundBorder(x, y, w, h, r int, col color.RGBA)
    SetPixel(x, y int, col color.RGBA)
    DrawHLine(x, y, length int, col color.RGBA)
    DrawVLine(x, y, length int, col color.RGBA)
    
    // ── Images ──
    DrawImage(src image.Image, x, y int)
    DrawImageScaled(src image.Image, x, y, w, h int)
    
    // ── Text ──
    DrawText(text string, x, y int, col color.RGBA)  // default font, DefaultFontSizePt
    DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA)
    DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA)
    MeasureText(text string, sizePt float64) int  // width in pixels
    MeasureTextFont(text string, sizePt float64, fontName string) int
    MeasureRunePositions(text string, sizePt float64) []int  // per-char widths
    
    // ── Clipping ──
    SetClip(r image.Rectangle)
    ClearClip()
}
```

### Input Interfaces

```go
type MouseClickHandler interface {
    OnMouseButton(e MouseEvent) bool  // returns true if consumed
}

type MouseMoveHandler interface {
    OnMouseMove(x, y int)
}

type KeyHandler interface {
    OnKeyEvent(e KeyEvent)
}

type Focusable interface {
    SetFocused(focused bool)
    IsFocused() bool
}

type CaptureRequester interface {
    WantsCapture(e MouseEvent) bool  // called at mouse press
}

type CaptureAware interface {
    SetCaptureManager(cm CaptureManager)
}

type CaptureManager interface {
    SetCapture(w Widget)
    ReleaseCapture()
}
```

### Overlay Drawing

```go
type OverlayDrawer interface {
    HasOverlay() bool
    DrawOverlay(ctx DrawContext)  // called after main tree draw
}
```

### Theme Support

```go
type Themeable interface {
    ApplyTheme(t *Theme)
}
```

### Modal Support

```go
type ModalWidget interface {
    Widget
    IsModal() bool
    DimColor() color.RGBA  // overlay color
}

type ModalShower interface {
    ShowModal(m ModalWidget)
    CloseModal(m ModalWidget)
}
```

---

## Constants and Enums

### Mouse Buttons

```go
const (
    MouseLeft       MouseButton = 0
    MouseRight      MouseButton = 1
    MouseMiddle     MouseButton = 2
    MouseWheelUp    MouseButton = 3
    MouseWheelDown  MouseButton = 4
)
```

### Keyboard Keys

```go
const (
    KeyUnknown   KeyCode = 0
    KeyBackspace KeyCode = 8
    KeyTab       KeyCode = 9
    KeyEnter     KeyCode = 13
    KeyEscape    KeyCode = 27
    KeySpace     KeyCode = 32
    KeyHome      KeyCode = 36
    KeyLeft      KeyCode = 37
    KeyUp        KeyCode = 38
    KeyRight     KeyCode = 39
    KeyDown      KeyCode = 40
    KeyDelete    KeyCode = 46
    KeyEnd       KeyCode = 35
    KeyA...KeyZ  KeyCode = 65...90
)
```

### Keyboard Modifiers

```go
const (
    ModNone  KeyMod = 0
    ModShift KeyMod = 1 << 0
    ModCtrl  KeyMod = 1 << 1
    ModAlt   KeyMod = 1 << 2
    ModMeta  KeyMod = 1 << 3
)
```

### Layout Orientation

```go
const (
    OrientationVertical   Orientation = iota      // top to bottom
    OrientationHorizontal Orientation = iota + 1  // left to right
)
```

### Window Styles

```go
const (
    WindowStyleSingleBorder WindowStyle = iota
    WindowStyleNone
    WindowStyleToolWindow
)
```

### Window Title Styles

```go
const (
    WindowTitleAuto WindowTitleStyle = iota  // auto-detect by OS
    WindowTitleWin                           // Windows-style (text left, buttons right)
    WindowTitleMac                           // macOS-style (lights left, text center)
)
```

### Window Resize Modes

```go
const (
    ResizeModeCanResize  ResizeMode = iota   // full resize + minimize
    ResizeModeNoResize                       // fixed size
    ResizeModeCanMinimize                    // minimize only
)
```

### Icon Positions

```go
const (
    IconLeft   IconPosition = iota
    IconTop
    IconOnly
)
```

### Dock Positions

```go
const (
    DockLeft   DockPosition = iota
    DockTop
    DockRight
    DockBottom
    DockFill
)
```

### Output

```go
const (
    TileSize = 64  // dirty tile is 64×64 pixels
)
```

---

## Type Hierarchy

### Base Type

All widgets embed `Base` which provides:
- Bounds storage and caching
- Children slice management
- Margin support
- Visibility flag
- Enabled flag

```go
type Base struct {
    bounds   image.Rectangle
    children []Widget
    visible  bool
    enabled  bool
    // ... internal fields
}
```

### Container Widgets (embed Base)

- `Panel` — simple container, no layout
- `StackPanel` — auto-layout horizontal/vertical
- `DockPanel` — auto-layout with attached Dock property
- `Grid` — row/column-based auto-layout
- `Canvas` — fixed positioning (no auto-layout)
- `ScrollView` — scrollable container
- `TabControl` — tabbed container
- `Window` — OS-level window container
- `Dialog` — modal window container

### Control Widgets (embed Base)

- `Button` — clickable button
- `Label` — text display
- `TextInput` — text editing field
- `CheckBox` — toggle checkbox
- `RadioButton` — radio button (group-based)
- `ToggleSwitch` — on/off toggle
- `Slider` — range slider
- `ProgressBar` — progress indicator
- `Dropdown` — dropdown list
- `ListView` — list selection
- `PopupMenu` — context menu
- `ImageWidget` — image display
- `MenuBar` — menu bar

### Specialized Widgets

- `TreeViewWidget` — wrapper around `treeview.TreeView`
- `DataGridWidget` — wrapper around `datagrid.DataGrid`
- `ModalAdapter` — makes any widget modal
- `MessageBox` — system message box

### Subpackage Types

#### `widget/datagrid`

- `DataGrid` — high-performance table
- `DataGridTextColumn` — text column definition
- `DataGridTemplateColumn` — custom column
- `ObservableCollection` — bindable data source
- `SelectionChangedEvent`
- `SortingEvent`
- `CellEditEndingEvent`

#### `widget/treeview`

- `TreeView` — hierarchical list
- `TreeViewItem` — node in tree
- `HierarchicalDataTemplate` — item template
- `SelectedItemChangedEvent`
- `ExpandedEvent`
- `CollapsedEvent`
- `ItemInvokedEvent`

#### `output`

- `Frame` — rendered frame with dirty tiles
- `DirtyTile` — one changed 64×64 block

#### `window`

- `Window` — native OS window wrapper
- `NativeWindow` — platform-specific implementation

---

## Common Mistakes and Gotchas

### SetBounds is Required for Manual Layout

When using `Panel` (no auto-layout), you **must** manually set bounds on children:

```go
// WRONG: child bounds never set
panel.AddChild(btn)

// RIGHT: set bounds explicitly
btn.SetBounds(image.Rect(10, 10, 100, 40))
panel.AddChild(btn)
```

Auto-layout containers (`StackPanel`, `Grid`, `DockPanel`) handle bounds for you—set parent bounds only.

### ProgressBar.SetValue is Thread-Safe

`ProgressBar.SetValue()` uses atomic operations and is safe to call from any goroutine:

```go
// Safe to call from background goroutine
go func() {
    for i := 0; i <= 100; i++ {
        progressBar.SetValue(float64(i) / 100.0)
        time.Sleep(100 * time.Millisecond)
    }
}()
```

**Most other widget methods are NOT thread-safe**—modify UI state only from the rendering goroutine or use proper synchronization.

### Label.SetText is Thread-Safe

`Label.SetText()` uses a mutex and is safe from any goroutine:

```go
// Safe
go func() {
    label.SetText("Status: Ready")
}()
```

### RadioButton Groups are Global by GroupName

RadioButton groups are identified by `GroupName` string (global within engine):

```go
rb1 := widget.NewRadioButton("Option 1", "myGroup")
rb2 := widget.NewRadioButton("Option 2", "myGroup")
rb3 := widget.NewRadioButton("Option 3", "otherGroup")

// rb1 and rb2 are in same group; rb3 is separate
// Selecting rb1 auto-deselects rb2
```

### OnClick Fires on MOUSE RELEASE, Not PRESS

Button click callbacks fire when mouse button is **released** over the button, not on press:

```go
btn.OnClick = func() {
    // This fires on RELEASE
    println("Click completed")
}
```

This allows canceling clicks by dragging away before release.

### Window.OnClose and Panel.OnClose Fire on MOUSE PRESS

Unlike button clicks, close button events fire on **press**:

```go
panel.OnClose = func() {
    // Fires immediately when user presses close button
    // Panel is still visible; you must close it explicitly
    eng.CloseModal(panel)
}
```

### DrawContext is Only Valid Inside Draw()

You **cannot** cache or use `DrawContext` outside the `Draw()` call:

```go
// WRONG
ctx := lastDrawContext  // saved from Draw call
ctx.FillRect(...)  // crashes or draws wrong

// RIGHT: only use during Draw
func (w *MyWidget) Draw(ctx DrawContext) {
    ctx.FillRect(...)
}
```

### DataGrid Uses Subpackage `widget/datagrid`

DataGrid is in a subpackage and requires separate import:

```go
import dg "github.com/oops1/headless-gui/v3/widget/datagrid"

grid := widget.NewDataGridWidget()
grid.SetItemsSource(dg.NewObservableCollection())
grid.Columns = append(grid.Columns, &dg.DataGridTextColumn{...})
```

### TreeView Uses Subpackage `widget/treeview`

TreeView is in a subpackage:

```go
import tv "github.com/oops1/headless-gui/v3/widget/treeview"

tree := tv.New()  // Don't use widget.NewTreeView directly
tree.AddRoot(&tv.TreeViewItem{Text: "Root"})
```

### ObservableCollection Shared Between DataGrid and TreeView

Both `DataGrid` and `TreeView` use `datagrid.ObservableCollection` as their data source:

```go
import dg "github.com/oops1/headless-gui/v3/widget/datagrid"

// For TreeView
collection := dg.NewObservableCollection()
tree.SetItemsSource(collection)

// For DataGrid
grid.SetItemsSource(collection)
```

### Callback Execution Model (sync vs goroutine)

The model differs by widget. **As of GUI_ISSUES A5/A7 fix, Button is fully synchronous on both mouse and keyboard paths.** Older callbacks
on other widgets may still spawn a goroutine on the keyboard path; this is being unified.

| Widget | Mouse path | Keyboard path | Notes |
|---|---|---|---|
| `Button.OnClick` | sync | sync | Use `AddClickHandler(fn)` for multiple subscribers; OnClick (field) fires first, then handlers in registration order. |
| `CheckBox.OnChange(checked bool)` | sync | goroutine (Space) | The field is `OnChange`, **not** `OnClick`. Tracks tri-state press → release. |
| `ListView.OnSelect` | goroutine | goroutine | Long-running work OK. |
| `DataGrid.OnRowActivated(row, item)` | sync (after Unlock) | sync | NEW. Fires on dbl-click and Enter, even if grid is read-only. Use for "open detail / toggle breakpoint" UX. |
| `DataGrid.OnSelectionChanged` | goroutine | goroutine | |

Treat the callback as potentially concurrent — guard shared state with a mutex.
For Button specifically you can rely on synchronous semantics:

```go
btn.OnClick = func() { /* runs in caller goroutine */ }
btn.AddClickHandler(func() { /* runs after OnClick, same goroutine */ })
```

### SetRoot Must Be Called Before Start

You **must** call `SetRoot()` before `Start()`:

```go
eng := engine.New(800, 600, 20)
eng.SetRoot(rootWidget)  // Required!
eng.Start()
```

However, you **can** call `SetRoot()` while engine is running to replace the UI tree.

### XAML LoadXAML Returns interface{}, Not *Panel

`widget.LoadXAML()` returns `interface{}` which could be any widget type:

```go
root, err := widget.LoadXAML(xmlBytes)
if err != nil {
    log.Fatal(err)
}

// Type-assert if you know the root type
if panel, ok := root.(*widget.Panel); ok {
    // ...
}
```

### Margin vs Padding

- **Margin**: external space (outside border) — WPF Margin
- **Padding**: internal space (inside border) — used by StackPanel, some containers

Button and Label support `Padding` for internal text spacing.

### Bounds are Absolute, Not Relative

All `Bounds()` and `SetBounds()` use **absolute canvas coordinates**, not relative:

```go
// Absolute coordinates in canvas space
btn.SetBounds(image.Rect(100, 50, 200, 80))  // x1, y1, x2, y2

// NOT relative to parent
// NOT (x, y, width, height)
```

### Frame Output Contains Only Changed Tiles

The `Frame.Tiles` slice contains **only dirty (changed) tiles**:

```go
for frame := range eng.Frames() {
    // frame.Tiles may be empty if nothing changed
    if len(frame.Tiles) == 0 {
        continue
    }
    
    for _, tile := range frame.Tiles {
        // tile.X, tile.Y: top-left in canvas
        // tile.W, tile.H: actual size (≤ 64)
        // tile.Data: RGBA bytes, W*H*4 bytes
    }
}
```

### SetBounds May Trigger Layout Recalculation

When you call `SetBounds()` on containers, they may recalculate child layout:

```go
sp := widget.NewStackPanel(widget.OrientationVertical)
btn1 := widget.NewButton("A")
btn2 := widget.NewButton("B")
sp.AddChild(btn1)
sp.AddChild(btn2)

// SetBounds triggers layout calculation
sp.SetBounds(image.Rect(0, 0, 200, 100))
// btn1 and btn2 positions are now auto-calculated
```

Avoid modifying child bounds after adding to auto-layout containers.

---

## Summary Table: Quick Widget API

| Widget | Constructor | Key Fields | Key Methods | Events |
|--------|-------------|-----------|------------|--------|
| Button | `NewButton(text)` | Text, OnClick | SetPressed, IsPressed | OnClick |
| Label | `NewLabel(text, col)` | Text, TextColor | SetText | - |
| TextInput | `NewTextInput(ph)` | Text, OnChange | SetText, Text | OnChange, OnEnter |
| CheckBox | `NewCheckBox(text)` | Checked, OnChange | SetChecked | OnChange |
| RadioButton | `NewRadioButton(text, group)` | Selected | SetSelected | OnChange |
| Slider | `NewSlider()` | Value, Min, Max | SetValue, Value | OnChange |
| NumericUpDown | `NewNumericUpDown()` | Min, Max, Step, Decimals | SetValue, Value | OnChange |
| VirtualizingItemsControl | `NewVirtualizingItemsControl()` | ItemHeight, Buffer | SetItems, SetItemBuilder, BindCollectionView | - |
| Dropdown | `NewDropdown(items...)` | Items, Selected | SetSelected, Items | OnChange |
| ListView | `NewListView(items...)` | Items, Selected | SetSelected, Items | OnSelect |
| Panel | `NewPanel(bg)` | Background | AddChild | OnClose |
| StackPanel | `NewStackPanel(orient)` | Orientation, Spacing | AddChild | - |
| Dialog | `NewDialog(title, w, h)` | Title, DimColor | AddChild, SetBounds | - |
| Window | `NewWindow(title, w, h)` | Title, Style | AddChild, SetBounds | OnClose |
| TabControl | `NewTabControl()` | Tabs | SetActive, AddTab | OnTabChange |
| DataGrid | `NewDataGridWidget()` | Columns, ItemsSource | SetItemsSource | OnSelectionChanged, OnSorting, OnCellEditEnding |
| TreeView | `treeview.New()` | ItemHeight, ShowIndentGuides | AddRoot, AddChild | OnSelectedItemChanged, OnExpanded, OnItemInvoked |

---

## Recent Engine Additions (Tooltips, Visibility, Locale, Fonts, Layout)

This section documents features added on top of the base API. They are
WPF-compatible where WPF defines equivalent behavior, with documented
headless-only extensions.

### ToolTips (all widgets)

Every widget embeds `Base`, which now carries a `ToolTip string` field plus
`GetToolTip()/SetToolTip(s)`. The engine renders a tooltip box near the cursor
after it rests over a widget with a non-empty tooltip.

```go
btn := widget.NewButton("Save")
btn.ToolTip = "Save the current document"   // or btn.SetToolTip("...")
```

XAML (works on ANY element, not just Button):

```xml
<Button Content="Save" ToolTip="Save the current document"/>
<TextBox ToolTip="Enter your name"/>
```

Engine control (global, toggleable):

```go
eng.SetTooltipsEnabled(true)               // default true
eng.SetTooltipDelay(600 * time.Millisecond) // hover delay before showing
```

Tooltips are detected via the hit-test path (deepest widget wins), respect
widget visibility, and draw above modal dialogs.

### Visibility (Show/Hide) — BUG-5

`Base` exposes WPF-style visibility:

```go
w.SetVisible(false)  // hide: not drawn, excluded from hit-test (≈ Visibility="Collapsed")
w.SetVisible(true)   // show (default)
w.IsVisible()        // current state
```

XAML:

```xml
<Button Content="Hidden" Visibility="Collapsed"/>   <!-- or "Hidden" -->
<Button Content="Shown"  Visibility="Visible"/>
```

Helper for custom containers: `widget.IsWidgetVisible(child) bool`.

### Locale Indicator (windows & dialogs) — toggleable

A global current-locale label (e.g. "EN", "RU") is shown as a small badge in
the title bar of `Window`, `Dialog`, and header-`Panel`. It is a toggleable
property per widget.

```go
widget.SetLocale("RU")          // global; thread-safe; normalized to upper-case
widget.Locale()                  // "RU"

win.ShowLocaleIndicator = false  // disable badge on this window
dlg.ShowLocaleIndicator = true   // Dialog default true (MessageBox uses Dialog)
panel.ShowLocaleIndicator = true
```

XAML: `<Window ... ShowLocaleIndicator="False">`. Applies to `Window`,
`Dialog`, `Panel`.

**OS keyboard-layout sync + context menu.** In windowed mode the `window`
package binds the badge to the OS keyboard layout:

- The badge shows the **current OS input language** and updates live when the
  user switches layout with the **system hotkey** (e.g. Alt+Shift / Win+Space on
  Windows) — a poller reflects it. (Windows: full live sync. Linux: switching via
  the in-app menu works through `setxkbmap`; live following of the system hotkey
  is limited without XKB bindings. macOS: app-driven only.)
- **Right-click the title bar** (or click the badge) opens a **context menu**
  listing the OS-installed layouts; picking one switches the OS layout AND the
  badge. The list is `widget.AvailableLocales()`.

Programmatic API:

```go
widget.SetLocale("RU")              // reflect a locale (no OS switch) — headless source of truth
widget.RequestLocale("RU")          // user intent: switch OS layout (if applier set) + reflect
widget.SetAvailableLocales([]string{"EN","RU","DE"}) // menu list (window pkg fills from OS)
widget.AvailableLocales()           // current list
widget.AddLocaleListener(func(code string){ /* re-translate UI, etc. */ })
widget.SetLocaleApplier(func(code string) bool { /* custom OS switch */ return true })
```

In **headless** mode there is no native window, so `SetLocale`/`RequestLocale`
are the source of truth and `AddLocaleListener` lets the app react (switch
translated strings) when the locale changes. The context menu still works
(it is rendered by the engine and driven by mouse events).

### Localized string resources (`{Loc Key}`) — backward-compatible

A string-table mechanism for UI translation. **Opt-in and fully
backward-compatible**: plain text in XAML/code is never touched — only attributes
written as `{Loc Key}` (or `widget.Tr` calls) are translated. If no table is
registered, `Tr` returns the key itself, so adding `{Loc ...}` never crashes.

> **UI language ≠ keyboard layout.** Two independent axes:
> - `widget.SetLanguage("RU")` / `Language()` / `AddLanguageListener` — the
>   language the **UI is displayed in** (drives `Tr` and `{Loc}`).
> - `widget.SetLocale(...)` / `Locale()` — the **keyboard input layout** shown by
>   the locale badge (and switched via the badge menu / OS poller).
>
> An app can be in Russian while the user types English or Chinese — changing one
> never changes the other. `{Loc}`/`Tr` follow **Language**, not Locale.

```go
// Register tables (merge-on-register). Language codes are upper-cased.
widget.RegisterStrings("EN", map[string]string{"Greeting": "Hello", "Save": "Save"})
widget.RegisterStrings("RU", map[string]string{"Greeting": "Привет", "Save": "Сохранить"})
widget.SetFallbackLanguage("EN")         // used when current language lacks the key (default "EN")

widget.SetLanguage("RU")                  // UI language (NOT keyboard layout)
widget.Tr("Greeting")                    // "Привет"
widget.Tr("Missing")                     // "Missing"  (key returned as-is)
widget.Trf("Count", 5)                   // printf-style: table "Count"="Items: %d" → "Items: 5"
widget.TrIn("DE", "Greeting")            // explicit language lookup
widget.Translation("RU", "Save")         // (value, ok) without key-fallback

// Load from JSON ({"key":"value"}):
widget.LoadStringsJSON("RU", jsonBytes)
widget.LoadStringsFile("RU", "i18n/ru.json")
widget.LoadStringsDir("i18n")            // every <locale>.json in the dir (ru.json → "RU")
```

XAML markup — resolved at load **and re-applied automatically on locale change**:

```xml
<TextBlock Text="{Loc Greeting}"/>       <!-- updates live when SetLocale is called -->
<Button    Content="{Loc Save}"/>
<TextBox   ToolTip="{Loc NameHint}"/>    <!-- works on any string attribute -->
```

Lookup chain for `Tr(key)`: current language table → fallback language table →
the key itself. Lifecycle: the XAML loader collects every `{Loc}` attribute,
applies the initial translation, and subscribes to language changes via
`AddLanguageListener`, so switching `widget.SetLanguage(...)` re-translates the
whole tree (verified with `TextBlock`/`Button`). Inside a `DataTemplate` the
value is resolved once per row at build time. `widget.ClearStrings()` resets all
tables (useful in tests). (`SetFallbackLocale`/`FallbackLocale` remain as
deprecated aliases of the `*Language` functions.)

### Font Glyph Fallback — BUG-2

Missing glyphs (✓ ✗ ⚠, box-drawing, arrows ▲ ▼ →) no longer render as tofu.
The engine auto-loads system fonts with wide coverage as a fallback chain
(Segoe UI Symbol/Arial on Windows, DejaVu/Noto on Linux, Apple Symbols/Arial
Unicode on macOS). You can register more:

```go
eng.RegisterFallbackFont(ttfBytes)        // append to fallback chain
eng.RegisterFallbackFontFile("symbols.ttf")
```

Per-rune resolution: primary font → fallbacks in order → primary (.notdef).
`MeasureText`/`MeasureRunePositions` are fallback-aware so layout stays correct.
Note: color/emoji fonts (e.g. Segoe UI Emoji) rasterize as outline glyphs only.

### TabControl Runtime API — BUG-4

```go
tc.SetTabHeader(i, "CARRY (3)")  // change header at runtime (badges/counters)
tc.TabHeader(i)                   // read header
tc.TabContent(i)                  // get content widget
tc.SetTabVisible(i, false)        // hide a tab from the strip (auto-switches active)
tc.IsTabVisible(i)
tc.RemoveTab(i)                   // remove a tab
tc.ClearTabs()                   // remove all
```

`TabItem` gained a `Hidden bool` field.

### TabControl honors Grid.Row/Column — BUG-1

`<TabControl>` (and all container builders: Grid, Canvas, StackPanel, DockPanel,
Border, ToolBar, StatusBar, MenuBar, TreeView, PopupMenu) now apply the full set
of attached properties (Grid.Row/Column/Span, DockPanel.Dock, Margin, Alignment,
ToolTip, Visibility) via a shared `applyCommonProps`. Previously container
builders skipped Grid.Row, so a `<TabControl Grid.Row="1">` drew from the top of
the window. Fixed — no Canvas wrapper workaround needed.

### DataGrid Conditional Row Coloring — BUG-3

```go
grid := widget.NewDataGridWidget()
grid.Grid.RowStyleSelector = func(item interface{}, rowIndex int) (color.RGBA, bool) {
    o := item.(Order)
    if o.Side == "BUY" {
        return color.RGBA{R: 20, G: 60, B: 20, A: 255}, true // green row
    }
    return color.RGBA{R: 70, G: 25, B: 25, A: 255}, true     // red row
}
```

Returns `(bg, true)` to paint the row background (overrides
AlternatingRowBackground); selection/hover render on top. Return `(_, false)`
to fall back to default striping.

### Adaptive Layout: Canvas Stretch + Resize — BUG-6

A Canvas child with an EXPLICIT `HorizontalAlignment="Stretch"` /
`VerticalAlignment="Stretch"` fills the Canvas along that axis (respecting
Canvas.Left/Top/Right/Bottom and Margin as insets). This is a headless-gui
extension to WPF Canvas. Default (unset) alignment keeps fixed Width/Height, so
existing layouts are unaffected.

```xml
<Canvas Width="1500" Height="800">
  <DataGrid Canvas.Left="10" Canvas.Top="10" Canvas.Bottom="10"
            HorizontalAlignment="Stretch"/>  <!-- fills width on window resize -->
</Canvas>
```

Existing both-anchor stretch still works: setting both `Canvas.Left` and
`Canvas.Right` (or `Top`+`Bottom`) computes the size from the Canvas dimensions.
On window resize, `Window.SetBounds → Canvas.SetBounds → layout` re-applies
stretch automatically.

---

## WPF Resources, Styles & Binding (P0)

The XAML loader now supports the WPF resource/style/binding foundation via a
pre-processing pass over the parsed tree (no behavior change for files that
don't use these features).

### Resources & `{StaticResource}` / `{DynamicResource}`

```xml
<Window.Resources>            <!-- or <Grid.Resources>, <ResourceDictionary> -->
  <SolidColorBrush x:Key="Accent" Color="#FF8800"/>
  <Color x:Key="Bg">#202830</Color>
  <sys:Double x:Key="Big">18</sys:Double>
</Window.Resources>
...
<StackPanel Background="{StaticResource Bg}">
  <Button Background="{StaticResource Accent}" FontSize="{StaticResource Big}"/>
</StackPanel>
```

Scalar resources (brushes/colors/strings/numbers) are resolved into attribute
values. `{DynamicResource}` is treated as static (no live swap yet).
`{x:Null}` resolves to empty.

### Styles (`<Style>` / `<Setter>`)

```xml
<Window.Resources>
  <Style x:Key="Card" TargetType="Button">
    <Setter Property="Background" Value="{StaticResource Accent}"/>
    <Setter Property="Foreground" Value="#FFFFFF"/>
    <Setter Property="Height" Value="40"/>
  </Style>
  <Style TargetType="TextBlock">           <!-- implicit: applies to ALL TextBlocks -->
    <Setter Property="Foreground" Value="#9CDCFE"/>
    <Setter Property="FontSize" Value="16"/>
  </Style>
  <Style x:Key="BigCard" TargetType="Button" BasedOn="{StaticResource Card}">
    <Setter Property="FontSize" Value="20"/>
  </Style>
</Window.Resources>
...
<Button Style="{StaticResource Card}" Content="Styled"/>
```

- **Keyed** styles applied via `Style="{StaticResource key}"`.
- **Implicit** styles (TargetType, no key) apply to every element of that type.
- **BasedOn** chains setters (base first, derived overrides).
- Setters are merged as element attribute defaults — **explicit attributes win**.
- Setter `Property` names match the attributes builders read (`Background`,
  `Foreground`, `FontSize`, `Content`, `Width`, `Height`, `Padding`,
  `IsEnabled`, `Visibility`, `ToolTip`, `CornerRadius`, …).

### Live `{Binding}` with DataContext (OneWay / TwoWay / OneTime)

```go
type VM struct {
    datagrid.PropertyNotifier        // optional: enables live UI auto-refresh
    title string
}
func (v *VM) GetTitle() string  { return v.title }
func (v *VM) SetTitle(s string) { v.title = s; v.NotifyPropertyChanged(v, "Title") }

vm := &VM{title: "Hello"}
root, reg, err := widget.LoadUIFromXAMLWithContext(xamlBytes, vm)
// scope variant for manual control:
root, reg, scope, err := widget.LoadUIFromXAMLBindings(xamlBytes, vm)
// widget.LoadUIFromXAMLFileWithContext / LoadUIFromXAMLFileBindings also exist
```

```xml
<TextBlock Text="{Binding Title}"/>                          <!-- OneWay (default) -->
<TextBox   Text="{Binding Title, Mode=TwoWay}"/>             <!-- UI -> model too -->
<CheckBox  IsChecked="{Binding IsOn, Mode=TwoWay}"/>
<Slider    Value="{Binding Volume, Mode=TwoWay}"/>
<TextBlock Text="{Binding Count, StringFormat=Count = %d}"/> <!-- Go fmt -->
<TextBlock Text="{Binding User.Name}"/>                       <!-- dotted paths -->
```

- Paths resolve via `datagrid.GetPropertyValue` (struct fields, `Get<Prop>()`
  methods, maps, `PropertyGetter`, dotted paths).
- **OneWay/OneTime/TwoWay** via `Mode=`. **StringFormat** is a Go format string.
- **Live model→UI:** if the DataContext implements `INotifyPropertyChanged`
  (embed `datagrid.PropertyNotifier`, call `NotifyPropertyChanged`), bound
  widgets refresh automatically.
- **TwoWay UI→model** for `TextBox`, `CheckBox`, `ToggleButton`, `RadioButton`,
  `Slider`, `ComboBox.SelectedIndex` (a feedback-loop guard prevents cycles).
- `BindingScope.SetDataContext(obj)` / `.Refresh()` for manual control. Bound
  properties: `Text`/`Content`, `IsChecked`, `Value`, `Foreground`,
  `Background`, `IsEnabled`, `Visibility`, `ToolTip`, `SelectedIndex`,
  `IsExpanded` (Expander), `Header` (Expander/GroupBox),
  `Value` (NumericUpDown — TwoWay-ready).
- **`BindingScope.Dispose()`** — unsubscribes from model and language listener.
  Call when the loaded XAML tree is discarded (UI reload) to prevent leaks.
- **Duplicate-free subscriptions**: `SetDataContext` can be called repeatedly
  with the same model — no duplicate `PropertyChanged` handlers are created.

**Not yet (next increments):** `{RelativeSource}`/`{TemplateBinding}`,
`ControlTemplate`/`ContentPresenter`, property `Trigger`/`EventTrigger`,
live `ItemsControl` updates on `ObservableCollection` change.

### Value Converters (`IValueConverter`)

```go
type PctConverter struct{}
func (PctConverter) Convert(v interface{}) interface{}     { return fmt.Sprintf("%.0f%%", v.(float64)*100) }
func (PctConverter) ConvertBack(v interface{}) interface{} { return v }

widget.RegisterValueConverter("Pct", PctConverter{})   // register by key, then use in XAML
```

```xml
<TextBlock Text="{Binding Ratio, Converter={StaticResource Pct}}"/>
```

`Convert` runs model→UI (all binding paths incl. DataTemplate items);
`ConvertBack` runs UI→model on TwoWay write-back. Converters are Go objects
registered by key (custom types can't be instantiated from XAML in Go).

### Element-to-Element Binding (`{Binding ElementName}`)

```xml
<Slider Name="vol" Minimum="0" Maximum="1" Value="0.3"/>
<TextBlock Text="{Binding Value, ElementName=vol, Converter={StaticResource Pct}}"/>
```

Binds a property to another named element's property (one-way, live). The
source element must have `Name`/`x:Name`. Supported source properties: `Text`,
`Value`, `IsChecked`, `SelectedIndex`. Updates when the source raises its
change callback (Slider/TextBox/CheckBox/ToggleButton/RadioButton/ComboBox).

### DataTriggers (`Style.Triggers`)

```xml
<Style x:Key="StatusStyle" TargetType="TextBlock">
  <Setter Property="Foreground" Value="#80FF80"/>          <!-- base (inactive) -->
  <Style.Triggers>
    <DataTrigger Binding="{Binding Status}" Value="ERROR">
      <Setter Property="Foreground" Value="#FF5050"/>       <!-- active when Status==ERROR -->
    </DataTrigger>
  </Style.Triggers>
</Style>
```

`DataTrigger` setters apply when the bound value equals `Value`, and revert to
the base setter value otherwise. Re-evaluated on `INotifyPropertyChanged`
(same engine as bindings).

**Property `Trigger`** reacts to the styled control's own property:

```xml
<Style TargetType="CheckBox">
  <Setter Property="Foreground" Value="#888888"/>
  <Style.Triggers>
    <Trigger Property="IsChecked" Value="True">
      <Setter Property="Foreground" Value="#40FF40"/>
    </Trigger>
  </Style.Triggers>
</Style>
```

Supported trigger properties: `IsChecked`, `Value`, `SelectedIndex`,
`IsEnabled`, `Text`. Live for properties with a change callback
(CheckBox/ToggleButton/RadioButton/Slider/ComboBox/TextBox); `IsEnabled` and
others are evaluated at refresh time. `EventTrigger`/`MultiTrigger` are not yet
supported (parsed-but-ignored).

### ItemsControl + DataTemplate

```go
type Order struct{ Name string; Price float64 }
items := datagrid.NewObservableCollection()
items.Add(Order{"Widget A", 19.99})
vm := &VM{Items: items}
root, _, _ := widget.LoadUIFromXAMLWithContext(xaml, vm)
```

```xml
<ItemsControl ItemsSource="{Binding Items}">
  <ItemsControl.ItemTemplate>
    <DataTemplate>
      <StackPanel Orientation="Horizontal" Spacing="12">
        <TextBlock Text="{Binding Name}"/>
        <TextBlock Text="{Binding Price, StringFormat=$%.2f}"/>
      </StackPanel>
    </DataTemplate>
  </ItemsControl.ItemTemplate>
</ItemsControl>
```

For each item of the bound collection (`ObservableCollection` or a slice), the
`DataTemplate` is cloned and bound to that item (one-way, with `StringFormat`),
then laid out in a vertical `StackPanel`. Live updates on collection change are
a follow-up (reload or rebuild for now).

---

## WPF Feature Coverage (P1 batch)

### New layout panels & controls

| XAML | Notes |
|------|-------|
| `<WrapPanel Orientation="..." Spacing="..">` | Wraps children to new lines/columns |
| `<UniformGrid Rows=".." Columns="..">` | Equal cells; auto rows/cols if omitted |
| `<GroupBox Header="..">` | Bordered container with title |
| `<Expander Header=".." IsExpanded="True">` | Collapsible; click header toggles |
| `<ContentControl Template=".." Content="..">` | ControlTemplate host (see below) |

### Text styling

```xml
<TextBlock Text="Bold"      FontWeight="Bold"/>
<TextBlock Text="Italic"    FontStyle="Italic"/>
<TextBlock Text="Both"      FontWeight="Bold" FontStyle="Italic"/>
<TextBlock Text="Link"      TextDecorations="Underline"/>
```

Bold/italic use built-in Go Bold/Italic faces (registered automatically). Works
on `Label`/`TextBlock`.

### Brushes via property elements + gradients

```xml
<Button.Background><SolidColorBrush Color="#C04000"/></Button.Background>

<Panel.Background>
  <LinearGradientBrush StartPoint="0,0" EndPoint="0,1">
    <GradientStop Color="#1565C0" Offset="0"/>
    <GradientStop Color="#000814" Offset="1"/>
  </LinearGradientBrush>
</Panel.Background>
```

Property-element syntax `<X.Prop><...></X.Prop>` is lifted to attributes for
scalar brush properties (Background/Foreground/BorderBrush/Fill/CornerRadius/…).
`LinearGradientBrush` (2+ stops, vertical/horizontal) renders on `Panel`
backgrounds. `Opacity`, `RadialGradientBrush`, `ImageBrush` are not supported
(the renderer composites opaque solid fills only).

### Commands & hotkeys

```go
vm := &VM{ SaveCmd: widget.NewRelayCommand(func(){ /* save */ }) }
```

```xml
<Window.InputBindings>
  <KeyBinding Modifiers="Ctrl" Key="S" Command="{Binding SaveCmd}"/>
</Window.InputBindings>
...
<Button Content="Save" Command="{Binding SaveCmd}"/>
```

`Button.Command` (ICommand) runs on click if `CanExecute`. `Window.InputBindings`
(`KeyBinding`) are routed by the engine before focus dispatch. Implement
`widget.ICommand` or use `widget.NewRelayCommand`/`widget.RelayCommand`.

### MultiTrigger

```xml
<Style.Triggers>
  <MultiDataTrigger>
    <MultiDataTrigger.Conditions>
      <Condition Binding="{Binding A}" Value="on"/>
      <Condition Binding="{Binding B}" Value="ready"/>
    </MultiDataTrigger.Conditions>
    <Setter Property="Foreground" Value="#40FF40"/>
  </MultiDataTrigger>
</Style.Triggers>
```

`<MultiTrigger>` (property conditions) and `<MultiDataTrigger>` (data
conditions) apply setters when ALL conditions match.

### ControlTemplate + ContentPresenter + TemplateBinding

```xml
<ControlTemplate x:Key="Card">
  <Border Background="{TemplateBinding Background}" BorderBrush="#FFF" BorderThickness="1">
    <ContentPresenter/>
  </Border>
</ControlTemplate>
...
<ContentControl Template="{StaticResource Card}" Background="#2E7D32" Content="Hi"/>
```

`ContentControl` with a `ControlTemplate` renders the template's visual tree.
`ContentPresenter` is replaced by the control's content (child elements or
`Content` text). `{TemplateBinding Prop}` and
`{Binding RelativeSource={RelativeSource TemplatedParent}, Path=Prop}` pull from
the templated control's properties. (ControlTemplate on interactive controls
like Button keeps default click behavior — templating is supported on
`ContentControl`.)

### Still not supported (engine/scope limits)

`Opacity` & per-widget alpha compositing, `RadialGradientBrush`/`ImageBrush`,
animations/`Storyboard`/`EventTrigger`/`VisualStateManager`, `RelativeSource
AncestorType`, rich-text inlines (`<Run>/<Bold>` inside TextBlock),
`RenderTransform`/`LayoutTransform`.

---

## Interaction polish (Tier A)

- **GridSplitter** — now a real draggable splitter (was visual-only). Place it in
  its own thin Grid column/row; dragging resizes the adjacent cells.
  ```xml
  <Grid.ColumnDefinitions>
    <ColumnDefinition Width="*"/><ColumnDefinition Width="6"/><ColumnDefinition Width="*"/>
  </Grid.ColumnDefinitions>
  <Panel Grid.Column="0"/><GridSplitter Grid.Column="1"/><Panel Grid.Column="2"/>
  ```
- **ContextMenu (right-click)** — attach a menu to any widget; the engine shows it
  at the cursor on right-click (one context menu open at a time).
  ```xml
  <Button Content="...">
    <Button.ContextMenu>
      <ContextMenu>
        <MenuItem Header="Copy"/><MenuItem Header="Paste"/>
        <MenuItem Separator="True"/><MenuItem Header="Delete"/>
      </ContextMenu>
    </Button.ContextMenu>
  </Button>
  ```
  Also `widget.Base.SetContextMenu(pm)` in code.
- **ScrollViewer mouse wheel** — `ScrollView` scrolls on wheel up/down.
- **Mouse cursors** — widgets implement `widget.CursorProvider` (`Cursor(x,y)`):
  TextBox → I-beam, GridSplitter → resize. `Engine.CursorAt(x,y)` computes it; the
  Windows backend applies it natively (Linux/macOS: no-op for now).
- **TabIndex** — `<TextBox TabIndex="1"/>` controls focus order; negative excludes
  from Tab navigation. Honored by `CollectFocusables` (stable sort by TabIndex).

---

## Vector shapes

Real WPF shape elements (render via built-in primitives) with `Fill`, `Stroke`,
`StrokeThickness`:

```xml
<Ellipse   Left="20" Top="20" Width="120" Height="80" Fill="#E06C75" Stroke="white" StrokeThickness="3"/>
<Rectangle Left="20" Top="20" Width="160" Height="80" Fill="#98C379" RadiusX="14"/>
<Line      X1="20" Y1="140" X2="200" Y2="200" Stroke="#E5C07B" StrokeThickness="4"/>
<Polygon   Points="280,130 340,210 220,210" Fill="#C678DD" Stroke="white"/>
<Polyline  Points="360,200 380,150 400,200" Stroke="#56B6C2" StrokeThickness="3"/>
```

`Ellipse`/`Rectangle` are bounds-based (work in any layout container);
`Line`/`Polygon`/`Polyline` use explicit coordinates (intended for `Canvas`).
Go types: `widget.Ellipse`, `widget.RectangleShape`, `widget.Line`,
`widget.Polygon`, `widget.Polyline`.

---

## Editing maturity (Tier B)

### TextBox / TextInput

- **MaxLength** — caps the character count (typing and paste are both clamped):
  ```xml
  <TextBox MaxLength="20"/>
  ```
  Go: `ti.MaxLength = 20`.
- **Undo / Redo** — `Ctrl+Z` undoes, `Ctrl+Y` *or* `Ctrl+Shift+Z` redoes. Each
  text change pushes one step (cap 200). Starting a new edit clears the redo
  stack (WPF behaviour). Selection-only moves are not recorded.
- **Double-click word select** — a double left-click (≤400 ms, ≤4 px apart)
  selects the whole word under the cursor; word runes are letters, digits, `_`,
  and any non-ASCII (Cyrillic etc.).

### NumericUpDown (Extended-Toolkit-style spinner)

```xml
<NumericUpDown Minimum="0" Maximum="100" Increment="1"  Value="42"/>
<NumericUpDown Minimum="0" Maximum="10"  Increment="0.5" Decimals="1" Value="3.5"/>
```

- Aliases: `<IntegerUpDown>`, `<DoubleUpDown>`.
- Attributes: `Minimum`/`Min`, `Maximum`/`Max`, `Increment`/`Step`,
  `Decimals`/`DecimalPlaces` (or infer from `FormatString="F2"`), `Value`.
- Interaction: click the ▲/▼ spinner, mouse-wheel, or `Up`/`Down` arrows when
  focused; type a number directly and press `Enter` to commit. Value is always
  clamped to `[Min, Max]`; `Enter`/focus-loss parse the typed buffer.
- Go: `n := widget.NewNumericUpDown(); n.Min, n.Max, n.Step = 0, 100, 1`
  `n.SetValue(42)`, `n.Value()`, `n.OnChange = func(v float64){…}`.

### Input validation (IDataErrorInfo / ValidatesOnDataErrors)

WPF-style validation on TwoWay bindings. The `DataContext` model implements
`DataErrorInfo` (analogue of `IDataErrorInfo`); a binding marked
`ValidatesOnDataErrors=True` queries it after each write-back and puts the target
widget into an error state (red border + the message as ToolTip).

```go
type Form struct {
    datagrid.PropertyNotifier
    Age int
}
func (f *Form) DataError(prop string) string { // "" == valid
    if prop == "Age" && (f.Age < 0 || f.Age > 150) {
        return "Age must be 0..150"
    }
    return ""
}
```

```xml
<TextBox Text="{Binding Age, Mode=TwoWay, ValidatesOnDataErrors=True}"/>
```

- Interfaces (`widget` package): `DataErrorInfo{ DataError(prop string) string }`
  and `ValidationAware{ SetValidationError(msg string); ValidationError() string }`
  (implemented by `TextInput`).
- `scope.Validate() bool` re-checks every `ValidatesOnDataErrors` binding against
  the current model and returns `true` when all are valid — call it before
  committing/saving a form. Initial state is validated automatically on load.
- `TextInput.ErrorBorder` is the error colour (default Win10 red `#E81123`);
  `TextInput.SetValidationError("")` clears the state.

> Note: all widget callbacks (`OnChange`, `OnSelect`, `OnClick`, `OnTabChange`,
> DataGrid events, …) are dispatched **synchronously**, outside the widget's own
> mutex — write-backs reach the model in exact input order, and a handler may
> safely call back into the widget. Don't block inside a handler (it runs on the
> event thread); spawn your own goroutine for slow work.

### CollectionView (sort / filter / group)

`widget.CollectionView` is the engine's analogue of WPF `ICollectionView` /
`CollectionViewSource`. It wraps an `ObservableCollection` (or a plain slice) and
produces a derived view with **filtering**, **multi-key sorting**, and
**grouping**. Bind an `ItemsControl` to the view (via a `DataContext` property)
and it rebuilds automatically whenever the source changes *or* the view
parameters change.

```go
view := widget.NewCollectionView(people)            // people: *datagrid.ObservableCollection

view.SetFilter(func(it any) bool {                  // keep adults
    return it.(*Person).Age >= 18
})
view.SetSort(                                       // City asc, then Age desc
    widget.SortDescription{Property: "City"},
    widget.SortDescription{Property: "Age", Direction: widget.Descending},
)
view.SetGroup("City")                               // optional grouping

view.Items()   // []any — current filtered+sorted view
view.Groups()  // []CollectionViewGroup{ Name, Items }
view.Count()
view.Refresh() // force recompute (also fires automatically)
```

```xml
<!-- VM exposes  People *widget.CollectionView -->
<ItemsControl ItemsSource="{Binding People}">
  <ItemsControl.ItemTemplate>
    <DataTemplate><TextBlock Text="{Binding Name}"/></DataTemplate>
  </ItemsControl.ItemTemplate>
</ItemsControl>
```

- Sorting: numbers compared numerically, strings case-insensitively, `bool`
  `false < true`; multiple `SortDescription`s act as primary/secondary keys.
  `AddSort` appends, `SetSort` replaces, `ClearSort` removes.
- Filtering: `SetFilter(nil)` clears. Filter runs before sort before group.
- Live updates: when the source is an `ObservableCollection`, the view subscribes
  to its changes; subscribe to the view yourself via `AddViewChanged(func())`.
- `collectionItems` (used by `ItemsControl` expansion) recognises `*CollectionView`
  transparently, so anywhere an `ItemsSource` accepts a collection it also accepts
  a view.

> `DataGrid.SetItemsSource` still takes a raw `*ObservableCollection` (the
> `datagrid` package cannot import `widget`); for grids, apply sort/filter on the
> collection you hand it, or feed `view.Items()` into a fresh collection.

### UI virtualization (VirtualizingItemsControl)

`widget.VirtualizingItemsControl` is the engine's `VirtualizingStackPanel`
equivalent: it materialises a widget **only for the visible window** (+a small
buffer), so a list of 100 000 rows keeps ~15 live widgets. Scrolling recycles the
window; widgets are cached by item index.

```go
v := widget.NewVirtualizingItemsControl()
v.ItemHeight = 28                          // fixed row height (required)
v.Buffer = 2                               // extra rows above/below the viewport
v.SetItemBuilder(func(item any, i int) widget.Widget {
    return widget.NewLabel(item.(*Person).Name, white)
})
v.SetItems(people)                         // []any
v.BindCollectionView(view)                 // or: auto-refresh from a CollectionView
```

```xml
<!-- VM exposes  People *widget.CollectionView (or *ObservableCollection) -->
<VirtualizingItemsControl ItemHeight="24" Width="240" Height="320"
                          ItemsSource="{Binding People}">
  <VirtualizingItemsControl.ItemTemplate>
    <DataTemplate><TextBlock Text="{Binding Name}"/></DataTemplate>
  </VirtualizingItemsControl.ItemTemplate>
</VirtualizingItemsControl>
```

- Built-in vertical scrollbar + mouse-wheel + thumb drag (capture-aware);
  `ScrollBy(px)`, `ScrollY()`, `ItemCount()`.
- Clicks route to the per-row child widgets normally (they are real children for
  the visible window only).
- The `DataTemplate` is built per visible row via the same machinery as
  `ItemsControl`, so `{Binding}` paths, styles and converters work inside rows.
- Live updates: bound to a `CollectionView` it refreshes on sort/filter/group; to
  an `ObservableCollection` on add/remove.
- Note: requires a **fixed `ItemHeight`** (no per-item variable height yet) and a
  plain `ItemsControl` is still the right choice for short lists.

> The string-based `ListView` was already virtualized in its `Draw` (it only
> paints the visible rows); `VirtualizingItemsControl` extends that to arbitrary
> templated widgets.

---

## Render-on-demand & invalidation

By default the engine redraws every tick (full backward compatibility). Enable
on-demand rendering to skip frames while the UI is unchanged — near-zero CPU on
idle:

```go
eng.SetRenderOnDemand(true)

eng.Invalidate()                 // mark the whole frame changed (cheap, atomic)
eng.InvalidateRect(r)            // declare a changed region; the next frame's
                                 // tile-diff is limited to tiles touching it
eng.RenderCount()                // frames actually rendered (diagnostics)
```

What is tracked automatically (no Invalidate needed):
- input events (`SendMouseMove/SendMouseButton/SendKeyEvent`), `SetFocus`;
- `SetRoot`, `SetTheme`, `SetResolution`, `SetBackgroundFile`, modals;
- the data layer via `widget.SetUIChangeNotifier` (registered by the engine):
  `BindingScope.Refresh`, `{Loc}` re-translation, live `ItemsControl` /
  `VirtualizingItemsControl` rebuilds, `SetLocale`/`SetLanguage`;
- time-driven visuals: while the focused widget implements `widget.Animated`
  with `NeedsAnimation()==true` (TextInput caret, DataGrid cell editor) or a
  tooltip is "ripening", frames keep flowing.

**You must call `Invalidate()` yourself** only when mutating widgets directly
from app code in on-demand mode (e.g. `label.SetText` from your own goroutine).
In the default mode nothing changes.

Locking note: a frame no longer holds the engine's main lock — `SetRoot`/`Root`
and event dispatch are never blocked by rendering. Structural operations
(`SetResolution`, `RegisterFont*`, `SetTheme`, `SetBackgroundFile`) serialize
with the frame via an internal `frameMu`.

---

## Fonts

The engine ships with the built-in **Go Regular** font and registers bold/italic
variants automatically. Adding or switching fonts:

```go
// 1. Drop *.ttf/*.otf into assets/fonts/ — auto-registered at engine.New():
//      Roboto-Regular.ttf  -> usable as "Roboto" AND "Roboto-Regular"
//    If Roboto-Regular.ttf is present, it becomes the DEFAULT font.
eng := engine.New(1280, 800, 30)

eng.SetDefaultFont("Inter")              // switch the default font
eng.RegisterFontFile("Roboto", path)     // register a single named font
eng.RegisterFontDir("my/fonts")          // register a whole directory
eng.AvailableFonts()                      // []string of registered names
eng.RegisterFallbackFont(ttfBytes)        // glyph fallback (✓✗⚠, …)
```

XAML uses the family name via `FontFamily`:

```xml
<TextBlock Text="Hi" FontFamily="Roboto"/>
<TextBox FontFamily="Inter" FontSize="16"/>
```

**Recommended free fonts** (place the TTF in `assets/fonts/`; see
`assets/fonts/README.md`): Roboto (Apache-2.0, default), Open Sans (Apache-2.0),
Inter (SIL OFL-1.1). These are free/redistributable but **not MIT** — fonts are
licensed under OFL or Apache, both permissive. **Google Sans is proprietary and
must not be bundled** — use Inter instead.

---

## New APIs (v3 increments)

### PropertyNotifier — Handle-based subscriptions

```go
// Register handler; returns an ID for later removal.
id := notifier.AddPropertyChangedHandle(handler)
notifier.RemovePropertyChangedHandle(id)
notifier.HandlerCount() int  // number of active subscriptions (useful in tests)
```

Old `AddPropertyChanged` / `RemovePropertyChanged` still work (no breaking change).

### Language/Locale listener removal

```go
id := widget.AddLanguageListener(fn)   // returns int id
widget.RemoveLanguageListener(id)

id := widget.AddLocaleListener(fn)
widget.RemoveLocaleListener(id)
```

### BindingScope.Dispose

```go
scope.Dispose()  // unsubscribes from model + language listener
```

Call when the XAML tree is replaced (e.g., on navigation) to prevent memory leaks.

### XAML Cursor attribute

Any widget in XAML can declare its cursor shape:

```xml
<Button Cursor="Hand"/>
<TextInput Cursor="IBeam"/>
<GridSplitter Cursor="SizeWE"/>
```

Values: `Arrow`, `IBeam`, `Hand`, `SizeWE`, `SizeNS`.
Programmatically: `widget.SetCursor(c Cursor)` / `widget.CursorOverride() (Cursor, bool)` on `Base`.

### NumericUpDown bindings

```xml
<NumericUpDown Value="{Binding Qty, Mode=TwoWay}" Minimum="0" Maximum="100"/>
```

`Value` is now fully bindable (OneWay, TwoWay, ElementName source).

### Expander / GroupBox bindings

```xml
<Expander Header="{Binding Title}" IsExpanded="{Binding Open}"/>
<GroupBox Header="{Binding SectionName}"/>
```

`IsExpanded` can also serve as an ElementName source for other widgets.

### SetText API on interactive widgets

`Button`, `CheckBox`, `RadioButton`, `ToggleSwitch` now expose `SetText(s string)` / `GetText() string`
for uniform programmatic text updates (XAML binding internally calls `SetText`).

---

## End of Reference

This document covers the essential API for AI code generation with headless-gui. For detailed implementation examples, refer to:
- `cmd/showcase/main.go` — example application
- `tests/` directory — unit tests with usage patterns
- GUIDE.md and GUIDE_EN.md — user documentation
