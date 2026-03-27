# headless-gui

Go-based headless GUI engine with XAML support, tile-based delta rendering, and pluggable output backends (RDP, WebSocket, Ebiten).

## Overview

**headless-gui** renders a full widget UI off-screen into an RGBA buffer and streams only changed 64x64 tiles (delta compression). The engine knows nothing about displays or OS windows — you feed it mouse/keyboard events and consume rendered frames through a Go channel. This makes it suitable for remote desktop protocols, WebSocket-based thin clients, automated testing, and native windows alike.

## Features

- **Off-screen rendering** — no OS window required; output via `<-chan output.Frame`
- **Delta tile streaming** — only changed 64x64 regions are sent each frame
- **25 widgets** — Panel, Button, TextInput, PasswordBox, Label, Dropdown, ProgressBar, CheckBox, RadioButton, TabControl, Slider, ToggleSwitch, ScrollView, ListView, ImageWidget, MessageBox, Dialog, and more
- **XAML layout** — load UI from WPF-compatible `.xaml` files (opens in Blend / Visual Studio)
- **Theming** — built-in Dark and Light themes, 60+ customizable color tokens
- **Drag & drop** — panels are draggable with recursive child movement
- **Modal dialogs** — centered overlay with background dim, input isolation
- **Font support** — TTF fonts via `golang.org/x/image/font`; custom registration by name
- **Native window** — optional Ebiten v2 backend (`window/` module) for desktop preview

## Quick Start

### Headless (no window)

```bash
go run main.go
# Renders demo UI, writes PNG frames to out_test/
```

### Native Window (Ebiten)

```bash
cd window
go run ../cmd/guiview/main.go
```

Windows binary without console:

```bash
cd window
go build -ldflags="-H windowsgui" -o guiview.exe ../cmd/guiview
```

## Project Structure

```
headless-gui/
  engine/       Core: canvas, render loop, event dispatch, font manager
  widget/       All widgets, themes, XAML loader, drag support
  output/       Frame + DirtyTile types for delta streaming
  window/       Ebiten v2 native window (separate go.mod)
  cmd/guiview/  Demo app with native window
  gui/          XAML demo files (login window, modal dialogs)
  assets/       Fonts, demo layouts
  tests/        Unit tests (engine, widgets, drag, modals)
  main.go       Headless demo entry point
```

## Minimal Example

```go
package main

import (
    "image/color"
    "headless-gui/engine"
    "headless-gui/widget"
)

func main() {
    eng := engine.New(800, 600, 30)

    root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
    root.SetBounds(image.Rect(0, 0, 800, 600))

    btn := widget.NewWin10AccentButton("Click me")
    btn.SetBounds(image.Rect(50, 50, 200, 90))
    btn.OnClick = func() { /* handle click */ }
    root.AddChild(btn)

    eng.SetRoot(root)
    eng.Start()
    defer eng.Stop()

    // Consume frames
    for frame := range eng.Frames() {
        // frame.Tiles contains only changed 64x64 regions
        _ = frame
    }
}
```

## XAML Support

UI can be defined in WPF-compatible XAML and loaded at runtime:

```xml
<Canvas xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        Width="400" Height="300" Background="#2D2D30"
        Caption="My Window" CornerRadius="8">

    <TextBlock Canvas.Left="20" Canvas.Top="42"
               Width="360" Height="20"
               Text="Hello, World!" Foreground="White"/>

    <Button x:Name="btnOK" Canvas.Left="150" Canvas.Top="240"
            Width="100" Height="36"
            Content="OK" Tag="Accent"/>
</Canvas>
```

```go
root, named, err := widget.LoadUIFromXAMLFile("gui/window.xaml")
if btn, ok := named["btnOK"].(*widget.Button); ok {
    btn.OnClick = func() { /* ... */ }
}
eng.SetRoot(root)
```

Coordinates inside containers are relative (standard WPF Canvas behavior).

## Widget List

| Widget | Constructor | Notes |
|---|---|---|
| Panel | `NewPanel(bg)` / `NewWin10Panel()` | Container, drag, rounded corners, title bar |
| Button | `NewButton(text)` / `NewWin10AccentButton(text)` | Click handler, hover/press states |
| TextInput | `NewTextInput(placeholder)` | Selection, clipboard, Home/End |
| PasswordBox | `NewPasswordInput(placeholder)` | Masked input |
| Label | `NewLabel(text, color)` / `NewWin10Label(text)` | Static text |
| Dropdown | `NewDropdown(items...)` | Overlay popup, keyboard nav |
| ProgressBar | `NewProgressBar()` | `SetValue(0.0 .. 1.0)` |
| CheckBox | `NewCheckBox(text)` | Toggle with label |
| RadioButton | `NewRadioButton(text, group)` | Mutual exclusion by group |
| TabControl | `NewTabControl()` | `AddTab(header, content)` |
| Slider | `NewSlider()` | Min/Max/Value, drag thumb |
| ToggleSwitch | `NewToggleSwitch(text)` | On/Off with animation |
| ScrollView | `NewScrollView()` | Scrollbar, mouse wheel |
| ListView | `NewListView(items...)` | Selection, keyboard nav |
| ImageWidget | `NewImageWidget()` | PNG/JPEG, stretch modes |
| MessageBox | `NewMessageBox(eng)` | OK / YesNo / YesNoCancel |
| Dialog | `NewDialog(title, w, h)` | Modal base, custom content |

## Themes

```go
eng.SetTheme(widget.DarkTheme())   // Windows 10 Dark (default)
eng.SetTheme(widget.LightTheme())  // Windows 10 Light
```

60+ color tokens: accent, window background, button states, input focus, scrollbar, etc.

## Output Format

Each frame contains only changed tiles:

```go
type DirtyTile struct {
    X, Y int    // Position on canvas
    W, H int    // Size (up to 64x64)
    Data []byte // Raw RGBA pixels
}

type Frame struct {
    Seq       uint64
    Timestamp time.Time
    Tiles     []DirtyTile
}
```

Consume via `eng.Frames()` channel (buffered, depth 8).

## Dependencies

| Module | Dependency |
|---|---|
| `headless-gui` | `golang.org/x/image` |
| `headless-gui/window` | `github.com/hajimehoshi/ebiten/v2` |

Go 1.22+. The `window/` module is optional — the core engine has zero CGO dependencies.

## Documentation

See [GUIDE.md](GUIDE.md) for the full developer guide (in Russian): widget API, XAML reference, theming, event system, font registration, and architecture details.

## License

[MIT](LICENSE)
