# AI Agent Reference: headless-gui Framework

**Framework**: `github.com/oops1/headless-gui/v3`  
**Language**: Go  
**Rendering**: Off-screen to RGBA buffer with dirty tile output  
**No CGO**: Pure Go implementation  

---

---

## Working Rules (READ FIRST — repo conventions for AI agents)

Hard rules for any AI agent editing this codebase. Human docs: README.md /
README_RU.md, GUIDE.md / GUIDE_EN.md; roadmap: TODO.md.

1. **The headless contract is inviolable.** No feature may break:
   `engine.Frames()` (64×64 delta tiles, physical pixels),
   `SendMouseMove/SendMouseButton/SendKeyEvent`, logical widget coordinates,
   and **zero CGO** in every module. Anything that needs an OS window lives
   only under `window/` behind build tags.
2. **Branches:** work happens on `develop`. The git-flow release leaves HEAD
   on `master` — ALWAYS check `git branch --show-current` before committing
   and `git checkout develop` if needed.
3. **No mass formatting.** The repo is not gofmt-clean (CRLF); `gofmt -l`
   flags almost every file — that is expected. Format only the lines you touch.
4. **Verification matrix before any commit:**
   ```bash
   go build ./...
   go test ./...
   go test -race ./tests/ ./engine/
   go vet -unsafeptr=false ./...          # -unsafeptr=false required (purego)
   GOOS=linux  CGO_ENABLED=0 go build ./...
   GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...
   ```
5. **Verify visual changes by rendering:** build a tiny app,
   `eng.SaveFrames(dir)` writes PNG frames — inspect them. Golden tests
   (`tests/golden_*`) catch pixel regressions; regenerate goldens only for
   intentional visual changes.

### Load-bearing patterns

- **Invalidation:** widget setters compare state and call
  `Invalidate()`/`InvalidateRect` only on an actual change — the engine
  renders on demand. A missed invalidate = "frozen" UI; a spurious one =
  wasted CPU.
- **Mutexes:** a widget holds its `mu` only around state; callbacks
  (`OnChange`, `OnClick`) and `Invalidate()` are called AFTER Unlock.
  `Draw` copies state under mu, then draws without it.
- **Text measurement outside Draw:** `widget.MeasureUIText(text, sizePt)` —
  the precise measurer registered by the engine (`RegisterTextMeasurer`;
  `SetTextMeasurer` still works for a consumer that sets one once and for
  all). The most recently created engine answers; a stopped engine hands the
  measurer back to the previous live one. Use it
  for layout computed before painting (dialogs, TextBox). Inside Draw use
  `ctx.MeasureText`.
- **Theming:** constructors read the global palette `win10.*` (updated by
  `ApplyGlobalTheme`); `ApplyTheme(t *Theme)` recolors live widgets.
  Internal helper widgets may read `win10.*` directly in Draw. Secondary
  text: `Label.Muted = true` (theme paints it InputPlaceholder). Classic
  Win2000 is a separate draw branch (`currentStyle().Classic3D`, bevel
  helpers) — do not forget it.
- **Translucent colors — the premultiplied trap:** Go's `color.RGBA` is
  alpha-premultiplied. A color with channels above alpha (e.g.
  `{0,120,215,90}`) overflows during Over blending and turns magenta on
  light backgrounds. Build such colors via `premulAlpha(base, alpha)`
  (widget/textbox.go). Also: `FillRoundRect` with A<255 takes a legacy
  Src path (does NOT blend) — use `FillRectAlpha` for real blending.
- **Modal dialogs:** `engine.ShowModal` centers the dialog and shifts its
  children; Enter/Escape/Ctrl+C flow through
  `Dialog.HandleInputBinding`/`OnCancel` BEFORE focus dispatch; the ✕
  button is wired by the engine via `SetCloser`. Localization uses `dlg.*`
  keys with live switching through `Dialog.OnLanguageChange`
  (unsubscribed in `SetModal(false)`).
- **Key codes:** `widget.KeyCode` values match Windows VK codes and the
  browser's `e.keyCode`. A new key must be added to ALL mappings:
  `window/native.go` (VK_*), `window/window.go` (vkToKeyCode),
  `window/native_linux.go` (X11 keycode; Wayland reuses it via +8),
  `window/native_darwin.go`.
