# headless-gui — Developer Guide

## Overview

`headless-gui` is an off-screen GUI engine written in Go. It renders widgets into an RGBA buffer and outputs only changed 64×64 px tiles (delta compression). It does not depend on any window system — output is pluggable (RDP, WebSocket, native window).

```
headless-gui/          ← main module
  engine/              ← render loop, canvas, events
  widget/              ← widgets, themes, XAML loader
  output/              ← Frame / DirtyTile types
  cmd/                 ← utilities

headless-gui/window/   ← separate module: native window (Ebiten v2)
```

---

## Quick Start

```go
import (
    "image"
    "image/color"
    "headless-gui/engine"
    "headless-gui/widget"
)

eng := engine.New(1920, 1080, 30)   // width, height, FPS

// Build widget tree
root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
root.SetBounds(image.Rect(0, 0, 1920, 1080))

btn := widget.NewWin10AccentButton("Login")
btn.SetBounds(image.Rect(860, 500, 1060, 540))
btn.OnClick = func() { fmt.Println("Clicked!") }
root.AddChild(btn)

eng.SetRoot(root)
eng.Start()
defer eng.Stop()

// Consume frames
for frame := range eng.Frames() {
    for _, tile := range frame.Tiles {
        // tile.X, tile.Y, tile.W, tile.H, tile.Data (RGBA bytes)
        sendToClient(tile)
    }
}
```

---

## Engine (engine.Engine)

```go
eng := engine.New(width, height, fps)

eng.SetRoot(w widget.Widget)          // root widget
eng.SetTheme(t *widget.Theme)         // apply theme to entire tree
eng.SetBackgroundFile(path string)    // background image (PNG/JPEG)
eng.SetResolution(width, height int)  // change resolution on the fly
eng.SaveFrames(dir string)            // debug: save PNG frames to disk

eng.RegisterFont(name string, ttf []byte)  // named font
eng.RegisterFontFile(name, path string)    // font from TTF file
eng.SetDPI(dpi float64)                    // rendering DPI (default 96)

eng.Start()                           // start render loop
eng.Stop()                            // stop (closes Frames() channel)
eng.Frames() <-chan output.Frame      // channel of rendered frames
eng.CanvasSize() (w, h int)

// Input
eng.SetFocus(w widget.Widget)
eng.SendKeyEvent(e widget.KeyEvent)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
eng.SendMouseMove(x, y int)
```

`output.Frame` contains `Seq uint64`, `Timestamp time.Time`, and `[]DirtyTile{X,Y,W,H int; Data []byte}`.

---

## Widgets

Every widget embeds `widget.Base`, which implements `SetBounds`, `AddChild`, `Children`.

```go
w.SetBounds(image.Rect(x, y, x+w, y+h))  // required before first frame
parent.AddChild(child)
```

### Panel

Container with background, border, rounded corners, and built-in window title bar.

```go
p := widget.NewPanel(color.RGBA{R: 45, G: 45, B: 65, A: 255})
p.ShowBorder    = true
p.BorderColor   = color.RGBA{...}
p.CornerRadius  = 8    // rounded corners
p.UseAlpha      = true // alpha-blend background

widget.NewWin10Panel()  // standard semi-transparent dark panel
```

#### Title Bar

Panels can display a built-in title bar with window control buttons.

```go
p := widget.NewWin10Panel()
p.Caption    = "My Application v1.0"   // title text
p.ShowHeader = true                    // show header (default true)
p.MacStyle   = false                   // false=Windows (default), true=macOS

// Header height (default 32px)
p.HeaderHeight = 38

// Custom colors (defaults taken from theme if not set)
p.HeaderBG     = color.RGBA{R: 29, G: 29, B: 32, A: 240}
p.CaptionColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}

// Close button callback
p.OnClose = func() { eng.CloseModal(modal) }

// Content area below header
contentRect := p.ContentBounds()
```

**Windows style** (`MacStyle=false`): dark bar, left-aligned text, decorative ─ □ × buttons on the right. The × button is active when `OnClose` is set.

**macOS style** (`MacStyle=true`): bar with traffic lights (red/yellow/green) on the left, centered text. The red button is active when `OnClose` is set.

The header is drawn only when `ShowHeader=true` **and** `Caption` is not empty.

### Button

```go
btn := widget.NewButton("Text")           // standard
btn := widget.NewWin10AccentButton("OK")  // blue accent, primary action

btn.OnClick = func() { ... }
btn.SetPressed(true/false)   // programmatic
btn.IsHovered() bool         // hover state
```

