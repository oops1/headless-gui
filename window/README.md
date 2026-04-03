# headless-gui/window

Нативное OS-окно для headless-gui движка на базе платформенных API (Win32, Cocoa, X11). Без CGO на всех платформах.

## Платформы

| ОС | Рендер | CGO |
|---|---|---|
| **Windows** | Win32 API (StretchDIBits) | не нужен |
| **macOS** | Cocoa (NSWindow + purego) | не нужен |
| **Linux** | X11 (PutImage, direct socket) | не нужен |

## Быстрый старт

```go
package main

import (
    "image"
    "image/color"
    "log"

    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
    "github.com/oops1/headless-gui/v3/window"
)

func main() {
    eng := engine.New(1280, 720, 30)

    root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
    root.SetBounds(image.Rect(0, 0, 1280, 720))

    btn := widget.NewWin10AccentButton("Привет!")
    btn.SetBounds(image.Rect(500, 300, 780, 340))
    btn.OnClick = func() { log.Println("Клик!") }
    root.AddChild(btn)

    eng.SetRoot(root)
    eng.Start()
    defer eng.Stop()

    win := window.New(eng, "Моё приложение")
    win.SetMaxFPS(60)
    if err := win.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Демо-приложения

Из корневой директории GuiEngine:

```bash
go run ./cmd/showcase        # все виджеты + живая анимация
go run ./cmd/smartgit        # SmartGit-подобный UI
go run ./cmd/guiview         # интерактивное демо с модальными окнами
go run ./cmd/griddemo        # Grid-раскладка

# Бинарник без консольного окна (Windows)
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```

## Использование в своём проекте

Добавьте в `go.mod`:

```
require github.com/oops1/headless-gui/v3/window v0.x.x
```

Модуль больше не зависит от Ebiten — только платформенные API (Win32, Cocoa, X11).

Для локальной разработки:

```
require github.com/oops1/headless-gui/v3/window v0.0.0
replace github.com/oops1/headless-gui/v3/window => ../GuiEngine/window
```

## Структура

```
GuiEngine/window/
  go.mod              модуль github.com/oops1/headless-gui/v3/window
  window.go           Window, NativeWindow интерфейс, маппинг ввода
  native_windows.go   Win32 API (WS_POPUP, StretchDIBits, WndProc)
  native_darwin.go    Cocoa через purego (NSWindow, NSApplication)
  native_linux.go     X11 протокол (direct socket, PutImage)
```

## API

```go
win := window.New(eng, "Заголовок")   // создать окно
win.SetMaxFPS(60)                     // ограничить FPS (по умолчанию 60)
win.SetResizable(true)                // разрешить изменение размера

err := win.Run()                      // открыть (блокирует до закрытия)
snap := win.FullFrameSnapshot()       // *image.RGBA — снимок текущего кадра
```

## EngineAPI

`window.New` принимает `EngineAPI` — интерфейс, а не конкретный `*engine.Engine`. Это позволяет подключить любой совместимый источник кадров:

```go
type EngineAPI interface {
    Frames() <-chan output.Frame
    CanvasSize() (w, h int)
    SendMouseMove(x, y int)
    SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
    SendKeyEvent(e widget.KeyEvent)
}
```