- **Headless input tests:** unit — call `w.OnKeyEvent/OnMouseButton`
  directly; end-to-end — `eng.SetFocus(w)` + `eng.SendKeyEvent`
  (see tests/*_test.go).

### Environment quirks

- `go vet` without `-unsafeptr=false` complains about purego — expected.
- Tests create engines without windows — they run in CI and WSL.
- On Windows shells, multi-line heredoc/patch strings can break on
  encoding — run Python patchers as `py -3 -X utf8`.

### Docs to update when adding a feature

README.md + README_RU.md (keep them symmetric!), GUIDE.md + GUIDE_EN.md
(the "New features" section), TODO.md (tick the item with a date), and
this file — new APIs go into a version section below.

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
NewDockManager() *DockManager           // VS-style toolbox docking zone (see "Docking" section)
NewDockPane(id, title string, content Widget) *DockPane
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
// Sizes are clamped to engine.MinCanvasSide..MaxCanvasSide (1..16384) and
// MaxCanvasPixels (64 Mpx); out-of-range values are logged (same in New)
func (e *Engine) SetResolution(width, height int)

// Load background image from file (PNG or JPEG)
// Automatically scales to canvas size
// Saved internally for rescaling on SetResolution
// Decoded through widget.DecodeImageBounded: header first, then
// widget.MaxImagePixels() (64 Mpx default, SetMaxImagePixels) and a 256 MB
// file cap — oversized/decompression-bomb images return an error.
func (e *Engine) SetBackgroundFile(path string) error

// Set the background from an image already in memory (v3.15.0). Same
// scaling and rescale-on-SetResolution behaviour as SetBackgroundFile;
// no decoding, no file cap. A shell that received the wallpaper down its
// own wire had to write it to a temporary file before this existed.
func (e *Engine) SetBackground(img image.Image) error

// Remove the background (v3.15.0). Widgets draw on the canvas background
// colour again. Idempotent.
func (e *Engine) ClearBackground()

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
// Capped at DefaultSaveFramesLimit (10000) frames; SaveFramesLimit(0) = no cap
func (e *Engine) SaveFrames(dir string)
func (e *Engine) SaveFramesLimit(n int)
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
| `<Window>` | `Window` | `Title`, `Width`, `Height`, `WindowStyle`, `ResizeMode`, `MainWindow`, `TrayIcon`, `TrayTooltip` (see "Tray from XAML") |
| `<TrayMenu>` | (child of `<Window>`) | tray context menu; `<MenuItem>`/`<Separator>` children (see "Tray from XAML") |
| `<MenuItem>` | (nested in MenuBar) | `Header`, `Items` |
| `<TreeView>` | `TreeViewWidget` | `Items`, `ItemHeight`, `ShowIndentGuides` |
| `<DataGrid>` | `DataGridWidget` | `ItemsSource`, `Columns` |
| `<SplitPanel>` | `SplitPanel` | `Orientation`, `Position`, `SplitterSize`, `MinFirst`, `MinSecond` (first two children = panes) |
| `<SVGIcon>` | `SVGIcon` | `Source`, `Color`, `Tint` |
| `<DockManager>` | `DockManager` | `Background`, `NativeFloating` (see "Docking") + children `<DockPane>`×N, one `<DockContent>` |
| `<DockPane>` | `DockPane` | `Id`, `Title`, `Side` (Left/Top/Bottom/Right), `Size` (px), `State` (Docked/AutoHidden/Floating/Closed); valid only inside `<DockManager>` |
| `<DockContent>` | (marker, not a widget) | single child → `DockManager.SetCenter`; valid only inside `<DockManager>` |

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

### DataGrid: header click, column order, per-row tooltip, striping

```go
// Header click apart from sorting. Fires regardless of CanUserSortColumns;
// true means "handled" and cancels the sort. The resize edge and the
// scrollbar are claimed first — no need to tell a click from a grab.
dg.OnHeaderClick = func(col dg.Column, idx, x, y int) bool { ... }

// Column order. Dragging is OFF by default: turning it on moves the header
// click from press to release (a click cannot be told from a grab until the
// button comes up).
dg.CanUserReorderColumns = true
dg.MoveColumn(from, to)
dg.OnColumnsReordered = func(from, to int) { ... }

// Per-row tooltip (Base.ToolTip is one text for the whole widget).
dg.RowToolTip = func(item interface{}, row int) string { ... }
dg.HoverRow() int          // row under the cursor, or -1
dg.RowIndexAtY(y int) int  // row at a Y coordinate, or -1
dg.ScrollX() int           // horizontal scroll (absolute cell X)

// Row striping. AlternateBG alone is not enough: ApplyTheme recomputes it
// from the theme background on every theme change.
dg.ZebraStripes = false

// Template columns can draw images:
//   cdc.DrawCtx.DrawImage(img, x, y)
//   cdc.DrawCtx.DrawImageScaled(img, x, y, w, h)
```

### Mouse modifiers — Ctrl/Shift+Click

```go
// MouseEvent carries Mod (ModShift | ModCtrl | ModAlt | ModMeta).
// The engine cannot derive it from key events: there is no modifier key in
// KeyCode, and holding Ctrl while clicking produces no key event at all.
eng.SetModifiers(widget.ModCtrl) // window.Run does this itself
eng.SendMouseButton(x, y, widget.MouseLeft, true)

// DataGrid with SelectionExtended: Ctrl+Click toggles, Shift+Click ranges.
```

### ToggleButton — persistent pressed state

```go
btn.SetToggle(true)
btn.SetChecked(true)   // set state; does NOT call the handler
btn.IsChecked()
btn.Toggle()           // flip and notify
btn.OnCheckedChanged = func(on bool) { ... }
```

XAML: `<ToggleButton IsChecked="True"/>`. No separate type — a ToggleButton
differs from a Button by exactly this state.

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

// Window.OnClose (fires on RELEASE of close button, if the cursor is still
// over it — Windows semantics; releasing off the button cancels the action).
// Same release-semantics apply to Window.OnMinimize / Window.OnMaximize.
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
    // Scaled result is cached by source identity (pointer + Bounds) and
    // size (32 entries / 16 Mpx LRU). Mutating an image in place requires
    // canvas.InvalidateImageCache(src) (nil clears everything).
    
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
    KeyInsert    KeyCode = 45
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
// DockSide — used by both DockPanel's attached Dock property (Left/Top/
// Bottom/Right/Fill) and DockManager/DockPane (Left/Top/Bottom/Right only —
// DockFill is not a valid DockManager side). See "Docking" section below.
const (
    DockLeft   DockSide = iota // 0 — WPF DockPanel.Dock default
    DockTop
    DockBottom
    DockRight
    DockFill // DockPanel only — last child fills remaining space
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
- `DockManager` — VS-style docking zone (center + 4 dockable sides)
- `DockPane` — a single docking panel hosted by `DockManager`

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

### Panel.OnClose Fires on MOUSE PRESS; Window Title Buttons Fire on RELEASE

`Panel.OnClose` fires on **press**:

```go
panel.OnClose = func() {
    // Fires immediately when user presses close button
    // Panel is still visible; you must close it explicitly
    eng.CloseModal(panel)
}
```

`Window.OnClose` / `Window.OnMinimize` / `Window.OnMaximize`, by contrast,
follow Windows semantics: pressing a title button **arms** it, and the callback
fires on **release** only if the cursor is still over the same button.
Releasing off the button (or moving away first) cancels the action without
firing. This lets a user abort a close/minimize/maximize by dragging away.

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
// widget.LoadUIFromXAMLFS(data, fsys) — markup resources from an fs.FS
// (embed.FS single-file builds); same SEC-8 containment as baseDir on disk.
// Resource paths resolve from the fsys ROOT, so use fs.Sub(embedded, "ui")
// to keep Source="icons/ok.png" relative to the markup file.
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
- **Method calls are restricted (security audit):** only *getter-shaped*
  methods are invoked — no arguments, exactly one result (an optional second
  `error` result is allowed). `Save() error`, `Delete(id)` and the like are
  never called from a binding path. A model may publish an explicit allowlist
  via `BindableMethods() []string`; `datagrid.SetStrictBindingMethods(true)`
  makes the allowlist mandatory (use it when XAML comes from an untrusted
  source). Reflection panics are recovered and logged; TwoWay writes require
  assignable/convertible types (no string↔number coercion) and setter-shaped
  `Set<Prop>` methods.
- **OneWay/OneTime/TwoWay** via `Mode=`. **StringFormat** is applied through
  `datagrid.SafeFormat`: WPF-style `{0}`, `{0:F2}`, `{0:N2}`, `{0:P1}`,
  `{0:D3}`, `{0:X}` and Go formats with exactly one verb; `%#v`, `%+v`,
  `%T`, `%p` and multi-verb strings degrade to `%v` + literal text.
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
- **`BindingScope.Dispose()`** — unsubscribes from the model (including the
  legacy `AddPropertyChanged` path), `ObservableCollection`/`CollectionView`
  sources and the language listener. Call when the loaded XAML tree is
  discarded (UI reload) to prevent leaks. Related unsubscribe APIs:
  `ObservableCollection.AddCollectionChanged` returns an id for
  `RemoveCollectionChanged(id)`; `CollectionView.AddViewChangedHandle` /
  `RemoveViewChanged(id)`; `DataGrid.Dispose()`, `CollectionView.Dispose()`,
  `VirtualizingItemsControl.UnbindCollectionView()`.
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
- Registered in `HasOwnLayout` (`SetBounds` → `updateVisible` re-positions the
  materialized rows itself), so hosting it in a `Canvas` / `TabItem` / `DockPane`
  does not double-shift the rows.

> The string-based `ListView` was already virtualized in its `Draw` (it only
> paints the visible rows); `VirtualizingItemsControl` extends that to arbitrary
> templated widgets.

---

## Theme presets & control styling (Win10/Win11/Win2000/Mac)

A theme is no longer just a palette — `Theme.Style` (`widget.ThemeStyle`)
controls the **shape** of controls:

- `ControlCorner int` — corner radius of Button/TextBox/ComboBox/ProgressBar
  (0 = square; Win11 = 6, Mac = 8). An explicit `Button.CornerRadius` (XAML)
  takes precedence.
- `Classic3D bool` + `BevelLight/BevelShadow/BevelDark` — classic Win9x/2000
  rendering: square corners, raised bevel buttons (sunken when pressed), sunken
  text fields and checkboxes, a raised arrow button inside ComboBox, a blocky
  segmented ProgressBar, **no hover tinting**.

Built-in presets (colors + style):

```go
widget.ThemeNames()            // ["Win10 Dark","Win10 Light","Win11 Dark","Win11 Light","Win2000","Mac"]
widget.ThemeByName("Win2000")  // fresh *Theme copy (case-insensitive), nil if unknown
eng.SetTheme(widget.ThemeByName("Mac"))
// Direct constructors: Win10DarkTheme/Win10LightTheme/Win11DarkTheme/
// Win11LightTheme/Win2000Theme/MacTheme. DarkTheme/LightTheme == Win10.
widget.CurrentThemeStyle()     // style of the active theme (for custom widgets)
```

Mac preset: #007AFF accent, green (#34C759) ToggleSwitch. Win11: #4CC2FF /
#005FB8 accents on Mica-like backgrounds. Win2000: silver `#D4D0C8` face,
navy title/selection.

Window shape follows the theme via `Window.ApplyTheme`: `ThemeStyle.WindowCorner`
rounds the window (Win11 = 8, Mac = 10, others 0) and `ThemeStyle.MacTitleBar`
switches the title bar to macOS traffic-lights (Mac theme) vs Windows-style
(everything else). Win2000 title buttons (─ □ ×) render as raised bevel buttons
with black glyphs so they stay visible on the navy gradient.

**Native OS window rounding.** The widget-level `Window.CornerRadius` only rounds
the engine-drawn `widget.Window` chrome. To round the *actual borderless OS
window* (whose root is a plain `Canvas`), call `window.Window.SetCornerRadius(r)`
— on Windows it applies a round-rect region via `SetWindowRgn` (reapplied on
resize, dropped when maximized); macOS/X11 are no-ops for now. A `widget.Window`
root auto-propagates its `CornerRadius` to the native window. The showcase wires
`win.SetCornerRadius(theme.Style.WindowCorner)` into the theme dropdown, so
switching to Win11/Mac rounds the real app window (visible only when running on
the OS — not in headless frames).

Classic (Win2000) extras: scrollbars get ▲/▼ arrow buttons (click = one step;
ScrollView/ListView/VirtualizingItemsControl), title bars use a horizontal
navy→#A6CAF0 gradient (`Theme.TitleBG2`, A=0 disables) with **bold** caption
text (Window/Dialog/Panel headers), focus is a dotted rectangle (Button inner
rect; CheckBox/RadioButton around the label), menus are face-colored with a
raised 3D border and navy hover (`Theme.MenuBG`/`MenuHoverBG`/`MenuHoverText`),
tabs are compact labels hugging a raised page.

Note on `SetTheme` semantics: container backgrounds follow the theme too —
`Canvas.ApplyTheme` repaints an opaque Background to `WindowBG`,
`Panel.ApplyTheme` to `PanelBG` (explicit XAML colors are replaced, consistent
with all other widgets). MenuBar text uses `LabelText`.

`window.Window.Close()` closes the native window programmatically (`Run()`
returns) — use it for a "File → Exit" menu item.

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

// 2. assets/fonts is resolved relative to the PROCESS WORKING DIRECTORY.
//    A program installed elsewhere finds nothing there and silently stays on
//    the built-in Go Regular. Ship the fonts inside the binary instead:
//
//      //go:embed assets/fonts
//      var fontsFS embed.FS
//
err := eng.RegisterFontFS(fontsFS, "assets/fonts") // same names as on disk
```

XAML uses the family name via `FontFamily`:

```xml
<TextBlock Text="Hi" FontFamily="Roboto"/>
<TextBox FontFamily="Inter" FontSize="16"/>
```

**Bundled free fonts** (already in `assets/fonts/`, each with its license file;
see `assets/fonts/README.md` for the licenses and the redistribution duties):
Roboto (default), Open Sans, Inter, Liberation Sans, Liberation Mono, DejaVu
Sans, DejaVu Sans Mono, Go Regular. All are SIL OFL-1.1 except DejaVu
(Bitstream Vera + public domain) and Go (BSD-3-Clause) — free/redistributable
including commercially, but **not MIT**: the license file must travel with the
font. Liberation is metric-compatible with Arial/Courier New; DejaVu has the
widest glyph coverage and doubles as the engine's own fallback (so do not
rename `DejaVuSans.ttf` / `DejaVuSansMono.ttf`). Bold/Italic files are separate
named fonts (`FontFamily="LiberationSans-Bold"`), not weight variants.
**Google Sans is proprietary and must not be bundled** — use Inter instead.

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

## v3.7.0 additions (dialogs, multiline TextBox, browser streaming)

### Standard dialogs (engine-drawn, headless-capable)

All dialogs are built from widgets and shown via `engine.ShowModal` — they
work headless/streamed (file dialogs list the PROCESS/server filesystem),
follow the theme, and are localized (`dlg.*` keys, EN/RU built in, live
switch). Modern themes draw rounded chrome + soft shadow + close button;
Win2000 keeps the classic bevel look.

```go
mb := widget.NewMessageBox(eng)

// MessageBox: severity presets; empty caption -> localized default title.
mb.ShowInfo(caption, msg)                        // blue "i"
mb.ShowWarning(caption, msg)                     // orange triangle
mb.ShowError(caption, msg)                       // red X
mb.ShowQuestion(caption, msg, func(r widget.MessageBoxResult) {})   // Yes/No
mb.ShowSeverity(caption, msg, widget.SeverityWarning, widget.MBYesNoCancel, onResult)
// First paragraph = primary tone; paragraphs after "\n" render muted.
// Enter = default (accent) button, Escape / close btn = cancel,
// Ctrl+C = Windows-style dump ("---" separators, CRLF).

// Input dialog: validator returns "" (ok) or an error message.
id := mb.ShowInput(title, label, initial,
    func(s string) string { return "" },
    func(text string, ok bool) {})
id.SetHint("gray persistent hint under the field")   // error replaces it in red

// Progress dialog (thread-safe setters; onCancel nil -> no Cancel, no close btn).
pd := mb.ShowProgress(title, status, onCancel)
pd.SetStatus("file.jpg"); pd.SetDetail("34 of 120 - 61 MB/s")
pd.SetProgress(0.28)          // updates the percent label
pd.SetIndeterminate(true)     // running block, percent hidden
pd.Close()                    // idempotent

// File dialogs.
opts := widget.FileDialogOptions{
    StartDir: dir, InitialName: "a.txt", ShowHidden: false,
    Filters: []widget.FileFilter{{Label: "Tables", Exts: []string{".xlsx", ".csv"}}},
    Places:  []widget.FilePlace{{Label: "srv-share", Path: "//srv/share"}},
}
mb.ShowOpenFile(opts, func(path string, ok bool) {})  // places sidebar, breadcrumb,
                                                      // Name/Size/Modified columns
// Confine the dialog to a set of roots (navigation, Places and typed paths
// cannot leave them) — mandatory when the UI is streamed to a browser, since
// the dialog lists the SERVER's file system:
opts.AllowedRoots = []string{"/srv/uploads"}          // per dialog
widget.SetDefaultAllowedRoots("/srv/uploads")         // for all new dialogs
mb.ShowSaveFile(opts, onResult)                       // compact form + overwrite warning
mb.ShowPickFolder(opts, onResult)
```

Hidden entries: dot-prefixed names everywhere; on Windows additionally the
Hidden/System file attributes (like Explorer). Long names are ellipsized to
the column. `Dialog` gained `DefaultAction`/`CancelAction`/`CopyText`,
`ShowCloseButton`, `RequestClose()`, `OnLanguageChange(apply)`.

### Multiline TextBox

```go
tb := widget.NewTextBox("placeholder")
tb.Wrap = true        // word wrap (false -> horizontal scroll)
tb.ReadOnly = false
tb.SetText("line1\nline2"); tb.GetText(); tb.SelectedText()
tb.CaretPosition(); tb.LineCount(); tb.ScrollTop()
tb.OnChange = func(text string) {}
```

Keys: arrows (Up/Down keep a goal column), Ctrl+arrows word jumps,
Home/End, Ctrl+Home/End, PgUp/PgDn, Shift+navigation selects,
Ctrl+A/C/X/V, Ctrl+Z/Y, Enter inserts a newline. Mouse: click/drag
selection, double-click word, wheel scroll, context menu. Layout uses
`widget.MeasureUIText` -> caret math works headless.

XAML: `<TextBox AcceptsReturn="True"/>` or `TextWrapping="Wrap"` builds
this editor; a plain `<TextBox>` still builds the single-line TextInput.
TwoWay `Text` binding supported for both.

New key codes (mapped on ALL backends): `KeyPageUp` (33), `KeyPageDown`
(34), `KeyY` (89 — Ctrl+Y redo).

### Browser streaming (output/webstream)

```go
srv := webstream.New(eng)   // the SOLE consumer of eng.Frames()
// Hardened variant (recommended for anything beyond loopback):
srv = webstream.NewWithOptions(eng, webstream.Options{
    Token:          "s3cret",           // required on /ws, /snapshot.png, /stats
    AllowedOrigins: []string{"app.example.com"}, // extra Origins besides Host
    MaxClients:     16,                 // extra connections get 503
})
go srv.Run()
defer srv.Close()
hs := &http.Server{Addr: "127.0.0.1:8091", Handler: srv,
    ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
    IdleTimeout: 2 * time.Minute}       // no WriteTimeout — it is a stream
webstream.LogListen(hs.Addr, "s3cret") // honest listen log + exposure warning
hs.ListenAndServe()                    // "/" embedded viewer, "/ws" stream
```

Security posture: the server has no TLS and no user accounts. Bind to
loopback unless you set a token; the viewer forwards `?token=` from its own
URL to the WebSocket and `/stats`. WebSocket handshake requires GET,
`Connection: upgrade`, version 13, and an Origin whose host equals the
request Host (or is in `AllowedOrigins`); unmasked client frames close with
1002, frames and reassembled messages above 1 MB with 1009. Read/write
deadlines (60 s / 10 s), server pings (20 s) and the client cap keep dead
peers from leaking goroutines. All client input is funnelled through ONE
dispatcher goroutine (the engine's input API is single-threaded), with a
per-client token bucket (500 ev/s, burst 100) and coordinate/button/key
validation; drops are counted in `Stats.InputDropped`. Keyframes and
`/snapshot.png` are cached per frame and encoded outside the server lock;
a client that dropped a delta receives a keyframe next.

Zero-dep WebSocket server (RFC 6455: handshake, masked client frames,
fragmentation, ping/pong). Binary protocol: `0x02 init [u16 w][u16 h]`,
`0x01 tiles [u16 n] { [u16 x][u16 y][u16 w][u16 h][u32 len][PNG] }xn`
(big-endian). The server keeps a composite image of the screen, so every
new client receives a full keyframe, then deltas; slow clients skip
frames (buffered chan, non-blocking send). Input events arrive as JSON
text messages: `{"t":"mm","x":..,"y":..}`, `{"t":"mb",..,"b":0,"p":true}`,
`{"t":"wh",..,"d":1}`, `{"t":"kd"/"ku","c":keyCode,"r":codepoint,"m":mods}`
(mods: 1=Ctrl 2=Shift 4=Alt; browser `e.keyCode` == `widget.KeyCode`).
Demo: `go run ./cmd/webdemo` -> http://localhost:8091.

---

## v3.8.0 additions (animation framework)

```go
// Ядро: тик получает прогресс 0..1 ПОСЛЕ кривой; часы у движка
// (StepAnimations в рендер-цикле — ни горутин, ни таймеров на анимацию).
a := widget.Animate(300*time.Millisecond, widget.EaseOutCubic, func(t float64) {
    pb.SetValue(widget.LerpF(from, to, t)) // сеттеры сами инвалидируются
})
// Owner-replace (CSS-transition): новая анимация с тем же (owner, tag)
// останавливает предыдущую — анимации не «дерутся» при быстрых кликах.
widget.AnimateOwned(w, "bounds", dur, curve, tick)
a.Stop(); a.Running(); a.OnDone = ...; a.Loop; a.AutoReverse
// Конфиг-поля (OnDone/Loop/AutoReverse) выставлять сразу после Animate,
// в той же горутине — до первого Step (иначе data race).

// Кривые: EaseLinear, In/Out/InOut × Quad/Cubic/Sine, EaseOutBack,
// EaseOutElastic, EaseOutBounce. Лерпы: LerpF/LerpInt/LerpRect/LerpColor
// (premultiplied-корректно). Обёртки: AnimateFloat, AnimateRect.

// Готовые анимации виджетов: ToggleSwitch (ручка), Dialog (fade-in dim),
// ProgressBar.AnimateValue(v). Classic3D (Win2000) — всё мгновенно.
```

Правила для новых анимаций виджетов: тик — внешний колбэк (зовёт только
потокобезопасные сеттеры, не лезет под чужой mu); при отсутствии живого
движка виджет обязан рисовать каноническое конечное состояние (пример —
флаг `animating` у ToggleSwitch, взводимый ПЕРВЫМ тиком); полноэкранные
эффекты (dim диалога) инвалидируют весь UI (`notifyUIChanged`), а не свой
rect — иначе частичная перерисовка заморозит эффект вне bounds.

Диагностика по PNG-кадрам: `SaveFrames` пишет ТОЛЬКО кадры с тайлами, а
именует их сквозным seq → в on-demand нумерация С ДЫРКАМИ. «Последний
кадр» ищи через ReadDir + максимальное имя, НЕ перебором от 1 до первого
отсутствующего.

---

## v3.9 additions (old-school clipboard keys + Win2000 chrome)

Text editing (`TextInput` и `TextBox`) теперь понимает «старошкольные»
клавиши буфера обмена и удаление слова:

- **Ctrl+Insert** = копировать (алиас Ctrl+C; в password-режиме запрещено).
- **Shift+Insert** = вставить (алиас Ctrl+V; при `ReadOnly` у TextBox запрещено).
- **Shift+Delete** = вырезать (алиас Ctrl+X) — приоритетнее обычного Delete;
  password/ReadOnly блокируют.
- **Ctrl+Delete** = удалить слово ВПЕРЁД от каретки.
- **Ctrl+Backspace** = удалить слово НАЗАД от каретки.

Новый код клавиши: `KeyInsert = 45`, замаплен во ВСЕХ бэкендах
(`VK_INSERT=0x2D`; X11 keycode 118; на macOS — best-effort через Help=114).
Undo-история и `OnChange` работают как у обычных правок.

Классический chrome окна (Win2000): в классике введена ЭФФЕКТИВНАЯ высота
заголовка `Window.effTitleH()` (24px вместо полных 32) — вся геометрия
классики (titleBarRect, кнопки, локаль-бейдж, текст, ContentBounds) считается
от неё, поэтому титлбар и кнопки (side = effTitleH-6 = 18px) компактнее и
ближе к референсу. Хром-рамка (толстая 3D в классике; 1px XOR-рамка главного
окна в современных темах) рисуется ПОСЛЕ детей — контент с абсолютными
координатами больше не «замазывает» полосу рамки.

---

## v3.10 additions (mouse wheel in all scrollable widgets)

Колесо мыши теперь прокручивает **все** скроллируемые виджеты (шаг — 3 строки за тик, с клампом на границах): `ScrollView`, `TextBox`, `ListView`, `TreeView` (`TreeViewWidget`), `DataGrid` (`DataGridWidget`), `VirtualizingItemsControl`, `NumericUpDown` и `fileTable` в диалогах. Правило поглощения: виджет возвращает `true` из `OnMouseButton` только когда прокрутка реально сдвинулась; если контент помещается или каретка уже у границы — возвращает `false`, чтобы событие всплыло к родительскому `ScrollView` (вложенный список внутри страницы не блокирует прокрутку страницы, когда сам доскроллен). Ядра `treeview.TreeView` и `datagrid.DataGrid` получили `WheelScroll(up bool) bool` и `ScrollY() int`; у `ListView` добавлен `ScrollY() int`. Dropdown и PopupMenu колеса не получили — их раскрытые списки рисуются целиком без механики прокрутки (out of scope).

---

## Нативные окна (v3.10)

Модальные диалоги и popup-оверлеи (dropdown/меню) выносятся в **собственные
окна ОС**. Плюс — иконка в трее, balloon-уведомления и живое превью окна в
панели задач (Windows). Всё это живёт под `window/` за build-тегами; на
платформах без поддержки — вежливый фолбэк или no-op. Headless-контракт не
затронут: без нативного окна диалоги и оверлеи рисуются в холст, как раньше.

### Поведение по платформам

| Возможность | Win32 | X11 | Wayland / macOS / headless |
|---|---|---|---|
| Модалка в своём окне (`ModalHost`) | нативно | in-canvas | in-canvas фолбэк |
| Popup-оверлей в своём окне (`PopupSink`) | нативно | нативно | in-canvas фолбэк |
| Трей / balloon / превью | да | no-op (ошибка/false) | no-op (ошибка/false) |

Фолбэк выбирается автоматически по способностям бэкенда (наличие owner-окон,
окон-попапов, маршалинга на UI-поток); приложению делать ничего не нужно.

### Модальные диалоги в своих окнах

Движок делегирует показ модалки хосту, если он установлен:

```go
// engine.ModalHost — хост нативных модалок (реализует window.dialogHost).
eng.SetModalHost(host)          // window.Window ставит его сам в Run()
```

`window.Window.Run()` вызывает `installModalHost()` — если бэкенд умеет
owner-окна (Win32), каждый `*widget.Dialog` открывается в отдельном окне ОС со
своим вторичным движком (холст ровно по размеру диалога). Диалог теперь может
быть **больше** главного окна и перетаскиваться за его пределы. `Dialog`
получил для этого:

```go
dlg.OnDragMove = func(dx, dy int) { /* перенос нативного окна */ } // ставит хост
dlg.CornerRadius = 8   // скругление окна диалога (Win11 — DWM, Win10 — регион)
```

Диалог поверх диалога (например, файловый из обычного) даёт стек окон: owner
нового — верхнее окно стека; закрытие возвращает фокус вниз по стеку.

### Popup-оверлеи в своих окнах (dropdown / меню)

```go
// engine.PopupSink — приёмник кадров активных оверлеев.
eng.SetPopupSink(func(frames []engine.PopupFrame) { /* хост окон-попапов */ })
```

Оверлей-виджет объявляет себя двумя интерфейсами (уже реализованы у `Dropdown`,
`PopupMenu`, каскадных подменю):

```go
type OverlayDrawer interface { HasOverlay() bool; DrawOverlay(ctx DrawContext) }
type OverlayBoundsProvider interface { OverlayBounds() image.Rectangle } // абс. лог. коорд.
```

Движок рендерит каждый оверлей в отдельный буфер и отдаёт `PopupSink`; хост
(`window.popupHost`) создаёт/двигает/закрывает окно-попап у нужной точки и
транслирует его мышь обратно в движок-носитель. Пока `PopupSink` установлен,
`widget.SetPopupsHosted(true)` отключает клэмп меню по границам холста —
позиционированием занимается хост, оверлей вправе выходить за окно.

### Трей, balloon-уведомления, превью (Windows)

Публичный API на `*window.Window`. Можно вызывать до `Run()` (состояние
буферизуется и применяется при создании окна) или из обработчиков UI. На
не-Windows — no-op/ошибка.

```go
// Иконка в трее. icon масштабируется до SM_CXSMICON, прозрачность из альфы.
// Повторный вызов — обновление (NIM_MODIFY). Дефолт: если иконка задана и
// OnTrayClick не переопределён, двойной левый клик восстанавливает окно.
win.SetTrayIcon(icon image.Image, tooltip string) error
win.RemoveTrayIcon()
win.SetOnTrayClick(func(button widget.MouseButton, doubleClick bool))

