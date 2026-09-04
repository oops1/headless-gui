# headless-gui — руководство разработчика

## Обзор

`headless-gui` — off-screen GUI-движок на Go. Рендерит виджеты в RGBA-буфер и выдаёт только изменившиеся тайлы 64x64 px. Не зависит от оконной системы — вывод подключается отдельно (RDP, WebSocket, нативное окно).

```
headless-gui/
  engine/              рендер-цикл, canvas, события, шрифты
  widget/              виджеты, темы, XAML-загрузчик, Grid layout
    treeview/          ядро TreeView (модель, шаблоны, рендер, ввод)
    datagrid/          ядро DataGrid (ObservableCollection, PropertyNotifier)
  output/              типы Frame / DirtyTile
  window/              нативное окно Win32/Cocoa/X11 (отдельный go.mod, без CGO)
  cmd/
    showcase/          полная демонстрация всех виджетов
    guiview/           интерактивное демо с модальными окнами
    griddemo/          демо Grid-раскладки
    smartgit/          SmartGit-подобный UI (Window + Menu + TreeView + DataGrid)
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
eng.SetBackgroundFile(path string)    // PNG/JPEG с диска
eng.SetBackground(img image.Image)    // готовое изображение из памяти
eng.ClearBackground()                 // снять обои
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

### Window

Корневой элемент нативного окна. Заменяет Canvas/Panel как корень при работе с нативным окном ОС.

```go
// XAML-загрузка (рекомендуемый способ)
root, reg, _ := widget.LoadUIFromXAMLFile("ui/app.xaml")
eng.SetRoot(root)

// Программно
ww := widget.NewWindow()
ww.Title = "Моё приложение"
ww.TitleStyle = widget.WindowTitleWin  // или WindowTitleMac
ww.Resize = widget.ResizeModeCanResize
```

В XAML:

```xml
<Window Title="Приложение" Width="1100" Height="700"
        TitleStyle="Win" ResizeMode="CanResize" Background="#1E1E1E">
    <DockPanel>
        <Menu DockPanel.Dock="Top">...</Menu>
        <Grid>...</Grid>
    </DockPanel>
</Window>
```

Стили заголовка:
- `WindowTitleWin` — Windows: текст слева, кнопки ─ □ × справа
- `WindowTitleMac` — macOS: traffic lights ● ● ● слева, текст по центру

Режимы изменения размера: `CanResize`, `NoResize`, `CanMinimize`.

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

**Кнопка-переключатель.** Состояние «нажата» держится между кликами: кнопка
сама красится в `PressedBG`, пока включена, а под курсором фон слегка ведётся
в сторону `HoverBG` — чтобы и состояние было видно, и наведение.

```go
btn.SetToggle(true)            // сделать переключателем
btn.SetChecked(true)           // задать состояние (обработчик НЕ зовётся)
btn.IsChecked()                // спросить состояние
btn.Toggle()                   // переключить и сообщить обработчику
btn.OnCheckedChanged = func(on bool) { ... }
```

В XAML переключатель даёт сам тег, начальное состояние — атрибут:

```xml
<ToggleButton x:Name="fltModified" Content="Изменённые" IsChecked="True"/>
```

Отдельного типа `ToggleButton` нет: от `Button` он отличается ровно этим
состоянием, а не раскладкой, иконками, командами или подписками. `IsChecked`
читается и у обычной кнопки — «нарисуй выбранной», без поведения
переключателя.

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

Каскадные подменю (вложенные MenuItem):

```xml
<Menu Name="mainMenu">
    <MenuItem Header="Settings">
        <MenuItem Header="Theme">
            <MenuItem Header="Dark"/>
            <MenuItem Header="Light"/>
        </MenuItem>
    </MenuItem>
</Menu>
```

Пункты с вложенными подменю отображают стрелку ▸ справа. При наведении раскрывается дочернее меню.

Навигация: Left/Right переключает разделы, Up/Down/Enter — по подменю, Right — войти в каскадное подменю, Left — выйти, Escape — закрыть.

### TreeView

WPF-совместимый иерархический список с виртуализацией, HierarchicalDataTemplate, иконками и клавиатурной навигацией. Архитектура: ядро в `widget/treeview/`, обёртка `widget.TreeViewWidget`.

```go
tw := widget.NewTreeViewWidget()
tw.SetBounds(image.Rect(0, 0, 300, 500))

// Создание узлов
root := widget.NewTreeNode("Корень")
child1 := widget.NewTreeNode("Ветка 1")
child2 := widget.NewTreeNode("Ветка 2")
leaf := widget.NewTreeNode("Лист")

child1.AddChild(leaf)
root.AddChild(child1)
root.AddChild(child2)
root.Expanded = true

tw.Tree.AddRoot(root)
```

Свойства узла (TreeViewItem / TreeNode):

```go
item.Text       // текст
item.Header     // WPF-алиас для Text
item.Icon       // image.Image (иконка перед текстом)
item.Expanded   // раскрыт ли
item.IsSelected // выбран ли
item.IsEnabled  // активен ли
item.Tag        // произвольные данные
item.DataContext // объект для data binding
item.Children   // дочерние []*TreeViewItem
```

Методы узла:

```go
item.AddChild(child)
item.InsertChild(idx, child)
item.RemoveChild(child)
item.RemoveChildAt(idx)
item.ClearChildren()
item.HasChildren() bool
item.Parent() *TreeViewItem
item.Depth() int
item.DisplayText() string  // Header → Text → fmt.Sprint(DataContext)
```

Свойства TreeView (через `tw.Tree`):

```go
tw.Tree.ItemHeight       // высота строки (px), по умолчанию 22
tw.Tree.IndentSize       // отступ уровня (px), по умолчанию 18
tw.Tree.FontSize         // размер шрифта, по умолчанию 10
tw.Tree.IconSize         // размер иконки (px), по умолчанию 16
tw.Tree.IsReadOnly       // только чтение
tw.Tree.ShowIndentGuides // линии иерархии
```

Управление деревом:

```go
tw.Tree.AddRoot(item)
tw.Tree.SetRoots(items)
tw.Tree.ClearRoots()
tw.Tree.Roots() []*TreeViewItem
tw.Tree.SelectedItem() *TreeViewItem
tw.Tree.SetSelectedItem(item)
tw.Tree.ExpandItem(item)
tw.Tree.CollapseItem(item)
tw.Tree.ToggleExpand(item)
```

События:

```go
tw.Tree.OnSelect = func(item *treeview.TreeViewItem) { ... }

tw.Tree.OnSelectedItemChanged = func(e treeview.SelectedItemChangedEvent) {
    // e.OldItem, e.NewItem
}
tw.Tree.OnExpanded = func(e treeview.ExpandedEvent) { ... }
tw.Tree.OnCollapsed = func(e treeview.CollapsedEvent) { ... }
tw.Tree.OnItemInvoked = func(e treeview.ItemInvokedEvent) { ... } // двойной клик
```

Data Binding с HierarchicalDataTemplate:

```go
import "github.com/oops1/headless-gui/v3/widget/treeview"

tmpl := &treeview.HierarchicalDataTemplate{
    ItemsSourcePath: "Children",
    HeaderPath:      "Name",
    IconPath:        "Icon",
}
tw.Tree.SetItemTemplate(tmpl)

// ObservableCollection
coll := datagrid.NewObservableCollection()
coll.Add(myDataObject)
tw.Tree.SetItemsSource(coll)
```

Клавиатура: ↑/↓ перемещение, ←/→ свёртывание/раскрытие и переход к родителю/ребёнку, Home/End, PageUp/PageDown, Enter/Space — переключение + invoke.

Мышь: клик — выделение, двойной клик — раскрытие/свёртывание, клик по стрелке — toggle.

В XAML:

```xml
<TreeView Name="tree" Width="300" Height="500"
          IndentSize="20" ShowIndentGuides="True">
    <TreeViewItem Header="Корень" IsExpanded="True">
        <TreeViewItem Header="Ветка 1">
            <TreeViewItem Header="Лист"/>
        </TreeViewItem>
        <TreeViewItem Header="Ветка 2"/>
    </TreeViewItem>
</TreeView>
```

С HierarchicalDataTemplate:

```xml
<TreeView Name="tree" Width="300" Height="500">
    <TreeView.ItemTemplate>
        <HierarchicalDataTemplate ItemsSource="{Binding Children}">
            <StackPanel Orientation="Horizontal">
                <Image Source="{Binding Icon}" Width="16" Height="16"/>
                <TextBlock Text="{Binding Name}"/>
            </StackPanel>
        </HierarchicalDataTemplate>
    </TreeView.ItemTemplate>
</TreeView>
```

Виртуализация: рендерятся только видимые строки. Поддержка 10 000+ узлов.

### DataGrid

WPF-совместимая таблица данных с колонками, сортировкой, редактированием ячеек, изменением ширины колонок и виртуализацией. Архитектура: ядро в `widget/datagrid/`, обёртка `widget.DataGridWidget`.

```go
dg := widget.NewDataGridWidget()
dg.SetBounds(image.Rect(0, 0, 800, 400))

// Добавление колонок
dg.Grid.AddColumn(datagrid.NewTextColumn("Имя", "Name"))
dg.Grid.AddColumn(datagrid.NewTextColumn("Возраст", "Age"))
dg.Grid.AddColumn(datagrid.NewCheckBoxColumn("Активен", "IsActive"))

// Источник данных
coll := datagrid.NewObservableCollection()
coll.Add(&User{Name: "Алексей", Age: 30, IsActive: true})
coll.Add(&User{Name: "Мария", Age: 25, IsActive: false})
dg.Grid.SetItemsSource(coll)
```

Типы колонок:

```go
// Текстовая колонка — отображает и редактирует строковые значения
datagrid.NewTextColumn("Заголовок", "BindingPath")

// Чекбокс-колонка — отображает bool как флажок
datagrid.NewCheckBoxColumn("Активен", "IsActive")

