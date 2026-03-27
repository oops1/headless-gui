# headless-gui — руководство разработчика

## Обзор

`headless-gui` — off-screen GUI-движок на Go. Рендерит виджеты в RGBA-буфер и выдаёт только изменившиеся тайлы 64×64 px. Не зависит от оконной системы — вывод подключается отдельно (RDP, WebSocket, нативное окно).

```
headless-gui/          ← основной модуль
  engine/              ← рендер-цикл, canvas, события
  widget/              ← виджеты, темы, XAML-загрузчик
  output/              ← типы Frame / DirtyTile
  cmd/                 ← утилиты

headless-gui/window/   ← отдельный модуль: нативное окно (Ebiten v2)
```

---

## Быстрый старт

```go
import (
    "image"
    "image/color"
    "headless-gui/engine"
    "headless-gui/widget"
)

eng := engine.New(1920, 1080, 30)   // ширина, высота, FPS

// Строим дерево виджетов
root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
root.SetBounds(image.Rect(0, 0, 1920, 1080))

btn := widget.NewWin10AccentButton("Войти")
btn.SetBounds(image.Rect(860, 500, 1060, 540))
btn.OnClick = func() { fmt.Println("Клик!") }
root.AddChild(btn)

eng.SetRoot(root)
eng.Start()
defer eng.Stop()

// Потребляем кадры
for frame := range eng.Frames() {
    for _, tile := range frame.Tiles {
        // tile.X, tile.Y, tile.W, tile.H, tile.Data (RGBA байты)
        sendToClient(tile)
    }
}
```

---

## Движок (engine.Engine)

```go
eng := engine.New(width, height, fps)

eng.SetRoot(w widget.Widget)          // корневой виджет
eng.SetTheme(t *widget.Theme)         // применить тему к дереву
eng.SetBackgroundFile(path string)    // фоновое изображение (PNG/JPEG)
eng.SetResolution(width, height int)  // изменить разрешение на лету
eng.SaveFrames(dir string)            // дебаг: сохранять PNG-кадры

eng.RegisterFont(name string, ttf []byte)  // именованный шрифт
eng.RegisterFontFile(name, path string)    // шрифт из TTF-файла
eng.SetDPI(dpi float64)                    // DPI рендеринга (по умолчанию 96)

eng.Start()                           // запустить рендер-цикл
eng.Stop()                            // остановить (закрывает Frames())
eng.Frames() <-chan output.Frame      // канал готовых кадров
eng.CanvasSize() (w, h int)

// Ввод
eng.SetFocus(w widget.Widget)
eng.SendKeyEvent(e widget.KeyEvent)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
eng.SendMouseMove(x, y int)
```

`output.Frame` содержит `Seq uint64`, `Timestamp time.Time` и `[]DirtyTile{X,Y,W,H int; Data []byte}`.

---

## Виджеты

Каждый виджет встраивает `widget.Base`, которая реализует `SetBounds`, `AddChild`, `Children`.

```go
w.SetBounds(image.Rect(x, y, x+w, y+h))  // обязательно перед первым кадром
parent.AddChild(child)
```

### Panel

Контейнер с фоном, рамкой, скруглёнными углами и встроенным заголовком окна.

```go
p := widget.NewPanel(color.RGBA{R: 45, G: 45, B: 65, A: 255})
p.ShowBorder    = true
p.BorderColor   = color.RGBA{...}
p.CornerRadius  = 8    // скруглённые углы
p.UseAlpha      = true // альфа-смешивание фона

widget.NewWin10Panel()  // стандартная полупрозрачная тёмная панель
```

#### Заголовок окна (title bar)

Панель может отображать встроенный заголовок с кнопками управления окном.

```go
p := widget.NewWin10Panel()
p.Caption    = "Моё приложение v1.0"   // текст заголовка
p.ShowHeader = true                     // показывать (по умолчанию true)
p.MacStyle   = false                    // false=Windows (по умолчанию), true=macOS

// Высота заголовка (по умолчанию 32px)
p.HeaderHeight = 38

// Кастомные цвета (если не задать — берутся из темы)
p.HeaderBG     = color.RGBA{R: 29, G: 29, B: 32, A: 240}
p.CaptionColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}

// Область под заголовком
contentRect := p.ContentBounds()
```

**Windows-стиль** (`MacStyle=false`): тёмная полоса, текст слева, декоративные кнопки ─ □ × справа.

**macOS-стиль** (`MacStyle=true`): полоса с traffic lights (красный/жёлтый/зелёный) слева, текст по центру.

Заголовок рисуется только если `ShowHeader=true` **и** `Caption` не пуст.

### Button

```go
btn := widget.NewButton("Текст")          // стандартная
btn := widget.NewWin10AccentButton("OK")  // синяя, основное действие

btn.OnClick = func() { ... }
btn.SetPressed(true/false)   // программно
btn.IsHovered() bool         // hover-состояние
```