// Трей-меню — НАШЕ widget.PopupMenu по правому клику, рендерится в окне-попапе
// у курсора (даже при скрытом главном окне). Меню добавляется в дерево корня.
win.SetTrayMenu(menu *widget.PopupMenu)

// Balloon-уведомление (NIF_INFO). Значок по severity: Info/Question→NIIF_INFO,
// Warning→NIIF_WARNING, Error→NIIF_ERROR. ТРЕБУЕТ ранее заданной иконки трея —
// иначе возвращает ошибку.
win.ShowBalloon(title, text string, severity widget.DialogSeverity) error
win.SetOnBalloonClick(func())

// Сворачивание в трей (SW_HIDE — исчезает из панели задач) и восстановление.
win.HideToTray()
win.RestoreFromTray()
```

Пример:

```go
win := window.New(eng, "My App")
win.SetTrayIcon(iconImg, "My App")             // до Run()
m := widget.NewPopupMenu()
m.AddItem("Показать", func() { win.RestoreFromTray() })
m.AddItem("Выход",    func() { win.Close() })
win.SetTrayMenu(m)
win.SetOnBalloonClick(func() { win.RestoreFromTray() })
go func() { win.ShowBalloon("Готово", "Задача выполнена", widget.SeverityInfo) }()
win.Run()
```

**Превью в панели задач.** Раньше `PrintWindow`/DWM-превью нашего borderless-окна
были чёрными (блит шёл мимо путей захвата). Теперь `wndProc` обрабатывает
`WM_PRINTCLIENT`/`WM_PRINT` — блитит кэш последнего кадра (`frameBuf`) в
переданный HDC. Этого достаточно для миниатюры при наведении и Aero Peek.
Дополнительно есть честный **iconic-путь** (`DWMWA_FORCE_ICONIC_REPRESENTATION`
+ `DWMWA_HAS_ICONIC_BITMAP`, `WM_DWMSENDICONICTHUMBNAIL` /
`WM_DWMSENDICONICLIVEPREVIEWBITMAP` из кэша кадра). Он **выключен по умолчанию**
и включается переменной окружения `HEADLESS_GUI_ICONIC_PREVIEW=1` — на случай,
если WM_PRINTCLIENT окажется недостаточно на конкретной системе.

Идентификаторы сообщений: `InvokeOnUIThread` занимает `WM_APP` (0x8000);
callback иконки трея — `WM_APP+1` (`wmTrayCallback`).

---

## v3.11 additions (smooth scroll, SplitPanel, SVG icons, file drop, color emoji)

Пять фич поверх v3.10. Для каждой ниже явно указано, что работает **headless**
(без окна ОС, через `engine.Send*` + тесты) и что требует **нативного** окна с
платформенными оговорками.

### Плавный / инерционный скролл — `SendMouseWheelPixels` / `OnMouseWheelPixels`

Точная пиксельная дельта колеса/тачпада вместо целых «тиков».

```go
// Движок: точная дельта в физических пикселях окна/кадра. dy>0 — вниз, dx>0 — вправо.
func (e *Engine) SendMouseWheelPixels(xPhys, yPhys int, dx, dy float64)
```

Событие всплывает от самого глубокого виджета под курсором к корню; первый,
реализующий `wheelPixelHandler` и вернувший `true`, поглощает дельту.

```go
// Опт-ин интерфейс виджета (widget package). dy>0 — вниз. true = поглощено;
// false = прокручивать нечего / упёрлись в край по жесту (дельта всплывёт к родителю).
type wheelPixelHandler interface {
    OnMouseWheelPixels(x, y int, dx, dy float64) bool
}
```

- **Фолбэк (headless-контракт цел):** если точную дельту никто не принял,
  движок синтезирует эквивалентные тики через `SendMouseButton`
  (`MouseWheelUp`/`Down`, 40 px = 1 тик) — старые виджеты продолжают работать.
- **Реализовали `OnMouseWheelPixels`:** `ScrollView` (инерция-«маховик» через
  `AnimateOwned` на часах движка, без горутин; любой press/клик гасит бросок;
  в `Classic3D` — мгновенно, без инерции), `ListView` и `TextBox`
  (попиксельно с субпиксельным накоплением, всплытие на краях).
- **Headless:** полностью — `SendMouseWheelPixels` + инерция на часах движка
  работают без окна (см. `tests/smoothscroll_test.go`).
- **Нативно (бэкенды):** Win32 конвертирует `WM_MOUSEWHEEL` delta/120 в пиксели
  (точные тачпады шлют дробные дельты); Wayland форвардит `wl_pointer.axis`
  (`wl_fixed`) как пиксели. **X11 остаётся на тиках** (кнопки 4/5 не несут
  пиксельных данных); **macOS-колесо по-прежнему не эмитится** (доверху).

### SplitPanel — контейнер двух панелей с разделителем

`widget/splitpanel.go`. Держит ДВУХ детей (первые два `AddChild` → First/Second),
раскладывает по обе стороны перетаскиваемой полосы. Позиция — доля `0..1`, так что
ресайз сохраняет соотношение.

```go
func NewSplitPanel(orient Orientation) *SplitPanel   // Position=0.5, SplitterSize=6