// Шаблонная колонка — пользовательский рендер ячеек.
// cdc.DrawCtx умеет то же, что и обычный контекст отрисовки, включая
// DrawImage/DrawImageScaled: значок статуса в ячейке рисуется здесь же.
datagrid.NewTemplateColumn("Действия", func(cdc datagrid.CellDrawContext) {
    cdc.DrawCtx.DrawImageScaled(icon, cdc.Rect.Min.X+4, cdc.Rect.Min.Y+4, 16, 16)
})
```

Ширина колонок (WPF-стиль):

```go
col.SetWidth(datagrid.StarWidth(1))    // пропорциональная (*)
col.SetWidth(datagrid.StarWidth(2))    // двойной вес (2*)
col.SetWidth(datagrid.PixelWidth(150)) // фиксированная 150px
col.SetWidth(datagrid.AutoWidth())     // по содержимому
```

Свойства DataGrid (через `dg.Grid`):

```go
dg.Grid.AutoGenerateColumns  // автогенерация колонок из структуры данных
dg.Grid.IsReadOnly           // только чтение
dg.Grid.CanUserSortColumns   // сортировка по клику на заголовок (по умолчанию true)
dg.Grid.CanUserResizeColumns // изменение ширины колонок (по умолчанию true)
dg.Grid.CanUserReorderColumns // перетаскивание колонок мышью (по умолчанию false)
dg.Grid.SelectionMode        // SelectionSingle или SelectionExtended
dg.Grid.ZebraStripes         // чередовать фон строк (по умолчанию true)
dg.Grid.RowHeight            // высота строки (по умолчанию 28px)
dg.Grid.HeaderHeight         // высота заголовка (по умолчанию 30px)
dg.Grid.FontSize             // размер шрифта (по умолчанию 10)
```

**Полосатость строк.** Обнулить разницу между `AlternateBG` и `Background`
недостаточно: `ApplyTheme` вычисляет `AlternateBG` из фона темы заново на
каждую смену темы. Цветом владеет тема, признаком «полосы нужны» —
`ZebraStripes`.

**Щелчок по заголовку отдельно от сортировки.** `OnHeaderClick` вызывается
независимо от `CanUserSortColumns`; вернув `true`, обработчик отменяет
сортировку. Кромка изменения ширины и ползунок прокрутки отбираются раньше —
отличать «щелчок» от «потянули за границу» приложению не нужно.

```go
dg.Grid.OnHeaderClick = func(col datagrid.Column, idx, x, y int) bool {
    showColumnsMenu(x, y) // своё меню выбора видимых колонок
    return true           // сортировать не надо
}
```

**Порядок колонок.** `MoveColumn(from, to)` переставляет колонку программно,
`OnColumnsReordered` сообщает о перестановке. С `CanUserReorderColumns = true`
колонку можно тащить за заголовок: порог в несколько пикселей отделяет
перетаскивание от щелчка, место вставки показывается линией.

По умолчанию перетаскивание ВЫКЛЮЧЕНО, потому что включение переносит момент
щелчка по заголовку с нажатия на отпускание: пока кнопка мыши не отпущена,
щелчок от захвата колонки не отличить.

**Подсказка на строку.** У `Base.ToolTip` один текст на весь виджет; строке
свой даёт `RowToolTip`:

```go
dg.Grid.RowToolTip = func(item interface{}, row int) string {
    return item.(*FileRow).State // «изменён», «конфликт», …
}
```

Если нужно посчитать что-то самому — наружу отданы `HoverRow()`,
`RowIndexAtY(y)`, `ScrollX()` и `ScrollY()`.

**Множественный выбор мышью.** При `SelectionMode = SelectionExtended`
Ctrl+Click добавляет и снимает строку, Shift+Click выделяет диапазон.
Модификаторы приходят в `MouseEvent.Mod`; приложение, которое кормит движок
событиями само, сообщает их через `eng.SetModifiers` (см. «Ввод»).

Управление данными:

```go
dg.Grid.SetItemsSource(coll)           // задать источник данных
dg.Grid.ItemsSource()                  // получить ObservableCollection
dg.Grid.SelectedItem() interface{}     // текущий выбранный элемент
dg.Grid.SelectedItems() []interface{}  // все выбранные (Extended)
dg.Grid.SetSelectedIndex(idx)          // выбрать строку по индексу
```

ObservableCollection — коллекция с уведомлениями об изменениях:

```go
coll := datagrid.NewObservableCollection()
coll.Add(item)            // добавить
coll.Insert(idx, item)    // вставить по индексу
coll.RemoveAt(idx)        // удалить по индексу
coll.Set(idx, item)       // заменить
coll.Clear()              // очистить
coll.Count() int          // количество
coll.Get(idx) interface{} // получить по индексу

coll.AddCollectionChanged(func(e datagrid.CollectionChangedEvent) {
    // e.Action: CollectionAdd, CollectionRemove, CollectionReplace, CollectionReset
})
```

Data Binding — привязка свойств объектов:

```go
// Binding с Path, Converter, StringFormat
b := &datagrid.Binding{
    Path:         "User.Name",        // вложенные пути через точку
    Mode:         datagrid.TwoWay,    // OneWay, TwoWay, OneTime
    StringFormat: "%.2f",             // формат вывода (необязательно)
}

// IValueConverter — преобразование значений
type MyConverter struct{}
func (c *MyConverter) Convert(value interface{}) interface{} { ... }
func (c *MyConverter) ConvertBack(value interface{}) interface{} { ... }
```

INotifyPropertyChanged — уведомления об изменении свойств объектов:

```go
type User struct {
    datagrid.PropertyNotifier
    name string
}

func (u *User) SetName(name string) {
    u.name = name
    u.NotifyPropertyChanged(u, "Name")
}
```

События:

```go
dg.Grid.OnSelectionChanged = func(e datagrid.SelectionChangedEvent) {
    // e.SelectedIndex, e.SelectedItem
}
dg.Grid.OnSorting = func(e *datagrid.SortingEvent) {
    // e.Column, e.Direction; e.Handled = true чтобы отменить
}
dg.Grid.OnCellEditEnding = func(e *datagrid.CellEditEndingEvent) {
    // e.RowIndex, e.Column, e.Item, e.NewValue; e.Cancel = true чтобы отменить
}
dg.Grid.OnRowEditEnding = func(rowIndex int, item interface{}) { ... }
```

Клавиатура: ↑/↓/←/→ навигация, Home/End, PageUp/PageDown, Tab/Shift+Tab между ячейками, Enter — начать/завершить редактирование, Escape — отмена, Ctrl+A — выделить все (Extended).

Мышь: клик — выделение, двойной клик — редактирование, перетаскивание грани колонки — изменение ширины, клик по заголовку — сортировка.

В XAML:

```xml
<DataGrid Name="grid" Width="800" Height="400"
          AutoGenerateColumns="False"
          CanUserSortColumns="True"
          CanUserResizeColumns="True"
          SelectionMode="Extended"
          IsReadOnly="False"
          RowHeight="28" HeaderHeight="30">
    <DataGrid.Columns>
        <DataGridTextColumn Header="Имя"
                           Binding="{Binding Name}" Width="*"/>
        <DataGridTextColumn Header="Возраст"
                           Binding="{Binding Age}" Width="100"/>
        <DataGridCheckBoxColumn Header="Активен"
                               Binding="{Binding IsActive}" Width="60"/>
    </DataGrid.Columns>
</DataGrid>
```

Форматы привязки: `{Binding Name}`, `{Binding Path=User.Name}`, `"Name"` (без фигурных скобок).

Форматы ширины: `"*"`, `"2*"`, `"Auto"`, `"150"` (пиксели).

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

**Модификаторы при щелчке.** Ctrl+Click и Shift+Click доезжают до виджета в
`MouseEvent.Mod`. Взять их из клавиатурных событий движок не может: отдельной
клавиши-модификатора в `KeyCode` нет, и человек, зажавший Ctrl и щёлкнувший
мышью, не порождает ни одного клавиатурного события. Состояние сообщает тот,
кто его знает:

```go
eng.SetModifiers(widget.ModCtrl) // держат Ctrl
eng.SendMouseButton(x, y, widget.MouseLeft, true)
eng.SetModifiers(widget.ModNone) // отпустили
```

`window.Run` делает это сам на каждое нажатие и отпускание Ctrl/Shift/Alt —
приложению с нативным окном звать `SetModifiers` не нужно. Свои события мыши
движок метит текущим состоянием: нажатия, движения и колесо.

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

Тема содержит 80+ цветовых токенов, сгруппированных по виджетам:

- Окно/панели: `WindowBG`, `PanelBG`, `TitleBG`, `TitleText`, `Border`, `ShadowColor`
- Кнопки: `BtnBG`, `BtnHoverBG`, `BtnPressedBG`, `BtnText`, `BtnBorder`
- Текстовые поля: `InputBG`, `InputText`, `InputFocus`, `InputCaret`, `InputPlaceholder`
- Выпадающие списки/PopupMenu: `DropBG`, `DropText`, `DropBorder`
- TreeView: `TreeText`, `TreeArrow`
- ListView/ScrollView: `ListItemHover`, `ListItemSelect`, `ScrollTrackBG`, `ScrollThumbBG`
- Dialog: `DialogBG`, `DialogTitleBG`, `DialogDim`
- GridSplitter: `SplitterBG`, `SplitterHoverBG`
- StatusBar: `StatusBarBG`, `StatusBarText`
- DataGrid header: `HeaderBG`, `HeaderText`
- Системные: `Accent`, `Disabled`, `Scrollbar`

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

**Однофайловая сборка.** У приложения, уехавшего одним исполняемым файлом, нет
каталога с ресурсами на диске — `LoadUIFromXAMLWithBase` там не к чему
привязать. Для этого случая `LoadUIFromXAMLFS`: ресурсы разметки (`Image
Source`, `Button Icon/IconSource`, `Panel BackgroundImage`, `SVGIcon Source`,
`Window TrayIcon`) читаются из `fs.FS`.

```go
//go:embed ui
var uiFS embed.FS

