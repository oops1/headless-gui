# headless-gui

Go-based headless GUI engine with XAML support, tile-based delta rendering, and pluggable output backends (RDP, WebSocket, native platform windows).

## Overview

**headless-gui** renders a full widget UI off-screen into an RGBA buffer and streams only changed 64x64 tiles (delta compression). The engine knows nothing about displays or OS windows — you feed it mouse/keyboard events and consume rendered frames through a Go channel. This makes it suitable for remote desktop protocols, WebSocket-based thin clients, automated testing, and native windows alike.

## Features

- **Off-screen rendering** — no OS window required; output via `<-chan output.Frame`
- **Delta tile streaming** — only changed 64x64 regions are sent each frame
- **XAML layout** — load UI from WPF-compatible `.xaml` files (opens in Blend / Visual Studio)
- **Grid layout** — WPF-style `<Grid>` with Pixel / Star / Auto sizing, `Grid.Row`, `Grid.Column`, spans
- **Theming** — built-in Dark and Light themes, 80+ customizable color tokens
- **Drag & drop** — panels are draggable with recursive child movement
- **Modal dialogs** — centered overlay with background dim, input isolation
- **Font support** — TTF fonts via `golang.org/x/image/font`; custom registration by name
- **Cascading menus** — nested submenus with arrow indicators and keyboard navigation
- **Native window** — platform-native backends (Win32/Cocoa/X11), zero CGO on all platforms

## Widget List

| Widget | XAML Tag | Description |
|---|---|---|
| Panel | `Canvas`, `Border`, `StackPanel`, `DockPanel` | Container, drag, rounded corners, title bar, background image |
| Grid | `Grid` | WPF-style grid with RowDefinitions/ColumnDefinitions (Pixel/Star/Auto) |
| Label | `Label`, `TextBlock` | Static text, word wrap (`TextWrapping="Wrap"`) |
| Button | `Button`, `ToggleButton`, `RepeatButton` | Click handler, hover/press/accent states, custom colors |
| TextInput | `TextBox`, `TextInput` | Selection, clipboard, Home/End |
| PasswordBox | `PasswordBox` | Masked input |
| Dropdown | `ComboBox`, `Dropdown` | Overlay popup, keyboard nav |
| ProgressBar | `ProgressBar` | `Value` 0.0..1.0, custom fill color |
| CheckBox | `CheckBox` | Toggle with label |
| RadioButton | `RadioButton` | Mutual exclusion by `GroupName` |
| ToggleSwitch | `ToggleSwitch` | On/Off with animated knob |
| Slider | `Slider` | Min/Max/Value, drag thumb |
| TabControl | `TabControl` / `TabItem` | Multiple tabs with content widgets |
| ScrollView | `ScrollViewer` | Scrollbar, mouse wheel, `ContentHeight` |
| ListView | `ListView`, `ListBox` | Selection, keyboard nav, scrollbar |
| Image | `Image` | PNG/JPEG, stretch modes (Fill/Uniform/None) |
| PopupMenu | `PopupMenu`, `ContextMenu` | Context/popup menu, overlay, keyboard nav |
| MenuBar | `Menu`, `MenuBar`, `MainMenu` | Horizontal menu bar with dropdown submenus |
| Separator | `Separator`, `Line`, `Rectangle` | Divider line |
| MessageBox | — (code only) | OK / YesNo / YesNoCancel |
| Dialog | — (code only) | Modal base, custom content |
| Window | `Window` | Native OS window with title bar (Win/Mac style), resize, minimize/maximize |
| TreeView | `TreeView` | Expandable tree with arrow indicators |
| GridSplitter | `GridSplitter` | Resizable splitter between Grid cells |
| StatusBar | `StatusBar` | Bottom status bar with text |
| DataGrid | `DataGrid` | Column headers with data rows (maps to ListView) |

## Quick Start

### Headless (no window)

```bash
go run main.go
# Renders demo UI, writes PNG frames to out_test/
```

### Native Window

```bash
go run ./cmd/showcase    # Full widget showcase
go run ./cmd/smartgit    # SmartGit-like UI demo
```

Windows binary without console:

```bash
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```

## Project Structure

```
headless-gui/
  engine/          Core: canvas, render loop, event dispatch, font manager
  widget/          All widgets, themes, XAML loader, Grid layout, drag support
  output/          Frame + DirtyTile types for delta streaming
  window/          Native window (Win32/Cocoa/X11, zero CGO)
  cmd/
    showcase/      Full widget showcase (all widgets + live animation)
    smartgit/      SmartGit-like UI (Window + Menu + TreeView + DataGrid)
  assets/ui/       XAML demo layouts (demo.xaml, grid_demo.xaml, showcase.xaml)
  gui/             XAML files for RDP UI (login, block, error dialogs)
  tests/           Unit tests (engine, widgets, drag, modals)
  main.go          Headless demo entry point
```

## Minimal Example

```go
package main

import (
    "image"
    "image/color"
    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
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

    for frame := range eng.Frames() {
        _ = frame // frame.Tiles contains only changed 64x64 regions
    }
}
```

## XAML Support

UI can be defined in WPF-compatible XAML and loaded at runtime:

```xml
<Canvas Name="root" Width="800" Height="600" Background="#1E1E2E">

    <Grid Left="50" Top="50" Width="700" Height="500" ShowGridLines="True">
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
        <TextBox Grid.Row="1" Grid.Column="1" Placeholder="Type here..."/>
        <Button Grid.Row="2" Grid.Column="1" Content="OK" Style="Accent"/>
    </Grid>

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

## Dependencies

| Module | Dependency |
|---|---|
| `github.com/oops1/headless-gui/v3` | `golang.org/x/image` |
| `github.com/oops1/headless-gui/v3/window` | `golang.org/x/sys/windows`, `github.com/ebitengine/purego` |

Go 1.22+. The `window/` module is optional — the core engine has zero CGO dependencies. The window module is also CGO-free on all platforms.

## Documentation

Full developer guide with widget API, XAML reference, Grid layout, theming, event system, font registration, and architecture details:

- [GUIDE.md](GUIDE.md) — Русский
- [GUIDE_EN.md](GUIDE_EN.md) — English

## License

[MIT](LICENSE)