### TextInput

```go
inp := widget.NewTextInput("placeholder...")

inp.SetText("value")
inp.GetText() string

inp.OnEnter  = func() { ... }
inp.OnChange = func(text string) { ... }

// Keyboard: Backspace, Delete, ←/→, Home, End
//           Shift+←/→/Home/End  — selection
//           Ctrl+A/C/X/V        — clipboard
// Mouse: click to position cursor, horizontal scroll on overflow
```

### Dropdown

```go
dd := widget.NewDropdown("Item 1", "Item 2", "Item 3")

dd.SetSelected(idx int)
dd.Selected() int
dd.SelectedText() string
dd.OnChange = func(idx int, text string) { ... }
```

### Label

```go
lbl := widget.NewWin10Label("Text")  // Win10 Dark style
lbl := widget.NewLabel("Text", color.RGBA{...})

lbl.SetText("new text")  // thread-safe
lbl.Text() string
```

### ProgressBar

```go
pb := widget.NewProgressBar()
pb.SetValue(0.75)   // [0.0, 1.0], thread-safe
pb.Value() float64
```

### Image

```go
img := widget.NewImageWidget()
img.SetSource("assets/logo.png")  // PNG or JPEG
img.SetImage(myImage)             // image.Image directly
img.Stretch = widget.ImageStretchFill     // stretch to fill (default)
             widget.ImageStretchUniform   // fit preserving aspect ratio
             widget.ImageStretchNone      // original size
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
// Widgets with the same GroupName are automatically linked
rb1 := widget.NewRadioButton("Option A", "myGroup")
rb2 := widget.NewRadioButton("Option B", "myGroup")
rb3 := widget.NewRadioButton("Option C", "myGroup")

rb1.SetSelected(true) // rb2, rb3 are automatically deselected
rb1.IsSelected() bool

rb1.OnChange = func(selected bool) { ... }

// Remove from group (on destruction)
rb1.RemoveFromGroup()
```

### TabControl

```go
tc := widget.NewTabControl(
    widget.TabItem{Header: "General",   Content: generalPanel},
    widget.TabItem{Header: "Settings",  Content: settingsPanel},
    widget.TabItem{Header: "About",     Content: aboutPanel},
)

tc.AddTab("More", anotherPanel)
tc.SetActive(0)
tc.Active() int
tc.TabCount() int

tc.OnTabChange = func(index int, header string) { ... }
```

### Slider

```go
s := widget.NewSlider()            // [0.0, 1.0]
s := widget.NewSliderRange(0, 100) // custom range

s.SetValue(0.5)
s.Value() float64

s.OnChange = func(value float64) { ... }

// Keyboard: ←/→ — 5% step, Shift+←/→ — 1% step, Home/End — min/max
```

### ToggleSwitch

```go
ts := widget.NewToggleSwitch("Dark Theme")

ts.SetOn(true)
ts.IsOn() bool

ts.OnChange = func(on bool) { ... }
```

### ScrollView

```go
sv := widget.NewScrollView()
sv.ContentHeight = 2000 // total content height

sv.AddChild(longPanel)
sv.SetBounds(image.Rect(100, 100, 500, 400))

sv.ScrollY() int
sv.SetScrollY(100)
sv.ScrollBy(50) // scroll down 50 pixels
```

### ListView

```go
lv := widget.NewListView("Item 1", "Item 2", "Item 3")

lv.SetItems([]string{"A", "B", "C"})
lv.AddItem("D")
lv.Items() []string

lv.SetSelected(0)
lv.Selected() int        // -1 if no selection
lv.SelectedText() string

lv.OnSelect = func(index int, text string) { ... }

// Keyboard: ↑/↓, Home/End, Enter
// Mouse: click to select, scrollbar with drag
```

---

## Input

### Mouse

```go
// Call from your client event processing thread
eng.SendMouseMove(x, y int)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
// btn: widget.MouseLeft | widget.MouseRight | widget.MouseMiddle
```

The engine performs hit-testing and dispatches the event to the appropriate widget. On left click, focus automatically transfers to the `Focusable` widget under the cursor.

### Keyboard

```go
eng.SendKeyEvent(widget.KeyEvent{
    Code:    widget.KeyLeft,    // physical key
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
// Built-in themes
eng.SetTheme(widget.DarkTheme())   // Windows 10 Dark (default)
eng.SetTheme(widget.LightTheme())  // Windows 10 Light

// Custom theme
t := widget.DarkTheme()
t.Accent = color.RGBA{R: 200, G: 50, B: 50, A: 255}  // red accent
t.InputFocus = t.Accent
eng.SetTheme(t)
```