// fs.Sub переносит корень к самой разметке: пути ресурсов в XAML считаются
// от КОРНЯ fsys, а не от файла разметки. Без этого Source="icons/ok.png"
// пришлось бы писать как "ui/icons/ok.png".
sub, _ := fs.Sub(uiFS, "ui")
data, _ := fs.ReadFile(sub, "app.xaml")
root, named, err := widget.LoadUIFromXAMLFS(data, sub)
```

Путь из разметки удерживается внутри `fsys` по той же политике, что и `baseDir`
на диске: абсолютный путь или выход за пределы корня отклоняются. Шрифты
укладываются в сборку тем же приёмом — `eng.RegisterFontFS` (см. «Шрифты»).

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
| `NumericUpDown`, `IntegerUpDown`, `DoubleUpDown` | NumericUpDown | `Minimum`, `Maximum`, `Increment`, `Decimals`, `Value` |
| `ToggleSwitch` | ToggleSwitch | `Content`, `IsOn` |
| `ScrollViewer` | ScrollView | `ContentHeight`, `Background` |
| `ListView`, `ListBox` | ListView | `Items`, `SelectedIndex`, `ItemHeight`, дочерние `<ListViewItem>` |
| `VirtualizingItemsControl` | VirtualizingItemsControl | `ItemHeight`, `Buffer`, `ItemsSource`, `VirtualizingItemsControl.ItemTemplate` |
| `WrapPanel` | WrapPanel | `Spacing`, `Orientation` |
| `UniformGrid` | UniformGrid | `Rows`, `Columns`, `Spacing` |
| `GroupBox` | GroupBox | `Header` |
| `Expander` | Expander | `Header`, `IsExpanded` |
| `Ellipse`, `Rectangle`, `Line`, `Polygon`, `Polyline` | Shapes | `Fill`, `Stroke`, `StrokeThickness`, `Points`, `RadiusX` |
| `Image` | Image | `Source`, `Stretch` (Fill/Uniform/None) |
| `PopupMenu`, `ContextMenu` | PopupMenu | дочерние `<MenuItem Text="..." Separator="True" Disabled="True"/>` |
| `Menu`, `MenuBar`, `MainMenu` | MenuBar | дочерние `<MenuItem Header="...">` с вложенными `<MenuItem>` |
| `TreeView` | TreeViewWidget | `IndentSize`, `IsReadOnly`, `ShowIndentGuides`, дочерние `<TreeViewItem>`, `<TreeView.ItemTemplate>` |
| `TreeViewItem` | TreeViewItem | `Header`, `IsExpanded`, `Icon`, `IsEnabled` |
| `HierarchicalDataTemplate` | HierarchicalDataTemplate | `ItemsSource="{Binding ...}"`, дочерние `<StackPanel>` с `<Image>` + `<TextBlock>` |
| `DataGrid` | DataGridWidget | `AutoGenerateColumns`, `IsReadOnly`, `CanUserSortColumns`, `CanUserResizeColumns`, `SelectionMode`, `RowHeight`, `HeaderHeight` |
| `DataGridTextColumn` | DataGridTextColumn | `Header`, `Binding`, `Width`, `IsReadOnly`, `SortMemberPath` |
| `DataGridCheckBoxColumn` | DataGridCheckBoxColumn | `Header`, `Binding`, `Width`, `IsReadOnly` |
| `DataGridTemplateColumn` | DataGridTemplateColumn | `Header`, `Width` |
| `SplitPanel` | SplitPanel | `Orientation`, `Position`, `SplitterSize`, `MinFirst`, `MinSecond` (первые два дочерних — панели) |
| `SVGIcon` | SVGIcon | `Source`, `Color`, `Tint` |
| `Separator` | Separator | `Background` |
| `DockManager` | DockManager | `Background`, `NativeFloating`; дочерние `<DockPane>`×N + один `<DockContent>` (см. «Докинг-панели») |
| `DockPane` | DockPane | `Id`, `Title`, `Side` (Left/Top/Bottom/Right), `Size`, `State` (Docked/AutoHidden/Floating/Closed); только внутри `<DockManager>` |
| `DockContent` | — (маркер) | единственный ребёнок → `DockManager.SetCenter`; только внутри `<DockManager>` |
| `Window` | Window | `Title`, `Width`, `Height`, `WindowStyle`, `ResizeMode`, `MainWindow`, `TrayIcon`, `TrayTooltip` (см. «Трей из XAML») |
| `TrayMenu` | — (ребёнок `<Window>`) | меню трея: дочерние `<MenuItem>`/`<Separator>` (см. «Трей из XAML») |

Общие атрибуты: `Name`/`x:Name`, `Left`/`Canvas.Left`, `Top`/`Canvas.Top`, `Width`, `Height`, `Grid.Row`, `Grid.Column`, `Grid.RowSpan`, `Grid.ColumnSpan`, `ToolTip`, `Visibility`, `IsEnabled`, `TabIndex`. Привязки `{Binding ...}` и локализация `{Loc Key}` работают на любом строковом атрибуте.

---

## Нативное окно (window) — Win32 / Cocoa / X11

Отдельный модуль с платформенными бэкендами. Без CGO на всех платформах (Windows: Win32 API, macOS: Cocoa через purego, Linux: X11 protocol).

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
ctx.Clip() image.Rectangle   // текущая область отсечения (для вложенного клиппинга)
```

### Контракт отрисовки

Начиная с v3.15.0 движок умеет пропускать поддеревья, чей охватывающий
прямоугольник не пересекает ни одну изменившуюся область (damage) — метод
`Draw` таких виджетов в этом кадре просто не вызывается. Это резко дешевле на
экранах с редко меняющимися участками (панель задач, часы, статичная форма
рядом с активным полем ввода), но означает, что `Draw` перестаёт быть
надёжным местом для чего бы то ни было, кроме собственно рисования. Если ваш
виджет соответствует четырём правилам ниже, оптимизация для него прозрачна;
если нет — она молча ломает то, что раньше работало по случайности.

Приложение, которое ещё не привели в соответствие, может вернуть прежнее
поведение одной строкой: `eng.SetSubtreeCulling(false)` отключает пропуск
целиком (по умолчанию пропуск включён). Выключатель — свойство ДВИЖКА, а не
процесса: движки в одном процессе не влияют друг на друга.

**1. `Draw` не гарантирован каждый кадр.** Пропуск вызова означает «на экране
и так то, что нужно» — виджет не имеет права требовать иного и не должен
предполагать, что раз он существует в дереве, то и рисуется.

**2. Анимация — только через `widget.Animate` / `widget.AnimateOwned`,
никогда счётчиком кадров или замером времени внутри `Draw`.** Тикающая
переменная, которую `Draw` увеличивает при каждом вызове, останавливается,
как только виджет выпадает из damage — анимация просто замирает вместо того,
чтобы доиграть до конца:

```go
// Плохо: анимация живёт, пока движок зовёт Draw
func (w *MyWidget) Draw(ctx widget.DrawContext) {
    w.frame++                    // без вызова Draw кадр не продвинется
    ...
}

// Хорошо: тикает независимо от того, рисуют виджет в этом кадре или нет
widget.AnimateOwned(w, "pulse", 400*time.Millisecond, widget.EaseOutCubic, func(t float64) {
    w.phase = t
    w.Invalidate()
})
```

Обратите внимание на `Invalidate()` в тике: **он обязателен**. Движок не
считает существование анимации поводом рисовать — он рисует по damage. Тик,
меняющий виджет через сеттеры (`SetBounds`, `SetValue` и прочие), заявляет
damage сам; тик, пишущий прямо в поле, обязан позвать `Invalidate()`, иначе
изменение не попадёт ни в один кадр.

Так сделано ради простоя. Раньше движок готовил кадр на каждый тик, пока
хоть одна анимация зарегистрирована, — и секундные часы на панели задач
держали неподвижный рабочий стол на полной частоте. Причём кадр выходил
ПОЛНЫМ: damage при таком пробуждении пуст, а пустой damage уводит кадр по
полному пути — блит фона во весь холст, обход всего дерева без клипа,
сравнение всех тайлов. Кадр, которому нечего делать, стоил дороже кадра с
изменением.

**3. `Draw` не меняет состояние виджета — только рисует.** Особенно
опасно считать в `Draw` раскладку (позиции дочерних элементов, зоны
попадания мыши) и запоминать её там же: без вызова `Draw` эти координаты не
обновятся, и клик придёт по тому месту, где элемент был в последний
отрисованный кадр, а не там, где он визуально находится сейчас. Раскладку
считайте в `SetBounds`/`Layout`, а `Draw` пусть только читает готовый
результат.

**4. Любое изменение, влияющее на вид, обязано сопровождаться
`Invalidate()` (если меняется то, что внутри границ виджета) либо
`widget.InvalidateRect(r)` (если меняется область за пределами его границ —
это нужно оверлеям и всплывающим панелям, которые рисуют не там, где стоят).
Без этого движок не узнает, что кадр устарел, и в новой схеме это не просто
«лишний кадр простоит дольше», а «виджет не перерисуется вовсе», пока
что-то другое не заденет ту же область.**

Отдельно: виджет, который рисует **за своими границами** — тень от
`Elevation`, оверлей, всплывающая панель, — обязан сам расширить свою
заявленную область, потому что охватывающий прямоугольник поддерева, по
которому движок решает пропускать или не пропускать `Draw`, считается по
границам виджетов (`Bounds()`), а не по тому, что виджет реально закрашивает.
Виджет с невидимой тенью снаружи своих границ — верный кандидат на артефакт
после включения пропуска.

---

## Новые возможности

Раздел описывает функционал, добавленный после первой версии руководства:
привязки данных, стили/триггеры/шаблоны, команды, локализацию, валидацию,
`CollectionView`, виртуализацию и новые виджеты.

### Новые виджеты

#### NumericUpDown

Числовое поле со «спиннером» (аналог WPF Extended Toolkit). XAML-теги:
`<NumericUpDown>`, `<IntegerUpDown>`, `<DoubleUpDown>`.

```go
n := widget.NewNumericUpDown()
n.Min, n.Max, n.Step, n.Decimals = 0, 100, 0.5, 2
n.SetValue(9.99)
n.Value()                       // float64
n.OnChange = func(v float64) { ... }
```

```xml
<NumericUpDown Minimum="0" Maximum="100" Increment="1" Value="10"/>
<NumericUpDown Minimum="0" Maximum="10" Increment="0.5" Decimals="1" Value="3.5"/>
```

Управление: стрелки ▲/▼, колесо мыши, клавиши Up/Down при фокусе, прямой ввод с
фиксацией по Enter. Значение всегда зажимается в `[Min, Max]`.

#### VirtualizingItemsControl — UI-виртуализация

Список, материализующий виджеты только для видимого окна (+буфер). Подходит для
десятков тысяч строк.

```go
v := widget.NewVirtualizingItemsControl()
v.ItemHeight = 28
v.SetItemBuilder(func(item any, i int) widget.Widget {
    return widget.NewLabel(item.(*Person).Name, white)
})
v.SetItems(people)              // []any
v.BindCollectionView(view)      // авто-обновление из CollectionView
```

