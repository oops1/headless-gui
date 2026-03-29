# headless-gui — руководство разработчика

## Обзор

`headless-gui` — off-screen GUI-движок на Go. Рендерит виджеты в RGBA-буфер и выдаёт только изменившиеся тайлы 64x64 px. Не зависит от оконной системы — вывод подключается отдельно (RDP, WebSocket, нативное окно).

```
headless-gui/
  engine/              рендер-цикл, canvas, события, шрифты
  widget/              виджеты, темы, XAML-загрузчик, Grid layout
  output/              типы Frame / DirtyTile
  window/              нативное окно Ebiten v2 (отдельный go.mod)
  cmd/
    showcase/          полная демонстрация всех виджетов
    guiview/           интерактивное демо с модальными окнами
    griddemo/          демо Grid-раскладки
  assets/ui/           XAML-макеты (demo.xaml, grid_demo.xaml, showcase.xaml)
  gui/                 XAML для RDP UI (логин, блокировка, ошибки)
  tests/               юнит-тесты
```

---

## Быстрый старт

```go
import (
    "image"
    "image/color"
    "github.com/oops1/headless-gui/v3/engine"
    "github.com/oops1/headless-gui/v3/widget"
)

eng := engine.New(1920, 1080, 30)   // ширина, высота, FPS

root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 46, A: 255})
root.SetBounds(image.Rect(0, 0, 1920, 1080))

btn := widget.NewWin10AccentButton("Войти")
btn.SetBounds(image.Rect(860, 500, 1060, 540))
btn.OnClick = func() { fmt.Println("Клик!") }
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

## Движок (engine.Engine)

```go
eng := engine.New(width, height, fps)

// Корень и оформление
eng.SetRoot(w widget.Widget)
eng.SetTheme(t *widget.Theme)
eng.SetBackgroundFile(path string)    // PNG/JPEG
eng.SetResolution(width, height int)  // изменить на лету

// Шрифты
eng.RegisterFont(name string, ttf []byte)
eng.RegisterFontFile(name, path string)
eng.SetDPI(dpi float64)              // по умолчанию 96

// Жизненный цикл
eng.Start()
eng.Stop()                            // закрывает Frames()
eng.Frames() <-chan output.Frame
eng.CanvasSize() (w, h int)
eng.SaveFrames(dir string)            // дебаг: PNG на диск

// Ввод
eng.SetFocus(w widget.Widget)
eng.SendKeyEvent(e widget.KeyEvent)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
eng.SendMouseMove(x, y int)

// Модальные окна
eng.ShowModal(m widget.ModalWidget)
eng.CloseModal(m widget.ModalWidget)
```

`output.Frame` содержит `Seq uint64`, `Timestamp time.Time` и `[]DirtyTile{X, Y, W, H int; Data []byte}`.

---

## Виджеты

Каждый виджет встраивает `widget.Base`, которая реализует `SetBounds`, `AddChild`, `Children`, а также Grid-свойства (`GridRow`, `GridColumn`, `GridRowSpan`, `GridColSpan`).

```go
w.SetBounds(image.Rect(x, y, x+w, y+h))  // обязательно перед первым кадром
parent.AddChild(child)
```

### Panel

Контейнер с фоном, рамкой, скруглёнными углами, фоновым изображением и встроенным заголовком окна.

```go
p := widget.NewPanel(color.RGBA{R: 45, G: 45, B: 65, A: 255})
p.ShowBorder    = true
p.BorderColor   = color.RGBA{...}
p.CornerRadius  = 8
p.UseAlpha      = true

widget.NewWin10Panel()  // стандартная полупрозрачная тёмная панель
```

**Фоновое изображение** — загружается через XAML-атрибут `BackgroundImage="pam.png"` (путь относительно XAML-файла). Изображение масштабируется под размер панели. Поддерживаются PNG и JPEG.

**Заголовок окна:**

```go
p.Caption      = "Моё приложение"
p.ShowHeader   = true           // по умолчанию true
p.MacStyle     = false          // false=Windows, true=macOS
p.HeaderHeight = 38             // по умолчанию 32px
p.OnClose      = func() { ... } // кнопка × в заголовке
```

Windows-стиль: тёмная полоса, текст слева, кнопки ─ □ × справа. macOS-стиль: traffic lights слева, текст по центру.

### Grid

WPF-совместимая сетка с тремя режимами размеров: Pixel, Star (пропорциональный), Auto (по содержимому).

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
g.ShowGridLines = true  // для отладки
```