### TextInput

```go
inp := widget.NewTextInput("placeholder...")

inp.SetText("значение")
inp.GetText() string

inp.OnEnter  = func() { ... }
inp.OnChange = func(text string) { ... }

// Клавиатура: Backspace, Delete, ←/→, Home, End
//             Shift+←/→/Home/End  — выделение
//             Ctrl+A/C/X/V        — работа с буфером
// Мышь: клик позиционирует курсор, горизонтальный скролл при переполнении
```

### Dropdown

```go
dd := widget.NewDropdown("Пункт 1", "Пункт 2", "Пункт 3")

dd.SetSelected(idx int)
dd.Selected() int
dd.SelectedText() string
dd.OnChange = func(idx int, text string) { ... }
```

### Label

```go
lbl := widget.NewWin10Label("Текст")  // стиль Win10 Dark
lbl := widget.NewLabel("Текст", color.RGBA{...})

lbl.SetText("новый текст")  // потокобезопасно
lbl.Text() string
```

### ProgressBar

```go
pb := widget.NewProgressBar()
pb.SetValue(0.75)   // [0.0, 1.0], потокобезопасно
pb.Value() float64
```

### Image

```go
img := widget.NewImageWidget()
img.SetSource("assets/logo.png")  // PNG или JPEG
img.SetImage(myImage)             // image.Image напрямую
img.Stretch = widget.ImageStretchFill     // растянуть (по умолчанию)
             widget.ImageStretchUniform   // вписать с пропорциями
             widget.ImageStretchNone      // оригинальный размер
```

---

## Ввод

### Мышь

```go
// Вызывать из потока обработки событий клиента
eng.SendMouseMove(x, y int)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
// btn: widget.MouseLeft | widget.MouseRight | widget.MouseMiddle
```

Движок сам делает hit-test и передаёт событие нужному виджету. При нажатии ЛКМ фокус автоматически переходит на `Focusable`-виджет под курсором.

### Клавиатура

```go
eng.SendKeyEvent(widget.KeyEvent{
    Code:    widget.KeyLeft,    // физическая клавиша
    Rune:    'А',               // для символьного ввода (Code = KeyUnknown)
    Mod:     widget.ModCtrl | widget.ModShift,
    Pressed: true,
})
```

Коды клавиш: `KeyBackspace, KeyEnter, KeyEscape, KeyTab, KeySpace, KeyLeft/Right/Up/Down, KeyHome, KeyEnd, KeyDelete, KeyA/C/V/X/Z`.

Модификаторы: `ModShift, ModCtrl, ModAlt, ModMeta`.

---

## Темы

```go
// Встроенные темы
eng.SetTheme(widget.DarkTheme())   // Windows 10 Dark (по умолчанию)
eng.SetTheme(widget.LightTheme())  // Windows 10 Light

// Кастомная тема
t := widget.DarkTheme()
t.Accent = color.RGBA{R: 200, G: 50, B: 50, A: 255}  // красный акцент
t.InputFocus = t.Accent
eng.SetTheme(t)
```

`SetTheme` применяет цвета ко всем существующим виджетам через `ApplyTheme(t)` и обновляет глобальные дефолты для новых.

---

## XAML

Движок читает стандартный WPF XAML. Файлы совместимы с Blend/Visual Studio.

```go
root, named, err := widget.LoadUIFromXAMLFile("gui/window.xaml")
if err != nil { log.Fatal(err) }
eng.SetRoot(root)

// Найти виджет по x:Name
loginBtn := named["btnLogin"].(*widget.Button)
loginBtn.OnClick = func() { ... }
```

### Координаты

**Координаты дочерних элементов — относительные** (стандарт WPF Canvas).
Загрузчик прибавляет абсолютную позицию родителя к координатам потомков:

```
root Canvas (0,0)
  └─ Border mainWin (Canvas.Left=100, Canvas.Top=50)  → абсолютно: (100, 50)
       └─ Canvas
            └─ TextBlock (Canvas.Left=10, Canvas.Top=5) → абсолютно: (110, 55)
```

Для **плоских макетов** (все потомки на корневом Canvas) координаты совпадают
с абсолютными, т.к. корень в (0,0).

### Пример XAML