type SplitPanel struct {
    Base
    Orientation  Orientation  // Horizontal — панели слева/справа (полоса вертикальная); Vertical — сверху/снизу
    SplitterSize int          // толщина полосы, px (по умолчанию 6)
    Position     float64      // доля 0..1 доступного места (размер First)
    MinFirst     int          // мин. размеры панелей, px (клэмп при drag/раскладке, не при коллапсе)
    MinSecond    int
    Background   color.RGBA   // цвет полосы (обычный)
    HoverColor   color.RGBA   // цвет полосы при hover/drag
    OnPositionChanged func(pos float64) // drag, коллапс, SetPosition
}

// Методы
sp.First() Widget           // sp.Second() Widget
sp.SetPosition(pos float64) // клэмп 0..1, снимает коллапс, уведомляет
sp.Collapse()               // свернуть First (Position→0, прежняя позиция запоминается)
sp.Expand()                 // развернуть обратно
sp.ToggleCollapse()
sp.IsCollapsed() bool
```

Взаимодействие: hover над полосой → курсор `CursorSizeWE`/`CursorSizeNS`;
drag ЛКМ через `CaptureManager` (drag НЕ считается кликом для double-click);
двойной клик по полосе — коллапс/восстановление First; SplitPanel'ы вложены.
Зарегистрирован в `HasOwnLayout` (сам перекладывает детей в `SetBounds`), так что
анкор-контейнеры (Canvas/DockPanel) не «двоят» сдвиг. Тема — `Theme.SplitterBG` /
`Theme.SplitterHoverBG` (`ApplyTheme`).

XAML (первые два дочерних элемента — панели):

```xml
<SplitPanel Orientation="Horizontal" Position="0.35" SplitterSize="6"
            MinFirst="120" MinSecond="200">
  <Panel Background="#1E1E1E"/>   <!-- First -->
  <Panel Background="#252526"/>   <!-- Second -->
</SplitPanel>
```

Атрибуты: `Orientation` (Horizontal/Vertical), `Position` (0..1), `SplitterSize`,
`MinFirst`, `MinSecond`, `Background`, `HoverColor` + общие (`Name`, `Grid.Row/Column`, …).

**Headless:** полностью (drag/коллапс через `Send*`, см. `tests/splitpanel_test.go`).
Для ячеек `Grid` по-прежнему используйте `GridSplitter`.

### SVG-иконки — пакет `widget/svg` + виджет `SVGIcon`

`widget/svgicon.go` рендерит темизируемую векторную иконку, растеризуя документ
под размер bounds с сохранением пропорций (центрирование).

```go
func NewSVGIcon() *SVGIcon                    // цвет по умолчанию = Theme.LabelText (следует за темой)
func NewSVGIconFromData(data []byte) *SVGIcon