Дочерние виджеты указывают ячейку через attached-свойства:

```go
label.SetGridProps(row, col, rowSpan, colSpan)
// или в XAML: Grid.Row="1" Grid.Column="0" Grid.ColumnSpan="2"
```

В XAML:

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
           Text="Заголовок" Foreground="White" Background="#0078D4"/>
    <Button Grid.Row="2" Grid.Column="1" Content="OK" Style="Accent"/>
</Grid>
```

### Label

```go
lbl := widget.NewWin10Label("Текст")
lbl := widget.NewLabel("Текст", color.RGBA{...})

lbl.SetText("новый текст")  // потокобезопасно
lbl.Text() string
lbl.WrapText = true          // перенос слов по ширине
lbl.FontSize = 14.0
```

В XAML: `TextWrapping="Wrap"`, `FontSize="14"`.

### Button

```go
btn := widget.NewButton("Текст")
btn := widget.NewWin10AccentButton("OK")  // синяя, основное действие

btn.OnClick   = func() { ... }
btn.HoverBG   = color.RGBA{...}  // цвет при наведении
btn.PressedBG = color.RGBA{...}  // цвет при нажатии
```

В XAML: `HoverBG="#C42B1C"`, `PressedBG="#A01E14"`, `Background`, `Foreground`, `BorderBrush`.

### TextInput

```go
inp := widget.NewTextInput("placeholder...")

inp.SetText("значение")
inp.GetText() string

inp.OnEnter  = func() { ... }
inp.OnChange = func(text string) { ... }
```

Клавиатура: Backspace, Delete, стрелки, Home, End. Shift+стрелки — выделение. Ctrl+A/C/X/V — буфер обмена.

### PasswordBox

```go
inp := widget.NewPasswordInput("Введите пароль...")
```

В XAML: `<PasswordBox Placeholder="Пароль..."/>`.

### Dropdown

```go
dd := widget.NewDropdown("Пункт 1", "Пункт 2", "Пункт 3")

dd.SetSelected(idx int)
dd.Selected() int
dd.SelectedText() string
dd.OnChange = func(idx int, text string) { ... }
```

В XAML — два варианта:

```xml
<ComboBox Items="RDP,VNC,SSH" SelectedIndex="0"/>

<ComboBox>
    <ComboBoxItem Content="Администратор"/>
    <ComboBoxItem Content="Оператор"/>
</ComboBox>
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
rb1 := widget.NewRadioButton("Вариант A", "myGroup")
rb2 := widget.NewRadioButton("Вариант B", "myGroup")

rb1.SetSelected(true)  // rb2 автоматически сбрасывается
rb1.IsSelected() bool
rb1.OnChange = func(selected bool) { ... }
rb1.RemoveFromGroup()  // при деструкции
```

### ToggleSwitch

```go
ts := widget.NewToggleSwitch("Тёмная тема")

ts.SetOn(true)
ts.IsOn() bool
ts.OnChange = func(on bool) { ... }
```

### ProgressBar

```go
pb := widget.NewProgressBar()
pb.SetValue(0.75)   // [0.0, 1.0], потокобезопасно
pb.Value() float64
```

В XAML: `<ProgressBar Value="0.65" Foreground="#A6E3A1"/>`.

### Slider

```go
s := widget.NewSlider()            // [0.0, 1.0]
s := widget.NewSliderRange(0, 100) // произвольный диапазон

s.SetValue(0.5)
s.Value() float64
s.OnChange = func(value float64) { ... }
```

Клавиатура: стрелки — шаг 5%, Shift+стрелки — шаг 1%, Home/End — мин/макс.

### TabControl

```go
tc := widget.NewTabControl(
    widget.TabItem{Header: "Общие",    Content: generalPanel},
    widget.TabItem{Header: "Настройки", Content: settingsPanel},
)