```xml
<Canvas xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        x:Name="root" Width="1920" Height="1080" Background="#1E1E2E">

  <!-- Панель: Tag="Win10" → стиль Win10Panel.
       Дочерние элементы вложены — координаты относительно панели. -->
  <Border x:Name="mainWin" Canvas.Left="660" Canvas.Top="240"
          Width="600" Height="500" Background="#2D2D30"
          BorderBrush="#555569" BorderThickness="1"
          CornerRadius="8" Tag="Win10">
    <Canvas>

      <!-- TextInput: Tag → placeholder -->
      <TextBox x:Name="loginInput" Canvas.Left="20" Canvas.Top="142"
               Width="560" Height="36"
               Tag="user@domain.com"/>

      <!-- Button: Tag="Accent" → синяя кнопка -->
      <Button x:Name="btnOK" Canvas.Left="20" Canvas.Top="210"
              Width="160" Height="40" Content="  Войти  "
              Tag="Accent"/>

      <!-- Image -->
      <Image Canvas.Left="40" Canvas.Top="20"
             Width="200" Height="80" Source="assets/logo.png"/>

    </Canvas>
  </Border>

</Canvas>
```

### Особые атрибуты движка

Некоторые WPF-атрибуты используются движком для маппинга на виджеты.
Blend их игнорирует, но XAML остаётся парсируемым.

| WPF элемент | Виджет | Особые атрибуты |
|---|---|---|
| `Canvas`, `Border`, `Grid`, `StackPanel` | `Panel` | `Tag="Win10"` → Win10Panel, `Caption`, `ShowHeader`, `MacStyle` |
| `Button` | `Button` | `Tag="Accent"` → AccentButton |
| `TextBox` | `TextInput` | `Tag="placeholder text"` |
| `PasswordBox` | `TextInput` (password) | `Tag="подсказка"` |
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
| `Separator` | `Panel` (тонкая линия) | |

### Пример: дополнительные виджеты

```xml
<!-- CheckBox -->
<CheckBox x:Name="cbRemember" Canvas.Left="10" Canvas.Top="10"
          Width="200" Height="24"
          Content="Запомнить меня" IsChecked="True"/>

<!-- RadioButton с группой -->
<RadioButton x:Name="rbAdmin" Canvas.Left="10" Canvas.Top="40"
             Width="200" Height="24"
             Content="Администратор" GroupName="role" IsChecked="True"/>
<RadioButton x:Name="rbUser" Canvas.Left="10" Canvas.Top="70"
             Width="200" Height="24"
             Content="Пользователь" GroupName="role"/>

<!-- Slider -->
<Slider x:Name="volume" Canvas.Left="10" Canvas.Top="100"
        Width="300" Height="30"
        Minimum="0" Maximum="100" Value="50"/>

<!-- ToggleSwitch (расширение движка) -->
<ToggleSwitch x:Name="darkMode" Canvas.Left="10" Canvas.Top="140"
              Width="200" Height="28"
              Content="Тёмная тема" IsOn="True"/>

<!-- TabControl -->
<TabControl x:Name="tabs" Canvas.Left="0" Canvas.Top="0"
            Width="600" Height="400">
    <TabItem Header="Общие">
        <Canvas Width="600" Height="368" Background="Transparent">
            <TextBlock Canvas.Left="10" Canvas.Top="10"
                       Width="200" Height="20" Text="Содержимое"/>
        </Canvas>
    </TabItem>
    <TabItem Header="Настройки">
        <Canvas Width="600" Height="368" Background="Transparent"/>
    </TabItem>
</TabControl>

<!-- ListView -->
<ListView x:Name="userList" Canvas.Left="10" Canvas.Top="200"
          Width="400" Height="200">
    <ListViewItem Content="Пользователь 1"/>
    <ListViewItem Content="Пользователь 2"/>
    <ListViewItem Content="Пользователь 3"/>
</ListView>
```

---

## Нативное окно (headless-gui/window)

Отдельный модуль на базе Ebiten v2. На Windows — DirectX 11 без CGO.

```go
// go.mod вашего приложения:
// require headless-gui/window v0.0.0
// replace headless-gui/window => ../GuiEngine/window

import "headless-gui/window"

eng := engine.New(1280, 720, 30)
// ... строим UI, eng.Start() ...

win := window.New(eng, "Заголовок окна")
win.SetMaxFPS(60)
win.SetResizable(true)

// Блокирует до закрытия окна. Вызывать из main().
if err := win.Run(); err != nil {
    log.Fatal(err)
}
```

### CheckBox

```go
cb := widget.NewCheckBox("Запомнить меня")

cb.SetChecked(true)
cb.IsChecked() bool

cb.OnChange = func(checked bool) { ... }
```

### RadioButton

```go
// Виджеты с одинаковым GroupName автоматически связаны
rb1 := widget.NewRadioButton("Вариант A", "myGroup")
rb2 := widget.NewRadioButton("Вариант B", "myGroup")
rb3 := widget.NewRadioButton("Вариант C", "myGroup")

rb1.SetSelected(true) // rb2, rb3 автоматически сбрасываются
rb1.IsSelected() bool

rb1.OnChange = func(selected bool) { ... }

// Удаление из группы (при деструкции)
rb1.RemoveFromGroup()
```

### TabControl