`SetTheme` applies colors to all existing widgets via `ApplyTheme(t)` and updates global defaults for newly created widgets.

---

## XAML

The engine reads standard WPF XAML. Files are compatible with Blend / Visual Studio.

```go
root, named, err := widget.LoadUIFromXAMLFile("gui/window.xaml")
if err != nil { log.Fatal(err) }
eng.SetRoot(root)

// Find widget by x:Name
loginBtn := named["btnLogin"].(*widget.Button)
loginBtn.OnClick = func() { ... }
```

### Coordinates

**Child element coordinates are relative** (standard WPF Canvas behavior).
The loader adds the parent's absolute position to children's coordinates:

```
root Canvas (0,0)
  └─ Border mainWin (Canvas.Left=100, Canvas.Top=50)  → absolute: (100, 50)
       └─ Canvas
            └─ TextBlock (Canvas.Left=10, Canvas.Top=5) → absolute: (110, 55)
```

For **flat layouts** (all children on the root Canvas) coordinates match
absolute positions, since the root is at (0,0).

### XAML Example

```xml
<Canvas xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        x:Name="root" Width="1920" Height="1080" Background="#1E1E2E">

  <!-- Panel: Tag="Win10" → Win10Panel style.
       Children are nested — coordinates relative to panel. -->
  <Border x:Name="mainWin" Canvas.Left="660" Canvas.Top="240"
          Width="600" Height="500" Background="#2D2D30"
          BorderBrush="#555569" BorderThickness="1"
          CornerRadius="8" Tag="Win10">
    <Canvas>

      <!-- TextInput: Tag → placeholder -->
      <TextBox x:Name="loginInput" Canvas.Left="20" Canvas.Top="142"
               Width="560" Height="36"
               Tag="user@domain.com"/>

      <!-- Button: Tag="Accent" → accent (blue) button -->
      <Button x:Name="btnOK" Canvas.Left="20" Canvas.Top="210"
              Width="160" Height="40" Content="  Login  "
              Tag="Accent"/>

      <!-- Image -->
      <Image Canvas.Left="40" Canvas.Top="20"
             Width="200" Height="80" Source="assets/logo.png"/>

    </Canvas>
  </Border>

</Canvas>
```

### Engine-Specific Attributes

Some WPF attributes are used by the engine for widget mapping.
Blend ignores them, but the XAML remains parseable.

| WPF Element | Widget | Special Attributes |
|---|---|---|
| `Canvas`, `Border`, `Grid`, `StackPanel` | `Panel` | `Tag="Win10"` → Win10Panel, `Caption`, `ShowHeader`, `MacStyle` |
| `Button` | `Button` | `Tag="Accent"` → AccentButton |
| `TextBox` | `TextInput` | `Tag="placeholder text"` |
| `PasswordBox` | `TextInput` (password) | `Tag="hint"` |
| `ComboBox` | `Dropdown` | `<ComboBoxItem Content="..."/>` |
| `TextBlock`, `Label` | `Label` | |
| `ProgressBar` | `ProgressBar` | |
| `Image` | `ImageWidget` | |
| `CheckBox` | `CheckBox` | `IsChecked="True"` |
| `RadioButton` | `RadioButton` | `GroupName="grp"` |
| `TabControl` | `TabControl` | `SelectedIndex="0"` |
| `Slider` | `Slider` | `Minimum`, `Maximum`, `Value` |
| `ToggleSwitch` | `ToggleSwitch` | `IsOn="True"` |
| `ScrollViewer` | `ScrollView` | `ContentHeight="2000"` |
| `ListView` | `ListView` | `ItemHeight`, `SelectedIndex` |
| `Separator` | `Panel` (thin line) | |

### Additional Widget Examples

