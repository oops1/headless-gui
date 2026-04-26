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
| `<TextBox>` | `TextInput` | `Text`, `PlaceholderText`, `Width`, `Height`, `AcceptsReturn` |
| `<PasswordBox>` | `TextInput` | (created with NewPasswordInput) |
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

## End of Reference

This document covers the essential API for AI code generation with headless-gui. For detailed implementation examples, refer to:
- `cmd/showcase/main.go` — example application
- `tests/` directory — unit tests with usage patterns
- GUIDE.md and GUIDE_EN.md — user documentation