```xml
<VirtualizingItemsControl ItemHeight="24" Width="240" Height="320"
                          ItemsSource="{Binding People}">
    <VirtualizingItemsControl.ItemTemplate>
        <DataTemplate><TextBlock Text="{Binding Name}"/></DataTemplate>
    </VirtualizingItemsControl.ItemTemplate>
</VirtualizingItemsControl>
```

Требуется фиксированная высота строки `ItemHeight`. Есть встроенный скроллбар,
колесо мыши, перетаскивание ползунка.

#### WrapPanel / UniformGrid

```xml
<WrapPanel Spacing="6">
    <Button Content="One" Width="84" Height="30"/>
    <Button Content="Two" Width="84" Height="30"/>
</WrapPanel>
<UniformGrid Columns="3" Spacing="6">
    <Button Content="A"/><Button Content="B"/><Button Content="C"/>
</UniformGrid>
```

`WrapPanel` переносит дочерние элементы на новую строку; `UniformGrid`
раскладывает их по равным ячейкам (`Rows`/`Columns`).

#### GroupBox / Expander

```xml
<GroupBox Header="Группа" Width="240" Height="104">
    <StackPanel Padding="8" Spacing="6">
        <CheckBox Content="Опция 1"/>
        <CheckBox Content="Опция 2"/>
    </StackPanel>
</GroupBox>
<Expander Header="Развернуть" IsExpanded="True" Width="234" Height="104">
    <StackPanel Padding="8"><TextBlock Text="Содержимое"/></StackPanel>
</Expander>
```

`GroupBox` — рамка с заголовком; `Expander` — сворачиваемая по клику панель.
Содержимое обоих **отсекается по внутренней области** и не выходит за рамку.

#### Векторные фигуры

```xml
<Ellipse   Left="20" Top="20" Width="120" Height="80" Fill="#E06C75" Stroke="white" StrokeThickness="3"/>
<Rectangle Left="20" Top="20" Width="160" Height="80" Fill="#98C379" RadiusX="14"/>
<Line      X1="20" Y1="140" X2="200" Y2="200" Stroke="#E5C07B" StrokeThickness="4"/>
<Polygon   Points="280,130 340,210 220,210" Fill="#C678DD" Stroke="white"/>
<Polyline  Points="360,200 380,150 400,200" Stroke="#56B6C2" StrokeThickness="3"/>
```

Go-типы: `widget.Ellipse`, `widget.RectangleShape`, `widget.Line`,
`widget.Polygon`, `widget.Polyline`.

### Зрелость TextBox

- **MaxLength** — ограничение длины (ввод и вставка): `<TextBox MaxLength="20"/>`.
- **Undo/Redo** — `Ctrl+Z` отменить, `Ctrl+Y` или `Ctrl+Shift+Z` повторить.
- **Двойной клик** — выделение слова под курсором.

### Привязка данных ({Binding})

Загрузка XAML с DataContext делает привязки «живыми»:

```go
root, reg, scope, err := widget.LoadUIFromXAMLBindings(data, viewModel)
scope.SetDataContext(other)   // сменить источник
scope.Refresh()               // принудительно обновить UI
```

```xml
<TextBox  Text="{Binding Name, Mode=TwoWay}"/>
<TextBlock Text="{Binding Price, StringFormat=$%.2f}"/>
<TextBlock Text="{Binding Value, ElementName=slider1, Converter={StaticResource Pct}}"/>
```

- Режимы: `OneWay` (по умолчанию), `TwoWay`, `OneTime`.
- `INotifyPropertyChanged` (через `datagrid.PropertyNotifier`) обновляет UI.
- `IValueConverter`: `widget.RegisterValueConverter("Pct", conv)`.
- `ElementName` и `RelativeSource Self` — привязка к свойству другого элемента.
- `ItemsControl` с `ItemsSource="{Binding Coll}"` + `<DataTemplate>` живо
  перестраивается при изменении `ObservableCollection`.

### Ресурсы, стили, триггеры, шаблоны

```xml
<Canvas.Resources>
    <SolidColorBrush x:Key="Accent" Color="#0E639C"/>
    <Style x:Key="H1" TargetType="TextBlock">
        <Setter Property="FontSize" Value="15"/>
        <Setter Property="FontWeight" Value="Bold"/>
        <Style.Triggers>
            <DataTrigger Binding="{Binding Status}" Value="ERROR">
                <Setter Property="Foreground" Value="#F38BA8"/>
            </DataTrigger>
        </Style.Triggers>
    </Style>
    <ControlTemplate x:Key="Card">
        <Border Background="{TemplateBinding Background}" BorderBrush="#5A5A6A" BorderThickness="1">
            <ContentPresenter/>
        </Border>
    </ControlTemplate>
</Canvas.Resources>
```

Поддержаны `StaticResource`, неявные стили по `TargetType`, `BasedOn`,
`Trigger`/`DataTrigger`/`MultiTrigger`/`MultiDataTrigger`, `ControlTemplate` +
`ContentPresenter` + `TemplateBinding`, property-element синтаксис
(`<X.Background><SolidColorBrush/></X.Background>`), `LinearGradientBrush`.

### Команды и горячие клавиши

```go
vm.SaveCommand = widget.NewRelayCommand(func() { /* ... */ })
```

```xml
<Canvas.InputBindings>
    <KeyBinding Modifiers="Ctrl" Key="S" Command="{Binding SaveCommand}"/>
</Canvas.InputBindings>
<Button Content="Save" Command="{Binding SaveCommand}"/>
```

### Локализация: язык интерфейса ≠ раскладка клавиатуры

Это **две независимые оси**:

| Что | API | Назначение |
|---|---|---|
| Язык интерфейса | `SetLanguage` / `Language` / `AddLanguageListener` | язык **надписей**; управляет `Tr` и `{Loc}` |
| Раскладка клавиатуры | `SetLocale` / `Locale` (+ бейдж/applier ОС) | язык **ввода** текста |

Приложение может быть на русском, а ввод вестись на английском/китайском —
смена одного не меняет другое.

```go
widget.RegisterStrings("EN", map[string]string{"Greeting": "Hello", "Save": "Save"})
widget.RegisterStrings("RU", map[string]string{"Greeting": "Привет", "Save": "Сохранить"})
widget.SetFallbackLanguage("EN")
widget.SetLanguage("RU")
widget.Tr("Greeting")            // "Привет" (нет перевода → откат на EN → сам ключ)
widget.Trf("Count", 5)           // printf: "Count"="Элементов: %d" → "Элементов: 5"
widget.LoadStringsDir("i18n")    // ru.json, en.json → таблицы по имени файла
```

В XAML — markup `{Loc Key}`, обновляется **вживую** при `SetLanguage`:

```xml
<TextBlock Text="{Loc Greeting}"/>
<Button Content="{Loc Save}"/>
```

**Обратная совместимость:** обычные строки не трогаются — переводится только то,
что помечено `{Loc ...}`; без таблиц `Tr` возвращает сам ключ.

### Валидация (IDataErrorInfo)

Модель `DataContext` реализует `widget.DataErrorInfo`; привязка с
`ValidatesOnDataErrors=True` после записи спрашивает текст ошибки и переводит
виджет в состояние ошибки (красная рамка + подсказка).

```go
func (m *Form) DataError(prop string) string { // "" == корректно
    if prop == "Email" && !strings.Contains(m.Email, "@") {
        return "E-mail должен содержать «@»"
    }
    return ""
}
```

```xml
<TextBox Text="{Binding Email, Mode=TwoWay, ValidatesOnDataErrors=True}"/>
```

`scope.Validate() bool` перепроверяет все валидируемые привязки (удобно перед
сохранением формы).

### CollectionView (сортировка/фильтр/группировка)

```go
view := widget.NewCollectionView(people)          // *ObservableCollection или срез
view.SetFilter(func(it any) bool { return it.(*Person).Age >= 18 })
view.SetSort(
    widget.SortDescription{Property: "City"},
    widget.SortDescription{Property: "Age", Direction: widget.Descending},
)
view.SetGroup("City")
view.Items()    // текущее представление
view.Groups()   // []CollectionViewGroup{Name, Items}
```

`ItemsControl` и `VirtualizingItemsControl`, привязанные к `CollectionView`,
перестраиваются при изменении фильтра/сортировки/группы.

### Подсказки, курсоры, индикатор локали

- `ToolTip="..."` на любом виджете; задержка/вкл-выкл через
  `eng.SetTooltipsEnabled`/`SetTooltipDelay`.
- Курсоры мыши: виджеты реализуют `Cursor() widget.Cursor` (TextBox → I-beam,
  GridSplitter → resize); `eng.CursorAt(x, y)`.
- Индикатор раскладки в окнах/диалогах — свойство `ShowLocaleIndicator`,
  контекстное меню переключения.

### Преднастроенные темы и стиль контролов (Win10/Win11/Win2000/Mac)

Тема — не только палитра: поле `Theme.Style` (`widget.ThemeStyle`) задаёт
**форму** контролов:

- `ControlCorner` — радиус скругления Button/TextBox/ComboBox/ProgressBar
  (0 — прямые; Win11 = 6, Mac = 8); явный `CornerRadius` из XAML приоритетнее;
- `Classic3D` — классика Win2000: прямые углы, выпуклые bevel-кнопки
  (утопленные при нажатии), утопленные поля ввода и чекбоксы, выпуклая кнопка
  со стрелкой в ComboBox, «блочный» ProgressBar, без hover-подсветки.

```go
widget.ThemeNames()            // ["Win10 Dark", ..., "Win2000", "Mac"]
eng.SetTheme(widget.ThemeByName("Win2000"))
widget.CurrentThemeStyle()     // стиль активной темы (для своих виджетов)
```

Конструкторы: `Win10DarkTheme/Win10LightTheme/Win11DarkTheme/Win11LightTheme/
Win2000Theme/MacTheme` (`DarkTheme`/`LightTheme` — это Win10). Mac: акцент
#007AFF и зелёный ToggleSwitch; Win2000: серебро #D4D0C8 + navy.

Семантика `SetTheme`: фоны контейнеров тоже следуют теме — непрозрачный фон
`Canvas` перекрашивается в `WindowBG`, `Panel` — в `PanelBG` (явные XAML-цвета
заменяются, единообразно с остальными виджетами).

`window.Window.Close()` программно закрывает нативное окно (Run() вернётся) —
для пункта меню «Файл → Выход».