ic.SetSVG(data []byte) error     // ic.SetSVGFile(path string) error
ic.SetColor(c color.RGBA)        // явный цвет (перекрывает тему); = currentColor и цвет Tint
ic.Color() color.RGBA
ic.SetTint(on bool)              // true — перекрасить ВЕСЬ контент в Color (монохром); false — только fill="currentColor"
ic.Tint() bool
ic.Err() error                   // ic.Document() *svg.Document
```

Перекраска: `fill="currentColor"` → `Color` виджета; без явного `SetColor`
`ApplyTheme` берёт `Theme.LabelText` (иконка «под текст»).

Пакет `widget/svg` — парсер+растеризатор icon-ориентированного подмножества:

```go
func svg.Parse(data []byte) (*svg.Document, error)  // svg.ParseFile(path)
func (d *svg.Document) RasterizeCached(w, h int, current color.RGBA, tint bool) *image.RGBA
```

- **Поддержано:** `path` со всеми командами (включая дуги `A` и smooth-кривые),
  `rect`/`circle`/`ellipse`/`line`/`polyline`/`polygon`, group-трансформы,
  `fill`/`fill-rule` (nonzero + even-odd)/`fill-opacity`/`currentColor`,
  атрибут `style`. Растеризация через `x/image/vector` (AA), кэш по
  face-независимым параметрам (размер/цвет/tint).
- **Ограничения (честно):** нет градиентов, `clipPath`, `text`; обводка (stroke) —
  упрощённая аппроксимация.
- **Headless/нативно:** одинаково — чистый CPU-растеризатор, окно ОС не нужно
  (см. `tests/svgicon_test.go`).

XAML: `Source` резолвится относительно базовой директории XAML-файла.

```xml
<SVGIcon Source="icons/menu.svg" Color="#FF3366" Tint="True"/>
<SVGIcon Source="icons/folder.svg"/>   <!-- без Color — цвет текста темы -->
```

Атрибуты: `Source`, `Color` (алиасы `Foreground`/`Fill`), `Tint` (True/False).

### Drag & Drop файлов из ОС — `SetOnFilesDropped` / `FileDropTarget`

Приём файлов, перетащенных из проводника/файлового менеджера в окно.

```go
// Окно: колбэк приложения. paths — абсолютные пути; x,y — ЛОГИЧЕСКИЕ пиксели
// клиентской области. Вызывать до Run().
func (win *window.Window) SetOnFilesDropped(fn func(paths []string, x, y int))

// Движок: доставка виджету под точкой (x,y — ФИЗИЧЕСКИЕ пиксели, как у SendMouse*).
// Всплытие от глубокого виджета к корню; первый вернувший true поглощает.
func (e *Engine) SendFilesDropped(x, y int, paths []string)

// Опт-ин интерфейс виджета-приёмника (widget package). x,y — ЛОГИЧЕСКИЕ координаты.
type FileDropTarget interface {
    OnFilesDropped(x, y int, paths []string) bool
}
```

Событие идёт двумя путями одновременно: в движок (`SendFilesDropped` → виджет
`FileDropTarget` под точкой, для headless-симметрии и тестов) и в
`win.onFilesDropped` (колбэк приложения, логические координаты).

- **Headless:** маршрутизация к `FileDropTarget` через `SendFilesDropped`
  полностью тестируема без окна (`tests/filedrop_test.go`).
- **Нативно по платформам:** **Win32** — полно (`WM_DROPFILES`, `DragAcceptFiles`
  на главном окне, не на попапах); **X11** — полно (XDND v5: `XdndAware`,
  Enter/Position/Status/Drop, `XConvertSelection` → async `SelectionNotify`,
  `text/uri-list`); **Wayland** — **каркас** (`wl_data_device`, принимает
  `text/uri-list`, требует живой проверки на реальной сессии); macOS — нет.
  `parseURIList` декодирует `file://` URI (percent-escapes, hostname, CRLF).

### Цветные эмодзи (COLR/CBDT) — автоматически

Шейпинг-тракт теперь рендерит цветные глифы вместо их отбрасывания. Публичного
API нет — работает само в общем текстовом пути (`DrawText*`, все виджеты).

- **Поддержано:** COLRv0 (плоские CPAL-слои), COLRv1 (обход графа paint с
  аффинными трансформами и сплошными заливками), CBDT/sbix (PNG-битмапы).
  Цветные глифы кэшируются отдельно от монохромных масок и блитятся как
  premultiplied RGBA без подкраски. Детектор шейпинга гонит эмодзи-диапазоны
  (включая ZWJ-последовательности и VS16) через HarfBuzz; fallback-цепочка
  предпочитает цветной эмодзи-шрифт. Проверено на Segoe UI Emoji (COLRv1):
  👍🎉🚀🔥 в цвете.
- **Ограничения (честно):** BMP-символы ниже U+1F000 остаются **монохромными**;
  **региональные флаги** (буквенные лигатуры) — известный пробел;
  **COLRv1-градиенты аппроксимируются средним** цветом (сплошная заливка).
- **Headless/нативно:** одинаково (растеризация в буфер; окно не нужно,
  см. `engine/emoji_test.go`).
- **Лицензии:** эмодзи-шрифт НЕ встроен в движок — глифы берутся из шрифта ОС
  во время работы (`systemFallbackFontPaths`: `seguiemj.ttf` на Windows и т.п.),
  как и прочие системные фолбэки. В строках — Unicode-кодпоинты, не артворк;
  проект ничего эмодзи-шрифтового не распространяет → лицензионных обязательств
  нет. Для гарантии на всех ОС — бандлить свободный шрифт (Noto Color Emoji, OFL).

> Внутреннее (без публичного API, для полноты): `FontCache.Kern` теперь кэширует
> пары кернинга (сброс на `SetDPI`) — ~1.7× на шрифтах с реальной kern-таблицей.
> Плюс точечная построчная инвалидация DataGrid/TreeView (`TakeDirty`) —
> selection/hover перерисовывают только затронутые строки; и исправлен бандинг
> градиента на дробных HiDPI-масштабах (ramp 1×h / w×1 + `DrawImageScaled`;
> при scale==1 байт-в-байт идентично).

---

## v3.12 additions (docking panels)

### Docking (DockManager/DockPane)

`widget/dockmanager.go` + `widget/dockpane.go`. A Visual Studio Toolbox-style
docking zone: a central document area (`Center`) surrounded by up to 4
dockable sides (Left/Top/Bottom/Right), each holding a stack of `DockPane`
panels. `DockManager` owns layout, resize-by-dragging-the-gutter, tabbed
stacks (2+ panes on the same side), auto-hide flyouts, and drag&dock (drag a
pane's title bar → docking guides appear via `DockManager`'s `OverlayDrawer`;
drop on a guide docks it, drop elsewhere floats it).

```go
func NewDockManager() *DockManager

type DockManager struct {
    Base
    SplitterSize   int // gutter thickness, px (0 → default 6)
    MinSideSize    int // min side size, px (0 → default 60)
    StripThickness int // auto-hide strip thickness, px (0 → default 22)
    TabStripHeight int // stack tab-strip height, px (0 → default 22)
    Background, GutterColor, GutterHoverBG, StripBG,
    TabBG, TabActiveBG, TabText, AccentColor, BorderColor, GuideFace color.RGBA
}

// Center / panes
func (m *DockManager) SetCenter(w Widget)
func (m *DockManager) Center() Widget
func (m *DockManager) Panes() []*DockPane           // all panes, incl. floating/closed
func (m *DockManager) FindPane(id string) *DockPane // nil if not found
func (m *DockManager) AddPane(p *DockPane, side DockSide) // registers + docks p to side

// Side sizing
func (m *DockManager) SideSize(side DockSide) int
func (m *DockManager) SetSideSize(side DockSide, px int) // clamped by layout

// Layout persistence
func (m *DockManager) SaveLayout() []byte
func (m *DockManager) RestoreLayout(data []byte) error
```

`DockSide` is the same type used by `DockPanel.Dock` (see "Dock Positions"
above), but only `DockLeft`/`DockTop`/`DockBottom`/`DockRight` are valid for
`DockManager` — `DockFill` is DockPanel-only and gets coerced to `DockLeft`.

```go
func NewDockPane(id, title string, content Widget) *DockPane // state starts PaneDocked

type DockPane struct {
    Base
    ID    string // stable id for SaveLayout/FindPane
    Title string // title-bar text
    TitleBarHeight int // 0 → default 24
    TitleBG, TitleActiveBG, TitleText, Background, BorderColor color.RGBA

    OnStateChanged func(p *DockPane)   // fires on any Dock/Float/Pin/Unpin/Close/Show
    OnFloatNative  func(p *DockPane)   // native-detach hook, see below
}

func (p *DockPane) Content() Widget
func (p *DockPane) SetContent(w Widget)
func (p *DockPane) State() DockPaneState
func (p *DockPane) Side() DockSide   // current/last dock side (meaningful even when AutoHidden/Floating)
func (p *DockPane) IsPinned() bool   // false only when AutoHidden

// State transitions — delegate to the owning DockManager if the pane was
// added via AddPane; otherwise just set local state (no manager to lay out).
func (p *DockPane) Dock(side DockSide)
func (p *DockPane) Float()
func (p *DockPane) Pin()   // AutoHidden → Docked
func (p *DockPane) Unpin() // Docked → AutoHidden (strip label at the edge)
func (p *DockPane) Close() // → Closed (hidden; Show() restores to last side)
func (p *DockPane) Show()
```

`DockPaneState` (`p.State()`, has a `.String()` for logging/debugging):

```go
const (
    PaneDocked     DockPaneState = iota // pinned to a side, in the stack
    PaneAutoHidden                      // collapsed to an edge label (pin off)
    PaneFloating                        // floats above the dock zone (drag/resize with mouse)
    PaneClosed                          // hidden; Show() brings it back
)
```

The title bar ships 3 buttons with release-semantics (same press-arms /
release-fires model as `Window.OnClose`): pin (Docked↔AutoHidden),
float/dock (Floating↔Docked), close.

**Native pane detach — `window.Window.EnableDockFloating(dm)`.** Call it
before `Run()`: on backends with owned windows + UI-thread marshaling
(Win32, X11) the float title-bar button pops the pane out into a real
non-modal OS window (own engine + surface, title drag moves the OS window,
dropdowns get their own popup host); the dock button/✕ returns/closes it,
and all detached windows tear down when the main window exits. Internally
this assigns the widget-layer hook `DockPane.OnFloatNative`. Where the
backend lacks support (Wayland/macOS) — and always in headless — the hook
stays unset and **floating is a widget-drawn overlay inside the canvas**: a
draggable/resizable rectangle on top of the center, fully headless-testable
via `SendMouseMove`/`SendMouseButton` (no OS window needed). Limitations:
drag-return onto the guides is not implemented yet (return via the dock
button), and the detached OS window is not resizable in this phase.

XAML (see also the "Docking" tab in `cmd/showcase`):

```xml
<DockManager Name="dockDemo" Background="#232338">
  <DockPane Id="tools" Title="Инструменты" Side="Left" Size="220" State="Docked">
    <ListView><ListViewItem Content="item 1"/></ListView>
  </DockPane>
  <DockPane Id="props" Title="Свойства" Side="Right" Size="200"/>
  <DockPane Title="Вывод" Side="Bottom" Size="120" State="AutoHidden">
    <TextBlock Text="log..."/>
  </DockPane>
  <DockContent>
    <TextBox Text="document area"/>
  </DockContent>
</DockManager>
```

