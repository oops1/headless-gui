<a href="https://github.com/oops1/headless-gui">
     <img width="1280" height="640" alt="claude-skills" src="https://github.com/oops1/headless-gui/blob/main/social_preview.png" />
     
</a>


# headless-gui

**English** · [Русский](README_RU.md)

Pure-Go headless GUI engine (zero CGO): WPF-style XAML, data binding, complex text shaping, antialiasing, HiDPI. Renders off-screen to 64×64 delta tiles — stream the UI **to any web browser** (built-in WebSocket viewer), over RDP, or show native windows (Win32 / X11 / Wayland / macOS).

## Overview

**headless-gui** renders a full widget UI off-screen into an RGBA buffer and streams only changed 64x64 tiles (delta compression). The engine knows nothing about displays or OS windows — you feed it mouse/keyboard events and consume rendered frames through a Go channel. This makes it suitable for remote desktop protocols, WebSocket-based thin clients, automated testing, and native windows alike.

## Screenshots

Rendered headlessly by the engine itself (no OS window involved):

| Widget showcase | Text shaping & antialiased graphics |
|---|---|
| ![Widgets](docs/screenshots/showcase-widgets.png) | ![Text & graphics](docs/screenshots/showcase-text-graphics.png) |

## Performance

Software renderer, fully on CPU (Intel Core Ultra 7 265K, single engine):

| Scenario | Cost |
|---|---|
| Idle UI (render-on-demand, default) | ~0 CPU — frames are skipped entirely |
| Button hover (event + partial redraw + tile diff) | ~45 µs |
| Partial frame via `InvalidateRect` | ~38 µs |
| Full 1280×800 frame, ~180 text labels | ~2.2 ms |
| Text line, 40 glyphs (cached) | ~13 µs / shaped Arabic ~3 µs |
| Full-HD tile diff (no changes, parallel) | ~110 µs |

Run `go test ./engine/ -bench .` to reproduce.

## Features