### Рендер по запросу (on-demand) и инвалидация

Рендер по запросу — **режим по умолчанию** (с v3.5): кадры рендерятся только
когда UI изменился, причём перерисовывается и диффается только изменившаяся
область (авто-damage). В простое CPU почти нулевой; hover/ввод стоят
микросекунды вместо полного кадра. Прежнее поведение «рендер каждый тик»:
`eng.SetRenderOnDemand(false)`.

```go
eng.Invalidate()        // пометить весь кадр изменившимся (дёшево, атомарно)
eng.InvalidateRect(r)   // заявить изменённую область — отрисовка и diff кадра
                        // ограничатся ею
eng.RenderCount()       // сколько кадров реально отрендерено (диагностика)
```

Отслеживается автоматически: сеттеры виджетов (`SetText`, `SetValue`,
`SetChecked`, hover/press/focus, `SetBounds` при перемещении/ресайзе —
виджеты самоинвалидируются при фактическом изменении), события ввода и фокус,
SetRoot/SetTheme/SetResolution/модалки, слой данных (Refresh биндингов,
`{Loc}`, live-коллекции, смена локали/языка), мигающая каретка
(`widget.Animated`) и tooltip.

ВАЖНО: прямые записи в экспортированные поля виджетов (`btn.Text = "..."`)
движку не видны — после них вызывайте `btn.Invalidate()` (метод есть у всех
виджетов через Base) либо `eng.Invalidate()`. Кастомные виджеты со своим
визуальным состоянием должны вызывать `Invalidate()` при его изменении.

Блокировки: кадр больше не держит общий мьютекс движка — `SetRoot`/события не
блокируются рендером; структурные операции (SetResolution, RegisterFont*,
SetTheme) сериализуются с кадром отдельным внутренним мьютексом.

### HiDPI

Виджеты живут в ЛОГИЧЕСКИХ пикселях (как WPF DIP), буферы кадра — в
физических (логические × масштаб). Текст растеризуется в честном физическом
размере (чётче, а не растянут), скругления и AA-фигуры масштабируются гладко.

```go
eng.SetScale(2.0)       // 200% (или 1.25/1.5/...)
eng.CanvasSize()        // логический размер (система координат виджетов)
eng.PhysicalSize()      // физический размер кадров/тайлов
```

Кадры `Frames()` и события `SendMouse*` — в физических пикселях (события
переводятся в логические внутри движка).

Нативное окно (`window.Run`) определяет масштаб автоматически: на Windows
включается per-monitor DPI awareness (v2) + обработка WM_DPICHANGED при
переносе окна между мониторами; на X11/macOS масштаб задаётся переменной
окружения `HEADLESS_GUI_SCALE=1.5` (автоопределение — в планах).

### Accessibility (семантическое дерево)

Движок строит семантический снапшот UI — роли, имена, значения и состояния
всех видимых виджетов:

```go
tree := eng.AccessibilityTree() // *widget.AccessNode, сериализуем в JSON
```

Роли встроенных виджетов выводятся автоматически (button, checkbox,
combobox, textinput, slider, tablist, …); состояния — checked/selected/
disabled/focused/expanded/modal/inactive. Кастомный виджет может задать
свою семантику, реализовав `widget.Accessible`:

```go
func (w *MyWidget) AccessInfo() widget.AccessInfo {
    return widget.AccessInfo{Role: widget.RoleButton, Name: "Моя кнопка"}
}
```

Применения: side-channel семантики в стриминговых сценариях (клиент
озвучивает UI скринридером), поиск элементов в автотестах по роли/имени.
Платформенные мосты (UI Automation / AT-SPI / NSAccessibility) — в планах;
клавиатурная навигация (Tab/Shift+Tab, Enter/Space) работает уже сейчас.

### Шрифты

В комплекте восемь свободных семейств, все с файлами лицензий (полный список,
лицензии и обязанности при распространении — `assets/fonts/README.md`):

- **Roboto** (по умолчанию), **Open Sans**, **Inter** — интерфейсные гротески;
- **Liberation Sans** и **Liberation Mono** — метрически совместимы с Arial и
  Courier New: та же ширина строки при том же кегле. Нужны там, где макет
  пришёл из Windows и должен совпасть по ширине;
- **DejaVu Sans** и **DejaVu Sans Mono** — самое широкое покрытие символов:
  кириллица, греческий, псевдографика, стрелки, ✓ ✗ ⚠. Движок берёт их ещё и
  как фолбэк на машине, где системных шрифтов нет вовсе;
- **Go Regular** — встроенный шрифт движка.

Авто-загрузка из `assets/fonts/`, плюс цепочка фолбэка системных шрифтов для
символов/эмодзи (нет «тофу»).

Начертания Bold/Italic здесь — самостоятельные именованные шрифты, а не
варианты веса: `FontWeight`/`FontStyle` переключают только встроенные
Go-шрифты. Жирный Liberation берётся как `FontFamily="LiberationSans-Bold"`.

```go
eng.SetDefaultFont("Inter")
eng.RegisterFontFile("Roboto", path)
eng.RegisterFontDir("my/fonts")
eng.AvailableFonts()
```

Папка `assets/fonts` ищется **относительно рабочего каталога процесса**.
Программа, установленная куда-нибудь в Program Files или запущенная службой,
там ничего не найдёт и молча останется на встроенном Go Regular. Чтобы шрифты
ехали внутри исполняемого файла — `RegisterFontFS`:

```go
//go:embed assets/fonts
var fontsFS embed.FS

if err := eng.RegisterFontFS(fontsFS, "assets/fonts"); err != nil {
    log.Fatal(err) // опечатка в //go:embed — искать её по пропавшему шрифту дорого
}
eng.SetDefaultFont("Roboto")
```

```xml
<TextBlock FontFamily="Roboto" FontSize="16"/>
```

---

### Стандартные диалоги (MessageBox, ввод, прогресс, файлы)

Полный набор модальных диалогов, отрисованных самим движком — работают в
headless/стриминге (файловые показывают ФС процесса/сервера), темизируются
и локализованы (ключи `dlg.*`, EN/RU встроены, живое переключение):

```go
mb := widget.NewMessageBox(eng)
mb.ShowInfo("", "Документ сохранён.")                 // значок i, заголовок по severity
mb.ShowQuestion("", "Сохранить изменения?", func(r widget.MessageBoxResult) { ... })
id := mb.ShowInput("", "Имя:", "default", validate, onResult); id.SetHint("подсказка")
pd := mb.ShowProgress("Копирование", "file.jpg", onCancel)
pd.SetDetail("34 из 120 · 61 МБ/с"); pd.SetProgress(0.28) // или SetIndeterminate(true)
mb.ShowOpenFile(widget.FileDialogOptions{Filters: ...}, func(path string, ok bool) { ... })
mb.ShowSaveFile(widget.FileDialogOptions{InitialName: "a.txt"}, ...)  // компактная форма
mb.ShowPickFolder(widget.FileDialogOptions{}, ...)
```

Горячие клавиши: Enter — кнопка по умолчанию, Escape/✕ — отмена, Ctrl+C в
MessageBox копирует содержимое в формате Windows (разделители `---`).
Открытый браузер файлов: панель мест (Places в опциях + домашняя/диски),
кликабельный breadcrumb-путь, колонки Имя/Размер/Изменён, фильтры.

### Многострочный TextBox

`widget.NewTextBox(placeholder)` — редактор: перенос по словам
(`Wrap=false` — горизонтальный скролл), вертикальный скролл (колесо,
PgUp/PgDn), выделение мышью и Shift+навигацией, Ctrl+стрелки по словам,
Ctrl+Home/End, Ctrl+A/C/X/V, Ctrl+Z/Y, двойной клик — слово, контекстное
меню, `ReadOnly`. В XAML строится тегом
`<TextBox AcceptsReturn="True" TextWrapping="Wrap"/>` (без этих атрибутов
тег по-прежнему создаёт однострочный TextInput). Компоновка считается
через `widget.MeasureUIText`, поэтому каретка/скролл работают headless.

### Вьювер в браузере (output/webstream)

UI любого приложения на движке можно показать в браузере без пересборки
клиента:

```go
srv := webstream.New(eng)          // единственный потребитель eng.Frames()
go srv.Run()
http.ListenAndServe(":8091", srv)  // "/" — встроенный вьювер, "/ws" — поток
```

WebSocket-сервер написан с нуля (RFC 6455, без зависимостей). Протокол:
бинарные сообщения `init` (размер холста) и батчи тайлов (PNG на тайл с
u16-заголовком x/y/w/h); сервер держит композит экрана и отдаёт новому
клиенту полный keyframe, дальше — только дельты; медленные клиенты
пропускают кадры. Ввод возвращается JSON-сообщениями (мышь/колесо/клавиши;
`e.keyCode` браузера совпадает с `widget.KeyCode`). Демо:
go run ./cmd/webshowcase → http://localhost:8091 — ВСЯ витрина виджетов в
браузере: та же разметка showcase.xaml, те же вкладки, темы и локализация, но
ни одного окна ОС. Минимальный пример — `go run ./cmd/webdemo`.

Сервер отдаёт ещё две вспомогательные точки: `/stats` (JSON: зрители, кадры,
тайлы, трафик) и `/snapshot.png` (текущий кадр целиком — удобно для
документации и мониторинга).

### Нативные модальные окна и попапы (v3.10)

С v3.10 модальные диалоги и всплывающие оверлеи (dropdown, контекстные/трей-
меню) выносятся в **собственные окна ОС**, а не рисуются внутри главного окна:

- **Модальный диалог** открывается отдельным окном. Поэтому он может быть
  больше главного окна и перетаскиваться за его пределы. Ничего настраивать не
  нужно — `window.Window` включает хост в `Run()`. Диалог поверх диалога
  (файловый из обычного) образует стек окон.
- **Dropdown и меню** раскрываются в маленьком окне у нужной точки и **не
  обрезаются** границей главного окна.
- Работает нативно на **Windows (Win32)** и **Linux (X11)**. На **Wayland,
  macOS и в headless** — прежний фолбэк: всё рисуется в холст (функционально
  идентично, просто в пределах окна).

```go
dlg := widget.NewDialog("Настройки", 1000, 700) // может быть больше окна
dlg.CornerRadius = 8                              // скругление окна диалога
eng.ShowModal(dlg)
```

