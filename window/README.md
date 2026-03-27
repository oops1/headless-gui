# headless-gui/window

Нативное OS-окно для `headless-gui` движка на базе **[Ebiten v2](https://ebitengine.org/)**.

## Платформы

| ОС | Рендер | CGO |
|---|---|---|
| **Windows** | DirectX 11 | ❌ не нужен |
| Linux | OpenGL | ✅ нужен (libGL, X11 или Wayland) |
| macOS | Metal | ✅ нужен |

## Быстрый старт

```go
package main

import (
    "image"
    "image/color"
    "log"

    "headless-gui/engine"
    "headless-gui/widget"
    "headless-gui/window"
)

func main() {
    eng := engine.New(1280, 720, 30)

    // Строим UI
    root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
    root.SetBounds(image.Rect(0, 0, 1280, 720))

    btn := widget.NewWin10AccentButton("Привет!")
    btn.SetBounds(image.Rect(500, 300, 780, 340))
    btn.OnClick = func() { log.Println("Клик!") }
    root.AddChild(btn)

    eng.SetRoot(root)
    eng.Start()
    defer eng.Stop()

    // Открываем окно (блокирует до закрытия)
    win := window.New(eng, "Моё приложение")
    win.SetMaxFPS(60)
    if err := win.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Запуск демо

```bash
# Из директории GuiEngine/window (там лежит go.mod с ebiten)
go run ../cmd/guiview/main.go

# Бинарник без консольного окна (Windows)
go build -ldflags="-H windowsgui" -o guiview.exe ../cmd/guiview
```

## Использование с rdp-ui

Добавьте в `rdp/go.mod`:
```
require headless-gui/window v0.0.0

replace headless-gui/window => ../GuiEngine/window
```

## Структура модуля

```
GuiEngine/window/
  go.mod        — модуль headless-gui/window (ebiten v2.7.x)
  window.go     — Window, EngineAPI интерфейс, маппинг ввода
  README.md     — эта документация

GuiEngine/cmd/guiview/
  main.go       — демо-приложение (запускается из директории window/)
```

## API

```go
// Создать окно
win := window.New(eng, "Заголовок")

// Настройки (опционально)
win.SetMaxFPS(60)
win.SetResizable(true)

// Открыть (блокирует до закрытия)
err := win.Run()

// Снимок текущего кадра
snap := win.FullFrameSnapshot() // *image.RGBA
```

## EngineAPI интерфейс

`window.New` принимает `EngineAPI` — не конкретный `*engine.Engine`, а интерфейс.
Это позволяет подключить любой совместимый источник кадров:

```go
type EngineAPI interface {
    Frames() <-chan output.Frame
    CanvasSize() (w, h int)
    SendMouseMove(x, y int)
    SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
    SendKeyEvent(e widget.KeyEvent)
}
```
