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

// Шаблонная колонка — пользовательский рендер ячеек
datagrid.NewTemplateColumn("Действия", func(cdc datagrid.CellDrawContext) {
    // рисуем через cdc.DrawCtx...
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
dg.Grid.SelectionMode        // SelectionSingle или SelectionExtended
dg.Grid.RowHeight            // высота строки (по умолчанию 28px)
dg.Grid.HeaderHeight         // высота заголовка (по умолчанию 30px)
dg.Grid.FontSize             // размер шрифта (по умолчанию 10)
```

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
| `Separator` | Separator | `Background` |

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

В комплекте свободные шрифты Roboto (по умолчанию), Open Sans, Inter. Авто-загрузка
из `assets/fonts/`, цепочка фолбэка системных шрифтов для символов/эмодзи (нет
«тофу»).

```go
eng.SetDefaultFont("Inter")
eng.RegisterFontFile("Roboto", path)
eng.RegisterFontDir("my/fonts")
eng.AvailableFonts()
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
`go run ./cmd/webdemo` → http://localhost:8091.

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
go run ./cmd/guiview     # интерактивное демо с модальными XAML-окнами
go run ./cmd/griddemo    # Grid-раскладка
go run ./cmd/smartgit    # SmartGit-подобный UI
go run ./cmd/webdemo     # стриминг UI в браузер (http://localhost:8091)

# Бинарник без консоли (Windows)
go build -ldflags="-H windowsgui" -o showcase.exe ./cmd/showcase
```