### Трей и уведомления (Windows)

`window.Window` умеет работать с областью уведомлений. Методы можно вызывать до
`Run()` (состояние применится при создании окна) или из обработчиков UI. На
платформах кроме Windows — вежливый no-op (методы возвращают ошибку/ничего).

```go
win := window.New(eng, "My App")

// Иконка в трее (масштабируется до системного размера, прозрачность из альфы).
win.SetTrayIcon(iconImg, "My App")

// Контекстное меню по правому клику (наше widget.PopupMenu, у курсора).
m := widget.NewPopupMenu()
m.AddItem("Показать", func() { win.RestoreFromTray() })
m.AddItem("Свернуть", func() { win.HideToTray() })
m.AddItem("Выход",    func() { win.Close() })
win.SetTrayMenu(m)

// Клики по иконке (иначе дефолт: двойной левый клик восстанавливает окно).
win.SetOnTrayClick(func(btn widget.MouseButton, dbl bool) { /* ... */ })

// Balloon-уведомление (значок по severity). Требует заданной иконки трея.
win.ShowBalloon("Готово", "Задача выполнена", widget.SeverityInfo)
win.SetOnBalloonClick(func() { win.RestoreFromTray() })

// Свернуть в трей / восстановить.
win.HideToTray()
win.RestoreFromTray()

win.Run()
```

**Превью в панели задач.** Миниатюра при наведении на кнопку приложения и Aero
Peek показывают живое содержимое окна (раньше было чёрным). Дополнительный
iconic-путь DWM включается переменной окружения `HEADLESS_GUI_ICONIC_PREVIEW=1`
(по умолчанию не требуется).

**Трей из XAML.** Иконку, подсказку и меню трея можно объявить прямо в корневом
`<Window>` — без императивных `SetTrayIcon`/`SetTrayMenu`:

```xml
<Window Title="Моё приложение" TrayIcon="icons/app.svg" TrayTooltip="Моё приложение">
  <TrayMenu Name="trayMenu">
    <MenuItem Text="Показать"/>
    <Separator/>
    <MenuItem Text="Выход"/>
  </TrayMenu>
  <!-- …обычный контент… -->
</Window>
```

`TrayIcon` — путь относительно XAML-файла: `.png`/`.jpg` декодируется как есть,
`.svg` растеризуется 32×32 (свои цвета SVG сохраняются, `currentColor` → цвет
текста темы; трей намеренно не темизируется). `TrayTooltip` по умолчанию равен
`Title`. `<TrayMenu>` — единственный дочерний тег окна с пунктами `<MenuItem>` и
разделителями `<Separator/>`; хранится полем `Window.TrayMenu`, а не ребёнком
дерева. Обработчики вешаются в коде: найдите меню по `Name` и используйте
`PopupMenu.OnSelect(idx, text)`. **Приоритет у кода:** если приложение вызвало
`SetTrayIcon`/`SetTrayMenu` до `Run()`, XAML-декларация не применяется (заполняет
только незаданное). Balloon-уведомления, `SetOnTrayClick`, `HideToTray` в XAML не
выражаются — только в коде.

### SplitPanel — две панели с разделителем

Контейнер `SplitPanel` держит двух детей (первые два `AddChild` — First/Second) и
раскладывает их по обе стороны перетаскиваемой полосы. Позиция хранится как доля
`0..1`, поэтому ресайз окна сохраняет соотношение. Полосы можно вкладывать друг
в друга (панель-в-панель).

```go
sp := widget.NewSplitPanel(widget.OrientationHorizontal) // слева/справа; Vertical — сверху/снизу
sp.SplitterSize = 6
sp.Position = 0.35        // доля под First
sp.MinFirst, sp.MinSecond = 120, 200
sp.OnPositionChanged = func(pos float64) { /* обновить подпись позиции */ }

sp.AddChild(leftPanel)    // First
sp.AddChild(rightPanel)   // Second
sp.SetBounds(image.Rect(0, 0, 800, 500))

// Управление коллапсом (аналог двойного клика по полосе):
sp.Collapse(); sp.Expand(); sp.ToggleCollapse(); _ = sp.IsCollapsed()
```

Hover над полосой даёт курсор изменения размера (`SizeWE`/`SizeNS`), drag ЛКМ
двигает границу с клэмпом по `MinFirst`/`MinSecond`, двойной клик по полосе
сворачивает/разворачивает First. Цвет полосы следует за темой
(`Theme.SplitterBG`/`SplitterHoverBG`). SplitPanel зарегистрирован в
`HasOwnLayout`, так что вложение в Canvas/DockPanel не «двоит» сдвиг.

XAML (первые два дочерних элемента — панели):

```xml
<SplitPanel Orientation="Horizontal" Position="0.35" SplitterSize="6"
            MinFirst="120" MinSecond="200">
  <Panel Background="#1E1E1E"/>   <!-- First -->
  <Panel Background="#252526"/>   <!-- Second -->
</SplitPanel>
```

Для ресайза ячеек `Grid` по-прежнему используйте `GridSplitter`.

### SVG-иконки

Виджет `SVGIcon` рисует векторную иконку из подмножества SVG, растеризуя её под
размер bounds с сохранением пропорций. Без явного цвета иконка перекрашивается
под цвет текста темы — удобно для панелей инструментов и меню.

```go
ic := widget.NewSVGIcon()          // цвет = Theme.LabelText, пока не задан явно
ic.SetSVGFile("assets/menu.svg")   // или ic.SetSVG(data []byte)
ic.SetColor(color.RGBA{0xFF, 0x33, 0x66, 0xFF}) // явный цвет = currentColor
ic.SetTint(true)                   // перекрасить ВЕСЬ контент в Color (монохром)
ic.SetBounds(image.Rect(8, 8, 32, 32))
```

- `fill="currentColor"` в SVG заменяется на `Color` виджета.
- `Tint=true` перекрашивает весь контент в `Color` (монохромный режим);
  `Tint=false` красит только `currentColor`.
- Поддержано: `path` (все команды, включая дуги и smooth-кривые),
  `rect`/`circle`/`ellipse`/`line`/`polyline`/`polygon`, трансформы групп,
  `fill`/`fill-rule` (nonzero + even-odd)/`fill-opacity`, атрибут `style`.
- Ограничения: нет градиентов, `clipPath` и `text`; обводка (stroke) упрощённая.

Пакет `widget/svg` доступен и напрямую: `svg.Parse(data)` / `svg.ParseFile(path)`
→ `*svg.Document` с методом `RasterizeCached(w, h, current, tint)`.

XAML (`Source` резолвится относительно каталога XAML-файла):

```xml
<SVGIcon Source="icons/menu.svg" Color="#FF3366" Tint="True"/>
<SVGIcon Source="icons/folder.svg"/>   <!-- без Color — цвет текста темы -->
```

### Плавный / инерционный скролл

Помимо целых «тиков» колеса движок принимает **точные пиксельные дельты** — так
тачпады и колёса высокой точности скроллят плавно.

```go
// Точная дельта в физических пикселях окна/кадра (dy>0 — вниз, dx>0 — вправо).
eng.SendMouseWheelPixels(x, y, dx, dy)
```

Событие всплывает от глубокого виджета к корню; первый, реализующий
`OnMouseWheelPixels(x, y int, dx, dy float64) bool` и вернувший `true`, поглощает
дельту. `ScrollView` запускает инерцию-«маховик» (импульс скорости, затухание на
часах движка — без горутин); любой клик/press гасит бросок, в `Classic3D` скролл
мгновенный. `ListView` и `TextBox` прокручиваются попиксельно с субпиксельным
накоплением и отдают дельту родителю на краях. Если точную дельту никто не принял,
движок синтезирует эквивалентные тики — старый путь `MouseWheelUp`/`Down` цел.

По платформам: Win32 (`WM_MOUSEWHEEL`) и Wayland (`wl_pointer.axis`) дают точные
пиксели; X11 остаётся на тиках (кнопки 4/5), macOS-колесо пока не эмитится. В
headless всё работает через `SendMouseWheelPixels`.

### Drag & Drop файлов из ОС

Приложение может принимать файлы, перетащенные из проводника/файлового менеджера
в окно.

```go
win := window.New(eng, "My App")
win.SetOnFilesDropped(func(paths []string, x, y int) {
    // paths — абсолютные пути; x, y — ЛОГИЧЕСКИЕ координаты точки сброса.
    for _, p := range paths { open(p) }
})
win.Run()
```

Параллельно событие уходит в движок (`eng.SendFilesDropped(x, y, paths)`, где
`x,y` — физические пиксели) и доставляется виджету под точкой, реализующему
интерфейс приёмника — это даёт headless-симметрию и тестируемость:

```go
type FileDropTarget interface {
    OnFilesDropped(x, y int, paths []string) bool // true — поглотить (прекратить всплытие)
}
```

По платформам: **Win32** (`WM_DROPFILES`) и **X11** (XDND v5) — полностью;
**Wayland** (`wl_data_device`) — каркас, требует проверки на живой сессии;
macOS — нет. В headless маршрутизация к `FileDropTarget` тестируется через
`SendFilesDropped` без окна.

### Цветные эмодзи

Текстовый тракт рендерит цветные глифы автоматически — отдельного API не нужно,
достаточно эмодзи в строке любого виджета (`👍🎉🚀🔥`). Поддержаны COLRv0
(плоские CPAL-слои), COLRv1 (граф paint с трансформами и сплошными заливками) и
CBDT/sbix (PNG-битмапы, напр. Noto Color Emoji). Цветные глифы кэшируются отдельно
от монохромных масок.

Ограничения (честно): BMP-символы ниже U+1F000 остаются монохромными;
региональные флаги (буквенные лигатуры) — известный пробел; градиенты COLRv1
аппроксимируются средним цветом. Работает одинаково headless и в окне.

> **Лицензии.** Движок не встраивает и не распространяет эмодзи-шрифты — глифы
> берутся из шрифта, уже установленного в ОС пользователя (Segoe UI Emoji на
> Windows, Apple Color Emoji на macOS, Noto Color Emoji на Linux — если стоит),
> ровно как и системные фолбэк-шрифты для арабского/иврита/тайского. В строках
> лежат обычные Unicode-кодпоинты, а не картинки. Поэтому дополнительных
> лицензионных обязательств у проекта нет. Если вам нужно гарантированное
> отображение эмодзи на всех платформах — забандльте свободный шрифт (напр.
> Noto Color Emoji, OFL) в свой продукт и приложите его лицензию.