```xml
<!-- CheckBox -->
<CheckBox x:Name="cbRemember" Canvas.Left="10" Canvas.Top="10"
          Width="200" Height="24"
          Content="Remember me" IsChecked="True"/>

<!-- RadioButton with group -->
<RadioButton x:Name="rbAdmin" Canvas.Left="10" Canvas.Top="40"
             Width="200" Height="24"
             Content="Administrator" GroupName="role" IsChecked="True"/>
<RadioButton x:Name="rbUser" Canvas.Left="10" Canvas.Top="70"
             Width="200" Height="24"
             Content="User" GroupName="role"/>

<!-- Slider -->
<Slider x:Name="volume" Canvas.Left="10" Canvas.Top="100"
        Width="300" Height="30"
        Minimum="0" Maximum="100" Value="50"/>

<!-- ToggleSwitch (engine extension) -->
<ToggleSwitch x:Name="darkMode" Canvas.Left="10" Canvas.Top="140"
              Width="200" Height="28"
              Content="Dark Theme" IsOn="True"/>

<!-- TabControl -->
<TabControl x:Name="tabs" Canvas.Left="0" Canvas.Top="0"
            Width="600" Height="400">
    <TabItem Header="General">
        <Canvas Width="600" Height="368" Background="Transparent">
            <TextBlock Canvas.Left="10" Canvas.Top="10"
                       Width="200" Height="20" Text="Content"/>
        </Canvas>
    </TabItem>
    <TabItem Header="Settings">
        <Canvas Width="600" Height="368" Background="Transparent"/>
    </TabItem>
</TabControl>

<!-- ListView -->
<ListView x:Name="userList" Canvas.Left="10" Canvas.Top="200"
          Width="400" Height="200">
    <ListViewItem Content="User 1"/>
    <ListViewItem Content="User 2"/>
    <ListViewItem Content="User 3"/>
</ListView>
```

---

## Native Window (headless-gui/window)

Separate module based on Ebiten v2. On Windows — DirectX 11, no CGO required.

```go
// go.mod of your application:
// require headless-gui/window v0.0.0
// replace headless-gui/window => ../GuiEngine/window

import "headless-gui/window"

eng := engine.New(1280, 720, 30)
// ... build UI, eng.Start() ...

win := window.New(eng, "Window Title")
win.SetMaxFPS(60)
win.SetResizable(true)

// Blocks until window closes. Call from main().
if err := win.Run(); err != nil {
    log.Fatal(err)
}
```

---

## Custom Widget

```go
type MyWidget struct {
    widget.Base                      // required
    Color color.RGBA
    Value int
}

func (w *MyWidget) Draw(ctx widget.DrawContext) {
    b := w.Bounds()
    ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, w.Color)
    ctx.DrawText(fmt.Sprintf("%d", w.Value), b.Min.X+8, b.Min.Y+8,
        color.RGBA{R: 255, G: 255, B: 255, A: 255})
    w.Base.DrawChildren(ctx)          // draw children
}

// Optional interfaces:
func (w *MyWidget) OnMouseButton(e widget.MouseEvent) bool { ... }   // MouseClickHandler
func (w *MyWidget) OnMouseMove(x, y int)                   { ... }   // MouseMoveHandler (hover)
func (w *MyWidget) OnKeyEvent(e widget.KeyEvent)           { ... }   // KeyHandler
func (w *MyWidget) SetFocused(v bool)                      { ... }   // Focusable
func (w *MyWidget) IsFocused() bool                        { ... }
func (w *MyWidget) ApplyTheme(t *widget.Theme)             { ... }   // Themeable
```

### DrawContext API

```go
// Rectangles
ctx.FillRect(x, y, w, h int, col color.RGBA)
ctx.FillRectAlpha(x, y, w, h int, col color.RGBA)   // alpha blending
ctx.FillRoundRect(x, y, w, h, r int, col color.RGBA)
ctx.DrawBorder(x, y, w, h int, col color.RGBA)
ctx.DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

// Lines and pixels
ctx.DrawHLine(x, y, length int, col color.RGBA)
ctx.DrawVLine(x, y, length int, col color.RGBA)
ctx.SetPixel(x, y int, col color.RGBA)

// Images
ctx.DrawImage(src image.Image, x, y int)
ctx.DrawImageScaled(src image.Image, x, y, w, h int)

// Text
ctx.DrawText(text string, x, y int, col color.RGBA)           // 10pt, default font
ctx.DrawTextSize(text string, x, y int, pt float64, col)      // custom size
ctx.DrawTextFont(text string, x, y int, pt float64, name string, col) // named font
ctx.MeasureText(text string, pt float64) int
ctx.MeasureRunePositions(text string, pt float64) []int       // character positions

// Clipping
ctx.SetClip(r image.Rectangle)   // restrict drawing area
ctx.ClearClip()
```

---

## Module Structure

```
go.mod:  module headless-gui
  require golang.org/x/image

go.mod:  module headless-gui/window
  require headless-gui => ../
  require github.com/hajimehoshi/ebiten/v2
```

Consumer application (`rdp-ui`) imports the main module:
```
replace headless-gui => ../GuiEngine
```
If native window is needed, additionally:
```
replace headless-gui/window => ../GuiEngine/window
```