- `<DockManager>`: `Background` + common bounds/props (`Name`, `Grid.Row/Column`,
  `Margin`, …) via `applyCommonProps`. Children: any number of `<DockPane>`
  plus at most one `<DockContent>`. `NativeFloating="True"` declares native pane
  detach: `window.Window.Run()` walks the tree, and if the app didn't call
  `EnableDockFloating` itself, enables it for the first manager marked this way
  (`DockManager.NativeFloating` field). An explicit `EnableDockFloating(dm)` call
  wins; with several `NativeFloating` managers only the first is wired (the host
  holds one) and the rest are logged. Headless / unsupported backends ignore it
  (floating stays a widget-drawn overlay). See also the "Docking" tab in
  `cmd/showcase` (`assets/ui/showcase.xaml`), which enables it declaratively.
- `<DockPane>`: `Id` (if omitted, generated by slugifying `Title`; if `Title`
  is also empty, an auto id like `pane1`); `Title` (defaults to the resolved
  `Id`); `Side` = `Left`/`Top`/`Bottom`/`Right` (case-insensitive, default
  `Left`); `Size` in px → `SetSideSize` for that side (if several panes on
  the same side set `Size`, the last one wins — harmless, it's one shared
  region); `State` = `Docked` (default, no-op) / `AutoHidden` (calls `Unpin()`
  right after adding) / `Floating` (calls `Float()`) / `Closed` (calls
  `Close()`); content is the pane's **first** child widget. `Name`/`x:Name`
  registers the pane in the XAML registry under that key instead of `Id`
  (`reg[name]`, so `FindByName` works); otherwise it's registered under `Id`.
  A `<DockPane>` outside `<DockManager>` is ignored (parses to nothing).
- `<DockContent>`: not a widget — a marker whose **single** child becomes the
  center via `SetCenter`. Ignored outside `<DockManager>`.

Layout persistence — `SaveLayout()`/`RestoreLayout()` round-trip through JSON:

```json
{
  "sizes": [left, top, bottom, right],
  "panes": [
    {"id": "tools", "state": 0, "side": 0, "active": true, "float": [x0, y0, x1, y1]}
  ]
}
```

`state`: 0=Docked 1=AutoHidden 2=Floating 3=Closed. `side`: 0=Left 1=Top
2=Bottom 3=Right. Panes are matched by `id`; ids missing from the manager are
ignored, and panes the manager has that are missing from the JSON keep their
current state untouched. `float` is the saved floating-window rect (only
meaningful when `state==2`).

- **Headless:** fully — layout, resize-by-gutter, stack tabs, auto-hide
  flyouts, drag&dock, and floating (widget-drawn) all drive through
  `SendMouseMove`/`SendMouseButton`/`SendKeyEvent`; no OS window required
  (see `tests/dock_test.go`).
- **Integration:** `DockManager` is registered in `HasOwnLayout` (it lays out
  its own children in `SetBounds`), so nesting it in `Canvas`/`Grid` doesn't
  double-shift its content. `DockPane` itself does not need a `HasOwnLayout`
  entry — it only ever lives inside a `DockManager`, which sets its bounds
  directly.

---

## Tray from XAML

A root `<Window>` can declare its system-tray icon, tooltip and context menu
right in XAML — no imperative `SetTrayIcon`/`SetTrayMenu` needed for the basic
case. The tray itself works only on Windows (Shell_NotifyIcon); on other
platforms the declaration parses fine and is simply a no-op at run time.

```xml
<Window Title="Моё приложение" Width="900" Height="600"
        TrayIcon="icons/app.svg" TrayTooltip="Моё приложение">
  <TrayMenu Name="trayMenu">
    <MenuItem Text="Показать"/>
    <Separator/>
    <MenuItem Text="Выход"/>
  </TrayMenu>
  <!-- …обычный контент окна… -->
  <Grid> … </Grid>
</Window>
```

- `TrayIcon` — path relative to the XAML file's directory (`baseDir`), like
  every file reference in markup (`Image Source`, `Button Icon`, `SVGIcon
  Source`, `Panel BackgroundImage`). **Resource sandbox:** when XAML is loaded
  from a file, the resolved path must stay inside that file's directory —
  absolute paths, drive-qualified paths and `..` escapes are rejected and
  logged once. XAML loaded from a string keeps CWD-relative semantics
  (absolute allowed — the program author supplied it).
  `.png`/`.jpg`/`.jpeg` are decoded as-is; `.svg` is rasterized to 32×32 with
  the document's own colors preserved (`currentColor` → theme label color,
  no monochrome tint — the tray icon is intentionally **not** themed). A load /
  parse error is logged (`log.Printf`) and skipped (no icon).
- `TrayTooltip` — tooltip string; defaults to the window `Title` if omitted.
- `<TrayMenu>` — a single child of `<Window>` holding the tray context menu.
  Parsed by the same popup-menu builder as `<ContextMenu>`/`<PopupMenu>`:
  `<MenuItem Text="…"/>` items and `<Separator/>` (or `<MenuItem Separator="True"/>`).
  It is stored in the `widget.Window.TrayMenu` **field**, not added to the widget
  tree (a `PopupMenu` as a direct `Window` child is unsafe — `Window.SetBounds`
  skips `*PopupMenu`); `window.attachTrayMenu` inserts it into the tree correctly
  at `Run()`. Give it a `Name` so code can find it (registered in the XAML
  registry under that name).

Wiring handlers stays in code — the same way `cmd/showcase` does it. Per-item
callbacks aren't expressible in XAML, so look the menu up by `Name` and use
`PopupMenu.OnSelect(idx, text)`:

```go
root, reg, _ := widget.LoadUIFromXAMLFile("app.xaml")
win := window.New(eng, "")           // Title comes from XAML
eng.SetRoot(root)
if m, ok := reg["trayMenu"].(*widget.PopupMenu); ok {
    m.OnSelect = func(idx int, text string) {
        switch text {
        case "Показать": win.RestoreFromTray()
        case "Выход":    win.Close()
        }
    }
}
win.Run()
```

**Priority: explicit code wins over the XAML declaration.** If the app calls
`win.SetTrayIcon(...)` / `win.SetTrayMenu(...)` before `Run()`, those values are
kept and the XAML `TrayIcon`/`<TrayMenu>` are **not** applied (the pickup only
fills fields the app left unset). This lets an app draw its icon programmatically
(as `cmd/showcase` does with `makeTrayIcon()`) while still loading the rest of
the UI from XAML.

The declaration flows through widget-layer fields because the `widget` package
cannot import `window` (the dependency runs `window` → `widget`):
`buildXAMLWindow` fills `Window.TrayIconImage` / `Window.TrayTooltip` /
`Window.TrayMenu`, and `window.Window.Run()` picks them up
(`pickupDeclarativeTray`) before `applyPendingTray` flushes to the OS.

**Not expressible in XAML** (do these in code): per-`MenuItem` click handlers
(use `OnSelect`), balloon notifications (`ShowBalloon`), tray click behavior
(`SetOnTrayClick`, default = double-left-click restores), and hide/restore
(`HideToTray`/`RestoreFromTray`).

---

## v3.14.0 additions (themes as data, desktop shell)