### Докинг-панели (Toolbox)

`DockManager` + `DockPane` — зона докинга в стиле Visual Studio: документная
область по центру (`Center`) и до четырёх сторон (Left/Top/Bottom/Right), на
каждой из которых может быть стопка панелей `DockPane`. Менеджер сам умеет:
ресайз сторон перетаскиванием кромки, табы стопки (2+ панели на одной
стороне), auto-hide (сворачивание в ярлык у края), drag&dock (перетащи
панель за титлбар — появятся направляющие; отпусти над стрелкой — панель
пришвартуется, мимо — станет плавающей).

```go
mgr := widget.NewDockManager()
mgr.SetBounds(image.Rect(0, 0, 1000, 600))

tools := widget.NewDockPane("tools", "Обозреватель", widget.NewListView("Файл.txt"))
mgr.AddPane(tools, widget.DockLeft)
mgr.SetSideSize(widget.DockLeft, 220)

props := widget.NewDockPane("props", "Свойства", widget.NewWin10Label("—"))
mgr.AddPane(props, widget.DockRight)

mgr.SetCenter(editor) // документная область

// Состояния панели: делегируют менеджеру, если панель ему принадлежит.
tools.Unpin()  // Docked → AutoHidden (ярлык у края)
tools.Pin()    // обратно
props.Float()  // Docked/AutoHidden → Floating (плавающая поверх центра)
props.Dock(widget.DockRight) // обратно в стопку

tools.OnStateChanged = func(p *widget.DockPane) {
    log.Println(p.Title, "→", p.State()) // docked/autohidden/floating/closed
}
```

Раскладку можно сохранить и восстановить (JSON, панели матчатся по `ID`):

```go
data := mgr.SaveLayout()
// ...
_ = mgr.RestoreLayout(data)
```

**Плавающие панели.** По умолчанию `Float()` включает виджетную плавающую
панель прямо в холсте (drag/resize мышью, headless-тестируемо). Хук
`DockPane.OnFloatNative func(p *DockPane)`, если задан, отдаёт отрыв
нативному ОС-окну (`window/**`) вместо виджетного floating — на момент
написания этого раздела ни один платформенный бэкенд его не назначает
(состояние "в процессе"); при отсутствии хука работает фолбэк в холсте.

XAML:

```xml
<DockManager Background="#232338">
  <DockPane Id="tools" Title="Инструменты" Side="Left" Size="220" State="Docked">
    <ListView><ListViewItem Content="item 1"/></ListView>
  </DockPane>
  <DockPane Id="props" Title="Свойства" Side="Right" Size="200"/>
  <DockPane Title="Вывод" Side="Bottom" Size="120" State="AutoHidden">
    <TextBlock Text="log..."/>
  </DockPane>
  <DockContent>
    <TextBox Text="документная область"/>
  </DockContent>
</DockManager>
```

`<DockManager>` также принимает `NativeFloating="True"` — декларацию нативного
отрыва панелей в отдельные ОС-окна: `window.Window.Run()` обходит дерево и, если
приложение не вызвало `EnableDockFloating(dm)` само, включает отрыв для первого
такого менеджера. Явный вызов `EnableDockFloating` имеет приоритет; при
нескольких `NativeFloating`-менеджерах включается только первый (хост держит
один), остальные логируются. В headless и на неподдерживаемых бэкендах атрибут
без эффекта (floating остаётся виджетным оверлеем в холсте).

`<DockPane>`: `Id` (если не задан — слаг от `Title`), `Title`, `Side`
(Left/Top/Bottom/Right, по умолчанию Left), `Size` в px (→ `SetSideSize` для
своей стороны — несколько панелей одной стороны просто делят один размер,
побеждает последний `Size`), `State` (Docked по умолчанию; AutoHidden сразу
после добавления вызывает `Unpin()`; Floating/Closed — по желанию). Контент —
первый дочерний виджет панели. `<DockContent>` — не виджет, маркер: его
единственный ребёнок становится центром (`SetCenter`). Оба тега вне
`<DockManager>` игнорируются.

Смотри вкладку «Докинг» в `cmd/showcase` — пример с тремя панелями и кнопками
сохранения/восстановления раскладки.


### Темы как данные и рабочий стол (v3.14.0)

Пакет `theme/` описывает облик приложения **данными**, а не кодом, а пакет
`desktop/` даёт из этих данных собранную системную панель задач.

#### Профиль темы

Профиль — плоские таблицы токенов: цвета, метрики, признаки, шрифты, иконки,
анимации, презентеры и стили компонентов.

```go
p := theme.NewProfile("mytheme")
p.Parent = theme.ProfileWindows11        // наследование: берём всё, меняем нужное
p.SetColor("accent", theme.RGB(200, 60, 60)).
    SetMetric("taskbar.height", 44).
    SetFlag("taskbar.centered", true)
p.SetStyle("taskbutton", "", theme.StateHover, theme.StyleDelta{
    Fill: theme.C(theme.RGBA(255, 255, 255, 24)), Corner: theme.N(6),
})

m := theme.NewManager()
theme.RegisterBuiltinProfiles(m)          // Windows 11/10/2000, macOS + тёмные
m.RegisterTheme(p)
m.SetTheme("mytheme")                     // смена на лету, подписчики уведомлены
```

Стиль спрашивается по тройке «компонент, часть, состояние»; отсутствующее
состояние падает на покой, отсутствующая часть — на компонент, отсутствующий
компонент — на общий вид темы. `GetStyle` возвращает указатель в готовую
таблицу и не выделяет памяти — его можно звать из `Draw`.

```go
s := m.GetStyle("taskbutton", "", theme.StateHover|theme.StateActive)
```

Состояния — битовая маска, но таблица хранит по одному стилю на состояние:
`Dominant()` сводит маску к главному (Disabled > Pressed > Active > Hover >
Focused). Поэтому шесть записей на компонент, а не тридцать две.

Тёмная разновидность объявляет **только отличия** — обычно три-четыре цвета:
заливка и текст стилей берутся из плоских токенов `surface` и `text`, если
стиль их не задал.

Тема грузится и из JSON — без единой правки движка:

```go
res, err := theme.LoadTheme(file)   // res.Profile, res.Warnings
m.RegisterTheme(res.Profile)
```

#### Стекло, тени и скруглённый клип

Стиль темы умеет просить то, что раньше приходилось рисовать руками:

```go
p.SetStyle("taskbar", "", theme.StateNormal, theme.StyleDelta{
    Backdrop:  &theme.BackdropSpec{Mode: theme.BackdropBlur, Radius: 30,
                                   Tint: theme.RGBA(243, 243, 243, 200)},
    Corner:    theme.N(8),
    Elevation: theme.N(12),                       // мягкая тень
    Shadow:    theme.C(theme.RGBA(0, 0, 0, 70)),
})
```

Размытие подложки берёт готовые пиксели холста, уменьшает их вчетверо,
размывает разделимым box-blur (стоимость не зависит от радиуса) и возвращает
обратно. `Canvas.SetRoundClip` обрезает по скруглённому контуру, а не по
охватывающему прямоугольнику.

#### Панель задач и её компоненты

```go
bar := desktop.NewTaskbar(m)
bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
bar.AddItem(desktop.SlotApps, desktop.NewApplicationArea(m, catalog, windows))
bar.AddItem(desktop.SlotTray, tray)      // desktop.NewSystemTray(m)
bar.AddItem(desktop.SlotTray, desktop.NewClock(m, desktop.SystemClock{}))
bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
```

Компоненты не лезут в систему: данные приходят через интерфейсы
`WindowModel`, `AppCatalog`, `SystemStatus`, `Notifications`, `Clock`, которые
реализует потребитель. Движок поставляет их фейки (`FakeWindowModel`,
`StaticAppCatalog`, `FakeSystemStatus`, `FakeNotifications`, `FakeClock`) —
на них держатся тесты и демонстрация.

Всплывающие панели — меню «Пуск», календарь, быстрые настройки, центр
уведомлений — рисуются оверлеем движка, поэтому их можно вынести в отдельные
окна ОС (`engine.SetPopupSink`) и они не обрезаются окном оболочки:

```go
menu := desktop.NewStartMenu(m, catalog)
menu.Screen = image.Rect(0, 0, w, h)
startBtn.OnClick = func() { menu.Toggle(startBtn.Bounds()) }
root.AddChild(menu)                       // в дереве — иначе оверлей не найдут
```

Панель занимает всю ширину, переживает `SetBounds` с другим разрешением и
уважает масштаб холста. Компонент, которому не хватило места, деградирует
предсказуемо: кнопки окон сжимаются до значков, значки трея прячутся за
кнопку-шеврон.

Панели, стоящие одна над другой — в Windows так показывают уведомления над
календарём, — объединяются в группу: они считают прямоугольники друг друга
своими и закрываются вместе. Без группы клик по числу календаря пришёлся бы
вне центра уведомлений, и тот закрылся бы:

```go
desktop.NewFlyoutGroup(calendar.Flyout, notifications.Flyout)
```

#### Предпросмотр окна

Наведение на кнопку окна показывает миниатюру этого окна, прижатую к кнопке:

```go
preview := desktop.NewWindowPreview(m, windows)
preview.Screen = image.Rect(0, 0, w, h)
preview.Track(area)   // area — desktop.NewRunningApplications(...)
root.AddChild(preview)
```

Миниатюру отдаёт сама модель окон, если умеет:

```go
// Необязательный интерфейс: модель без него просто не получает предпросмотра.
func (m *MyWindows) Preview(id desktop.WindowID, max image.Point) image.Image
```

Спрашивают её **по требованию**, а не полем в `WindowInfo`: модель
перестраивается на каждое изменение состава окон и на каждую смену фокуса, а
показывается в один момент ровно одна миниатюра. Свёрнутое окно отдаёт
последний снимок, сделанный пока оно было видно, — Windows ведёт себя так же.

Частота обращения задаётся темой (`preview.refresh`), и это требование, а не
настройка: живая миниатюра в каждом кадре вернула бы непрерывный поток
кадров. Прочие ключи — `preview.width`, `preview.height`, `preview.pad`,
`preview.header`, `preview.delay.open`, `preview.delay.close`; флаг `preview`
выключает предпросмотр целиком (у Windows 2000 его не было, у дока macOS свой
механизм).