tc.AddTab("Ещё", anotherPanel)
tc.SetActive(0)
tc.Active() int
tc.TabCount() int
tc.OnTabChange = func(index int, header string) { ... }
```

В XAML:

```xml
<TabControl SelectedIndex="0">
    <TabItem Header="Общие">
        <Canvas Width="600" Height="368">
            <Label Left="10" Top="10" Text="Содержимое"/>
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
lv := widget.NewListView("Элемент 1", "Элемент 2", "Элемент 3")

lv.AddItem("Ещё")
lv.Clear()
lv.SetSelected(0)
lv.Selected() int        // -1 если нет выделения
lv.SelectedText() string
lv.OnSelect = func(index int, text string) { ... }
```

В XAML:

```xml
<ListView>
    <ListViewItem Content="Запись 1"/>
    <ListViewItem Content="Запись 2"/>
</ListView>
```

### Image

```go
img := widget.NewImageWidget()
img.SetSource("assets/logo.png")  // PNG или JPEG
img.SetImage(myImage)             // image.Image напрямую
img.Stretch = widget.ImageStretchFill     // растянуть (по умолчанию)
              widget.ImageStretchUniform  // вписать с пропорциями
              widget.ImageStretchNone     // оригинальный размер
```

### PopupMenu

Контекстное / всплывающее меню. Рисуется как overlay поверх всего UI.

```go
menu := widget.NewPopupMenu()
menu.AddItem("Копировать", func() { /* ... */ })
menu.AddItem("Вставить", func() { /* ... */ })
menu.AddSeparator()
menu.AddItem("Удалить", func() { /* ... */ })

menu.OnSelect = func(idx int, text string) {
    log.Printf("Выбрано: %s", text)
}

menu.Show(x, y)          // показать в координатах
menu.ShowBelow(button)    // показать под виджетом
menu.ShowRight(widget)    // показать справа от виджета
menu.Close()              // закрыть
```

XAML:

```xml
<PopupMenu Name="ctxMenu">
    <MenuItem Text="Копировать"/>
    <MenuItem Text="Вставить"/>
    <MenuItem Separator="True"/>
    <MenuItem Text="Отключено" Disabled="True"/>
    <MenuItem Text="Удалить"/>
</PopupMenu>
```

Меню закрывается по клику за пределами или по Escape. Навигация стрелками и Enter.

### MenuBar

Горизонтальная полоса меню (как в классических Windows-приложениях). Каждый пункт верхнего уровня раскрывает PopupMenu с подпунктами. При наведении на соседний пункт подменю автоматически переключается.

```go
menu := widget.NewMenuBar()
menu.AddMenu("Файл",
    widget.MenuItem{Text: "Новый"},
    widget.MenuItem{Text: "Открыть"},
    widget.MenuItem{Separator: true},
    widget.MenuItem{Text: "Выход"},
)
menu.AddMenu("Правка",
    widget.MenuItem{Text: "Копировать"},
    widget.MenuItem{Text: "Вставить"},
)

menu.OnSelect = func(topIdx, subIdx int, text string) {
    log.Printf("Меню: %s", text)
}
```

XAML:

```xml
<Menu Name="mainMenu" Left="0" Top="0" Width="800" Height="28">
    <MenuItem Header="Файл">
        <MenuItem Text="Новый"/>
        <MenuItem Text="Открыть"/>
        <MenuItem Separator="True"/>
        <MenuItem Text="Выход"/>
    </MenuItem>
    <MenuItem Header="Правка">
        <MenuItem Text="Копировать"/>
        <MenuItem Text="Вставить"/>
    </MenuItem>
</Menu>
```

Навигация: Left/Right переключает разделы, Up/Down/Enter — по подменю, Escape — закрыть.

### Separator

В XAML: `<Separator Width="400" Height="1" Background="#FF0000"/>`.

### MessageBox

```go
mb := widget.NewMessageBox(eng)

