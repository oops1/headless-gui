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
  measurer back to the previous live one. An engine made with
  `engine.NewOffscreen` (printing, thumbnails, export) never takes it (v3.34).
  Use it
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
// A page of rows at once (v3.36): one CollectionAdd event, selection kept.
collection.AddRange(page) // page []any

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
| `<SplitButton>`, `<MenuButton>` | `MenuButton` | `Button` attributes + nested `<MenuItem>`; `Split`, `IsActionEnabled`, `IsMenuEnabled` |
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
| `<TreeView>` | `TreeViewWidget` | `Items`, `ItemHeight`, `ShowIndentGuides`, `SelectionMode` (Single/Extended) |
| `<Stretch>`, `<ToolBarStretch>` | `Stretch` | `Weight`; takes the free space in `StackPanel`/`ToolBar` |
| `<DataGrid>` | `DataGridWidget` | `ItemsSource`, `Columns` |
| `<SplitPanel>` | `SplitPanel` | `Orientation`, `Position`, `SplitterSize`, `MinFirst`, `MinSecond` (first two children = panes) |
| `<SVGIcon>` | `SVGIcon` | `Source`, `Color`, `Tint` |
| `<RichText>` | `RichText` | `Foreground`, `Background`, `LinkForeground`, `FontSize`, `FontFamily`, `Padding`, `LineSpacing`, `Text`, `IsReadOnly="False"`/`Editable="True"`, `AcceptsTab`; children `<Paragraph>` (`TextAlignment`, `Margin`, `Indent`, `SpaceBefore/After`) with `<Run>`, `<Bold>`, `<Italic>`, `<Underline>`, `<Span>`, `<Hyperlink NavigateUri>`, `<LineBreak/>` (see "RichText") |
| `<DiffView>` | `DiffView` | `LeftFile`, `RightFile`, `ReadOnlyLeft/Right`, `HideUnchanged`, `ContextLines`, `IgnoreWhitespace`, `SyntaxHighlight`, `ShowHeaders`, `ShowReadOnlyMark`, `WatchFiles`, `FontFamily`, `HeaderFontFamily`, `FontSize`, `SaveCommand`/`TextChangedCommand`/`DiffChangedCommand`/`FileChangedCommand` (see "v3.17 additions") |
| `<MergeView>` | `MergeView` | `OursFile`, `BaseFile`, `TheirsFile`, `ShowBase`, `ConflictStyle` (merge/diff3), `MarkerSize`, `ReadOnly`, `SyntaxHighlight`, `FontFamily`, `HeaderFontFamily`, `FontSize`, `SaveCommand`/`ResultEditedCommand`/`ResolvedCommand` |
| `<DatePicker>` | `DatePicker` | `SelectedDate`, `DisplayDateStart`, `DisplayDateEnd` (ISO 8601 or invariant M/d/yyyy), `DateFormat` (.NET pattern or Go layout), `FirstDayOfWeek`, `Placeholder`, `FontSize`, `SelectedDateChangedCommand` |
| `<DockManager>` | `DockManager` | `Background`, `NativeFloating` (see "Docking") + children `<DockPane>`×N, one `<DockContent>` |
| `<DockPane>` | `DockPane` | `Id`, `Title`, `Side` (Left/Top/Bottom/Right), `Size` (px), `MinSize` (px, min of its side), `State` (Docked/AutoHidden/Floating/Closed); valid only inside `<DockManager>` |
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

// Scrolling: virtualization has always been there, now it is observable.
dg.OnScroll = func(first, count int) { ... } // outside the lock
dg.FirstVisibleRow() int
dg.VisibleRowCount() int

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

### Resizable dialogs, separators, cell shapes, content measuring

```go
// Dialog that can be resized. Only the SetContent widget is stretched; plain
// children move with the dialog (dialogs place them absolutely).
dlg.SetResizable(true); dlg.SetMinSize(500, 400); dlg.SetContent(panel)
dlg.Resize(800, 600)

// Standalone separator; color follows Theme.Border. XAML: <Separator/>.
widget.NewSeparator(); widget.NewVerticalSeparator()
// Theme.SecondaryText — muted explanatory text, distinct from Disabled.

// DataGrid columns honour their own SetMinWidth/SetMaxWidth in both the star
// distribution and mouse resizing.
// Text cells ellipsize; template columns can use the same helper and shapes:
datagrid.EllipsizeText(cdc.DrawCtx, s, maxW, sizePt)
cdc.DrawCtx.FillEllipseAA(cx, cy, rx, ry, col)
cdc.DrawCtx.FillRoundRect(x, y, w, h, r, col)cdc.DrawCtx.FillEllipseAA(cx, cy, rx, ry, col)
cdc.DrawCtx.FillRoundRect(x, y, w, h, r, col)
// Lines and outlines (the rest of AAShapes); thickness is float64 and stays
// fractional on an antialiased canvas — only the fallback rounds it.
cdc.DrawCtx.DrawLineAA(x1, y1, x2, y2, 1.5, col)
cdc.DrawCtx.StrokePolylineAA(pts, 2, false, col)
cdc.DrawCtx.StrokeEllipseAA(cx, cy, rx, ry, 2, col)
cdc.DrawCtx.FillPolygonAA(pts, col)
// NOTE: DrawContextBridge gained those four — external implementations
// (test stubs) need them too.
dg.EmptyStateText = "No keys yet"          // or EmptyStateRenderer
// NOTE: DrawContextBridge gained FillEllipseAA/FillRoundRect — external
// implementations (test stubs) need the two methods.

// TextInput leading icon (XAML: Icon="...", IconSize="16").
inp.LeadingIcon = img

// Content measuring: DesiredSizer interface, asked only of those who implement.
sp.DesiredSize(); sp.Relayout(); sv.FitContent(); sv.ContentSize()
// DockPanel/WrapPanel re-measure children without explicit size on every layout
// (caption changed → new width; size set from code is kept). After SetText from
// code call dock.Relayout() / wrap.Relayout(); XAML trees re-lay on language change.
// A collapsed Expander asks for its header height only.

// NOTE: <Separator/> in XAML now builds *widget.Separator, not *widget.Panel.
```

### Localized titles, own font size, close veto, chromeless dialog

```go
// {Loc Key} works in Title too (Window, Dialog, DockPane) and follows
// SetLanguage live. XAML: <Window Title="{Loc app.title}">.
dlg.SetTitle("..."); pane.SetTitle("...") // a pane re-lays the manager: tab width

// Own font size in points; 0 = the global DefaultFontSizePt.
btn.FontSize = 18; chk.FontSize = 9; dd.FontSize = 14 // XAML: FontSize="18"
// A button caption that does not fit is ellipsized (the button does no clipping).

// Veto over closing: asked BEFORE a close by Escape or ✕; false keeps the
// dialog on the stack. CancelAction is skipped when the close is stopped.
dlg.OnClosing = func() bool { eng.ShowModal(askSave()); return false }
// Engine.CloseModal (an application-ordered close) does not ask the hook.

// Closed (v3.21): fires however the dialog was closed — engine, ✕, Escape, or
// together with its native window (Alt+F4). Release what the dialog held here.
dlg.OnClosed = func() { widget.ReleaseXAML(root) }
id := eng.AddOnModalClosed(func(m widget.ModalWidget) {}) // any number of subscribers
eng.RemoveOnModalClosed(id)
// SetOnModalClosed is ONE slot and is taken by the native window host — use
// AddOnModalClosed / Dialog.OnClosed in applications.

// No built-in title bar: content starts at the top, the ✕ goes away with the
// bar, drag areas replace it. Coordinates are RELATIVE to the top-left corner.
dlg.SetChromeless(true); dlg.IsChromeless()
dlg.AddDragArea(image.Rect(0, 0, 240, 48)); dlg.ClearDragAreas()
win.AddDragArea(image.Rect(0, 0, 240, 48)) // borderless window (WindowStyleNone)
// A declared area beats the widgets inside it — cut your buttons out yourself.
```

### Title bar with app controls, collapse button, content padding

```go
// A widget of the application inside the BUILT-IN title bar: laid out after the
// caption and before the ✕ / window buttons. Not stretched to the client area,
// and a press on it does not drag the window.
dlg.SetTitleBarContent(searchBox); dlg.TitleBarContentBounds()
win.SetTitleBarContent(searchBox) // same on widget.Window
// NOTE: not offered in the mac title-bar layout (buttons left, centered
// caption): bounds come back empty and the widget stays hidden there.

// Collapse button at the left of the bar. The engine keeps the state and
// notifies; collapsing the side area is the application's job.
dlg.SetNavButton(true); dlg.SetNavIcons(iconMenu, nil) // nil icon → built-in "≡"
dlg.OnNavToggle = func(collapsed bool) { nav.SetVisible(!collapsed) }
dlg.SetNavCollapsed(true); dlg.IsNavCollapsed(); dlg.HasNavButton()

// Content padding (was the private dlgPad constant): 0 = content to the edges,
// -1 = back to the default (14/12; a chromeless dialog defaults to 0).
dlg.SetContentPadding(0); dlg.ContentPadding()
// NOTE: SetChromeless now implies zero content padding — a chromeless dialog
// laid out against the old 14/12 inset will shift.
```

### Side navigation (widget.NavPanel)

```go
nav := widget.NewNavPanel()
nav.AddItem(icon, "General")     // an item is icon AND caption; nil icon → first letter
nav.OnSelect = func(i int) { ... } // user's click only, not SetSelected
nav.SetCollapsed(true)             // icon-only strip; width follows, host re-lays out
nav.ExpandedWidth, nav.CollapsedWidth, nav.ItemHeight, nav.IconSize, nav.FontSize
nav.SetSelected(i); nav.Selected(); nav.ItemRect(i); nav.ItemCount()

// Hosted column: full height INCLUDING the title bar; the caption is redrawn on
// top of the panel. ContentBounds moves right of the column.
dlg.SetNavPanel(nav); dlg.NavPanelBounds()
win.SetNavPanel(nav) // mac style: the column starts below the bar (window buttons)
// The "≡" button (SetNavButton) collapses the panel itself when one is set.
```

### Window buttons on a dialog, title-bar drag handle

```go
// All three window buttons in the dialog's bar instead of a lone ✕. The actions
// belong to the OS window: OnMinimize / OnMaximizeRestore are installed by the
// native modal host (window/modal_host.go); inert in headless.
dlg.SetWindowButtons(true); dlg.HasWindowButtons()
dlg.SetMaximized(true); dlg.IsMaximized() // switches the glyph to "restore"
dlg.OnMinimize, dlg.OnMaximizeRestore     // override only if you host the window

// The title bar keeps a 72px free strip right of SetTitleBarContent — the drag
// handle. Both Dialog and Window; TitleBarContentBounds already accounts for it.
```

### Context menus per row/node, ToolBar, tree drag, dialog localization

```go
// Menu built for the point under the cursor (ContextMenuProvider). The engine
// asks it BEFORE the plain ContextMenu field. An empty slice means "no menu
// here"; a right click does NOT change the selection.
dg.RowContextMenu = func(item interface{}, row int) []widget.MenuItem { ... }
tw.NodeContextMenu = func(it *treeview.TreeViewItem) []widget.MenuItem { ... }
dg.Grid.ItemAtRow(row)      // model item shown in a row
tw.Tree.ItemAtY(y)          // node under a Y coordinate
tw.Tree.IndexAtY(y)

// ToolBar: separators, icons-only, overflow into a chevron menu.
tb := widget.NewToolBar(); tb.AddSeparator(); tb.SetIconsOnly(true)
// XAML: <ToolBar IconsOnly="True" Overflow="True"><Separator/></ToolBar>
// NOTE: <ToolBar> now builds *widget.ToolBar, not *widget.StackPanel.

// MenuButton (v3.20): button with a drop-down menu.
pull := widget.NewSplitButton("Pull", doPull) // main part = action, arrow = menu
flow := widget.NewMenuButton("Git-Flow")      // whole button opens the menu
pull.Items = []widget.MenuItem{...}; pull.OnOpening = func() { /* rebuild Items */ }
pull.SetActionEnabled(false); pull.SetMenuEnabled(true) // parts enabled separately
pull.OpenMenu(); pull.CloseMenu(); pull.IsMenuOpen()
// Keys: ↓ opens; Enter/Space = action (split) or menu (plain). In ToolBar width
// includes the arrow, IconsOnly applies, overflow turns it into a submenu.
// XAML: <SplitButton Content="Pull"><MenuItem Header="Fetch"/></SplitButton>

// Stretch (v3.22): takes the free space in StackPanel/ToolBar — the items after
// it go to the right edge (in a column, to the bottom).
tb.AddStretch(); sp.AddStretch(); widget.NewStretchWeighted(2)
// XAML: <ToolBarStretch/> or <Stretch Weight="2"/>

// Tab strip overflow (v3.23): tabs that do not fit go into a chevron menu;
// the active tab is always shown (the strip scrolls to it).
tc.Overflow = true // default
tc.OverflowCount() // tabs hidden in the menu

// ListView reorder (v3.22): drag rows; the drag starts after a few pixels.
lv.Reorderable = true
lv.OnReorder = func(from, to int) {} // to = index AFTER the move
lv.SetSelected(i)      // now fires OnSelect (WPF-like); SetSelectedQuiet stays silent
// NOTE: *MenuButton embeds *Button but is a different type — w.(*widget.Button) misses it.

// Tree drag & drop (off by default — dragging changes what a click does).
tw.Tree.CanUserDragNodes = true
tw.Tree.CanDropNode = func(d, target *treeview.TreeViewItem, p treeview.DropPosition) bool
tw.Tree.OnNodeDrop  = func(d, target *treeview.TreeViewItem, p treeview.DropPosition) bool
// true from OnNodeDrop means "I handled it" — the tree moves nothing.
tw.Tree.MoveNode(node, target, treeview.DropInside)

// Dialog strings: override dlg.* directly, or alias them to your own keys.
widget.RegisterStrings("EN", map[string]string{"dlg.cancel": "Never mind"})
widget.AliasStrings(map[string]string{"dlg.ok": "btn.ok"})
// {Loc Key} in a DataGrid column Header now follows SetLanguage.
```

### Window icon, modal theming, secret input, tree item style

```go
// Minimum window size in LOGICAL pixels; works before Run() and at runtime,
// recomputed on a DPI change, overrides widget.Window MinWidth/MinHeight.
// Honoured by Win32 (WM_GETMINMAXINFO), X11 (WM_NORMAL_HINTS) and Wayland
// (xdg_toplevel.set_min_size); no-op on macOS.
win.SetMinSize(800, 600)
win.MinSize() (int, int)

// Window icon at runtime: Win32 WM_SETICON, X11 _NET_WM_ICON, Wayland
// xdg-toplevel-icon-v1 (v3.34, only if the compositor announces the global).
// Nearest size wins; a backend without support returns ErrIconUnsupported.
win.SetIcon(icon16, icon32)
win.SetIcon() // drop it

// ShowModal now applies the current theme to the dialog, so a widget that
// caches colors (DataGridWidget) is no longer left with the defaults.
// Consequence: colors set by hand on dialog widgets BEFORE showing are
// overwritten — exactly as already happened to a shown modal on SetTheme.
widget.CurrentTheme() *Theme // a COPY of the active theme

// Password input without ever creating a string.
secret := inp.TakeSecret() // []byte; clears the field in the same call
inp.WipeSecret()           // on cancel

// Tree item look: fields on the item, or a callback for application state.
item.Foreground = col
item.Bold = true
tv.ItemStyle = func(it *treeview.TreeViewItem) (color.RGBA, bool, bool) { ... }
// ok=false means "you decide" — the item's own fields are used then.
```

### DockPanel.LastChildFill, DockPane active title

```go
dp.LastChildFill = false   // XAML: <DockPanel LastChildFill="False">
// Without it the LAST child always fills the remainder and its own Dock is
// ignored — a row where EVERY child is docked was impossible.

mgr.ActivatePane(pane)     // switch the active pane of a stack
pane.IsActive() bool
pane.TitleTextActive = col // title color on the accent background of an
                           // active pane; zero alpha = same as inactive.
// OnStateChanged now fires on an active-pane change too (old and new pane).

// App buttons in the title bar (v3.20), left of the built-in pin/float/close:
pane.SetTitleButtons([]widget.DockPaneButton{
    {Icon: img, Tooltip: "Refresh", OnClick: fn},       // icon tinted to title text color
    {Tooltip: "View", MenuFunc: func() []widget.MenuItem { ... }}, // nil Icon → "≡", menu below
}) // fields: Icon, KeepIconColors, Tooltip, OnClick, Menu, MenuFunc, Hidden, Disabled
pane.SetTitleButtonHidden(i, true); pane.SetTitleButtonDisabled(i, true); pane.TitleButtons()
// Engine asks ToolTipAt(x, y) string before GetToolTip — per-part tooltips for any widget.

// Per-pane side minimum (v3.20): side min = max(MinSideSize, MinSize of its DOCKED panes);
// the splitter stops there; in a cramped manager it yields to the center.
pane.MinSize = 300; pane.SetMinSize(300)   // XAML: <DockPane MinSize="300">
mgr.OnSideResized = func(side widget.DockSide, size int) {} // on splitter release + after RestoreLayout
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
// Menu item icon: image, not a path (compiled-in SVG has no file on disk).
// BREAKING: MenuItem.Icon was `string` (reserved, never drawn) — now image.Image.
widget.MenuItem{Text: "Copy", Icon: img, IconSize: 14} // 0 = item height
// XAML: <MenuItem Text="Copy" Icon="icons/copy.png" IconSize="14"/>
// The icon gutter is reserved for the whole menu when any item has an icon
// (same rule as Checkable); mark left, icon next, caption last.

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
// SelectionChangedEvent.SelectedIndex (int; -1 = nothing selected)
// SelectionChangedEvent.SelectedItem (interface{})
// v3.21: fires on a programmatic change (SetSelectedIndex) and when the selected
// row is gone (collection cleared / that row removed) — selection is dropped.
// SetSelectedIndexQuiet(idx) sets it without the event (restoring after a refill).

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

// Multi-selection (v3.22): Ctrl toggles, Shift takes a range, Shift+↑/↓ extends.
tree.SelectionMode = treeview.SelectionExtended // XAML: SelectionMode="Extended"
tree.SelectedItems() []*treeview.TreeViewItem; tree.SetSelectedItems(items)
tree.IsItemSelected(item) bool
tree.OnSelectionChanged = func(e treeview.SelectionChangedEvent) {} // e.Items, e.Current
// A plain click on an already selected node keeps the set (drag / group menu).
// Highlight is drawn from TreeViewItem.IsSelected, not from one selectedItem.

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
    // size (32 entries / 16 Mpx LRU); an entry retains the source. Mutating an
    // image in place requires eng.InvalidateImageCache(src) (nil clears
    // everything) or widget.InvalidateImageCache(ctx, src). Images obey
    // SetRoundClip. For zoom/pan of big pictures use the optional
    // widget.ImageRectDrawer / widget.DrawImageRect(ctx, src, sr, dr, filter):
    // only the part under the clip is scaled (see "v3.34 additions").
    
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
    MouseButtonX1   MouseButton = 5 // side "back" (v3.34)
    MouseButtonX2   MouseButton = 6 // side "forward" (v3.34)
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

### A Child Over a Window's Title Bar Gets the Press Only If It Owns It

`Window.WantsCapture` answers true to every left press in the title bar (the window
drags itself by it), and the engine asks for a capturer from the deepest child up.
A button or label you lay over the bar without `WantsCapture` therefore never sees
the press — the window starts a drag. Either return true from your own
`WantsCapture`, or (no mouse capture needed) implement
`widget.TitleBarPressOwner.OwnsTitleBarPress(pt) bool`, or give the window
`SetTitleBarHitTest(func(pt) bool)`. Details: "Window caption" near the end of this
file.

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

**All widget callbacks are synchronous** — no widget spawns a goroutine for a
callback. And with a running engine they all run on **one goroutine, the frame
loop goroutine** (GG-68, v3.18): the native window and webstream queue input
events on the engine instead of calling `Send*` from their own goroutines, and
`eng.Post` functions and animation ticks run on that same goroutine. Shared
state touched only by handlers and `Post` functions needs no mutex.

| Widget | Mouse path | Keyboard path | Notes |
|---|---|---|---|
| `Button.OnClick` | sync | sync | Use `AddClickHandler(fn)` for multiple subscribers; OnClick (field) fires first, then handlers in registration order. |
| `CheckBox.OnChange(checked bool)` | sync | sync | The field is `OnChange`, **not** `OnClick`. Tracks tri-state press → release. |
| `ListView.OnSelect` | sync | sync | Move long-running work to a goroutine and come back with `eng.Post`. |
| `DataGrid.OnRowActivated(row, item)` | sync (after Unlock) | sync | Fires on dbl-click and Enter, even if grid is read-only. |
| `DataGrid.OnSelectionChanged` | sync | sync | |

```go
btn.OnClick = func() { /* engine goroutine */ }
btn.AddClickHandler(func() { /* runs after OnClick, same goroutine */ })

go func() {
    data := load()                            // background goroutine
    eng.Post(func() { list.SetItems(data) })  // back on the engine goroutine
}()
eng.Flush() // tests: wait until the Post queue is drained, no frame rendered
```

Caveats: without `Start()` nobody drains the queue except `RenderOnce`,
`RenderFrameNow` and `Flush` (on the calling goroutine). Code that calls
`Send*` from its own goroutine while the engine runs loses the guarantee —
deliver such input through `eng.Post` as well.

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

### Layout fixes — v3.23

Two defects that only showed on a SECOND layout pass (switching a tab, docking
a pane), so the first paint looked right:

- `ToolBar` and `DockPane` lay their content out in their own `SetBounds`, but
  were missing from `HasOwnLayout` — the parent shifted their descendants once
  more and the shift doubled. Toolbar buttons drew on top of the next heading.
- An element with no `Width`/`Height` got `Rect(Left, Top, 0, 0)`, which
  `image.Rect` normalises to `(0,0)-(Left,Top)`: its size became its own
  coordinates. A label with `Left="24"` lived in a 24px-wide box — the text
  drew outside it and was clipped away by any partial repaint.

`Label` now reports `DesiredSize()` (measured line), and Canvas asks
`desiredOf` instead of falling back to 80×30. A size given in the markup still
wins: `Label` stays silent on an axis where `Width`/`Height` is set.

### TabControl overflow — v3.23

Headers that do not fit the strip used to run off the right edge: the tab
existed but nothing could open it. They now go into a menu under a chevron at
the end of the strip (the same sign ToolBar uses).

```go
tc.Overflow = true   // default; false keeps the old "headers run off" behaviour
tc.OverflowCount()   // how many tabs did not fit
```

The active tab never disappears from the strip: if it lands past the edge, the
strip scrolls to it and the tabs that went off to the left join the menu.

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
  sources and the language listener, and forgets the tree's folded localized
  strings (menu items, tab headers, list items, column headers). Call when the
  loaded XAML tree is discarded (UI reload) to prevent leaks.
  **`widget.ReleaseXAML(root)`** does the same for a tree from any other
  `LoadUIFromXAML*` loader (those don't return the scope). Related unsubscribe APIs:
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
AncestorType`, rich-text inlines inside `TextBlock` (use `<RichText>`, see
"RichText" below), `RenderTransform`/`LayoutTransform`.

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
  Windows, X11 and Wayland backends apply it natively (macOS: no-op). An
  application-wide shape that beats every widget: `Engine.SetCursorOverride` (v3.34).
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

Since v3.5 the engine renders on demand by default (`engine.New` turns it on):
frames are skipped while the UI is unchanged — near-zero CPU on idle. The old
behaviour, a redraw every tick, is the opt-out `eng.SetRenderOnDemand(false)`:

```go
eng.SetRenderOnDemand(true)     // the default; false = redraw every tick

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
from app code in on-demand mode — the default (e.g. `label.SetText` from your own
goroutine). With `SetRenderOnDemand(false)` every tick is drawn anyway.

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

// 3. Or take Open Sans (Light/Regular/SemiBold/Bold/Italic/BoldItalic, ~0.8 MB)
//    already embedded by the engine's own package -- opt-in, so programs that
//    do not import it do not grow:
//      import "github.com/oops1/headless-gui/v3/assets/fonts"
err = fonts.Register(eng) // then eng.SetDefaultFont("OpenSans") if wanted
```

**Family + weight + italic.** Every registered font is indexed by its own
metadata (name table, `OS/2.usWeightClass`; the registration name is only a
fallback), so a font is selectable as "family, weight, italic" besides by file
name. Family names ignore case/spaces/hyphens ("Open Sans" == "OpenSans").
`widget.FontFace(family, weight, italic)` builds the name for
`DrawTextFont`/`MeasureTextFont`/`Label.FontName` (regular weight returns the
family unchanged; the DrawContext interface did not change). Matching follows
CSS Fonts (nearest weight; italic beats weight). An empty/unknown family means
"default font at that weight": Open Sans default -> `OpenSans-Bold`; built-in Go
Regular -> built-in Go Bold. In themes: `theme.FontSpec{Family, Size (may be
fractional, 8.5), Weight (theme.WeightLight/SemiBold/...), Bold, Italic}`;
`desktop.FontFaceName(spec)` converts it (desktop/paint.go uses it, so
`Bold`/`Italic` are no longer dropped). `Manager.GetFont(key)` returns named
theme fonts ("default", "caption", "title", "clock.large" in Windows 10).

**Sub-pixel text (opt-in).** By default each glyph advance is rounded to a whole
pixel (`HintingFull`; x/image has no real hinting, only that rounding), which
makes 11 px text uneven. `eng.SetTextSubpixel(true)` keeps advances fractional
and rasterizes each glyph for the quarter-pixel its pen falls in (4 phases in
the glyph cache). Off by default so existing themes and `tests/golden_*` stay
byte-identical. It changes string widths. Shaped (RTL/complex) text is unchanged.

**The profile flag is read by the engine.** The Windows 10 profile sets
`theme.FlagTextSubpixel`; `Engine.SetTheme` / `SetThemeProfile` /
`ApplyThemeProfile` read it (through `widget.ThemeStyle.TextSubpixel`, which
`widget.Materialize` fills and `ProfileFromTheme` writes back): a profile with
the flag turns the mode on, a profile without it turns off ONLY what a theme
turned on. Priority: an explicit `Engine.SetTextSubpixel(on)` of the application
(either value) wins over any theme until `Engine.UseThemeTextSubpixel()` hands
the choice back (the mode then follows the last applied profile at once).
A real switch drops cached glyph layouts and bumps the text-metrics revision
(`widget.BumpTextMetricsRev`), so widgets re-measure on the next frame. Other
profiles do not set the flag: their frames are byte-identical.
`Manager.SetFlag(theme.FlagTextSubpixel, true)` + `ApplyThemeProfile` switches it
on the fly for any profile. Tests: `tests/subpixel_theme_test.go`.

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
scope.Dispose()          // model, collections, language listener, folded {Loc} strings
widget.ReleaseXAML(root) // same, for trees from LoadUIFromXAML / WithBase / FS / WithContext
```

Call when the XAML tree is replaced (e.g., on navigation, or a dialog loaded on
every open) to prevent memory leaks: an unreleased tree is translated and
re-laid on every language change. Releasing twice or a root with nothing to
release is a no-op. `ClearLocalizedItems()` still drops the strings of ALL trees.

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
// Width follows the content (>= 380, <= 640: label, title, initial text);
// ShowInputWidth(..., width) fixes it explicitly (width <= 0 = auto, min 200).

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

### Wayland: полноценное окно — v3.24

Под Wayland окном распоряжается компоновщик: позиции клиент не знает и задать
её не может. Раньше бэкенд умел только показать окно и принять ввод — окно
нельзя было ни перетащить, ни растянуть, кнопки — и квадрат в заголовке
молчали, а панель задач видела все программы движка под одним именем
`headless-gui`.

```go
win.SetAppID("winline-calc") // xdg_toplevel.app_id, под X11 — WM_CLASS
// по умолчанию — имя исполняемого файла (было общее "headless-gui")
```

Что теперь делает бэкенд (всё — стабильный xdg-shell версии 1, без cgo):

- перетаскивание за заголовок — `xdg_toplevel.move`, ресайз за любой из
  восьми краёв — `xdg_toplevel.resize` (с serial'ом нажатия);
- кнопки заголовка — `set_minimized` / `set_maximized` / `unset_maximized`, а
  `IsMaximized` читается из состояний `configure`, поэтому кнопка
  разворачивания переключается в «восстановить» (полный экран развёрнутым не
  считается — см. «v3.34»);
- форма курсора — `wp_cursor_shape_manager_v1`, если компоновщик его объявил,
  иначе своя картинка через `wl_pointer.set_cursor`;
- кадровые буферы пересоздаются под новый размер окна.

Окно с нативным хостом умеет попросить систему двигать себя само:

```go
// widget.Window — хуки, которые ставит window.Window, если бэкенд так умеет
ww.OnNativeMove = func() bool { return true }            // одна просьба на нажатие
ww.OnNativeResize = func(edges int) bool { return true } // widget.NativeEdgeTop и т. д.
```

Бэкенд, который этого не умеет (Win32, macOS), хуки не получает (на X11 они
ставятся — `_NET_WM_MOVERESIZE`, с v3.26), и окно двигается прежним путём — `OnDragMove` плюс `SetPosition`; край окна в
нативном режиме там по-прежнему отдан системе.

### Поведение по платформам

| Возможность | Win32 | X11 | Wayland / macOS / headless |
|---|---|---|---|
| Модалка в своём окне (`ModalHost`) | нативно | нативно | in-canvas фолбэк |
| Popup-оверлей в своём окне (`PopupSink`) | нативно | нативно | Wayland — `xdg_popup`, macOS — in-canvas |
| Трей / balloon / превью | да | трей — StatusNotifierItem по D-Bus (`tray_sni_linux.go`, меню — dbusmenu), balloon — `org.freedesktop.Notifications`; превью нет | Wayland — трей и balloon как на X11 (`HideToTray` — no-op); macOS/headless — no-op (ошибка/false) |
| Перетащить / растянуть окно | `SetPosition` | `_NET_WM_MOVERESIZE` (`BeginMove`/`BeginResize`, с v3.26) | Wayland — просит компоновщик, macOS — нет |
| Свернуть / развернуть | да | да | Wayland — да (`set_minimized`/`set_maximized`) |
| Форма курсора | да | да (шрифт «cursor») | Wayland — да (`cursor_shape` или своя картинка) |
| Масштаб интерфейса (HiDPI) | да (per-monitor DPI) | да (`Xft.dpi`) | Wayland — да (fractional / `wl_output`), macOS — `HEADLESS_GUI_SCALE` |
| Незавершённый ввод (IME) | да (IMM32) | нет (XIM не делался) | Wayland — `text-input-v3`, если есть у компоновщика |
| Текст для скринридера | UIA Value и Text | AT-SPI Text | Wayland — то же, что X11 (мост общий), macOS — моста нет |
| Тема ОС (`window.DetectSystemTheme`, `SetOnSystemThemeChanged`) | `AppsUseLightTheme` + `WM_SETTINGCHANGE` | `GTK_THEME`, портал Settings (D-Bus), сигнал `SettingChanged` | Wayland — то же, что X11; macOS — всегда `SystemThemeUnknown`, смены не видно |
| Открыть ссылку / файл (`window.OpenURL`, `OpenFile`, `RevealFile`) | `ShellExecuteW`, показ — Проводник `/select` | портал `OpenURI` и `FileManager1.ShowItems`; `xdg-open` только если портала на шине нет | Wayland — то же, что X11; macOS — `/usr/bin/open`; прочие — `ErrOpenUnsupported` |
| Файлы в буфере (`widget.ClipboardSetFiles`, `ClipboardFiles`) | `CF_HDROP` + «Preferred DropEffect» | `text/uri-list`, `x-special/gnome-copied-files` (нативное окно) | Wayland — те же типы (нативное окно); macOS — пути текстом |
| HTML в буфере (`widget.SetClipboardHTML`) | «HTML Format» | `text/html` (нативное окно) | Wayland — `text/html` (нативное окно); macOS — только простой текст |
| Изображение в буфере (`widget.ClipboardSetImage`, `ClipboardImage`, v3.34) | «PNG» + `CF_DIBV5` + `CF_DIB` | `image/png` лениво, INCR для больших; читает `image/png`, затем `image/bmp` | Wayland — `image/png` лениво, читает `image/png`, затем `image/bmp`; macOS — нет (`ErrClipboardImageUnsupported`) |
| Значок окна (`Window.SetIcon`) | `WM_SETICON` | `_NET_WM_ICON` | Wayland — `xdg-toplevel-icon-v1` (v3.34), иначе `ErrIconUnsupported`; macOS — `ErrIconUnsupported` |
| Окно на передний план (`Window.Activate`, `ActivateWithToken`, `IsMinimized`, `window.TakeActivationToken`, `AllowForeground`, v3.35) | разворачивание + `SetForegroundWindow`; отказ — мигает кнопка (`FlashWindowEx`); `AllowSetForegroundWindow`; `IsIconic` | `_NET_ACTIVE_WINDOW` с меткой времени из `DESKTOP_STARTUP_ID` / последнего ввода, `_NET_STARTUP_INFO remove`; `_NET_WM_STATE_HIDDEN` / `WM_STATE` | Wayland — `xdg_activation_v1` (токен из `XDG_ACTIVATION_TOKEN` или свой), без глобала `false`; `IsMinimized` всегда `false`; macOS — `false` |
| Печать (`printing.Print`) | `PrintDlgEx` + GDI | CUPS по IPP, системного диалога нет | Wayland — то же, что X11; macOS — `ErrUnsupported`. `SavePDF` — везде |
| Несколько окон в процессе | да | да | Wayland — да, macOS — не проверялось |
| Имя для панели задач | — | `WM_CLASS` | Wayland — `app_id` |

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

### Трей, balloon-уведомления, превью (Windows; трей и balloon есть и на Linux)

Публичный API на `*window.Window`. Можно вызывать до `Run()` (состояние
буферизуется и применяется при создании окна) или из обработчиков UI. На Linux
(X11 и Wayland) трей — StatusNotifierItem по D-Bus (`window/tray_sni_linux.go`:
нужен `StatusNotifierWatcher` на шине, в GNOME — расширение AppIndicator; меню
рисует панель, одиночный клик вместо двойного), balloon —
`org.freedesktop.Notifications` без иконки в трее; превью в панели задач — только
Windows. На macOS и без нативного окна — no-op/ошибка.

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
- **Значки приложений Linux (Adwaita/Humanity/hicolor) рисуются:**
  - градиенты `fill|stroke="url(#id) [fallback]"`: `linearGradient`
    (`x1 y1 x2 y2`) и `radialGradient` (`cx cy r fx fy`), `gradientUnits`
    (objectBoundingBox по умолчанию / userSpaceOnUse), `gradientTransform`,
    `spreadMethod` (pad/reflect/repeat), `<stop offset stop-color stop-opacity>`
    (атрибутом, `style`, CSS-классом; `currentColor`), стопы и атрибуты по
    цепочке `xlink:href`/`href`. Ссылка в пустоту без запасного цвета —
    «не рисовать» (по SVG), НЕ чёрный. Один стоп = сплошной цвет;
    `pattern` см. ниже;
  - `<defs>`, `<symbol>`, `<clipPath>`, `<mask>`, `<pattern>`, `<marker>`,
    `<filter>`, `<style>` напрямую не рисуются; чужие XML-пространства
    (inkscape:, sodipodi:) пропускаются; `display:none`/`visibility:hidden`
    (атрибут, `style`, CSS) учитываются;
  - `<use>` (на фигуру/группу/`<symbol>`/вложенный `<svg>`, `x y width height
    transform`, `viewBox`+`preserveAspectRatio`), вложенный `<svg>`; защита от
    циклов и «взрыва» (`maxUseVisits`, `maxUsePoints`);
  - `clip-path` (clipPathUnits, clip-rule, цепочка clip-path на самом clipPath),
    `mask` (maskUnits/maskContentUnits, область, яркость×альфа, `mask-type`);
  - `<style>`: селекторы `tag`, `.class`, `#id`, их сочетания, `*`, списки;
    приоритет: атрибут < таблица стилей < `style=""`;
  - `<image href="data:image/png|jpeg|gif;base64,…">` (aspect, transform,
    opacity, усредняющее уменьшение);
  - `filter`: `feGaussianBlur` и `feColorMatrix` — прежним путём (покрытие и
    цвет фигуры); `feOffset`, `feFlood`, `feComposite` (over/in/out/atop/xor/
    arithmetic), `feMerge`, `feBlend` (normal/multiply/screen/darken/lighten),
    `feDropShadow`, входы `SourceGraphic`/`SourceAlpha` и именованные `result` —
    графом над слоем (`svg.FilterGraph`, `Group.Filter`; листовой фигуре с таким
    фильтром движок заводит свою группу-слой), типичная тень и «Figma»-цепочка
    с `BackgroundImageFix` совпадают с rsvg. Если в фильтре есть примитив
    вне списка (`feTurbulence`, `feMorphology`, `feDisplacementMap`…), вход
    `BackgroundImage`/`FillPaint`, ссылка на несуществующий `result` или
    `primitiveUnits="objectBoundingBox"`, фильтр пропускается ЦЕЛИКОМ — элемент
    рисуется без него (раньше оставалось «только размытие» и тень становилась
    размытой копией элемента). Цвета фильтров считаются в sRGB, область
    `x/y/width/height` фильтра не обрезает. Все именованные цвета CSS.
  - Разбор терпим к сущностям: `&nbsp;`/`&copy;` (xml.HTMLEntity), голый `&` и
    объявления `<!ENTITY>` из DOCTYPE (так Illustrator задаёт `xmlns`) не
    роняют `svg.Parse`; структура проверяется строго (оборванный файл — ошибка),
    корректные файлы разбираются побитно как раньше.
- **API-дополнения** (`Shape` и новые типы, только добавления): `Shape.FillGradient/
  StrokeGradient *Gradient` (`Fill` для градиента = `Gradient.MeanColor()`),
  `Shape.Clips []*ClipPath`, `Shape.Masks []*Mask`, `Shape.BlurX/BlurY`,
  `Shape.ColorMatrix`, `Shape.Image *Image`; `Paint.Ref/Fallback`, `PaintURL`.
  Геометрия в `Shape` по-прежнему в координатах viewBox.
- **`pattern`:** `fill|stroke="url(#id)"` — `patternUnits` (objectBoundingBox
  по умолчанию), `patternContentUnits`, `viewBox`+`preserveAspectRatio`,
  `patternTransform`, `x y width height`, цепочка `xlink:href` (атрибуты и
  содержимое). Плитка растрируется под масштаб вывода и повторяется с
  билинейной выборкой (`Shape.FillPattern/StrokePattern *Pattern`, содержимое —
  `Pattern.Shapes`). Узор без размера/габаритов — «ничего»; ссылка в пустоту —
  запасной цвет. Вложенные узоры и градиенты в узоре работают, цикл отсекается.
- **`<text>` — мост к шрифтам.** Пакет `svg` шрифтов не знает: контуры букв
  даёт интерфейс `svg.TextRasterizer` (`Outline(FontSpec, size, text)` →
  контуры с началом на базовой линии, Y вниз, и ширина). Регистрация:
  `svg.RegisterTextRasterizer(tr) uint64` / `UnregisterTextRasterizer(h)`
  (отвечает последний; `svg.ParseWith(data, svg.ParseOptions{Text: tr})` — для
  одного разбора). Движок регистрирует свою реализацию в `engine.New` и
  снимает в `Stop` (`engine/svgtext.go`: контуры из sfnt-шрифтов канваса,
  семейство/вес/наклон — по таблице семейств движка, запасные шрифты для
  отсутствующих рун). Раскладка идёт при РАЗБОРЕ: документ, разобранный до
  регистрации, останется без текста. Поддержано: `x y dx dy` (в том числе
  списки по буквам), `font-family/size/weight/style` (px/pt/em/%/ключевые
  слова), `text-anchor`, `<tspan>`/`<a>`, `xml:space`, `fill/stroke`/градиенты/
  `clip-path`/`mask` (текст — обычные фигуры; маска из текста вырезает буквы).
  Без моста текст не рисуется. Не поддержано: `letter-spacing`, `textPath`,
  `dominant-baseline`, `textLength`, `text-decoration`, `rotate`.
- **Точный режим `svg.Options`** (по умолчанию нулевой — прежний результат
  побитно, хеш-тест `TestFlatRasterUnchanged`). Выбирается при РАСТЕРИЗАЦИИ, три
  уровня: `svg.SetDefaultOptions(o)` (процесс) < `doc.SetOptions(o)` /
  `doc.UseDefaultOptions()` (документ) < `doc.RasterizeWith/RasterizeCachedWith`,
  `svg.RenderWith(doc, w, h, tint, o)` (вызов). Опции входят в ключ кэша.
  `svg.PreciseOptions` = всё сразу. Поля:
  - `StrokeJoins` — настоящая обводка (`stroke.go`): `stroke-linejoin`
    (miter + `stroke-miterlimit`, round, bevel), `stroke-linecap` (butt, round,
    square), `stroke-dasharray`/`-dashoffset` (нечётный список удваивается,
    фаза по спецификации — окружность стартует справа), точки нулевой длины
    при round/square, толщина ровно как задана (без «минимальных» 0,75 px).
    Контур обводки — единый многоугольник на подконтур (левая сторона вперёд,
    правая назад, у замкнутого — два встречных кольца), все одного направления
    обхода: `vector.Rasterizer` считает покрытие знаковой суммой, и встречные
    многоугольники вычли бы друг друга — НЕ нормализовать направление по знаку
    площади. Атрибуты разбираются всегда (`Shape.StrokeJoin/StrokeCap/
    MiterLimit/Dash/DashOffset`; поиск их в документе включается, только если
    они где-то упомянуты — плоские значки разбираются как раньше), но без
    флага не действуют;
  - `GroupLayers` — `opacity` группы слоем. Опасное место: opacity по-прежнему
    УМНОЖЕНА в `Shape.FillOpacity/StrokeOpacity` (для режима без флага), слой
    её снимает делением (`rctx.div`), поэтому в `Shape.Groups` лежит ровно по
    одной `Group{Opacity}` на каждый элемент с opacity<1 (контейнер или фигура
    с заливкой И обводкой).
  - Групповые `mask` и `filter` (блюр, `feColorMatrix`) контейнеров (g, svg, use,
    symbol, text) рисуются слоем ВСЕГДА (`layer.go`: `Group.Masks/BlurX/BlurY/
    ColorMatrix`, `Shape.Groups`, слой накладывается при смене цепочки групп —
    фигуры группы лежат в `Document.Shapes` подряд; предел `maxLayerDepth`=16).
    В `Shape.Masks/BlurX/ColorMatrix` теперь только эффекты самой листовой
    фигуры. `clip-path` по-прежнему действует на каждую фигуру (результат тот
    же). Флаг на mask/filter не влияет, плоские значки их не содержат.
- **Ограничения (честно):** нет внешних картинок, `marker`, области фильтра
  (`x/y/width/height` filter не обрезают размытие), `letter-spacing`, `textPath`;
  без `StrokeJoins` обводка — упрощённая аппроксимация (на скруглённых кривых
  виден «гребень»); без `GroupLayers` opacity группы действует на каждую фигуру
  отдельно (разница видна лишь при перекрытии полупрозрачных потомков).
  Проверка глазами: `SVG_SHEET_DIR=… SVG_SHEET_OUT=… [SVG_SHEET_REF=…]
  [SVG_SHEET_OPTS=stroke|layers|all] go test ./widget/svg -run VisualSheet -v`
  (эталоны — `rsvg-convert`); для текста (нужен движок) — то же в
  `go test ./engine -run SVGTextVisualSheet`.
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
  `text/uri-list`); **Wayland** — `wl_data_device` (`text/uri-list`; проверено
  на проводе с фальшивым компоновщиком, на живой сессии — нет); macOS — нет.
  События входа/движения/выхода и перетаскивание ИЗ окна — раздел «v3.34 additions».
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
| `taskbutton.group` | flag | several windows of one app share ONE button (Windows 10); default off = a button per window |
| `taskbutton.muted` | flag | minimised windows and pinned-not-running apps are drawn `StateDisabled` (default true; Windows 10 sets false so hover highlights them) |
| `taskbutton.underline.idle`, `.idle.len` | metric | thickness / length (fraction of the button) of the mark under a running but NOT active window; length 0 = old rule |
| `taskbutton.stack`, `.stack.width`, `.stack.gap`, `.stack.offset` | metric | "stack" edges drawn right of the icon of a multi-window button: count (0 = none), thickness, gap, distance from the icon |
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

`Flyout` fields: `Component`, `Anchor`, `Edge` (EdgeBottom/EdgeTop/EdgeLeft/EdgeRight), `Align`
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

### Vector kernels (GOEXPERIMENT=simd)

```bash
GOEXPERIMENT=simd go build ./...   # Go 1.27+, amd64; AVX2 checked at startup
HEADLESS_GUI_NOSIMD=1 ./app        # force the scalar path without rebuilding
```

`internal/pixsimd` holds the hot pixel loops (mask×color blend, translucent fill,
RGBA↔BGRA swap) with AVX2 versions behind
`//go:build goexperiment.simd && amd64 && go1.27`. The `go1.27` term matters:
the `archsimd` API changed between 1.26 and 1.27, so on Go 1.26 with the
experiment the vector files must drop out (scalar path) rather than fail to
compile. Keep that term on every new vector file and on tests that reference
`selectVector`/`avx2`. `go.mod` stays at `go 1.22` — the core has no 1.27
dependency.
Output is bit-identical to the scalar path (golden frames pass in both builds);
the full frame is ~39 % faster. Rules when touching it: keep the scalar version
as the reference, prove every vector change with the "vector = scalar" tests
(`go test` with `GOEXPERIMENT=simd`), and end every vector loop with
`archsimd.ClearAVXUpperBits()` — without VZEROUPPER the SSE code that follows
(including runtime map lookups) pays a false-dependency penalty.

Kernels (`kernels` struct) are selected as one set in `selectVector`: AVX2
(CPUID + OS YMM support), no `HEADLESS_GUI_NOSIMD`, and `selfTest` passes —
each kernel vs. generic on canned inputs, panics recovered as failures. The
reason for staying scalar is in `pixsimd.Fallback()`. Box blur lives here too
(`BoxBlurRows`/`BoxBlurCols` with a reusable `BlurBuf`): the scalar path
divides by the window via an exact ⌈2⁴⁰/d⌉ reciprocal, AVX2 via ⌈2²⁴/d⌉ for
windows ≤ 256 (exhaustive tests `TestDividerExact`, `TestRecip24Exact`); the
horizontal pass transposes 8×8 blocks. A new kernel must be added to both the
`generic` and `avx2` sets and to `selfTest`.

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

### Context menu from the theme profile — v3.32

`widget.PopupMenu` reads its look from the profile itself (WinLine remarks
after v3.31.0, "Контекстное меню"): styles of the `menu` component and the
metrics `theme.KeyMenu*` (`theme/menu.go`). A profile that declares none of
them leaves the menu exactly as it was; golden frames of all flat presets are
byte-identical.

| Token | Meaning |
|---|---|
| style `menu` | `Fill`, `Text`, `Border`, `Corner` (menu rounding), `Shadow` + `Elevation` (soft shadow) |
| style `menu.item` Hover | `Fill` (plate, translucent allowed), `Text`, `Corner` (plate rounding) |
| style `menu.item` Disabled | `Text` |
| style `menu.separator` | `Fill` — line colour (else the border colour) |
| style `menu.shortcut` | `Text` — shortcut colour (else the text colour muted by 0.45) |
| `menu.item.height`, `menu.separator.height`, `menu.icon.size`, `menu.icon.gap` | row, separator, icon side, icon-to-label gap |
| `menu.pad.x` (+ `menu.pad.left`, `menu.pad.right`) | side margins; left = before check/icon, right = after the shortcut |
| `menu.pad.y` | gap between the border and the first/last row |
| `menu.item.inset` | hover plate inset from the menu edge; a declared 0 = flush to the border (min 1) |
| `menu.separator.inset` | separator line inset |
| `menu.width.min`, `menu.submenu.width.min` | min width of the ROOT menu; submenus have no minimum unless the second is set |
| `menu.chevron.right`, `menu.chevron.size` | thin `›` chevron drawn as a shape; the number is the distance from the right edge to its centre |
| `menu.submenu.delay` (ms) | open/close delay of a submenu on hover (click and keys do not wait) |
| flag `menu.icon.tint` | icons are monochrome and drawn in the item's text colour |

Built-in values: Windows 11 (24/8, pad 12, icon 16, inset 4, corner 8/4,
chevron 18, min 290, hover `RGBA(0,0,0,9)`), Windows 10 (same, square, plate
across the width), Windows 2000 (20/7, pad 10, submenu delay 400). macOS
already declared `Corner`/`Elevation`, so its menu is now rounded with a soft
shadow. Windows 11 Dark overrides colours only.

```go
// Where the menu gets the profile
m := widget.NewPopupMenu()          // follows the theme the engine applied
                                    // (ThemeStyle.Menu, set by Materialize)
themeMenu / m.ApplyMenuStyle(widget.MenuStyleFromTheme(tm.Active())) // explicit

// New API
widget.MenuStyle                    // what a profile declared (zero = nothing)
widget.MenuStyleFromTheme(rt)       // read it from a resolved theme
(*PopupMenu).ApplyMenuStyle(ms)     // colours: only declared; sizes: reset then declared
widget.ThemeStyle.Menu              // carried by Materialize / ProfileFromTheme
widget.SetPopupWorkArea(r)          // popups stay inside r (screen minus taskbar)
(*PopupMenu).WorkArea, .MaxHeight   // per-menu override / height limit
widget.MenuItem{IconHover, IconTint}
// PopupMenu fields filled by the profile (zero = old behaviour): PadLeft,
// PadRight, PaddingY, ItemInset, SeparatorInset, CornerRadius, ItemCorner,
// Elevation, ShortcutColor, IconSize, IconGap, ChevronRight, ChevronSize,
// TintIcons, SubMenuMinWidth (0 = inherit, <0 = none), SubMenuDelay.
```

Behaviour changes that apply whenever the fields are set:

- The hover plate, border and separator are blended (`FillRectAlpha`,
  `FillRoundRect` with A<255), so a translucent plate (Windows 11 `(0,0,0,9)`)
  is a film, not black.
- `ApplyTheme` no longer resets the height to 30/22 for a profile that
  declared it (`t.Style.Menu`); a profile that stops declaring a value gives the
  default back.
- Shadow: `Elevation > 0` + `ShadowDrawer` -> `DrawSoftShadow(rect, corner,
  Elevation, ShadowColor)`, otherwise the old 2 px offset rectangle. All of it
  is in `PopupMenu.drawShadow`: the shared theme shadow tokens
  (`ShadowBlur/ShadowOffset/ShadowOpacity`) plug in there instead of
  `Elevation`/`ShadowColor`. The shadow is not part of `OverlayBounds`: it
  would turn clicks next to the menu into clicks on it, and a hosted native
  popup window clips it (as it always clipped the offset rectangle).
- Long lists: a menu taller than the work area (or `MaxHeight`) gets that
  height and scrolls — wheel, a click on the arrow bands at both ends, hover
  on a band (repeats every 50 ms), Up/Down keep the highlighted row visible.
  There are no columns.
- The work area: `widget.SetPopupWorkArea(bar.WorkArea())` (or
  `PopupMenu.WorkArea`) keeps menus and submenus off the taskbar drawn on the
  same canvas; empty = the whole canvas; ignored when popups are hosted.
- `SubMenuDelay` is a timer built on `widget.Animate` (advance it with
  `widget.StepAnimations(now)` in tests); while a switch is pending only the
  hovered row carries the plate.
- Icons: `MenuItem.IconTint` (or `PopupMenu.TintIcons`, flag
  `menu.icon.tint`) repaints a monochrome icon in the item's text colour of
  the current state; `IconHover` replaces the picture under the plate.

`ThemeStyle.Menu` keeps only what the flat theme cannot carry (sizes, corners,
`Elevation`, text/border/shadow/shortcut/separator colours): fill, plate and
disabled colours stay in `MenuBG`/`MenuHoverBG`/`MenuHoverText`/`Disabled`, so
the six presets round-trip unchanged and a preset's control corner is NOT read
as a menu corner (a profile built from a flat theme declares none; use the
metrics `menu.corner`/`menu.item.corner` to state one).

### A lagging channel consumer, and frames of another size — v3.31

Frames are differences against the previous frame. The channel used to drop a
frame silently on overflow while the engine's `front` already matched it, so
every following frame was a difference against a picture the consumer never
got. A window lagging during a resize kept the old layout forever (WinLine
calculator: 1920-wide keys in an 851-wide window after maximize/restore).

Now (`engine/fullframe.go`):

- a drop sets `lostFrame`; the next frame rendered while the channel has room
  is FULL — all tiles via `Canvas.allTiles()`, no diff — and the ticker renders
  it even when nothing changed (`fullFramePending`);
- `SetResolution` / `SetScale` drain the channel (frames of the old size are
  useless) and mark the next frame full;
- `output.Frame.Width`, `Height` — canvas size (physical px) the frame was
  taken for; zero = unknown (not produced by the engine);
- the window's `applyFrame` skips a frame whose size differs from the canvas
  its buffer was made for, under the same lock that `resizeTo` uses to swap
  the buffer; in `FitScale` it compares with the canvas, not the letterboxed
  buffer;
- a consumer that never reads the channel (sink-only) pays nothing: its
  channel never has room, so no full frames are rendered.

Tests: `engine/fullframe_test.go` — a consumer 100 ms slower than the engine,
resizes 851→1920→851 and scale 1→2→1 with mouse activity; after settling its
buffer equals `front` byte for byte. Without the fix 60–270 KB differ.

### Pointer leaving the window — v3.31

`wl_pointer.leave` (main surface or popup) and X11 `LeaveNotify` now deliver a
move to (-1,-1), as Win32 already did on `WM_NCMOUSELEAVE`: hover and tooltips
clear the same way as when the mouse crosses the edge. X11 adds `LeaveWindow`
to the event mask; `NotifyInferior` and `NotifyUngrab` are ignored
(`window/pointerleave.go`).

### Rounded window corners on Wayland — v3.31

A top-level with `CornerRadius > 0` gets an ARGB8888 shm buffer; after the
usual conversion the four corners are masked (`window/cornermask.go`): outside
the arc (0,0,0,0), on it a premultiplied partial alpha. `set_opaque_region`
covers the window minus the corners. A maximized or fullscreen window is not
rounded and fully opaque; windows without a radius stay XRGB. X11 and Win32 are
unchanged (X11 would need an ARGB visual and a compositing manager).

### Accent, light taskbar and Windows 10 acrylic — v3.31

(The `taskbar.light` flag also switches the `menu` style, see "App buttons".)

**Accent is a live token** (`theme/accent.go`). `Manager.SetAccent(c)`
re-resolves the active theme with another `accent` and notifies subscribers
exactly as `SetTheme` does (`Taskbar` repaints without being recreated); the
choice survives `SetTheme` and `RegisterTheme`. `ResetAccent()` returns the
profile's accent; `Accent()` reads the effective one;
`Engine.SetAccent(m, c)` / `Engine.ApplyThemeProfile(m)` (`engine/themeapply.go`)
also re-materialise the legacy `widget.Theme`. Setting the same colour again
does nothing.

- Derived tokens are computed from the base: `accent.hover` (+12 % white),
  `accent.pressed` (+15 % black), `accent.dark` (+25 % black), `accent.light`
  (+30 % white), `accent.text` (white, black when the accent is light).
  `theme.DeriveAccent(c) AccentSet` exposes the arithmetic. A value the profile
  declares itself wins until `SetAccent`; after it everything is recomputed.
- `selection` defaults to the accent when a profile does not declare it.
- Styles follow the token **by reference**: `StyleDelta.FillFrom / TextFrom /
  BorderFrom Key` (JSON: `"fill": "@accent.hover"`). Resolved against the final
  merged tokens of the chain, so a child profile may swap the token without
  rewriting styles. A literal in a LATER delta (a child's) overrides an earlier
  reference; within one delta the reference wins; a missing token leaves the
  colour alone. Windows 10/11/macOS profiles use references for every accent
  fill, border and text-on-accent. Windows 2000 follows `SetAccent` too (since
  the v3.32 work): caption (`FillFrom accent`), menu / Start / slider / tile /
  calendar highlights (`FillFrom selection`, text `accent.text`); `selection`
  is no longer declared there, so it equals the accent, and without `SetAccent`
  every style is bit-identical (`theme/classic_accent_test.go` hashes them).
  `Windows2000 Blue` declares `selection` itself (it was navy, not the accent),
  so only its caption follows.

**Flags override live too.** `Manager.SetFlag(k, v)` / `ResetFlag(k)` re-resolve
like `SetAccent`. A flag is useful because of conditional styles:
`Profile.SetStyleWhen(flag, component, part, state, delta)` is applied on top of
the profile's ordinary rules only while the flag is true
(`Profile.Conditional`, `ConditionalStyle`).

**Component aliases.** `Profile.SetStyleBase("notificationcenter",
"notifications")` makes a new component name start from every rule of the old
one (own rules win), so new names add no visual change to existing themes.
All base profiles declare `notificationcenter → notifications`; the flat
`desktop.NotificationCenter` still uses `"notifications"`.

**Acrylic.** `BackdropSpec` gained `Noise float64` (grain amplitude as a share
of full scale, 0.02 = ±2 %) and `Fallback color.RGBA` (opaque colour used when
the draw context cannot blur). `desktop.PaintStyle`: blur → tint → noise
(`widget.NoiseDrawer`, implemented by `Canvas.DrawNoise(r, amount)` — a
deterministic per-pixel hash, clip-aware, keeps the premultiplied invariant);
no `BackdropDrawer` → `Fallback` if set, else the old translucent tint. JSON:
`"noise"`, `"fallback"` in `backdrop`.

**Rotated text.** `widget.RotatedTextDrawer` (optional context interface, like
`BackdropDrawer`): `DrawTextRotated(text, x, y, sizePt, fontName, angle, col)
bool` draws the string as `DrawTextFont` would at `(x, y)` rotated around that
point by 90 (reads bottom-up; `(x, y)` is the bottom-left corner of the label),
270 (top-down; top-right corner), 180 or 0. `engine.Canvas` lays the string out
normally (shaping, fallbacks, kerning) into a physical-size scratch raster and
permutes the glyph alpha mask by pixels — no resampling, so at 200 % the glyphs
are as sharp as horizontal text. `translatingContext` (popup windows) and
`widget.OffsetContext`/ScrollView forward it; a wrapper over a context without
rotation returns `false` (nothing drawn). Width: `MeasureTextFont`; line height:
`FontMetrics`/`MeasureUIFontMetrics`.

**Dialog in built-in profiles.** The profiles declare `dialog.titlebar` (copy of
the focused `window.titlebar`: navy with white text on Windows 2000, the blue
variant its own) and `dialog.scrim` (black, alpha 90 classic / 110 others;
`theme/dialogstyles.go`, added by each `…Profile()` constructor). Before, both
fell back to `surface`: the scrim was opaque grey and hid the desktop and the
title was white on grey. Declared styles are never overwritten.

**Windows 10 profile.** The taskbar is acrylic (`win10Acrylic`: radius 20, tint
`RGBA(31,31,31,210)`, noise 0.02, fallback `RGB(31,31,31)` — the previous
solid colour). Hover/pressed/active on the bar are white films instead of grey
plates (same shade on a solid bar). Flag `theme.KeyTaskbarLight`
(`"taskbar.light"`, default false = as before) switches the bar, its items and
the new shell panels to a light palette via conditional styles. Windows 11,
Windows 2000 and macOS render byte-identical to v3.31 (checked on rendered
frames of all eight profiles).

New style PARTS for the Windows 10 shell (`theme/profiles_win10_shell.go`;
they do not exist for other themes, existing flat panels are untouched):

| component | parts |
| --- | --- |
| `startmenu` | `panel`, `sidebar` (acrylic); `sidebar.item`, `row`, `letter` (Normal/Hover/Pressed/Active/Focused/Disabled); `tile` (accent fill, hover frame); `tile.group` |
| `notificationcenter` | `panel` (acrylic), `header`, `link` (accent text), `group`, `card`, `action`, `quick.tile` (off), `quick.tile.on` (accent) |

Parts already drop the inherited border/shadow/elevation (`flat`). A presenter
asks `GetStyle("startmenu", "row", state)` and never branches on a theme name.

Tests: `theme/accent_test.go`, `desktop/accent_test.go`, `engine/noise_test.go`.
`TestGolden_Windows10Acrylic` writes dark and light bar PNGs when `GOLDEN_OUT`
is set.

### Backdrop materials, shadow tokens, reduced motion (next minor)

New public API only; every built-in profile renders byte-identically without
the new flags (checked on 48 frames: 8 profiles × start/quick/notify/calendar/
menu/window — `desktop/snapshots_test.go`, `SNAP_OUT=dir` writes PNGs and
`sums.txt` to diff before/after).

**Materials.** `theme.BackdropMaterial` (`MaterialDefault` = old behaviour by
`Mode`, `MaterialSolid`, `MaterialAcrylic`, `MaterialMica`, `MaterialMicaAlt`),
`BackdropSpec.Material`, JSON `backdrop.material` (`solid|acrylic|mica|mica-alt`).
Painting is one function: `widget.PaintMaterial(ctx, rect, corner, spec) bool`
(false = no material named, caller paints by `Mode`); `desktop.PaintStyle` and
`widget.Window` use it. Context capabilities: `widget.MicaDrawer`
(`MicaBehind(r, radius, tint) bool`, implemented by `engine.Canvas` and
`translatingContext`) and `widget.ShadowParamDrawer`. No `MicaDrawer`, or it
returns false (no wallpaper) → solid `Fallback` (else `Tint`).

*Mica = blurred WALLPAPER, not content.* `engine/mica.go`: wallpaper
(`Canvas.bgImage`, or `Engine.SetWallpaperSource(img)` which overrides it; nil
clears) is box-averaged by 16, blurred (3 passes) once per (wallpaper, radius)
and cached; a layer bilinearly stretches the small image into its rect
(`Src` write, honours clip and round clip) and lays `Tint` over it. The result is
a pure function of (pixel position, wallpaper), so partial redraw == full
redraw and `backdropdamage.go` needs no change. Popup overlay buffers
(`cloneForSize` clones) get `wallParent` = the main canvas and a shift through
`translatingContext.MicaBehind`. `SetResolution/SetScale/SetPixelFormat` rebuild
the cache. Radius ≤ 0 → 80 logical px.

*Profile.* `theme.FlagBackdropMica` (`backdrop.mica`), `FlagBackdropMicaAlt`
(`backdrop.mica.alt`, wins when both), `FlagShadowSoft` (`shadow.soft`). Conditional
styles in `theme/profiles_win11_material.go` (`declareWin11Materials`, called at
the END of `Windows11Profile`): panels `startmenu quicksettings notifications
notificationcenter calendar window`. Colours are tokens, so `Windows11Dark` only
adds two: `surface.alt` (`KeySurfaceAlt`) and `shadow.color` (`KeyShadowColor`);
styles reference them through the new `StyleDelta.BackdropFrom` (Fallback and the
Tint colour from a token, Tint alpha stays declared) and `StyleDelta.ShadowFrom`.
**Parts do not inherit**: every part of those components declared before the call
gets `Backdrop{}` / `ShadowBlur 0` under the flags (a tile painted by
`PaintStyle` would otherwise get its own Mica); a part declared after must do the
same. Mica is OFF by default in all profiles.

**Shadow tokens.** `Style.ShadowBlur`, `ShadowOffsetX`, `ShadowOffsetY`,
`ShadowOpacity` (+ `StyleDelta` fields, JSON `shadow_*`). `Style.ExplicitShadow()`
(tokens only), `Style.ResolveShadow() (theme.ShadowSpec, bool)` (tokens, else the
old `Elevation`/`Shadow` formula: blur = elevation, offset Y = elevation/2),
`ShadowSpec.Extent()` (reserve in px). Engine: `Canvas.DrawShadow(r, corner,
blur, offX, offY, col)`; `DrawSoftShadow` now calls it (byte-identical).
`widget.DrawShadowSpec(ctx, r, corner, spec)` draws via `ShadowParamDrawer`, else
approximates through `DrawSoftShadow`. Consumers: `PaintStyle`,
`Flyout.motionRegion` (reserve), `widget.Window` (`SetShadow`, tokens of style
`window` via `ThemeStyle.WindowShadow`, `DrawMargin()` for culling,
`SetBounds` invalidates the shadow ring), `widget.Dialog` (`ThemeStyle.DialogShadow`;
without tokens the old strips). `ThemeStyle.MenuShadow/WindowShadow/DialogShadow`
are filled by `Materialize` only when the profile declares tokens.

*How `PopupMenu` should take the tokens* (not wired here — its file belongs to
another change): in `Draw`, `if sp, ok := widget.MenuShadow(); ok {
DrawShadowSpec(ctx, menuRect, corner, sp) } else { /* legacy offset rect */ }`,
and add `sp.Extent()` to the overlay bounds / invalidation margin. Windows 11
declares the tokens on style `menu` under `FlagShadowSoft`.

**Reduced motion.** `theme.FlagMotionReduce` (`motion.reduce`).
`Manager.GetAnimation` is the single point (`theme/motion.go`): decorative
tokens → duration 0, functional (`notification.expand`, `RegisterAnimationKind`)
→ 0 or ≤ metric `motion.reduce.functional_ms`. `Manager.GetAnimationRaw`,
`MotionReduced`, `Theme.AnimReduced`. Consumers needing no change: Flyout,
`Tween`, task button transitions, `notification.expand` (without its own token
the `menu.open` duration is shortened as functional), Start menu. Fixed:
`Taskbar` auto-hide treated a zero token as "absent" and fell back to 140 ms.
Widgets with hard-coded durations go through `widget.MotionDur(d)` (toggle knob,
dialog fade, progress value, thin scrollbar fade/expand, DiffView scroll, dock
flyout, `AnimateFloat/AnimateRect`) fed by `widget.SetReduceMotion(bool)`;
timers and scroll inertia are untouched. `Engine.ApplyThemeProfile` /
`SetThemeProfile` copy the flag to the widgets; `Engine.SetMotionReduce(m, on)`
does flag + apply.

**One canvas, one DPI.** `Engine.SetScale(k)` is per canvas; `desktop.Monitor`
has logical bounds in one space and no scale. Two monitors with different DPI in
one canvas share one scale. Different scales need one `engine.Engine` (and OS
window) per monitor; the bar-per-monitor logic and the scale per monitor are the
consumer's.

Tests: `theme/materials_test.go`, `engine/mica_test.go`,
`engine/window_shadow_test.go`, `widget/backdrop_material_test.go`,
`desktop/reducemotion_test.go`, `desktop/materials_visual_test.go`
(`MAT_OUT=dir` writes Solid/Mica/MicaAlt/soft-shadow PNGs).

### Windows 11 taskbar — `desktop/taskpill.go`, `taskbar_align.go`, `iconbutton.go`, `taskview.go`, `widgetsbutton.go`, `traygroup.go`, `theme/profiles_win11_taskbar.go` (next minor)

Public API is additive; Windows 10, Windows 2000 and macOS render byte-identical
frames (`TestTaskbar_OtherProfilesFrozen`: sha256 of the frames of 7 profile variants
× 100 %/200 %, taken BEFORE the work; `TestOverlay_OtherProfilesIgnoreModelFields`).
Everything below is switched on by Windows 11 TOKENS — a component never asks for a
theme name; a theme without the tokens gets the old drawing.

**Geometry (100 %, plan §2).** Bar 48; Start, search, Task View and window buttons
40×40 (metric `taskbar.item.height` 40 for items that ask for no height,
`taskbutton.height` 40 for the app area, centred in the bar), gap 4, icon 24,
highlight corner 4. Metrics: `taskbutton.icon.size`, `taskbutton.width`.

**Alignment on the fly.** The centred group is `SlotStart` + `SlotApps` (put Start,
search and Task View in `SlotStart`, the window buttons in `SlotApps`). Two live
ways: `Manager.SetFlag("taskbar.centered", v)` (the bar is a theme observer) or
`Taskbar.SetAlignment(desktop.BarAlignLeft|BarAlignCenter)` / `ResetAlignment()` /
`Alignment()` (an explicit choice beats the flag, survives theme switches). Items are
re-laid out, not recreated, and only the bar strip is invalidated
(`TestWin11Bar_AlignmentDamagesOnlyBar`). New slot `desktop.SlotWidgets` (value after
`SlotTray`, so old values are unchanged) — the far left edge (top of a side bar)
whatever the alignment; the centred group never goes under it. Tab order is
`slotOrder` = widgets, start, apps, tray.

**Widgets button** — `desktop.NewWidgetsButton(tm)`, put in `SlotWidgets`. Data from
the consumer: `SetContent(WidgetsContent{Icon|IconAt, Temperature, Caption, ToolTip})`
(an empty content is the plain widgets glyph); the width follows the text
(`widgets.width.max` caps it, text is elided) and the bar re-lays out by itself
(`SetRelayout`). Metrics `widgets.icon.size/text.gap/width.max`, icon `widgets.icon`,
style component `widgets` (parts `temperature`, `caption`), string `desktop.widgets`.

**Task View button** — `desktop.NewTaskViewButton(tm)`: plate + glyph (built-in SVG,
theme icon `taskview.icon`, or `Icon/IconAt`), states hover/pressed/"open"
(`SetActive`, `Track(src)`, `TrackManager(mgr, name)`), focus ring, `OnClick` (what
it opens is the consumer's), tooltip `desktop.taskview`. Style `taskview`, metric
`taskview.icon.size`. `widgets`, `taskview` and `tray.group` share `iconButton`
(`iconbutton.go`): press arms, release over fires, Enter/Space = click (no auto-repeat),
ring only for the keyboard.

**Search.** New mode `SearchModeIconAndLabel` (value after `SearchModeBox`): magnifier
+ the word "Search" on the field plate, width `search.label.width`. Metric
`search.height` (32; Box and IconAndLabel only, 0 = bar decides), flag
`search.hint.short` (empty field shows "Search" instead of the long Windows 10 hint).
Windows 11 declares the field through profile styles `searchbox` (`""`, `hint`,
`icon`): `FillFrom field.fill`, corner 16, 1 px border from `border`, accent border
on focus/open. Without text the icon modes are buttons: Enter/Space open the search
(like a click), arrows/Home/End move focus along the slot (in the field they still move
the caret); a printable character still goes to the query. This keyboard behaviour also
applies to Windows 10 `SearchModeIconOnly` (before, Enter called `OnSubmit("")`, Space
typed a blank, arrows did nothing).

**Tray.** `desktop.NewTrayGroup(tm, items...)` — "network + volume + power" as ONE
button: one plate (style `tray.group`, metrics `tray.group.pad/gap/height`), one
`OnClick`, one Tab stop, one hover for the whole group. Members are not tree children
(the group draws them and takes mouse/keys); `ToolTipAt` returns the tooltip of the
member under the pointer; `Close()` closes members; a zero-width member (no battery)
takes no room. `Track/TrackManager/SetActive` as for the buttons. Metric
`tray.item.height` (40) is the plate height of tray icons when the theme does not fill
the whole strip (`tray.fill.strip`); the glyph stays centred. Windows 11 tray icons
have `PadX` 4 (step 24; group = 3·24 + 2·4 = 80).

**Bell and Do Not Disturb.** `NotificationButton.SetDoNotDisturb(bool)` /
`DoNotDisturb()`; a notification source may implement the optional
`desktop.DoNotDisturbReporter{ DoNotDisturb() bool }` and then wins over the manual
flag (read on every draw; the source's `Subscribe` already repaints the button).
**This is the taskbar agent's own DND model — one bool, no events**; the notification
centre agent introduces its own model, to be unified when merging. DND: tooltip
`desktop.tray.dnd`, bell crossed out, counter hidden; theme icon key
`tray.notifications.icon.dnd` is tried first. Flag `tray.bell` draws a bell (outline
SVG, `taskglyph.go`) with a round counter badge (`tray.badge.size`, part `badge` of
`tray.notifications`) instead of the old speech bubble; it also makes the button a Tab
stop (`TabIndex() == -1` without the flag, so classic themes get no new stop). The
button is `Focusable` now (Enter/Space = click).

**Window buttons — indicators from the window model.** `desktop.WindowInfo` got
`ProgressState` (`ProgressNone/Normal/Paused/Error`), `Progress` (0..1), `Badge` (int,
>99 shows "99+") and `Attention` (ignored for the active window). A stack (`group`
mode) shows the worst progress (error > pause > normal, the largest share inside that
state), the SUM of the counters and attention of any non-active window
(`overlayOf`). Theme gating (nothing is drawn unless the metric is set):
- **Pill** (`taskbutton.pill.height` 3): running = `taskbutton.pill.idle` (6) wide in
  the text colour at `taskbutton.pill.idle.opacity` 0.6, active = `.active` (16) in the
  accent; width and colour glide between them (`AnimTaskPill`, 150 ms out-cubic;
  `numMotion`), centred under the button at `taskbutton.pill.offset` (-2 = below the
  plate, at the bar edge). A cell seen for the first time stands at its target at once
  (no growth from nothing on the first frame). Styles: part `pill`
  Normal (`FillFrom text`) and Active (`FillFrom accent`). Replaces `drawTaskMarkAt`
  (the Windows 10 mark) only where the metric is set; `RunningApplications` keeps the old
  mark, the indicators live in `ApplicationArea`.
- **Progress** (`taskbutton.progress.height` 4): track the width of the icon at the
  icon's bottom edge + bar by the share; parts `progress`, `progress.fill` (accent),
  `progress.paused` (yellow), `progress.error` (red).
- **Badge** (`taskbutton.badge.size` 14): accent circle (oval for 2-3 chars) at the
  top-right corner of the icon, clamped inside the button; part `badge` (`Text`,
  `Font.Size`, `PadX`).
- **Attention** (flag `taskbutton.attention`): the plate (part `attention`) blinks
  `taskbutton.attention.blinks` (3) times, one blink = `AnimTaskAttention` (450 ms,
  linear), then stays lit until the window is activated or the flag is cleared.
- `motion.reduce`: pill width changes at once, attention is steady at once, no
  animation is registered (`TestWin11Pill_ReducedMotionIsInstant`,
  `TestWin11Attention_BlinksThenSteady`).
- **Partial repaint.** `ApplicationArea.refresh` compares old and new cells; when only
  overlays changed (`overlayChanges`, `sameLook`) it invalidates just those buttons'
  rectangles (a download percent does not repaint the area); anything else repaints the
  area. Every animation step repaints its own button only
  (`TestWin11Pill_AnimatesAndRepaintsOnlyItsButtons`,
  `TestWin11Overlay_RepaintsOnlyChangedButton`).

**Profile.** `theme/profiles_win11_taskbar.go` (`declareWin11Taskbar`, called from
`Windows11Profile` after `inheritTrayStyles`); `profiles.go` got one call and the dark
profile three tokens. New colour tokens `film.hover`, `film.pressed`, `field.fill`
(Windows11Dark overrides only these; its two hover styles were removed — it declares 13
own tokens, limit 15, `TestBuiltinProfiles_DarkVariantsAreThin`). Tray icon colour is
`TextFrom text` (it was the light theme's black literal — black icons on the dark bar
before). Other palette needs go through tokens too.

Built-in glyphs (`taskglyph.go`): `glyphTaskView`, `glyphWidgets`, `glyphBell`,
`glyphBellFilled`, `glyphSlash` — added to the Start-menu glyph table at package init.

Tests: `desktop/win11shell_test.go` (scene, geometry, alignment, theme/accent live),
`win11shell_indicators_test.go` (pill, progress, badge, attention, partial repaint, other
profiles), `win11shell_items_test.go` (keyboard, tray group, bell, widgets, Task View,
search, scales 100-200 %), `taskbar_frozen_test.go`. Pictures:
`GOLDEN_OUT=dir go test ./desktop -run TestWin11Bar_Shots` (light/dark × centred/left, search
modes, DND bell, mid-blink attention; `*_x2.png` at 200 %).

Not done: indicators in `RunningApplications` and in the side-bar column; the Windows 11
"widgets after Task View" placement for left alignment (the widgets slot stays at the far
left); indeterminate progress; badge on the Start button; the clock plate is not 40 high.

### Physical-size SVG, tray icons from the theme set, thin scrollbar — v3.31

- `widget/physical.go`: `ContextScale(ctx)` (1 for contexts without `Scale()`),
  `PhysicalRect(ctx, r)` (edge-rounded like `Canvas.sRect`), `DrawSVG(ctx, doc,
  rect, current, tint)` rasterises at physical size and hands the engine an image
  of exactly the receiver's physical size. `Canvas.DrawImageScaled` blits an
  `*image.RGBA` of that size as is (no resample, no cache copy);
  `engine/popupcontext.go` forwards `Scale()` so popups see the scale too.
  `SVGIcon.Draw` uses it when the scale is not 1 (scale 1 path unchanged).
- `desktop/trayglyph.go`: network/volume/power ask `tm.GetIcon(key, physicalSide)`
  with keys from most specific to general (`tray.network.wifi.N`, `tray.volume.N`,
  `tray.power.ac.N`, …, `*.icon`); no key declared → the old shapes, pixel for
  pixel. Flag `tray.icon.tint` recolours by the alpha channel.
  `TrayIcon` (image / SVG / `func(size) image.Image`), `NotificationButton`
  (counter, `99+`), `ShowDesktopButton` (`tray.showdesktop.width`).
  `theme/profiles_tray.go` copies the `tray.volume` styles to the new components in
  every built-in profile (otherwise the default surface fill shows through);
  conditional rules (`taskbar.light`) are copied too (unless the profile declared
  the same flag+key itself).
- **Windows 10 tray (`theme/profiles_win10_tray.go`)**: `taskbar.gap` 0, Start
  button 48 (`PadX` 16, icon 16), tray step 24 (`PadX` 4), notification button 40
  (`PadX` 12), show-desktop strip 5 px + 1 px left line (`tray.showdesktop.line`,
  colour = part `line` of `tray.showdesktop`). Flag `tray.fill.strip`
  (`desktop.KeyTrayFillStrip`): `Taskbar`/`SystemTray` stretch `stripFiller` items
  (network, volume, power, `TrayIcon`, `NotificationButton`) to the full strip
  height (`stretchToStrip`); `trayInner` keeps the glyph at icon size, centred.
  Vertical bars are untouched. `NotificationButton.SetActive / Active / Track /
  TrackManager` — lit (`StateActive`) while the centre is open.
- **Tray strings**: keys `desktop.tray.showDesktop|notificationCenter|noNotifications`
  and `desktop.tray.notifications.{one,few,many,other}` (`desktop.PluralForm`:
  RU/UK/BE 1·2-4·5+, PL, CS/SK, default one/other). Legacy keys (`ShowDesktop`…)
  are aliases: an app override of a legacy key (value differs from the built-in)
  is honoured unless the new key was overridden too. All tray tooltips use `tr()`
  with `DefaultLanguage`.
- **Optical icon sizes**: `theme.IconRef.Sizes` (string `"16 20 24"`,
  `theme.IconSizes`) + `{size}` in `Source`; `IconRef.SourceFor(size)` /
  `theme.OpticalSize` choose the smallest size >= requested else the largest;
  `IconSet.ResolveIcon` uses it (requested size is physical). JSON `sizes`.
  `AppInfo.IconAt(size)` + `IconFor`; `appicon.go:drawAppIcon` feeds it the
  physical side. Strings (`ShowDesktop`, `NotificationCenter`,
  `NoNewNotifications`, `NewNotificationsCount`) are `widget.Tr` keys registered
  for EN/RU in `traystrings.go`.
- `widget/scrollbar_thin.go`: `ScrollbarThin` is an overlay (`reserve()==0`),
  hidden at rest (`alpha`), shown by `thinPoke()` on mouse move / any scroll,
  hidden by a single pause timer that re-arms itself for the remainder instead of
  being recreated per event. `animateQuiet` is `AnimateOwned` without the full
  `notifyUIChanged` (that wakes `Engine.Invalidate` = whole frame); steps repaint
  only the bar column (`stripW`/`horiz` atomics so a tick never takes `sv.mu`).
  Lock order `sv.mu → thin.mu`, never the reverse. A hidden bar is not
  `interactive()` and neither captures nor hit-tests the mouse. Theme fields
  `ThemeStyle.Scrollbar*` ⇄ profile tokens `scrollbar.*` (declared only when set,
  so presets round-trip unchanged); instance setters win over the theme.

### Windows key — `widget.KeyWin`

`KeyWin` (0x5B, VK_LWIN) is the Windows/Super key; the right one (VK_RWIN, 0x5C)
is folded into the same code. Win32 passes both VKs through `vkToKeyCode`;
X11/Wayland map the physical keycodes 133/134 (`x11VKTable`), and fall back to
the keysym (`Super_L`/`Super_R`: `keysymToVK` for the X11 keymap,
`xkbKeymap.vkFor` for the Wayland keymap) when Super was moved to another key.
The key is also a modifier: `surface.keyEventRepeat` keeps `modMeta`, so events
while Win is held carry `ModMeta`, the `KeyWin` press itself too, and Win in the
middle of an Alt tap cancels the menu gesture. XAML `Key="Win"` (also `LWin`,
`RWin`, `Super`) parses to `KeyWin`. Wayland does not auto-repeat it.

### Desktop localization — `desktop/locale.go`

Strings of `desktop/` are `widget.Tr` keys `desktop.*` (`StrStart`,
`StrStartPinned`, `StrStartAllApps`, `StrNotifEmpty`, `StrNotifClearAll`,
tray tooltips `StrNet*`, `StrSound*`, `StrPower*`) with RU and EN tables
registered in `init`. A consumer overrides a key with `RegisterStrings`, adds a
language, or points the keys at its own table with `widget.AliasStrings`.
Components read the string at draw time, so `widget.SetLanguage` (which redraws
the whole frame) changes the text without re-creating anything; `Taskbar`
listens to the language and re-lays its items out (clock width depends on the
format); tray tooltips (`trayTooltip.build`) rebuild themselves on a language
change, a tooltip set by `SetToolTip` is left alone.

Until the application calls `widget.SetLanguage` (`widget.LanguageExplicit()` is
false) the desktop stays Russian as before: `desktop.DefaultLanguage = "RU"`.

Clock, calendar and notification times take their rules from a
`desktop.DateCulture` (`ClockItem.Culture`, `CalendarFlyout.Culture`,
`NotificationCenter.Culture`; nil = `LocaleCulture`): month names (nominative
and genitive), short weekday names, first day of the week, time/date layouts and
the long date (`{d} {M} {y}` template). `LocaleCulture` reads the same
`date.month.N`, `date.wd.N`, `date.firstDay` keys as `widget.DatePicker`, plus
`desktop.clock.timeFormat`, `desktop.clock.dateFormat`, `desktop.cal.longDate`,
`desktop.cal.monthGen.N`. A consumer embeds `LocaleCulture` and overrides what
differs (ru-KZ). `ClockItem.TimeFormat/DateFormat` still win over the culture.
`NotificationCenter.EmptyText` is now empty by default (use `EmptyLabel()`).

### Keyboard focus on the taskbar — `desktop/focus*.go`

Every taskbar item is a `widget.Focusable`: `StartButton`, `ApplicationArea`,
`RunningApplications`, `NetworkItem`, `VolumeItem`, `PowerItem`, `TrayLabel`,
`ClockItem` (the last two only when `OnClick` is set), and a hidden child
`trayChevronButton` for the overflow chevron. A custom item embeds
`desktop.FocusState` and writes `SetFocused`, `TabIndex` (return
`-1` when it has no bounds — `CollectFocusables` ignores visibility),
`OnKeyEvent` (call `HandleKey` or, for a many-cell item, `HandleCellKey`) and
optionally `FocusRing`. The search box of the Windows 10 start slot only needs
to be a focusable `Item` in `SlotStart`; nothing else is required for it to take
part in Tab and the arrow keys.

- Tab order is the slot order: `Taskbar.Children()` returns items by slot
  (start, apps, tray), not by insertion.
- Arrows/Home/End move inside one slot through `FocusNavigator`
  (`Taskbar.MoveFocus`, wired by `AddItem` via `FocusNavigable`) and do not wrap;
  in the application area they move the selected cell (`FocusState.Cell`).
- Enter and Space activate (not on `Repeat`, not with Ctrl/Alt/Meta).
- The ring is painted by `Taskbar.drawFocus` over the children
  (`PaintFocusRing`): an outer outline in the text colour of the item's style (or
  the `focus.ring` colour token, `focus.ring.width` metric, default 1) and a
  1 px inner outline of the contrasting black/white, so it shows on any
  background in light and dark themes. It is shown only for keyboard focus:
  `FocusState.NotePointer` (called from `OnMouseButton`) hides it after a click.
  `StateFocused` is also passed to `StateOf` by the start button and the app
  cells, so a profile can style it.

### App buttons — window groups, command menu, tooltip, Start "active" (`desktop/appgroup.go`, `previewlist.go`)

With `taskbutton.group` (Windows 10 profile) `ApplicationArea` shows all windows
of one app (`AppID != ""`) as ONE cell; `WindowsAt(i)` lists them, the cell is
active if any window is, minimised only if all are. The grouping follows a theme
switch by itself (`syncGrouping` in `layout`/`PreferredSize`). The cell key for
colour transitions is the app, not the window.

- **Click on a stack.** `WindowPreview.Track(area)` also registers
  `GroupClickArea.SetGroupListener`; a click opens the window LIST at once
  (hover opens it after `preview.delay.open`). The list is the same `Flyout`:
  icon, title, thumbnail and a hover `×` (closes the window) per window; more
  windows than fit in a row (max 7, never wider than the room next to the bar:
  the whole `Screen` at a bottom/top bar, the part of it beyond the button at a
  side bar) become a column of titles without thumbnails. It follows the window
  model while open (`ListedWindows()`). Without a listener (no preview) a click
  and Enter/Space cycle the windows. Optional interfaces:
  `GroupHoverArea.WindowsAt`, `GroupClickArea.SetGroupListener`,
  `GroupKeyArea.SetGroupKeyListener` — an area that does not know them keeps
  working as a one-window area.
- **Keyboard control of the list.** Enter/Space on a stack opens the list through
  `GroupKeyArea` and the list TAKES keyboard focus (`internal/focusreq`: the
  widget asks the engine for focus during the key delivery, and gives it back
  when it closes, whatever closed it). `WindowPreview` is `Focusable` with
  `TabIndex() = -1`. Left/Up and Right/Down move the selection (wrapping),
  Home/End jump to the ends, Enter/Space raise the window, Delete closes it (the
  selection stays at the closed window's place; one window left closes the list),
  Esc closes the list (the engine's `EscapeDismisser` path) and focus returns to
  the stack button. The selected window is outlined with the two-colour focus ring
  (`PaintFocusRing`, 2 px outside the item); `ListSelection() (idx, keyboard)`
  tells the selection and whether the ring is on. The mouse over the list takes
  the selection over (ring off); the mouse leaving does not close a list that holds
  the keyboard; losing focus (Tab) closes it by a timer (NOT directly: the engine
  calls `SetFocused` under its focus lock). A list opened by mouse never takes
  focus. Tests: `desktop/previewkeys_test.go`.
- **Right click / Menu key / Shift+F10** → `PopupMenu` from `AppCommands`
  (`area.SetCommands(...)`, `AppCommandsFunc`); the consumer gets an `AppButton`
  (`App`, `Title`, `Pinned`, `Windows`) and returns `[]AppCommand` (title, icon,
  disabled, separator, `Run`). Default set: `DefaultAppCommands(cat, wm, btn)` —
  launch, pin/unpin, close window / close all; captions are `widget.Tr` keys
  `desktop.app.pin|unpin|closeWindow|closeAll` (RU+EN). The menu opens at the
  button edge facing the desktop (`taskbar.top` flips it; at a side bar it opens
  beside the button: right of it for `EdgeLeft`, left for `EdgeRight`) and takes
  colours from the theme's `menu` styles (`themeMenu`: fill, text, border,
  hover, disabled text — a style the theme does not declare keeps the widget
  default). The Windows 10 profile declares `menu` for the dark panel and, under
  the `taskbar.light` flag, a light one (`F2F2F2`, black text, `CCCCCC` border,
  hover `DEDEDE`), so the command menu and the right-click menu of the Start menu
  follow the panel mode. The mode may change while the menu is open: the area
  re-themes the menu on every overlay draw, nothing is recreated. Tests:
  `desktop/appmenu_light_test.go`. The area draws and routes the menu itself
  (`HasOverlay/DrawOverlay/OverlayBounds/Dismiss/DismissOnEscape`); the menu is
  NOT a child (a `PopupMenu` is `Focusable` and would become a Tab stop).
- **Tooltip.** `ApplicationArea.ToolTipAt` returns the app title (with the window
  count for a stack, key `desktop.app.windows`) only where no preview will show:
  pinned-not-running buttons, or when nothing tracks the area / the theme has
  `preview` off / the model has no `WindowPreviews`. Tray icons already carry
  tooltips (`NetworkItem`, `VolumeItem`, `PowerItem`, `TrayIcon.SetToolTip`).
- **Marks.** `drawTaskMark`: active = `taskbutton.underline` × `.underline.len`
  (full width in Windows 10, accent via `BorderFrom`); running non-active =
  `.underline.idle` × `.underline.idle.len` in the TEXT colour; pinned-not-running
  = none. Hover is a rectangular film (`Corner 0`) also on minimised / pinned
  buttons (`taskbutton.muted=false`). Other themes keep `DrawUnderline` as is.
  In a column of a side bar the same marks stand on the cell side facing the
  screen edge (left bar: left side, right bar: right side), length measured along
  the cell height (`taskmark_edge.go`: `drawTaskMarkAt`, `markEdgeOf`).
- **Start button.** `StartButton.SetActive`/`Active` + `Track(OpenStateSource)` /
  `TrackManager(mgr, name)`: lit (`StateActive`) between the open and close
  events of any `*Flyout` (or manager panel), whoever closes it. Windows 10 has
  the film, Windows 2000 a sunken bevel, Windows 11 the pressed film.
- Windows 10 geometry: button 48 (`taskbutton.icon.size` 24 + `PadX` 12 each side),
  `taskbutton.gap` 0. `ClockItem.GetToolTip` defaults to the long date.
- Windows 10 preview panel styles (`preview`, `preview.header`, `preview.thumb`)
  are acrylic in dark and light (`theme/profiles_win10_taskbutton.go`).

Tests: `desktop/appbuttons_test.go`; `TestGolden_Windows10AppButtons` writes PNGs
with `GOLDEN_OUT`.

### Edges and monitors — `desktop/taskbar_vertical.go`, `screens.go`, `screenbars.go`, `flyoutedge.go`

`desktop.Edge` gained `EdgeLeft` and `EdgeRight` (`Edge.Vertical()`). The bottom
and top layouts are untouched (rendered frames of all built-in profiles are
byte-identical to before); the side layout is a separate code path.

- **Side bar.** `Taskbar.SetEdge(e)` / `ResetEdge()` set the edge explicitly (the
  theme only knows `taskbar.top`); `Edge()` returns the explicit one, else the
  theme's. At a side edge the slots lie in a column: Start on top, apps in the
  middle (squeezed proportionally), tray and clock at the bottom.
  `Thickness()` = metric `taskbar.width` (declared by Windows 2000 and 10: 62),
  falling back to `taskbar.height`. Auto-hide (`autohide.go`) slides along the
  bar's own axis and the reveal band lies on the left/right edge.
- **`VerticalItem`** (`Item` + `SetVertical(bool)`): an item that can lie in a
  column. In a column `PreferredSize(avail)` gets `avail.X` = thickness and
  returns X across / Y along. Implemented by `ApplicationArea` (icon-only cells
  top to bottom, icon centred across the bar, no dock presenter),
  `RunningApplications` (the same column of squares; before it was a row squeezed
  into one square), `SystemTray` (icon grid, hidden icons behind the chevron at
  the bottom) and `ClockItem` (full thickness; the date is dropped if it does not
  fit the width). **`EdgeAware`** (`SetBarEdge(Edge)`, optional, next to
  `VerticalItem`): the bar tells an item which screen edge it stands at when the
  item is added and on every relayout; `ApplicationArea` and
  `RunningApplications` use it to put the running/active mark on the edge side. An item that does not implement it gets the full
  thickness across and the height it asks for a square avail, capped at the
  thickness (this is how Start and tray icons lie).
- **Flyouts at a side edge.** `Flyout.Edge = EdgeLeft/EdgeRight` opens the window
  to the right/left of the anchor; `Align` then works vertically (`AlignStart` =
  window top at icon top, `AlignEnd` = window bottom at icon bottom; `fitInto`
  keeps it on screen). `SlideAuto` slides from the bar's side; the motion region
  stops at the bar's edge so the window grows out of it. The window list of a
  stack (`WindowPreview`, bound with `BindFlyouts`) opens beside the bar too, and
  its row of thumbnails is limited by the room between the button and the far
  screen edge (`listSpace`): where three thumbnails do not fit it turns into a
  column of titles instead of being pushed under the bar. Tests:
  `desktop/sidestack_test.go`, `sidestack_edge_test.go` (PNGs with `GOLDEN_OUT`:
  `win10_side_list_*.png`).
- **Monitors.** `desktop.Monitor{ID, Name, Bounds, WorkArea, Primary}`,
  `Monitor.Work()`, consumer interface `Screens{Monitors(); Subscribe(func()) func()}`,
  `FakeScreens`. `Taskbar.DockTo(m)` puts the bar at its edge of monitor `m` and
  returns the rectangle; `Monitor()`, `WorkArea()` (monitor work area minus the
  bar). `Taskbar.BindFlyouts(panels...)` hands `Edge` and the monitor (`Screen` =
  bounds, work area without the bar) to the panels and keeps them in sync on every
  edge/theme/monitor change; `UnbindFlyouts`, `BoundFlyouts`. A theme switch that
  changes edge or thickness re-docks the bar by itself.
- **`Flyout.SetMonitor(m)` / `Monitor()`** — screen of the panel.
  **`Flyout.PinToEdge(e)` / `Unpin()` / `Pinned()`** — press the window against an
  edge of the monitor's work area instead of the anchor (the notification center
  at the right edge of its monitor: `center.PinToEdge(desktop.EdgeRight)`; `Align`
  chooses top/bottom/middle along the edge, `Margin` is the gap from the edge,
  `SlideAuto` slides from that edge). The anchor is still used for `Toggle` /
  `DismissAt`. **`SetPinMargins(edge, panel)` / `PinMargins()`** split the gap:
  `edge` to the screen edge, `panel` to the sides the taskbar cut out of the work
  area (without it both are `Margin`) — the Windows 10 centre sits flush with the
  screen edge yet keeps a gap above the taskbar.
- **Flyout close**: `Close` remembers `restRect` (`closeRest`); every animation step
  and the last one (presence 0) invalidate `paintedRegion()` = `dirtyRect()` plus
  that rect (shadow padding, clipped to the screen), so no 9 % trace of the panel
  stays for a consumer that assembles frames from damage.
- **`ScreenBars`** (a widget; put it in the root instead of the bars and the
  `FlyoutManager`): `NewScreenBars(screens, func(Monitor) *MonitorShell)` asks the
  consumer for `MonitorShell{Bar, Flyouts map[string]FlyoutPanel}` of every new
  monitor, docks and binds it, registers the flyouts in ONE shared
  `FlyoutManager` (names `"<monitorID>/<name>"`, so one panel is open on the
  whole desktop), re-docks on `Sync()` and on `Screens` changes (`Post` field —
  `Engine.Post` — marshals the change to the UI thread), removes shells of gone
  monitors (`OnShellRemoved`, then flyouts closed and `Bar.Close()`). `Bars()`,
  `Shell(id)`, `BarAt(pt)`, `Flyout(id, name)`, `Toggle(id, name, anchor)`,
  `Flyouts()`, `Close()`.

Tests: `desktop/edges_test.go`, `edges_items_test.go`, `flyoutedge_test.go`;
`GOLDEN_OUT=<dir> go test ./desktop -run "TestEdges_Shots|TestScreens"` writes the
PNGs of every edge and of two monitors.

### Default font size follows the theme

`widget.DefaultFontSizePt` stays a constant (10 pt), but widgets that have no
size of their own (window titles, tabs, menu items, labels, `Canvas.DrawText`)
now read `widget.DefaultFontSize()`. It is 10 pt until the app calls
`widget.SetDefaultFontSize(pt)` (0 = back to 10; an explicit value beats the
theme) or a theme asks for it: a profile with the flag
`theme.FlagFontDefaultGlobal` (`"font.default.global"`) makes `Fonts["default"].Size`
the default (`ThemeStyle.DefaultFontSize`, applied by `ApplyGlobalTheme`, i.e. by
`Engine.SetTheme`, `SetThemeProfile` and `ApplyThemeProfile`); switching to a
profile without the flag restores 10 pt. Only the Windows 10 profiles set the flag
(8.5 pt); Windows 11/2000/macOS declare 9 pt in `Fonts["default"]` but do not ask,
so applying them changes nothing. Any profile can opt in on the fly:
`m.SetFlag(theme.FlagFontDefaultGlobal, true)` + `Engine.ApplyThemeProfile(m)`.
The size changes widths and line heights: set it before building the UI (a built
UI is repainted, but sizes already computed are kept until the next layout).
`desktop/` falls back to it as well when a style has no font size.
Tests: `tests/defaultfont_theme_test.go`.

### Notification center of Windows 10 — `desktop/notifyview*.go`, `notificationcenter_win10.go`

`desktop.NotificationCenter` stays one component; a profile picks its look through
a presenter (`Profile.Presenters["notificationcenter"] = theme.NotificationCenterPresenter`,
set by the Windows 10 profile and inherited by its dark variant). The component
asks `PresenterFor(tm, "notificationcenter")`, never the theme name. Windows 2000
and macOS keep the flat list (rendered frames are byte-identical to before;
checked on 4 themes x 4 panels). Windows 11 has its own presenter since the
next minor (see "Notification center and calendar of Windows 11" below); the
flat tests of the repo run under `flatNotifTheme` — Windows 11 without the
centre and calendar presenters.

Rich variant, all sizes are metrics `notificationcenter.*` (`theme/profiles_win10_notify.go`):
the panel is `width` (396) wide, glued to the right screen edge and spans from the
top of the work area to the taskbar (`Flyout.Place` hook: height = anchor top minus
`margin`, which is 0 here; other flyouts keep `Flyout.Margin` 6). Set
`NotificationCenter.WorkArea` when the anchor is a small tray icon: the anchor is
used as the bar edge otherwise, so pass a rectangle that spans the bar height.
Layout (top to bottom): header link "Manage notifications" (`OnManage`, the panel
closes first) / scrolling list of groups (icon, name, collapse, close group) with
cards / footer links "Expand|Collapse" (left) and "Clear notifications" (right) /
quick action tiles (4 columns, one row collapsed). Cards: icon, title (theme font
`title`), body (2 lines, 8 expanded, ellipsis), time (`caption`), hover cross,
chevron, actions. Theme parts of component `notificationcenter`: `panel`, `toast`,
`header`, `link`, `group`, `card`, `action`, `field`, `glyph`, `dim`, `quick.tile`,
`quick.tile.on` (accent), `scrollbar`; they follow `SetAccent` and `taskbar.light`
live.

Model additions (all additive): `Notification{AppName, Icon, IconAt, Timestamp,
Actions}` (`At()` = `Timestamp` or `Time`), `NotificationAction{ID, Kind, Title,
Placeholder, Options, Selected, Keep}` with kinds Button / Link / Reply (field +
send button) / Select (dropdown), `NotificationActionEvent{Notification, Action,
Kind, Value, Index, Inputs}` (Action "" = the card itself was pressed; `Inputs` carries
current values of fields, so a button sends the chosen dropdown value). Delivery:
`NotificationCenter.OnAction` and/or the optional source interface
`NotificationActions.InvokeNotificationAction`. A button/link/reply removes the
notification unless `Keep`; pressing the card removes it and closes the center.
Quick actions: `QuickAction{ID, Title, Icon, IconAt, On, Disabled}`,
`QuickActionModel{List, Toggle, Subscribe}`, ready `QuickActionList`;
`nc.SetQuickActions(m)`, `SetQuickExpanded`, `SetGroupCollapsed`. A change of
`On`/`Disabled`/`Title` of tiles in an unchanged set repaints only those tiles
(a partial repaint is pixel-equal to a full one: tested).

Fixed errors: the center subscribes to its sources on open and unsubscribes on any
close (button, outside click, Esc, `DismissAt`) so the list updates after reopen;
an open center repaints on a new notification; height no longer depends on the
number of cards (list scrolls: wheel, keys, the thin auto-hiding thumb).

Scrollbar: the thumb is dragged with the mouse. Pressing it grabs the mouse
(`NotificationCenter` is `widget.CaptureAware`, so the drag keeps working outside
the panel), the list moves proportionally to the thumb (`dy * maxScroll / free
track`, the grab point does not jump), the thumb never fades while held and is
drawn in the `scrollbar` part state `Pressed` (`Hover` under the cursor). A click
on the track above/below the thumb scrolls one page (90 % of the list height) in
that direction. Only the list area is repainted while dragging.

Expand/collapse animations: the text of a card (2 lines <-> up to 8, actions), a
whole group and the quick action grid change height smoothly, the cards below
slide with them. Duration and curve come from the theme animation token
`notification.expand` (`desktop.AnimNotificationExpand`; Windows 10 declares
150 ms `out-cubic`; a profile that does not declare it falls back to `menu.open`;
0 = instant, no frames at all). The view keeps only a `k` (0..1 openness) per
card/group/grid (`notifyview_slide.go`); while `k` is on the way the layout takes
the height between the collapsed and the expanded layout and clips the expanded
content by the card frame. The motion starts on demand: the layout notices that
the target state changed (click, key, `SetGroupCollapsed`, `SetQuickExpanded`),
so no setter has to be wired. An item seen for the first time (panel just
opened, new notification) is drawn in its state at once; closing the panel drops
all motions. An interrupted motion goes back from the current height without a
jump. Each step repaints only the rectangle from the top of the moving element to
the bottom of the list (the grid: from the list top to the panel bottom); the
header is not repainted. `k` is clamped to 0..1, so an `out-back` fallback cannot
turn the height inside out.

Severity: `Notification.Severity` Warning/Error is drawn on a Windows 10 card as
a strip of `notificationcenter.severity.width` (3 px) at the left edge, in the
colour of the theme part `severity.warning` / `severity.error` (dark panel:
yellow/pink, light panel: dark amber/red); Info gets nothing. The toast has the
same strip. Flat themes keep the old `card.warning` / `card.error` border.

Keyboard: the open rich center is `Focusable` + `TabAcceptor`; Tab/Shift+Tab,
Up/Down walk stops (link, groups, cards, actions, reply field, footer links, tiles),
arrows walk the tile grid, Left/Right collapse/expand a card or group, Enter/Space
activate, Delete dismisses a card or group, PgUp/PgDn scroll, typing goes into the
reply field (Backspace, Delete, arrows, Home/End, Ctrl+V, Enter sends), Esc closes
the dropdown first, then the center. Focus ring: `PaintFocusRing`, keyboard only.
Strings are `desktop.notif.*` (`notifystrings.go`); to reuse a catalog map them with
`widget.AliasStrings(map[string]string{desktop.StrNotifManage: "ManageNotifications",
desktop.StrNotifClear: "ClearAllNotifications", desktop.StrNotifExpand: "Expand",
desktop.StrNotifCollapse: "Collapse"})`. Time on a card: today -> `TimeFormat`,
yesterday -> "Yesterday", older -> `DateFormat` of the `DateCulture`; `nc.Clock` fixes "now".

Toast: `desktop.NewNotificationToast(tm, ns)` shows each NEW notification (those in
the source at creation are ignored) as the same card in the bottom right corner above
the tray (`Anchor`/`Screen` or `WorkArea`), slides in from the right, hides after
`Timeout` (default metric `notificationcenter.toast.timeout`, 5000 ms), stays while the
mouse is over it, cross hides only the toast, actions work as in the center. It is
NOT registered in `FlyoutManager` (opening it must not close the Start menu) and is
added to the root after the manager; `toast.Suppress(center)` keeps it quiet while
the center is open; only themes with the presenter show it. In the flat themes
(Windows 2000, macOS) the toast is intentionally NOT shown: there is
no rich card to pop up (a shell that needs a pop-up there can use the tray
balloon, see "Tray, balloon notifications"). It is a limitation of the flat
variant, not a bug.

Hooks added to `Flyout` (nil = old behaviour): `Place` (own placement) and `Plate`
(component and part of the plate style).

Tests: `desktop/notifyview_test.go` (behaviour, errors, keyboard, toast),
`desktop/notifyview_render_test.go` (pixels: accent, light flag, partial repaint,
open area), `desktop/notifyview_slide_test.go` (expand/collapse animation, thumb
drag, severity), `theme/profiles_win10_notify_test.go`,
`theme/profiles_win10_notify_severity_test.go`. Pictures: `NC_OUT=<dir> go test
./desktop -run TestVisual_NotificationCenter` and `...Toast` (also
`dark_card_expanding`, `dark_group_collapsing`, `dark_quick_expanding`,
`*_scrollbar_drag`, `*_severity`). A test that toggles a card, a group or the grid
finishes the animation first (`finishAnimations()`), because the theme animates.

### Notification center and calendar of Windows 11 — `desktop/notifyview_w11.go`, `notificationcenter_win11.go`, `calendarflyout_win11.go` (next minor)

Third presenter of `desktop.NotificationCenter` (next to flat and Windows 10) and a
Windows 11 look of `desktop.CalendarFlyout` with a "Focus" module. Public API is
additions only; Windows 10, Windows 2000 and macOS frames are unchanged (checked
with `SNAP_OUT` before/after: only `Windows11*__notify` and `Windows11*__calendar`
differ; flat `__notify` hashes of the base commit itself differ run to run, the raw
pixels of an isolated frame are identical).

**Choosing the look.** Profile `Windows11Profile` sets
`Presenters["notificationcenter"] = theme.NotificationCenterWin11Presenter` and
`Presenters["calendar"] = theme.CalendarWin11Presenter` (`theme/profiles_win11_notify.go`,
`declareWin11Notifications`, called before `declareWin11Materials` because that
one switches the material off for every part declared before it). The components
ask the presenter name of their own component (`richView.w11()`, `CalendarFlyout.win11()`),
never the theme name. A theme switch on open panels re-lays them out without
re-creating anything (tested: Win11 light → dark → Win10 → Win11).

**Center.** Card 364 wide (`notificationcenter.width`), radius 8 from the panel
style, glued to the right edge of the work area with `notificationcenter.edge` (12);
its bottom is `notificationcenter.stack.gap` (8) above the calendar when the
calendar is linked and open, otherwise `edge` above the bar (`w11Area`: `WorkArea`,
else `Screen` cut at the anchor). Height = content (header 52 + list + pad), capped
by the free space; empty list = `list.min` under the "No new notifications" label.
Header: "Notifications" (font `heading`), bell "Do not disturb" (zone `zoneDND`,
part `headbtn`, `headbtn.on` + slash when on) and "Clear all" (inactive and without
a hit zone while the list is empty). Cards (radius 4, part `card`): app row (icon,
name, time) for a single notification; title; text (2 lines, 8 expanded); chevron
(expand) and cross (the cross replaces the time under the pointer); the four action
kinds exactly as in Windows 10 (shared `layoutActions`, zones, dropdown, reply);
severity strip inset from the rounded corners. A group exists only for 2+
notifications of one app: header with icon, name, count pill (`pill`), chevron and
cross; its cards drop the app row and carry the time in their title row. The list
scrolls (thin thumb), expand/collapse use `notification.expand` (150 ms out-cubic,
shortened by `motion.reduce`). Quick actions are not drawn (Windows 11 has a
separate `QuickSettings`). The toast (`NewNotificationToast`) shows the same card
(`toast` part, solid + hairline, radius 8) 12 from the corner. Hit-testing,
scrolling, keyboard and animation are the Windows 10 code: `richView` got
`flowGroups`/`shiftGroups` (extracted from `layout`, identical for Windows 10) and
dispatches `layoutW11`, `drawW11`, `drawCardW11`, `layoutCardAsW11`. Rounded parts:
`PaintStyle` now returns the caller clip after `SetRoundClip` (it replaced the clip
with the layer rect and a card at the edge of a scrolling list painted over its
neighbours).

**Do not disturb (the model the taskbar bell uses).**
`desktop.DoNotDisturb{ Enabled() bool; SetEnabled(bool); Subscribe(func()) func() }`,
ready `desktop.NewDoNotDisturb(initial) *DoNotDisturbState` (+ `OnChange` hook).
`nc.SetDoNotDisturb(d)` / `toast.SetDoNotDisturb(d)`: the centre subscribes while
open and repaints only the bell rect; the toast stays silent while `Enabled()` (the
notification still lands in the centre) and hides a shown toast when the mode turns
on. What the mode does elsewhere (sound, focus assist) is the consumer's. The tray
bell takes the same model: `bell.BindDoNotDisturb(d)` reads it at once and follows
every switch (unsubscribed in `Close`; `nil` unbinds). The older manual
`bell.SetDoNotDisturb(bool)` and a source implementing `DoNotDisturbReporter` still
work; one model for centre, toast and bell is the intended setup.

**Calendar.** Card 364 wide (`calendar.w11.*`), at the right edge `calendar.w11.edge`
(12) above the bar (`CalendarFlyout.WorkArea` is new, like the centre's). Top row:
"Monday, October 5" (`DateHeaderCulture.DateHeader`, optional extension of
`DateCulture`; `LocaleCulture` implements it through `desktop.cal.headDate` and
`desktop.cal.weekdayLong.N`; a culture without it gets `{W}, {d} {M}` from
`WeekdayShort`/`MonthGenitive`) and the collapse chevron; then month title with
‹ ›, weekday row, 4–6 week grid (first weekday by culture, today = accent circle
`calendar.w11.day`, selected = ring), then the Focus module. Collapse hides the
grid; `SetCollapsed`/month change repaint the OLD rect too and re-lay the centre
above (`CalendarFlyout.relayout`). On a low screen the calendar stays collapsed
while it is linked and the centre would otherwise get less than header + `list.min`
(`effCollapsed`; the user's `Collapsed()` is unchanged).

**Group.** `desktop.LinkNotificationCenter(nc, cal) *FlyoutGroup` sets both in one
`FlyoutGroup`, makes `nc` sit on `cal` and returns the group: `group.OpenAll(anchor)`
from the clock or the bell, `CloseAll`, Esc in either closes both, a click in the
neighbour neither closes nor is swallowed (`richMouseButton` and the calendar
now use `ownsPoint`). Register both in `FlyoutManager` (the group keeps them
open together). Win+N is the consumer's.

**Focus.** `desktop.FocusSession{ State() FocusSessionState; SetDuration(time.Duration);
Start(); Stop(); Subscribe(func()) func() }`, `FocusSessionState{Duration, Running,
EndsAt}`; `cal.SetFocusSession(s)` (nil = no module). Idle: "Focus", `−` `30 min`
`+` (step `FocusStep` 5 min, `FocusMinDuration`..`FocusMaxDuration` 5..240) and the
accent "Start". Running: `mm:ss` countdown (rounded up), a progress line and
"Stop"; remaining = `EndsAt − now` where now is the calendar `Clock` (never
negative — the consumer calls `Stop` at the end, it is idempotent). While open and
running the calendar redraws only the module once a second through
`cal.Ticker func(d, f) (stop func())` (default: a `time.AfterFunc` chain; set
`cal.Ticker = func(d, f) func() { t := eng.Every(d, f); return t.Stop }` to run it
on the engine goroutine). The tick is not an animation: `motion.reduce` does not
touch it. Ready model for demos and tests: `NewFakeFocusSession(clock)`.

**Keyboard.** Tab in the centre: bell, "Clear all", cards/groups/actions…; from the
last stop it hands focus to the calendar (collapse, ‹, ›, the day grid, `−`, `+`,
Start/Stop) and Shift+Tab from the calendar's first stop hands it back; Enter/Space
activate; on the grid the arrows move the day (leaving the month pages it) and
Enter selects, elsewhere ←/→ and PgUp/PgDn page months; Esc closes the group; the
focus ring is drawn for the keyboard only. Both panels are `Focusable` + `TabAcceptor` while open and Windows 11.

**Profile.** Tokens: `surface.card` (`KeySurfaceCard`), `text.secondary`
(`KeyTextSecondary`); `Windows11Dark` replaces exactly these two (14 of 15
allowed own tokens; the other Windows 11 panels must fit in the one left). Parts of
component `notificationcenter`: `toast header headbtn headbtn.on link group pill
card action field glyph dim severity.* scrollbar`; of `calendar`: `date month nav
divider dim focus.title focus.value focus.button focus.accent progress
progress.fill` and `day` (circle, no shadow). Colours are tokens, accent or a
neutral grey film (readable in both modes); links use the accent (dark mode: the
default accent is low contrast on dark — a profile may override `link`).
Strings (RU/EN, `widget.Tr`): `desktop.notif.title|dnd`,
`desktop.focus.title|minutes|hours|hoursMinutes|start|stop|less|more`,
`desktop.cal.headDate|weekdayLong.N`.

**Cost.** First frame of both panels (open + one draw each, warm font cache) is
well under 100 ms (`TestW11_OpensUnder100ms`); a hover repaints the card, a bell
change the bell, a tick the Focus module, a month page the calendar and the
centre. Heights at 100–200 %: 1920×1080 … 640×480 logical fit the screen without
overlap (`TestW11_FitsEveryScreen`); on a short screen the list scrolls.

Tests: `desktop/notifycenter_win11_test.go` (behaviour), `desktop/paint_clip_test.go`,
`theme/profiles_win11_notify_test.go`; pictures: `NC_OUT=dir go test ./desktop -run
TestVisual_NotificationCenterWin11` and `TestVisual_NotificationToastWin11`
(`light`, `dark`, `*_collapsed`, `*_running`, `*_dnd`, `*_empty`, `*_focus_ring*`,
`*_group_collapsed`, `light_200`…).

### Windows 10 Start menu with tiles and the taskbar search box — v3.33

`desktop.StartMenu` has a second layout: sidebar | app list | tiles. A theme asks
for it with the presenter `theme.PresenterStartTiles` (`"tiles"`,
`Profile.Presenters["startmenu"]`, set by the Windows 10 profile and inherited by
its dark variant); the component asks `StartMenu.tiled()` and never looks at the
theme name. Windows 2000 and macOS keep the flat list (rendered frames of those
profiles are byte-identical to v3.30; Windows 11 got its own grid layout, see
"Windows 11 Start menu" below). Same object, same `Open/Close/Toggle`: a theme switch on an open menu
changes the look without re-creating anything.

```go
menu := desktop.NewStartMenu(m, catalog)
menu.Screen = screen
menu.SetSource(src)                        // desktop.StartMenuSource: Recent(), Groups(), Subscribe()
menu.SetSidebarItems([]desktop.StartSidebarItem{
    {ID: "user", Title: "oops", Glyph: desktop.GlyphUser},
    {ID: "power", Title: "Power", Glyph: desktop.GlyphPower, KeepOpen: true},
})
menu.OnSidebarActivate = func(id string) { ... }
menu.SetTileGroups(groups)                 // []desktop.TileGroup{ID, Title, Tiles []Tile}
menu.SetTileContent("mail", desktop.TileContent{Title: "Mail", Badge: "3"}) // repaints ONE tile
menu.OnTilesChanged = func(g []desktop.TileGroup) { save(g) }   // after a drag
menu.ContextMenu = func(t desktop.StartTarget) []widget.MenuItem { ... } // right button
box := desktop.NewSearchBox(m, provider)   // Item for SlotStart, after the Start button
box.Bind(menu, startButton.Bounds)         // results appear IN the menu instead of the list
bar.AddItem(desktop.SlotStart, box)
```

- **Data comes from the consumer.** `StartMenuSource` (recent, letter groups,
  folders, second line) is optional: without it the menu groups `AppCatalog.Apps()`
  by the first letter of the title (`GroupByLetter`, `StartLetter`: `#`, Latin,
  Cyrillic). `AppInfo.IconAt`/`StartEntry.IconAt`/`TileContent.IconAt` get the side
  in PHYSICAL pixels. Built-in vector glyphs (`GlyphUser`, `GlyphDocuments`,
  `GlyphPictures`, `GlyphSettings`, `GlyphPower`; hamburger, magnifier and
  chevrons are internal) are SVG rasterised at physical size, drawn in the style's
  text colour. `FakeStartSource`, `FakeSearchProvider` are the test fakes.
- **Metrics (profile, logical px at 100 %):** `startmenu.sidebar.collapsed` 48,
  `.sidebar.expanded` 256, `.sidebar.icon.size` 20, `startmenu.list.width` 260,
  `.row.height` 36, `.letter.height` 36, `.icon.size` 24, `.row.pad` 20,
  `.row.icon.gap` 8, `.corner` 0, `.tiles.pad.left/right/top/bottom` 21/16/18/12,
  `.tiles.columns` 6, `.height` 700 (0 = fill), `.height.min` 320, `.margin` 0;
  `tile.unit` 48, `tile.gap` 4 (medium 100, wide/large 204), `tile.group.header`
  32, `.header.gap` 5, `tile.group.gap` 4; `scrollbar.thin.width/.hover.width`;
  `search.width` 344, `.icon.width` 48, `.icon.size` 16, `.pad` 12, `.icon.gap` 8.
  Width = 48 + 260 + 21 + 308 + 16 + 2 (border) = 655; height = min(`startmenu.height`,
  anchor.Min.Y - Screen.Min.Y - margin) but not below `.height.min` while it
  fits. A narrower screen shrinks the tile columns, below two columns the tile area
  is hidden.
- **Style parts** (`theme/profiles_win10_shell.go`, both palettes): `startmenu`:
  `panel` (PadX/PadY 1 = border), `sidebar`, `sidebar.item`, `row`, `row.sub`,
  `letter`, `tile`, `tile.group`, `scrollbar`; `searchbox`: `""` (field), `hint`,
  `icon`. `Flyout` got four unexported hooks: `partFn` (which part paints the
  base — `panel` for tiles), `marginFn` (`startmenu.margin`, 0 for tiles) and
  `beforeOpen`/`afterOpen` (state reset, subscriptions, focus request). The last
  two exist because Go embedding has no virtual `Open`: a menu opened through
  `Toggle`, `FlyoutManager` or a group never ran an `Open` override of the
  enclosing type.
- **Sidebar.** Collapsed 48 px (icons only), the hamburger toggles it to 256 px
  with a `Tween` on the `menu.open` token (0 ms = instant); the expanded panel
  lies over the list; each animation step invalidates only the panel column. A
  press outside the sidebar collapses it. The title is the upper-cased
  `desktop.start`.
- **List.** `Recent`, then letter groups; folder rows expand in place
  (`StartEntry.Folder/Children`, Right/Left keys, `SetFolderExpanded`). Rows are
  laid out by cumulative offsets and only the visible range is drawn
  (`firstRowAt`, 3000 apps draw <120 strings). Wheel = 3 rows, `OnMouseWheelPixels`
  for smooth devices; the thin scrollbar (`fadeBar`) shows on mouse move/scroll and
  fades after 1.2 s on one timer.
- **Tiles.** `placeTiles` packs tiles first-fit into `tiles.columns` units
  (small 1×1, medium 2×2, wide 4×2, large 4×4); a group is header + grid; groups
  stack in one scrollable column. `SetTileContent` is copy-on-write and
  invalidates the one tile rectangle. Drag: press on a tile captures the mouse
  (`WantsCapture`), a threshold of `tile.gap` starts the drag, the layout previews
  the drop (`dragPreview`), release calls `OnTilesChanged` with the whole new
  order (between groups too). The consumer persists it. A tile released into
  EMPTY space makes a new group (`startmenu_tiles_groups.go`): below the last group,
  in the gap between two groups together with the upper half of the lower header,
  or above the first group; the lower half of a header and the tile rows still mean
  "into this group". The new group has an empty `Title` and a unique `ID`
  (`group-N`) and is reported by `OnTilesChanged` like any other; while it is
  carried the preview shows it with the muted hint `desktop.start.newGroup` over the
  header. A group emptied by the drag is removed on release (kept as a bare header
  while dragging so the layout does not jump); dropping the lone tile of a group
  into a "new group" right next to it changes nothing and fires no event. While a
  tile is carried the scrollable height gets extra room under the last group
  (`dragExtra`) so a full area can still be dropped below. A left release outside
  the menu now ends a drag (the mouse is captured). Tile layout cache is keyed by
  `tilesRev` too — before, `SetTileGroups` and a drop did not refresh it.
- **Letter jump.** A click (or Enter/Space) on a letter header opens, in place of the
  list, a grid of all letters (`startmenu_tiles_letters.go`): `#`, A-Z, the
  Cyrillic block when the list has Cyrillic groups (Ё after Е), then any other
  letters of the source. 4 columns, up to 8 on a low menu so the alphabet fits
  without scrolling. Letters without a group are muted (`letter` part, Disabled)
  and inert; active ones sit on a faint tint and use `letter` hover/pressed/active.
  Picking a letter scrolls the list so the group header is on top (clamped at the
  end) and closes the grid; Esc closes only the grid (selection goes back to the
  header), Tab, a press on tiles/sidebar, typing (search) and reopening the menu
  close it too; the wheel is ignored over it. The grid fades in by the `menu.open`
  token (`gridFade`). Letter headers became focusable rows: arrows walk through
  them, Home/End and the first Down still land on applications. API:
  `LetterGridOpen`, `OpenLetterGrid`, `CloseLetterGrid`, `JumpToLetter(letter)`.
- **Scrollbar mouse control** (`startmenu_tiles_scrollbar.go`). The track is a column
  at the right edge of the list and of the tile area, `scrollbar.thin.hover.width`+2
  wide (narrower than `startmenu.row.pad`/2, so a folder chevron stays clickable).
  Press on the thumb captures the mouse; the thumb follows it proportionally (scroll
  is computed from the pointer, not accumulated; the grab offset is kept), it is
  drawn wide and in the `scrollbar` Hover colour and is pinned (`fadeBar.pin`) while
  dragged or hovered; release anywhere ends it. A press on the empty track pages by
  the view height minus one row (list) / one tile step (tiles). Same for the tile area.
- **Keyboard.** The open menu requests focus (`focusreq`), is `Focusable` and a
  `TabAcceptor`: Tab/Shift+Tab cycle sidebar → list → tiles (empty areas are
  skipped); arrows inside; in tiles the arrows follow the grid geometry
  (`nearestTile`: along + 2×across); Home/End, PageUp/PageDown, Enter/Space, Esc
  (closes the context menu first, then the menu). A printable character moves
  input to the bound search box (focus moves to it) or, without a box, into the
  menu's own query bar. The keyboard selection is drawn with `PaintFocusRing`.
- **Search.** `SearchBox` (modes `SearchModeHidden/IconOnly/Box`, width from
  `search.width`, placeholder `desktop.search.placeholder`, hover/focus styles,
  caret editing, Ctrl+V). It talks to `SearchProvider` (`SetQuery`, `Results`,
  `Activate`, `Subscribe`); the menu shows `Results()` instead of the list
  (grouped by `Category`, "No results" row) and re-reads them on the provider's
  notification. Enter/Up/Down in the box are forwarded to the menu
  (`StartMenu.SearchKey`); Esc closes the menu and clears the text. A press on
  the box does not dismiss the open menu (`StartMenu.DismissAt`). `SetMode`
  relayouts the taskbar (the item implements `SetRelayout`, `Taskbar.AddItem`
  hands it the callback).
- **Strings.** `desktop.start.recent|expand|collapse|noResults|results|newGroup`,
  `desktop.search.placeholder|label`, RU and EN. To read them from your own table:
  `widget.AliasStrings(desktop.StartMenuAliases("Start", "RecentlyAdded",
  "SearchPlaceholder", "Expand", "Collapse"))`.
- **Lifetime.** Subscriptions (theme, language, source, provider) live while the
  menu is open (`resubscribe` / `detachOnClose`); closing clears the query and
  the box. Not done: naming a new tile group in place (the consumer sets `Title`
  after `OnTilesChanged`), scrolling the new group into view after a drop.

Cost (1280×720, 300 apps, Windows 10, `TestStartMenu10_FirstFrameUnder100ms`):
first frame after `Open` ≈ 9 ms cold, ≈ 8 ms warm; opening invalidates only the
menu rectangle (`TestStartMenu10_OpenDamagesOnlyMenuArea`). Snapshots:
`GOLDEN_OUT=dir go test ./desktop -run TestStartMenu10_` writes dark/light PNGs at
100/150/200 %.

### Quick settings of Windows 11 24H2 — `desktop/quickpanel*.go`, `quickglyphs.go`, `theme/profiles_win11_quick.go`

`desktop.QuickSettings` stays one component. The Windows 11 profile names the
presenter (`Profile.Presenters["quicksettings"] = theme.QuickSettingsPresenter`;
the dark profile inherits it, it adds no token of its own). The component asks
`PresenterFor(tm, "quicksettings")` — never the theme name — and draws the 24H2
variant only when it also has a tile model (`SetQuickActions`). Windows 10,
Windows 2000, macOS and Windows 11 without a model draw the old three-tile panel;
their frames are byte-identical to before (checked on 8 profiles x 2 scales, two
states each).

Consumer API (all additive):

| what | API |
|---|---|
| tiles | `QuickSettings.SetQuickActions(QuickActionModel)`, `QuickActions()`; `QuickAction` got `Unavailable`, `Detail`, `HasDetails` |
| reorder | `QuickActionReorderer` (optional model interface), `QuickActionList.Reorder([]QuickActionID)`, `QuickSettings.OnReorder` |
| volume | existing `SystemStatus.Volume()`, `OnVolumeChange`, `OnToggleMute`; new `VolumeDetails` (shows the volume "›") |
| brightness | `SetBrightness(0..1)`, `ClearBrightness()`, `Brightness()`, `OnBrightnessChange`; no data = no slider |
| nested page | `Details func(QuickActionID) *QuickDetails` (`Title`, `Content widget.Widget`, `OnClose`), `QuickVolumeID`, `OpenDetails(id)`, `CloseDetails()`, `DetailsOpen()` |
| footer | battery from `SystemStatus.Power()` (`NoBattery` hides it), `OnEdit(bool)`, `OnSettings()` |
| edit mode | `SetEditing(bool)`, `Editing()` |
| theme | `theme.QuickSettingsPresenter`, `theme.KeyQuickPage` (animation), `theme.KeyQuickIconTint` (flag, on) |
| strings | `desktop.StrQuick*` (`desktop.quick.*`, RU and EN) |

Layout is plain arithmetic from the panel rectangle and the state
(`quickpanel_layout.go`): the same `qsLayout` is used by drawing and by hit
testing, so what is clicked is what is drawn, also while the panel slides in.
Sizes are metrics `quicksettings.w11.*` (width 360, side padding 24, top 24,
tile 96x48, column and row gap 12, caption 6 + 32, "›" zone 28, slider row 40,
footer 48, header 48, margin 12). Panel height = padding + visible tile rows
(`rows`, 2) + sliders + footer; it does not depend on the page or on edit mode
(the sliders area shows a hint in edit mode), so the panel never jumps. Parts of
style `quicksettings`: `w11.tile`, `w11.tile.on` (accent; hover/pressed use
`accent.hover`/`accent.pressed`; Active is not used), `w11.tile.chevron`,
`w11.tile.edit`, `w11.label`, `w11.detail`, `w11.title`, `w11.dim`, `w11.button`,
`w11.done`, `w11.slider.track|fill|thumb|dot`, `w11.footer`, `w11.scrollbar`.
They are declared BEFORE `declareWin11Materials`, so Mica and the soft shadow of the
panel are not inherited by tiles.

Behaviour worth knowing:

- Tile click -> `model.Toggle(id)`; `Disabled`/`Unavailable` do not toggle (the
  list model also refuses them). "›" is a separate focus and hit zone; it opens
  `Details(id)` (nil result = nothing opens). Not drawn in edit mode.
- Nested page: both pages are drawn shifted by `page` (`Tween`, token
  `quicksettings.page`, decorative: 0 under `motion.reduce`). Content gets
  `SetBounds(body)` when drawn, receives the mouse through its own deepest
  widget under the cursor (`MouseClickHandler`, `MouseMoveHandler`,
  `OnMouseWheelPixels`) and keys on the root if it is a `widget.KeyHandler`; Tab
  switches between "back" and the content. Back: arrow, Backspace, Alt+Left.
  Esc steps back one level: edit mode -> page -> panel (also through
  `DismissOnEscape`, which the engine calls when the panel is not focused).
- Scroll: wheel, thin thumb (drag with mouse capture), focus follows the tile
  (`ensureVisible`). Rows scrolled under the edge are clipped by the grid:
  `PaintStyle` of a rounded layer replaces the rect clip, so parts inside a
  clipped area (tiles, and everything while pages slide) are painted by
  `qsPaint` (fill + border under the rect clip) when they are not fully inside.
- Edit mode (pencil): drag a tile (threshold 5 px, mouse capture, auto-scroll at
  the grid edge, others make room live), or Alt/Ctrl+arrows on the focused tile.
  The new order goes to the model (`QuickActionReorderer`), `OnReorder`, and stays
  visible until the panel closes (`pn.order`). A drop on the same place is not an
  event.
- Sliders: click, drag (capture), wheel, keys (arrows 5 %, PageUp/PageDown 10 %,
  Home/End). The shown level follows the hand at once (`volOver`, `bright`) and the
  consumer's `SystemStatus`/`SetBrightness` wins on the next notification.
- Repaint: a model change with the same set and order repaints only the changed
  tile and its caption (`InvalidateRect`, no `Invalidate`); a status change only the
  volume row and the battery; hover only the zone. Subscriptions (model, status)
  live while the panel is open and are set by the `Flyout` hooks (`afterOpen`,
  `afterClose`), so they work when a `FlyoutManager` or a group opens the panel.
- Placement (`Flyout.Place`, bottom/top bars): centred on the anchor, `margin` (12)
  from the screen edge and from the bar; side bars use the common placement.
- Keyboard: Tab/Shift+Tab walk tiles (each followed by its "›"), brightness, volume
  icon, volume, "›", pencil, gear; arrows move over the grid and adjust sliders;
  Enter/Space activate; the ring is `PaintFocusRing`, shown only after a key.
- Tile icons: `QuickAction.IconAt(physicalSide)` / `Icon`; under flag
  `quicksettings.icon.tint` they are recoloured with the tile text colour.
  Glyphs of the panel (chevrons, pencil, gear, sun, speaker, grip, bolt) are SVG
  rasterised at physical size (`quickglyphs.go`).
- Not done: the Windows 11 flyout shows tile labels on one line (elided), not two;
  `Unavailable` is ignored by the Windows 10 center; tooltips of tile captions that
  are elided are not shown.

Tests: `desktop/quickpanel_test.go` (variant selection, geometry at 100-200 %,
toggle, single-tile repaint, nested page and reduce motion, scroll, sliders,
edit and reorder, footer, keyboard, ring, accent/theme/language live, first frame
< 100 ms, no subscriptions when closed), `quickpanel_clip_test.go` (nothing is
drawn outside the grid or the panel), `theme/profiles_win11_quick_test.go`.
Frames: `QS_OUT=dir go test -run TestVisual_QuickSettingsWin11 ./desktop/`.

### Windows 11 Start menu — pinned grid, Recommended, All apps (next minor)

`desktop.StartMenu` has a third layout. A theme asks for it with the presenter
`theme.PresenterStartGrid` (`"grid"`, `Profile.Presenters["startmenu"]`, set by the
Windows 11 profile and inherited by its dark variant); the component asks
`StartMenu.grid()` / `AsGrid()` and never looks at the theme name. Windows 10 keeps
tiles, Windows 2000 and macOS keep the flat list: rendered frames of those three
are byte-identical to before (every frame of the desktop golden and snapshot tests,
135 PNGs; only the Windows 11 `start` frames and the `edge_Windows11 Dark_*_start`
goldens changed). Same object, same `Open/Close/
Toggle`: switching the theme between the three on an open menu changes the look
without re-creating anything (`syncKind` resets the state that belongs to the old
look: area, selection, hover, page, drag, view).

```go
menu := desktop.NewStartMenu(m, catalog)
menu.Screen = monitorBounds
menu.SetPinned([]desktop.StartPinned{{ID: "edge", App: "edge", Title: "Edge", IconAt: icon}}) // nil = AppCatalog.Pinned()
menu.OnPinnedChanged = func(p []desktop.StartPinned) { save(p) }  // after a drag: whole new order
menu.OnPinnedLaunch = func(id string) { ... }                      // cell without App
menu.SetRecommended(src)            // desktop.StartRecommendedSource; nil = StartMenuSource.Recent()
menu.SetRecommendedEnabled(false)   // policy: the section disappears, pinned take its room
menu.OnRecommendedActivate = func(id string) { ... }               // row without App
menu.SetUser(desktop.StartUser{Name: "oops", AvatarAt: avatar})
menu.UserMenu = func() []widget.MenuItem { ... }                   // footer menus (consumer decides)
menu.PowerMenu = func() []widget.MenuItem { ... }                  // nil -> OnSidebarActivate("user"|"power")
menu.GroupBounds = func() image.Rectangle { return appsGroup }     // centre of the taskbar group
menu.ContextMenu = func(t desktop.StartTarget) []widget.MenuItem  // StartTargetPinned/Recommended/User/Power
menu.SetSource(src); menu.SetSearchProvider(p)   // "All apps" list and search: same as Windows 10
menu.View() / SetView(desktop.StartViewMain | StartViewAllApps | StartViewRecommended)
menu.PageCount() / Page() / SetPage(n)           // pages of the pinned grid
```

- **Data comes from the consumer.** `StartPinned{ID, App, Title, Icon, IconAt}`,
  `StartRecommendedItem{ID, App, Title, Subtitle, Icon, IconAt}`, `StartUser{Name,
  Avatar, AvatarAt}`; `FakeStartRecommended` is the test fake. A cell with `App` is
  launched through `AppCatalog.Launch`, without it through `OnPinnedLaunch` /
  `OnRecommendedActivate`; the menu closes after a launch. Once the user drags a cell
  the menu keeps its own order (`SetPinned` semantics) and the catalog's later
  changes are no longer read until the consumer calls `SetPinned(nil)`.
- **Layout (profile metrics, logical px at 100 %).** `startmenu.w11.width` 642,
  `.height` 726 (`.height.min` 360), `.corner` 8, `.pad` 32, `.margin` 12 (gap above
  the Start button), `.grid.columns` 6, `.grid.cell.w/.h` 96×84, `.grid.gap.y` 16,
  `.grid.icon` 32, `.footer.height` 64, `.search.top/.height/.corner/.pad/.icon`
  28/32/16/12/16, `.section.height` 28, `.section.gap` 16, `.rec.columns/.rows` 2/3,
  `.rec.row.height` 56, `.rec.icon` 32, `.dots.size/.gap` 6/8, `.footer.avatar` 32,
  `.footer.button` 40. A value the profile does not declare falls back to the one
  above (`StartMenu.gm`). The list ("All apps", "All recommendations", results) uses
  the Windows 10 metrics `startmenu.row.height` (40 here), `.letter.height`,
  `.icon.size`, `.row.pad`, `.row.icon.gap`, `scrollbar.thin.*`.
  Geometry is one function, `gridGeometry(panel)` (search 32 high at the top, section
  headers 28, grid centred, recommended rows anchored to the footer, footer inside the
  1 px border); hit testing, drawing and keys use it, so they never disagree. The
  number of Recommended rows (3 → 0) and of pinned rows shrinks with the available
  height, a narrower screen drops grid columns; the panel never leaves the monitor
  (`Flyout.fitInto`) and its height is cut to the work area. Checked for 100-200 %
  on 1920×1080, 1366×768 and 1280×720 (`TestStartGrid_ScalesWithoutClipping`).
- **Position** (`Flyout.Place`, `StartMenu.placeGrid`): above the Start button with the
  `.margin` gap; with the flag `taskbar.centered` (read live, so the alignment
  switch needs no re-creation) centred on `GroupBounds()` or, without it, on the
  monitor centre (the taskbar centres its group on the panel); with it off, at the
  left edge of the button. Side taskbars use the ordinary side placement.
- **Views.** Main: search, "Pinned" + "All apps ›", pinned grid, "Recommended" +
  "More ›", footer. "All apps" is the Windows 10 list unchanged (letters, folders,
  letter-jump grid, thin scrollbar) under a "Back" header; "More ›" shows every
  recommendation as two-line rows of the same list (`rowRec`). A non-empty query
  replaces the middle with search results (`SearchProvider`), like Windows 10. Opening
  always starts on the main view. Only the middle changes: search and footer stay.
- **Pinned pages.** Pinned apps that do not fit are paged, dots on the right
  (`dots`, `Page`); wheel flips one page (not more often than `wheelFlipGap`, 220 ms —
  a touchpad sends dozens of events per gesture), dots click, PageUp/PageDown.
- **Drag.** Press on a cell captures the mouse (`WantsCapture`), a 4 px threshold
  starts the drag, the layout previews the new order (the dragged cell leaves an
  outline, the cell follows the pointer), hovering the dots flips the page, release
  calls `OnPinnedChanged` with the whole new order; the same order is no event; a
  release outside the menu still ends the drag.
- **Search field.** Own, inside the menu (32 high, corner 16): typing in any area goes
  into it (`typeToSearch`), caret editing (Left/Right/Home/End/Backspace/Delete,
  Ctrl+V), click places the caret, accent frame while it is the keyboard area. The
  query/results/provider are the shared `startView` ones, so a taskbar `SearchBox`
  bound with `Bind` still works and mirrors the text. `AsTiled()` is false for this
  view: the taskbar search box is not needed for it.
- **Keyboard.** Tab/Shift+Tab: search → pinned → recommended → footer (list views:
  search → list → footer); arrows follow the grid (Up from the first row → "All apps",
  Down from the last → Recommended → footer), Home/End, PageUp/PageDown, Enter/Space,
  Esc (closes the context menu, then the letter grid, then the menu), Backspace with
  an empty query goes back from a list view, Menu or Shift+F10 opens the context
  menu of the selected object. Focus ring: `PaintFocusRing`.
- **Look (`theme/profiles_win11_start.go`).** Parts of `startmenu`: `heading`, `link`
  (the "All apps ›" button), `pin`, `rec`, `rec.sub`, `row`, `row.sub`, `letter`,
  `scrollbar`, `search`, `search.hint`, `footer`, `footer.item`, `avatar`, `dot`.
  Plates are neutral grey films (`RGBA(128,128,128,a)`) that work on both light and
  dark surfaces, text and hairlines are token references (`text`, `border`,
  `accent`), so `Windows11Dark` only gained ONE token, `field` (`theme.KeyField`, the
  search field fill) — 13 of its 15. The declaration runs before
  `declareWin11Materials`, so the parts get no Mica and no big shadow of their own;
  the panel itself follows `FlagBackdropMica`/`FlagBackdropMicaAlt`/`FlagShadowSoft`
  (Solid by default). The base style got the 1 px `border` token frame.
- **Strings.** `desktop.start.recommended|more|back|power`, RU and EN; "Pinned",
  "All apps", the search hint and "No results" are shared with Windows 10.
  `desktop.StartGridAliases(recommended, more, back, power)` binds them to the
  consumer's table (`widget.AliasStrings`).
- **Shared code touched.** `startGeometry`, `hitTest`, `keyRect`, `activate`,
  `targetFor`, `refreshResults`, `typeToSearch`, `SearchKey`, letter grid and the
  wheel dispatch on `grid()`; `tiled()` stays Windows-10-only and `modern()` is
  `tiled() || grid()` for what both share (focus request, Tab, capture, subscriptions).
  New `startArea`s: `areaSearch/Pinned/Rec/Footer`; the list keeps `areaList`.

Cost (1280×800, 300 apps, Windows 11, `TestStartMenu11_FirstFrameUnder100ms`): first
frame after `Open` ≈ 13 ms cold, ≈ 12 ms warm, the 300-app "All apps" frame < 100 ms,
hover frame ≈ 1 ms; opening, hover, keys, paging and search invalidate only the menu
rectangle (`TestStartMenu11_OpenDamagesOnlyMenuArea`). Snapshots: `GOLDEN_OUT=dir go
test ./desktop -run TestStartMenu11_` writes light/dark PNGs of the main view, hover,
focus, All apps, More, search, drag, pages, context/power menus, Mica, 100/150/200 %.
Not done: moving a pinned cell with the keyboard (drag only), a separate Windows 11
power flyout width (the menu takes the profile's `menu.width.min`), animated page
change (instant), recommendation rows beyond 2 columns in "All recommendations".

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


## v3.17 additions (DiffView, MergeView and what an editor needs from the engine)

### DiffView — compare and edit two files

```go
dv := widget.NewDiffView("", "")      // mono font, header font; "" → built-in Go Mono / Go Bold
dv.LoadFile(widget.DiffLeft, a)      // error for binary files (also sent to OnError)
dv.LoadFile(widget.DiffRight, b)
dv.SetText(side, title, note, text)  // content without a file (Save → ErrDiffNoPath, use SaveAs)
dv.Save(side); dv.SaveAs(side, path) // keeps EOL (CRLF/LF), BOM, final newline
dv.Reload(side)                      // undoable
dv.SetWatchFiles(true); defer dv.Close()
dv.SetHideUnchanged(true); dv.SetContextLines(3)
dv.SetIgnoreWhitespace(v); dv.SetSyntaxHighlight(v); dv.SetReadOnly(side, v)
dv.SetShowHeaders(v)                 // false → no side header cards, code starts at the top edge (default true)
dv.SetShowReadOnlyMark(v)            // false → no "read-only" mark in the header; side stays read-only (default true)
dv.SetFont(mono, bold, sizePt)
dv.ChangeCount(); dv.Changes() []widget.DiffChange // {Kind, LeftFrom, LeftTo, RightFrom, RightTo}
dv.NextChange(); dv.PrevChange(); dv.GoToChange(i); dv.CurrentChange()
dv.CopyBlock(i, toRight); dv.CopyCurrent(toRight); dv.CopyAll(toRight)
dv.Undo(); dv.Redo(); dv.CanUndo(); dv.CanRedo()
dv.SetCaret(side, line, col); dv.Caret(side); dv.InsertText(s); dv.SelectedText()
dv.Selection(side) (from, to int, ok bool) // lines touched by the selection, [from, to); ends at col 0 → that line excluded
dv.Text(side); dv.Lines(side); dv.IsModified(side); dv.FilePath(side)
```

Events (all called without the control's lock held, so handlers may call back
into it): `OnTextChanged`, `OnModifiedChanged`, `OnFileLoaded`, `OnFileSaved`,
`OnFileChangedOnDisk`, `OnDiffChanged`, `OnCurrentChangeChanged`,
`OnCaretMoved`, `OnActiveSideChanged`, `OnBlockCopied`, `OnSaveRequest`
(Ctrl+S), `OnError`. `OnFileChangedOnDisk` comes from the watcher goroutine but
is delivered **on the engine goroutine** via `engine.Post` (the engine hands
itself to the control through `SetCaptureManager`); without an engine it is
called directly. Modified state is revision-based: undo back to the saved
state clears it.

XAML: `<DiffView LeftFile=… RightFile=… SaveCommand="{Binding Save}"/>` — file
paths are confined to the markup directory (SEC-8); with `LoadUIFromXAMLFS`
the content is loaded without a disk path. Any widget with
`SetCommand(name string, cmd ICommand) bool` gets `…Command="{Binding X}"`
attributes wired by the binding scope; DiffView accepts `SaveCommand`
(param `DiffSide`), `TextChangedCommand` (`DiffSide`), `DiffChangedCommand`
(`int`), `FileChangedCommand` (`DiffFileChange`).
### DataGrid: clicking a cell — v3.25

The grid reported the row (`OnSelectionChanged`, `OnRowActivated`) and said
nothing about the cell, so an app that draws a link in a cell — a commit hash,
a path — had nothing to catch the click with: the coordinate lookup was
private.

```go
dg.OnCellClicked = func(e datagrid.CellClickedEvent) {
    // e.RowIndex (current order, same as OnRowActivated), e.ColumnIndex,
    // e.Column, e.Item, e.X/e.Y inside the cell, e.Button, e.DoubleClick
}
dg.CellAt(x, y) (row, col int, ok bool) // ok=false outside the data area
dg.CellRect(row, col) image.Rectangle   // empty if there is no such cell

col.SetCursor(datagrid.CursorHand) // a link cell without a hand looks like plain text
dg.CursorAt(x, y) int              // what DataGridWidget.Cursor reports to the engine
```

Both the single and the double click raise `OnCellClicked` (the second one
with `DoubleClick` set), after the row selection and outside the grid's lock,
so a handler may call back into the grid. Cursor values match `widget.Cursor`
but are plain ints here — `widget` imports `datagrid`, not the other way
round.

### TreeView: a second text fragment on a node — v3.25

`Foreground` colours the whole node line, so a trailing note — an `↑3 ↓1`
counter on a branch, a size on an attachment — fought with the label. Doing it
with `CustomRenderer` means taking over drawing of *every* node (icon, arrow,
selection, hover).

```go
item.Detail = "↑3 ↓1"                          // drawn right after the label
item.DetailColor = color.RGBA{...}             // zero alpha → muted shade of the label colour
// XAML: <TreeViewItem Header="main" Detail="↑3" DetailColor="#8C8C8C"/>
```

The offset is measured in the same face the label is drawn with, so a detail
after a bold label does not overlap it. `<TreeViewItem>` also accepts
`Foreground` and `FontWeight="Bold"`, which until now were code-only.

### Horizontal scrolling in DiffView and MergeView — v3.25

A long line used to be visible only as far as the pane went. `DiffView` could
shift sideways, but the shift was set by exactly one thing — a horizontal
wheel or a trackpad gesture; `MergeView` could not shift at all (its `hscroll`
was read when drawing and never written). On an ordinary mouse the right half
of a long line was unreachable.

Both controls now show a horizontal scrollbar under the code when the longest
line does not fit, and it works like the overview ruler: drag the thumb, or
click the track to jump. Sideways scrolling also answers **Shift + wheel** and
follows the caret (End walks to the end of the line and takes the view with
it). In `MergeView` the shift is shared by all four panes — the panes are
matched line by line, and panes drifting apart sideways are unreadable.

Nothing to call: the bar appears by itself and takes its height from the code
area only while it is needed.

```go
// Shift + wheel needs the modifiers, and the pixel-wheel interface has no
// room for them — widgets that care implement this one instead:
type wheelPixelModHandler interface { // engine-side name; any widget may implement it
    OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool
}
```

The engine tries it first and falls back to `OnMouseWheelPixels`, so widgets
that implement only the old one keep working unchanged. Modifiers come from
`engine.SetModifiers` — the same source that fills `MouseEvent.Mod`.

### MergeView — three-way merge (pair to DiffView)

```go
mv := widget.NewMergeView("", "")     // mono font, header font; "" → built-in
mv.SetSides(ours, base, theirs widget.MergeSideInfo{Title, Note})
mv.SetChunks(chunks []widget.MergeChunk) // app-computed (git merge-file) — main path
mv.SetTexts(base, ours, theirs string)   // control computes chunks itself (demos)

mv.Resolve(i, widget.MergeTakeOurs)   // TakeTheirs / TakeBase / TakeOursThenTheirs /
mv.ResolveCurrent(how)                //   TakeTheirsThenOurs / MergeUnresolved
mv.ResolveAll(how)
mv.Unresolved() / mv.ConflictCount() / mv.Resolution(i)
mv.Result() string                    // markers for unresolved conflicts
mv.NextConflict() / PrevConflict() / GoToConflict(n) / CurrentConflict()
mv.SetShowBase(false)                 // ours, theirs and the result only
mv.SetStyle(widget.MergeStyleDiff3)   // or MergeStyleMerge
mv.SetMarkerSize(9)                   // git conflict-marker-size; 0 → 7
mv.SetResultEOL("\r\n", bom, finalNL) // default "\n" + final newline; SetTexts copies ours
mv.Scroll() / SetScroll(y) / ResultScroll() / SetResultScroll(y) / ScrollToLine(side, line)
mv.SetSyncScroll(false)               // default true: top and result scroll together, by chunk
mv.SetResultInfo(widget.MergeSideInfo{Title, Note, Hint}) // result pane labels; Sides() / ResultInfo() read back
// MergeSideInfo.Hint — second header line; top headers grow when any visible side has one
```

`MergeChunk{Conflict, Ours, Base, Theirs, Merged}` — `Merged` is the result of
a clean chunk, `nil` meaning "same as Ours". Model without a view:
`widget/mergeview` (`Merge`, `Result`, `ResultText`, `UnresolvedCount`), by
analogy with `widget/diffview`.

Rows are aligned by chunk (a chunk occupies the same number of rows in every
pane, padded with `dvRowPad`), so the three top panes share one scroll — no
piecewise mapping like DiffView's. Only the result pane is editable; resolving
splices the chunk's lines **in place**, so hand edits elsewhere survive, and
undo covers resolutions too.

`SetStyle`, `SetSides` and `SetMarkerSize` re-splice only the markers of
unresolved conflicts (`respliceUnresolvedLocked`) — never rebuild the whole
result, it would wipe hand edits. The overview ruler is interactive: thumb drag
keeps the grab offset (no jump), a click on a conflict mark goes to it, a click
elsewhere centers the view there. Marks and thumb share one scale (content
points): drawing and hit-testing both go through `rulerTrackLocked`,
`rulerMarkLocked` and `rulerThumbLocked`, so what is drawn is what gets clicked.

### Children of a scroll view: clicks, hover and repaint where they are seen — v3.29

A long-standing and serious defect: in a scrolled `ScrollView` a click hit the
wrong widget. A list of buttons A, B, C, D scrolled so that C and D are shown —
a click on C pressed A, invisible, scrolled past the top edge.

The cause is how the scroll view draws its children: the offset is applied by
a translating draw context (PERF-12), and the children's `Bounds()` stay in
CONTENT coordinates. Everything that looked at those bounds saw unshifted
coordinates: hit testing; the coordinates of presses, moves, wheel and mouse
capture; hover; repaint (a child refreshed the wrong place on screen); menus
and drop-downs opened by children; the IME caret and accessibility bounds;
nested scroll views; the same horizontally.

Shifting the children's bounds for real is not an option: the performance
tests require them not to change with scrolling, applications lay children out
in content coordinates, and `Panel`/`Canvas` keep grandchildren in absolute
coordinates — every wheel step would walk the whole subtree. So children stay
in the content frame and coordinates are translated at every boundary with the
outside world:

```go
// A container that shows its children shifted reports the offset:
type ContentOffsetter interface {
    ContentOffset() image.Point // screen = child-frame point + offset
}
```

`ScrollView` implements it. Hit testing builds a path with a frame per step
(`engine/frames.go`); presses, moves, wheel, file drops, cursor, tooltips, the
mouse-capture owner and `ContextMenuAt` receive the point in their widget's
frame; `Base` remembers its parent (set in `AddChild`, also by
`TabControl.AddTab` and `DockManager`) and translates repaint areas up the
chain, clipped to the visible part of the scroll view; overlays are drawn
through `widget.OffsetContext`; `BuildAccessTree` reports screen bounds;
`PopupMenu.Show` uses the frame of the event being handled
(`widget.SetEventFrame`, set by the engine around the handler).

Cost is unchanged: walking 5050 widgets on a mouse move takes the same ≈78 µs,
scrolling stays one operation. With a zero offset behaviour is the same, with
one deliberate difference: the cursor outside the scroll view no longer
highlights children sticking out past its edge — hit testing behaved so
before, hover now agrees with it.

Remaining (documented in the code): hover after a wheel scroll updates on the
next mouse move; a menu opened from the KEYBOARD by a widget inside a scroll
view is clamped to the canvas the old way; overlays of scrolled children are
drawn without the soft shadow; a custom container that keeps children past
`Base.AddChild` translates repaint areas inexactly.

### Printing and saving to PDF — v3.29

The engine had no printing. An application had to call the OS print subsystem
and lay the document out a second time, with other fonts and other layout, so
the screen and the paper disagreed.

The model is the screen's: the application draws each page into an image with
the same `DrawContext` (`engine.New` + `SetScale` + `RenderOnce` renders a
widget tree into an `*image.RGBA`) and the engine sends the finished pages to a
printer. Package `printing` (`github.com/oops1/headless-gui/v3/printing`) holds
paper, orientation, margins, printers and printing; package `output/pdf` builds a
PDF from page images with no dependencies.

```go
setup := printing.NewPageSetup(printing.A4)  // portrait, margins 20/10/10/10 mm, 300 dpi
setup.Margins = printing.UniformMargins(15)  // mm
w, h := setup.SheetSize()                    // 2480 x 3508 px for A4 at 300 dpi
content := setup.ContentRect()               // draw the content here

job := printing.Job{Name: "Report", Setup: setup, Pages: pages} // pages []image.Image

err := printing.SavePDF("report.pdf", job, printing.PDFOptions{Title: "Report"})

target := printing.Target{Copies: 2}              // Printer "" = the default printer
if printing.HasPrintDialog() {
    target, err = printing.PrintDialog(0, job)    // 0 = the process's active window
}
err = printing.Print(target, job)                 // blocks for seconds: call in a goroutine

printers, err := printing.Printers()              // []printing.Printer
def, err := printing.DefaultPrinter()             // errors.Is(err, printing.ErrNoPrinter)

// Rendering a widget tree into a page: a canvas scaled by dpi/96.
k := float64(setup.EffectiveDPI()) / 96
eng := engine.NewOffscreen(int(float64(w)/k+0.5), int(float64(h)/k+0.5), 1)
defer eng.Stop()
eng.SetScale(k)
eng.SetRoot(pageUI)
img := eng.RenderOnce() // may differ from w x h by one pixel
```

**Offscreen engine — v3.34.** `engine.New` takes three services shared by the
process: it becomes the text measurer outside `Draw` (`widget.MeasureUIText`,
`MeasureUITextFont`, rune positions, font metrics), the rasterizer of SVG
`<text>`, and it reports its size as the screen (`widget.SetScreenBounds`,
which keeps popup menus on screen). A print job, thumbnail or export made with
`New` took all three from the window: while the job ran, the window measured
text with the job's canvas, fonts and scale, and its menus were clamped to the
page size. `engine.NewOffscreen(w, h, fps)` is the same engine without them —
same default fonts, `RenderOnce`, `Start`, `Stop`; `Engine.IsOffscreen()`
tells which one it is. Inside `Draw` widgets measure with `ctx.MeasureText` of
their own canvas, so the page is exact; measurement OUTSIDE `Draw` by widgets
of an offscreen engine is answered by the process's measurer (the window), so a
font known only to the job falls back to the default there.

The named-font registry of a canvas is now under a lock (`engine/fonttable.go`)
and `fontRev` is atomic: `RegisterFont` from the application's goroutine while
another goroutine measures or draws is no longer a data race (it was, under
`-race`, in Notepad's PDF export).

Paper: `A3 A4 A5 A6 Letter Legal Tabloid Executive`, `CustomPaper(name, wMM, hMM)`;
`Portrait` (zero value) / `Landscape`. A page image covers the WHOLE sheet;
margins only tell where to draw (`ContentRect`), so printers with different
unprintable edges print the same. `PageSetup.DPI == 0` means `DefaultDPI` (300);
valid 72..1200 (`MinDPI`, `MaxDPI`); the page area is capped by `MaxPagePixels`
(100<<20), so A4 at 1200 dpi fails `Validate` with `ErrBadSetup`. The engine
canvas has its own cap (`engine.MaxCanvasPixels`). `PagePixels(paper, orientation,
dpi)` gives the pixel size without a `PageSetup`. A page image need not match
`SheetSize()`: the physical size is then taken from the pixels and the dpi.

`Target`: `Copies` 0 means 1, max `MaxCopies` (999); `FromPage`/`ToPage` from 1,
inclusive, zeros mean all, an upper bound past the job is clipped, a start past
it or an inverted range is an error; `Collate` — copies in sets (1 2 3, 1 2 3).
`Print` returns when the system has accepted the job, not when paper comes out.
Errors (`errors.Is`): `ErrNoPrinter`, `ErrCanceled`, `ErrNoPrintService` (no
CUPS), `ErrUnsupported`, `ErrNoPages`, `ErrBadSetup`.

PDF: `printing.SavePDF(path, job, opts)` / `printing.WritePDF(w, job, opts)`;
`PDFOptions` = `pdf.Options{Title, Author, Creator, Created, JPEGQuality}`.
Lower level: `pdf.NewWriter(w, opts)`, `AddPage(pdf.PageFromImage(img, dpi))`,
`Close()`, or `pdf.Write(w, pages, opts)`; pages are written one by one, so
memory does not grow with the document. Each page is one image: text is
rasterized, matches the screen, and cannot be selected or searched in the file.
Flate (lossless) by default, `JPEGQuality` 1..100 switches to JPEG (blurs small
text). Transparency is blended with white. A zero `Created` is not written, so
equal input gives equal bytes. Page size 3..14400 pt. `SavePDF` writes a
temporary file and renames it. `SavePDF` works on every platform.

Platforms: Windows — the system `PrintDlgEx` dialog (`HasPrintDialog()` true) and
GDI printing: the page lies on the sheet allowing for unprintable margins, is
rotated when the orientation differs, and goes to the driver in bands; driver
settings from the dialog (duplex, tray) reach the `Target` only via
`PrintDialog`. Linux — IPP 2.0 to CUPS over `/run/cups/cups.sock` or
`localhost:631` (`CUPS_SERVER` honored), the document is `application/pdf`; no
`lp`/`lpr`; no system dialog (`PrintDialog` returns `ErrUnsupported`, build your
own from `Printers()`); the `org.freedesktop.portal.Print` portal is not done
(it needs a file descriptor over D-Bus, which the engine's D-Bus client cannot
pass), so a sandbox without CUPS gets `ErrNoPrintService`. macOS and others —
`ErrUnsupported`. Not tested on a real printer: IPP checked against the RFC and
a test server, GDI run on "Microsoft Print to PDF"; a live test is enabled by
`HEADLESS_GUI_PRINT_LIVE`.

### TextBox for code — v3.29

An editor on this engine found `TextBox` unfit and began writing its own:
the font could not be chosen, Tab moved focus, a line could not be coloured, a
long line had no horizontal bar, and every frame copied the whole document.

```go
type commentStyler struct{}

func (commentStyler) LineSpans(line int, text string) []widget.Span {
    if strings.HasPrefix(strings.TrimSpace(text), "//") {
        return []widget.Span{{From: 0, To: utf8.RuneCountInString(text),
            Style: widget.Style{Color: color.RGBA{0, 128, 0, 255}}}}
    }
    return nil
}

tb := widget.NewTextBox("")
tb.Wrap = false                      // no wrap: a horizontal scrollbar appears
tb.FontName = widget.BuiltinFontMono // or a name from RegisterFont; XAML FontFamily
tb.AcceptTab = true                  // Tab inserts '\t'; XAML AcceptsTab="True"
tb.TabSize = 4                       // spaces per tab stop; 0 with AcceptTab means 4
tb.SetStyler(commentStyler{})        // nil removes it
tb.InvalidateStyles()                // ask again when the rules changed without a text edit
```

`Styler.LineSpans(line int, text string) []widget.Span`: `line` is the LOGICAL
line (split on `\n`, from 0, wrapping does not change it), `text` the line
without its newline. `Span{From, To int; Style Style}` — a rune range in `text`.
`Style{Color, BG color.RGBA; Face string}`: zero fields mean "as the widget has
it"; `Face` is a registered font name (bold/italic are separate fonts).
Overlapping spans are merged field by field (colour from syntax, background
from search); ranges outside the line are clipped, empty or inverted ones are
ignored. Only visible lines are asked; answers are cached until the text changes
or `InvalidateStyles()`; `LineSpans` is called WITHOUT the widget lock (it may
call `TextBox` methods), must be fast, and may depend only on `(line, text)` and
on application state that `InvalidateStyles()` announces. A late answer for
stale text does not enter the cache.

Layout, caret and selection are measured with the widget's font (`FontName`),
not `Style.Face`, so the faces must match in glyph width. `Tab` without
`AcceptTab` is focus traversal as before; `Ctrl+Tab` always stays navigation;
`Shift+Tab` inserts nothing and does not remove an indent; a read-only box does
not accept Tab. The horizontal scrollbar (thumb drag, horizontal wheel,
Shift+wheel) shares geometry with `DiffView`; the sideways offset is now capped.

The frame no longer copies the document: only visible lines are taken under the
lock, drawing is lock-free. Undo stores edits (position, removed, inserted)
instead of 200 text copies. Measured on 100k lines: a frame 2.3 ms / 16.7 MB ->
0.13 ms / 0.28 MB, a keystroke 28 ms / 22 MB -> 11 ms / 0.12 MB. A zero-width
selection no longer leaves a phantom anchor (it led to a slice panic after
Ctrl+Backspace and typing). Context-menu edits and IME input undo as one edit;
`SetText` with different text clears the history. Without font, styles and tab
the frames are bit-identical to the previous ones.

### UI Automation Text pattern on Windows — v3.29

The previous section ("Text accessibility — v3.28") gave Windows only the Value
pattern: the whole text as one string. NVDA and Narrator could not read the
current line or the word under the caret and announced the whole text again.

`ITextProvider` and `ITextRangeProvider` are now implemented. This is bridge
behaviour: an application only implements `widget.AccessTextProvider`. The
units are character (`\r\n` is one), word (with the spaces after it), line and
paragraph, document; format and page are treated as the document. The units
cover the text with no gaps and no overlaps (property-tested on eight texts). The
bridge raises `TextSelectionChanged` and `TextChanged` for the focused field —
NVDA does not follow the caret without them. A password field has no pattern.

A range is selected through the new optional interface:

```go
type AccessSelectionSetter interface {
    // from <= to in runes; the caret goes to `to`; from == to clears the
    // selection and places the caret; false — the widget declined.
    AccessSetSelection(from, to int) bool
}
```

`TextBox` and `TextInput` implement it. Without it, `Select` on a non-empty
range places the caret at the end of the range (through `AccessCaretSetter`),
and a screen reader cannot highlight the word it has read. Ranges are separate
COM objects, freed on the last `Release` (a client pointer is only a registry
key: a call on a freed one gives `E_FAIL`); ranges a client never released are
dropped when the bridge stops. Simplified, and said in the code: no text
attributes (a legal UIA answer); one bounding rectangle per element; wrapping by
widget width is not a line; ranges do not follow edits and are clamped to the
text length. Linux already had `org.a11y.atspi.Text`.

### Rich text on the clipboard — v3.29

The clipboard held only plain text: headings, bold and links copied from an
application pasted into Word and a mail client as bare text.

```go
widget.SetClipboardHTML(`<h1>Title</h1><p><b>bold</b> text</p>`, "Title\n\nbold text")
html, ok := widget.ClipboardHTML() // ok == false: no HTML on the clipboard, or the provider cannot do HTML
text := widget.ClipboardGetText()  // the fallback

// optional interface of a provider (ClipboardProvider itself did not change):
type ClipboardHTMLProvider interface {
    SetHTML(html, plain string)
    GetHTML() (html string, ok bool)
}
```

Both representations are put at once; the receiver chooses. `plain` is required:
without it pasting into Notepad or a terminal gives nothing. A provider without
`ClipboardHTMLProvider` (a custom one) gets only `plain` from `SetClipboardHTML`.
`ClipboardHTML` never returns plain text posing as HTML (it would have to be
escaped and the caller could not tell the two apart). `UseMemoryClipboard()`
supports HTML too.

Windows: the "HTML Format" format; its header offsets are counted in UTF-8
BYTES (a wrong count is the usual reason Word pastes garbage; building and
parsing are pure functions `widget.BuildCFHTML(fragment) []byte` and
`widget.ParseCFHTML(data) (fragment string, ok bool)`, tested on Cyrillic, CJK
and emoji). Both formats are put in one opening of the clipboard. Wayland and
X11: `text/html` next to the text types (the data-request path used to ignore the
type; it now answers what is asked); reading accepts UTF-8 (with or without
BOM) and UTF-16 (Firefox, some Qt). These two providers are installed by the
native window (`window.Run`); without a native window Linux uses the xclip/xsel
fallback, which has plain text only. macOS: plain text only (`pbcopy` cannot do
`public.html`, and `NSPasteboard` is unreachable without CGO).

### Files on the clipboard — v3.30

The engine's file manager (WinLine Explorer) copied files into a list inside
the program: only text reached the system clipboard, so files never got to
another file manager or to the RDP client, and nothing came back.

```go
widget.ClipboardSetFiles(paths []string, cut bool)            // replaces the clipboard; paths as text next to it
paths, cut, ok := widget.ClipboardFiles()                     // ok == false: not files, or the provider cannot do files

type ClipboardFilesProvider interface {                       // optional, like ClipboardHTMLProvider
    SetFiles(paths []string, cut bool)
    GetFiles() (paths []string, cut bool, ok bool)
}
```

`ClipboardFiles` may block for a long time (up to 10 minutes on Linux): a
remote desktop downloads client files at paste time and the channel delivers
the list when the download is done. Call it in a goroutine and `Engine.Post`
the result. A list containing any non-`file://` URI or a foreign host is not
files.

| | Windows | X11 | Wayland |
|---|---|---|---|
| put | `CF_HDROP` (UTF-16) + `Preferred DropEffect` + `CF_UNICODETEXT` (CRLF) | targets `text/uri-list`, `x-special/gnome-copied-files`, text | `wl_data_source` with the same types |
| read | `CF_HDROP` (UTF-16 parsed with bounds checks; ANSI via `DragQueryFileW`) + `Preferred DropEffect` | gnome format, then `text/uri-list` | the same; only types the offer announced |

The gnome format is read first when offered — it is the only one carrying
`cut`, with the same URIs. URIs are percent-encoded like `g_filename_to_uri`.
Pure helpers: `window/clipboard_files.go` (uri-list, gnome format),
`widget/clipboard_files.go` (DROPFILES, DropEffect). macOS has no provider —
the text fallback applies. Verified on Windows in both directions against
Windows PowerShell `Get-Clipboard -Format FileDropList` / `Set-Clipboard
-Path`; Wayland and X11 on the wire (fake compositor and X server).

### Extra drop-downs in the file dialog — v3.29

The make-up of file dialogs was fixed. An application that needed one more
field — an "Encoding" list, as in the Windows Notepad — had to write its own
file dialog whole (path, places, table, overwrite warning).

```go
var fd *widget.FileDialog
fd = mb.ShowSaveFile(widget.FileDialogOptions{
    Choices: []widget.FileDialogChoice{
        widget.EncodingChoice(),
        {ID: "eol", Label: "Line endings", Options: []string{"LF", "CRLF"}, Default: 0},
    },
}, func(path string, ok bool) { // the callback signature did not change
    if !ok {
        return
    }
    enc, _ := fd.Choice(widget.EncodingChoiceID) // option index
    eol, _ := fd.ChoiceText("eol")               // option text
    save(path, enc, eol)
})
fd.SetChoice("eol", 1) // programmatic; false on an unknown id or an index out of range
```

`FileDialogChoice{ID, Label, Options []string, Default int, LabelKey string}`.
`LabelKey` is a localization key, wins over `Label` and follows the language
live; `Options` are never translated. `ShowOpenFile`, `ShowSaveFile` and
`ShowPickFolder` return the `*widget.FileDialog`; declare `var fd` first, the
callback refers to it (it is called after the user's choice, never
synchronously). `Choice(id) (int, bool)` and `ChoiceText(id) (string, bool)`
return `false` for an unknown id; the values stay readable after the dialog
closes, cancel included (look at `ok`). A list without options is dropped, a
`Default` out of range becomes 0, with repeated ids the first answers.

`widget.EncodingChoice()`: id `widget.EncodingChoiceID` ("encoding"), label key
`dlg.file.encoding`, options UTF-8, UTF-8 (BOM), UTF-16 LE, UTF-16 BE,
Windows-1251; indices are the constants `EncodingUTF8` (default),
`EncodingUTF8BOM`, `EncodingUTF16LE`, `EncodingUTF16BE`, `EncodingWindows1251`.
The engine only asks: it neither reads, writes, converts nor detects encodings.
Append new options at the END so the constants stay valid.

Layout: the lists go under the "name + file type" row, right-aligned; the ones
that do not fit go to the next row and the dialog grows by a row (in
`ShowPickFolder` the first row takes the empty name row). Without `Choices`
sizes and the widget set are unchanged.

### Opening links and files with the system — v3.29

The engine could not open a link in a browser, a file in its program, or show a
file in the file manager; each application did it its own way (on Linux, a
`xdg-open` subprocess). A zero-dependency package has no external utilities.

```go
err := window.OpenURL("https://example.com")  // errors.Is(err, window.ErrOpenScheme)
err = window.OpenFile("/path/report.pdf")     // errors.Is(err, window.ErrOpenLaunchable)
err = window.RevealFile("/path/report.pdf")   // select in Explorer / the file manager
// window.ErrOpenUnsupported — a platform with no implementation
```

`OpenURL` allows only `http`, `https`, `ftp`, `mailto`, `file`; anything else is
`ErrOpenScheme` (an application that shows links from an untrusted document would
otherwise open `javascript:` or a foreign protocol handler). Also rejected: control
characters anywhere in the link (`java\tscript:`), no scheme, `http(s)`/`ftp` with no
host, `file://` with a host other than `localhost` (on Windows a trip to the
network with a password hash), and `file://` pointing at a program file
(`ErrOpenLaunchable`). The scheme case is ignored; `mailto:` with no recipient is
allowed. `OpenFile` needs an existing path (made absolute) and rejects program
files: `.exe .com .scr .pif .cpl .bat .cmd .ps1 .psm1 .psd1 .vbs .vbe .js .jse
.wsf .wsh .hta .msi .msp .msc .reg .lnk .url .appref-ms .gadget .jar .app
.command .workflow .desktop`. This guards against an application's mistake and
is not a security boundary (the list is finite). `RevealFile` needs an existing
path, does not run the file, so it has no program ban; if the manager cannot
select, at least the folder opens. Success means "the system accepted the
request"; the call may block for seconds on a cold portal start — call it from a
goroutine, not from a frame handler.

Windows: `ShellExecuteW` with the `open` verb; reveal — Explorer with
`/select,"path"`. Linux: the `org.freedesktop.portal.OpenURI` portal through the
engine's own D-Bus client; reveal — `FileManager1.ShowItems` (without it the
folder is opened). `xdg-open` is the last resort only when the bus has no portal
at all and the binary is in `PATH`; if the portal answered with a refusal,
`xdg-open` is NOT called (it would bypass the sandbox policy). macOS:
`/usr/bin/open` (`-R` to reveal). Other platforms: `ErrOpenUnsupported`. The
system calls are replaceable in tests (they check what would be called, and open
nothing).

### Font metrics — v3.29

Ascent, descent and line gap lived only inside the engine (`DrawText` sets the
baseline by them). Notepad parsed TTF files for one number and risked
diverging in rounding; a widget with mixed point sizes in one line could not
align them on a common baseline.

```go
m := widget.MeasureUIFontMetrics(11, "")      // sizePt, family ("" = default font)
m.Ascent, m.Descent, m.LineGap, m.Height      // ints, LOGICAL pixels; Height = sum of the three

big := widget.MeasureUIFontMetrics(18, "")
small := widget.MeasureUIFontMetrics(10, "")
base := y + max(big.Ascent, small.Ascent)     // y — top of the line
ctx.DrawTextSize("12", x, base-big.Ascent, 18, fg)
ctx.DrawTextSize(" px", x+widget.MeasureUIText("12", 18), base-small.Ascent, 10, fg)
```

`Ascent` is exactly how far `DrawText` lowers the baseline from the `y` it is
given (same cache). `Descent` is positive, `LineGap` is never negative. The
argument order matches `MeasureUITextFont(text, sizePt, family)`: size, then
family. An empty or unregistered family is the default font, as when drawing. It
goes through the measurer the engine registers (`Measurers.FontMetrics`), like
widths; an application-supplied measurer without that field keeps working. On
HiDPI physical values are converted to logical by rounding to the nearest (not
up as widths are). With no registered engine an approximation from the point
size is returned (non-zero, so layout does not divide by zero); `sizePt <= 0`
gives all zeros.

### Horizontal scrolling in ScrollView — v3.29

`ScrollView` scrolled only vertically: content wider than the area was clipped,
and its right part could not be reached. The horizontal bar existed only inside
`DiffView` and `MergeView`.

```go
sv := widget.NewScrollView()
sv.ContentHeight = 2000
sv.ContentWidth = 1600   // full content width; 0 (default) = no horizontal scrolling
sv.AddChild(wide)        // children are laid out by the application in content coordinates
sv.ScrollX() int
sv.SetScrollX(300)       // clamped to [0, ContentWidth - area width]; 0 while ContentWidth is 0
sv.ScrollXBy(-40)        // positive = right
```

Everyone who leaves `ContentWidth` zero gets the previous behaviour (checked by
comparing the draw-op stream and mouse answers with the old version, in the
normal and the classic theme); the default cannot be "width by children", or a
bar would appear for everyone who holds wide children. The bar is drawn with the
`hscrollbar.go` geometry shared with `DiffView`. The two bars depend on each
other (the vertical one may be needed only because the horizontal one took
height) and that is resolved in one place. Wheel: a horizontal delta scrolls
sideways; Shift + the vertical wheel does too, but only when the bar exists
(otherwise Shift still means nothing). The thumb drags with no jump, a click off
the thumb jumps and starts dragging at once, and a click on the bar does not reach
the child under it. There is no horizontal inertia (the flywheel holds one speed
and one edge flag; sideways paging is rare): a pixel delta is applied at once.
Children are clipped in a nested way (see "Nested clipping").

### System theme: light or dark — v3.29

The engine could not learn which theme the person had picked in the OS.
Applications (the WinLine Explorer and Notepad) each carried a copy of the
registry / settings reading.

```go
switch window.DetectSystemTheme() { // any goroutine, before a window exists
case window.SystemThemeDark:
case window.SystemThemeLight:
case window.SystemThemeUnknown:     // not "light": take the application's own default
}

win.SetOnSystemThemeChanged(func(t window.SystemTheme) {
    // on the engine goroutine; change the palette and repaint right here
})
win.SetOnSystemThemeChanged(nil) // unsubscribe
```

`SystemTheme` has `String()` ("light", "dark", "unknown"). The callback fires
only for a real change: repeats are filtered (Windows sends several
`WM_SETTINGCHANGE` per switch, the portal signals accent changes too) and
`Unknown` is not a change, so the window does not jump to a default and back. It
may be set before `Run` (the subscription starts when the window is created) and
while running; each window, `OpenWindow` ones included, has its own callback. The
platform subscription is lazy: it exists only while a callback is set, and is
removed when the window closes. The engine recolours nothing by itself.

Windows: the `AppsUseLightTheme` value under Personalize (not
`SystemUsesLightTheme`, which is the taskbar and Start); a change is the
broadcast `WM_SETTINGCHANGE` with `ImmersiveColorSet`, after which the value is
re-read. Before Windows 10 1607 — `Unknown`. Linux (X11 and Wayland alike): the
`GTK_THEME` variable (`Name:dark` / `Name:light`; a name containing `dark` is
dark; any other name decides nothing), then the portal
`org.freedesktop.portal.Settings` key `color-scheme` (1 dark, 2 light, 0 no
preference = `Unknown`) through the engine's own D-Bus client — no external
utilities; the change is the `SettingChanged` signal, with one bus subscription
per process. With a bus but a silent portal the first call may take seconds;
without a bus it is instant. macOS: always `Unknown` and no observation: the theme
can be read, but the purego calls cannot be verified, and the engine will not
lie.

### Timers on the engine goroutine: After and Every — v3.29

An application had `Post` and nothing else: for "in half a second" or "once a
minute" each place started a `time.AfterFunc` and called `Post` from it. Two
failures: a timer had to be cancelled when the window closed and some code did
not, so goroutines outlived the exit and woke a dead engine; and `time.Ticker`
sends a tick without waiting for the handler, so a slow handler piled calls into
the queue that then ran all at once.

```go
t := eng.After(400*time.Millisecond, func() { tip.Show() }) // *engine.Timer
save := eng.Every(time.Minute, func() { doc.Save() })
save.Stop()      // idempotent, any goroutine, also from the handler itself
save.Stopped()   // will not fire again
```

The handler runs on the engine goroutine through the same path as `Post`: no
engine locks, the widget tree may be touched. The timer holds no goroutine while
it waits (the system timer only puts a call into the queue). For `Every` the
next run is scheduled AFTER the handler returns: a slow handler slows the
repetition instead of piling up a queue, and the period drifts by the handler's
time. A period under 1 ms is rounded up to 1 ms. `Stop` also works after the
timer has fired but before the call has run (the queued call checks the flag); a
call already running is neither interrupted nor waited for. `d <= 0` in `After`
means the nearest pass of the loop, like `Post`. Stopping the engine stops all its
timers; a timer created on a stopped engine, or with a nil function, comes back
already stopped; creation and `Stop` of the engine are ordered by one lock, so a
timer cannot outlive the engine. Without `Start()` the queue is drained only by
`RenderOnce`, `RenderFrameNow` and `Flush`, so the handler of a non-running
engine runs when the application asks for a frame.

### Title tabs: scrolling and reordering — v3.29

Title tabs (`Window.EnableTitleTabs`, `AddTitleTab`) shrank to a limit and the
rest were not drawn: with ten documents some were invisible AND unreachable. They
could not be reordered either. With a full strip the "+" and "v" buttons were
pushed past the right edge, so a new tab could not be opened.

```go
w.EnableTitleTabs()
w.OnTitleTabMoved = func(from, to int) { /* after a move: by drag or MoveTitleTab */ }
ok := w.MoveTitleTab(0, 2) // false: wrong indices or from == to; the active tab stays the same tab
w.ScrollTitleTabs(+60)     // dx points, positive = right
```

The strip scrolls by the wheel over the title bar (the vertical wheel too; the
event is not consumed when everything fits, so the wheel over the content belongs
to the content), by `ScrollTitleTabs`, and by itself to the active tab when it
leaves the edge (selected from the keyboard, or shifted after a neighbour
closed). The wheel step is 60 points. Tabs outside the visible part are skipped,
and the strip is clipped to its own bounds so the outermost tabs do not run over
the window buttons. Dragging swaps tabs on the fly, as Windows Terminal does:
neighbours step aside before the button is released, so the release moves
nothing; `OnTitleTabMoved` therefore fires on every swap during a drag. A press
becomes a drag only after 5 points of movement, or a trembling hand would
reorder on a click. The "+" and "v" buttons now stand after the tabs but not
beyond the space reserved for them.

### Nested clipping — v3.29

`SetClip` REPLACES the clip rectangle and `ClearClip` removes it whole. A widget
that narrowed the clip to its own part removed, at the end, the narrowing of its
parent TOO. Nested inside another, it let the rest draw past the edges: scroll
content spilled outside the scroll view, a grid inside a scroll view drew over
its neighbours. Applications worked around it by hand.

```go
func (w *MyPanel) Draw(ctx widget.DrawContext) {
    defer widget.PushClip(ctx, w.Bounds())() // intersect with the current area
    // ...draw...
}

r := widget.ClipRect(ctx, item.Bounds()) // intersection with the visible area, changes nothing
if r.Empty() { return }                  // skip what is invisible
```

`PushClip(ctx, r) func()` narrows by the INTERSECTION with the current area and
returns a function that restores the PREVIOUS one (calling it twice is safe; a
nil `ctx` gives a no-op). A free function rather than a `DrawContext` method:
applications implement `DrawContext` (wrappers, recording contexts in tests), and
a new interface method would break their build. The built-in widgets use it.
Containers (`ScrollView`, the virtualizing list) set the clip anew before EACH
child — a foreign child may still remove it the old way. `TreeView` and
`DataGrid` live in separate packages with a narrow context; they get nesting in
the adapter: their `SetClip` intersects with the outer area and `ClearClip`
restores it. Also fixed along the way: the ruler and the horizontal bar in
`DiffView` and `MergeView` were drawn inside the clip left by the last pane,
held only by `SetClip` wiping it whole.

### Wayland: menus outside the window (xdg_popup) — v3.28

Overlays on Wayland were drawn INSIDE the window: the engine's canvas is the
window, and whatever did not fit was cut by its edge. In a narrow window (the
WinLine notepad, 490 px by reference) the File menu lost its right edge and a
context menu its bottom items. On Win32 and X11 `popupHost` carries an
overlay into a separate OS window by screen coordinates.

On Wayland there is no such path at all: a client does not know where its
window stands, and a point on the screen cannot be addressed. The protocol
has `xdg_popup` for this — a child surface placed RELATIVE to its parent
through `xdg_positioner`, with the compositor making the final decision,
since only it knows where the screen ends. Hence a second host,
`childPopups` (`window/popup_host_child.go`): instead of creating windows it
asks the backend to show the popup inside its own connection.

The popup has its own surface, its own buffer in a format WITH alpha (a
cascading menu is a staircase, and the gap between its bands would be a black
rectangle in an opaque buffer), an input region built from the painted bands
(a click in the gap counts as a click outside), and an input grab on the
first popup — with it the compositor closes the menu on a click elsewhere and
says so through `popup_done`. While the grab is alive the next popups are
created as its children: otherwise the compositor answers with a protocol
error, which disconnects the client.

Input: while the cursor is over a popup the events belong to it — only
`wl_pointer.enter` names the surface, so it is remembered. Coordinates are
translated into the engine by where IT drew the overlay, not by where the
compositor put the window: the picture is the same either way.

### HiDPI on Linux — v3.28

The engine only ever asked Windows for the interface scale. On Linux nobody
asked: on a 4K monitor everything came out half the size it should be, and
the only cure was the `HEADLESS_GUI_SCALE` variable — that is, the user had
to know about it.

X11 has no system-wide scale; what every toolkit uses is the `Xft.dpi`
resource in the X server's resource database (the RESOURCE_MANAGER property
of the root window), where 96 dots per inch mean 1. That is what is read,
with `GDK_SCALE` as a fallback. `connect` became idempotent for this: the
scale is asked before the window is created, and a second connection would
leak the first.

Wayland sizes are counted in surface units while the buffer is sent in
pixels; the scale binds them. `wl_output.scale` is the integer scale of the
output the window landed on (`wl_surface.enter` says which);
`wp_fractional_scale_v1` is the fractional scale the compositor considers
right for this surface — more precise, and 1.25 cannot be expressed as an
integer at all. With a fractional scale the buffer stays in pixels and
`wp_viewport` says how many surface units it occupies; with an integer one
`set_buffer_scale` is enough. Everything that goes into xdg-shell (window
fixation, minimum size, popup anchor and size, input regions) is converted to
surface units, and the incoming `configure` back to pixels. Damage at a scale
other than 1 goes through `damage_buffer`, which counts in buffer pixels.

### Several top-level windows in one process — v3.28

A process had one window: `Run` created it and occupied the goroutine until
it closed. Secondary windows existed internally (native modals, torn-off dock
panes), but were never exposed and are both subordinate.

`Window.OpenWindow(child)` opens a standalone top-level window: its own
taskbar button, its own engine, its own widget tree. `Run` is therefore split
in two — `bringUp` raises the window and everything attached to it, the
blocking loop stays in `Run`. Creating the second window is marshalled to the
main window's thread: on Windows the message queue belongs to the thread that
created the window. On X11 and Wayland the second window has its own
connection and gets its own pump (Wayland did not have one before).

### Text accessibility — v3.28

The content of a text field travelled to a screen reader as one string — the
element's "value", as on a slider. A screen reader reads a document in
pieces, follows the caret and announces the selection; NVDA, Narrator and
Orca all announced the field as empty.

A widget now hands over text semantics directly (`widget.AccessTextProvider`:
text, caret, selection, read-only, plus optional setters). The data is asked
of the LIVE widget rather than taken from the semantic snapshot, which is
rebuilt every 150 ms and would lag behind typing. Offsets are in runes.

Windows: the Value pattern (`IValueProvider` — read, read-only flag and
replacing the content). Linux: `org.a11y.atspi.Text` — the text and its
ranges, caret, selection, and the pieces around a position (character, word,
line) in both request forms, the old one and the current one. Multi-line text
gets its own role: document (UIA Document, AT-SPI text) against an edit field
(Edit, entry).

### Composition input (IME) — v3.28

Chinese, Japanese and Korean are not typed letter by letter: a person types
syllables, the system shows candidates, and only the chosen variant becomes
text. The engine knew nothing of this — only finished characters reached a
field — so not a single ideograph could be typed.

The composition is stored INSIDE the field's content rather than beside it:
that way it is visible without touching the drawing code, the caret stands
where it is expected, and line layout counts the typed text. Only the RANGE
is remembered separately — to replace it with the next composition and to
underline it.

Windows: the `WM_IME_*` messages were not handled at all. The composition and
the result are taken from the input context, the system's composition window
is suppressed (the field is drawn by the application, and a second one on top
would look like a fault), and the candidate window is placed under the caret.

Wayland: `text-input-v3`, where the compositor stands between client and
input method. Events accumulate and are applied in one batch on `done` —
separately, the finished text and the composition would land in the field in
the wrong order. The extension is optional: the WinLine compositor does not
have it yet, and there everything works as before.

X11 (XIM) was not done: the protocol is obsolete, in current desktops input
goes through the same text-input extensions, and the cost is out of
proportion.

### Menu item: shortcut text and mnemonics — v3.27

A menu writes the shortcut on the right — "Ctrl+S". There was no field for it,
so applications appended it to the item's own caption: it then lined up after
the caption and read as a second label, not as a hint.

```go
menu.UseMnemonics = true
menu.SetItems([]widget.MenuItem{
    {Text: "_Save", Shortcut: "Ctrl+S", OnClick: save},
    {Text: "report__2026.txt"}, // a real underscore
})
```

`MenuItem.Shortcut` is only SHOWN: the keystroke is handled by the application
through `InputBindings`, or the same combination would be declared twice and
one day the two would disagree. Markup: `InputGestureText` (the WPF name) or
`Shortcut`. The muted colour is mixed with the menu background rather than made
translucent — translucent text over a highlighted item looks dirty.

Mnemonics are opt-in through `UseMnemonics` (`PopupMenu`, `MenuBar`, and the
markup attribute): captions do contain real underscores — file names in a
context menu — and swallowing them silently is not allowed. A Cyrillic mnemonic
is matched by the physical key, because what reaches the application is the
Latin letter's code (so that `Ctrl+S` works in a Russian layout) and the engine
does not know which layout is active; the layout assumed is ЙЦУКЕН, and the
event's character, when a backend supplies it, wins over that guess.

The menu bar holds no focus while keys go to the focused widget, so Alt+letter
is handed to it by the application: `MenuBar.ActivateMnemonic(e)` reports
whether such a mnemonic was found. Since v3.29.1 the bar then takes the focus
from the engine delivering the current key (`internal/focusreq`; the engine
serves such requests during `SendKeyEvent`/`SendMouseButton`) and gives it back
to the previous widget when the menu closes — Escape, a picked item, a click
elsewhere. Called outside key or click handling it cannot ask, so focus the bar
with `eng.SetFocus`.

**Focus for a panel opened outside an engine event** (a Win key caught by the
shell, a timer, another goroutine): `Engine.RequestFocus(w)` takes focus the way
a widget's own request does (the previous owner is remembered) and
`Engine.ReturnFocus(w)` gives it back unless focus has moved on. Safe from any
goroutine: on the frame goroutine (or an engine that is not running) it runs at
once, otherwise it is queued with `Post`, in call order. For flyouts:
`desktop.FocusOnOpen(flyout, eng, target)` and `FlyoutManager.FocusOnOpen(eng)`
(the `desktop.FocusRequester` interface is implemented by `*engine.Engine`).

Also since v3.29.1, `ItemsSource="{Binding X}"` on `DataGrid` and `TreeView`
passes the `*ObservableCollection` itself (it used to become a string and the
grid stayed empty); the source is set again only when the model returns a
different collection, so selection and sorting survive unrelated property
changes. Paths inside `<TreeView.ItemTemplate>` (`{Binding Children}`,
`{Binding Name}`) are no longer resolved against the window's DataContext, so a
`HierarchicalDataTemplate` written in markup works.

### Click count in MouseEvent — v3.27

Every widget used to spot a double click on its own — its own "time of the last
click" field, its own 400 ms threshold and its own notion of "the mouse did not
move". The threshold was invented rather than taken from the system, where the
user sets it in the mouse settings. Nobody handled a triple click, and on
Windows the second press arrives as a separate message (`WM_LBUTTONDBLCLK`) and
looked like an ordinary first one.

`MouseEvent.Clicks` now carries the number of the press in the series: 1, 2, 3
and on. The engine counts it once for the whole tree (`countClick` in
`engine/events.go`); a series is broken by another button, a cursor farther than
four points, or a pause longer than the interval. A release carries the number
of its own press, so a release handler sees the same series as the press.

The interval comes from the system: `window.Run` reports it through the
optional `doubleClickSink` (Win32 `GetDoubleClickTime`), and `SetDoubleClickTime`
is public for anyone feeding the engine themselves. X11 and Wayland have no
system-wide value in the protocol itself — it lives in desktop settings
(XSettings, KDE's own key) — so there the default stays.

`Clicks == 0` means the event did not come from the engine (a direct
`OnMouseButton` call from a test or an application with its own dispatch): the
widget then counts the series itself through `clicksOf` (`widget/clicks.go`),
exactly as before. First use: a triple click selects the line in `TextBox` and
the whole field in `TextInput`.

### Window areas: Snap Layouts and edge snapping — v3.26

Engine windows are borderless: the title bar and its buttons are drawn by the
application, and the system knows nothing about them. So on Windows 11 the
window did not snap to screen edges (it was moved by setting its position
from mouse deltas), and hovering the maximise button showed no snap layouts —
the system offers those only over an area declared as the maximise button.

```go
win.SetHitTest(func(x, y int) window.HitArea {
    switch {
    case closeRect.Contains(x, y): return window.HitCloseButton
    case maxRect.Contains(x, y):   return window.HitMaxButton
    case minRect.Contains(x, y):   return window.HitMinButton
    case y < titleBarH:            return window.HitCaption
    }
    return window.HitClient
})
```

Coordinates are client-side and logical — the same grid the application draws
in. The resize edge wins over the buttons, otherwise the top edge of the
window could not be grabbed. Events over a declared button are translated
back into ordinary mouse events, so the application keeps drawing its own
hover and pressed states. Without the callback nothing changes, and backends
without the notion (X11, Wayland, macOS) ignore it.

### X11: system move, resize and cursor shapes — v3.26

`SetResizable` was an empty function and there were no cursor shapes at all:
the window always showed an arrow, and its edge looked like its middle.

Moving and resizing now go through `_NET_WM_MOVERESIZE` — the client asks the
window manager to start, and the manager drives the window as it does its
own, with edge snapping. It is the same `interactiveMover` the Wayland backend
implements, so `Window.BeginMove` and `BeginResize` work on X11 too.

Cursor shapes come from the «cursor» font — present in every X server, with
no theme, no Xcursor and no external files. Each shape is created once and
cached: the cursor changes hundreds of times a minute.

### Horizontal wheel on Windows and X11 — v3.26

`WM_MOUSEHWHEEL` was not handled, and on X11 buttons 6 and 7 — which is how
the X server reports sideways scrolling, just as 4 and 5 report vertical —
were dropped. Both now deliver `dx` the same way `dy` has always been
delivered.

### Text input on Windows: surrogates and AltGr — v3.26

Two things got lost on the way from the OS to the widget.

Windows sends text in UTF-16, so a character outside the basic plane — an
emoji, a rare script — arrives as TWO WM_CHAR messages, a high and a low
surrogate. On their own they mean nothing, and the application used to
receive a pair of garbage runes. They are now assembled into one rune; an
unpaired surrogate is dropped rather than shown as a replacement mark.

Windows also reports AltGr as Ctrl+Alt. Widgets treat any Ctrl as a shortcut,
so a character typed with AltGr (on a German layout AltGr+Q gives «@»)
disappeared. A text event now arrives without that pair — it is typing, not a
shortcut. A lone Ctrl or Alt is left alone: Ctrl+V pastes, it does not type.

### System move and resize for any root — v3.26

System dragging was wired up automatically only when the root was a
widget.Window, so an application with its own root had to assert
win.Native() to an unexported interface — reaching into the engine.

    win.BeginMove() bool          // drag the window as the system does
    win.BeginResize(edges) bool   // edges: widget.NativeEdgeTop и далее

Both answer false when the backend cannot do it (Win32 and macOS today; X11
implements them since v3.26 through `_NET_WM_MOVERESIZE`, Wayland through
`xdg_toplevel.move/resize`) or
the system refused — then the application moves the window itself through
SetPosition, exactly as before.

### Alt, F10 and Alt+key — v3.26

`WM_SYSKEYDOWN` / `WM_SYSKEYUP` / `WM_SYSCHAR` were not handled at all, so on
Windows `ModAlt` was never set and F10 never arrived. They now go the same way
as the ordinary key messages, and what the window does not consume is passed
to `DefWindowProc` — `Alt+F4` and `Alt+Space` keep working as the system
intends. The error beep on `WM_SYSCHAR` is suppressed (except for `Alt+Space`,
which belongs to the window menu): the application has already received that
combination as a key event.

`widget.KeyAlt` is the gesture «Alt pressed and released with nothing in
between» — how a menu bar is opened on Windows. As a modifier Alt still
travels in `KeyEvent.Mod` and has no event of its own: `Alt+F` is `KeyF` with
`ModAlt`, not two keys in a row.

The gesture is recognised in `surface`, once for all three backends, so it
works the same on X11 and Wayland. Auto-repeat of a held Alt neither cancels
nor multiplies it, and any other key in between turns it into a plain
combination.

F10 is an ordinary key with its own code (`KeyF10`) — the application opens
its menu itself.

**Since v3.29** the window also swallows the keyboard entry into its system
menu. On releasing F10 or a lone Alt, `DefWindowProc` sent `WM_SYSCOMMAND` with
`SC_KEYMENU` and `lParam == 0`; the engine window has no menu bar of its own,
but the modal menu loop still started and took the keyboard — the application
got `KeyF10`/`KeyAlt`, highlighted «File», and the next ↓ (even Escape) never
reached it. Now exactly that entry returns 0 (`window/syskeymenu.go`);
`Alt+Space` comes with `lParam == ' '` and still opens the window menu,
`Alt+F4` goes through `SC_CLOSE` and is untouched. Verified with real key
presses in a live window.

### Clipboard on Linux without external tools — v3.26

The Linux clipboard ran `xclip` or `xsel` as a subprocess — a tool that may
not be installed at all (the WinLine package has none), and the copied text
lost its trailing newline on the way (`TrimRight`). Under Wayland it did not
work at all.

Both are native now, and the window registers the right provider itself as
soon as the connection is up:

- **Wayland** — `wl_data_device`. Copying creates a `wl_data_source`,
  advertises `text/plain;charset=utf-8`, `text/plain`, `UTF8_STRING` and
  `STRING`, and hands it over as the selection; the compositor then asks us
  for the data (`send` with a file descriptor) for every paste. Pasting asks
  the offer for data and reads the pipe. `set_selection` needs the serial of
  recent input, so the engine now records it from keys, buttons and pointer
  enter.
- **X11** — real selections: we own `CLIPBOARD` and answer `SelectionRequest`
  (including `TARGETS`), and paste goes through `ConvertSelection`.
  `xclip`/`xsel` stay as a fallback for the case where there is no window yet.

Reads do not hang the UI: the owner is another process and may never answer,
so a paste gives up after 300 ms and returns empty.

The trailing newline is preserved — copying `"abc\n"` and pasting gives
`"abc\n"` back.

`SetHTML` is not implemented yet: ask when you need it.

### Key auto-repeat — v3.26

Under Wayland the repeat of a held key is the CLIENT's job: the compositor
sends the press and the release and says nothing in between. So holding an
arrow or Backspace fired exactly once, and text could not be edited in a
WinLine session at all.

The Wayland backend now runs its own repeat timer, with the rate and delay the
compositor reports (`wl_keyboard.repeat_info`, protocol version 4; older ones
get the usual desktop defaults — 500 ms, then 25/s). A rate of `0` means the
compositor turned repeat off, and that is honoured. The repeat stops on
release, on a new key (only one key repeats, the last one), on focus loss and
on close. Modifiers are never repeated.

`KeyEvent.Repeat` marks a repeat on every platform (on Windows from bit 30 of
`lParam`; on X11 since v3.34 from the `KeyRelease`+`KeyPress` pair of equal time, see
"v3.34 additions"). It matters where a press toggles something: holding Caps Lock must
not flip it twenty-five times a second. For typing and caret movement the
repeat IS what is wanted, so most handlers ignore the flag.

`wl_seat` is now bound at version 4 instead of 1 — that is what carries
`repeat_info`. Higher versions are not taken: they oblige the client to handle
`wl_pointer.frame` and the rest of the event grouping, which this backend does
not do.

### Full key table — v3.26

The engine handed the application codes only for navigation keys, A/C/V/X/Y/Z
and F1–F12; everything else became `KeyUnknown` and was dropped. So
`Ctrl+S`, `Ctrl+O`, `Ctrl+N`, `Ctrl+B`, `Ctrl +/-/0` — anything an app with a
menu is built on — never arrived.

`widget.KeyCode` now covers all letters A–Z, digits 0–9, the numpad
(`KeyNumpad0`…`KeyNumpad9`, `KeyAdd`, `KeySubtract`, `KeyMultiply`,
`KeyDivide`, `KeyDecimal`), the OEM keys (`KeyOemPlus`, `KeyOemMinus`,
`KeyOemComma`, `KeyOemPeriod`, `KeyOemSemicolon`, `KeyOemSlash`,
`KeyOemTilde`, `KeyOemOpenBrace`, `KeyOemCloseBrace`, `KeyOemBackslash`,
`KeyOemQuote`), F13–F24, and `KeyCapsLock`, `KeyNumLock`, `KeyScrollLock`,
`KeyPrintScreen`, `KeyPause`, `KeyMenu` (the context-menu key). Values are the
Windows virtual-key codes, as everywhere in the engine — existing constants
are unchanged.

**Shortcuts follow the physical key, not the printed symbol.** On X11 and
Wayland the keycode is mapped through `window/keytable.go` (X11 keycode =
evdev + 8), so `Ctrl+S` works on a Russian layout, where that key prints «ы».
The character still arrives separately, from the layout: a key-down with
`Code` first, the text event with `Rune` after it.

The numpad Enter reports `KeyEnter`, like on Windows — it has no virtual key
of its own there.

Markup can name the new keys too: `Numpad5`, `Add`, `OemPlus` (or just `=`),
`CapsLock`, `Apps`, `F13`.

### ColorPicker and Swatch — v3.25

A colour field, built the way `DatePicker` is: the code can be typed straight
into the field, or the palette can be opened and a swatch picked. Until now an
app editing theme colours typed `#RRGGBB` into a plain TextBox and put a
coloured panel next to it — you edited a colour without seeing it.

```go
cp := widget.NewColorPicker()
cp.SetValue(color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255})
cp.Value(); cp.Text()                 // "#0078D7"
cp.SetPalette(cols); cp.Palette()     // empty list → the built-in set
cp.SetValueQuiet(c)                   // same, without OnChanged — for the app's own writes
cp.SetDropDownOpen(true); cp.IsDropDownOpen()
cp.OnChanged = func(c color.RGBA) {}  // also ValueChangedCommand

widget.HexColor(c)          // "#RRGGBB", upper case
widget.ParseHexColor("#0AF") // short form expands, like in CSS

sw := widget.NewSwatch(c)   // plain colour rectangle with the theme's border
sw.SetColor(c); sw.Border = ...; sw.CornerRadius = 3
```

XAML: `<ColorPicker Value="#0078D7" Palette="#000,#FFF,#E81123"/>` and
`<Swatch Color="#C42B1C" CornerRadius="3"/>`.

Keyboard as in the other fields: typing edits the code (focus selects it all),
Enter commits, Esc reverts, leaving the field commits, Alt+↓ / F4 opens the
palette. In the open palette arrows walk the swatches, Enter picks,
Ctrl+←/→ nudges the channel under the pointer; the wheel over a channel
changes it. A code that does not parse leaves the colour alone and marks the
field — it never silently becomes black.

Alpha is deliberately not supported: an interface colour is a colour, not a
film over its neighbour — the same translucent value would look like two
different colours on a light and a dark background.

### DatePicker — date field with a drop-down calendar

```go
p := widget.NewDatePicker()
p.SetSelectedDate(t) bool           // time of day dropped; false if outside the range
p.SelectedDate() (time.Time, bool)
p.ClearSelectedDate()
p.SetDisplayDateRange(start, end)   // zero bound = unbounded; selection outside is cleared
p.SetFormat("02.01.2006")           // "" → culture key date.format
p.SetFirstDayOfWeek(time.Monday)    // default → culture key date.firstDay (0 Sun … 6 Sat)
p.SetDropDownOpen(true); p.IsDropDownOpen(); p.ViewMonth(); p.Text()
p.Now = func() time.Time { … }      // "today" source for tests and golden frames
p.OnSelectedDateChanged = func(d time.Time, ok bool) {}
```

Culture is data: `date.format`, `date.firstDay`, `date.month.1..12`, `date.wd.0..6`,
`date.placeholder` in the string tables (RU, EN built in; unknown language → ISO
8601 + Monday). Parsing (`parseDateInput`) accepts no leading zeros, any of
`. / -`, 2-digit year and ISO. The month grid comes from `internal/calendar`
(`MonthGrid` with the first weekday as a parameter), shared with
`desktop.CalendarFlyout` — never duplicate it. Calendar layout is one function
(`dpLayoutFor`) for drawing and hit-testing. The overlay follows the Dropdown
pattern: `OverlayDrawer`, `OverlayBoundsProvider`, `Dismissable`, `Bounds()`
widened while open.

Events: `OnResolvedChanged(left int)`, `OnResultEdited()`,
`OnCurrentConflict(index int)`, `OnCaretMoved(line, col int)`,
`OnActiveSideChange(side)`, `OnSaveRequest()` (Ctrl+S). Commands:
`SaveCommand`, `ResultEditedCommand`, `ResolvedCommand` (`int`). Keys: F7 /
Shift+F7 navigate, Alt+1/2/3 resolve with ours/base/theirs, Alt+0 unresolves,
Tab inserts a tab into the result. Strings — `merge.*` keys; colors — the same
`Diff*`/`Syntax*` theme fields as the comparison. Demo: `cmd/mergedemo`.

Model without the view lives in `widget/diffview` (Myers line diff with
paragraph-sliding, intra-line range, tokenizer, text decode/encode). Strings:
`diff.*` keys. Colors: theme fields `DiffAddText`/`DiffDelText` (text colors, picked to ≥ 4.5:1 contrast on `InputBG` via `readableOn`), `DiffAddBG`, `DiffAddStrong`, `DiffDelBG`,
`DiffDelStrong`, `TextSelectionBG`, `Syntax*`. Only glyphs present in the Go
fonts are used (`…`, not `⋯`) so golden frames match on every OS. Demo:
`cmd/diffdemo` (`-shot dir` renders check frames headlessly).

### Editor hooks

```go
// Tab reaches the widget instead of focus traversal while AcceptsTab() is true.
// Ctrl+Tab always stays navigation.
type TabAcceptor interface{ AcceptsTab() bool }

// Children for the accessibility tree of a widget that draws its content itself.
type AccessChildrenProvider interface{ AccessChildren() []widget.AccessInfo }

widget.StateReadOnly      // new AccessState
widget.KeyS               // new KeyCode (Ctrl+S)
widget.BuiltinFontMono    // "$hg_mono" — Go Mono, registered by every engine
```

### Paths with fractional coordinates

```go
if ps, ok := ctx.(widget.PathShapes); ok {
    ps.StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness, col)
    ps.StrokePathAA(pts []widget.Point2F, thickness, closed, col) // one outline, round joins
    ps.FillPathAA(pts, col)
}
widget.FlattenCubic(x0, y0, x1, y1, x2, y2, x3, y3) []widget.Point2F // same subdivision as the canvas
```

`StrokePolylineAA` now uses the same single-outline stroker (no notches at bends).

### RichText (formatted text, display and editing)

```go
rt := widget.NewRichText()
rt.SetParagraphs([]widget.RichParagraph{{
    Runs: []widget.RichRun{
        {Text: "Big ", Size: 24, Font: widget.BuiltinFontBold},
        {Text: "small, "},
        {Text: "link", Link: "https://example.com"},
    },
    Align: widget.TextAlignCenter, Indent: 0, SpaceBefore: 0, SpaceAfter: 6,
}})
rt.AppendRun(widget.RichRun{Text: "tail"})                 // last paragraph
rt.AppendParagraph(widget.RichParagraph{Runs: runs})       // incremental, log-friendly
rt.SetText("plain\ntext"); rt.Text(); rt.Paragraphs(); rt.Clear()
rt.FontSize, rt.FontName, rt.LineSpacing, rt.LinkUnderline // defaults for runs without own
rt.TextColor, rt.LinkColor, rt.SelColor, rt.Background     // themed by ApplyTheme
rt.OnLinkClick = func(url string) {}                       // the widget opens nothing
rt.SelectAll(); rt.Select(from, to); rt.SelectedText(); rt.Copy() // plain + HTML to clipboard
rt.ScrollY(); rt.SetScrollY(y); rt.ScrollToEnd(); rt.ContentHeight()
```

`RichRun{Text, Font, Size, Color, BG, Underline, Strike, Link}` — zero field =
"as the widget". `Font` is a registered font NAME (bold/italic are separate
fonts: `BuiltinFontBold`, ...). A line is as tall as its tallest run
(ascent+descent from `MeasureUIFontMetrics`), all runs share one baseline.
Word wrap across runs, hard break of over-long words, `\n` in a run = line
break. Offsets (selection, `AccessTextProvider`) are rune indexes into the text
with paragraphs joined by `\n`. A vertical scrollbar appears when the content
is taller than the widget; wheel/PgUp/PgDn/Home/End scroll. Mouse: drag, double
click = word, triple click = paragraph; Ctrl+C/Ctrl+A. Hand cursor over links
(`CursorProvider`). Role `RoleDocument`. Layout is pure
(`widget/richtext_layout.go`, testable with a fake measurer). Not done:
bullets/tables/images, per-run baseline shift, virtualization of very
long documents (layout is recomputed whole when width or content changes).

**Editor mode** (`Editable`, default `false` = display only, exactly as before):

```go
ed := widget.NewRichText(); ed.Editable = true   // caret drawn automatically (ShowCaret not needed)
ed.AcceptTab = true                              // TabAcceptor: Tab inserts 4 spaces
ed.OnChange = func() {}                          // after any user/panel edit; no widget locks held;
                                                 // NOT called by SetParagraphs/SetText/SetHTML/Append*
ed.ToggleBold(); ed.ToggleItalic(); ed.ToggleUnderline(); ed.ToggleStrike()
//  selection: all-on -> off, else on for all; no selection: changes the "typing style"
//  (next typed text), reset by caret move/click/Select/SetCaretPosition. Bold on italic = BuiltinFontBoldItalic.
ed.SetSelectionStyle(func(r *widget.RichRun) { r.Color = c })   // selection, or typing style if none
ed.SetParagraphFormat(func(p *widget.RichParagraph) { p.Align = widget.TextAlignRight })
st := ed.SelectionStyle()            // RichRun at caret/selection start; st.IsBold(), st.IsItalic()
pf := ed.SelectionParagraphFormat()  // RichParagraph (Runs nil) of the caret's paragraph
ed.Undo(); ed.Redo(); ed.CanUndo(); ed.CanRedo()
ed.Cut(); ed.Paste(); ed.Copy()      // Paste: clipboard HTML first, else plain text in the caret style
ed.HTML() string; ed.SetHTML(src)    // whole document <-> HTML (RichParagraphsFromHTML)
```

Keys (editor only): printable runes, Enter (paragraph), Shift+Enter (soft break),
Backspace/Delete, Ctrl+Backspace/Delete (word), Ctrl+Z / Ctrl+Y / Ctrl+Shift+Z,
Ctrl+X/Shift+Delete, Ctrl+V/Shift+Insert, Ctrl+B/I/U, Tab (with `AcceptTab`).
A typed word, and a replaced selection plus the typing after it, are one undo
entry; caret move, click and focus loss end the typing run. `IMEComposer` is
implemented (composition underlined in the document, one undo entry per commit;
click/key/focus loss accept the composition as text). Right click opens
Cut/Copy/Paste/Select All (`ContextMenuProvider`; the app's own `ContextMenu`
wins). In the editor a link opens only on Ctrl+click. `AccessReadOnly() ==
!Editable`, `AccessSetText` edits (undoable). XAML: `IsReadOnly="False"` or
`Editable="True"`, `AcceptsTab="True"` on `<RichText>`. `SetParagraphs` merges
adjacent runs with equal formatting and drops empty runs (document invariants);
`AppendParagraph` keeps undo history, `AppendRun` resets it. Not done: tab
stops, lists/tables/images, find/replace, bold for fonts not built in.

**The edit model — `RichDocument`** (no widget, no locks; the editor holds one):

```go
d := widget.NewRichDocument(paras)          // or NewRichDocumentFromText(s)
end := d.Insert(at, "text", style)          // '\n' splits the paragraph; returns position after
end = d.Type(at, "a", style)                // typing: consecutive Type calls merge into one undo
d.InsertInline(at, "line\nbreak", style)    // '\n' stays a soft line break
d.InsertParagraphs(at, widget.RichParagraphsFromHTML(src)) // paste
d.Delete(from, to)                          // across runs and paragraphs; '\n' merges paragraphs
d.Replace(from, to, "new", style)           // one undo entry; TypeReplace also merges further typing
d.ApplyStyle(from, to, func(r *widget.RichRun) { r.Underline = true })
d.SetParagraphFormat(from, to, func(p *widget.RichParagraph) { p.Align = widget.TextAlignCenter })
st := d.StyleAt(pos)                        // style new text gets here: the rune to the left
sel, ok := d.Undo()                         // RichDocSel{Anchor, Caret}: where to put caret/selection
d.BreakUndoGroup()                          // call on caret move, click, focus loss
d.BeginGroup(); /* several edits */; d.EndGroup() // one undo entry
d.Revision()                                // grows on every change, incl. Undo/Redo
```

Positions are runes of the document, paragraphs separated by one `\n` (same as
layout and selection). After every edit adjacent runs with equal formatting are
merged and empty runs dropped — except the single run of an empty paragraph,
which remembers the formatting so typing there does not start «by default».
An undo record keeps the range of affected paragraphs and what stood there, so
undo and redo are one swap operation and restore bit-for-bit; there are no
whole-document snapshots, text strings are shared. Depth 200 by default
(`SetUndoDepth`).

`RichParagraphsFromHTML(src)` parses clipboard HTML from Word and browsers with
no dependencies: paragraphs, headings, bold, italic, underline, strike, links,
inline `color`/`background-color`/`font-size`/`font-weight`/`font-style`/
`text-decoration`/`text-align`, lists (bullet + indent), `code`/`pre` as
`BuiltinFontMono`. Scripts, styles, comments and Word markers are dropped, as
are `javascript:`/`vbscript:`/`data:` links. `font-family` is not carried over
(the engine does not know foreign fonts). `RichParagraphsFromText` splits on
`\n`, `\r\n`, `\r`.

**Caret** (both modes): `CaretPosition()`, `SetCaretPosition(pos)` (clears the
selection, scrolls to the caret), `IMECaretRect()` (canvas coordinates, 1 px
wide — where the IME candidate window goes). The caret is the active end of
the selection; ←/→, Ctrl+←/→ (words), ↑/↓ with a remembered «desired X»,
Home/End (visual line), Ctrl+Home/End, PgUp/PgDn, all with Shift to extend;
Shift+click extends from the anchor. In display mode the caret is not drawn
unless `ShowCaret`, and plain ↑/↓/PgUp/PgDn/Home/End scroll as before.

### Window, theme and containers

```go
win.SetTitle("config.go *")                // native window: taskbar + drawn title bar
win.Title()
win.SetOnCloseRequest(func() bool { ... }) // × and Alt+F4/taskbar; runs on the engine goroutine;
                                           // false keeps the window; Close() skips the hook
ww.SetFrameColor(c)                        // widget.Window outer frame, pinned over the theme; A=0 → theme
t.WindowFrame                              // *widget.Theme default for the frame (A=0 in presets = old behavior)
dd.ArrowStyle = widget.ArrowChevron        // ArrowAuto (theme decides) | ArrowTriangle | ArrowChevron; XAML ArrowStyle
panel.SetBackgroundRole(widget.BackgroundPanel) // StackPanel/DockPanel follow Theme.PanelBG / WindowBG;
                                                // XAML Background="{Theme PanelBG}"
```

---

## Window caption: gradient, metrics, window icon, system menu, title-bar presses (next minor)

Answers the WinLine remarks on the classic Windows 2000 window
(`pkg/apps/terminal/classic.go`). Public API is additive; Windows 10, Windows 11
and macOS windows render byte-identical to before (checked on rendered frames
active and inactive); Windows 2000 changed only where the remarks ask.

**Title gradient.** The Windows 2000 profile declares the second point of the
caption gradient: flat tokens `window.titlebar.gradient2` (#A6CAF0) and
`window.titlebar.gradient2.inactive` (#C0C0C0); the first points are the fills of
`window/titlebar` (navy / grey #808080). `widget.Window` already drew a gradient
whenever `Theme.TitleBG2` was set — only the profile never filled it.
`theme.KeyWindowTitleGradient2` / `KeyWindowTitleGradient2Inactive` name the keys.
After `Manager.SetAccent` the active gradient ends at `accent.light`
(`Profile.SetColorFrom`, below); `ResetAccent` brings #A6CAF0 back. The inactive
grey never follows the accent. `Windows2000 Blue` inherits both.

```go
// Profile.SetColorFrom(k, from): token k follows token `from` when the application
// overrides the accent. Without SetAccent the value declared with SetColor stays
// (bit-identical look); a token the profile did not declare is taken from `from`
// even without SetAccent. A child profile's reference overrides its parent's;
// a missing source cancels the reference. Counts in Profile.TokenCount.
p.SetColor(theme.KeyWindowTitleGradient2, theme.RGB(166, 202, 240)).
    SetColorFrom(theme.KeyWindowTitleGradient2, theme.KeyAccentLight)
```

**Caption metrics.** Theme metrics, read by `widget.Window` through
`ThemeStyle` (`Materialize` / `ProfileFromTheme` carry them both ways; 0 = not
declared = old look):

| metric (`theme.Key…`) | `ThemeStyle` | Windows 2000 | without |
|---|---|---|---|
| `window.titlebar.height` (`KeyWindowTitleBarHeight`) | `TitleBarHeight` | 18 | 32 (24 classic) |
| `window.caption.button.w` / `.h` | `CaptionButtonW/H` | 16 × 14 | square, `height-6` (18×18) |
| `window.caption.icon.size` | `CaptionIconSize` | 16 | 16 |

Rules: an explicit `Window.TitleBarHeight` beats the metric; title tabs
(`EnableTitleTabs`) ignore it (tabs need the room); the buttons are centered
vertically in the bar (18 − 14 → 2 px, the old 24 − 18 → 3 px). Only
`widget.Window` reads them — `Dialog` keeps its own title height. A Windows 2000
window now has an 18 px bar, 16×14 buttons, frame 5, content starts at
`frame + 18`. The locale badge (`ShowLocaleIndicator`) is one pixel taller on bars
under 20 px (`bar-3`, was `bar-4`) so its text no longer touches the bottom border.

**Window icon and system menu** (`widget/window_icon.go`).

```go
win.SetIcon(img)                 // image.Image, scaled to the metric size; nil removes
win.SetIconSVG(svgBytes) error   // rasterized at the physical size; currentColor = caption text
win.HasIcon(); win.IconBounds()  // absolute rect, empty when hidden (mac layout, no bar)
// The caption moves behind the icon by itself (titleTextLeft, also for
// TitleBarContentBounds and title tabs); the nav button ("≡") goes after the icon.
// Icon: 2 px from the bar edge on classic, 8 on the others; vertically centred.
// Windows layout only: in the mac layout the buttons sit left and there is no icon.

// A press on the icon opens the system menu (on the PRESS, like Windows); a second
// press on the open menu closes it (PopupMenu.Toggle). A DOUBLE click calls OnClose.
// The press does not drag the window.
items := win.SystemMenuItems() // []MenuItem: Restore, Move, Size, Minimize, Maximize, —, Close (Alt+F4)
win.SetSystemMenu(items)       // your own list (build it from the default one); nil = default;
                               // an empty non-nil slice removes the menu (icon = a picture, drags the window)
win.SystemMenu() *PopupMenu; win.OpenSystemMenu() bool
win.SetMaximized(true); win.IsMaximized() // host state; window.Window reports it on every resize
```

Labels go through `widget.Tr` (`win.sys.restore|move|size|minimize|maximize|close`,
EN and RU built in, live language switching on the next open). Actions use what
the window already has: Restore and Maximize call `OnMaximize` (the host toggles);
Minimize `OnMinimize`; Close `OnClose`; Move `OnNativeMove`; Size
`OnNativeResize(NativeEdgeBottom|NativeEdgeRight)`. An item without its handler
(or a button the window does not have: `Resize`, `Style`) is shown DISABLED, not
removed, so the menu does not jump from window to window; Move/Size exist only in
a host that can move a window by the OS (Wayland) — elsewhere they stay grey and
the window is dragged by its caption. The default list is rebuilt on every open;
a list you set is yours (its `Disabled` flags are not recomputed).

**Who owns a press on the title bar** (the unobvious part, `widget/titlebar_hit.go`).
`Window.WantsCapture` answers true to every left press in the bar — the window
drags itself by it — and the engine looks for a capturer from the DEEPEST child up
to the root. A child lying over the bar therefore gets the press only if its own
`WantsCapture` returns true; otherwise the press goes to the window and starts a
drag, and the child never sees it. Built-in exceptions the window knows itself:
the collapse button, `SetTitleBarContent`, the window icon, the caption buttons,
title tabs. Two explicit ways for the rest, neither needs a mouse capture and
neither changes an existing application:

```go
// 1. The child says "presses on me are mine" (any descendant of the window, any depth).
func (b *MyBtn) OwnsTitleBarPress(pt image.Point) bool { return pt.In(b.Bounds()) }  // widget.TitleBarPressOwner
// 2. The application decides by point (areas drawn by the app, widgets you cannot change).
win.SetTitleBarHitTest(func(pt image.Point) bool { return pt.In(searchArea) }); win.SetTitleBarHitTest(nil)
```

The press then takes the ordinary path (deepest widget first, bubbling up) and the
window does not start a drag; the question is asked only for points inside the bar.
Keep `WantsCapture` when the widget needs the mouse AFTER the press (slider, text
selection, release semantics of a button). `AddDragArea` still beats everything.

**`PopupMenu.DismissedByPress()` and `Toggle`.** The engine dismisses open menus
outside the click path BEFORE the press reaches widgets, so a button that opens a
menu finds it already closed and would reopen it — the menu could not be closed by
pressing its button again. `DismissedByPress()` is true when the press being handled
is the one that dismissed the menu. The "time window" is ONE PRESS, not a duration
(unlike `desktop.Flyout.DismissedByAnchor`): true from the dismissal until the next
press begins (`widget.CurrentPressSeq`, bumped by the engine), so it does not depend
on the user's speed, and a menu closed by Esc does not make the button lose the next
press. Without an engine (unit tests) there are no presses and it is always false.
`Toggle(x, y)` is the usual button body: open → close; just dismissed by this press →
do nothing; else `Show(x, y)`; returns whether the menu is open afterwards. The tab
strip chevron and the system menu use it.

Also fixed on the way: a menu built from a built-in Windows profile
(`Materialize(Windows2000/Windows10)`) drew its DISABLED items with a transparent
text (the flat token `disabled.default` is not declared there) — `Materialize` now
takes the colour from `menu.item` in the Disabled state (only when the flat token
and the menu style do not carry one).

Tests: `theme/window_title_test.go` (gradient, accent following, `SetColorFrom`,
metrics, other profiles untouched), `tests/window_caption_test.go` (gradient pixels
active / inactive / accent, 18 px bar and 16×14 buttons, hit-test of the small
button, old geometry without metrics, bridge round trip, icon placement and pixels,
caption shift, SVG, system menu items / languages / callbacks / click / toggle /
double click / custom list, press owner, `SetTitleBarHitTest`,
`DismissedByPress`+`Toggle`).

---

## v3.34 additions (images under a round clip, image cache, DrawImageRect, X11 key repeat, SetMaxFPS)

Answers the PixGo (image viewer) remarks E8, E9, E10, E13. Public API is
additive; frames of existing scenes are byte-identical (golden and hash tests pass
without updates).

### Images under `SetRoundClip`

`DrawImage`, `DrawImageScaled` (stretched, 1:1 fast path, HiDPI) and the new
`DrawImageRect` now obey the rounded clip like fills do. Before, `blitOver` honoured
only the rectangular clip, and a picture inside a rounded panel had square corners.
Only the corner rows (`radius` rows at the top and at the bottom) are blitted line by
line with the narrowed span; the straight middle is ONE blit, as without rounding.
Without `SetRoundClip` the path is the old one, byte for byte. Benchmarks
(`go test ./engine -bench Image`) did not move: 64→24 icon 1.81 µs/op before and after;
a 300×200 picture under a round clip costs the same as without it.

### Scaled-image cache: source is retained, `InvalidateImageCache` everywhere

```go
eng.InvalidateImageCache(src image.Image)    // nil — the whole cache; any goroutine
widget.InvalidateImageCache(ctx, src)        // helper for the draw context (no-op if unsupported)
// optional interface (engine.Canvas, translatingContext, ScrollView wrapper implement it):
type widget.ImageCacheInvalidator interface{ InvalidateImageCache(src image.Image) }
```

- **A cache entry now holds `src`.** The key is the address of the source plus bounds
  and target size; the GC was free to collect the image and hand its address to a NEW
  image of the same size, which then received somebody else's frame. While the entry
  lives the address is taken, and a hit is additionally verified by `ent.src == src`.
  `weak.Pointer` was not used: `go.mod` says Go 1.22, `weak` appeared in 1.24.
  Cost: the 32 cached sources stay alive until evicted or invalidated (the byte budget
  still counts only scaled results).
- **`Engine.InvalidateImageCache`.** The cache lives on a canvas, and an engine has
  several (main plus one per popup in its own native window). The call goes through
  a journal shared by the engine's canvases and is applied by each of them before its
  next use of the cache, so it is safe from any goroutine and reaches popups. A canvas
  that lags more than 64 entries behind drops its whole cache (always correct). The call
  does not schedule a frame: if the picture is on screen, call `Invalidate`/`InvalidateRect`.
- Wrappers forward it: `svOffsetCtx` (ScrollView) and `translatingContext` (popups). The
  narrow adapters of `widget/datagrid` and `widget/treeview` do not (their own context
  types have no image-cache notion).

### `DrawImageRect` with a filter — only the visible part is scaled

```go
widget.DrawImageRect(ctx, src, sr, dr, widget.FilterCatmullRom)  // helper with a fallback
// optional interface, implemented by engine.Canvas and the wrappers:
type widget.ImageRectDrawer interface {
    DrawImageRect(src image.Image, sr, dr image.Rectangle, f widget.ImageFilter)
}
// filters: widget.FilterNearest, FilterBilinear, FilterCatmullRom
sr, dr = widget.ClampImageRect(src.Bounds(), sr, dr)   // clamp sr to the source, shrink dr proportionally
```

`sr` is the source rectangle (pixels of `src`), `dr` the destination in LOGICAL
coordinates of the context (like `x, y, w, h` of `DrawImageScaled`; may be far larger
than the window and run past its edges). Unlike `DrawImageScaled`, which stretches the
WHOLE source to the FULL size before the clip, only the destination pixels under the
current clip (rectangular and rounded) are computed, and only the source rows and
columns they need. A 12000×8000 picture at 400 % in a 1200×800 window costs like
1200×800 pixels. The result is not cached; the cost is proportional to the visible area
(`TestDrawImageRect_WorkProportionalToClip`).

- Kernels follow `golang.org/x/image/draw` (centre projection `(d+0.5)·s−0.5`, kernel
  widened by the scale when shrinking, weights normalised, edges truncated) but are an
  own separable implementation: x/image builds an intermediate `dw×sh` buffer for the
  whole destination width, which is exactly what must be avoided. Memory is a sliding
  window of a few rows, independent of the source and `dr` sizes.
- Sources read directly: `*image.RGBA`, `*image.NRGBA`, `*image.YCbCr` (all subsamplings),
  `*image.Gray`; any other through `RGBA64Image`/`At`. Output is premultiplied; colour
  never exceeds alpha (Catmull-Rom ringing is clamped).
- Matches full scaling by `x/image/draw` in the visible part within ±2 (±3 for YCbCr and
  Gray: other YCbCr→RGB rounding), exactly for `FilterNearest` on RGBA.
- `sr` outside `src.Bounds()` is clamped together with a proportional part of `dr`.
- Fallback for a context without `ImageRectDrawer` (`widget.DrawImageRect` helper): the
  cropped source goes through `DrawImageScaled` — bilinear regardless of the filter and
  without the clip saving. A part of the source is a new object on each call, so the
  32-entry cache turns over.
- Reference timings (i7, 1200×800 window, 4000×3000 source): zoom ×4 — nearest 6 ms,
  bilinear 7 ms, Catmull-Rom 9 ms; fit-to-window (shrink 3.3×) bilinear 48 ms.

### X11: `KeyEvent.Repeat` and the end of spurious key-ups

By default an X server shows auto-repeat as a `KeyRelease` + `KeyPress` pair with the
SAME time and key code. The application saw a stream of "released, pressed", `Repeat` was
always false, and toggling keys fired on every tick. The backend now applies the standard
trick of toolkits: on `KeyRelease` it peeks (2 ms, once per ordinary release) whether a
`KeyPress` with the same time and code follows; if so, the release is swallowed and the
press is delivered with `Repeat=true`. A press of a key already held is a repeat in any
case, so a server with XKB `DetectableAutoRepeat` works unchanged. XKB itself is not used
(the backend speaks the bare protocol). The table of held keys is kept even for a window
blocked by a modal, and reset on `FocusOut`. A half-read event is always completed.
`X11Window` implements `keyRepeatSource` (`SetOnKeyDownRepeat`). Tests drive
`readEvent`/`handleX11Event` over `net.Pipe`: pairs, real release+press, single release,
split event, detectable repeat, disabled window (`window/x11_keyrepeat_linux_test.go`).
Checked on the wire only, not against a live server.

### `Window.SetMaxFPS` now limits the main engine

It used to change only a field read by modal windows and dock hosts when they create
their engines; the main engine ran at the `fps` of `engine.New` whatever the window said.
Now `Window.SetMaxFPS(n)` also calls the optional `Engine.SetMaxFPS(n)` (atomic, any
goroutine, picked up on the next tick, the animation step follows). It is a CAP, not a
target: an engine created at 20 fps stays at 20 under `SetMaxFPS(60)`. Until the window
method is called the engine keeps its own rate, even when it is above the window default
of 60. `n <= 0` removes the cap.

### Documentation corrections

- Render on demand has been the default since v3.5 (`engine.New` sets it); the
  "redraws every tick by default" text is gone, `SetRenderOnDemand(false)` is the opt-out.
- `BeginMove`/`BeginResize` are implemented on X11 (`_NET_WM_MOVERESIZE`, v3.26) and Wayland;
  they answer false only on Win32 and macOS. The native-move hooks of `widget.Window` are
  installed on X11 too.
- Tray on X11 and Wayland works through StatusNotifierItem (`window/tray_sni_linux.go`,
  dbusmenu for the menu); balloons through `org.freedesktop.Notifications`
  (`notify_linux.go`). The taskbar preview stays Windows-only.
- The statement that `KeyEvent.Repeat` marks a repeat "on every platform" was false for X11
  (the field was always false there); it is true since this version.

Tests: `engine/imageround_test.go` (round clip: every drawing path, scales 1 and 2,
RGBA and BGRX, opaque and translucent, partial overlap, unchanged frame without rounding),
`engine/imagecache_engine_test.go` (finalizer-based retention test, forged key, engine
invalidation of main and popup canvases, journal overflow, concurrency, popup wrapper),
`engine/imagerect_test.go` (full-scale reference for 3 filters × 6 source types × 6
geometries, ×1000 destination, work proportional to the clip, `sr` clamp, round clip and
BGRX, agreement with `DrawImageScaled` at scales 1, 1.5, 2), `widget/imagerect_test.go`
(fallback, `ClampImageRect`, ScrollView wrapper), `engine/maxfps_test.go`,
`window/maxfps_test.go`, `window/x11_keyrepeat_linux_test.go`.

---

## v3.34 additions (cursor override and new shapes, pointer leave on Win32, side mouse buttons)

Answers the PixGo remarks E2, E4, E5 (image viewer: slide show that hides the
cursor, panning with a "grab" hand, back/forward mouse buttons). Public API is
additive; existing numeric values (`widget.Cursor` 0–6, `widget.MouseButton` 0–4)
did not move.

### Hiding the cursor and new shapes (E2)

```go
// Appended to the END of widget.Cursor (values 7..12).
widget.CursorNone      // hidden
widget.CursorGrab      // open hand (panning)
widget.CursorGrabbing  // closed hand (dragging right now)
widget.CursorSizeAll   // four arrows
widget.CursorCrosshair // crosshair (crop, picker)
widget.CursorWait      // busy

// A shape over EVERY widget, applied at once, without waiting for a mouse move.
eng.SetCursorOverride(widget.CursorNone)   // any goroutine; same shape again = no-op
eng.ClearCursorOverride()                  // back to the cursor of the widget under the mouse
c, ok := eng.CursorOverride()
eng.SetOnCursorOverrideChanged(fn)         // window.Run installs it; applications do not
```

`Engine.CursorAt` returns the override first (stronger than any widget and any
modal), so the window needs no new interface: `window.Run` subscribes to the
change (`cursorOverrideNotifier`, optional like `modifierSink`) and on the engine
goroutine recomputes the cursor for the LAST known mouse position and hands it to
the OS window (`surface.refreshCursor`). Before the mouse was ever in the window
the point is (-1,-1), so after `ClearCursorOverride` the arrow comes back.
Popup windows (menus moved into their own OS window) are covered since the second
round of v3.34: see "Full screen on a chosen monitor, cursor over popups, drag
icon, XdndProxy" below.
A custom `EngineAPI` without `SetOnCursorOverrideChanged` just never gets the
immediate refresh (the override still wins on the next move).

How each backend shows the shapes:

| shape | Win32 | X11 | Wayland |
|---|---|---|---|
| None | `SetCursor(NULL)` in `WM_SETCURSOR` (`ShowCursor` untouched) | transparent cursor from two 1×1 depth-1 pixmaps (`CreatePixmap`, `CreateGC`, `PolyFillRectangle`, `CreateCursor`) | `wl_pointer.set_cursor(serial, NULL, 0, 0)` — also with `cursor-shape-v1` (it has no hidden shape) |
| Grab / Grabbing | own 24×24 `HCURSOR` (`CreateIconIndirect`, cached per window, freed on `WM_DESTROY`); falls back to `IDC_HAND`/`IDC_SIZEALL` | own image via two pixmaps + `CreateCursor` (core protocol, no RENDER) | `grab` / `grabbing` shapes; without `cursor-shape-v1` the same 24×24 image |
| SizeAll | `IDC_SIZEALL` | glyph `fleur` (52) | `all-scroll` (33) |
| Crosshair | `IDC_CROSS` | glyph `crosshair` (34) | `crosshair` (8) |
| Wait | `IDC_WAIT` | glyph `watch` (150) | `wait` (6) |

XAML: `Cursor="None|Grab|Grabbing|SizeAll|Cross|Crosshair|Wait"` joins the old
`Arrow|IBeam|Hand|SizeWE|SizeNS`.

Own images are 24×24 (`wlCursorSize`, the RDP limit for WinLine is 32×32); the
art lives in `window/waylandcursor.go` (`wlCursorShapes`, `wlCursorImage`) and is
shared by all three backends. macOS has no cursor support at all (`CocoaWindow`
has no `SetCursor`): the override is simply ignored there.

Win32 detail: `SetCursor` applies the shape at once only when the window is under
the pointer (`WindowFromPoint`), because the cursor is global.

### Pointer leave on Win32 (E4)

`WM_MOUSEMOVE` arms `TrackMouseEvent(TME_LEAVE)` once per entry
(`leaveTracker`, one-shot like the system request; a refusal is retried on the
next move); `WM_MOUSELEAVE` goes the same way as `LeaveNotify` on X11 and
`wl_pointer.leave`: the application gets `OnMouseMove(-1, -1)` (`pointerOutside`),
so hover and tooltips are dropped. Not a leave (`mouseLeaveReal`): the pointer
moved from the client area onto a caption button the application drew itself
(`SetHitTest` → `HTMIN/MAX/CLOSE`): the system calls it non-client, sends
`WM_MOUSELEAVE` and drives the highlight with `WM_NCMOUSEMOVE`; the order is not
guaranteed, so the handler looks at the cursor position and the hit area and stays
quiet for the buttons. The caption itself and the resize edges are a real leave.
`WM_NCMOUSEMOVE` over our buttons arms `TME_LEAVE|TME_NONCLIENT` the same way, so
`WM_NCMOUSELEAVE` no longer depends on `DefWindowProc` (which we skip for them).
While a button is held (mouse capture) Windows sends no leave; it comes after the
release if the pointer is outside.

### Side mouse buttons X1 / X2 (E5)

```go
widget.MouseButtonX1 // "back"    (Win32 XBUTTON1, X11 button 8, Linux BTN_SIDE)
widget.MouseButtonX2 // "forward" (Win32 XBUTTON2, X11 button 9, Linux BTN_EXTRA)
eng.SendMouseButton(x, y, widget.MouseButtonX1, true)
```

Backends: Win32 `WM_XBUTTONDOWN/UP/DBLCLK` (`GET_XBUTTON_WPARAM`; returns TRUE,
otherwise `DefWindowProc` turns the release into `WM_APPCOMMAND`); X11 buttons 8
and 9; Wayland `BTN_SIDE`/`BTN_BACK` → X1 and `BTN_EXTRA`/`BTN_FORWARD` → X2
(multimedia mice and drivers name the buttons by purpose); Cocoa `buttonNumber`
3/4 (before, any "other" button was taken for the middle one). Codes in
`NativeWindow.SetOnMouseButton`: 5 = X1, 6 = X2. No mouse capture. A held X1/X2 is
released by the Wayland backend on pointer leave like the others.

Handling rules for widgets: the event bubbles from the deepest widget under the
pointer to the root like any button. All standard widgets react to `MouseLeft`
only (checked: `OnMouseButton` of every widget, `switch` on buttons have no
default that falls to the left button), so an application hangs back/forward on
its root or on a container:

```go
func (r *Root) OnMouseButton(e widget.MouseEvent) bool {
    if e.Pressed && e.Button == widget.MouseButtonX1 { r.goBack(); return true }
    if e.Pressed && e.Button == widget.MouseButtonX2 { r.goForward(); return true }
    return false
}
```

Tests: `tests/cursor_override_test.go` (values, override without a move, change
notifications, concurrency), `tests/xbuttons_test.go` (values, X1/X2 reach the
root, Button/CheckBox/ToggleSwitch/RadioButton/TextInput/Dropdown ignore them),
`window/cursor_override_test.go` (the window applies the override without a move,
real engine), `window/mousebuttons_test.go` (X11 / evdev / `WM_XBUTTON` / Cocoa
mapping), `window/pointerleave_tracker_test.go` (arm once, false-leave rule),
`window/x11cursorbitmap_test.go` (pixmap-cursor request stream, art limits),
`window/mouse_windows_test.go` (`WM_XBUTTON*`, `WM_MOUSELEAVE` through `wndProc`,
caption-button case, hidden cursor, own cursors), `window/x11_cursor_shapes_linux_test.go`
and `window/wayland_cursor_linux_test.go` (on the wire: hidden cursor, shapes,
side buttons).

---

## v3.34: full screen and monitor geometry (`window` package)

Answers the PixGo task E1 (full screen) and E11 (monitors = E1 item 5). The API is
additive; nothing public was removed, no signature changed. One behaviour change:
**on Wayland `IsMaximized` no longer reports a full-screen window as maximized**
(see "Maximized vs full screen").

```go
win.SetFullscreen(true) bool             // false: the backend (or the window system) cannot; true: the request was SENT
win.IsFullscreen() bool                  // what the system reports; before Run and without backend support: false
win.SetOnFullscreenChanged(func(on bool)) // engine goroutine; may be set before Run

window.Monitors() []window.Monitor       // primary first, then left→right, top→bottom; may be empty (macOS, no session)
window.MonitorOf(win) (window.Monitor, bool)  // monitor with the biggest share of the window
window.MonitorAt(x, y int) (window.Monitor, bool)
window.MonitorForRect(list []Monitor, r image.Rectangle) (Monitor, bool) // ok=false: r touches no monitor

type Monitor struct {
    Name         string          // may be empty
    Bounds, Work image.Rectangle // whole monitor / without panels
    Scale        float64         // 1.0 = 96 dpi
    Primary      bool
}
```

**`SetFullscreen` contract.** The answer is "was the request accepted for execution",
not "it is done": Win32 changes the window on the window thread a moment later, X11
and Wayland answer asynchronously and the compositor may refuse. The result is always
visible through `IsFullscreen` and `SetOnFullscreenChanged`. The callback fires for
**every** change, including those the system makes itself (the user pressed Esc/F11
at the compositor, the window manager returned the window). Before `Run` the request
is remembered and applied when the window is created (the answer is then only
"the platform can do it at all": `false` on macOS). The default `□` caption button of
a full-screen window leaves full screen (it used to "restore" because Wayland counted
full screen as maximized — that behaviour is kept explicitly in `window.Window`).

**Maximized vs full screen.** `IsMaximized` of a full-screen window returns the value
it had before full screen (a maximized window stays "maximized" underneath, a normal
one stays "normal"); on every backend. Consumers checked: the `□` button and
`widget.Window.SetMaximized` (system menu "Restore") in `window/window.go`, the
dialog hosts (`window/modal_host.go`, they never go full screen). A full-screen window
is not rounded on any backend (`SetCornerRadius`: Win32 `applyCorners`, Wayland
`maskRadius`; X11 never rounds).

**Backends.**

| | request | state |
|---|---|---|
| Win32 | window thread: remember rect + style + "was maximized" (a maximized window is first restored, otherwise the system keeps it `zoomed` and the taskbar stays), drop `WS_THICKFRAME`, `SetWindowPos` to `rcMonitor` of `MonitorFromWindow` (not `rcWork`: the taskbar is covered) | `IsFullscreen` is our own flag. Back: style and rect restored (a rect that fell off all monitors is moved to the nearest one), `SW_MAXIMIZE` again if it was maximized. `WM_DISPLAYCHANGE`, `WM_WINDOWPOSCHANGED` (Win+Shift+arrow), `WM_DPICHANGED` re-fit the window to its monitor; `WM_GETMINMAXINFO` raises the size ceiling to that monitor; no resize edges/caption drag in full screen. `SetSize`/`SetPosition`/`Maximize`/`Restore` in full screen are remembered and applied on exit |
| X11 | `ClientMessage _NET_WM_STATE` (`ADD`/`REMOVE` `_NET_WM_STATE_FULLSCREEN`) to the root window. `false` if the window manager lists its supported states in `_NET_SUPPORTED` and full screen is not among them | read from `_NET_WM_STATE` after `PropertyNotify` (the manager may refuse or leave itself); the maximized atoms refresh `IsMaximized` too, except while full screen |
| Wayland | `xdg_toplevel.set_fullscreen(output=nil)` (opcode 11) / `unset_fullscreen` (12); a fixed-size window gets its size limits lifted first and fixed again at the old size afterwards | the `fullscreen` state of `configure`. A `configure` WITHOUT it after a request in flight is a refusal (this is how WinLine answers without `WINLINE_FULLSCREEN`): the callback gets `false`, `IsFullscreen()==false`, `SetFullscreen` had returned `true`. A stale `configure` that was already on its way can look like a refusal: then `false` is followed by `true` when the real answer arrives |
| Cocoa | stub: `SetFullscreen` returns `false` | - |

**Monitors.** `Bounds`/`Work` are in the coordinates of `NativeWindow.GetPosition`.

- Win32: `EnumDisplayMonitors` + `GetMonitorInfoW` (`rcMonitor`/`rcWork`, device name
  `\\.\DISPLAY1`) + `GetDpiForMonitor` (Windows 8.1+, else 1.0); physical pixels, `Scale = dpi/96`.
- X11: **RandR 1.5 `RRGetMonitors`** over a short separate connection (the window's
  connection is read by its event loop). `Work` is `_NET_WORKAREA` clipped by the
  monitor (EWMH has no per-monitor work area: exact for one panel, an approximation
  for several). `Scale` is `Xft.dpi` (same for all monitors — X11 has no per-monitor
  scale). **Without RandR or older than 1.5 the root window is reported as ONE
  monitor.**
- Wayland: `wl_output` (geometry, mode, scale, name) and, when the compositor
  advertises `zxdg_output_manager_v1`, `zxdg_output_v1` (logical position/size, name).
  `Bounds` are LOGICAL (compositor coordinates, the units of `configure`); `Scale` is
  the integer `wl_output` scale, i.e. buffer pixels per `Bounds` unit (fractional
  scale is per surface, not per monitor — see `SetOnDpiChanged`). `Work == Bounds`
  (the protocol has no work area). There is no "primary" in `wl_output`: the first
  output is reported as primary. `MonitorOf` is the output of the last
  `wl_surface.enter`.
- macOS: `Monitors()` is empty, `MonitorOf` returns `false`.
- `Monitors()` without a window costs one short connection on Linux; do not call it
  per frame. "Does the saved window rect still land on a monitor?":
  `_, ok := window.MonitorForRect(window.Monitors(), saved)`.

**Not done / limits.** `SetFullscreen` itself takes no monitor (the one the window
is on is used) — the monitor is chosen with `SetFullscreenOn` (below).
Win32 `Name` is the device name, not the marketing name from the monitor EDID.
macOS has neither full screen nor monitors yet. Wayland: `Work == Bounds` (the
protocol has no work area; layer-shell panels are invisible to a client) and
`Primary` is simply the first output — documented on `Monitor`, nothing to fix.

Tests: `window/fullscreen_rect_test.go` (Win32 save/restore arithmetic: style, rect,
vanished monitor, re-fit), `window/fullscreen_test.go` (the `Window` layer: pre-Run
request, delegation, `□` button, monitor helpers), `window/fullscreen_windows_test.go`
(a real Win32 window: enter/leave, maximized round trip, re-fit, monitors),
`window/wayland_fullscreen_linux_test.go` (wire: opcodes 11/12, accepted, refused
like WinLine, left by the compositor, maximized value kept, fixed size restored),
`window/wayland_monitor_linux_test.go` (`wl_output`/`xdg_output` events and a full
query against a fake compositor on a real unix socket), `window/x11_fullscreen_linux_test.go`
(`ClientMessage` bytes, state from `_NET_WM_STATE`, `_NET_SUPPORTED`, RandR 1.5
over a fake server, fallback), `*_e2e_linux_test.go` (live X server / compositor via
`DISPLAY` / `WAYLAND_DISPLAY`; skipped without them).

---

## v3.34 additions: image on the clipboard, Wayland window icon

### Image on the clipboard (E3)

```go
// Optional extension of ClipboardProvider (like ClipboardHTMLProvider / ClipboardFilesProvider):
type ClipboardImageProvider interface {
    SetImage(img image.Image) error
    GetImage() (img image.Image, ok bool)
}
err := widget.ClipboardSetImage(img)   // replaces the clipboard; the image is copied
img, ok := widget.ClipboardImage()     // ok == false: not an image, or the provider cannot do images

widget.ErrClipboardImageUnsupported    // provider without ClipboardImageProvider (custom provider, macOS)
widget.ErrClipboardImageEmpty          // nil or empty image
```

`ClipboardImage` may BLOCK (waits for the owner: 300 ms for the owner's answer and
again 300 ms for every next chunk of a big picture on X11/Wayland, then PNG/BMP
decoding): call it in a goroutine and `Engine.Post` the result, like
`ClipboardFiles`. `ClipboardSetImage` on Windows encodes PNG/DIB immediately
(no window owner, so no delayed rendering) — for big pictures call it off the
engine goroutine too; on X11/Wayland it only copies the picture and encodes PNG
on the first request (cached for the next ones). `UseMemoryClipboard()` supports
images (copy in, copy out; text/HTML/files and the image displace each other).

| | Windows | X11 | Wayland |
|---|---|---|---|
| put | `PNG` (registered), `CF_DIBV5` (32-bit BGRA, `BI_BITFIELDS`, NOT premultiplied), `CF_DIB` (24-bit, alpha blended over white) — one `OpenClipboard`, most descriptive first | owner of `CLIPBOARD`; `TARGETS` = `TARGETS`, `image/png` only (no text targets); `INCR` above 128 KiB | `wl_data_source` with the single type `image/png` |
| read | `PNG` → `CF_DIBV5` → `CF_DIB` (a present-but-broken format falls through to the next) | `TARGETS`, then `image/png`, else `image/bmp`; owner not answering `TARGETS` with a refusal is probed blindly (PNG, BMP); `INCR` from the owner supported | the offer's `image/png`, else `image/bmp`/`image/x-bmp` |

- DIB decoder (`internal/dib`): `BI_RGB`, `BI_BITFIELDS`, `BI_ALPHABITFIELDS`;
  1/4/8-bit palettes, 16/24/32-bit; bottom-up and top-down; `CORE`/`INFO`/`V4`/`V5`
  headers; alpha only from an explicit alpha mask (the 4th byte of 32-bit
  `BI_RGB` is ignored). RLE and JPEG/PNG-in-DIB are `ErrUnsupported`. Limits:
  100 Mpx, 64 MiB of data on Linux; PNG size is checked from the header before
  decoding.
- X11 `INCR` (both directions): sending uses 128 KiB chunks (a `ChangeProperty`
  without BIG-REQUESTS is capped at 256 KiB), subscribes to `PropertyNotify` of
  the requestor window and unsubscribes at the end; an abandoned transfer is
  dropped after 30 s. Receiving collects chunks on `PropertyNotify`. Text
  paste from an `INCR` owner works too now (it used to return the 4-byte size).
- Pure helpers: `internal/dib` (DIB/BMP), `internal/clipimage` (type choice,
  PNG, clone), `window/clipboard_image.go` (lazy PNG), `widget/clipboard_image.go`.
- WinLine note: `cliprdr` carries only text and files, so an image does not reach
  the RDP client until WinLine adds `CF_DIB` there; `image/png` works for ordinary desktops.
- Limitations: no `image/bmp` is offered (only read); macOS has no provider.

### Wayland window icon (E7)

`Window.SetIcon(icons...)` now works on Wayland through
`xdg_toplevel_icon_manager_v1` (`xdg-toplevel-icon-v1`; KDE 6.1+, Sway, ...) when
the compositor announces the global in the registry; otherwise it returns
`ErrIconUnsupported` as before (WinLine takes the icon from `.desktop`). Each image
becomes a square ARGB8888 `wl_shm` buffer (premultiplied; non-square is centred
in a transparent square; equal sizes deduplicated; more than 1024 px skipped),
`add_buffer(scale=1)`, then `set_icon(toplevel, icon)` and `icon.destroy`; the
pool of the previous icon is released on replacement, `SetIcon()` with no
arguments sends `set_icon(toplevel, null)`. Before `Run()` the icon is remembered
and applied after the first configure.

---

## Windows 11 shell remarks: quick settings, notification stacks, focus card — v3.34

Answers the WinLine remarks after connecting v3.33 (real Windows 11 screenshots
`real_quick_settings.png`, `real_notification_center_calendar.png`). Public API is
additive; Windows 10, Windows 2000 and macOS frames are byte-identical to before
(the frozen taskbar tests and the golden tests pass without updates). The Windows
11 frames change where the remarks ask. Every new look is switched by a **profile
flag** raised in the Windows 11 profiles (light and dark); a consumer that relied
on the v3.33 look lowers the flag: `m.SetFlag(theme.KeyQuickPaged, false)`. The
dark profile still declares ≤ 15 own tokens (all new colours are grey veils or
references to existing tokens).

| Flag (theme key) | Default (Windows 11) | Off = v3.33 look |
|---|---|---|
| `theme.KeyQuickPaged` `quicksettings.paged` | on | tile grid scrolls pixel by pixel with a thin bar |
| `theme.KeyQuickFooterEditLeft` `quicksettings.footer.edit.left` | on | battery left, pencil and gear right |
| `theme.KeyNotifyStack` `notificationcenter.stack` | on | group header with a counter and all cards |
| `theme.KeyCalendarFocusCompact` `calendar.focus.compact` | on | title row and an accent «Start» button |

A custom profile that names the presenter but not the flag keeps the old look.

**1. Split tile.** A tile with `QuickAction.HasDetails` is two halves with a 1 px
line between them (part `w11.tile.split`): the left half toggles (`model.Toggle`),
the right half (`›`, metric `quicksettings.w11.chev.w` = 48, half the tile) opens
the page. Hover and press highlight each half separately (the rounded tile is
clipped to the half, so outer corners stay round). An `Unavailable` tile keeps a
live `›`. Tiles without details are single buttons.

**2. Pages instead of scrolling** (`KeyQuickPaged`). A page is `rows` tile rows;
the column right of the grid shows ▲, one dot per page (current = text colour) and
▼ (ends dimmed). Wheel over the grid or the column turns pages (pixels accumulate
to 30 per turn, so a touchpad does not skip pages); click ▲ ▼ and dots; keyboard
focus on a hidden tile turns the page; in edit mode dragging a tile to the grid
edge turns the page once per entry into the edge zone. New: `QuickSettings.Pages()`,
`Page()`, `SetPage(i)` (no-op with scrolling). The page resets to 0 on reopen.

**3. Footer** (`KeyQuickFooterEditLeft`): left «Edit» (pencil + label, string
`StrQuickEditShort`), then the battery, gear on the right. «Done» of edit mode takes
the pencil's place.

**4. Disabled sliders.** `QuickSettings.SetVolumeEnabled(bool)` /
`VolumeEnabled()`, `SetBrightnessEnabled(bool)` / `BrightnessEnabled()` (enabled by
default). A disabled slider is grey (disabled states of `w11.slider.*`), ignores
mouse, wheel and keys, gives no tooltip and drops out of Tab order; disabling in
the middle of a drag ends the drag.

**5. Two-line tile label.** The title wraps on words to at most two lines, the
second one ends with `…`; a word wider than the cell is elided. When the title takes
one line the second line is `QuickAction.Detail` (it is dropped when the title takes
two).

**6. Page builder** (`desktop/quickpage.go`). Instead of own widgets plus
`ApplyThemeTree`:

```go
q.Details = func(id desktop.QuickActionID) *desktop.QuickDetails {
    page := q.NewDetailPage()            // or desktop.NewDetailPage(tm)
    list := page.AddList([]desktop.DetailListItem{
        {Icon: wifi, Title: "Avanpost", Subtitle: "Secured", State: "Connected", Selected: true},
        {Title: "Guest", Subtitle: "Open"},
    }, func(i int, it desktop.DetailListItem) { connect(it) })
    page.AddSlider(sunIcon, 0.7, setBrightness)
    page.AddToggle("Metered connection", false, setMetered)
    page.AddButton("More networks", openSettings).SetAccent(true)
    page.AddText("Hint")
    return page.Details("Wi-Fi")         // QuickDetails{Title, Content: page}
}
```

The page draws every frame from the theme parts `w11.page.*`, `w11.slider.*`,
`w11.label/detail/dim`, so theme, dark mode and accent changes show immediately.
Handles: `DetailList` (`SetItems`, `Items`, `Selected`, `SetSelected`),
`DetailSlider` (`SetLevel`, `Level`, `SetEnabled`, `SetIconAt`), `DetailToggle`
(`SetOn`, `On`, `SetTitle`, `SetEnabled`), `DetailButton` (`SetTitle`, `SetAccent`,
`SetEnabled`), `DetailText.SetText`; `Set*` are safe from any goroutine and do not
call the callbacks. The page lays elements out in a column, scrolls with the wheel
when too tall, supports keyboard (Up/Down between rows, Enter/Space, Left/Right on
the slider) and tints list icons like tiles (`KeyQuickIconTint`). Panel side:
the widget that took a press keeps the mouse until release (a slider released
outside the page does not stick), and `widget.Focusable` content is told when
the keyboard enters or leaves it. Existing `QuickDetails.Content` widgets work as
before.

**7. Focus card** (`KeyCalendarFocusCompact`). As in the screenshot: no title, one
row `− 30 min +` … `▶ Focus` (neutral button, string `StrFocusTitle`; running:
countdown and `■ Stop`, progress bar under the row); module height metric
`calendar.w11.focus.compact` = 60. `CalendarFlyout.FocusSingleButton = true` leaves a
single full-width `▶ Focus` button without the duration stepper (the session runs
for the duration already set by `FocusSession.SetDuration`). Flag off: title,
stepper and accent «Start» as in v3.33. Note: the screenshot itself shows the
stepper together with the button, so the stepper is the default.

**8. Notification stacks** (`KeyNotifyStack`). Several notifications of one app:
group header (icon, name), the newest card and a pill button inside it
`+N notifications` (`StrNotifStackMore` + `.one/.few/.many/.other`, Russian
«+N уведомл.», picked by `PluralForm`); click expands all cards, the header
(click, ←/→) collapses back; Tab/Enter work on the pill. Closing the group (×) removes
all. A single notification stays a plain card with the app row.

**9. Page-only tile** (`QuickAction.PageOnly`). A tile that only opens a nested
page ("Accessibility"): no halves, no divider line, one hover/press over the whole
tile; click anywhere (and Enter/Space on the focused tile) opens the page via
`QuickSettings.Details`, and the "›" at the right is just a part of the tile (no own
zone, no own focus stop, no "More" tooltip). `PageOnly` wins over `HasDetails`, so a
tile with both flags is whole; `PageOnly` alone is enough to get a page. `Disabled`
keeps it shut; `Unavailable` dims it, the page still opens (it shows why);
`model.Toggle` is not called for the tile; in edit mode it is a draggable tile
as any other. The flag is only read by the Windows 11 quick settings.

**10. Animations** (existing animation system, tokens in `theme.GetAnimation`).
- Quick settings pages slide vertically (`theme.KeyQuickPages`
  `quicksettings.pages`, 200 ms `out-cubic`, a decorative animation: reduced motion
  = instant). The grid window moves between pages; events and focus are laid out
  for the target page at once, only the frame is animated; a new `SetPage` during
  the motion restarts it from the current position. Only the grid window is
  repainted (the pager column stays still). The page resets without motion on reopen.
- Notification stack expand/collapse uses the existing token `notification.expand`
  (functional: reduced motion makes it instant). While moving, the cards are laid out
  as expanded and cut by a growing window; the first card shrinks between the view
  with the "+N" pill and without it, the pill leaves with the card's bottom edge, the
  panel (it grows upward from the calendar) is repainted as a whole, old and new
  rectangles only. The end of the motion equals the static layout.

**11. Tab order.** The pager column joins the Tab order right after the tiles:
▲ (when not on the first page), one stop per dot, ▼ (when not on the last page); an
arrow on an end page is dimmed and skipped. Enter/Space turns the page / goes to the
dot; when the arrow the focus stood on gets dimmed, the focus moves to the dot of the
shown page. The focus ring is the usual one. Arrow Down from the last tile row still
goes to the sliders.

**12. Focus duration menu** (`CalendarFlyout.OpenFocusDurationMenu`,
`FocusMenuOpen`, `CloseFocusMenu`, `FocusDurations`). Right click on the "▶ Focus"
button (on release) or the Menu key / Shift+F10 on the focused button opens a
`PopupMenu` above it: 15, 30, 45, 60, 90 min (`DefaultFocusDurations`, replace per
calendar with `FocusDurations`), a check on the current one; a choice calls
`FocusSession.SetDuration` (clamped to `FocusMinDuration`..`FocusMaxDuration`). The
menu is a child of the calendar, so a click on an item is not a click outside; Esc
closes the menu first and the calendar next; closing the calendar closes the menu;
no menu while a session runs. Works with `FocusSingleButton` too. The menu is
painted by the theme's menu style like every shell menu.

Tests: `desktop/quickpanel_v334_test.go`, `quickpanel_page_test.go`,
`quickpanel_wrap_test.go`, `notifycenter_stack_test.go`, `quickpanel_pageonly_test.go`
(page-only tile, pager Tab order, page slide), `notifycenter_stackanim_test.go`,
`calendarflyout_focusmenu_test.go` (+ `_engine_test.go`), `theme/profiles_win11_quick_test.go`;
visual frames with `QS_OUT` / `NC_OUT`.

---

## v3.34 additions (drag of files: receiver events and source)

Two additions to file drag & drop. The drop itself (`SetOnFilesDropped`) is unchanged
on every platform; everything below is added next to it. Package `window`.

### Receiver events — `SetOnDragEnter` / `SetOnDragOver` / `SetOnDragLeave`

Lets a drop zone light up while files are dragged over the window.

```go
func (win *Window) SetOnDragEnter(fn func(x, y int))
func (win *Window) SetOnDragOver(fn func(x, y int) bool) // true: a drop here is accepted
func (win *Window) SetOnDragLeave(fn func())
```

- `x, y` are LOGICAL pixels of the client area (like `SetOnFilesDropped`). The
  callbacks run on the engine goroutine.
- `enter` is always followed by `over` at the same point (on X11 `XdndEnter` carries no
  point, so `enter` is reported with the first `XdndPosition`).
- `over` answers whether a drop at that point is accepted. `false` becomes a refusal
  on the OS side (Win32 `DROPEFFECT_NONE`, X11 `XdndStatus` without accept, Wayland
  `wl_data_offer.accept(null)`), `SetOnFilesDropped` is not called. Without the
  callback a drop is accepted everywhere (as before). The backend waits for the
  answer for 25 ms; a slower callback is answered with its previous answer.
- `leave` is reported when the pointer leaves or the drag is cancelled. After a drop
  `leave` does NOT come — clear the highlight in `SetOnFilesDropped` too.
- Only drags that carry FILES are reported (`CF_HDROP` / `text/uri-list`); a text or
  image drag does not reach the callbacks and is refused.
- Win32 effect for an accepted drag follows the keys, as in Explorer: nothing/Ctrl —
  copy, Shift — move, Ctrl+Shift or Alt — link (limited to what the source allows).
  Wayland and X11 accept as copy. The application only reads the files: it never
  moves them for the source.

Win32: the window is now registered as an OLE drop target (`RegisterDragDrop`,
`IDropTarget`) instead of `DragAcceptFiles`. If OLE cannot be started or the
registration is refused, the old `WM_DROPFILES` path is enabled (no enter/over/leave
then, `SetOnFilesDropped` works as before).

### Source — `BeginDragFiles`

```go
type DragEffect uint32 // bit mask; values equal DROPEFFECT_*
const ( DragEffectCopy DragEffect = 1; DragEffectMove = 2; DragEffectLink = 4 )

func (win *Window) BeginDragFiles(paths []string, effect DragEffect) bool
func (win *Window) SetOnDragFinished(fn func(effect DragEffect))
```

Call it from a mouse handler while the button is HELD (usually from the first move
that leaves the press point by a few pixels). `effect` is the set of actions the
source allows; the target chooses one.

- Return `true`: the drag was started (it is carried by the OS asynchronously or in a
  modal OS loop). `false`: it cannot be started — no button held / no press serial,
  the backend cannot (macOS; Wayland compositor older than `wl_data_device_manager`
  v3), empty list or mask, another drag is running. Relative paths are made absolute.
- The end is reported once to `SetOnDragFinished(effect)`: the action chosen by the
  target (`DragEffectCopy|Move|Link`), `0` — cancelled or refused (Esc, drop on
  nothing, target refused, the pointer grab failed, X11 target silent for 10 s).
  For `DragEffectMove` the APPLICATION removes the originals — the OS does not.
- The engine is told the button was released when the drag ends (the OS took the
  release), so widgets do not keep a mouse capture.

Per platform:

| | mechanism |
|---|---|
| Win32 | `DoDragDrop` on the window thread with a hand-built `IDataObject` (`CF_HDROP`, `IEnumFORMATETC` from the shell) and `IDropSource` (Esc cancels, release of the starting button drops) |
| X11 | XDND v5 source: owns `XdndSelection`, grabs pointer and keyboard (Esc), finds the `XdndAware` window under the cursor, `XdndEnter/Position/Leave/Drop`, serves `TARGETS` and `text/uri-list`, waits for `XdndFinished`. Starts from the event pump (a service `ClientMessage` to itself), because only the pump may read server replies |
| Wayland | `wl_data_device.start_drag` with a `wl_data_source` offering `text/uri-list`, actions copy/move (`set_actions`), data written lazily on `send`; finish from `dnd_finished`/`action`/`cancelled`. `Link` has no Wayland action (it is dropped; link-only returns `false`) |

The drag icon (`BeginDragFilesImage`) and X11 `XdndProxy` came in the second round —
see "Full screen on a chosen monitor, cursor over popups, drag icon, XdndProxy".
Not done (honestly): X11 draws no drag icon, target type lists longer than one type,
drag from a popup window on Wayland, dropping a virtual (non-file) item on Win32
(`FileGroupDescriptor`).
Checked: Win32 on the real OLE (two windows, `SendInput`, `HEADLESS_GUI_DND_E2E=1`),
X11 on a real X server (two windows, two connections; the pointer events are fed to
the source directly, no XTest), Wayland on the wire against a fake compositor only
(not on a live one).

Tests: `window/dnd_drag_test.go` (events, coordinates, answer timeout, source
wiring), `window/dnd_hdrop_test.go` (`CF_HDROP` block, `QueryContinueDrag` table),
`window/dnd_windows_test.go` (vtable layout, `QueryInterface`/`AddRef`/`Release`,
`GetData`/`EnumFormatEtc`, the receiver cycle), `window/dnd_windows_e2e_test.go`,
`window/wayland_drag_linux_test.go`, `window/x11_drag_linux_test.go`,
`window/x11_drag_e2e_linux_test.go`.

---

## v3.34: full screen on a chosen monitor, cursor over popups, drag icon, XdndProxy

Second round for the PixGo list "not done" (`ENGINE_TASKS.md`). Package `window`.
Additive; nothing removed, no signature changed.

### `SetFullscreenOn(Monitor)`

```go
func (win *Window) SetFullscreenOn(m Monitor) bool
// leave: the same win.SetFullscreen(false); state/notifications: IsFullscreen, SetOnFullscreenChanged
```

`m` is a value from `Monitors()`, `MonitorAt` or `MonitorOf`. `Monitor` got an
UNEXPORTED key (`id`: Win32 `HMONITOR`, X11 the 1-based index in the `RRGetMonitors`
reply — also the EWMH monitor index —, Wayland the registry global name of the
`wl_output`, which is the same on every connection to the compositor). The public
fields did not change. A monitor is found again in the CURRENT list by key, then by
`Name`, then by `Bounds` (`matchMonitor`), so a stored `Monitor` survives a replug
as long as one of the three still matches.

- `false`: the monitor is gone (matches nothing), `Monitor{}`, or the backend cannot
  choose (macOS). Nothing is sent in that case.
- Before `Run` the request is remembered with the monitor; at window creation
  `SetFullscreenOn` is applied, and when the monitor vanished meanwhile the window
  falls back to plain `SetFullscreen(true)`. A later `SetFullscreen(true)` before
  `Run` replaces the remembered monitor.
- A window that is ALREADY full screen is moved to `m`.

| | how |
|---|---|
| Win32 | window thread: `enterFullscreen(rcMonitor of the matched monitor)` instead of the monitor the window lies on. Already full screen: `SetWindowPos` to the new `rcMonitor`, no exit, no notification, the snapshot for leaving stays the first one. Checked live: every monitor of the machine in turn (one monitor on the development machine; the "other rect" path is checked with a synthetic rect) |
| X11 | monitors are re-queried (RandR 1.5, separate connection). If the window manager lists `_NET_WM_FULLSCREEN_MONITORS` in `_NET_SUPPORTED`: that `ClientMessage` (top = bottom = left = right = the index, source = application) and then `ADD _NET_WM_STATE_FULLSCREEN`; the window is NOT moved, so after leaving it returns to where it was. Without it: the window is moved to the monitor's top-left corner and then `ADD` (managers put a full-screen window on the monitor it mostly lies on); the window stays on that monitor after leaving. Already full screen: with the hint just the message; without it `REMOVE`, move, `ADD` (the application sees `false` then `true`) |
| Wayland | `xdg_toplevel.set_fullscreen(wl_output)` with the output object of the window's OWN connection that matches the monitor (a `Monitors()` value comes from another connection: matched via the global name). Already full screen: another `set_fullscreen(output)` (allowed by the protocol), the saved size is kept. The compositor may pick another output — `MonitorOf` shows where the window really is |

### Forced cursor over popup windows

`Engine.SetCursorOverride` now reaches menus and drop-down lists moved to their own OS
window ("popup host"). Each popup is an OS window with its own cursor; before, the
cursor of the carrier window never got there.

- Win32/X11 (`popupHost`): on a change (`surface.refreshCursor` → `popupCursor` hook)
  the form is set on every open popup window (window thread), and on a popup that is
  being created (so an override set before the menu opened works). Cleared → the
  arrow. The cursor of widgets inside popups is still not computed (arrow, as before).
- Wayland (`childPopups`): the popup is a `wl_surface` of the same client and the
  pointer/cursor are shared, so the cursor of the carrier already applied; what was
  missing is a form for the time the pointer is ON a popup. `WaylandWindow.SetPopupCursor`
  keeps it; `applyCursor` uses it while `ptrOverPopup` and falls back to the window's
  form otherwise. Dialog and dock windows (`modal_host`, `dock_host`) are wired the same way.

### Drag icon — `BeginDragFilesImage`

```go
func (win *Window) BeginDragFilesImage(paths []string, effect DragEffect, img image.Image, hotspot image.Point) bool
func (win *Window) DragImageSupported() bool // false: the drag starts but no icon is drawn (X11, macOS)
```

`img` is in SCREEN pixels (the window scale is not applied), `hotspot` is the point of
`img` under the cursor. A side above 512 px is scaled down (hotspot with it), the
alpha channel is kept; `nil`/empty `img` = plain `BeginDragFiles`. Return value and
`SetOnDragFinished` are the same as for `BeginDragFiles`.

- Win32: `IDragSourceHelper.InitializeFromBitmap` (`CLSID_DragDropHelper`,
  `SHDRAGIMAGE`, 32 bpp premultiplied DIB, `CLR_NONE` colour key = alpha). The helper
  stores the picture and service flags in OUR `IDataObject` through `SetData`, so the
  data object now keeps arbitrary `HGLOBAL`/`IStream` blocks (`CF_HDROP` cannot be
  replaced) and lists them in `EnumFormatEtc`. The window's `IDropTarget` forwards
  `DragEnter/Over/Leave/Drop` to `IDropTargetHelper`, so the icon is drawn over the
  window itself too, not only over foreign programs. Without the helpers the drag runs
  without an icon.
- Wayland: `wl_shm` pool (memfd, destroyed right after `create_buffer`) → ARGB8888
  premultiplied buffer → `wl_surface` passed as the `icon` of `wl_data_device.start_drag`;
  after `start_drag`: `attach(buffer, -hx, -hy)` (the compositor puts the surface's
  top-left at the cursor), `damage_buffer`, `commit`. HiDPI: integer window scale →
  `set_buffer_scale`, offsets in surface units. Surface and buffer are destroyed when the
  drag ends. Needs `wl_data_device_manager` v3 (as the source itself).
- X11: **no icon, on purpose.** XDND has no icon; a ghost window under the pointer needs
  a compositing manager (or the SHAPE extension) to be transparent, and the target search
  descends the windows under the pointer, so the ghost would be found as the target.
  `X11Window` does not implement the icon interface; the drag runs plain.

### X11 `XdndProxy` in the source

When the window under the pointer has an `XdndProxy` property (type `WINDOW`) the source
validates it per the XDND specification (the proxy window must carry its own `XdndProxy`
pointing to itself — otherwise it is a leftover of a crashed program and is ignored; a
window pointing to itself is not a proxy), reads `XdndAware` from the PROXY (then from
the window), and sends `XdndEnter/Position/Leave/Drop` to the proxy with the target
window in the event's `window` field. `XdndStatus`/`XdndFinished` still come from the
target window id. The property is read once per window per drag (cache cleared at the
start and end of a drag). Checked against a fake server only (no real proxying program
available); the `window`-field rule is taken from the specification.

Tests: `window/fullscreen_test.go` (`fsOnSpy`, `matchMonitor`, request before `Run`),
`window/fullscreen_windows_test.go` (`TestWin32Fullscreen_OnMonitor`,
`..._SetFullscreenOnPumped`), `window/x11_fullscreen_on_linux_test.go`,
`window/wayland_fullscreen_on_linux_test.go`, `window/popup_cursor_test.go`,
`window/popup_cursor_windows_test.go` (real popup windows),
`window/wayland_popup_cursor_linux_test.go`, `window/dnd_image_test.go`,
`window/dnd_image_windows_test.go` (live shell helpers, `SetData`/`IStream` store),
`window/dnd_windows_e2e_test.go` (`TestDndE2E_DragWithImage`, `HEADLESS_GUI_DND_E2E=1`),
`window/wayland_drag_image_linux_test.go`, `window/x11_drag_proxy_linux_test.go`.

---

## Touch and gestures (v3.34)

Touch is a separate kind of input, not "a mouse with a finger": there are several
fingers, each with its own history. Everything below is **additive**; an app that
knows nothing about touch behaves exactly as before.

### Widget API (`widget/touch.go`, `widget/gesture.go`)

```go
type TouchPhase int // TouchBegan, TouchMoved, TouchEnded, TouchCancelled
type TouchEvent struct {
    ID   int           // same from Began to Ended/Cancelled; unique among simultaneous fingers
    X, Y int           // widget gets LOGICAL px in ITS OWN frame (inside a ScrollView: content coords)
    Phase TouchPhase
    Time  time.Duration // monotonic, from the OS timestamp; 0 = unset (the engine stamps it)

    Pressure     float64 // 0..1; 0 = UNKNOWN (no force sensor / backend cannot read it), not "light touch"
    Major, Minor float64 // contact ellipse axes (diameters) in px, same units as X, Y; 0 = unknown
    Orientation  float64 // major-axis angle from vertical, clockwise, degrees; meaningful only if Major > 0
}
// ev.HasPressure(), ev.HasContact() — "is it known"
type TouchHandler interface{ OnTouch(ev TouchEvent) bool }
```

Delivery (`Engine.SendTouch(ev) bool`; input is PHYSICAL px like `SendMouseMove`):

1. `Began`: widgets under the point, deepest first (bubbling, like a mouse press), get
   `OnTouch`. The first one that returns `true` becomes the **owner** of this touch.
   An open overlay (menu, dropdown) is older than the tree: if the point is inside it
   the overlay takes the touch or nobody does — a widget under a menu never steals it.
   Like a press, a `Began` dismisses popups outside the path (`Dismissable`).
2. `Moved/Ended/Cancelled` go ONLY to the owner, wherever the finger went (capture by
   ID). Each finger has its own owner; two fingers on one widget arrive as two ID
   streams — that is what a pinch is built from. A ScrollView scrolling mid-touch is
   followed (frame re-evaluated per event).
3. A touch nobody took: `SendTouch` returns `false`. **The engine does NOT turn it
   into a mouse.** Stray events (Moved with an unknown ID) also return `false`.
4. `Engine.CancelTouches()` cancels all (focus lost, system gesture); a repeated
   `Began` with a busy ID cancels the old owner first. `Engine.TouchActive()`.

`Window.SetTouchEnabled(on)` / `TouchEnabled()` (package `window`): touch is **opt-in**.
Not asked ⇒ no backend catches touches and the OS keeps producing mouse as always
(Explorer, Calculator, Notepad, the shell are unchanged). Auto-on when the root widget
itself implements `TouchHandler`; an explicit call always wins. Call before `Run`
(Win32 and Wayland also accept it later; X11 turns it on later from the event loop,
so touch starts a moment after the call — see the table).

**No duplicate mouse.** A window that took touch takes it entirely:

| Backend | How the OS mouse-from-touch is kept out |
|---|---|
| Win32 | `WM_POINTERDOWN/UPDATE/UP/CAPTURECHANGED` for `PT_TOUCH` only (`GetPointerType`, `GetPointerTouchInfo`, syscall, no CGO). The message is handled and NOT passed to `DefWindowProc` ⇒ the system generates neither mouse nor `WM_GESTURE`. Pen and everything else go to `DefWindowProc` as before. **Pressure / contact**: `POINTER_TOUCH_INFO.pressure` (0..1024 → 0..1) and `rcContact` are read ONLY when the matching `TOUCH_MASK_*` bit is set (a driver stub in an unmasked field is not "force"); `Major`/`Minor` are the long/short side of the contact BOUNDING RECT (Windows gives no ellipse), `Orientation` is passed through when masked. `EnableMouseInPointer` is NOT enabled, so a real mouse is unchanged. The `0xFF515700` mouse signature is deliberately not filtered: it marks the pen too. Not asked ⇒ every `WM_POINTER*` goes to `DefWindowProc` (verified live: 4 touch events and 0 mouse when enabled; mouse events and 0 touch when disabled). Cancel: `POINTERUP` with `CANCELED` and `CAPTURECHANGED` of a still-tracked touch. |
| Wayland | `wl_seat.get_touch` only when asked AND the seat advertises `touch`; `down/up/motion/frame/cancel`; surface units × surface scale = buffer px (like window sizes); `up` has no coordinates — the last point is reused; `cancel` / losing the capability ⇒ `Cancelled`. **Contact shape** (`shape`/`orientation`, `wl_touch` v6; there is NO pressure in `wl_touch`, `Pressure` stays 0): the version of `wl_touch` is the version of the seat it was created from, and the window's main seat is bound at v4 on purpose (higher changes pointer events). So when the compositor advertises `wl_seat` ≥ 6, a SECOND `wl_seat` object of the same global is bound at v6 only for `get_touch` (released together with `wl_touch`); pointer/keyboard stay on v4. With v6 the events are held until `frame` (shape events follow `down`/`motion` of the same frame) and then delivered with `Major`/`Minor` (× surface scale) and `Orientation`; below v6 delivery is immediate as before. |
| X11 | XInput 2.2: `QueryExtension("XInputExtension")`, `XIQueryVersion(2,2)`, `XISelectEvents(XIAllMasterDevices, TouchBegin|Update|End)` at window creation; `GenericEvent` (type 35) tails are now read in full in every event loop. **Enabling after the window exists** (`Window.SetTouchEnabled(true)` later): the request is remembered, the sleeping event loop is woken with a harmless `GetInputFocus`, and at the top level of the loop (between events, like `x11SyncWMState`) the same negotiation runs through `x11Reply`, which dispatches events that arrive before the reply instead of dropping them (a request made before the loop starts runs first thing in `RunEventLoop`). Repeated enabling does not resubscribe. **Pressure / contact** are device axes (valuators), not event fields: after subscribing, `InternAtom(only_if_exists)` for "Abs MT Pressure" / "Touch Major" / "Touch Minor" / "Orientation" / "Position X" and one `XIQueryDevice(XIAllDevices)` give each device's axes with min/max; the event tail (buttons mask, valuator mask, FP3232 values) is read per `sourceid`; events carry only CHANGED axes, so the last value per touch is remembered. Pressure = (v−min)/(max−min); contact axes are in device units and are converted to px by `screen width / range of the X axis` (right when the touchscreen spans the whole X screen; overstated on multi-monitor setups); orientation = v / axis limit × 90°. A device that appears later (unknown `sourceid`) triggers ONE re-query between events. libinput-driven touchscreens usually expose no pressure axis (then 0), evdev-driven ones do. The server does not emulate a pointer for a client that selected touch; if one does anyway, core Button/Motion events of an emulating touch (`XITouchEmulatingPointer`, same timestamp as its end) are dropped (`x11TouchGuard`). |

**Unclaimed touch ⇒ mouse (done by the window, not the engine).** Otherwise ordinary
buttons, menus and text fields would be dead under a finger in a touch-enabled window.
The first unclaimed finger becomes: `MouseMove`, left `Down`, `MouseMove`s, left `Up`;
other unclaimed fingers are ignored (one mouse). `Cancelled` releases the button
OUTSIDE the window (`-1,-1`) so nothing is clicked. Lost activation cancels all touches.
**Long press ⇒ right button, as Windows does it:** a finger that lay within the tap slop (10 logical px, scaled)
for at least `Window.SetTouchLongPress` (0 = 500 ms, <0 = never) is released as a right click at the lift point
instead of a left one: the left button (already down, press feedback is immediate) is released OUTSIDE the window
(nothing fires, like on cancel), then the pointer moves back and `MouseRight` down+up is sent ⇒ context menu. It acts
on LIFT, not on timeout, so a finger that aims at a slider thumb or a selection and then drags (leaving the slop
at any moment, even if it returns) stays an ordinary left button, and no timer is needed. Time is taken from the
event timestamps when the backend gives them, else from arrival time. Claimed touches are never converted.
Cost: the OS touch-pan wheel is still not produced for unclaimed touches — use the recognizer's `Pan`/`Swipe`.
Wayland popups (`xdg_popup`) take only mouse: the first finger on a popup is its
mouse (same handlers as the pointer), other fingers there are ignored, cancel releases
outside so a menu item does not fire.

### Gesture recognizer

```go
var r widget.GestureRecognizer          // zero value = default thresholds
gs := r.Feed(ev)                        // []Gesture: Tap, Pan, Pinch, Swipe
// or embed the ready handler:
type Viewer struct{ widget.Base; widget.TouchGestures }
v.OnGesture = func(g widget.Gesture) { switch g := g.(type) { case widget.Pinch: … } }
```

* `Tap{X, Y, Count}` — short touch without movement; `Count` 2 = double (the first Tap
  is not delayed).
* `LongPress{X, Y}` — the finger lay still (within `TapSlop`) for `LongPressDuration` (500 ms). Fires while
  the finger is still down. No `Tap` after it, and the same finger no longer produces `Pan`/`Swipe` (Android
  semantics: a menu opened by long press must not scroll under a trembling finger); a second finger starts
  a pinch as usual. The recognizer has no goroutines and no timers; time comes from the owner, either through
  events (every event with `Time` first "settles" an overdue long press) or `Tick(now)` (same clock as
  `TouchEvent.Time`); `NextDeadline()` says when `Tick` is due (`ok=false`: nothing to wait for). `TouchGestures`
  does this itself with `widget.Animate` (engine-driven, same goroutine as `OnTouch`; `Reset()` also stops it).
* `Pan{Phase, X, Y, Delta, Total}` — one finger dragging. `Began.Delta` is the path
  already walked to the threshold (nothing is lost); `Ended`/`Cancelled` follow.
* `Swipe{Dir, Velocity, VX, VY, Start, End}` — a fast flick; comes AFTER the `Pan Ended`
  it grew from (use `Swipe` alone for page flipping, `Pan`+`Swipe` for inertia).
* `Pinch{Phase, Scale, Step, Center, CenterDelta}` — two fingers: `Scale` relative to
  the start, `Step` relative to the previous event (multiply zoom by `Step`), midpoint
  and its shift. Two-finger scrolling arrives as `Pinch` with `Scale≈1`.
* Second finger ends a `Pan`; the finger left after a pinch is dead until all lift;
  a third finger changes nothing.

`GestureConfig` (zero field = default; all in LOGICAL px): `TapSlop` 10,
`TapMaxDuration` 300 ms, `DoubleTapInterval` 300 ms, `DoubleTapSlop` 32, `PanSlop` 10,
`PinchSlop` 10 (change of finger distance), `SwipeMinDistance` 48,
`LongPressDuration` 500 ms (<0 = off), `SwipeMinVelocity` 350 px/s, `SwipeVelocityWindow` 100 ms (finger stopped before lift
⇒ no swipe), `SwipeAxisRatio` 1.5 (≈34° from the axis). Pure logic, no platform.
Events without `Time` cannot give tap duration or swipe speed.

Tests: `widget/gesture_test.go` (synthetic sequences), `engine/touch_test.go` (capture by
ID, two fingers, bubbling, overlay, modal, HiDPI + scroll frame, pinch through the engine),
`window/touch_test.go` (opt-in, mouse emulation, long press ⇒ right button with an injected clock), `window/touch_windows_test.go`
(struct layout, phases, pressure/contact by `TouchMask`, live `InjectTouchInput` with `HEADLESS_GUI_TOUCH_E2E=1` — checks pressure 32/1024 and a 4×4 contact arrive),
`window/waylandtouch_test.go` + `wayland_touch_linux_test.go` (wire bodies, binding,
popups, `wl_touch` v6 shape/orientation, second seat), `window/x11touch_test.go` +
`x11touch_linux_test.go` (requests, event bytes, `XIQueryDevice` reply parsing, valuator decoding,
fake X server handshake incl. events before a reply, deferred enabling, event loop with `GenericEvent`
tails, live XInput 2.2 on a real server, incl. enabling after creation).

Not verified on live hardware: Wayland `wl_touch` (incl. v6 shape, the second seat) against a real
compositor, and X11 touch events / pressure axes from a real touch device (wire format and fake server
only; the XI 2.2 negotiation, the deferred subscription and the parsing of a real `XIQueryDevice` reply were
checked against a real XWayland, which has no touch device). Win32 touch with pressure/contact was checked
live with `InjectTouchInput`; real digitizer force is not verified. Not done: Cocoa (no touch on macOS
desktop), `WM_POINTER` history points, a pen's pressure/tilt (the pen still goes to `DefWindowProc` as a mouse).

---

## v3.35: bringing the window to the front (`window` package)

Answers the PixGo task E14. The API is additive; nothing public was removed, no
signature changed. `SetForeground` of `Win32Window` / `X11Window` keeps working
(the X11 one now sends a timestamp, see below).

Use case: a viewer lives in one window. A second launch (double click in the file
manager) passes the file to the first process, which must come out from under other
windows and out of the minimized state.

```go
win.Activate() bool                       // un-minimize + bring to front; meaning of true differs per platform (below)
win.ActivateWithToken(token string) bool  // same with the launcher's activation token; "" = Activate()
win.IsMinimized() bool                    // Win32/X11 exact, Wayland always false; before Run: false
win.SetOnMinimizedChanged(func(minimized bool)) // engine goroutine; may be set before Run; Wayland never fires

window.TakeActivationToken() string       // reads AND clears XDG_ACTIVATION_TOKEN / DESKTOP_STARTUP_ID; "" if none
window.AllowForeground(pid int) bool      // Win32 AllowSetForegroundWindow; other platforms: always false
```

All three `Window` methods are goroutine-safe and may be called from the handler of an
inter-process message. Before `Run` and on a backend without support (macOS)
`Activate*` return `false`. They are optional backend interfaces (`activateBackend`,
`minimizedBackend`), so `NativeWindow` did not change.

### The two-process scheme

```go
// second process (started by the file manager), as early as possible in main:
tok := window.TakeActivationToken()          // before launching anything: the token is one-shot
window.AllowForeground(mainPID)               // Win32: grant the right BEFORE sending
instance.Send(path, tok)                      // PixGo's own channel; strings between processes

// main process, in the handler of that message:
win.ActivateWithToken(tok)                    // token empty / unknown platform: plain Activate()
```

`TakeActivationToken` reads both variables and always removes both from the process
environment (a child process must not inherit a one-shot token). In a Wayland session
(`WAYLAND_DISPLAY` set and `HEADLESS_GUI_X11` not `1`) it returns `XDG_ACTIVATION_TOKEN`
first, otherwise `DESKTOP_STARTUP_ID` first; if the preferred one is empty it returns
the other. On Windows and macOS there are no tokens: always `""`.

### What `true` means, per platform

| | `Activate()` | `ActivateWithToken(tok)` | `IsMinimized()` |
|---|---|---|---|
| Win32 | A minimized window is restored (`SW_RESTORE`), a hidden one is shown (`SW_SHOW`, e.g. hidden to the tray), then `BringWindowToTop` + `SetForegroundWindow`. **`true` = the window really became the foreground one.** On refusal: the window IS restored but stays under the others, its taskbar button flashes until the user switches to it (`FlashWindowEx`, `FLASHW_ALL \| FLASHW_TIMERNOFG`) and the result is `false`. No `AttachThreadInput` / fake Alt tricks: they circumvent the system's focus-stealing protection. | = `Activate()` (no token; the right is given by `AllowForeground`) | `IsIconic`; the event comes from `WM_SIZE` / `SIZE_MINIMIZED` |
| X11 | `_NET_ACTIVE_WINDOW` (source 1 = application) with the **time of the last key/button event in the window** (0 = `CurrentTime` if there was none) + `MapWindow` first if the window is minimized (ICCCM 4.1.4: Iconic → Normal is done by the client) + raise + `SetInputFocus` (skipped for a minimized window, it is not viewable yet). **`true` = the request was sent**; the window manager may only highlight the window if the time is older than its last input. | the time comes from `_TIME<n>` of the token (`DESKTOP_STARTUP_ID`, startup-notification); no `_TIME` ⇒ the time of the last input. The launcher is told that startup is over: `_NET_STARTUP_INFO` `remove: ID=<id>` (chunks of 20 bytes, `_NET_STARTUP_INFO_BEGIN` first, `PropertyChange` mask), otherwise GNOME/KDE keep the busy cursor until the timeout | `_NET_WM_STATE_HIDDEN` or `WM_STATE` = Iconic; both re-read at the top level of the event loop after `PropertyNotify`, so right after `Minimize()` the old value is still returned |
| Wayland | `xdg_activation_v1`: asks the compositor for a token (`get_activation_token` + `set_serial(serial of the window's last input, seat)` + `set_surface` + `set_app_id` + `commit`) and on `done(token)` sends `activate(token, surface)` and destroys the token. **`true` = the request was sent** (the answer is asynchronous); **`false` = no `xdg_activation_v1` global** (WinLine, Weston/WSLg): nothing can be raised. Without a fresh input serial GNOME/KDE usually only highlight the window. | `activate(token, surface)` at once | **always `false`**: `xdg_toplevel` has no "minimized" state (there is only the `set_minimized` request), the compositor does not tell the client. A window minimized by the user cannot be told from a hidden one. Compositors normally un-minimize on `activate` |

`Win32Window.SetForeground()` (SetForegroundWindow only, no restore) is unchanged.
`X11Window.SetForeground()` and the tray restore now put the last-input time into
`_NET_ACTIVE_WINDOW` instead of 0, with the same logic as `Activate`. On Wayland
`SetForeground` stays a no-op; use `Activate`.

### Incidental fix on X11

- `Window.Minimize` (`WM_CHANGE_STATE`, IconicState) addressed the **root** window
  instead of the client window, so window managers ignored it; the atom was also
  interned inside the event loop (a request-reply in the middle of reading events).
  Now the message names the client window (ICCCM 4.1.4) and the atoms
  (`WM_CHANGE_STATE`, `WM_STATE`, `_NET_WM_STATE_HIDDEN`, `_NET_STARTUP_INFO*`) are
  interned in `Create`.

### Verified

Tests: `window/activate_test.go` (common layer, `_TIME` parsing, `TakeActivationToken`
incl. the real process environment), `activate_windows_test.go` (a real window:
minimize → `Activate` → not minimized, hidden window shown, `FLASHWINFO` layout,
`AllowSetForegroundWindow` argument), `activate_x11_linux_test.go` (bytes of
`_NET_ACTIVE_WINDOW`, time from input events, `_TIME` token, `_NET_STARTUP_INFO`
chunks, `MapWindow` of a minimized window, state from `_NET_WM_STATE` and `WM_STATE`),
`activate_wayland_linux_test.go` (`bind`, `activate`, the whole token round trip),
`activate_e2e_linux_test.go` (live X11 / Wayland, skipped without the feature).
Every change was checked by mutation.

Not verified live: **a successful foreground change on a real X11 window manager**
(the WSLg window manager does not minimize or activate at all, so the e2e test
skips), **Wayland `xdg_activation_v1` against a real compositor** (WSLg/Weston and
WinLine do not announce the global; only the wire against a fake server), the Win32
refusal path with `FlashWindowEx` (the test session always has the foreground right).
Not done: macOS (`Activate` = `false`).

---

## v3.36: appending a batch to ObservableCollection (`widget/datagrid`)

Answers Go.Git GG-93 (the journal loads history in pages of 500 rows).

`ObservableCollection.AddRange(items []any)` appends in one operation and sends
ONE `CollectionAdd`: `Index` is the first new item, `Count` the number added,
`NewItems` the items in order (`NewItem` is the first one). An empty slice does
nothing and sends no event; the slice is copied, so the caller may reuse it.

`CollectionChangedEvent` gained `Count` and `NewItems`. `Add` and `Insert` still
send `Count == 0` (meaning one item) and `NewItems == nil`; `Added()` returns
the number of added items either way. **A handler that mirrors the collection
from events must read `Count`/`NewItems`**: `NewItem` of a batch is only its
first element. Every subscriber inside the engine (DataGrid, TreeView,
CollectionView, XAML `ItemsControl`) handles batches.

Why not the old ways: `SetItems(all)` copies the whole slice and sends `Reset`,
which drops the selection (GG-81) — the old rows could now be other items.
`Add` per item made `DataGrid` rebuild its row index and drop the cell cache on
every item, so a 500-row page was 500 full rebuilds.

`DataGrid` now takes a fast path for rows appended at the end when no column is
sorted: it appends only the new row indices, keeps the selection, and keeps the
cell cache (keyed by grid row, which does not move). This covers both
`AddRange` and a plain `Add` at the end. Insertion in the middle, an active sort
or a collection changed again in between take the full rebuild as before; with
a sort, new rows land in sorted order.

Tests: `widget/datagrid/addrange_test.go` (one event, copy of the slice, empty
batch, selection and cell cache kept, `Add` at the end on the fast path, sort,
insertion in the middle).

## v3.37: opaque parts of a popup menu — `PopupMenu.OverlayRects`

Asked by WinLine: its shell puts the picture of a menu over OTHER programs'
windows opaquely, into rectangles it gets from the shell. `OverlayBounds`
returns one rectangle around the menu and all open cascades, so it took the
empty space between a menu and its submenu, and the pixels outside the rounded
corners — wallpaper showed through over Notepad or Chromium.

```go
for _, r := range menu.OverlayRects() { // nil when the menu is closed
    addOpaque(r)                        // absolute logical coordinates, like OverlayBounds
}
```

- One part list per open level — the menu itself and every open cascade,
  recursively — of the opaque BODY only: no shadow, no pixel outside the
  rounded corners. A corner is a staircase of rectangles that keeps one logical
  pixel of margin inside the arc, which covers the anti-aliased edge and the
  rounding of physical coordinates on HiDPI. Rows with the same inset are merged
  (radius 8 gives about a dozen rectangles per level).
- `OverlayBounds` is unchanged: the engine still uses it to move a menu into a
  native popup window.
- Safe from any goroutine, like `OverlayBounds`.
- Interface `widget.OverlayRectsProvider` for a consumer that walks overlays
  generically; only `PopupMenu` implements it so far (menu bar menus, context
  menus and the window system menu are `PopupMenu`s).
- If the theme gives the menu a translucent background (a material), the body
  itself depends on what is drawn under it — the rectangles still say where the
  menu is, not that its pixels ignore the backdrop.

Tests: `engine/popuprects_test.go` renders a menu with an open cascade on two
different backgrounds at scales 1, 1.5 and 2 with radii 8, 4 and 0: no pixel
inside `OverlayRects` differs between the two frames, the rectangles cover at
least 97% of the background-independent pixels, and `OverlayBounds` does catch
background pixels (so the scene is meaningful).

### v3.37: a hosted popup's shadow is no longer declared opaque on Wayland

A menu moved into its own Wayland popup surface (`SetPopupSink`, Notepad's menu
bar with a cascade open) wore a solid black frame where its shadow is. The
popup host set the bands of `engine.OpaqueBands` — every pixel with non-zero
alpha, the translucent shadow included — both as the input region and as
`wl_surface.set_opaque_region`. A compositor that trusts the opaque region
(mutter, WinLine) copies those pixels without blending.

- New `engine.SolidBands(img)` — bands of pixels with alpha 255 only (no
  shadow, no anti-aliased edge); unlike `OpaqueBands` it also answers for an
  image without holes. `OpaqueBands` is unchanged and still drives the window
  shape and the input region.
- The Wayland popup sets the opaque region from `SolidBands` and the input
  region from `OpaqueBands` as before (a click on the shadow still belongs to
  the menu).

Tests: `engine/solidbands_test.go`, `window/wayland_popup_opaque_linux_test.go`
(on the wire: a shadow pixel is in the input region but not in the opaque one,
the menu bodies are opaque, the gap between levels is in neither).

---

## v3.37: transparent popup windows on X11 (`window` package)

No public API: a defect fix. Popups that the engine lifts into their own OS windows
(`Engine.SetPopupSink`: menus, cascaded submenus, dropdowns) on X11 used to be
created with the root visual — 24 bits, no alpha. The overlay image carries a
transparent/semi-transparent shadow and transparent points behind the rounded
corners; a window without an alpha channel shows them **black** (a thick black
frame around the menu body and black corners). Wayland popups were already
`ARGB8888`; X11 now matches.

- The server's setup reply is parsed past the screen header: the list of depths
  and visuals is scanned for a **TrueColor visual of depth 32 with masks
  R=`0xFF0000`, G=`0x00FF00`, B=`0x0000FF`** (`x11Screen.ArgbVisual`).
- `X11Window.CreatePopup` with such a visual: `CreateColormap(AllocNone, root,
  visual)`, then `CreateWindow` of depth 32 with that visual and the value mask
  `BackPixel=0 | BorderPixel=0 | OverrideRedirect | EventMask | Colormap`
  (values in ascending bit order; `BorderPixel` is mandatory when the visual
  differs from the parent's, otherwise `BadMatch`). A round trip right after the
  requests catches a server-side refusal and falls back to the old 24-bit window.
  The colormap is freed in `Close`.
- No 32-bit visual on the screen — the old behaviour (24 bits, root visual).
  The main window is untouched (XRGB, its own corner mask `cornermask.go`).
- Output into such a window — `PutImage` and `ShmPutImage` (MIT-SHM) with
  **depth 32**. The overlay image is already **premultiplied** (the whole engine
  back buffer is), which is what compositors expect from an ARGB window, so the
  conversion is the usual R/B swap with alpha kept; the colour is **not**
  multiplied by alpha a second time (that would darken semi-transparent edges).
- Popup `Expose` is now answered from the last frame the popup was given (the host
  has no repaint callback for popups): an exposed region of an ARGB window would
  otherwise stay transparent, of a 24-bit one — filled with the background.
- Without a compositing manager an ARGB window does not blend: transparent points
  show as black as before. The engine cannot yet be told "alpha is not
  supported" to cut the popup to the body without the shadow; the check would be
  the owner of the `_NET_WM_CM_S<screen>` selection. Not done: XWayland sessions
  blend regardless of that selection, so it cannot be used to *disable* the alpha
  window.

Tests: `window/x11argb_test.go` (visual list parsing incl. wrong class, masks,
depth, truncated data; `CreateColormap`/`FreeColormap`/`CreateWindow` bytes and
value order), `x11argb_linux_test.go` (setup reply parsing; `CreatePopup` against a
fake X server: depth 32, mask, value order, request order, 24-bit fallback without
a visual and after a server refusal; `PutImage`/`ShmPutImage` depth 32 and
premultiplied bytes; `Expose`; colormap freed; live e2e on a real X server with
`GetImage`: alpha is 0 outside the body, the shadow keeps its alpha). Every change
was checked by mutation.

### v3.37: pixels of alpha buffers are passed premultiplied, once

The engine's frames are premultiplied (`image.RGBA` by Go's definition, the
whole canvas back buffer): an anti-aliased edge of a white menu is
`(28,28,28,28)`. The Wayland popup buffer (ARGB8888) multiplied colour by alpha
a second time and got `(3,3,3,28)` — a dark fringe along rounded corners. Both
alpha buffers — the Wayland popup and the X11 depth-32 popup — now swap R/B and
clamp each channel to alpha (`convRectBGRAPremul`, `swapRBRowPremul`): a valid
premultiplied pixel passes unchanged, a pixel whose colour exceeds alpha (a
straight colour handed in as premultiplied — the light halo the old
multiplication was added against) is clamped, so a straight white edge
`(255,255,255,28)` becomes the correct `(28,28,28,28)`. 24-bit X11 windows and
XRGB Wayland windows ignore alpha and are unchanged.

### v3.37: X11 popup without a compositing manager — shaped to solid pixels

A 32-bit ARGB popup is transparent only where a compositing manager blends
alpha. Without one the server ignores alpha, and the menu's translucent shadow
and the pixels outside its rounded corners show black — exactly like the
24-bit window. At popup creation the X11 backend checks whether alpha is
blended: the `XWAYLAND` extension (XWayland's compositor blends ARGB windows
itself) or an owner of the EWMH selection `_NET_WM_CM_S0`. When alpha is not
blended — or the window is 24-bit — and the server has the SHAPE extension, the
popup window is shaped (`ShapeRectangles`, Bounding) to the SOLID pixels of each
frame (`engine.SolidBands`): no shadow, no anti-aliased edge, no gap between
cascade levels — nothing left to turn black. The same shape is not resent on
every frame. Cost without a compositor: no soft shadow, stepped rounded
corners. With a compositor nothing changes: the ARGB popup stays rectangular
with its soft shadow. The check runs once, when the popup is created.

Tests: `window/x11shape_test.go` (request bytes), `window/x11shape_linux_test.go`
(fake server: no compositor → shape to solid pixels, shadow and gap cut out,
no resend of the same shape; compositor or XWayland → no shape; 24-bit →
always shaped).

## v3.37.1: a click on a Start tile no longer moves it

Under an open overlay the engine delivers a mouse move in two steps: "cursor is
nowhere" (`widget.CursorNowhere`) to the whole tree — so whatever the cursor
left under the overlay drops its hover — then the real point to the overlay's
owner. The owner got the "nowhere" too, right before the real point. The
Windows 10 Start menu (and the Windows 11 pinned grid) took it for a jump of
2^30 points from the press and started dragging the tile, so a click with the
slightest hand tremor between press and release moved the tile instead of
launching it.

- The engine now skips the overlay owner's subtree when it broadcasts
  "nowhere" (`broadcastMouseMoveSkip`): the owner and its children get only the
  real point. Everything under the overlay still gets "nowhere". This applies to
  every overlay with dragging, not only Start.
- `StartMenu` itself ignores `widget.CursorIsNowhere` coordinates for tile and
  cell dragging and for its scroll thumb; such a move only drops the hover.

Tests: `engine/overlaymove_test.go`; in `desktop` a click through the engine
with a move on the spot and a one-pixel tremor launches the tile / pinned app
and leaves `TileGroups` unchanged (Windows 10 and 11), and a "nowhere" move
between press and release does not start a drag.

## v3.37.2: `SetFocused` is called outside the focus lock

The engine's focus manager held its lock while it called `SetFocused(false)` on
the old widget and `SetFocused(true)` on the new one. A widget that moved focus
(`Engine.SetFocus`) or asked for it (focused widget, `CaretRect`) from inside
`SetFocused` took the same lock and waited for itself forever — PixGo 1.0.0's
gallery address field hung that way (it finished editing on focus loss and
called `SetFocus`), and so did the Wayland event loop
(`textInputSendCaret` → `CaretRect`).

Now only "who has focus" changes under the lock; notifications run after
`Unlock`. If focus changes again during the notifications — from `SetFocused`
itself or from another goroutine — the last value wins: the widget that was
told `true` gets `false`, the latest target gets `true`, intermediate targets
get nothing. Widgets that keep bouncing focus between each other from
`SetFocused` are cut off after 16 rounds instead of hanging. Calling
`SetFocus` and querying focus from `SetFocused` is therefore safe.

Tests: `engine/focuslock_test.go` (focus moved from `SetFocused(false)`,
focus queried from `SetFocused(true)`, ping-pong) — each hangs with the old
implementation and fails on a timeout.

---

## v3.38: initial position and "maximized" before the window is shown (`window` package)

Asked by PixGo (E18): it saves the window position and the maximized flag, but
there was nothing to restore them with. `Win32Window.Create` put the window in
the centre of the screen and showed it at once, and `Native()` appeared only
after that; moving and `Maximize()` afterwards made a visible jump and the
maximize animation on every start.

```go
win.SetInitialBounds(x, y, w, h) // before Run
win.SetInitialMaximized(true)
```

Units: `x`, `y` — SCREEN coordinates of the top-left corner (as
`Monitor.Bounds` and `NativeWindow.GetPosition`: physical desktop pixels, may be
negative); `w`, `h` — LOGICAL pixels (as `SetMinSize` and the engine canvas), the
window multiplies them by the scale. On Win32 the scale is the one of the monitor
the rectangle (at the system scale) falls on (`initialScaleFor`: largest
intersection, else the monitor under the top-left corner, else the system scale;
`HEADLESS_GUI_SCALE` wins): `WM_DPICHANGED` is not sent at creation, so a window
created on a 200% monitor with the primary's scale would be half the size to the
eye. `w <= 0 || h <= 0` — the size is not set (the position still applies);
the size from `SetInitialBounds` wins over the root `widget.Window` markup size
and, in `FitScale`, only sets the first window size (the design size stays, the
frame is fitted at once as on a resize).

Applied once, in `bringUp` before `Create` (`initialPlaceBackend`, an optional
backend interface like `fullscreenBackend`); after `Run` the setters change
nothing, and `Native().SetPosition/SetSize` work as before. Together with
`SetFullscreen` before `Run`: the window is created with the bounds, goes
full screen after `Create`, and leaving it returns exactly the initial bounds
and the "maximized" state. After a maximized start `IsMaximized` is `true`,
and a root `widget.Window` gets `SetMaximized(true)` before the first frame.

| Backend | Position / size | Maximized |
| --- | --- | --- |
| Win32 | straight into `CreateWindowEx` (the window has no frame: window rect == client rect) | created without `WS_VISIBLE`, the first show is `ShowWindow(SW_SHOWMAXIMIZED)`: the system takes the creation rectangle as the normal one, so "restore" returns it (no `SetWindowPlacement`: its `rcNormalPosition` is in workspace coordinates, shifted by the taskbar) |
| X11 | `CreateWindow(x, y)`; `WM_NORMAL_HINTS` with `USPosition\|PPosition` (+ `USSize\|PSize` when the size is set), one property together with the minimum size | `_NET_WM_STATE` = `MAXIMIZED_VERT`,`MAXIMIZED_HORZ` as a property BEFORE `MapWindow` (EWMH initial state); `IsMaximized` is true at once, a manager that refuses rewrites the property and `PropertyNotify` returns false |
| Wayland | `x`, `y` ignored (the compositor places the window), `w`, `h` — the initial size | `xdg_toplevel.set_maximized` before the first `commit`; the min==max size fixing is skipped (it would not let the compositor maximize); the normal size is remembered and used when a `configure` of 0x0 comes after unmaximize |
| Cocoa | no-op (`SetPosition` is a stub there) | no-op |

`Win32Window.clientSize` (the `clientSizer` interface) is new: a window shown
maximized gets its size inside `Create` (`WM_SIZE` comes before the resize
handler is attached), and `adoptCreatedSize` now sees it for Win32 too and brings
the canvas to the real size; for an ordinary window the size matches and nothing
happens.

Tests: `window/initial_test.go` (order of calls to a fake backend, units and
HiDPI, monitor scale, `FitScale`, once-only, `widget.Window` state, pure
functions `win32InitialCreate`, `initialScaleFor`, `x11SizeHints`,
`wlNormalSize`); `window/initial_windows_test.go` (live Win32 windows:
`GetWindowRect` equals the requested one, created `IsZoomed`, restore returns the
initial rectangle, with full screen on top, `SetPosition` after start);
`window/initial_x11_linux_test.go` (`Create` against a fake X server: the
`CreateWindow` position, `WM_NORMAL_HINTS` and `_NET_WM_STATE` come BEFORE
`MapWindow`, the minimum size is not lost); `window/initial_wayland_linux_test.go`
(`Create` against a fake compositor: `set_maximized` after `get_toplevel` and
before the first `commit`, no fixed size, restore to the initial size, refusal).

---

## v3.38: shell remarks of WinLine after v3.32.0 and v3.37.1

Seven small fixes; every one has a test.

**Tray glyph size no longer depends on `PadY` or `tray.fill.strip`.**
`trayInner` (tray.go) now always gives the glyph a box of `tray.icon.size`,
centred in the item's bounds; the style's `PadX` still narrows it, `PadY` does
not eat from it (Windows 2000 drew a 14 px glyph at 6 px, macOS at 4).
`desktop.TrayIcon` uses `trayInner` too (it used its own `shrinkByPad`).
`drawDiagonalStrike` now clips to the half-open box, so the strike corner
(`Max.X, Max.Y`) no longer leaves the glyph. **Frames changed on purpose:** the
frozen taskbar hashes of Windows 2000 (both variants) and macOS (both) in
`taskbar_frozen_hashes_test.go` were updated; a pixel diff of the old and new
frames shows only the tray glyphs and the notification bell changed; all
Windows 10 frames are byte-identical.

**`widget.NewIconSetFS(fsys fs.FS, dir string)`** — an `IconSet` that reads
`IconRef.Source` SVG files from an `fs.FS` (for `go:embed`) under the
subdirectory `dir` (`""` = root), with the same optical sizes (`Sizes` +
`{size}`) as the disk set. Paths use `/` (a backslash is accepted); an absolute
path or `..` out of `dir` is rejected like on disk (the icon gets the
placeholder). `svg.ParseFS(fsys, name)` is the matching parser.

```go
//go:embed icons/*.svg
var iconsFS embed.FS

set := widget.NewIconSetFS(iconsFS, "icons")
tm.SetIconResolver(set)
ref := theme.IconRef{Source: "wifi_{size}.svg", Sizes: theme.IconSizes(16, 20, 24)}
```

**Menus of built-in profiles.** Windows 10 declares `menu.chevron.right` = 18
(the thin chevron of Windows 11 instead of the solid triangle; its soft shadow
— `Elevation` 6 and a `Shadow` colour — was already declared). Windows 2000
(and Blue) declares the flag `menu.icon.tint`: item icons are repainted in the
item's text colour per state, so a consumer no longer sets `MenuItem.IconTint`
on every item; an app that needs colour icons clears the flag with
`Manager.SetFlag`. The Windows 11 menu border is `RGBA(0,0,0,20)` instead of
15: on the 249 fill it gives 229, the reference value (15 gave 234).

**Submenu delay counts from the hover.** `menu.submenu.delay` used a plain
`widget.Animate` timer whose clock starts at the first `StepAnimations` — in a
headless test the first call only started the clock. The timer (internal
`animateTimer`) now starts at the hover, so one
`widget.StepAnimations(time.Now().Add(delay))` opens or closes the submenu, and
a frame that comes late does not wait for the delay again. `Animate` itself is
unchanged.

**No sound device.** `desktop.VolState.NoDevice` (new field; the zero value
means a device exists): the tray volume icon is drawn crossed out like `Muted`
(theme icon keys `tray.volume.nodevice`, then `tray.volume.muted`), the tooltip
is "Звук: нет устройства" / "Sound: no device" (`StrSoundNoDevice`) instead of
"0%", and the quick-settings volume tile is not active. A `VolumeItem` created
with a nil `SystemStatus` reports `NoDevice` too.

**Caption buttons of `widget.Window` (not classic).** The glyph colour comes
from the title bar instead of the hard-coded grey 180: the old grey stays where
it has contrast ≥ 3:1 with the title background (so dark titles render as
before), otherwise the title text colour is used (muted for an inactive window,
as in `titleColors`), and black or white if even that is unreadable. A hovered
minimize/maximize button gets a 15 % film — black on a light title bar, white
on a dark one — drawn with `FillRectAlpha` (the old grey was written without
blending and looked dirty on light titles); the hovered close button is red
(232,17,35) with a white glyph as before. `Panel`'s decorative header uses the
same glyph colour. The golden `window_win11_light.png` was updated on purpose:
only the caption glyphs changed (grey → black active, dark grey inactive).

Tests: `desktop/trayglyph_pady_test.go`, `widget/themeicons_fs_test.go`,
`widget/popupmenu_profile_test.go`, `desktop/menu_profile_v338_test.go`,
`tests/popupmenu_builtin_profiles_test.go`, `widget/popupmenu_delay_test.go`,
`desktop/trayvolume_nodevice_test.go`, `widget/window_captioncolors_test.go`,
`tests/window_captionglyph_test.go`.

---

## v3.38: opaque-buffer fast path, frame buffer recycling, lazy fallback fonts

Three engine-side speedups requested by a viewer application that pans a
3840x2160 picture (frame cost 33 ms before, 5.5 ms now) and wants a fast start.
Nothing is removed or changed for existing callers: every new path is opt-in or
gives the same bytes as before.

### Opaque image: `widget.OpaqueImage` (E15)

`DrawImage`, `DrawImageScaled` and `DrawImageRect` composite with `Over`: every
pixel's alpha is read and blended with the canvas. A viewer's own window buffer is
fully opaque, so that is wasted work (22 ms of a 4K frame, on one core). A source
declared opaque is composited with `Src` instead: rows are copied (with the R/B swap
for `FormatBGRX`), and areas of 256K pixels and more are split into horizontal bands
over `GOMAXPROCS` goroutines. Under a rounded clip (`SetRoundClip`) the straight
middle is one band-copy, the corner rows go span by span.

How the engine learns that a source is opaque (it must not scan: `(*image.RGBA).Opaque`
reads every pixel, as much as the copy itself):

```go
buf := image.NewRGBA(image.Rect(0, 0, w, h)) // every pixel has A == 255
img := widget.OpaqueImage(buf)               // keep it next to the buffer, not per frame
ctx.DrawImage(img, x, y)
ctx.DrawImageScaled(img, x, y, w, h)         // 1:1 (physical size == source size)
widget.DrawImageRect(ctx, img, sr, dr, f)    // 1:1 (sr size == dr size in physical pixels)
```

- `*image.YCbCr`, `*image.Gray`, `*image.Gray16` and `*image.CMYK` are opaque by type -
  no wrapping needed. `*image.RGBA` / `*image.NRGBA` are fast-pathed once declared.
- `widget.OpaqueImage(img)` returns `*widget.OpaqueSource` (an `image.Image`, with
  `RGBA64At` and `SubImage`, so other contexts still draw it normally) or `img` itself
  if it is opaque by type / already marked. `widget.OpaqueHinter` (`OpaqueHint() bool`)
  lets your own image type declare it; `widget.IsOpaqueImage(img)` answers without
  scanning and returns the unwrapped image.
- The mark is a promise: all pixels have alpha 255. A broken promise makes translucent
  places overwrite the background instead of letting it through.
- The scaled-image cache is keyed by the wrapped image, so `InvalidateImageCache` works
  with either the wrapper or the buffer.
- Scaled (not 1:1) draws still go through the cache and `Over`; only the 1:1 case is
  fast-pathed. `DrawImageRect` at 1:1 also skips the resampling for any `*image.RGBA`
  (the kernels degenerate to identity - same bytes as before), declared opaque or not.

Tests: `engine/blit_opaque_test.go` (byte-equal to `Over` for RGBA/NRGBA/YCbCr/Gray/
sub-image, both pixel formats, clip, round clip, small and parallel sizes; the `Src`
semantics are proven by a translucent source declared opaque), `widget/opaqueimage_test.go`.

### Recycled frame buffers: `Engine.SetFrameRecycling`, `Frame.Release` (E15)

The 33 MB allocated per full-window frame is the tile data slab (`DirtyTile.Data` are
sub-slices of it). The `Frames()` contract is that the consumer owns `Data` for good,
so the engine cannot reuse it on its own. Recycling is therefore explicit:

```go
eng.SetFrameRecycling(true)
for fr := range eng.Frames() {
    apply(fr)       // read fr.Tiles[i].Data
    fr.Release()    // Data is invalid from here on; the buffer returns to the pool
}
```

- Off by default: behaviour and lifetime of `Data` are exactly as before.
- On: `Data` is valid until `Release`. Not calling `Release` is allowed - the buffer is
  left to the garbage collector. `Release` is idempotent (also on copies of the frame)
  and a no-op for frames not built by the engine.
- Frames the engine drops from a full channel, or discards on resize, are returned by the
  engine itself. Frames shown to a synchronous `FrameSink` are pinned and never returned
  (what a sink keeps is unknown to the engine).
- `window` turns recycling on by itself and releases a frame right after copying it into
  the window buffer.
- Pool: two size classes (up to 1 MB / larger, rounded to 256 KB), `sync.Pool`, so idle
  memory is given back by the runtime. Public pieces: `output.GetSlab`, `output.Slab`,
  `Frame.AttachSlabs`, `Frame.Pin`, `Frame.Release`.

Measured (`BenchmarkPan4K_*` in `engine/pan_bench_test.go`, 3840x2160, one frame = a
whole new picture, Intel Core Ultra 7 265K, Windows):

| | ms/frame | allocated/frame |
|---|---|---|
| before (`Over`, no recycling) | 29.3 | 34.0 MB |
| recycling only | 29.3 | 0.8 MB |
| `OpaqueImage` only | 6.0 | 34.0 MB |
| `OpaqueImage` + recycling | 5.5 | 0.8 MB |
| `DrawImageScaled` 1:1, before / `OpaqueImage` + recycling | 31.8 / 5.5 | 34.0 / 2.6 MB |

The remaining 5.5 ms is the frame diff and the tile copy, spread over the cores.

### Lazy fallback fonts (E16)

`engine.New` used to read every file of the fallback chain (on Windows Segoe UI Symbol,
Nirmala, Segoe UI Emoji and others - about 22 MB, 21 ms, 53 MB allocated by a buffer that
doubled while reading). Now `New` only registers the paths (a `stat` each: a missing file
is not put in the chain, as before). A file is read and parsed when the chain is walked
down to it: a rune absent from the primary font is looked up in chain order and the walk
stops at the first font that has it, so fonts after that one are not touched; a rune that
no font has loads each file once. Ordinary text never enters the chain.

- The chain order is unchanged. Text frames are byte-equal to the eager version (tests
  compare pixels, measured widths and rune positions of plain and shaped text, for three
  chain orders).
- The shaper (RTL, Indic, emoji) walks the chain the same lazy way (`chainFontmap`).
  An emoji rune walks to the first coloured font that has it.
- Thread safety: the frame goroutine and the application's measurer may both trigger a
  load; a load is done once under the entry's mutex, the loaded font is read atomically.
  DPI and sub-pixel mode (`setDPIAll`, `setTextSubpixel`) are applied to loaded fonts at
  once and picked up by unloaded ones when they load.
- `readFileBounded` (used for every font file) now reads a file of known size into one
  exactly sized buffer instead of `io.ReadAll`.
- `Engine.RegisterFallbackFont*` stay eager (the caller hands over data or wants the error).

Measured (first `engine.New` of the process, 1280x800, cold file cache of fonts,
`TestEngineNew_ColdReport` with `HG_REPORT_NEW=1`): 19 ms and 60.4 MB allocated (30.7 MB
stay live) before; 1.5 ms and 7.9 MB (the two canvas buffers; 7.8 MB live) now.

Tests: `engine/fallbackfont_test.go` (no file is read by `New`, by a frame of plain text or
at registration; the first exotic glyph reads exactly the chain prefix up to its font; a
glyph found nowhere reads each file once; eager/lazy equality; DPI and sub-pixel at load;
concurrent loading under `-race`; `readFileBounded` allocation).

---

## v3.38: accessibility — actions on nodes, dialog, radio buttons, scrolling, menus

Asked by PixGo (E17). Additions only; the frames of existing themes are
unchanged.

### Actions on nodes — `widget.AccessActionHandler`

Invoke and SetFocus from a screen reader on a node from `AccessChildren` acted
on the OWNER widget (a synthetic click at its centre). Now the owner can take
the action itself:

```go
type AccessActionHandler interface {
    AccessAction(id string, action AccessActionKind) bool
}
// AccessInvoke, AccessFocus, AccessSelect, AccessToggle, AccessExpand, AccessCollapse
```

- `AccessInfo.ID` (new) is the node's identifier at its owner; an empty one is
  replaced by the node's number in the list. `AccessNode.ActionID` carries it
  to the bridge (`json:"-"`), `AccessNode.Widget` is still the owner.
- `Engine.AccessAction(w, id, action) bool` runs the handler on the engine
  goroutine (`callOnLoop`) and refuses a hidden/disabled owner. Before
  `AccessInvoke/Focus/Select` it moves input focus to the owner if the owner is
  `Focusable` and not an `OverlayDrawer` (a menu would be closed by it).
- `Window.accessibilityNodeAction(node, kind)` is the single entry the bridges
  use. A node with an `ActionID` whose owner implements the interface gets the
  action from it, and the owner's answer is final (no fallback click). Anything
  else behaves as before: Focus → focus the widget; Invoke/Select/Toggle →
  synthetic click; Expand/Collapse → refused.
- Bridges: UIA `Invoke`, `SetFocus`, `Toggle`, `SelectionItem.Select`,
  `ExpandCollapse`; AT-SPI `Action.DoAction` (0 = click, 1 = "expand or contract"
  for nodes with a sub-menu) and `Component.GrabFocus`. An owner's node has
  the `Action` interface in AT-SPI.
- Implemented by `DiffView` (ids `"0"`, `"1"` = side), `MergeView` (`"0".."3"` =
  `MergeSide`; the hidden base pane refuses), `PopupMenu` and `MenuBar`
  (below).

### Dialog in the tree

`Dialog` implements `Accessible`: `RoleDialog`, name = `Title`, description =
`Dialog.Description` (new field; `MessageBox`, `NewConfirmDialog` and the input
dialog fill it with the message), the close button `✕` is a named button.
`StateModal` is added by the engine as before. UIA: control type Window and
`IsDialog = true`; AT-SPI: role `dialog`.

### Scrolled-out elements — `StateOffscreen`, `AccessInfo.Visible`

`BuildAccessTree` tracks the window of every `ContentOffsetter` (ScrollView)
above a node. `Bounds` stay the element's full screen bounds; a partly visible
element gets `Visible` = the intersection (`json:"-"`, zero = not clipped), a
fully hidden one gets `StateOffscreen`. Virtual nodes from `AccessChildren`
are clipped the same way; a widget may preset `Visible`/`StateOffscreen` itself
(a scrolled popup menu does). Menus are never clipped by a scroll window (they
are overlays).

- UIA: `IsOffscreen`, `BoundingRectangle` = the visible part (empty for a hidden
  element). AT-SPI: `showing` is cleared (`visible` stays), `Contains` and
  `GetAccessibleAtPoint` use the visible part.
- Hit testing (`a11ySnapshot.hitTest`) uses the visible part.

### Menus in the tree

`PopupMenu` is `Accessible` + `AccessChildrenProvider` + `AccessActionHandler`:
a closed menu is a `RoleMenu` node with `StateOffscreen` and no children; an
open one covers itself and all open cascades (`StatePopup`) and lists the
items as `RoleMenuItem`: name without the mnemonic mark, shortcut in
`Description` (UIA `AcceleratorKey`), states `disabled`, `checkable`/`checked`,
`haspopup`/`expanded`, `focused` for the highlighted item. Separators are
skipped but keep their number. Item id = path of indices (`"2"`, `"4/1"` = item
1 of the cascade opened from item 4). Actions: Invoke/Toggle run the item like
Enter (cascade items open their sub-menu), Focus/Select highlight the item,
Expand/Collapse open/close the cascade; a disabled item, a separator and a stale
path are refused. A menu with scrolling marks the items outside its window
`offscreen`.

`MenuBar` is `Accessible` + provider + handler: top items (`"t0"`, `"t1"`, …,
focused only while their menu is open) and, while a menu is open, its items
(`"t1/0"`, …). Invoke on a top item toggles its menu (an item without a menu
runs its `OnClick`).

- The snapshot's focus (`UIA GetFocus`, `HasKeyboardFocus`, AT-SPI `focused`)
  goes to a highlighted menu item, over the widget that still holds the engine
  focus. Mere mouse hover over a closed menu bar does not take it.
- Hit testing looks at nodes with `StatePopup` first (they stick out of the
  owner and the window), so UIA `ElementProviderFromPoint` (which takes the
  cursor position, see below) and AT-SPI `GetAccessibleAtPoint` return the
  item under the cursor, not the widget under the menu. AT-SPI `GetLayer` of
  such a node is POPUP.
- UIA: control types Menu / MenuItem; a plain item has `Invoke`, a checkable
  one `Toggle`, an item with a sub-menu `ExpandCollapse`. AT-SPI: roles `menu`,
  `menu item`, `check menu item`; states `has popup`, `expandable`,
  `collapsed`/`expanded`, `checkable`/`checked`.
- Not in the tree (unchanged): the private context menus of `TextBox` and
  `TextInput`, and the drop-down list of `Dropdown`.

### Radio buttons — `SelectionItem` (UIA), `checked` (AT-SPI)

- UIA: the radio button has the `SelectionItem` pattern (`IsSelected`,
  `Select`, `AddToSelection`, `RemoveFromSelection` — refused for the selected
  one, `SelectionContainer` = NULL, the engine does not publish groups) and no
  longer `Invoke`. `IsSelected` is read from the live widget, not from the
  150 ms snapshot. A change of the state raises the property that changed
  (`IsSelected`, `ToggleState`, `ExpandCollapseState`, `IsOffscreen`,
  `IsEnabled`), not `IsEnabled` for everything.
- AT-SPI: a selected radio button is `checked` (and `checkable`), never
  `selected` — that is what Orca reads; the `StateChanged` event is
  `checked` too. List rows keep `selected`.

### Shared focus ring

`widget.DrawFocusRing(ctx, r, corner, base)` draws the two-tone ring that the
taskbar already used (`desktop.PaintFocusRing`): an outer contour (2 px by
default, colour = theme `FocusRing` or `base`, normally the text colour) and an
inner 1 px contrast contour. Theme tokens `theme.KeyFocusRingWidth`
(`focus.ring.width`) and `theme.KeyFocusRing` (`focus.ring`) — also
`ThemeStyle.FocusRingWidth/FocusRing`, carried both ways by the flat-theme
bridge; `desktop.KeyFocusRingWidth/KeyFocusRingColor` are now aliases of the
same keys. `Button.FocusRing` (new, default false) switches a button to it;
it is on for the buttons of `MessageBox`, the confirm, input, file and
progress dialogs. Before, a focused dialog button got a 1 px border in the
accent colour, which on an accent button (OK) was the colour of its fill.

### Known limits

- UIA `ElementProviderFromPoint` still uses the cursor position: `windows.NewCallback`
  cannot receive the `double` arguments. For a popup lifted into its own OS
  window (`SetPopupSink`) UIA asks the popup's window, not the main one, so the
  items are in the main window's tree but not found under the cursor there.
- The bridges are tested at the level of provider vtables (UIA) and the D-Bus
  wire against the real AT-SPI bus (AT-SPI); not with a live Narrator/NVDA/Orca.
- Scroll windows clip by the container's bounds, scroll bars included.
- A radio button group is not published (no UIA `SelectionContainer`).

Tests: `widget/accessibility_nodes_test.go`, `widget/focusring_test.go`,
`engine/accessibility_action_test.go`, `engine/focusring_test.go`,
`window/a11y_node_test.go` (all platforms), `window/a11y_nodes_windows_test.go`
(UIA through the vtables), `window/a11y_nodes_linux_test.go`,
`window/a11y_nodes_live_linux_test.go` (AT-SPI, the second on the real bus).

## v3.38.1: Windows 10 — a large window no longer stays blank

On Windows 10 a window larger than about 1024×1024 physical pixels stayed blank
(clicks still went to it), reported by the RDP client on v3.38.0 on two
Windows 10 22H2 machines. The Win32 frame buffer was a Go slice. The Go heap on
windows/amd64 reserves memory in 4 MB arenas by separate
`VirtualAlloc(MEM_RESERVE)` calls, so a buffer over 4 MB almost always spans two
reservations — and on Windows 10 `StretchDIBits` / `SetDIBitsToDevice` with such
a source copies no line and returns 0 with `ERROR_SUCCESS`. Windows 11 does not.

- The frame buffer is allocated with one `VirtualAlloc(MEM_COMMIT|MEM_RESERVE)`
  (`window/framebuf_windows.go`) and released on resize and in `WM_DESTROY`;
  a late frame after destruction is a no-op.
- The result of `StretchDIBits` is checked: on 0 the window asks for a repaint
  (`InvalidateRect`), at most three times in a row.
- The first `WM_PAINT`, which arrives inside `Create` before the window wires
  its repaint (`SetOnExpose`), is no longer lost: wiring the repaint asks the
  system to repaint again.

Tests: `window/framebuf_windows_test.go` — the buffer lies in one reservation
(`VirtualQuery`), a 1500×950 frame reaches a live window, a frame after
`WM_DESTROY` does nothing, the early paint is re-requested. The blank-window
failure itself only reproduces on Windows 10.

## v3.39: password fields and menu icons (WinLine after v3.38.0)

**Password field without left-behind copies.** A `TextInput` keeps its text in a
`[]rune` buffer that can be wiped (`TakeSecret`, `WipeSecret` since earlier
versions). Editing used to leave copies the application could not reach: every
keystroke allocated a new array and dropped the old one, holding the start of
the password, to the garbage collector; deleting shifted runes left and kept the
array tail; every keystroke built an Undo snapshot string even in password mode;
`OnChange` got a string copy on every change. In password mode now:

- a replaced array is wiped, a shrunk tail is wiped, no Undo snapshot string is
  built;
- `OnSecretChange func(n int)` — when set, a password field reports changes by
  length only and does not call `OnChange` (which would need a string);
- `SecretBytes() []byte` — UTF-8 copy, the field keeps its text (the caller wipes
  the copy); `TakeSecret()` reads and clears; `Clear()` = `WipeSecret()`;
- a password field collected by the GC wipes its buffer (finalizer).
`GetText()` still returns a string — do not call it on a password field.
Revealing the password with the eye button renders it as a string, as before.

**Password mask follows the font.** `NewPasswordInput` no longer pins
`MaskRune` to '●' (U+25CF): with `MaskRune == 0` the mask is '●' when the
default font has it and '•' (U+2022) otherwise. The built-in Open Sans lacks '●';
taken from a fallback font it could come out as a colour glyph ignoring the text
colour — invisible dots on ALT Workstation 11.2. New optional context interface
`widget.GlyphCoverage{HasGlyph(r) bool}` (default font only, implemented by
`engine.Canvas` and forwarded by the scroll and popup wrappers) and
`widget.ContextHasGlyph(ctx, r)`. An explicit `MaskRune` is used as is.

**`menu.icon.tint` tints only one-colour icons.** The Windows 2000 profile flag
used to tint every menu icon, so colour icons (yellow folders, program pictures
in the Start menu) came out as black silhouettes. Now the flag (`TintIcons`)
tints an icon only when all its opaque pixels share one colour (a glyph,
anti-aliased edges allowed); colour icons stay as they are, like in Windows
2000. `MenuItem.IconTint` still tints any icon.

---

## v3.39: showing someone else's buffer without a copy — `widget.ExternalSurface` (E7)

Requested by the RDP client: a session at 3840x2160 is a 33 MB buffer that changes
in pieces. `ImageWidget` / `DrawImage` copy it into engine tiles and compare it with
the previous frame; `ExternalSurface` does not own the buffer and does not keep a
copy. The application hands over its `*image.RGBA`, says which parts changed, and the
widget copies into the canvas only what the canvas repaints in this frame, straight
from the application's memory. Nothing is removed or changed for existing callers.

```go
surf := widget.NewExternalSurface()
surf.SetBounds(image.Rect(0, 0, 1920, 1080))     // logical bounds, like any widget
surf.SetBuffer(session, true)                    // true: every pixel has A == 255
root.AddChild(surf)

// A decoder goroutine: write under the lock, announce after it.
surf.Update(func(buf *image.RGBA) []image.Rectangle {
    copy(buf.Pix[...], decoded)                  // the write itself
    return []image.Rectangle{updated}            // areas, in buffer coordinates
})
```

- **Pixels, HiDPI.** The buffer is in *physical* pixels: one buffer pixel is one
  physical canvas pixel, no scaling at any scale (at scale 2 a 200x100 buffer fills
  logical 100x50). Its top-left pixel (`buf.Rect.Min`) sits at the physical position of
  the widget's top-left corner (edge rounding as for all canvas operations). A buffer
  larger than the widget is cropped, a smaller one leaves the rest to
  `SetBackground(color)` (black by default; keep it opaque). No buffer or an empty
  one - the whole widget is the background.
- **Channel order.** Bytes are `RGBA` as in any `*image.RGBA`. A buffer holding `BGRX`
  bytes would show with red and blue swapped - convert in the application.
- **Damage -> engine damage.** `InvalidateRects(rects)` / `InvalidateRect(r)` (areas of
  the buffer, `buf.Rect` coordinates) convert to logical frame coordinates (outward
  rounding at fractional scales, the scroll offset of a `ScrollView` is applied) and call
  the engine's `InvalidateRect` per area. The engine diffs and ships only the tiles
  those areas touch, not the whole widget. Areas outside the buffer or the widget are
  dropped, empty ones ignored. The scale used is the one the last `Draw` saw (1 before
  the first frame); a scale change repaints the frame in full anyway. `Base.Invalidate()`
  stays: it invalidates the whole widget.
- **What `Draw` copies.** The clip of the frame intersected with the widget, not "the
  areas the application announced". The engine erases and redraws the union of ALL damage
  of a frame (also foreign: a menu closed over the surface, a tooltip vanished), and the
  surface has to restore its pixels there although the application never touched them.
  With only the surface damaged, the clip equals the union of its areas, so exactly what
  was announced is copied. The union is ONE bounding rectangle: far-apart areas cost the
  bounding box (diff still goes per area) - merge them in the application.
- **Synchronisation contract.** The buffer is read only inside `Draw`, on the engine's
  goroutine, under the widget's lock. The application writes it under the same lock:
  `Lock()` / `Unlock()` (the surface is a `sync.Locker`) or `Update(fn)`, which locks,
  runs `fn`, unlocks and then announces the returned areas. Write first, announce after
  (and outside the lock): announcing before the write lets a frame start in between and
  the update is lost. Keep the lock short (a piece of the buffer: tenths of a
  millisecond; a full 4K copy: several ms). The lock is not re-entrant: no `SetBuffer`,
  `Swap`, `Update`, `Buffer` inside it, and nothing that waits for the engine's frame
  lock (screenshots, `SetScale`, `SetResolution`).
- **Double buffering.** `Swap(next, opaque, damage) (prev)` replaces the buffer and
  returns the previous one; it waits for a running `Draw`, so after it returns the engine
  never reads `prev` again and the application may write it without the lock. `next` must
  hold the whole frame; `damage` is what changed against what is on screen (`nil`, or a
  different buffer size / `opaque`, means everything). `SetBuffer(img, opaque)` is
  `Swap` without the result. `Buffer()` returns the current buffer (read it under the
  lock only).
- **Opaque.** `opaque == true` copies rows (`Src`, bands over `GOMAXPROCS` for areas of
  256K pixels and more, the R/B swap for `FormatBGRX`); `false` composites with `Over`.
  A broken promise makes translucent places overwrite the background. An opaque buffer
  with an opaque background makes the surface declare its bounds in `OpaqueRegion`, so the
  traversal skips what lies wholly under it.
- **Context.** `widget.PixelBlitter` (`BlitPixels(src, sr, x, y, opaque)`) is the optional
  context capability behind it, like `ImageRectDrawer`: copies part `sr` of a buffer pixel
  for pixel, the buffer pixel `src.Rect.Min` landing at the physical position of the
  logical point `(x, y)`, under the clip (also a rounded one), no cache, no scaling.
  `engine.Canvas`, the popup context and `OffsetContext` (scrolling) implement it; the
  helper `widget.BlitPixels(ctx, ...)` falls back to `DrawImage` / `DrawImageScaled` for a
  context without it. Usable by your own widgets too.

Numbers (3840x2160, `go test -bench ExternalSurface4K ./engine/`, a frame of the whole
engine including diff and tiles; Intel Core Ultra 7 265K, Windows 11):

| Scenario | ms/frame | B/op |
|---|---|---|
| `ExternalSurface`, 10% of the area changes (the application also writes 3.3 MB) | 0.95 | 0.11 MB |
| before: the application copies the 10% into an `ImageWidget`'s buffer, `OpaqueImage` + `InvalidateRect` | 1.2-1.3 | 0.10-0.19 MB |
| `ExternalSurface`, 10% as 4 far-apart corners (bounding box = the whole buffer) | 4.6 | 0.4 MB |
| `ExternalSurface`, `Swap` of the whole buffer | 5.5 | 1.8-2.7 MB |
| `DrawImage(OpaqueImage)` of the whole buffer (v3.38, `Pan4K_OpaqueRecycle`) | 5.5 | 2.4-2.9 MB |

Besides time, the surface needs no second 33 MB buffer (`ImageWidget` keeps its own
copy that the application has to refill).

Tests: `engine/externalsurface_test.go` (only the announced areas' tiles change in a
frame, exact pixels against the buffer; offset bounds and a buffer with non-zero `Min`;
areas outside; empty / nil / zero-size buffer; buffer smaller and larger than the widget,
`Swap`; scale 2 and 1.25; foreign damage over the surface; `OpaqueRegion`; `Src` vs
`Over`; a writer goroutine plus engine frames and a `Swap` goroutine under `-race`;
`Canvas.BlitPixels` with clip, scale 2, `BGRX`; popup and scroll contexts),
`widget/pixelblit_test.go` (the fallback of `widget.BlitPixels`), benchmarks in
`engine/externalsurface_bench_test.go`.

---

## v3.39: scan codes, raw wheel, window activation (RDP-client requests E1, E6, E9)

### E1 — `KeyEvent.ScanCode`, keys outside the VK table, modifiers as keys

```go
type KeyEvent struct {
    Code     KeyCode
    Rune     rune
    Mod      KeyMod
    Pressed  bool
    Repeat   bool   // auto-repeat (Win32, Wayland, X11)
    ScanCode uint16 // NEW: raw key code of the OS, 0 = not from an OS window
    Extended bool   // NEW: E0-prefixed key (Windows only)
}
```

`ScanCode` is **not normalised**, it is what the platform reports:

| Platform | `ScanCode` | `Extended` |
|---|---|---|
| Windows | bits 16–23 of `lParam` of `WM_KEYDOWN/KEYUP/SYSKEYDOWN/SYSKEYUP` (set-1 hardware scan code) | bit 24 of `lParam` |
| X11 | `keycode − 8` = evdev (`input-event-codes.h`) | always `false` |
| Wayland | `key` of `wl_keyboard.key` = evdev | always `false` |
| macOS | `keyCode` of `NSEvent` (`kVK_*`; note that 0 is the key "A") | always `false` |

A client that needs set-1 scan codes (RDP) keeps its own `evdev → set 1` /
`kVK → set 1` table: it depends on the protocol, not on the window. It comes
with every key — named (`Code == KeyA`) and unnamed.

What reaches the application now:

- A key with no entry in the VK table is **no longer dropped**: it arrives with
  `Code == KeyUnknown`, `Rune == 0` and the `ScanCode` filled (a text event has
  `Rune != 0` and `ScanCode == 0`; the two never mix). A key with neither a
  name nor a scan code (Windows `VK_PACKET` from `SendInput`) is still dropped.
- **Modifiers are keys now**: `KeyShift`, `KeyControl` (either side; the side is
  in `ScanCode` + `Extended`: Windows left Ctrl `0x1D`, right Ctrl `0x1D` +
  `Extended`) and `KeyAltKey` (the physical Alt; AltGr is Ctrl then right Alt on
  Windows, as the OS reports it). `KeyAlt` stays the *gesture* "Alt pressed and
  released alone" and is delivered exactly as before, after the release of
  `KeyAltKey`. `KeyWin` already existed. Only a backend that reports scan codes
  produces these events; `KeyEvent.Mod` and `SetModifiers` work as before.
- New `KeyCode` values, appended after the old ones (old values untouched, all
  equal to the Windows VK): `KeyClear`, `KeyKana` (Kana and Hangul), `KeyJunja`,
  `KeyFinal`, `KeyKanji` (Kanji and Hanja), `KeyConvert`, `KeyNonConvert`,
  `KeyAccept`, `KeyModeChange`, `KeySleep`, `KeyBrowserBack` … `KeyBrowserHome`,
  `KeyVolumeMute/Down/Up`, `KeyMediaNext/Prev/Stop/PlayPause`, `KeyLaunchMail`,
  `KeyLaunchMedia`, `KeyLaunchApp1/2`, `KeyIMEProcess` (Windows IME took the
  key — the character arrives as a text event), `KeyShift`, `KeyControl`,
  `KeyAltKey`. `KeyMenu`, `KeyPrintScreen`, `KeyPause`, `KeyF13`…`KeyF24`
  existed before; on X11/Wayland F13–F24, media, Kana/Hangul/Convert/NonConvert
  and browser keys are now in the evdev table (`window/keytable.go`) and get a
  `Code` too.
- `Repeat` arrives together with `ScanCode` on Win32 (bit 30 of `lParam`),
  Wayland (client-side repeat) and X11; macOS reports `isARepeat`.
- Text input (`SetOnChar` → `Code: KeyUnknown, Rune`) is untouched.

Internally: the optional `rawKeySource` interface (`SetOnRawKey(func(rawKey))`)
of a native window; a backend that has it stops calling `SetOnKeyDown`,
`SetOnKeyUp` and `SetOnKeyDownRepeat`. Custom `NativeWindow`s keep working on
the old path and produce events without `ScanCode`.

Tests: `window/rawkey_test.go` (lParam parsing, tables, the path
backend → surface → engine, text input, repeat, Alt gesture),
`window/rawkey_windows_test.go` (real `wndProc` messages),
`window/rawkey_linux_test.go` (X11 and Wayland wire events),
`engine/wheel_raw_test.go` (`SendKeyEvent` hands the event to the focused widget
unchanged).

### E6 — raw wheel: `widget.WheelEvent`

```go
// widget
type WheelEvent struct {
    X, Y           int     // logical point in the widget's frame
    DeltaX, DeltaY float64 // units of widget.WheelNotch (120 per notch)
    Precise        bool    // touchpad / free-spinning wheel: any value, frequent
    Mod            KeyMod
}
type WheelEventHandler interface{ OnWheelEvent(e WheelEvent) bool }
const WheelNotch = 120.0

// engine
func (e *Engine) SendMouseWheelRaw(xPhys, yPhys int, dx, dy float64, precise bool) bool
```

Signs follow the engine's pixel path: `DeltaY > 0` is **down**, `DeltaX > 0` is
**right**. Windows and RDP count the vertical axis the other way round (up is
positive), so for the RDP wheel field send `-DeltaY`; horizontal is the same
(`MOUSE_HWHEEL`, right positive).

Delivery: the widget that captured the mouse, then an open overlay under the
cursor, then the hit-test path from the deepest widget to the root (inside the
top modal window if there is one). The first `WheelEventHandler` that returns
`true` stops it and `SendMouseWheelRaw` returns `true`. Deltas are **not**
scaled by the HiDPI factor (they are not pixels); the point is.

Next to the old path, not instead of it: `window.Run` first offers the raw
event; **only if nobody takes it** the old path (`SendMouseWheelPixels` /
`MouseWheelUp/Down` ticks) runs for the same wheel movement. So existing
widgets keep getting pixels, and a widget that took the raw event does not get
the tree scrolled a second time. Headless code can call `SendMouseWheelRaw`
directly and ignore the result.

| Platform | Source | `Precise` |
|---|---|---|
| Windows | `WM_MOUSEWHEEL` (`DeltaY = −delta`), `WM_MOUSEHWHEEL` (`DeltaX = delta`), `GET_WHEEL_DELTA_WPARAM` | `delta % 120 != 0` |
| X11 | buttons 4/5 (vertical) and 6/7 (horizontal), ±120 per click | always `false` (no XInput2 smooth scroll in the backend) |
| Wayland | `wl_pointer.axis`: `value / 10 * 120` (a notch is about 10 units); `axis_discrete` (v5) / `axis_value120` (v8) are parsed if the compositor sends them | `true` when no discrete / value120 came with the event — **the backend binds `wl_seat` version 4**, where they never come, so a mouse notch is `DeltaY = 120, Precise = true` |
| macOS | not delivered (the wheel was not emitted before either) | — |

On X11, with the raw receiver installed, a vertical click is passed as one
press+release pair on the old path and the separate `ButtonRelease` of buttons
4/5 is no longer forwarded (otherwise a widget that took the raw event would see
a release without a press). Popup windows lifted out of the main window
(`SetPopupSink`) keep the old path only.

Tests: `engine/wheel_raw_test.go` (routing, bubbling, capture, HiDPI, the pixel
path still works), `window/rawkey_test.go` (consumed / not consumed, conversion
functions), `window/rawkey_windows_test.go` (real messages),
`window/rawkey_linux_test.go` (X11 buttons, Wayland `axis` + discrete + value120).

### E9 — pointer leave and window activation

**Pointer leave.** Already there, now documented and completed:

- Win32 `WM_MOUSELEAVE`/`WM_NCMOUSELEAVE` (`TrackMouseEvent`), X11 `LeaveNotify`,
  Wayland `wl_pointer.leave` (also for a popup) make the window send a mouse
  move to the point `(-1, -1)`; the engine delivers it as `OnMouseMove(-1, -1)`
  to the widgets that contained the previous point (hover is cleared the same
  way as on leaving across the edge).
- **Fixed:** at HiDPI scale 2 the engine truncated `-1 / 2` to `0`, which is the
  first point of the window, and the hover was not cleared. `toLogical` now
  rounds down (non-negative coordinates are unchanged).
- **New:** `widget.PointerLeaveHandler { OnPointerLeave() }` and
  `Engine.SendPointerLeave()` — an event about leaving the **window**, distinct
  from "the pointer moved off this widget" (a point outside a widget cannot tell
  those apart, a captured drag is also outside). Called on the engine goroutine
  for every visible widget that implements it, after the move to `(-1, -1)`.
  `CursorNowhere` is still only for "an overlay covers you".

**Window activation.**

```go
func (win *Window) SetOnActivate(fn func(active bool)) // before Run; nil disables
func (win *Window) IsActive() bool
```

The callback runs on the engine goroutine and only on a **change** (Wayland
reports the state on every `configure`; it is deduplicated). Sources: Win32
`WM_ACTIVATE` (a move of the focus to another window of the same application —
a menu popup, a dialog — is not a deactivation), X11 `FocusIn`/`FocusOut`
(the event mask was fixed in v3.37), Wayland `activated` in
`xdg_toplevel.configure`; macOS reports nothing. The first call is the first
state the platform reports.

**Stuck modifiers — now handled in the window.** The window keeps a list of the
keys that are down. When it becomes inactive it releases all of them in the
engine **before** the callback of the application: ordinary keys first, then
modifiers, each as a normal `KeyEvent{Pressed: false}` with `ScanCode` and the
`Mod` it was pressed with, then `Engine.SetModifiers(0)`. "Alt pressed and
released alone" does not fire in that case. The release goes through the same
path as a real one, so an RDP client sees key-ups for everything it had sent as
key-downs and does not need its own timer. The real release of such a key goes
to the other window and does not arrive; if it does arrive, it is delivered as
usual (a key-up of a key already up is harmless). Touches in progress are
cancelled and lifted popups are closed (as before).

Order for the application at deactivation: key-ups (engine goroutine) →
`SetModifiers(0)` → `Window.SetOnActivate(false)`; the capture of the mouse is
not released by the engine — the application does that in its callback (and, for
an RDP session, sends its own "synchronize keyboard state").

Tests: `window/rawkey_test.go` (`TestReleaseHeldKeys_*`, `TestWindow_SetOnActivate`,
`TestPointerLeave_*`), `engine/wheel_raw_test.go`
(`TestSendPointerLeave_*`, `TestMouseMove_OutsidePointStaysOutsideOnHiDPI`),
`window/rawkey_windows_test.go` (`WM_ACTIVATE`), `window/rawkey_linux_test.go`
(X11 `FocusIn/Out`), existing `wayland_toplevel_linux_test.go`.

Not verified on a live compositor / on a real keyboard: everything here is
checked on wire events and on `wndProc` messages; the scan codes of macOS are
not tested at all (compiled for darwin only).

---

## v3.39: keyboard grab, monitor changes, Wayland window methods, system language, accent colour (`window` package)

Requested by the RDP client (tasks E3, E5, E10, E11, E12). Everything below is
added API; nothing existing changed its signature or behaviour.

### Keyboard grab — `(*Window).SetKeyboardGrab(on bool) bool`

```go
ok := win.SetKeyboardGrab(true) // Win, Alt+Tab, Alt+Esc, Ctrl+Esc, PrintScreen go to the app
win.KeyboardGrabbed()           // is the grab in force right now
win.SetKeyboardGrab(false)      // also done by closing the window
```

While the window has focus the system combinations reach the application as
ordinary `KeyDown`/`KeyUp` events (the same path as every other key, so
`Mod` and `Repeat` behave as usual). A request made before `Run` is remembered
and applied when the window is created; the answer is then provisional ("can
the platform do it at all"). The result `true` means "request accepted": on X11
the grab is taken when the window gets focus, and on Wayland the compositor may
still refuse — `KeyboardGrabbed()` shows the real state.

| | Windows | X11 | Wayland | macOS |
|---|---|---|---|---|
| Mechanism | `WH_KEYBOARD_LL` hook (`window/keygrab_windows.go`), one per process, set on the window thread | `XGrabKeyboard` (`window/x11_keygrab_linux.go`) | `zwp_keyboard_shortcuts_inhibit_manager_v1.inhibit_shortcuts(surface, seat)` (`window/wayland_keygrab_linux.go`) | none, `false` |
| Active when | `GetForegroundWindow` is the window | the window has focus (taken on `FocusIn`, released on `FocusOut`) | the compositor sends `active` (surface focused) | |
| No support | hook refused → `false` | no connection → `false` | global missing → `false` | always `false` |

What is taken on Windows: the Win keys and **every key pressed while Win is held**
(Win+E, Win+D, Win+R...), Alt+Tab, Alt+Esc, Ctrl+Esc (so Ctrl+Shift+Esc too),
PrintScreen, Alt+F4 and Alt+Space. Alt+F4 does not close the window while the grab
is on — keep a way out of the grab in the application. Injected key events
(`LLKHF_INJECTED`: `SendInput`, AutoHotkey, the on-screen keyboard) are never
touched. A key release is swallowed exactly when its press was, even if the focus
left in between, so the shell never sees a release without a press; turning the
grab off "releases" the grabbed keys that are still down to the window. The
decision logic is the pure `keyGrabFilter` (`window/keygrab.go`).

**What cannot be grabbed, by anything:** Ctrl+Alt+Del (the Secure Attention
Sequence is handled by Winlogon on its own desktop) and Win+L on Windows;
Ctrl+Alt+F1...F12 and the screen locker on Linux (the X server or the kernel).
The application offers its own command ("Send Ctrl+Alt+Del", Ctrl+Alt+End), as
mstsc does. On GNOME/Wayland the compositor keeps an emergency shortcut (a long
press of Super+Esc) and may ask the user for consent.

X11 detail: the request is made by the event loop at its top level, not by the
caller (a reply cannot be read from the middle of the event stream). The loop is
woken by a `ClientMessage` the window sends to itself (`SendEvent` with an empty
mask), which — unlike `GetInputFocus` — produces no reply that could be mistaken
for the reply of `GrabKeyboard`. Focus events caused by the grab itself
(`mode = Grab/Ungrab`) are not reported through `SetOnActivate`. Without a window
manager (no focus events at all) the grab is taken at once. Windows opened with
`OpenWindow` have their own connection and event loop and work the same way.

### Monitor changes — `(*Window).SetOnMonitorsChanged(fn func([]Monitor))`

```go
win.SetOnMonitorsChanged(func(ms []window.Monitor) {
    // the fresh list, in the order of window.Monitors(); engine goroutine
})
win.SetOnMonitorsChanged(nil) // unsubscribe
```

`Monitors()`, `MonitorOf`, `MonitorAt`, `MonitorForRect` and `SetFullscreenOn`
are exported and documented since v3.34 (`window/monitor.go`,
`window/fullscreen.go`); the work-area field of `Monitor` is called **`Work`**
(it was never `WorkArea`) and stays so. The callback fires only for a real
change — bounds, `Work`, `Scale`, name, primary or the number of monitors
differ from the last list the application saw (`monitorListsEqual`) — about
150 ms after the last system message (the platforms send bursts, and moving a
window between monitors raises a DPI message although nothing changed). The
first list is read when the subscription starts (on Linux a short extra
connection). It can be set before `Run`; `OpenWindow` windows subscribe
separately; closing the window unsubscribes.

| | Source |
|---|---|
| Windows | `WM_DISPLAYCHANGE`, `WM_DPICHANGED`, `WM_SETTINGCHANGE` with `SPI_SETWORKAREA` (taskbar moved, hidden or resized) |
| X11 | RandR `ScreenChangeNotify`, `CrtcChangeNotify`, `OutputChangeNotify` (`RRSelectInput` on the root, RandR 1.2+; plain `ScreenChangeNotify` below that) and `PropertyNotify` of `_NET_WORKAREA` — on a **separate connection**, because the window's own is read by its event loop |
| Wayland | on the window's own connection: `wl_registry.global` of a new `wl_output` (bound on the fly) and `global_remove` (objects released — `wl_output.release` only from version 3, it is a protocol error earlier), `wl_output.geometry/mode/scale/done`, `zxdg_output_v1.logical_position/logical_size`. Wayland reports no work area (`Work == Bounds`), so a panel moving is invisible there |
| macOS | none (`Monitors()` is empty there) |

### Wayland: what the window methods can and cannot do (E10)

Checked method by method against `xdg_shell`; the tests are
`window/wayland_toplevel_linux_test.go` and `window/wayland_setsize_linux_test.go`.

| Method | On Wayland |
|---|---|
| `SetPosition(x, y)` | **impossible by protocol**: a client neither knows nor sets the global coordinates of a toplevel. A no-op that sends nothing. `GetPosition()` is always `(0, 0)`. The only thing allowed is to ask the compositor to drag the window by the pointer (`BeginMove`, `xdg_toplevel.move`, needs the serial of a button press). `SetInitialBounds` ignores `x`, `y` for the same reason |
| `Minimize()` | works: `xdg_toplevel.set_minimized`. There is no reverse request and no "is minimized" state: `IsMinimized()` is always `false`, `SetOnMinimizedChanged` is a no-op, a minimized window comes back only through `Activate` (`xdg_activation_v1`) or the user's panel |
| `Maximize()` / `Restore()` / `IsMaximized()` | work: `set_maximized` / `unset_maximized`; the state is read from the `maximized` flag of `configure`. A window whose size is fixed (`min == max`) is unfixed first, otherwise the compositor could not maximize it, and fixed again on restore |
| `SetResizable(v)` | works as `set_min_size` / `set_max_size`: `false` fixes `min == max == current size`, `true` removes the maximum and restores the application's minimum (`SetMinSize`). This is the only way Wayland has to forbid or allow stretching |
| `SetSize(w, h)` | **new in v3.39.** It was only remembering the size. For a non-resizable, non-maximized, non-fullscreen window the fixation (`min == max`) now moves to the new size, so the compositor accepts a buffer of that size; for the others the compositor chooses the size and only the frame size is remembered |
| `SetFullscreen` / `SetFullscreenOn` | `set_fullscreen(wl_output)`; refusal arrives as `configure` without `fullscreen` and is reported as `false` (v3.34) |
| Drag source | `BeginDragFiles` / `BeginDragFilesImage` through `wl_data_source` + `start_drag` — done in v3.34 (`window/wayland_drag_linux.go`, tests with a fake compositor) |

### System language — `window.SystemLanguages() []string`

```go
window.SystemLanguages() // ["ru-RU", "en-US"], most preferred first; nil — not set
```

BCP-47 tags (language lower case, script title case, region upper case:
`ru-RU`, `sr-Latn-RS`, `es-419`), no duplicates. It lives in `window` next to the
system theme (the `widget` package does not import `window`); the keyboard
layout language (`widget.Locale`) and the interface language (`widget.Language`)
are separate things.

- **Windows:** `GetUserPreferredUILanguages(MUI_LANGUAGE_NAME)` — the "Preferred
  languages" list in the order the user set; fallback `GetUserDefaultLocaleName`.
- **Linux:** the gettext rules. The main locale is the first non-empty of
  `LC_ALL`, `LC_MESSAGES`, `LANG`; if it is empty, `C`, `POSIX` or `C.UTF-8`
  there is no language and `LANGUAGE` is ignored (as gettext does) — the answer is
  `nil` and the application picks its own. Otherwise the `LANGUAGE` list
  (colon separated, `ru:en_US:en`) comes first, then the main locale. The
  encoding (`.UTF-8`) and modifiers (`@euro`) are dropped; `@latin` and
  `@cyrillic` become the script.
- **macOS:** the same environment variables (`NSLocale.preferredLanguages` is not
  read — the Cocoa backend cannot be verified without a Mac); empty when started
  from a `.app`.

Pure functions `normalizeLanguageTag` and `systemLanguagesFromEnv` are covered
by `window/syslang_test.go` on every platform.

### System appearance: theme and accent colour (E12)

`SetOnSystemThemeChanged` and `DetectSystemTheme` already existed (Windows:
`WM_SETTINGCHANGE` "ImmersiveColorSet" re-reading `AppsUseLightTheme`; Linux, X11
and Wayland alike: the `SettingChanged` signal of the Settings portal; macOS:
`SystemThemeUnknown`, no watching — see `window/systheme_darwin.go`). v3.39 adds
the accent colour:

```go
c, ok := window.DetectSystemAccent()      // color.RGBA (opaque), ok == false when unknown
a := window.DetectSystemAppearance()      // SystemAppearance{Theme, Accent, HasAccent}
win.SetOnSystemAppearanceChanged(func(a window.SystemAppearance) { /* engine goroutine */ })
```

- **Windows:** `HKCU\Software\Microsoft\Windows\DWM\AccentColor` — a DWORD in
  **ABGR** order (red in the low byte), fallback `ColorizationColor` (ARGB). The
  change arrives with the same `ImmersiveColorSet` as the theme (Windows puts both
  in one group), so the value is re-read from the registry.
- **Linux:** the portal key `org.freedesktop.appearance` / `accent-color`, a
  structure of three doubles (r, g, b in [0, 1]); a colour outside [0, 1] means
  "no accent" (GNOME's (-1, -1, -1)). Needs a portal 1.18+; older ones give
  `ok == false`. The signal rule on the bus is the one the theme already used
  (`arg0` = the namespace), so no extra subscription traffic.
- **macOS:** `ok == false`, no watching.

The callback receives the whole `SystemAppearance` and fires only when any field
really changed from what the application was told last; "unknown" theme or
accent never counts as a change and never overwrites the last known value. One
platform subscription serves `SetOnSystemThemeChanged` and
`SetOnSystemAppearanceChanged` together (on Windows each has its own cell in the
window, so setting one does not clobber the other); it is taken when either is
set and dropped when both are cleared or the window closes.

### Tests

`window/keygrab_test.go` (filter, window layer), `keygrab_windows_test.go` (a
real hook installed and removed, events fed to its procedure with a
`KBDLLHOOKSTRUCT` — a real keypress cannot be produced because `SendInput` is
marked injected), `x11_keygrab_linux_test.go` (a fake X server on a socket pair),
`wayland_keygrab_linux_test.go` (a fake compositor), `monitor_change_test.go`,
`monitor_change_windows_test.go`, `x11_monitor_change_linux_test.go`,
`wayland_monitor_change_linux_test.go`, `wayland_setsize_linux_test.go`,
`syslang_test.go`, `syslang_env_test.go`, `syslang_windows_test.go`,
`systheme_accent_test.go` (+ the Windows registry and the fake Settings portal
on a real bus in `systheme_windows_test.go` / `systheme_linux_test.go`).

Verified only on the wire, not on a live compositor or desktop: the Wayland
`zwp_keyboard_shortcuts_inhibit_manager_v1` and the `wl_output` hot-plug paths
(fake compositor); the X11 RandR events (the watcher connects to a real X server
in the test, but a real monitor change cannot be provoked there); the hook
delivering a real Win key press on Windows (the procedure is exercised, the OS
delivery is not).

---

## v3.39: cursor from an image and pointer lock (RDP client E2, E4)

Two additions for remote-desktop clients and games. Nothing public was removed or
changed in signature; frames of existing themes are unchanged.

### Cursor from an image — `Engine.SetCursorImage` (E2)

```go
eng.SetCursorImage(img *image.NRGBA, hotspotX, hotspotY int)   // any goroutine
eng.ClearCursorOverride()                                      // or SetCursorImage(nil, 0, 0)
ci, ok := eng.CursorImageOverride()                            // *widget.CursorImage
```

The pointer image (RDP: up to 384×384, 32 bpp with alpha) is shown above every
widget and above popup windows, exactly like `SetCursorOverride` (v3.34): it is applied at
once, without a mouse move, and menus / drop-down lists moved into their own OS windows
show it too. The last call wins: `SetCursorOverride(shape)` replaces an image,
`SetCursorImage` replaces a shape. `SetCursorImage(nil)` (or an empty / invalid image)
drops an image but leaves a shape set by `SetCursorOverride`; `ClearCursorOverride` drops both.

- The image is **copied** (`widget.NewCursorImage`): the application may reuse its
  buffer. Straight (non-premultiplied) alpha. Sides up to `widget.MaxCursorImageSide`
  (512); a hotspot outside the image is clamped to its edge.
- **Cache.** The image gets a key — SHA-256 of size, hotspot and pixels
  (`widget.CursorImageKey`, computed on the caller's buffer without a copy). The same
  image set again is a no-op for the engine (no copy) and for the window; every window
  keeps at most **16** ready system cursors (`cursorCacheSize`) by this key, evicts the
  least recently used and frees the OS object (Win32 `DestroyCursor`, X11 `FreeCursor`,
  Wayland `wl_buffer.destroy`). The cursor that is shown right now is never evicted.
- `engine.CursorOverride()` reports an image as "set, arrow" for code that knows only
  shapes; the image itself is in `CursorImageOverride()`.

Per platform (`window/cursorimage*.go`, `x11cursorimage*.go`, `wayland_cursorimage_linux.go`):

| | how the cursor is built |
|---|---|
| Win32 | `CreateIconIndirect`: 32-bit DIB with alpha + 1-bit mask. The DIB is read as **straight** alpha (checked with `DrawIconEx`: premultiplied bytes give a dark fringe) |
| X11 | RENDER: depth-32 pixmap, `PutImage` (premultiplied BGRA, cut into requests ≤ 200 KB), `RenderCreatePicture` with the server's ARGB32 format, `RenderCreateCursor`. No RENDER, version < 0.5, no ARGB32 format or a refused first cursor → a 1-bit cursor by the alpha threshold (128; dark pixel black, light white), like the grab/grabbing hands |
| Wayland | a `wl_shm` ARGB8888 buffer (one `memfd` per image, the pool destroyed at once) attached to the same cursor surface as the drawn shapes; `wl_surface.set_buffer_scale = round(window scale)` (compositor ≥ 4), the hotspot is divided by it. A fractional scale is rounded to the nearest integer (no `wp_viewport` for a cursor) |
| macOS | not supported (the window falls back to the override arrow) |

X11 builds the cursor in the event loop (it needs replies from the server): the call only
records a request and wakes the loop with a harmless `SendEvent` to the window itself
(`x11WakeRequest`); `SetCursor` called later withdraws a request not yet executed, so
the last call wins. A cursor wider than the server's limit is the server's refusal, not ours.

### Pointer lock — `Window.SetPointerLock` (E4)

```go
win.SetOnRelativeMotion(func(m widget.RelativeMotion) { /* m.DX, m.DY — device units */ })
ok := win.SetPointerLock(true)    // false: no window yet / no mechanism / refused
win.PointerLocked()               // is it in effect right now
win.SetOnPointerLockChanged(func(active bool) { /* focus lost / returned, compositor unlocked */ })
win.SetPointerLock(false)         // always true
```

The pointer is hidden and cannot leave the window; the application gets relative
motion in device units **without OS acceleration** (what RDP `TS_FP_RELPOINTER_EVENT`
needs). While the lock is in effect ordinary absolute moves (`OnMouseMove` of widgets) are
not delivered; button presses and the wheel come as usual.

**Why a callback and not a `RelativeMotionHandler` on the root.** The application that
locks the pointer already holds the `Window` (it calls `SetPointerLock` on it) and
forwards the motion over the network or into a game model; a root-widget interface would
force it to make its own root and wait for the engine to route the event through the tree.
The callback is one, can be replaced on the fly and is called on the engine goroutine, so
it may touch widgets. Consecutive motions are summed into one call (`relMotionQueue`), as
mouse moves are coalesced. `widget.RelativeMotion{DX, DY float64}` is the event type.

`true` from `SetPointerLock(true)` means the request is accepted: the lock is in effect or
will be as soon as the window is in focus (Wayland: as soon as the compositor grants the
lock — the pointer must be over the window). The lock is **released when the window loses
focus and restored when it returns**; `SetOnPointerLockChanged` reports both.

| | motion | lock |
|---|---|---|
| Win32 | `RegisterRawInputDevices` (usage page 1, usage 2, no `RIDEV_INPUTSINK`) + `WM_INPUT` → `GetRawInputData` → `RAWMOUSE.lLastX/lLastY` (`MOUSE_MOVE_RELATIVE`; absolute devices are turned into a delta) | `ClipCursor`, cursor hidden through `WM_SETCURSOR`. The clip is **one pixel in the centre of the client area**, not the whole area: windows here are borderless and the resize zones lie at the edges of the client area, so a pointer pinned at an edge would start dragging the window size. Raw input does not depend on the clip. Follows `WM_SIZE`/`WM_MOVE`; released on `WM_ACTIVATE` (inactive) and `WM_DESTROY`. The call is marshalled to the window thread |
| Wayland | `zwp_relative_pointer_v1.relative_motion`, the **unaccelerated** `dx_unaccel/dy_unaccel` | `zwp_pointer_constraints_v1.lock_pointer`, lifetime *persistent* (re-locked by the compositor when focus returns: `locked`/`unlocked` events); cursor hidden with `set_cursor(NULL)`. Missing either global → `false` |
| X11 | XInput2 `XI_RawMotion` selected on the **root** window for all master devices; raw (unaccelerated) values of axes 0 and 1 | `XGrabPointer` (`confine_to` the window, an invisible cursor). The pointer is warped back to the centre when it strays more than a quarter of the window from it. `FocusOut` ungrabs, `FocusIn` grabs again. **Without XInput2** the fallback is warp-to-centre: the pointer is warped to the centre after every motion and the delta is the offset from the centre (coarser: motion is lost until the warp lands, no raw units). Popup menus of another connection do not receive pointer events during the grab |
| macOS | not supported: `SetPointerLock(true)` returns `false` |

### Verified

- Pure functions and the wire: RAWINPUT parsing (64- and 32-bit headers, absolute flag,
  non-mouse); `relative_motion`; XI raw event (`parseXIRawMotion`: transformed and raw
  value sets, axis index by mask bits, truncated events); RENDER requests and replies;
  Wayland `lock_pointer`, `get_relative_pointer`, `create_pool`/`create_buffer`, `set_cursor`,
  `set_buffer_scale` messages; the LRU (order, eviction, "do not evict the shown one",
  freeing); coalescing of motions; the suppression of absolute moves under the lock.
- Win32 on a live window: cursor build with hotspot, alpha blend (`DrawIconEx`), cache and
  destruction of evicted cursors (`GetIconInfo` fails), popups; Raw Input registration and
  removal (`GetRegisteredRawInputDevices`), `ClipCursor` to the centre and back, focus loss and
  restore, a call from another thread; with `HEADLESS_GUI_POINTERLOCK_E2E=1` a real
  `SendInput` mouse move reaches the application as a `WM_INPUT` delta.
- X11: a toy X server over a socket pair with the real event loop (RENDER negotiation and
  fallbacks, cache, `FreeCursor`, lock handshake, focus loss/return, refusal, warp fallback);
  live against XWayland (WSLg): RENDER cursor created and verified, `XISelectEvents` + `XGrabPointer`
  accepted.
- Wayland: wire tests; live against the WSLg compositor: the compositor declares
  `zwp_pointer_constraints_v1` and `zwp_relative_pointer_manager_v1`, accepts the buffer,
  surface, `set_cursor` and `lock_pointer`/`destroy` requests without a protocol error.
  Not checked: a real `locked` event and real relative motion (no pointer over the test window).

## v3.40: keys without focus, button caption height, flat disabled buttons

Reported by the calc project (Windows 11 calculator) on v3.39.0.

- **Keys without focus.** A left click gives focus to the widget under the
  cursor if it is `Focusable`, otherwise clears focus; with no focus,
  `SendKeyEvent` used to drop the key. New optional interface
  `widget.UnfocusedKeyHandler{OnUnfocusedKey(ev KeyEvent) bool}` on the root
  (or the top modal): with no focused widget the key goes there whole — rune,
  scan code, release. Without the interface nothing changes. (`HandleInputBinding`
  on the root still runs first for presses.)
- **Button caption height.** `Button` centred its caption by a fixed 13 px
  regardless of `FontSize`, so a larger or smaller caption sat off-centre. The
  height now scales with the font size relative to the default; at the default
  size it is still 13, so existing frames do not change.
- **Disabled flat buttons.** A disabled button mixes its background 18 % toward
  white or black. A transparent background now stays transparent (it used to
  get a translucent plate), and `Button.FlatWhenDisabled` keeps the background
  as is and dims only the caption and border — for flat buttons whose
  background equals the window's.

### v3.40: always on top — `Window.SetAlwaysOnTop`

Asked for by the calculator ("Keep on top" of the Windows 11 calculator), later
Explorer and WinLine.

```go
ok := win.SetAlwaysOnTop(true) // false — the window system cannot do it
on := win.IsAlwaysOnTop()
```

- Win32: `SetWindowPos(HWND_TOPMOST / HWND_NOTOPMOST)` without activating;
  `IsAlwaysOnTop` reads `WS_EX_TOPMOST`.
- X11: `_NET_WM_STATE_ABOVE` through the `_NET_WM_STATE` ClientMessage (as
  full screen); `true` means the request was sent — the window manager decides,
  and `IsAlwaysOnTop` follows the state it writes to `_NET_WM_STATE`.
- Wayland: xdg-shell has no such request — `false`. macOS: not implemented —
  `false`.
- Before `Run` the request is remembered and applied when the window is created
  (`true` then means "remembered"; check `IsAlwaysOnTop` after start).

Tests: `window/alwaysontop_test.go` (before/after Run, unsupported backend),
`window/alwaysontop_windows_test.go` (live window, `WS_EX_TOPMOST` set and
cleared), `window/alwaysontop_x11_linux_test.go` (ClientMessage bytes, no atom).

## v3.41: Windows 10 window shadow and square corners where the OS does not round

Reported by the RDP client on v3.39.0: on Windows 10 a frameless window cast no
shadow and a light window merged with what is behind it.

- **Shadow.** On Windows 10 DWM does not round corners, and the engine used to
  cut a rounded region (`SetWindowRgn`) — DWM draws no shadow for a window with
  a region, and none for a window without a non-client area either. Now Windows
  10 keeps square corners (as every Windows 10 window has) and extends the DWM
  frame 1 px into the client area (`DwmExtendFrameIntoClientArea`), which turns
  the shadow on. Windows 11 is unchanged (DWM rounds, shadow kept).
- **Square body where the OS does not round.** Without the region, a
  `widget.Window` drawn with the theme's corner radius (8 on Windows 11 themes)
  left the canvas background visible in the corners. New
  `widget.Window.SquareCorners` (and `Dialog.SquareCorners` for dialogs moved
  into their own OS window): the window layer asks the backend
  (`RoundsCorners`) and sets it when the OS shows the window with square corners
  — Windows 10, X11, a window with radius 0. A maximized `widget.Window` is drawn
  square too: no OS rounds a maximized window. Headless rendering is unchanged.

## v3.42: what is in the clipboard, without reading it — `widget.ClipboardHasFiles` (RDP client E14)

`ClipboardFiles`, `ClipboardImage` and `ClipboardHTML` READ data from the clipboard
owner, and on X11/Wayland the owner is another process that may answer for
seconds or minutes (a remote desktop delivers the file list when the files are
downloaded; `ClipboardFiles` waits up to 10 minutes). An application that only
needs to know "are there files?" at every clipboard change cannot call it.

```go
widget.ClipboardHasFiles() bool                       // = ClipboardFormats().Has(ClipboardFormatFiles)
widget.ClipboardFormats() widget.ClipboardFormatSet   // Text | HTML | Image | Files flags
set.Has(widget.ClipboardFormatFiles | widget.ClipboardFormatText)  // ALL listed kinds are present

type ClipboardFormatsProvider interface {             // optional, like ClipboardFilesProvider
    ClipboardFormats() ClipboardFormatSet
}
```

Nothing is read and a foreign owner is never waited for beyond a short limit, so
the call is safe on every clipboard change (still call it outside locks: on X11
with a silent owner the first call takes up to 250 ms).

| | how it answers | cost |
|---|---|---|
| Windows | `IsClipboardFormatAvailable`: `CF_UNICODETEXT`, `HTML Format`, `PNG`/`CF_DIBV5`/`CF_DIB`, `CF_HDROP` or `FileGroupDescriptorW` (promised files) — the clipboard is not opened | instant, works while another process holds the clipboard |
| Wayland | the type list of the current `wl_data_offer` (already delivered by `offer` events) | instant |
| X11 | one `TARGETS` request with a 250 ms limit; the answer is remembered until the owner changes | first call ≤ 250 ms, then instant |
| own content (any) | from the engine's own copy | instant |
| memory provider | from its fields | instant |

X11 details. The owner change is reported by XFixes (`SelectSelectionInput`,
requested lazily on the first question and negotiated by the window's event loop,
because replies can only be read there). With XFixes the answer is valid until
the next change event; without it (no extension, version < 2, no event loop) it
lives 500 ms. A silent owner is remembered as "nothing" for 1 s so that polling
does not pay 250 ms each time. If another request to the owner is already in
flight (for example `ClipboardFiles` is waiting for a list) the call does not
queue behind it: it returns the last known answer, or an empty set.

Limits:
- the answer is about ANNOUNCED types, not data: on X11/Wayland `text/uri-list`
  counts as files although it could hold web links (reading filters those out);
  on Windows `FileGroupDescriptorW` counts as files even when `ClipboardFiles`
  cannot read promised files (it returns `CF_HDROP` lists only);
- an owner that answers `TARGETS` with a refusal is reported as an empty set;
- a custom `ClipboardProvider` without `ClipboardFormatsProvider` gives an empty
  set — `ClipboardFiles` is deliberately not used as a fallback, it may block;
- macOS: the provider (`pbcopy`/`pbpaste`) does not implement the extension, the
  set is empty.

Tests: pure functions (type names to kinds, `TARGETS` atoms to kinds, XFixes
requests and events) run everywhere; the X11 owner, the cache, the silent and the
busy owner and the XFixes negotiation run against a protocol-level fake server;
Windows tests use the real clipboard (`HEADLESS_GUI_TEST_SYSCLIPBOARD=1` or CI).

---

## v3.42: text quality layer (vertical hinting, contrast, LCD)

Reported by the RDP client: small text (Open Sans 12-14 px) looks softer than
Windows text. A vertical stem often lands on two pixels, the top of "o" is split
over two rows, thin dark strokes come out lighter. The engine now has an opt-in
quality layer on top of the `x/image/vector` rasterizer: no CGO, no system API,
and the masks are byte-identical on windows/amd64 and linux/amd64.

```go
eng.SetTextQuality(engine.TextQuality{
    Hinting:  true,                        // light autohinting, vertical only
    Contrast: engine.DefaultTextContrast,  // 0..1, boosts thin strokes (0.5)
    // LCD: true, LCDOrder: engine.LCDOrderRGB, // only on explicit request
})
eng.SetTextHinting(true)    // changes only the Hinting field
eng.SetTextContrast(0.5)    // changes only the Contrast field
q := eng.TextQuality()      // effective values (Contrast clamped to [0,1], step 1/64)
```

**Off by default**: the zero `TextQuality` draws text exactly as before (frames of
existing scenes, golden and hash tests unchanged). The mode is independent of
`SetTextSubpixel` (they can be combined). Changing it drops the glyph-mask
caches and redraws; it applies to every font of the engine, including fonts
registered later and lazily loaded fallbacks, and survives `SetScale` /
`SetResolution`.

**Measurement does not depend on the mode.** Advances, kerning, `MeasureText`
and `MeasureRunes` come from the same face as before; only the glyph mask
changes. No layout needs to be rebuilt after switching the mode.

- **Hinting.** For every font and size the engine finds vertical zones from
  reference glyphs: baseline, x-height (`x`, `o`; Cyrillic `х`, `о`), cap height
  (`H`, `O`; `Н`, `О`), ascender top (`l`), descender bottom (`p`). The outline of a
  glyph is stretched vertically, piecewise linearly, so that these heights land
  on whole PHYSICAL pixels (HiDPI scale is already in the size). Round-letter
  overshoot below half a pixel is merged into its zone, so `o`, `a`, `e`, `x`
  share the same top and bottom pixel rows. Flat horizontal strokes (the bar of
  `e` and `H`, shelves of `E`, `Z`) are snapped too: thickness to a whole
  number of pixels (at least 1), position to the grid or to the zone they sit on.
  Horizontal coordinates are not touched. The pen baseline is an integer pixel
  as it always was. It works on `DrawText`, `DrawTextFont`, the shaper path
  (RTL, combining marks), rotated text and the fallback chain. `<text>` inside
  SVG is vector art scaled arbitrarily — it is not hinted.
- **Contrast.** Glyph coverage goes through a table `a' = a + k*a*(1-a)`, where
  `k` depends on the brightness of the text colour: full boost for dark text
  (thin strokes do not get lighter), zero at brightness 75 %, and a slight
  thinning for light text on a dark background (a separate curve). The table is
  integer-only and independent of the background; it is applied per row at blit
  time, the cached glyph mask is not modified. Rotated text applies it once, to
  the real colour.
- **LCD** (`TextQuality.LCD`, experimental, off by default). The glyph is
  rasterized at 3x horizontal resolution, filtered with the FreeType default
  filter (`0x08 0x4D 0x56 0x4D 0x08`) and the three channels are blended with the
  background separately; `LCDOrder` selects RGB or BGR subpixels. It paints
  coloured pixels only over an opaque background (alpha 255); on transparent
  and translucent pixels, and for shaped and rotated text, the glyph is drawn
  with plain grey coverage. Useless in a remote session (RDP) and compresses
  worse; turn it on only when the application asked for it and knows the monitor.

Measured on the client's strings ("oops-hp.local", "Подключение", "Tandem",
"oops-hp · WinLine"), Open Sans Regular, text 20 and 99 on background 243, the
client's formula (full pixel: fill >= 0.6): at 12 px full pixels grow from 25.4 %
to 39.0 % and the mean fill from 0.422 to 0.517; at 14 px from 25.1 % to 37.3 %
and from 0.431 to 0.516 (Windows GDI+ AntiAliasGridFit: 31 % and 0.46).

Cost: a glyph mask is built once and cached; a cache miss costs about 1.5 us
with hinting (0.8 us before), 2 us with LCD; drawing a line with the contrast
table is about 8 % slower, LCD about 2x (scalar blend, no SIMD).

Tests: `engine/textquality_test.go` (client acceptance numbers, rows of lowercase
letters across sizes and across a line, measurement independence, off-mode
byte equality, mask hashes pinned for windows/linux),
`engine/textquality_paths_test.go` (contrast tables, engine API and inheritance,
named fonts, shaper path, rotated text, HiDPI, LCD order and BGRX buffer, clip),
`engine/textquality_bench_test.go` (glyph rasterization and line drawing).

---

## v3.42: deferred ("virtual") files on the clipboard (E13)

The RDP client must put server-side files on the clipboard **without
downloading them**: the names, sizes and times are announced when the user
copies, the bytes are fetched only when someone pastes. `ClipboardSetFiles`
takes ready paths only.

The engine holds the **mechanism only**: it tells the system "these files
exist" and calls an application callback at paste time. It keeps no policy: no
temporary folder, no downloading, no clean-up of temporary files. Where the
bytes come from, where they are put and when they are removed is the
application's business.

```go
type VirtualFile struct {
    Name    string    // relative path with "/" ("Report/total.txt")
    Size    int64
    ModTime time.Time // zero = unknown
    IsDir   bool
}

err := widget.ClipboardSetVirtualFiles(files, open,            // open: Windows
    widget.WithMaterialize(func(i int) (string, error) {...}), // materialize: X11/Wayland
    widget.WithRelease(func() {...}))                          // the promise is gone

files, ok := widget.ClipboardVirtualFiles()    // what we promised, while it is on the clipboard
rc, err := widget.ClipboardOpenVirtualFile(i)  // a paste inside the program (calls open)
widget.ClipboardCleanup()                      // withdraw the promise (call at exit)
files, err := widget.NormalizeVirtualFiles(l)  // the validation the call does
top, err := widget.VirtualTopLevel(files)      // indices of entries without "/" (what Linux asks paths for)

type ClipboardVirtualFilesProvider interface { // optional, like ClipboardFilesProvider
    SetVirtualFiles(files []VirtualFile, open func(int) (io.ReadCloser, error), opts VirtualFilesOptions) error
    GetVirtualFiles() ([]VirtualFile, bool)
    OpenVirtualFile(index int) (io.ReadCloser, error)
}
```

| | Windows | X11 / Wayland |
|---|---|---|
| announced | `IDataObject` in `OleSetClipboard`: `FileGroupDescriptorW`, `FileContents`, `Preferred DropEffect` | targets / mime types `text/uri-list`, `x-special/gnome-copied-files` (+ relative names as text) |
| app callback | `open(i)` — at the first read of the file's `IStream` | `WithMaterialize(i)` — when `text/uri-list` or the gnome type is requested |
| needs | `open` (else `ErrClipboardVirtualNoSource`) | `WithMaterialize` (else `ErrClipboardVirtualNoSource`; the clipboard is not touched) |
| macOS, a provider without the extension | `ErrClipboardVirtualUnsupported` (it matches `errors.ErrUnsupported`) | |

**Names** are validated (`ErrVirtualFileName`): relative, non-empty components,
no `.`/`..`, no backslash, NUL or control characters, valid UTF-8, no
duplicates, no file used as a directory; a trailing `/` is dropped. Windows
additionally rejects `<>:"|?*` and paths over 259 UTF-16 units. A directory
needs `IsDir` entry when it is empty; on X11/Wayland **every top-level
directory must be an explicit `IsDir` entry** (the application is asked for a
path per top-level entry).

**Windows.** The descriptor block is `FILEGROUPDESCRIPTORW` (592 bytes per
entry; flags `FD_ATTRIBUTES | FD_WRITESTIME | FD_FILESIZE | FD_PROGRESSUI`,
attributes `NORMAL`/`DIRECTORY`, `FILETIME` write time, 64-bit size, name with
`\`). `FileContents` for `lindex` = file index is an own `IStream` that reads
from the `io.ReadCloser` of `open(i)`: opened at the first `Read`, closed at the
last `Release`; `Seek` backwards reopens, forwards skips; a `Read` block is
filled completely unless the stream ends; `Stat`, `CopyTo`, `Clone` (not
supported), `HGLOBAL` for files up to 16 MiB. `OleFlushClipboard` is not
called — otherwise the system would ask for the contents at once. COM objects
are built by hand (vtables of `syscall.NewCallback`, the registry by address),
as in `window/dnd_windows.go`. OLE needs an STA thread with a message loop for
the clipboard object, which the application cannot guarantee, so the clipboard
has its own: a goroutine locked to an OS thread (`OleInitialize`, `GetMessage`).
Consequence: while a chunk of a file is being read (`open`'s reader blocks), a
replacement of the clipboard from another thread waits for that chunk.

**X11 / Wayland.** There is no virtual-files protocol, and the paster needs real
paths. The selection owner answers a request for `text/uri-list` or
`x-special/gnome-copied-files` by calling `WithMaterialize(i)` for each top-level
entry (a file: put it on disk and return the absolute path; a directory:
return the directory with its contents; nested entries are not asked
separately), then returns the paths. Calls are sequential, off the engine
goroutine (Wayland: in the writer goroutine; X11: the `SelectionNotify` is sent
from a goroutine), the result is remembered until the promise is withdrawn
(the second format does not ask again), an error is not remembered and means
a refusal (empty answer / property `None`). Limitations: some clipboard
managers request `text/uri-list` just to look at the clipboard — the callback
then runs without a paste; a paster may give up waiting (Qt: 5 s); the X11 list
of paths must fit in one 128 KiB request. A paste inside the program
(`ClipboardFiles`) returns the same paths.

`WithRelease` fires once, in its own goroutine, when the promise is gone: the
clipboard got other contents, another owner took the selection, the window was
closed, `ClipboardCleanup` was called (Windows: when OLE releases the data
object). That is where the application removes what it put on disk. Do not wait
for other clipboard calls inside it.

An example of the application side (an optional pattern, not engine code):

```go
dir, _ := os.MkdirTemp("", "myapp-clip-")
widget.ClipboardSetVirtualFiles(files, remoteOpen, // remoteOpen: for Windows
    widget.WithMaterialize(func(i int) (string, error) {
        p := filepath.Join(dir, filepath.FromSlash(files[i].Name))
        return p, download(files[i], p)   // a directory: create it with its children
    }),
    widget.WithRelease(func() { os.RemoveAll(dir) }))
```

Tests: `widget/clipboard_virtual_test.go` (names, `FILEGROUPDESCRIPTORW` byte by
byte, `FILETIME`, the in-memory provider),
`widget/clipboard_virtual_windows_test.go` (vtable layout, `QueryInterface`/
`AddRef`/`Release`, `GetData`/`QueryGetData`, the enumerator, `IStream` read in
parts / `Seek` / `Stat` / `CopyTo`; **live clipboard** — `OleGetClipboard` from
another thread as Explorer does, `open` is called only on the first read;
needs `HEADLESS_GUI_TEST_SYSCLIPBOARD=1` or CI), `window/clipboard_virtual_test.go`
(the shared promise: top-level only, caching, single flight, errors),
`window/clipboard_virtual_linux_test.go` (Wayland on a fake compositor, X11 on a
fake server: types announced, callback only on request, release on every way
out, window close).

## End of Reference

This document covers the essential API for AI code generation with headless-gui. For detailed implementation examples, refer to:
- `cmd/showcase/main.go` — example application
- `tests/` directory — unit tests with usage patterns
- GUIDE.md and GUIDE_EN.md — user documentation