#### Презентеры: тема меняет не только цвет

Токенами описывается палитра и геометрия, но не форма. Док macOS — не
перекрашенная полоса кнопок: значки крупные, стоят по центру, тот, что под
курсором, увеличивается и раздвигает соседей. Поэтому профиль вправе принести
с собой **презентер** — чужую отрисовку и раскладку известного ему компонента:

```go
p.Presenters["runningapps"] = "dock"      // в профиле macOS
```

Компонент остаётся один, его тесты на поведение проходят для обеих тем;
меняется только тот, кто рисует. Свой презентер регистрируется
`desktop.RegisterPresenter(name, p)`.

Демонстрация: `go run ./cmd/desktopdemo` — пять обликов одних и тех же
компонентов переключаются кнопками без перезапуска.


#### Радиальный градиент

Линейный градиент описывает переход вдоль оси, и подсветку под значком дока им
не выразить: там свет расходится кругом от точки.

```go
p.SetStyle("dock", "", theme.StateHover, theme.StyleDelta{
    Gradient: []theme.GradientStop{
        {Pos: 0, Color: theme.RGBA(255, 255, 255, 150)},
        {Pos: 1, Color: theme.RGBA(255, 255, 255, 0)},
    },
    GradientKind:   theme.GK(theme.GradientRadial),
    GradientRadius: theme.N(1.1),     // доля половины большей стороны
})
```

Центр (`GradientCenterX/Y`) и радиус задаются долями области, а не пикселями:
одна и та же подсветка ложится и под значок 24 точки, и под 64. Градиент
заменяет заливку, когда задан. Напрямую доступны и `widget.DrawRadialGradient`
с `widget.DrawLinearGradient`.

#### Тема на поддерево

Тема была одна на всё приложение: `ApplyGlobalTheme` пишет в общие переменные,
`Engine.SetTheme` обходит всё дерево. Оболочке удалённого стола нужно другое —
окно гостя в его теме рядом со своим интерфейсом.

```go
scope := widget.NewThemeScope(widget.Win2000Theme())
scope.SetBounds(image.Rect(0, 0, 400, 300))
scope.AddChild(button)          // ребёнок сразу оформляется темой области
root.AddChild(scope)

eng.SetTheme(widget.DarkTheme()) // область останется классической
```

Область раздаёт тему своему поддереву и защищает его от глобальной смены:
`ApplyThemeTree` в неё не заходит. Форма — фаски, скругления, признак
классики — читается из общей переменной прямо в `Draw`, поэтому на время
отрисовки поддерева она подменяется и возвращается через `defer`. Области
вкладываются друг в друга: внутренняя возвращает стиль ВНЕШНЕЙ, а не сбрасывает
его в общий. `NewThemeScope(nil)` — обычный контейнер, глобальная тема доходит
до детей.


### Конвейер кадра

Кадр рождается и уходит потребителю; здесь — то, что движок о нём сообщает и
кто задаёт темп.

#### Пропуск поддеревьев

Раньше кадр обходил дерево целиком: damage работал ножницами на уровне
холста, лишние пиксели отбрасывались, но обход и вызовы `Draw` случались всё
равно. Теперь ветка, не задевающая ни одной изменившейся области, не
рисуется вовсе.

```go
eng.SetSubtreeCulling(false) // вернуть прежний полный обход
```

Отсюда контракт отрисовки (см. «Свой виджет» → «Контракт отрисовки»): `Draw`
не гарантирован каждый кадр. Виджет, рисующий за своими границами, объявляет
поле:

```go
func (w *MyWidget) DrawMargin() int { return 12 } // тень, свечение
```

#### Не рисовать перекрытое

Движок рисует снизу вверх. Окно под другим окном раньше отрисовывалось
целиком, а потом закрашивалось; на рабочем столе, где окна лежат стопкой,
это кратные затраты на каждый полный кадр.

Виджет может объявить, что закрывает собой:

```go
func (w *MyWidget) OpaqueRegion() []image.Rectangle {
    if w.Background.A < 255 {
        return nil // сквозь полупрозрачное видно нижних — их надо рисовать
    }
    return []image.Rectangle{w.Bounds()}
}
```

Обход детей идёт сверху вниз, копит объявленную площадь и пропускает
поддеревья, целиком в неё попавшие. Окно и панель это уже умеют: сплошной
непрозрачный фон закрывает свои границы, скруглённый — без углов, а
полупрозрачный, градиентный и с фоновым изображением не закрывает ничего.

Объявлять надо только то, за что можно ручаться:

- виджет без `OpaqueRegion` считается ПРОЗРАЧНЫМ и ничего не скрывает;
- объявляется площадь, закрашиваемая ПОЛНОСТЬЮ и непрозрачно: тень,
  стекло и любое смешивание — не в счёт;
- вложенность проверяется в ОДИН объявленный прямоугольник, а не в их
  объединение. Поддерево, закрытое двумя окнами вскладчину, будет нарисовано
  зря — это дешевле, чем ошибиться формой объединения.

Ошибка «объявил лишнего» оставляет на экране дыру, ошибка в другую сторону
видна только в профиле.

Замер (полный кадр 1280×800, стопка окон с содержимым): одно окно — без
изменений, пять окон 1094 → 780 мкс, десять окон 1370 → 830 мкс.

Это дополняет пропуск поддеревьев, а не заменяет его: пропуск убирает работу
вне damage, вычитание — работу под другими окнами.

#### Несколько движков в одном процессе

Движков в процессе может быть сколько угодно — оболочка удалённого стола
держит по движку на окно. Всё состояние конвейера принадлежит движку: damage
кадра, выключатель пропуска поддеревьев, накопленные объявления о переносе,
формат пикселей, внешний темп. Движки не мешают друг другу и могут рисовать
одновременно из разных горутин (`tests/twoengines_test.go` проверяет это под
детектором гонок).

Общими на процесс остаются две вещи, и обе намеренно:

- **Дерево виджетов.** Один виджет живёт в одном дереве, дерево — в одном
  движке. Объявления о переносе (`widget.NotifyMove`) рассылаются всем
  движкам, и каждый берёт только то, что лежит на его холсте.
- **Измеритель текста** (`widget.MeasureUIText`). Его зовут оттуда, где
  движка не видно, — диалог считает свой размер при создании. Отвечает
  последний созданный движок; остановленный возвращает измеритель
  предыдущему живому. При разном `SetDPI` у сосуществующих движков замер
  придёт с метриками отвечающего: расхождение — округление логической длины,
  на отрисовку оно не влияет (внутри `Draw` меряет `ctx.MeasureText` своего
  холста).

#### Чем нарисован тайл

```go
frame := <-eng.Frames()
for i, tile := range frame.Tiles {
    switch frame.Regions[i].Kind {
    case output.RegionSolid: // залить одним цветом: Regions[i].Color
    case output.RegionText:  // жать без потерь
    case output.RegionImage: // можно с потерями
    case output.RegionMixed: // как раньше
    }
}
```

Движок знает, чем залил каждый участок, а раньше наружу уходили одни пиксели
— и потребитель это знание восстанавливал вторым проходом кодека. Признак
накапливается по тайлам: полная непрозрачная заливка стирает всё под собой,
текст и картинка поверх фона становятся признаком тайла, заливка поверх
содержимого даёт `RegionMixed` — честное «не знаю».

#### Переехавшее содержимое

```go
for _, m := range frame.Moves { // сначала переносы, потом тайлы!
    blit(m.Src(), m.Rect)
}
```

Перетаскивание окна не меняет пиксели, а переносит их.
`widget.NotifyWidgetMove(w, src, dst)` объявляет перенос; в RDP ему
соответствует пара команд через кэш поверхности вместо сотни килобайт. Виджет
в объявлении называет дерево, которому перенос принадлежит: движки в одном
процессе получают все объявления, а у двух движков одного разрешения
координаты совпадают, и по ним своё от чужого не отличить. Потребитель,
объявляющий переносы сам, зовёт `widget.NotifyMove(src, dst)` — такие
объявления движок разбирает по своему холсту.

Переносы одного кадра между собой не пересекаются: пересекающиеся движок
отбрасывает, и та область уезжает обычными тайлами. Значит, применять их
можно в любом порядке.

#### Порядок каналов и чужая память

```go
eng.SetPixelFormat(engine.FormatBGRX)          // растеризатор пишет сразу BGRX
eng.SetSurface(pix, stride, engine.FormatBGRX) // задний буфер — ваша память
```

Порядок каналов — свойство буфера, а не повод для попиксельного цикла:
потребитель по RDP переставлял каналы сам на каждом кадре. Значение по
умолчанию (`FormatRGBA`) сохраняет прежнее поведение байт в байт.

`SetSurface` отдаёт движку вашу память под задний буфер и убирает две копии
кадра; `stride` может быть больше ширины (выравнивание DIB). Передний буфер
остаётся внутренним — диффу нужна своя копия прошлого кадра.

#### Кто задаёт темп

```go
eng.SetFrameSink(sink)          // сток получает кадр синхронно и не теряет его
eng.SetPacing(engine.PacingExternal)
eng.RequestFrame()              // сток готов принять
```

При внешнем темпе внутренний тикер кадры не запускает (анимации продвигать
продолжает). Это нужно для привязки к вертикальной синхронизации на локальном
выводе — и заодно позволяет потребителю менять сцену и готовить кадр в одной
горутине, устранив гонку по построению. `Frames()` продолжает работать: сток
— альтернатива, а не замена.

---

## Структура модулей

```
go.mod:  module github.com/oops1/headless-gui/v3
  require golang.org/x/image

go.mod:  module github.com/oops1/headless-gui/v3/window
  require github.com/oops1/headless-gui/v3 => ../
  require github.com/ebitengine/purego, golang.org/x/sys
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

Запуск из корневой директории `GuiEngine`:

```bash
go run ./cmd/showcase    # все виджеты + живая анимация
go run ./cmd/desktopdemo # рабочий стол: панель задач и смена тем на ходу
go run ./cmd/guiview     # интерактивное демо с модальными XAML-окнами
go run ./cmd/griddemo    # Grid-раскладка
go run ./cmd/smartgit    # SmartGit-подобный UI
go run ./cmd/webshowcase # вся витрина в браузере (http://localhost:8091)
go run ./cmd/webdemo     # минимальный пример стриминга

# Бинарник без консоли (Windows)
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```