```go
tc := widget.NewTabControl(
    widget.TabItem{Header: "Общие",    Content: generalPanel},
    widget.TabItem{Header: "Настройки", Content: settingsPanel},
    widget.TabItem{Header: "О программе", Content: aboutPanel},
)

tc.AddTab("Ещё", anotherPanel)
tc.SetActive(0)
tc.Active() int
tc.TabCount() int

tc.OnTabChange = func(index int, header string) { ... }
```

### Slider

```go
s := widget.NewSlider()          // [0.0, 1.0]
s := widget.NewSliderRange(0, 100) // произвольный диапазон

s.SetValue(0.5)
s.Value() float64

s.OnChange = func(value float64) { ... }

// Клавиатура: ←/→ — шаг 5%, Shift+←/→ — шаг 1%, Home/End — мин/макс
```

### ToggleSwitch

```go
ts := widget.NewToggleSwitch("Тёмная тема")

ts.SetOn(true)
ts.IsOn() bool

ts.OnChange = func(on bool) { ... }
```

### ScrollView

```go
sv := widget.NewScrollView()
sv.ContentHeight = 2000 // полная высота содержимого

sv.AddChild(longPanel)
sv.SetBounds(image.Rect(100, 100, 500, 400))

sv.ScrollY() int
sv.SetScrollY(100)
sv.ScrollBy(50) // прокрутка на 50 пикселей вниз
```

### ListView

```go
lv := widget.NewListView("Элемент 1", "Элемент 2", "Элемент 3")

lv.SetItems([]string{"A", "B", "C"})
lv.AddItem("D")
lv.Items() []string

lv.SetSelected(0)
lv.Selected() int        // -1 если нет выделения
lv.SelectedText() string

lv.OnSelect = func(index int, text string) { ... }

// Клавиатура: ↑/↓, Home/End, Enter
// Мышь: клик по элементу, скроллбар с drag
```

---

## Свой виджет

```go
type MyWidget struct {
    widget.Base                      // обязательно
    Color color.RGBA
    Value int
}

func (w *MyWidget) Draw(ctx widget.DrawContext) {
    b := w.Bounds()
    ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, w.Color)
    ctx.DrawText(fmt.Sprintf("%d", w.Value), b.Min.X+8, b.Min.Y+8,
        color.RGBA{R: 255, G: 255, B: 255, A: 255})
    w.Base.DrawChildren(ctx)          // рисуем дочерние
}

// Опционально — интерфейсы:
func (w *MyWidget) OnMouseButton(e widget.MouseEvent) bool { ... }   // MouseClickHandler
func (w *MyWidget) OnMouseMove(x, y int)                   { ... }   // MouseMoveHandler (hover)
func (w *MyWidget) OnKeyEvent(e widget.KeyEvent)           { ... }   // KeyHandler
func (w *MyWidget) SetFocused(v bool)                      { ... }   // Focusable
func (w *MyWidget) IsFocused() bool                        { ... }
func (w *MyWidget) ApplyTheme(t *widget.Theme)             { ... }   // Themeable
```

### DrawContext API

```go
// Прямоугольники
ctx.FillRect(x, y, w, h int, col color.RGBA)
ctx.FillRectAlpha(x, y, w, h int, col color.RGBA)   // альфа-смешивание
ctx.FillRoundRect(x, y, w, h, r int, col color.RGBA)
ctx.DrawBorder(x, y, w, h int, col color.RGBA)
ctx.DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

// Линии и пиксели
ctx.DrawHLine(x, y, length int, col color.RGBA)
ctx.DrawVLine(x, y, length int, col color.RGBA)
ctx.SetPixel(x, y int, col color.RGBA)

// Изображения
ctx.DrawImage(src image.Image, x, y int)
ctx.DrawImageScaled(src image.Image, x, y, w, h int)

// Текст
ctx.DrawText(text string, x, y int, col color.RGBA)           // 10pt, шрифт default
ctx.DrawTextSize(text string, x, y int, pt float64, col)      // произвольный размер
ctx.DrawTextFont(text string, x, y int, pt float64, name string, col) // именованный шрифт
ctx.MeasureText(text string, pt float64) int
ctx.MeasureRunePositions(text string, pt float64) []int       // позиции символов

// Clip
ctx.SetClip(r image.Rectangle)   // ограничить область рисования
ctx.ClearClip()
```

---

## Структура модулей

```
go.mod:  module headless-gui
  require golang.org/x/image

go.mod:  module headless-gui/window
  require headless-gui => ../
  require github.com/hajimehoshi/ebiten/v2
```

Приложение-потребитель (`rdp-ui`) подключает основной модуль:
```
replace headless-gui => ../GuiEngine
```
Если нужно нативное окно — дополнительно:
```
replace headless-gui/window => ../GuiEngine/window
```