mb.Show("Ошибка", "Что-то пошло не так")                    // OK
mb.ShowYesNo("Выход", "Выйти без сохранения?", callback)    // Да/Нет
mb.ShowYesNoCancel("Сохранение", "Сохранить?", callback)     // Да/Нет/Отмена
```

---

## Ввод

### Мышь

```go
eng.SendMouseMove(x, y int)
eng.SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
// btn: widget.MouseLeft | widget.MouseRight | widget.MouseMiddle
```

Движок делает hit-test и передаёт событие нужному виджету. При ЛКМ фокус переходит на `Focusable`-виджет под курсором.

### Клавиатура

```go
eng.SendKeyEvent(widget.KeyEvent{
    Code:    widget.KeyLeft,
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
eng.SetTheme(widget.DarkTheme())   // Windows 10 Dark (по умолчанию)
eng.SetTheme(widget.LightTheme())  // Windows 10 Light

// Кастомная тема
t := widget.DarkTheme()
t.Accent = color.RGBA{R: 200, G: 50, B: 50, A: 255}
eng.SetTheme(t)
```

`SetTheme` применяет цвета ко всем существующим виджетам через `ApplyTheme(t)` и обновляет глобальные дефолты для новых.

---

## XAML

Движок читает стандартный WPF XAML. Файлы совместимы с Blend / Visual Studio.

### Загрузка

```go
root, named, err := widget.LoadUIFromXAMLFile("gui/window.xaml")
if err != nil { log.Fatal(err) }

// Найти виджет по Name / x:Name
loginBtn := named["btnLogin"].(*widget.Button)
loginBtn.OnClick = func() { ... }

eng.SetRoot(root)
```

Также доступны `LoadUIFromXAML(data []byte)` и `LoadUIFromXAMLWithBase(data, baseDir)` для загрузки из памяти.

### Координаты

Координаты дочерних элементов **относительные** (стандарт WPF Canvas):

```
root Canvas (0,0)
  └─ Border mainWin (Left=100, Top=50)       → абсолютно: (100, 50)
       └─ Label (Left=10, Top=5)             → абсолютно: (110, 55)
```

Для Grid-потомков координаты задаются сеткой через `Grid.Row` / `Grid.Column` — атрибуты `Left` и `Top` игнорируются.

### Таблица XAML-элементов

| WPF элемент | Виджет | Ключевые атрибуты |
|---|---|---|
| `Canvas`, `Border`, `StackPanel`, `DockPanel` | Panel | `Background`, `CornerRadius`, `Caption`, `ShowHeader`, `MacStyle`, `BackgroundImage`, `BorderBrush` |
| `Grid` | Grid | `ShowGridLines`, `Grid.RowDefinitions`, `Grid.ColumnDefinitions` |
| `Label`, `TextBlock` | Label | `Text`, `Foreground`, `Background`, `TextWrapping`, `FontSize` |
| `Button`, `ToggleButton`, `RepeatButton` | Button | `Content`, `Style="Accent"`, `HoverBG`, `PressedBG`, `Background`, `Foreground`, `BorderBrush` |
| `TextBox` | TextInput | `Placeholder`, `Text`, `Foreground` |
| `PasswordBox` | TextInput (пароль) | `Placeholder`, `Text` |
| `ComboBox` | Dropdown | `Items`, `SelectedIndex`, дочерние `<ComboBoxItem>` |
| `ProgressBar` | ProgressBar | `Value`, `Foreground` |
| `CheckBox` | CheckBox | `Content`, `IsChecked` |
| `RadioButton` | RadioButton | `Content`, `GroupName`, `IsChecked` |
| `TabControl` | TabControl | `SelectedIndex`, дочерние `<TabItem Header="...">` |
| `Slider` | Slider | `Minimum`, `Maximum`, `Value` |
| `ToggleSwitch` | ToggleSwitch | `Content`, `IsOn` |
| `ScrollViewer` | ScrollView | `ContentHeight`, `Background` |
| `ListView`, `ListBox` | ListView | `Items`, `SelectedIndex`, `ItemHeight`, дочерние `<ListViewItem>` |
| `Image` | Image | `Source`, `Stretch` (Fill/Uniform/None) |
| `PopupMenu`, `ContextMenu` | PopupMenu | дочерние `<MenuItem Text="..." Separator="True" Disabled="True"/>` |
| `Menu`, `MenuBar`, `MainMenu` | MenuBar | дочерние `<MenuItem Header="...">` с вложенными `<MenuItem>` |
| `Separator`, `Line`, `Rectangle` | Separator | `Background` |

Общие атрибуты: `Name`/`x:Name`, `Left`/`Canvas.Left`, `Top`/`Canvas.Top`, `Width`, `Height`, `Grid.Row`, `Grid.Column`, `Grid.RowSpan`, `Grid.ColumnSpan`.

---

## Нативное окно (window)

Отдельный модуль на базе Ebiten v2. На Windows — DirectX 11 без CGO.

```go
import "github.com/oops1/headless-gui/v3/window"

eng := engine.New(1280, 720, 30)
// ... строим UI, eng.Start() ...

win := window.New(eng, "Заголовок окна")
win.SetMaxFPS(60)
win.SetResizable(true)

if err := win.Run(); err != nil {  // блокирует до закрытия
    log.Fatal(err)
}
```

---

## Свой виджет

```go
type MyWidget struct {
    widget.Base                      // обязательно
    Color color.RGBA
}

func (w *MyWidget) Draw(ctx widget.DrawContext) {
    b := w.Bounds()
    ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, w.Color)
    w.Base.DrawChildren(ctx)
}

// Опционально — интерфейсы:
func (w *MyWidget) OnMouseButton(e widget.MouseEvent) bool { ... }  // клики
func (w *MyWidget) OnMouseMove(x, y int)                   { ... }  // hover
func (w *MyWidget) OnKeyEvent(e widget.KeyEvent)           { ... }  // клавиатура
func (w *MyWidget) SetFocused(v bool)                      { ... }  // фокус
func (w *MyWidget) IsFocused() bool                        { ... }
func (w *MyWidget) ApplyTheme(t *widget.Theme)             { ... }  // темы
```

### DrawContext API

```go
// Прямоугольники
ctx.FillRect(x, y, w, h int, col color.RGBA)
ctx.FillRectAlpha(x, y, w, h int, col color.RGBA)
ctx.FillRoundRect(x, y, w, h, r int, col color.RGBA)
ctx.DrawBorder(x, y, w, h int, col color.RGBA)
ctx.DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

// Линии
ctx.DrawHLine(x, y, length int, col color.RGBA)
ctx.DrawVLine(x, y, length int, col color.RGBA)
ctx.SetPixel(x, y int, col color.RGBA)

// Изображения
ctx.DrawImage(src image.Image, x, y int)
ctx.DrawImageScaled(src image.Image, x, y, w, h int)

// Текст
ctx.DrawText(text string, x, y int, col color.RGBA)
ctx.DrawTextSize(text string, x, y int, pt float64, col)
ctx.DrawTextFont(text string, x, y int, pt float64, name string, col)
ctx.MeasureText(text string, pt float64) int
ctx.MeasureRunePositions(text string, pt float64) []int

// Clip
ctx.SetClip(r image.Rectangle)
ctx.ClearClip()
```

---

## Структура модулей

```
go.mod:  module github.com/oops1/headless-gui/v3
  require golang.org/x/image

go.mod:  module github.com/oops1/headless-gui/v3/window
  require github.com/oops1/headless-gui/v3 => ../
  require github.com/hajimehoshi/ebiten/v2
```

Приложение-потребитель подключает основной модуль:

```
require github.com/oops1/headless-gui/v3 v0.x.x
```

Если нужно нативное окно:

```
require github.com/oops1/headless-gui/v3/window v0.x.x
```

Для локальной разработки используйте `replace`:

```
replace github.com/oops1/headless-gui/v3 => ../GuiEngine
replace github.com/oops1/headless-gui/v3/window => ../GuiEngine/window
```

---

## Демо-приложения

Запуск из директории `window/` (где лежит go.mod с Ebiten):

```bash
cd GuiEngine/window

go run ../cmd/showcase    # все виджеты + живая анимация
go run ../cmd/guiview     # интерактивное демо с модальными XAML-окнами
go run ../cmd/griddemo    # Grid-раскладка

# Бинарник без консоли (Windows)
go build -ldflags="-H windowsgui" -o showcase.exe ../cmd/showcase
```