Two new packages: `theme/` (an application's looks described by data) and
`desktop/` (a system taskbar built out of that data). Both are additive — the
old `widget.Theme` presets keep working through a two-way bridge, which is why
this is v3.14.0 and not v4.

### Theme profiles (`theme/`)

A `Profile` is flat token tables; a `Theme` is those tables resolved along the
inheritance chain; a `Manager` owns the registry, the active theme and
subscriptions.

```go
p := theme.NewProfile("mytheme")
p.Parent = theme.ProfileWindows11         // inherit, override only what differs
p.SetColor("accent", theme.RGB(200, 60, 60)).
    SetMetric("taskbar.height", 44).
    SetFlag("taskbar.centered", true)
p.SetStyle("taskbutton", "", theme.StateHover, theme.StyleDelta{
    Fill: theme.C(theme.RGBA(255, 255, 255, 24)), Corner: theme.N(6),
})

m := theme.NewManager()
theme.RegisterBuiltinProfiles(m)          // Windows2000(+Blue), Windows10(+Dark),
m.RegisterTheme(p)                        // Windows11(+Dark), MacOS(+Dark)
m.SetTheme("mytheme")                     // live switch; subscribers notified
unsub := m.Subscribe(theme.ObserverFunc(func(*theme.Theme) { eng.Invalidate() }))
```

Manager API: `RegisterTheme`, `GetTheme`, `SetTheme`, `UnloadTheme` (refuses the
active theme and any theme used as a parent), `Active`, `Subscribe`,
`SetIconResolver`, and the lookups `GetStyle(component, part string, st State)`,
`GetMetric(Key) float64`, `GetFlag(Key, def bool) bool`,
`GetAnimation(Key) (AnimSpec, bool)`, `GetIcon(IconRef, size int) image.Image`.

**`GetStyle` allocates nothing** — it returns a pointer into the resolved table
and is safe to call from `Draw` (guarded by `testing.AllocsPerRun` in
`theme/theme_test.go`). Never mutate the returned `*Style`; `Clone()` it first.

Lookup order: `(component, part, state)` → `(component, part, Normal)` →
`(component, "", state)` → the theme's default style. `State` is a bitmask
(`StateHover|StatePressed|StateActive|StateDisabled|StateFocused`) collapsed by
`Dominant()` to the leading one (Disabled > Pressed > Active > Hover > Focused),
so a component keeps six entries, not thirty-two.

`Style` fields: `Fill, Text, Border, Shadow color.RGBA`, `Gradient
[]GradientStop`, `GradientAngle`, `Corner, BorderWidth, PadX, PadY, Elevation
float64`, `Font FontSpec`, `Backdrop BackdropSpec`, `Bevel *BevelSpec`.
`StyleDelta` mirrors them with pointers ("set" vs "inherited"); helpers `C(col)`,
`N(f)`, `RGB(r,g,b)`, `RGBA(r,g,b,a)` (**alpha-premultiplies**, as everywhere in
this engine).

A dark variant declares only differences — usually three or four colors:
`defaultStyle` takes fill and text from the flat `surface` and `text` tokens
whenever a style leaves them unset. Do NOT write concrete colors into styles a
dark variant must override; write the token instead.

JSON themes need no engine changes:

```go
res, err := theme.LoadTheme(reader)  // res.Profile, res.Warnings ([]string)
m.RegisterTheme(res.Profile)
```
Style keys in JSON are `"component.part:state"`; colors are `#RGB`, `#RRGGBB`,
`#RRGGBBAA` in **straight** alpha and are premultiplied on load.

### Bridge to the old presets

`widget/theme_bridge.go` + `widget/theme_bindings.go`: one binding table walked
in both directions, so they cannot drift.

```go
p := widget.ProfileFromTheme(oldTheme)     // *widget.Theme  → *theme.Profile
t := widget.Materialize(resolvedTheme)     // *theme.Theme   → *widget.Theme
m := widget.DefaultThemeManager()          // presets registered as profiles
eng.SetThemeProfile(m, theme.ProfileWindows11)
```

### Engine additions

- `Canvas.SetRoundClip(r image.Rectangle, radius int)` / `ClearRoundClip()` /
  `HasRoundClip()` — clipping along the rounded outline (per-row span
  narrowing), not just its bounding box. Honored by `fillRectPx`, `setPixelPx`,
  `drawAlphaMask`, `drawColorGlyph`.
- `Canvas.BlurBehind(r image.Rectangle, radius int, tint color.RGBA)` — acrylic
  /mica: capture → 4× downscale → separable box blur (`engine.BlurRGBA`,
  `BlurRegion`; cost independent of radius) → upscale back through `fillRectPx`,
  so it inherits clips.
- `Canvas.DrawSoftShadow(r image.Rectangle, corner, elevation int, col
  color.RGBA)` — a shadow with a smooth falloff.
- `Engine.RenderOnce() *image.RGBA` — render one frame synchronously and return
  a **copy** of the image (snapshots, golden tests, debug dumps).
- `Engine.SetThemeProfile(m *theme.Manager, name string) error`.
- `widget.InvalidateRect(r image.Rectangle)` — claim an area outside any
  widget's own bounds. Needed by overlays: `Base.Invalidate` claims the
  widget's bounds, and a flyout's bounds are its taskbar icon, while what
  changes is a window drawn elsewhere.
- **Damage is a list, not one union.** `InvalidateRect` keeps up to
  `maxDamageRects` (16) separate rectangles, absorbing nested ones and
  collapsing to a union past the limit. Drawing still happens over the union
  (the canvas clip is one rectangle); the **diff** runs per rectangle, so two
  changes in opposite corners no longer compare every tile between them.

### Desktop shell (`desktop/`)

Components never touch the system. Data arrives through consumer-implemented
interfaces — `WindowModel`, `AppCatalog`, `SystemStatus`, `Notifications`,
`Clock` (see `desktop/contract.go`) — and the engine ships fakes for all of them
(`FakeWindowModel`, `StaticAppCatalog`, `FakeSystemStatus`, `FakeNotifications`,
`FakeClock`) that tests and `cmd/desktopdemo` run on.

```go
bar := desktop.NewTaskbar(m)              // m *theme.Manager
bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
bar.AddItem(desktop.SlotApps, desktop.NewApplicationArea(m, catalog, windows))
bar.AddItem(desktop.SlotTray, tray)       // desktop.NewSystemTray(m)
bar.AddItem(desktop.SlotTray, desktop.NewClock(m, desktop.SystemClock{}))
bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
defer bar.Close()                         // unsubscribes from the theme
area := bar.ReservedArea()                // keep maximized windows out of it
```

Slots: `SlotStart`, `SlotApps`, `SlotTray`. An `Item` is a `widget.Widget` plus
`PreferredSize(avail image.Point) image.Point`. Components: `Taskbar`,
`StartButton`, `ApplicationArea`, `RunningApplications`, `SystemTray`,
`NetworkItem`, `VolumeItem`, `PowerItem`, `ClockItem`, `StartMenu`,
`CalendarFlyout`, `QuickSettings`, `NotificationCenter`.

Everything with a subscription has `Close()`; forgetting it leaves a component
waking the renderer forever (the clock ticks once a second; status icons wake on
every network/volume/power change).

Painting helpers (`desktop/paint.go`): `PaintStyle` (shadow → backdrop → fill →
bevel/border), `DrawTextCentered`, `DrawTextLeft`, `DrawTextLeftElided`, `Elide`,
`MeasureText`, `StateOf(hover, pressed, active, disabled, focused)`.

**No color literals in `Draw`.** `desktop/nocolor_test.go` parses the package
and fails on any `color.RGBA{...}` inside `Draw`/`draw*`/`paint*`. Colors come
from the theme; that is what makes one taskbar look like four different ones.

### Theme tokens that change the shell's shape

Beyond colors and sizes, these decide what the taskbar *is*:

| Token | Kind | Meaning |
|---|---|---|
| `taskbar.height` | metric | strip height |
| `taskbar.top` | flag | strip is pinned to the TOP edge (macOS menu bar); flyouts then open downwards |
| `dock.height` | metric | height of a SEPARATE bottom strip (the macOS dock); 0 = one strip only |
| `taskbar.centered` | flag | Start + apps group is centered (Windows 11) |
| `taskbar.separators` | flag | draw grip separators between sections (classic panel) |
| `startbutton.label` | flag | show the "Start" caption next to the icon |
| `startbutton.icon` | icon | the Start button glyph from the theme's icon set |
| `taskbutton.label` | flag | show window titles on task buttons (Windows 10/11 hide them) |
| `taskbutton.underline` | metric | thickness of the mark under an open window |
| `taskbutton.underline.len` | metric | mark length as a fraction of button width: 1 = full bar (Windows 10), 0.4 = short tick (Windows 11, doubled for the active window) |
| `clock.date` | flag | show the date under the time (classic clocks never do) |
| `tray.gap`, `tray.chevron.width`, `tray.overflow.columns` | metrics | tray row and its overflow grid |

The Start button icon has three levels, in order: `StartButton.Icon` (a picture
the shell sets), the theme's `startbutton.icon`, and the built-in 2×2 glyph.

macOS splits the desktop into two strips, so the shell asks the taskbar what the
theme wants and lays the same components out accordingly:

```go
if h := bar.DockHeight(); h > 0 {      // menu bar + dock
    bar.SetItems(desktop.SlotStart, startBtn)
    bar.SetItems(desktop.SlotTray, tray, clock)
    dock.SetItems(desktop.SlotApps, apps)
} else {                                // one taskbar
    bar.SetItems(desktop.SlotApps, apps)
    dock.SetItems(desktop.SlotApps)
}
if bar.Edge() == desktop.EdgeTop { /* pin to the top */ }
```

`SetItems` replaces a slot's contents; components are NOT recreated, only
reassigned — otherwise a theme switch would drop their state (hover, open
flyouts, subscriptions).

### Flyouts

`desktop/flyout.go` — the shared base for the Start menu, calendar, quick
settings and notification center. They are **engine overlays**
(`HasOverlay`/`DrawOverlay`/`OverlayBounds`), so `engine.SetPopupSink` can host
them in separate OS windows and the shell window does not clip them.

```go
menu := desktop.NewStartMenu(m, catalog)
menu.Screen = image.Rect(0, 0, w, h)      // flyout is fitted into this
startBtn.OnClick = func() { menu.Toggle(startBtn.Bounds()) }
root.AddChild(menu)                       // must be in the tree, or no overlay
```

`Flyout` fields: `Component`, `Anchor`, `Edge` (EdgeBottom/EdgeTop), `Align`
(AlignStart/Center/End), `Margin`, `Screen`, `Content`, `Size`, `OnOpen`,
`OnClose`. Methods: `Open(anchor)`, `Close()`, `Toggle(anchor)`, `IsOpen()`.
A flyout closes on a click outside and on Esc; a click **inside** is not
consumed — the content handles it.

A component that asks the theme for metrics the theme never defined gets zero
and silently never opens. `desktop/flyouts_test.go` opens all four panels under
all eight built-in themes for exactly this reason.

### Presenters: a theme changes shape, not only color

Tokens cannot express "the macOS dock": large centered icons, the hovered one
growing and pushing its neighbours. So a profile may supply a presenter —
someone else's drawing AND layout for a component it knows.

```go
p.Presenters["runningapps"] = "dock"                  // in the macOS profile
desktop.RegisterPresenter("dock", DockPresenter{})    // registered in init()
```

```go
type Presenter interface {
    Measure(c Component, avail image.Point) image.Point
    Layout(c Component, bounds image.Rectangle) []image.Rectangle
    Draw(ctx widget.DrawContext, c Component)
}
type Component interface {          // what a presenter may ask of a component
    Bounds() image.Rectangle
    Theme() *theme.Manager
    Cells() []Cell                  // Title, Icon, Active, Muted
    HoverIndex() int
}
```

`Layout` returns the cell rectangles and the component adopts them for hit
testing — otherwise clicks follow the component's own row while the user sees a
dock. A component must **never** know the active theme's name; it asks
`PresenterFor(tm, key)` and hands drawing over if there is one.

Two traps worth remembering, both found by tests here:
- `Cells()` must not be derived from the laid-out cells: presenters call it from
  `Measure`, i.e. before any layout exists (zero width → no cells → never lays
  out).
- Do not hold the component's mutex while calling a presenter: it calls
  `Cells()`/`HoverIndex()` back and deadlocks.

### Radial gradients

`Style.Gradient` is drawn by `desktop.PaintGradient` (called from `PaintStyle`
whenever the style sets it) and replaces the fill. `GradientKind` picks the
shape:

```go
p.SetStyle("dock", "", theme.StateHover, theme.StyleDelta{
    Gradient:       []theme.GradientStop{{Pos: 0, Color: c1}, {Pos: 1, Color: c2}},
    GradientKind:   theme.GK(theme.GradientRadial),
    GradientCenterX: theme.N(0.5), GradientCenterY: theme.N(0.5),
    GradientRadius: theme.N(1.1),
})
```

Center and radius are fractions of the area (0 means "middle" / "to the edge"),
so one glow fits any icon size. Renderer entry points:
`widget.DrawRadialGradient(ctx, rect, *widget.RadialGradient)` and
`widget.DrawLinearGradient`. The radial one builds a 64×64 image and lets the
engine stretch it — cheaper than per-pixel writes and smooth on fractional DPI.
Colors are alpha-premultiplied, so a transparent edge is the ZERO color, not the
same color with `A=0`; `widget.NewRadialGradient(col)` builds that pair for you.

### A theme for a subtree (`widget.ThemeScope`)

```go
scope := widget.NewThemeScope(widget.Win2000Theme())
scope.AddChild(w)                 // themed immediately
root.AddChild(scope)
eng.SetTheme(widget.DarkTheme())  // the scope keeps its own theme
```

- `ApplyThemeTree` skips any widget implementing `HasOwnTheme() bool` returning
  true, so a global theme change cannot repaint a scoped subtree.
- Colors live in widget fields (handed out by `ApplyTheme`), but SHAPE
  (`Classic3D`, corners, bevel colors) is read from a shared variable inside
  `Draw` via `currentStyle()`. `ThemeScope.Draw` swaps it for the subtree and
  restores it with `defer`; nested scopes restore the OUTER style, not the
  global one. The swap is a plain variable: a frame is drawn on one goroutine.
- `NewThemeScope(nil)` behaves as a plain container.

### Icons

`widget.BuiltinIcons()` returns an `IconSet` implementing `theme.IconResolver`
(`ResolveIcon(theme.IconRef, size int) image.Image`), with a two-level cache and
a never-nil placeholder glyph. Give it to the manager — without a resolver the
tray renders empty and nothing warns you:

```go
m.SetIconResolver(widget.BuiltinIcons())
```

---

## v3.15.0 additions (render pipeline)

### Draw contract

The engine can skip a widget subtree whose bounding rectangle does not
intersect any damage region — that subtree's `Draw` is not called for the
frame. Controlled by:

```go
eng.SetSubtreeCulling(bool)   // default: true (on)
```

Set `false` to fall back to the pre-v3.15.0 behavior (every widget's `Draw`
runs every rendered frame) for an app that can't yet meet the contract below.

Since v3.16.0 this switch, the frame's damage and the accumulated move
declarations belong to the ENGINE, not to the process. Several engines in one
process (one per window) render independently and may do so concurrently from
different goroutines; `tests/twoengines_test.go` covers this under `-race`.

Rules a widget MUST follow so culling is transparent to it:

1. **`Draw` is not called every frame.** A widget must render correctly when
   skipped — skipped means "the screen already shows the right thing."
2. **Animation only via `widget.Animate` / `widget.AnimateOwned`.** Never a
   frame counter or `time.Now()` read inside `Draw` — a skipped `Draw` call
   would stall it instead of letting it finish.
3. **`Draw` must not mutate widget state.** In particular, do not compute
   layout / hit-test rects inside `Draw` and cache them there — a skipped
   `Draw` leaves the cache stale and clicks resolve against the wrong
   coordinates.
4. **Any visual change needs `Invalidate()` (own bounds) or
   `widget.InvalidateRect(r)` (area outside own bounds — overlays, popups).**
   Without it the engine has no signal the frame is stale, and with culling
   on that means the widget never repaints again, not just late.
5. **A widget that paints outside its own bounds (`Elevation` shadow,
   overlay, popup) must claim that area itself.** The subtree's bounding
   rectangle is computed from widget `Bounds()`, not from actual paint
   extent — an unclaimed shadow/overlay area is dropped from culling
   consideration and can go stale.

See `GUIDE.md` / `GUIDE_EN.md`, section "Свой виджет" / "Custom Widget" →
"Контракт отрисовки" / "The draw contract", for the full rationale and
before/after code examples.

---

### Frame contents: what the engine now reports

```go
type Frame struct {
    Seq       uint64
    Timestamp time.Time
    Tiles     []DirtyTile   // as before: raw RGBA
    Regions   []Region      // v3.15.0: what each tile is made of
    Moves     []MoveRegion  // v3.15.0: content that merely moved
}

type Region struct {
    Rect  image.Rectangle
    Kind  RegionKind // RegionMixed | RegionSolid | RegionImage | RegionText
    Color color.RGBA // meaningful only for RegionSolid
}

type MoveRegion struct {
    From image.Point     // take from here
    Rect image.Rectangle // put here; size comes from Rect
}
```

`Regions` runs parallel to `Tiles` (same order, same length). How the mark
accumulates per tile, in the rasteriser's own loops:

- an **opaque fill covering the whole tile** wipes whatever was under it and
  makes the tile `RegionSolid` with that color;
- a fill is a *weak* mark — it is the background. Text or an image drawn over
  it becomes the tile's mark, not `RegionMixed`: what matters to the consumer
  is what is on top;
- a fill that lands over content **without covering the tile** gives
  `RegionMixed` — an honest "don't know";
- text over an image (or the reverse) is `RegionMixed`;
- marks inside the damage are reset at the start of each frame.

**Apply `Moves` BEFORE `Tiles`.** That is DXGI's order and what a consumer
expects; the reverse would overwrite fresh pixels with stale ones. A widget
declares a move with `widget.NotifyWidgetMove(w, src, dst)` — `Window` does it
while dragging and on landing. The widget names the tree the move belongs to,
which is how an engine tells its own declarations from a neighbour's: with two
engines of the same resolution in one process the coordinates are identical.
`widget.NotifyMove(src, dst)` (no widget) still works for a consumer declaring
moves itself; those are matched by canvas instead. A declared move does NOT replace damage: it is a hint
about a cheaper way to reach the same result.

Moves within one frame never overlap each other, by source or destination
(v3.16.0): the engine drops overlapping declarations and that area travels as
ordinary tiles. Apply them in any order, or all at once — the result is the
same. Without that rule a consumer whose order differed from the engine's read
a source that had not moved yet, and copied the wrong rectangle.

### Buffer format and drawing into foreign memory

```go
eng.SetPixelFormat(engine.FormatBGRX)   // rasteriser writes BGRX directly
eng.SetSurface(pix, stride, engine.FormatBGRX) // back buffer = your memory
eng.SetSurface(nil, 0, engine.FormatRGBA)      // back to the engine's own
```

Channel order is a property of the buffer, not a reason for a per-pixel loop:
an RDP consumer used to swap RGBA→BGRX itself on every frame. `FormatRGBA` is
the default and byte-for-byte what shipped before.

`SetSurface` hands the engine the consumer's own memory as the back buffer,
removing two copies of the frame. `stride` may exceed `w*4` (DIB alignment).
The **front** buffer stays internal — the diff needs a private copy of the
previous frame to compare against. Buffer rotation (`SetSurfaces` with
per-buffer damage age, the equivalent of `EGL_buffer_age`) is not implemented.

`DirtyTile.Data` follows the chosen format: with `FormatBGRX` those bytes are
BGRX, which is what the caller asked for.

### Pacing and the frame sink

```go
eng.SetFrameSink(sink)                    // sink.Present(output.Frame), synchronous
eng.SetPacing(engine.PacingExternal)      // the internal ticker starts no frames
eng.RequestFrame()                        // the sink is ready for one
```

Under `PacingExternal` the ticker still advances animations — otherwise an
animation started by the application would freeze until the next request. The
`Frames()` channel keeps working; the sink is an alternative, not a
replacement, and unlike the channel it cannot drop a frame (the channel has a
depth of 8 and discards on overflow).

Why a consumer wants this beyond vblank pacing: `Engine.Start()` runs the
render loop on its own goroutine, so a consumer that mutates the widget tree
from another goroutine races the render walk. External pacing lets it do both
on one goroutine and remove the race by construction.

### Measured cost of a frame

Desktop scene from `desktop/` at 1280×800, Windows 11 theme, fake system data
with a fixed clock; `go test ./engine/ -bench BenchmarkPipeline -benchtime=200x`
on the maintainer's machine (Intel Core Ultra 7 265K, Windows). Rerun with
`engine/pipeline_bench_test.go`.

| scenario | ns/op | B/op | allocs/op |
|---|---|---|---|
| full frame (`Invalidate`) | 3 318 098 | 28 891 | 37 |
| clock tick (60×24 rect), culling on | 77 426 | 2 697 | 7 |
| clock tick, culling **off** | 82 122 | 2 968 | 9 |
| hover over a taskbar button | 95 390 | 51 583 | 10 |
| dragging a 720×440 window | 1 854 068 | 2 038 138 | 89 |

What the numbers say, and what they do not:

- A damage-sized frame costs ~43× less than a full one. That win comes from
  `SetRenderOnDemand` + `InvalidateRect`, which predate this work.
- Subtree culling adds ~6% on top of that (77.4 µs vs 82.1 µs) and removes two
  allocations. On a flat desktop most of the tree is a handful of wide panels,
  so there is little to skip; the deeper the tree away from the damage, the
  more it saves. It is not the headline win, and the numbers say so.
- Window dragging is the expensive case (1.85 ms, 2 MB) — that is what
  `Frame.Moves` addresses: the pixels have not changed, only moved.

Repeated after tile classification, move reporting and external pacing landed
(the same command, same machine): full frame 3 278 191 ns, clock tick 75 660 ns
with culling and 84 868 ns without, hover 102 311 ns, window drag 2 007 091 ns.
Within noise of the numbers above — the marks are one assignment inside loops
that already ran, and `Frame.Moves` is filled from a list that is empty unless
something declared a move.

Repeated again after v3.16.0 (per-engine pipeline state, one bevel
implementation, gradient stop cache): full frame 3 315 877 ns, clock tick
81 758 ns with culling and 86 152 ns without, hover 100 750 ns, window drag
1 853 372 ns. Still within noise — that release moved correctness, not cost.

Two targeted measurements from it, both with `-benchmem`:

| what | before | after |
|---|---|---|
| `markTiles` on a row-by-row fill (400×300, 300 one-pixel strips) | 6 120 ns | 4 063 ns |
| `PaintGradient` (desktop) | 3 590 ns, 1 alloc / 48 B | 3 510 ns, 0 allocs |

The first one: a strip thinner than a tile can never cover one, so the
per-tile "does this cover the whole tile?" test is answered once per call
instead of once per tile. The truncated edge tile is the exception the code
accounts for — on a canvas whose height is not a multiple of 64 the last tile
row can be one pixel tall, and then a one-pixel strip does cover it.

### Occlusion (v3.16.0)

A widget may implement `widget.OpaqueRegioner` to declare what it covers
opaquely; the child walk goes top-down and skips subtrees that fall entirely
inside the accumulated area. `Window` and `Panel` implement it already.

```go
OpaqueRegion() []image.Rectangle   // absolute logical coords, like Bounds
```

Rules: no method means transparent; declare only fully-painted opaque area (a
rounded fill loses its corners, translucent/gradient/image-backed fills declare
nothing); containment is tested against ONE declared rectangle, never a union.
Over-declaring leaves a hole on screen — under-declaring only costs work.

| stack of windows, full frame 1280×800 | before | after |
|---|---|---|
| one window | 824 µs | 810 µs |
| five windows | 1 094 µs | 780 µs |
| ten windows | 1 370 µs | 830 µs |

Allocations per frame are unchanged: the occluder list is an array, and the
skip marks and the `OpaqueRegion` answer live in widget-owned buffers.

Rule worth keeping: an optimisation without a paired measurement is not
accepted. Add the before/after here.


## End of Reference

This document covers the essential API for AI code generation with headless-gui. For detailed implementation examples, refer to:
- `cmd/showcase/main.go` — example application
- `tests/` directory — unit tests with usage patterns
- GUIDE.md and GUIDE_EN.md — user documentation
