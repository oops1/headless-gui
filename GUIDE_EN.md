# headless-gui — Developer Guide

## Overview

`headless-gui` is an off-screen GUI engine written in Go. It renders widgets into an RGBA buffer and outputs only changed 64x64 px tiles (delta compression). It does not depend on any window system — output is pluggable (RDP, WebSocket, native window).

```
headless-gui/
  engine/              render loop, canvas, events, fonts
  widget/              widgets, themes, XAML loader, Grid layout
  output/              Frame / DirtyTile types
  window/              native Ebiten v2 window (separate go.mod)
  cmd/
    showcase/          full widget showcase (all widgets + live animation)
    guiview/           interactive demo with modal windows
    griddemo/          Grid layout demo
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
    "github.com/oops1/headless-gui/engine"
    "github.com/oops1/headless-gui/widget"
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
| `Separator`, `Line`, `Rectangle` | Separator | `Background` |

Common attributes: `Name`/`x:Name`, `Left`/`Canvas.Left`, `Top`/`Canvas.Top`, `Width`, `Height`, `Grid.Row`, `Grid.Column`, `Grid.RowSpan`, `Grid.ColumnSpan`.

---

## Native Window (window)

Separate module based on Ebiten v2. On Windows — DirectX 11, no CGO required.

```go
import "github.com/oops1/headless-gui/window"

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
go.mod:  module github.com/oops1/headless-gui
  require golang.org/x/image

go.mod:  module github.com/oops1/headless-gui/window
  require github.com/oops1/headless-gui => ../
  require github.com/hajimehoshi/ebiten/v2
```

Consumer application imports the main module:

```
require github.com/oops1/headless-gui v0.x.x
```

If native window is needed:

```
require github.com/oops1/headless-gui/window v0.x.x
```

For local development use `replace`:

```
replace github.com/oops1/headless-gui => ../GuiEngine
replace github.com/oops1/headless-gui/window => ../GuiEngine/window
```

---

## Demo Applications

Run from the `window/` directory (where the Ebiten go.mod is located):

```bash
cd GuiEngine/window

go run ../cmd/showcase    # all widgets + live animation
go run ../cmd/guiview     # interactive demo with modal XAML windows
go run ../cmd/griddemo    # Grid layout

# Windows binary without console
go build -ldflags="-H windowsgui" -o showcase.exe ../cmd/showcase
```
