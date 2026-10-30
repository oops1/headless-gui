# headless-gui/window

Нативное OS-окно для движка headless-gui на платформенных API (Win32, X11,
Wayland, Cocoa). Без CGO на всех платформах.

## Платформы

| ОС | Бэкенд | Рендер | CGO |
|---|---|---|---|
| **Windows** | Win32 (`platform/win32`) | StretchDIBits из кэша кадра | не нужен |
| **Linux** | Wayland (`platform/wayland`), если есть компоновщик; иначе X11 (`platform/x11`) | wl_shm / PutImage и MIT-SHM | не нужен |
| **macOS** | Cocoa (`platform/cocoa`, через purego) | NSWindow | не нужен |

`HEADLESS_GUI_X11=1` выбирает на Linux X11 и при работающем компоновщике.

## Быстрый старт

```go
package main

import (
    "image"
    "image/color"
    "log"

    "github.com/oops1/headless-gui/v4/engine"
    "github.com/oops1/headless-gui/v4/widget"
    "github.com/oops1/headless-gui/v4/window"
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

## Два фасада: `window` и `window/minimal`

`window` подключает все возможности окна: доступность (UI Automation, AT-SPI),
трей и уведомления, перетаскивание файлов, службы оболочки (открытие ссылок,
системная тема и акцент, языки системы). `window/minimal` — тот же фасад без
них: тип `Window` у обоих один (окна взаимозаменяемы), а методы возможностей
честно отвечают «не поддерживается» — так же, как на оконной системе, которая
их не умеет. Переключить программу — сменить путь импорта:

```go
import window "github.com/oops1/headless-gui/v4/window/minimal"
```

Возможность можно подключить и поштучно — пустым импортом её пакета
`platform/*`. Минимальность «заражается»: если любой пакет программы
импортирует `window`, возможности подключены для всех окон процесса, —
библиотекам поэтому лучше импортировать `window/minimal`, а выбор оставить
программе. Выигрыш (замер шага 9.8e): бинарь меньше на 4,2 % на Windows,
на 3,8 % на Linux и на 0,2 % на macOS; на Windows `window/minimal` не грузит
`ole32.dll` и `UIAutomationCore.dll`.

## Устройство

```
window, window/minimal   фасады (internal/facadegen) над window/core и platform/native
window/core              Window: поверхность, ввод, попапы, модальные окна, док,
                         обвязка мониторов, полного экрана, активации; выбор бэкенда
platform/native          контракты бэкендов и возможностей, общие типы, реестр
platform/{win32,x11,wayland,cocoa}   бэкенды окна
platform/{a11y,shell,tray,dnd}       возможности (подключаются import _)
```

Решение и его уточнения — `docs/adr/0013-window-platform.md`. Контракт
бэкендов проверяет общий набор сценариев `platform/native/contracttest`:
Win32 — на живых окнах, X11 и Wayland — на настоящих серверах (при
`DISPLAY`/`WAYLAND_DISPLAY`), Cocoa — сборка.

## Матрица возможностей бэкендов

«да» — бэкенд умеет; «просьба» — оконный менеджер или компоновщик вправе
отказать, ответ приходит асинхронно; «—» — нет, метод отвечает честным
отказом (`false`, `ok=false`, `ErrTrayUnsupported`, no-op). Строки с
пометкой возможности в `window/minimal` отвечают отказом на всех ОС.

| Возможность | Win32 | X11 | Wayland | macOS |
|---|---|---|---|---|
| Размер, минимальный размер, развернуть и восстановить | да | да (просьба) | да (просьба) | базово |
| Полный экран | да | да (просьба) | да (просьба) | да |
| Полный экран на выбранном мониторе | да | да | да | — |
| Мониторы и их смена | да | да (RandR) | да (`wl_output`) | — |
| Начальное положение и «развёрнуто» до показа | да | да | да | — |
| Вывести вперёд (`Activate`) | да | да (просьба) | да (`xdg-activation`, нужен токен) | — |
| Поверх всех | да | да | — | — |
| Стиль: без рамки ОС (`Borderless`) | да | да | да | — |
| Стиль: инструмент, без панели задач (`Tool`) | да | да | — | — |
| Владелец, Z-порядок, положение после показа | да | да (просьба) | — | — |
| Попапы и меню в своих окнах | да | да | да (`xdg_popup`) | — |
| Модальные диалоги в своих окнах | да | да | — (в холсте) | — (в холсте) |
| Snap Layouts, системный ресайз за край | да | `BeginMove`/`BeginResize` | `BeginMove`/`BeginResize` | — |
| Касания | да | да (XInput2) | да | — |
| IME | да | — | да (text-input-v3) | — |
| Захват клавиатуры | да | да | да (shortcuts-inhibit) | — |
| Lock-клавиши: читать | да | да | да | — |
| Lock-клавиши: переключать | да | да (XKB) | только с `zwp_virtual_keyboard_manager_v1` | — |
| Раскладка ОС (индикатор локали) | да | да | — | — |
| Курсоры-картинки | да | да | да | — |
| Перетаскивание файлов в окно и из окна — `platform/dnd` | да | да (XDND) | да | — |
| Значок перетаскивания — `platform/dnd` | да | — | да | — |
| Трей и уведомления — `platform/tray` | да (Shell_NotifyIcon) | да (StatusNotifierItem) | да (StatusNotifierItem) | — |
| Превью в панели задач | да | — | — | — |
| Доступность — `platform/a11y` | да (UI Automation) | да (AT-SPI) | да (AT-SPI) | — |
| Открытие ссылок и файлов — `platform/shell` | да | да (портал, xdg-open) | да (портал, xdg-open) | да (`open`) |
| Системная тема и акцент — `platform/shell` | да | да (портал) | да (портал) | — |
| Языки системы — `platform/shell` | да | да (переменные среды) | да (переменные среды) | да (переменные среды) |

## Демо-приложения

Из корня репозитория:

```bash
go run ./cmd/showcase        # все виджеты + живая анимация
go run ./cmd/smartgit        # SmartGit-подобный UI
go run ./cmd/guiview         # интерактивное демо с модальными окнами
go run ./cmd/griddemo        # Grid-раскладка

# Бинарник без консольного окна (Windows)
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```

## Использование в своём проекте

`window` — пакет модуля `github.com/oops1/headless-gui/v4`, отдельного модуля
у него нет:

```
require github.com/oops1/headless-gui/v4 v4.x.y
```

## API

```go
win := window.New(eng, "Заголовок")   // создать окно
win.SetMaxFPS(60)                     // ограничить FPS (по умолчанию 60)
win.SetResizable(true)                // разрешить изменение размера

err := win.Run()                      // открыть (блокирует до закрытия)
snap := win.FullFrameSnapshot()       // *image.RGBA — снимок текущего кадра
```

Подробно — `GUIDE.md` (разделы о нативном окне) и
`docs/AI_AGENT_REFERENCE.md`.

## EngineAPI

`window.New` принимает `EngineAPI` — интерфейс, а не конкретный
`*engine.Engine`. Это позволяет подключить любой совместимый источник кадров:

```go
type EngineAPI interface {
    Frames() <-chan output.Frame
    CanvasSize() (w, h int)
    Root() widget.Widget
    SendMouseMove(x, y int)
    SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
    SendKeyEvent(e widget.KeyEvent)
    CursorAt(x, y int) widget.Cursor
}
```