- **Off-screen rendering** — no OS window required; output via `<-chan output.Frame`
- **Delta tile streaming** — only changed 64x64 regions are sent each frame
- **Browser viewer out of the box** — `output/webstream` streams the UI to any browser over WebSocket (zero-dep RFC 6455 server, per-tile PNG, keyframe for new clients, multiple concurrent viewers) and feeds mouse/keyboard back; one Go process on the server, no rebuild for the client (`go run ./cmd/webdemo`)
- **Standard dialogs, fully engine-drawn** — MessageBox with severity icons (Enter/Esc, Windows-style Ctrl+C dump), input and progress dialogs, file Open/Save/Folder with a built-in browser (places sidebar, clickable breadcrumb, columns) — they work headless and show the *server's* filesystem when streaming; themed and localized (EN/RU built in, live language switch)
- **Multiline TextBox editor** — word wrap or horizontal scroll, mouse/keyboard selection, Ctrl+arrows word jumps, PgUp/PgDn, clipboard, undo/redo, context menu; caret math works headless
- **Full keyboard layouts on Linux** — Wayland xkb keymap parsing (live layout switching) and X11 `GetKeyboardMapping`, so Russian/US/… layouts type correctly in native windows
- **Render-on-demand by default** — widgets self-invalidate; only the damaged region is redrawn and diffed (idle UI costs ~0 CPU, a hover ~45 µs)
- **Complex text shaping** — HarfBuzz-quality shaping in pure Go (go-text/typesetting): Arabic ligatures & joining, Hebrew RTL, Devanagari conjuncts, Thai marks, mixed-bidi strings; Latin/Cyrillic keep a fast per-rune glyph cache
- **Antialiasing** — smooth rounded corners (cached quarter-disc masks), AA ellipses/lines/polygons via vector rasterization
- **HiDPI** — widgets live in logical pixels (WPF DIP model), frames render at physical resolution; per-monitor DPI awareness (v2) + `WM_DPICHANGED` on Windows, `HEADLESS_GUI_SCALE` elsewhere
- **XAML layout** — load UI from WPF-compatible `.xaml` files (opens in Blend / Visual Studio)
- **Grid layout** — WPF-style `<Grid>` with Pixel / Star / Auto sizing, `Grid.Row`, `Grid.Column`, spans
- **Theming** — built-in Dark and Light themes, 80+ customizable color tokens
- **Drag & drop** — panels are draggable with recursive child movement
- **Modal dialogs** — centered overlay with background dim, input isolation
- **Font support** — TTF fonts via `golang.org/x/image/font`; custom registration by name
- **Cascading menus** — nested submenus with arrow indicators and keyboard navigation
- **Native window** — platform-native backends (Win32/Cocoa/X11/**Wayland**), zero CGO on all platforms; Wayland speaks the raw wire protocol (xdg-shell + wl_shm) over a unix socket and is auto-selected when a compositor is available (`HEADLESS_GUI_X11=1` forces X11); window chrome follows the theme, reacts to OS focus (inactive title bar), repaints from the frame cache on expose (X11/Win32; Wayland retains content)
- **Golden render tests** — pixel-exact snapshot tests of widgets/themes/AA/HiDPI guard against visual regressions (CI on Windows/Linux/macOS)
- **Accessibility semantic tree** — `eng.AccessibilityTree()` returns a JSON-serializable snapshot (roles, names, values, states) for screen-reader side-channels in streaming scenarios and for UI test automation; full keyboard navigation (Tab/Enter/Space) built in
- **Data binding** — `{Binding}` OneWay/TwoWay/OneTime, `INotifyPropertyChanged`, `StringFormat`, `IValueConverter`, `ElementName`/`RelativeSource`, live `ItemsControl`
- **Styles, triggers & templates** — `<Style>`/`<Setter>`, `DataTrigger`/`MultiTrigger`, `ControlTemplate` + `ContentPresenter` + `TemplateBinding`, `StaticResource`
- **Commands & input bindings** — `ICommand`/`RelayCommand`, `Button.Command`, `<KeyBinding>` hotkeys
- **Localization** — UI language **independent of keyboard layout**; `{Loc Key}` markup + string tables (JSON), live re-translation
- **Validation** — `IDataErrorInfo` / `ValidatesOnDataErrors=True` with red error adorner
- **CollectionView & UI virtualization** — sort/filter/group over a collection; `VirtualizingItemsControl` renders only visible rows (100k+ items)
- **Vector shapes** — `Ellipse`, `Rectangle`, `Line`, `Polygon`, `Polyline` with `Fill`/`Stroke`
- **Tooltips & cursors** — `ToolTip` on every widget; per-widget mouse cursors
- **Free bundled fonts** — Roboto (default), Open Sans, Inter + system glyph fallback chain (no tofu)

## Widget List

| Widget | XAML Tag | Description |
|---|---|---|
| Panel | `Canvas`, `Border`, `StackPanel`, `DockPanel` | Container, drag, rounded corners, title bar, background image |
| Grid | `Grid` | WPF-style grid with RowDefinitions/ColumnDefinitions (Pixel/Star/Auto) |
| Label | `Label`, `TextBlock` | Static text, word wrap (`TextWrapping="Wrap"`) |
| Button | `Button`, `ToggleButton`, `RepeatButton` | Click handler, hover/press/accent states, custom colors |
| TextInput | `TextBox`, `TextInput` | Single-line: selection, clipboard, Home/End, undo/redo, context menu |
| TextBox | `TextBox AcceptsReturn="True"` / `TextWrapping="Wrap"` | Multiline editor: word wrap, vertical scroll, Ctrl+arrows, PgUp/PgDn, clipboard, undo/redo |
| PasswordBox | `PasswordBox` | Masked input |
| Dropdown | `ComboBox`, `Dropdown` | Overlay popup, keyboard nav |
| ProgressBar | `ProgressBar` | `Value` 0.0..1.0, custom fill color |
| CheckBox | `CheckBox` | Toggle with label |
| RadioButton | `RadioButton` | Mutual exclusion by `GroupName` |
| ToggleSwitch | `ToggleSwitch` | On/Off with animated knob |
| Slider | `Slider` | Min/Max/Value, drag thumb |
| NumericUpDown | `NumericUpDown` / `IntegerUpDown` / `DoubleUpDown` | Spinner ▲/▼, wheel, typing, Min/Max/Increment/Decimals |
| TabControl | `TabControl` / `TabItem` | Multiple tabs with content widgets |
| ScrollView | `ScrollViewer` | Scrollbar, mouse wheel, `ContentHeight` |
| ListView | `ListView`, `ListBox` | Selection, keyboard nav, scrollbar (virtualized) |
| VirtualizingItemsControl | `VirtualizingItemsControl` | UI virtualization — materializes only visible rows; CollectionView-aware |
| Image | `Image` | PNG/JPEG, stretch modes (Fill/Uniform/None) |
| PopupMenu | `PopupMenu`, `ContextMenu` | Context/popup menu, overlay, keyboard nav |
| MenuBar | `Menu`, `MenuBar`, `MainMenu` | Horizontal menu bar with dropdown submenus |
| WrapPanel | `WrapPanel` | Flow layout, wraps children to next line |
| UniformGrid | `UniformGrid` | Equal-sized cells, `Rows`/`Columns` |
| GroupBox | `GroupBox` | Titled bordered container (content clipped to bounds) |
| Expander | `Expander` | Collapsible panel with header arrow |
| Shapes | `Ellipse`, `Rectangle`, `Line`, `Polygon`, `Polyline` | Vector shapes with `Fill`/`Stroke`/`StrokeThickness` |
| Separator | `Separator` | Divider line |
| MessageBox | — (code only) | Severity presets (Info/Question/Warning/Error), OK/YesNo/YesNoCancel, Enter/Esc, Ctrl+C dump |
| InputDialog / ProgressDialog | — (code only) | Prompt with validation & hint; progress with detail line, percent, indeterminate |
| FileDialog | — (code only) | Open / Save / Pick-folder with built-in browser (places, breadcrumb, columns, filters) |
| Dialog | — (code only) | Modal base: rounded chrome + shadow, ✕ close, custom content |
| Window | `Window` | Native OS window with title bar (Win/Mac style), resize, minimize/maximize |
| TreeView | `TreeView` | WPF-compatible hierarchical tree with virtualization, HierarchicalDataTemplate, icons, keyboard nav |
| GridSplitter | `GridSplitter` | Resizable splitter between Grid cells |
| StatusBar | `StatusBar` | Bottom status bar with text |
| DataGrid | `DataGrid` | WPF-compatible data table with columns, sorting, cell editing, resize, Data Binding, ObservableCollection |

## Quick Start

### Headless (no window)

```bash
go run main.go
# Renders demo UI, writes PNG frames to out_test/
```

### Browser (WebSocket streaming)

```bash
go run ./cmd/webdemo
# open http://localhost:8091 — the UI runs on the server, no native window at all
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
    treeview/      WPF-compatible TreeView (core logic, no widget dependency)
    datagrid/      DataGrid core logic (ObservableCollection, PropertyNotifier)
  output/          Frame + DirtyTile types for delta streaming
    webstream/     Browser viewer: WebSocket tile streaming + input (zero-dep)
  window/          Native window (Win32/Cocoa/X11/Wayland, zero CGO)
  cmd/
    showcase/      Full widget showcase (all widgets + live animation)
    webdemo/       Browser streaming demo (http://localhost:8091)
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
