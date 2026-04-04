// Package widget — загрузчик UI из XAML-файлов.
//
// Поддерживаемый синтаксис (подмножество WPF XAML):
//
//	<Panel Name="root" Width="1920" Height="1024" Background="Transparent">
//
//	    <!-- Полупрозрачная панель Win10 -->
//	    <Panel Name="win" Left="100" Top="80" Right="1820" Bottom="944" Style="Win10"/>
//
//	    <Label Left="110" Top="90" Right="800" Bottom="114"
//	           Text="Заголовок" Foreground="#FFFFFF"/>
//
//	    <TextBox  Name="login" Left="110" Top="200" Right="600" Bottom="234"
//	              Placeholder="user@domain.com" Focused="True"/>
//
//	    <PasswordBox Name="pass" Left="110" Top="250" Right="600" Bottom="284"/>
//
//	    <Button Name="btnOK" Left="110" Top="310" Right="260" Bottom="348"
//	            Content="Войти" Style="Accent"/>
//
//	    <ComboBox Name="role" Left="110" Top="360" Right="600" Bottom="394">
//	        <Item>Administrator</Item>
//	        <Item>Operator</Item>
//	    </ComboBox>
//
//	    <ProgressBar Name="pb" Left="110" Top="420" Right="600" Bottom="444"/>
//
//	</Panel>
//
// Позиционирование: абсолютные координаты холста.
// Размер задаётся через Left/Top/Right/Bottom  ИЛИ  Left/Top/Width/Height.
// Цвета: #RRGGBB, #RRGGBBAA, или имена: Transparent, White, Black.
package widget

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // поддержка JPEG в image.Decode
	_ "image/png"  // поддержка PNG в image.Decode
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ─── Внутреннее дерево XAML ─────────────────────────────────────────────────

// xElement — узел XAML-дерева. Хранит имя тега, атрибуты, потомков и текст.
type xElement struct {
	Tag      string
	attrs    map[string]string
	Children []xElement
	Text     string // текст между тегами (напр. <Item>Текст</Item>)
}

// attr возвращает значение первого найденного атрибута из списка.
func (e *xElement) attr(names ...string) string {
	for _, n := range names {
		if v, ok := e.attrs[n]; ok {
			return v
		}
	}
	return ""
}

// name возвращает идентификатор элемента (Name или x:Name).
func (e *xElement) name() string {
	return e.attr("Name", "x:Name")
}

// bounds вычисляет image.Rectangle из атрибутов позиции/размера.
// Поддерживает Left/Top/Right/Bottom и Left/Top/Width/Height.
func (e *xElement) bounds() image.Rectangle {
	left := xatoi(e.attr("Left", "X", "Canvas.Left"))
	top := xatoi(e.attr("Top", "Y", "Canvas.Top"))
	right := xatoi(e.attr("Right", "Canvas.Right"))
	bottom := xatoi(e.attr("Bottom", "Canvas.Bottom"))

	if w := xatoi(e.attr("Width")); w > 0 && right == 0 {
		right = left + w
	}
	if h := xatoi(e.attr("Height")); h > 0 && bottom == 0 {
		bottom = top + h
	}
	return image.Rect(left, top, right, bottom)
}

func xatoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

// ─── Парсер XML → xElement ──────────────────────────────────────────────────

func parseXAML(data []byte) (*xElement, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("xaml: scan: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			var root xElement
			if err := parseXAMLEl(d, start, &root); err != nil {
				return nil, err
			}
			return &root, nil
		}
	}
}

func parseXAMLEl(d *xml.Decoder, start xml.StartElement, el *xElement) error {
	el.Tag = start.Name.Local
	el.attrs = make(map[string]string, len(start.Attr))
	for _, a := range start.Attr {
		el.attrs[a.Name.Local] = a.Value
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("xaml: <%s>: %w", el.Tag, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var child xElement
			if err := parseXAMLEl(d, t, &child); err != nil {
				return err
			}
			el.Children = append(el.Children, child)
		case xml.EndElement:
			return nil
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				el.Text = s
			}
		}
	}
}

// ─── Публичный API ───────────────────────────────────────────────────────────

// LoadUIFromXAMLFile читает XAML-файл и строит дерево виджетов.
// Возвращает корневой виджет и map[name]Widget для именованных элементов.
func LoadUIFromXAMLFile(path string) (Widget, map[string]Widget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	baseDir := filepath.Dir(path)
	return LoadUIFromXAMLWithBase(data, baseDir)
}

// LoadUIFromXAML разбирает XAML и строит дерево виджетов.
// Ресурсы (изображения) не могут загружаться — baseDir пустой.
func LoadUIFromXAML(data []byte) (Widget, map[string]Widget, error) {
	return LoadUIFromXAMLWithBase(data, "")
}

// LoadUIFromXAMLWithBase разбирает XAML и строит дерево виджетов.
// baseDir используется для загрузки ресурсов (BackgroundImage и пр.).
func LoadUIFromXAMLWithBase(data []byte, baseDir string) (Widget, map[string]Widget, error) {
	root, err := parseXAML(data)
	if err != nil {
		return nil, nil, err
	}
	registry := make(map[string]Widget)
	w, err := buildXAMLWidget(*root, registry, image.Point{}, baseDir)
	if err != nil {
		return nil, nil, err
	}
	return w, registry, nil
}

// ─── Построитель виджетов ───────────────────────────────────────────────────

// buildXAMLWidget строит виджет из XAML-элемента.
// parentOff — абсолютное смещение родительского контейнера; координаты
// потомков (Canvas.Left, Canvas.Top, Left, Top, …) трактуются как
// относительные и сдвигаются на parentOff, что соответствует поведению
// WPF Canvas и позволяет открывать XAML-файлы в Blend.
// Для корневого элемента parentOff = image.Point{}.
func buildXAMLWidget(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	tag := strings.ToLower(el.Tag)

	// Игнорируем теги-свойства WPF (Panel.Children, Grid.RowDefinitions, …)
	if strings.Contains(tag, ".") {
		return nil, nil // пропускаем как дочерний виджет
	}

	var w Widget

	switch tag {
	// ── Grid ────────────────────────────────────────────────────────────────
	case "grid":
		return buildXAMLGrid(el, reg, parentOff, baseDir)

	// ── Window — корневой элемент нативного окна ────────────────────────────
	case "window":
		return buildXAMLWindow(el, reg, parentOff, baseDir)

	// ── StackPanel — контейнер с автораскладкой ─────────────────────────────
	case "stackpanel":
		return buildXAMLStackPanel(el, reg, parentOff, baseDir)

	// ── TreeView — иерархический список ─────────────────────────────────────
	case "treeview":
		return buildXAMLTreeView(el, reg, parentOff)

	// ── TreeViewItem вне TreeView — игнорируем ──────────────────────────────
	case "treeviewitem":
		return nil, nil

	// ── DataGrid column definitions — игнорируем вне DataGrid ───────────────
	case "datagridtextcolumn", "datagridtemplatecolumn",
		"datagridcheckboxcolumn", "datagridcomboboxcolumn":
		return nil, nil

	// ── DockPanel — контейнер с dock-layout ────────────────────────────────
	case "dockpanel":
		return buildXAMLDockPanel(el, reg, parentOff, baseDir)

	// ── GridSplitter → Separator (визуальный разделитель) ───────────────────
	case "gridsplitter":
		w = buildXAMLSeparator(el)

	// ── ToolBarTray / ToolBar → горизонтальный StackPanel (WPF ToolBar) ────
	case "toolbartray":
		return buildXAMLToolBarTray(el, reg, parentOff, baseDir)
	case "toolbar":
		return buildXAMLToolBar(el, reg, parentOff, baseDir)

	// ── StatusBar → StackPanel (горизонтальный) ────────────────────────────
	case "statusbar":
		return buildXAMLStatusBar(el, reg, parentOff, baseDir)

	// ── DataGrid → ListView (приближение) ──────────────────────────────────
	case "datagrid":
		w = buildXAMLListViewFromDataGrid(el)

	// ── Border — контейнер с фоном и одним потомком ─────────────────────────
	case "border":
		return buildXAMLBorder(el, reg, parentOff, baseDir)

	// ── Canvas — контейнер с абсолютным позиционированием (WPF Canvas) ──────
	case "canvas":
		return buildXAMLCanvas(el, reg, parentOff, baseDir)

	// ── Контейнеры ──────────────────────────────────────────────────────────
	case "usercontrol",
		"panel", "viewbox":
		w = buildXAMLPanel(el, baseDir)

	// ── Текст ────────────────────────────────────────────────────────────────
	case "label", "textblock", "text", "run":
		w = buildXAMLLabel(el)

	// ── Кнопки ───────────────────────────────────────────────────────────────
	case "button", "togglebutton", "repeatbutton":
		w = buildXAMLButton(el, baseDir)

	// ── Ввод текста ──────────────────────────────────────────────────────────
	case "textbox", "textinput", "input", "richtextbox":
		w = buildXAMLTextInput(el, false)

	case "passwordbox":
		w = buildXAMLTextInput(el, true)

	// ── Выпадающий список ────────────────────────────────────────────────────
	case "combobox", "dropdown":
		w = buildXAMLDropdown(el)

	// ── Прогресс ─────────────────────────────────────────────────────────────
	case "progressbar":
		w = buildXAMLProgressBar(el)

	// ── CheckBox ─────────────────────────────────────────────────────────────
	case "checkbox":
		w = buildXAMLCheckBox(el)

	// ── RadioButton ──────────────────────────────────────────────────────────
	case "radiobutton":
		w = buildXAMLRadioButton(el)

	// ── TabControl ───────────────────────────────────────────────────────────
	case "tabcontrol":
		return buildXAMLTabControl(el, reg, parentOff, baseDir)

	// ── Slider ───────────────────────────────────────────────────────────────
	case "slider":
		w = buildXAMLSlider(el)

	// ── ToggleSwitch ─────────────────────────────────────────────────────────
	case "toggleswitch":
		w = buildXAMLToggleSwitch(el)

	// ── ScrollViewer ─────────────────────────────────────────────────────────
	case "scrollviewer", "scrollview":
		w = buildXAMLScrollView(el)

	// ── ListView ─────────────────────────────────────────────────────────────
	case "listview", "listbox":
		w = buildXAMLListView(el)

	// ── Изображение ───────────────────────────────────────────────────────────
	case "image":
		w = buildXAMLImage(el)

	// ── Разделители / фигуры ─────────────────────────────────────────────────
	case "separator", "line", "rectangle":
		w = buildXAMLSeparator(el)

	// ── PopupMenu ────────────────────────────────────────────────────────────
	case "popupmenu", "contextmenu":
		return buildXAMLPopupMenu(el, reg, parentOff)

	// ── MenuBar (горизонтальное меню) ────────────────────────────────────────
	case "menu", "menubar", "mainmenu":
		return buildXAMLMenuBar(el, reg, parentOff)

	default:
		return nil, fmt.Errorf("xaml: неизвестный элемент <%s>", el.Tag)
	}

	// Координаты в XAML относительны родительского Canvas/Panel (стандарт WPF).
	// Прибавляем parentOff чтобы получить абсолютные экранные координаты.
	// Если el.bounds() пуст (нет координат в XAML) — не затираем bounds,
	// которые виджет мог установить сам (напр. Separator с дефолтным размером).
	absBounds := el.bounds().Add(parentOff)
	if !absBounds.Empty() {
		w.SetBounds(absBounds)
	}

	// Attached properties: Grid.Row/Column, DockPanel.Dock, Margin, Alignment
	applyGridAttachedProps(w, el)
	applyDockAttachedProp(w, el)
	applyMargin(w, el)
	applyAlignment(w, el)
	applyIsEnabled(w, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = w
	}

	// Смещение для дочерних виджетов — Min текущего элемента.
	// Если absBounds пуст — берём из реальных bounds виджета.
	childOff := absBounds.Min
	if absBounds.Empty() {
		childOff = w.Bounds().Min
	}

	// Дочерние виджеты (пропускаем <Item>, <TabItem> — уже обработаны)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag == "item" || childTag == "comboboxitem" || childTag == "listboxitem" ||
			childTag == "tabitem" || childTag == "listviewitem" {
			continue
		}
		if strings.Contains(childTag, ".") {
			// WPF property element — пропускаем сам тег, но обрабатываем его потомков
			for _, inner := range child.Children {
				cw, err := buildXAMLWidget(inner, reg, childOff, baseDir)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					w.AddChild(cw)
				}
			}
			continue
		}
		cw, err := buildXAMLWidget(child, reg, childOff, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			w.AddChild(cw)
		}
	}

	return w, nil
}

// ─── Grid attached properties ───────────────────────────────────────────────

// applyGridAttachedProps читает Grid.Row, Grid.Column и т.д. из XAML-атрибутов
// и устанавливает их в Base виджета.
func applyGridAttachedProps(w Widget, el xElement) {
	type gridSetter interface {
		GetGridRow() int // наличие этого метода означает, что Base встроен
	}
	// Все наши виджеты встраивают Base, поэтому можно писать напрямую.
	// Используем рефлексию через интерфейс не нужно — у нас есть конкретный тип.
	// Простой подход: пишем через указатель на Base.
	type baseAccessor interface {
		Widget
		GetGridRow() int
	}
	if _, ok := w.(baseAccessor); !ok {
		return
	}

	row := xatoi(el.attr("Grid.Row"))
	col := xatoi(el.attr("Grid.Column"))
	rowSpan := xatoi(el.attr("Grid.RowSpan"))
	colSpan := xatoi(el.attr("Grid.ColumnSpan"))

	// Нужно добраться до Base. Используем сеттер-интерфейс.
	type gridPropsSetter interface {
		SetGridProps(row, col, rowSpan, colSpan int)
	}
	if gs, ok := w.(gridPropsSetter); ok {
		gs.SetGridProps(row, col, rowSpan, colSpan)
	}
}

// ─── buildXAMLGrid ─────────────────────────────────────────────────────────

// buildXAMLGrid создаёт Grid из XAML-элемента, парсит RowDefinitions/ColumnDefinitions,
// создаёт потомков и вызывает layout.
func buildXAMLGrid(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	g := NewGrid()

	// Фон
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if strings.EqualFold(bgStr, "transparent") {
			g.UseAlpha = true
		} else if c, err := parseXAMLColor(bgStr); err == nil {
			g.Background = c
			g.UseAlpha = c.A < 255
		}
	} else {
		g.UseAlpha = true
	}

	// ShowGridLines
	if strings.EqualFold(el.attr("ShowGridLines"), "true") {
		g.ShowGridLines = true
	}

	// Парсим Grid.RowDefinitions и Grid.ColumnDefinitions (property elements).
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		switch childTag {
		case "grid.rowdefinitions":
			for _, rd := range child.Children {
				if strings.ToLower(rd.Tag) == "rowdefinition" {
					g.RowDefs = append(g.RowDefs, parseGridDef(rd, "Height"))
				}
			}
		case "grid.columndefinitions":
			for _, cd := range child.Children {
				if strings.ToLower(cd.Tag) == "columndefinition" {
					g.ColDefs = append(g.ColDefs, parseGridDef(cd, "Width"))
				}
			}
		}
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	g.SetBounds(absBounds) // вызовет layout() — но дети ещё не добавлены

	// Attached properties — важно для вложенных Grid'ов внутри родительского Grid.
	applyGridAttachedProps(g, el)
	applyDockAttachedProp(g, el)
	applyMargin(g, el)
	applyIsEnabled(g, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = g
	}

	// Дочерние виджеты.
	// Для Grid НЕ используем childOff — Grid сам расставляет потомков через layout.
	// Передаём parentOff = image.Point{} (нулевой), т.к. координаты потомков
	// будут заданы Grid.layout() по ячейкам. Но если у потомка есть Left/Top,
	// они будут смещением внутри ячейки (не используем для Grid-потомков).
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// Пропускаем property elements
		if strings.Contains(childTag, ".") {
			continue
		}
		// Пропускаем Item-подобные теги
		if childTag == "item" || childTag == "comboboxitem" || childTag == "listboxitem" {
			continue
		}

		// Для дочерних виджетов Grid передаём parentOff=0, т.к. Grid.layout()
		// сам задаст bounds через SetBounds.
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			g.AddChild(cw)
		}
	}

	// Перезапускаем layout с добавленными потомками.
	g.layout()

	return g, nil
}

// parseGridDef парсит <RowDefinition Height="..."/> или <ColumnDefinition Width="..."/>.
//
// Форматы значений:
//
//	"Auto"       → GridSizeAuto
//	"*"          → GridSizeStar, Value=1
//	"2*"         → GridSizeStar, Value=2
//	"100"        → GridSizePixel, Value=100
func parseGridDef(el xElement, sizeAttr string) GridDefinition {
	raw := strings.TrimSpace(el.attr(sizeAttr))
	d := GridDefinition{Mode: GridSizeStar, Value: 1} // default = 1*

	if raw == "" || raw == "*" {
		// default star
	} else if strings.EqualFold(raw, "auto") {
		d.Mode = GridSizeAuto
		d.Value = 0
	} else if strings.HasSuffix(raw, "*") {
		d.Mode = GridSizeStar
		numStr := strings.TrimSuffix(raw, "*")
		if numStr == "" {
			d.Value = 1
		} else {
			v, _ := strconv.ParseFloat(numStr, 64)
			if v <= 0 {
				v = 1
			}
			d.Value = v
		}
	} else {
		// Pixel
		v, _ := strconv.ParseFloat(raw, 64)
		if v > 0 {
			d.Mode = GridSizePixel
			d.Value = v
		}
	}

	// Min/Max
	d.Min = xatoi(el.attr("MinHeight", "MinWidth"))
	d.Max = xatoi(el.attr("MaxHeight", "MaxWidth"))

	return d
}

// ─── Построители конкретных виджетов ────────────────────────────────────────

// buildXAMLWindow строит виджет Window из XAML-элемента <Window>.
//
// Window — корневой элемент нативного окна. Не является контейнером-рабочим столом
// (в отличие от Canvas/Panel). Создаёт одно независимое окно приложения
// с собственным chrome (заголовок, рамка, кнопки управления).
//
// Поддерживаемые WPF-совместимые атрибуты:
//
//	Title            — заголовок окна
//	WindowStyle      — SingleBorderWindow | None | ToolWindow
//	TitleStyle       — Win | Mac  (расширение; WPF не имеет)
//	ResizeMode       — CanResize | NoResize | CanMinimize
//	Background       — цвет фона клиентской области (#RRGGBB / #RRGGBBAA)
//	BorderBrush      — цвет рамки
//	CornerRadius     — радиус скругления
//	TitleBarHeight   — высота заголовка (0 = авто)
//	TitleBackground  — цвет фона заголовка
//	TitleForeground  — цвет текста заголовка
//
// Дочерние виджеты размещаются в клиентской области (ContentBounds).
func buildXAMLWindow(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	b := el.bounds()
	if b.Empty() {
		b = image.Rect(0, 0, 800, 600) // default
	}
	win := NewWindow(el.attr("Title", "Caption"), b.Dx(), b.Dy())

	// WindowStyle: SingleBorderWindow (default) | None | ToolWindow
	switch strings.ToLower(el.attr("WindowStyle")) {
	case "none":
		win.Style = WindowStyleNone
	case "toolwindow":
		win.Style = WindowStyleToolWindow
	default:
		win.Style = WindowStyleSingleBorder
	}

	// TitleStyle: Win (default) | Mac
	switch strings.ToLower(el.attr("TitleStyle")) {
	case "mac":
		win.TitleStyle = WindowTitleMac
	default:
		win.TitleStyle = WindowTitleWin
	}

	// ResizeMode: CanResize (default) | NoResize | CanMinimize
	switch strings.ToLower(el.attr("ResizeMode")) {
	case "noresize":
		win.Resize = ResizeModeNoResize
	case "canminimize":
		win.Resize = ResizeModeCanMinimize
	default:
		win.Resize = ResizeModeCanResize
	}

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			win.Background = c
		}
	}

	// BorderBrush
	if bc := el.attr("BorderBrush"); bc != "" {
		if c, err := parseXAMLColor(bc); err == nil {
			win.BorderColor = c
		}
	}

	// CornerRadius
	if cr := xatoi(el.attr("CornerRadius")); cr > 0 {
		win.CornerRadius = cr
	}

	// TitleBarHeight
	if h := xatoi(el.attr("TitleBarHeight")); h > 0 {
		win.TitleBarHeight = h
	}

	// TitleBackground / TitleForeground
	if tbg := el.attr("TitleBackground"); tbg != "" {
		if c, err := parseXAMLColor(tbg); err == nil {
			win.TitleBG = c
		}
	}
	if tfc := el.attr("TitleForeground"); tfc != "" {
		if c, err := parseXAMLColor(tfc); err == nil {
			win.TitleColor = c
		}
	}

	// Bounds (с учётом parentOff — обычно 0,0 для корня)
	absBounds := b.Add(parentOff)
	win.SetBounds(absBounds)
	applyIsEnabled(win, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = win
	}

	// Дочерние виджеты размещаются относительно ContentBounds.
	contentOff := win.ContentBounds().Min
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// Пропускаем property elements
		if strings.Contains(childTag, ".") {
			// Но обрабатываем потомков property element (например Window.Resources)
			for _, inner := range child.Children {
				cw, err := buildXAMLWidget(inner, reg, contentOff, baseDir)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					win.AddChild(cw)
				}
			}
			continue
		}
		// Пропускаем Item-подобные теги
		if childTag == "item" || childTag == "comboboxitem" {
			continue
		}

		cw, err := buildXAMLWidget(child, reg, contentOff, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			win.AddChild(cw)
		}
	}

	// WPF-поведение: Content-элемент без явного размера заполняет клиентскую область.
	// Для Grid это особенно важно — SetBounds запустит layout() с правильными размерами.
	cb := win.ContentBounds()
	for _, child := range win.Children() {
		childB := child.Bounds()
		if childB.Dx() <= 0 || childB.Dy() <= 0 || childB.Empty() {
			child.SetBounds(cb)
		}
	}

	return win, nil
}

func buildXAMLPanel(el xElement, baseDir string) Widget {
	style := strings.ToLower(el.attr("Tag", "Style"))
	bgStr := el.attr("Background", "Fill", "Color")
	cr := xatoi(el.attr("CornerRadius"))

	var p *Panel

	switch style {
	case "win10":
		p = NewWin10Panel()
		if bgStr != "" {
			if c, err := parseXAMLColor(bgStr); err == nil && c.A > 0 {
				p.Background = c
			}
		}
		p.CornerRadius = cr

	default:
		if bgStr == "" || strings.EqualFold(bgStr, "transparent") {
			p = NewPanel(color.RGBA{})
			p.UseAlpha = true
			p.CornerRadius = cr
		} else if c, err := parseXAMLColor(bgStr); err == nil {
			p = NewPanel(c)
			p.UseAlpha = c.A < 255
			p.CornerRadius = cr
			if bc := el.attr("BorderBrush"); bc != "" {
				if bc2, err := parseXAMLColor(bc); err == nil {
					p.BorderColor = bc2
					p.ShowBorder = true
				}
			}
			if el.attr("BorderThickness") != "" {
				p.ShowBorder = true
			}
		} else {
			p = NewPanel(color.RGBA{})
			p.UseAlpha = true
			p.CornerRadius = cr
		}
	}

	// ── Заголовок окна ──────────────────────────────────────────────────────
	if caption := el.attr("Caption", "Title"); caption != "" {
		p.Caption = caption
	}
	// ShowHeader: по умолчанию true (задано в конструкторе).
	// XAML может явно выключить: ShowHeader="False".
	if sh := el.attr("ShowHeader"); sh != "" {
		p.ShowHeader = strings.EqualFold(sh, "true") || sh == "1"
	}
	// MacStyle: по умолчанию false.
	if ms := el.attr("MacStyle"); ms != "" {
		p.MacStyle = strings.EqualFold(ms, "true") || ms == "1"
	}
	// HeaderHeight
	if hh := xatoi(el.attr("HeaderHeight")); hh > 0 {
		p.HeaderHeight = hh
	}

	// BackgroundImage — фоновая картинка из файла (относительно XAML-файла).
	if bgImg := el.attr("BackgroundImage"); bgImg != "" && baseDir != "" {
		imgPath := bgImg
		if !filepath.IsAbs(imgPath) {
			imgPath = filepath.Join(baseDir, imgPath)
		}
		if img, err := loadImageFile(imgPath); err == nil {
			p.BackgroundImage = img
		}
	}

	return p
}

// ─── Canvas builder ─────────────────────────────────────────────────────────

// buildXAMLCanvas строит Canvas виджет из XAML-элемента.
// Canvas размещает дочерние виджеты по абсолютным координатам (Canvas.Left, Canvas.Top, и т.д.).
// Это полноценный аналог WPF Canvas, в отличие от Panel — Canvas сам управляет layout.
func buildXAMLCanvas(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	cv := NewCanvas()

	// Background
	if bgStr := el.attr("Background", "Fill", "Color"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			cv.Background = c
			cv.UseAlpha = c.A < 255
		}
	}

	// ClipToBounds (WPF default = false)
	if clip := el.attr("ClipToBounds"); clip != "" {
		cv.ClipToBounds = strings.EqualFold(clip, "true") || clip == "1"
	}

	// Bounds Canvas — абсолютные координаты
	absBounds := el.bounds().Add(parentOff)
	cv.SetBounds(absBounds)

	// Attached properties: Grid.Row/Column, DockPanel.Dock, Margin
	applyGridAttachedProps(cv, el)
	applyDockAttachedProp(cv, el)
	applyMargin(cv, el)
	applyIsEnabled(cv, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = cv
	}

	// ── Дочерние виджеты ────────────────────────────────────────────────────
	// Canvas передаёт image.Point{} как parentOff для дочерних виджетов,
	// потому что Canvas сам управляет позиционированием через attached properties.
	// el.bounds() внутри buildXAMLWidget уже читает Canvas.Left/Top как Left/Top,
	// что приводит к двойному учёту позиции. Поэтому parentOff=0 и десятка
	// полагается на Width/Height для desiredSize, а позицию задаёт Canvas layout.
	zeroOff := image.Point{}

	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// Пропускаем WPF property elements, но обрабатываем их потомков
		if strings.Contains(childTag, ".") {
			for _, inner := range child.Children {
				if err := addCanvasChild(cv, inner, reg, zeroOff, baseDir); err != nil {
					return nil, err
				}
			}
			continue
		}

		if err := addCanvasChild(cv, child, reg, zeroOff, baseDir); err != nil {
			return nil, err
		}
	}

	return cv, nil
}

// addCanvasChild строит дочерний виджет и добавляет его в Canvas с учётом
// Canvas.Left / Canvas.Top / Canvas.Right / Canvas.Bottom attached properties.
//
// Важно: дочерний виджет строится с parentOff=image.Point{} (нулевое смещение),
// потому что Canvas сам управляет позиционированием. buildXAMLWidget прибавит
// атрибуты Left/Top к parentOff, но нам нужно только Width/Height.
func addCanvasChild(cv *Canvas, child xElement, reg map[string]Widget, canvasOff image.Point, baseDir string) error {
	// ── Извлекаем Canvas attached properties ────────────────────────────────
	props := CanvasAttached{
		Left:   xatoiOrNeg1(child.attr("Canvas.Left")),
		Top:    xatoiOrNeg1(child.attr("Canvas.Top")),
		Right:  xatoiOrNeg1(child.attr("Canvas.Right")),
		Bottom: xatoiOrNeg1(child.attr("Canvas.Bottom")),
	}

	// Если Canvas.Left/Top не указаны, пробуем Left/Top/X/Y (упрощённый синтаксис)
	if props.Left < 0 {
		if v := child.attr("Left", "X"); v != "" {
			props.Left = xatoi(v)
		}
	}
	if props.Top < 0 {
		if v := child.attr("Top", "Y"); v != "" {
			props.Top = xatoi(v)
		}
	}
	if props.Right < 0 {
		if v := child.attr("Right"); v != "" {
			props.Right = xatoi(v)
		}
	}
	if props.Bottom < 0 {
		if v := child.attr("Bottom"); v != "" {
			props.Bottom = xatoi(v)
		}
	}

	// ── Желаемый размер из XAML атрибутов ───────────────────────────────────
	desiredW := xatoi(child.attr("Width"))
	desiredH := xatoi(child.attr("Height"))

	// ── Строим дочерний виджет ──────────────────────────────────────────────
	// Передаём canvasOff как parentOff — buildXAMLWidget использует его
	// для абсолютных координат. Это нужно чтобы вложенные контейнеры
	// (Canvas внутри Canvas, Grid внутри Canvas) получили правильный offset.
	// Для leaf-виджетов buildXAMLWidget вычислит bounds через el.bounds().Add(parentOff),
	// но Canvas потом переопределит позицию через layout.
	cw, err := buildXAMLWidget(child, reg, canvasOff, baseDir)
	if err != nil {
		return err
	}
	if cw == nil {
		return nil
	}

	// Если Width/Height не были заданы явно в XAML, попробуем взять
	// из bounds, которые buildXAMLWidget мог установить
	if desiredW <= 0 {
		desiredW = cw.Bounds().Dx()
	}
	if desiredH <= 0 {
		desiredH = cw.Bounds().Dy()
	}

	// Не сбрасываем bounds — Canvas.layoutChild пересчитает позицию
	// и сдвинет потомков на правильную дельту через shiftDescendants.
	// Если сбросить bounds в (0,0), дельта будет неверной для контейнеров.

	cv.AddChildAt(cw, props, desiredW, desiredH)
	return nil
}

// xatoiOrNeg1 парсит строку как int; при пустой строке или ошибке возвращает -1.
func xatoiOrNeg1(s string) int {
	if s == "" {
		return -1
	}
	v := xatoi(s)
	if v == 0 && s != "0" {
		return -1
	}
	return v
}

// loadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
func loadImageFile(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// image.Decode использует зарегистрированные декодеры (png, jpeg).
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	// Конвертируем в RGBA
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}

func buildXAMLImage(el xElement) Widget {
	iw := NewImageWidget()
	if src := el.attr("Source"); src != "" {
		// Загружаем файл. Ошибки игнорируем — виджет покажет Fallback.
		_ = iw.SetSource(src)
		iw.Source = src
	}
	switch strings.ToLower(el.attr("Stretch")) {
	case "uniform", "uniformtofill":
		iw.Stretch = ImageStretchUniform
	case "none":
		iw.Stretch = ImageStretchNone
	default:
		iw.Stretch = ImageStretchFill
	}
	return iw
}

func buildXAMLLabel(el xElement) Widget {
	// Текст: атрибут Text/Content, или текст между тегами
	text := el.attr("Text", "Content")
	if text == "" {
		text = el.Text
	}

	fg := el.attr("Foreground", "TextColor", "Fill")
	bg := el.attr("Background")

	var lbl *Label
	if fg != "" {
		if c, err := parseXAMLColor(fg); err == nil {
			lbl = NewLabel(text, c)
		}
	}
	if lbl == nil {
		lbl = NewWin10Label(text)
	}

	if bg != "" {
		if c, err := parseXAMLColor(bg); err == nil && c.A > 0 {
			lbl.HasBG = true
			lbl.Background = c
		}
	}

	// TextWrapping="Wrap" или TextWrapping="WrapWithOverflow" — перенос по словам.
	wrap := strings.ToLower(el.attr("TextWrapping"))
	if wrap == "wrap" || wrap == "wrapwithoverflow" {
		lbl.WrapText = true
	}

	// FontSize
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			lbl.FontSize = v
		}
	}

	// FontFamily — именованный шрифт (зарегистрированный через RegisterFont)
	if ff := el.attr("FontFamily"); ff != "" {
		lbl.FontName = ff
	}

	// Padding — внутренний отступ (WPF Thickness).
	// Поддерживает: "N" (все стороны), "H,V" (горизонтальный, вертикальный), "L,T,R,B".
	if pad := el.attr("Padding"); pad != "" {
		parts := strings.Split(pad, ",")
		switch len(parts) {
		case 1:
			v := xatoi(strings.TrimSpace(parts[0]))
			lbl.PaddingX = v
			lbl.PaddingY = v
		case 2:
			lbl.PaddingX = xatoi(strings.TrimSpace(parts[0]))
			lbl.PaddingY = xatoi(strings.TrimSpace(parts[1]))
		case 4:
			lbl.PaddingX = xatoi(strings.TrimSpace(parts[0])) // Left
			lbl.PaddingY = xatoi(strings.TrimSpace(parts[1])) // Top
			// Right и Bottom игнорируются (Label использует симметричные PaddingX/Y)
		}
	}

	return lbl
}

func buildXAMLButton(el xElement, baseDir string) Widget {
	text := el.attr("Content", "Text")
	if text == "" {
		text = el.Text
	}
	// Стиль кнопки: через Tag="Accent" (стандартный WPF) или Style="Accent" (наш).
	style := strings.ToLower(el.attr("Tag", "Style"))
	var btn *Button
	if style == "accent" || style == "primary" {
		btn = NewWin10AccentButton(text)
	} else {
		btn = NewButton(text)
	}

	// Кастомные цвета из XAML-атрибутов.
	if bg := el.attr("Background"); bg != "" {
		if c, err := parseXAMLColor(bg); err == nil {
			btn.Background = c
		}
	}
	if fg := el.attr("Foreground"); fg != "" {
		if c, err := parseXAMLColor(fg); err == nil {
			btn.TextColor = c
		}
	}
	if hbg := el.attr("HoverBG", "HoverBackground"); hbg != "" {
		if c, err := parseXAMLColor(hbg); err == nil {
			btn.HoverBG = c
		}
	}
	if pbg := el.attr("PressedBG", "PressedBackground"); pbg != "" {
		if c, err := parseXAMLColor(pbg); err == nil {
			btn.PressedBG = c
		}
	}
	if bc := el.attr("BorderBrush"); bc != "" {
		if c, err := parseXAMLColor(bc); err == nil {
			btn.BorderColor = c
		}
	}

	// ── Иконка ─────────────────────────────────────────────────────────────
	if iconSrc := el.attr("Icon", "IconSource"); iconSrc != "" {
		path := iconSrc
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, iconSrc)
		}
		if img, err := loadImageFile(path); err == nil {
			btn.Icon = img
			btn.IconPath = iconSrc
		}
	}
	// IconPosition: Left (default), Top, Only
	switch strings.ToLower(el.attr("IconPosition", "IconPos")) {
	case "top":
		btn.IconPos = IconTop
	case "only", "icononly":
		btn.IconPos = IconOnly
	}
	// IconSize
	if sz := xatoi(el.attr("IconSize")); sz > 0 {
		btn.IconSize = sz
	}

	return btn
}

func buildXAMLTextInput(el xElement, isPassword bool) Widget {
	// Placeholder: стандартный WPF не имеет Placeholder на TextBox,
	// поэтому используем Tag="подсказка" — валидный WPF-атрибут.
	placeholder := el.attr("Tag", "Placeholder", "PlaceholderText", "Hint")
	if isPassword && placeholder == "" {
		placeholder = "Пароль"
	}
	var ti *TextInput
	if isPassword {
		ti = NewPasswordInput(placeholder)
	} else {
		ti = NewTextInput(placeholder)
	}

	if text := el.attr("Text"); text != "" {
		ti.SetText(text)
	}
	// Focused: поддерживается для обратной совместимости; в новом WPF XAML
	// начальный фокус устанавливается из Go-кода (NewRuntime → eng.SetFocus).
	if strings.EqualFold(el.attr("Focused", "IsFocused", "Focus"), "true") {
		ti.SetFocused(true)
	}
	if fg := el.attr("Foreground"); fg != "" {
		if c, err := parseXAMLColor(fg); err == nil {
			ti.TextColor = c
		}
	}

	// Background
	if bgStr := el.attr("Background"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			ti.Background = c
		}
	}

	// FontFamily — именованный шрифт
	if ff := el.attr("FontFamily"); ff != "" {
		ti.FontName = ff
	}

	// FontSize
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			ti.FontSize = v
		}
	}

	// AcceptsReturn — многострочный режим (WPF TextBox.AcceptsReturn)
	if strings.EqualFold(el.attr("AcceptsReturn"), "true") {
		ti.AcceptsReturn = true
	}

	// TextWrapping — в WPF влияет на перенос строк в TextBox (здесь как индикатор multiline)
	if wrap := strings.ToLower(el.attr("TextWrapping")); wrap == "wrap" || wrap == "wrapwithoverflow" {
		ti.AcceptsReturn = true // WPF: Wrap на TextBox подразумевает многострочность
	}

	return ti
}

func buildXAMLDropdown(el xElement) Widget {
	// Пункты из атрибута Items="A,B,C"
	var items []string
	if raw := el.attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				items = append(items, s)
			}
		}
	}
	// Пункты из дочерних <Item> / <ComboBoxItem>
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		if t == "item" || t == "comboboxitem" || t == "listboxitem" {
			v := child.attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				items = append(items, v)
			}
		}
	}

	dd := NewDropdown(items...)
	if sel := el.attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			dd.SetSelected(idx)
		}
	}
	return dd
}

func buildXAMLProgressBar(el xElement) Widget {
	pb := NewProgressBar()
	if fill := el.attr("Foreground", "Fill"); fill != "" {
		if c, err := parseXAMLColor(fill); err == nil {
			pb.FillColor = c
		}
	}
	if val := el.attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			pb.SetValue(v)
		}
	}
	return pb
}

func buildXAMLSeparator(el xElement) Widget {
	bg := el.attr("Background", "Fill", "Stroke")
	c := color.RGBA{R: 76, G: 76, B: 76, A: 255}
	if bg != "" {
		if parsed, err := parseXAMLColor(bg); err == nil {
			c = parsed
		}
	}
	p := NewPanel(c)
	p.ShowHeader = false

	// Если Width/Height не заданы в XAML — задаём дефолтный тонкий размер.
	// Separator в WPF по умолчанию: горизонтальная линия (height=1).
	// В ToolBar (горизонтальный контейнер) — вертикальная линия (width=1, height=stretch).
	w := xatoi(el.attr("Width"))
	h := xatoi(el.attr("Height"))
	if w <= 0 && h <= 0 {
		// Дефолт: тонкая вертикальная линия для ToolBar
		p.SetBounds(image.Rect(0, 0, 1, 24))
	} else if w > 0 && h <= 0 {
		// Горизонтальный разделитель: задана ширина → высота = 1px
		p.SetBounds(image.Rect(0, 0, w, 1))
	} else if h > 0 && w <= 0 {
		p.SetBounds(image.Rect(0, 0, 1, h))
	} else if w > 0 && h > 0 {
		p.SetBounds(image.Rect(0, 0, w, h))
	}
	return p
}

// ─── Новые виджеты ──────────────────────────────────────────────────────────

func buildXAMLCheckBox(el xElement) Widget {
	text := el.attr("Content", "Text")
	if text == "" {
		text = el.Text
	}
	cb := NewCheckBox(text)
	if strings.EqualFold(el.attr("IsChecked", "Checked"), "true") {
		cb.SetChecked(true)
	}
	return cb
}

func buildXAMLRadioButton(el xElement) Widget {
	text := el.attr("Content", "Text")
	if text == "" {
		text = el.Text
	}
	group := el.attr("GroupName", "Group")
	rb := NewRadioButton(text, group)
	if strings.EqualFold(el.attr("IsChecked", "Checked", "Selected"), "true") {
		rb.SetSelected(true)
	}
	return rb
}

func buildXAMLSlider(el xElement) Widget {
	s := NewSlider()
	if min := el.attr("Minimum", "Min"); min != "" {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			s.Min = v
		}
	}
	if max := el.attr("Maximum", "Max"); max != "" {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			s.Max = v
		}
	}
	if val := el.attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			s.SetValue(v)
		}
	}
	return s
}

func buildXAMLToggleSwitch(el xElement) Widget {
	text := el.attr("Content", "Text", "Header")
	if text == "" {
		text = el.Text
	}
	ts := NewToggleSwitch(text)
	if strings.EqualFold(el.attr("IsOn", "IsChecked", "Checked"), "true") {
		ts.SetOn(true)
	}
	return ts
}

func buildXAMLScrollView(el xElement) Widget {
	sv := NewScrollView()
	if ch := el.attr("ContentHeight"); ch != "" {
		sv.ContentHeight = xatoi(ch)
	}
	if bg := el.attr("Background"); bg != "" {
		if c, err := parseXAMLColor(bg); err == nil && c.A > 0 {
			sv.Background = c
		}
	}
	return sv
}

func buildXAMLListView(el xElement) Widget {
	var items []string
	// Пункты из атрибута Items="A,B,C"
	if raw := el.attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				items = append(items, s)
			}
		}
	}
	// Пункты из дочерних <Item> / <ListViewItem>
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		if t == "item" || t == "listviewitem" || t == "listboxitem" {
			v := child.attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				items = append(items, v)
			}
		}
	}
	lv := NewListView(items...)

	// Background
	if bgStr := el.attr("Background"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			lv.Background = c
		}
	}

	if sel := el.attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			lv.SetSelected(idx)
		}
	}
	if ih := el.attr("ItemHeight"); ih != "" {
		if v := xatoi(ih); v > 0 {
			lv.ItemHeight = v
		}
	}
	return lv
}

func buildXAMLTabControl(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	tc := NewTabControl()
	absBounds := el.bounds().Add(parentOff)
	tc.SetBounds(absBounds)
	applyIsEnabled(tc, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = tc
	}

	// contentOff — смещение для содержимого вкладок (ниже полосы табов).
	contentOff := image.Pt(absBounds.Min.X, absBounds.Min.Y+tc.TabHeight)

	// Обрабатываем TabItem дочерние элементы
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag == "tabitem" {
			header := child.attr("Header", "Content", "Text")
			if header == "" {
				header = child.Text
			}

			// Содержимое вкладки — первый дочерний элемент TabItem.
			// Координаты дочерних виджетов относительно области контента (ниже табов).
			var content Widget
			for _, inner := range child.Children {
				innerTag := strings.ToLower(inner.Tag)
				if strings.Contains(innerTag, ".") {
					continue
				}
				cw, err := buildXAMLWidget(inner, reg, contentOff, baseDir)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					content = cw
					break
				}
			}
			tc.AddTab(header, content)
		} else if !strings.Contains(childTag, ".") {
			// Обычные дочерние виджеты (не TabItem)
			cw, err := buildXAMLWidget(child, reg, contentOff, baseDir)
			if err != nil {
				return nil, err
			}
			if cw != nil {
				tc.AddChild(cw)
			}
		}
	}

	if sel := el.attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			tc.SetActive(idx)
		}
	}

	return tc, nil
}

// ─── MenuBar ────────────────────────────────────────────────────────────────

func buildXAMLMenuBar(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	mb := NewMenuBar()
	absBounds := el.bounds().Add(parentOff)
	mb.SetBounds(absBounds)

	if id := el.name(); id != "" {
		reg[id] = mb
	}

	// Foreground
	if fgStr := el.attr("Foreground"); fgStr != "" {
		if c, err := parseXAMLColor(fgStr); err == nil {
			mb.TextColor = c
		}
	}

	// Background
	if bgStr := el.attr("Background"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			mb.Background = c
		}
	}

	// Attached properties
	applyDockAttachedProp(mb, el)
	applyMargin(mb, el)
	applyIsEnabled(mb, el)

	// Парсим верхнеуровневые <MenuItem Header="..."> с вложенными подпунктами.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag != "menuitem" && childTag != "item" {
			continue
		}

		header := child.attr("Header", "Text", "Content")
		if header == "" {
			header = child.Text
		}

		// Рекурсивно собираем подпункты.
		subItems := parseMenuItems(child)
		mb.AddMenu(header, subItems...)
	}

	return mb, nil
}

// parseMenuItems рекурсивно собирает MenuItem из дочерних <MenuItem>.
func parseMenuItems(parent xElement) []MenuItem {
	var items []MenuItem
	for _, sub := range parent.Children {
		subTag := strings.ToLower(sub.Tag)
		if subTag != "menuitem" && subTag != "item" {
			continue
		}

		sep := strings.EqualFold(sub.attr("Separator"), "True")
		if sep {
			items = append(items, MenuItem{Separator: true})
			continue
		}

		text := sub.attr("Header", "Text", "Content")
		if text == "" {
			text = sub.Text
		}

		disabled := strings.EqualFold(sub.attr("IsEnabled"), "False") ||
			strings.EqualFold(sub.attr("Disabled"), "True")

		item := MenuItem{
			Text:     text,
			Disabled: disabled,
		}

		// Рекурсивные подменю (3+ уровень).
		if len(sub.Children) > 0 {
			item.SubItems = parseMenuItems(sub)
		}

		items = append(items, item)
	}
	return items
}

// ─── PopupMenu ──────────────────────────────────────────────────────────────

func buildXAMLPopupMenu(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	pm := NewPopupMenu()
	absBounds := el.bounds().Add(parentOff)
	pm.SetBounds(absBounds)
	applyIsEnabled(pm, el)

	if id := el.name(); id != "" {
		reg[id] = pm
	}

	// Парсим дочерние <MenuItem> элементы.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag != "menuitem" && childTag != "item" {
			continue
		}

		sep := strings.EqualFold(child.attr("Separator"), "True")
		if sep {
			pm.AddSeparator()
			continue
		}

		text := child.attr("Header", "Text", "Content")
		if text == "" {
			text = child.Text
		}

		disabled := strings.EqualFold(child.attr("IsEnabled"), "False") ||
			strings.EqualFold(child.attr("Disabled"), "True")

		item := MenuItem{
			Text:     text,
			Disabled: disabled,
		}
		pm.mu.Lock()
		pm.items = append(pm.items, item)
		pm.mu.Unlock()
	}

	return pm, nil
}

// ─── Margin ────────────────────────────────────────────────────────────────

// parseMargin разбирает WPF Margin: "5", "5,10", "1,2,3,4".
func parseMargin(s string) Margin {
	s = strings.TrimSpace(s)
	if s == "" {
		return Margin{}
	}
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 1:
		v := xatoi(parts[0])
		return Margin{Left: v, Top: v, Right: v, Bottom: v}
	case 2:
		h := xatoi(parts[0])
		v := xatoi(parts[1])
		return Margin{Left: h, Top: v, Right: h, Bottom: v}
	case 4:
		return Margin{
			Left:   xatoi(parts[0]),
			Top:    xatoi(parts[1]),
			Right:  xatoi(parts[2]),
			Bottom: xatoi(parts[3]),
		}
	default:
		return Margin{}
	}
}

// applyMargin читает Margin из XAML-атрибутов и устанавливает в Base.
func applyMargin(w Widget, el xElement) {
	ms := el.attr("Margin")
	if ms == "" {
		return
	}
	m := parseMargin(ms)
	type marginSetter interface {
		SetMargin(m Margin)
	}
	if setter, ok := w.(marginSetter); ok {
		setter.SetMargin(m)
	}
}

// applyAlignment читает HorizontalAlignment и VerticalAlignment из XAML-атрибутов.
func applyAlignment(w Widget, el xElement) {
	type alignSetter interface {
		SetHAlign(a HorizontalAlignment)
		SetVAlign(a VerticalAlignment)
	}
	as, ok := w.(alignSetter)
	if !ok {
		return
	}
	if ha := el.attr("HorizontalAlignment"); ha != "" {
		switch strings.ToLower(ha) {
		case "left":
			as.SetHAlign(HAlignLeft)
		case "center":
			as.SetHAlign(HAlignCenter)
		case "right":
			as.SetHAlign(HAlignRight)
		case "stretch":
			as.SetHAlign(HAlignStretch)
		}
	}
	if va := el.attr("VerticalAlignment"); va != "" {
		switch strings.ToLower(va) {
		case "top":
			as.SetVAlign(VAlignTop)
		case "center":
			as.SetVAlign(VAlignCenter)
		case "bottom":
			as.SetVAlign(VAlignBottom)
		case "stretch":
			as.SetVAlign(VAlignStretch)
		}
	}
}

// ─── IsEnabled ──────────────────────────────────────────────────────────────

// applyIsEnabled читает IsEnabled из XAML-атрибутов и устанавливает в Base.
// WPF по умолчанию IsEnabled=True, поэтому false нужно задавать явно.
func applyIsEnabled(w Widget, el xElement) {
	type enabledSetter interface {
		SetEnabled(v bool)
	}
	es, ok := w.(enabledSetter)
	if !ok {
		return
	}
	if v := el.attr("IsEnabled"); strings.EqualFold(v, "False") {
		es.SetEnabled(false)
	}
}

// ─── DockPanel.Dock attached property ───────────────────────────────────────

// applyDockAttachedProp читает DockPanel.Dock из XAML-атрибутов и устанавливает в Base.
func applyDockAttachedProp(w Widget, el xElement) {
	dock := el.attr("DockPanel.Dock")
	if dock == "" {
		return
	}
	type dockSetter interface {
		SetDock(d DockSide)
	}
	if ds, ok := w.(dockSetter); ok {
		switch strings.ToLower(dock) {
		case "top":
			ds.SetDock(DockTop)
		case "bottom":
			ds.SetDock(DockBottom)
		case "left":
			ds.SetDock(DockLeft)
		case "right":
			ds.SetDock(DockRight)
		}
	}
}

// ─── buildXAMLDockPanel ────────────────────────────────────────────────────

// buildXAMLDockPanel строит DockPanel из XAML-элемента <DockPanel>.
// Последний дочерний элемент заполняет оставшееся пространство (LastChildFill).
func buildXAMLDockPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	dp := NewDockPanel()

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if strings.EqualFold(bgStr, "transparent") {
			dp.UseAlpha = true
		} else if c, err := parseXAMLColor(bgStr); err == nil {
			dp.Background = c
			dp.UseAlpha = c.A < 255
		}
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	dp.SetBounds(absBounds)

	// Attached properties
	applyGridAttachedProps(dp, el)
	applyDockAttachedProp(dp, el)
	applyIsEnabled(dp, el)

	// Регистрация
	if id := el.name(); id != "" {
		reg[id] = dp
	}

	// Дочерние виджеты (parentOff=0 — DockPanel.layout() сам расставит)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			dp.AddChild(cw) // AddChild → layout()
		}
	}

	return dp, nil
}

// ─── buildXAMLBorder ───────────────────────────────────────────────────────

// buildXAMLBorder строит Border — контейнер с фоном/рамкой и одним потомком.
// В WPF Border.Child заполняет всю область Border.
// Реализуем через DockPanel (последний ребёнок заполняет оставшееся пространство).
func buildXAMLBorder(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	dp := NewDockPanel()

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if strings.EqualFold(bgStr, "transparent") {
			dp.UseAlpha = true
		} else if c, err := parseXAMLColor(bgStr); err == nil {
			dp.Background = c
			dp.UseAlpha = c.A < 255
		}
	} else {
		dp.UseAlpha = true
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	dp.SetBounds(absBounds)

	// Attached properties
	applyGridAttachedProps(dp, el)
	applyDockAttachedProp(dp, el)
	applyMargin(dp, el)
	applyIsEnabled(dp, el)

	// Регистрация
	if id := el.name(); id != "" {
		reg[id] = dp
	}

	// Дочерние виджеты — DockPanel.layout() заполнит последнего ребёнка.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			dp.AddChild(cw)
		}
	}

	return dp, nil
}

// ─── buildXAMLStatusBar ────────────────────────────────────────────────────

// buildXAMLStatusBar строит StatusBar как горизонтальный StackPanel.
// WPF StatusBar — набор StatusBarItem. Мы упрощаем: строим StackPanel Horizontal.
func buildXAMLStatusBar(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	sp := NewStackPanel(OrientationHorizontal)
	sp.Spacing = 10
	sp.Padding = 6

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	sp.SetBounds(absBounds)

	// Attached properties
	applyGridAttachedProps(sp, el)
	applyDockAttachedProp(sp, el)
	applyIsEnabled(sp, el)

	// Регистрация
	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние виджеты (parentOff=0 — StackPanel.layout() сам расставит)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw)
		}
	}

	return sp, nil
}

// ─── buildXAMLToolBarTray ───────────────────────────────────────────────────

// buildXAMLToolBarTray строит ToolBarTray из XAML-элемента <ToolBarTray>.
//
// WPF ToolBarTray — контейнер для одного или нескольких ToolBar.
// Реализуется как горизонтальный StackPanel, в который вкладываются
// дочерние ToolBar (каждый тоже горизонтальный StackPanel с кнопками).
//
// Поддерживаемые WPF-совместимые атрибуты:
//
//	Background — цвет фона (#RRGGBB / имя)
//	Orientation — Horizontal (default) | Vertical
func buildXAMLToolBarTray(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	sp := NewStackPanel(OrientationHorizontal)
	sp.Spacing = 0
	sp.Padding = 0

	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	sp.SetBounds(absBounds)

	// Attached properties
	applyGridAttachedProps(sp, el)
	applyDockAttachedProp(sp, el)
	applyMargin(sp, el)
	applyIsEnabled(sp, el)

	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние элементы (ToolBar-ы и другие)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw)
		}
	}

	return sp, nil
}

// ─── buildXAMLToolBar ──────────────────────────────────────────────────────

// buildXAMLToolBar строит ToolBar из XAML-элемента <ToolBar>.
//
// WPF ToolBar — горизонтальная панель с кнопками, разделителями и другими элементами.
// Реализуется как горизонтальный StackPanel с небольшим spacing.
// Separator внутри ToolBar рендерится как вертикальная линия-разделитель.
//
// Поддерживаемые WPF-совместимые атрибуты:
//
//	Background — цвет фона
//	Band       — номер полосы (игнорируется, layout упрощён)
//	BandIndex  — позиция в полосе (игнорируется)
func buildXAMLToolBar(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	sp := NewStackPanel(OrientationHorizontal)
	sp.Spacing = 2
	sp.Padding = 4

	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		// Прозрачный фон по умолчанию — ToolBar наследует фон от ToolBarTray
		sp.UseAlpha = true
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	sp.SetBounds(absBounds)

	applyGridAttachedProps(sp, el)
	applyDockAttachedProp(sp, el)
	applyMargin(sp, el)
	applyIsEnabled(sp, el)

	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние виджеты: кнопки, разделители, и т.д.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw)
		}
	}

	return sp, nil
}

// ─── buildXAMLStackPanel ────────────────────────────────────────────────────

// buildXAMLStackPanel строит StackPanel из XAML-элемента <StackPanel>.
//
// Поддерживаемые атрибуты:
//
//	Orientation  — Horizontal | Vertical (default: Vertical)
//	Background   — цвет фона (#RRGGBB / имя)
//	Spacing      — расстояние между элементами (px)
//	Padding      — внутренний отступ (px)
//	Margin       — внешний отступ (игнорируется в текущей реализации)
func buildXAMLStackPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	orient := OrientationVertical
	if strings.EqualFold(el.attr("Orientation"), "horizontal") {
		orient = OrientationHorizontal
	}

	sp := NewStackPanel(orient)

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if strings.EqualFold(bgStr, "transparent") {
			sp.UseAlpha = true
		} else if c, err := parseXAMLColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Spacing
	if s := xatoi(el.attr("Spacing")); s > 0 {
		sp.Spacing = s
	}

	// Padding
	if p := xatoi(el.attr("Padding")); p > 0 {
		sp.Padding = p
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	sp.SetBounds(absBounds) // вызовет layout(), но дети ещё не добавлены

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Attached properties
	applyGridAttachedProps(sp, el)
	applyDockAttachedProp(sp, el)
	applyMargin(sp, el)
	applyIsEnabled(sp, el)

	// Дочерние виджеты. StackPanel сам расставляет детей через layout(),
	// поэтому передаём parentOff = image.Point{} (аналогично Grid).
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// Пропускаем property elements
		if strings.Contains(childTag, ".") {
			continue
		}

		cw, err := buildXAMLWidget(child, reg, image.Point{}, baseDir)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw) // AddChild вызывает layout()
		}
	}

	return sp, nil
}

// ─── buildXAMLTreeView ─────────────────────────────────────────────────────

// buildXAMLTreeView строит TreeView из XAML-элемента <TreeView>.
//
// Рекурсивно разбирает вложенные <TreeViewItem Header="..."> в TreeNode-дерево.
//
// Поддерживаемые атрибуты:
//
//	Background   — цвет фона (#RRGGBB / имя)
//	Foreground   — цвет текста
//	ItemHeight   — высота строки (px)
func buildXAMLTreeView(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	tv := NewTreeView()

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			tv.Background = c
		}
	}

	// Foreground
	if fgStr := el.attr("Foreground"); fgStr != "" {
		if c, err := parseXAMLColor(fgStr); err == nil {
			tv.Foreground = c
		}
	}

	// ItemHeight
	if ih := xatoi(el.attr("ItemHeight")); ih > 0 {
		tv.ItemHeight = ih
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	tv.SetBounds(absBounds)

	// Attached properties
	applyGridAttachedProps(tv, el)
	applyDockAttachedProp(tv, el)
	applyMargin(tv, el)
	applyIsEnabled(tv, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = tv
	}

	// Рекурсивный парсинг TreeViewItem → TreeNode
	for _, child := range el.Children {
		if strings.EqualFold(child.Tag, "TreeViewItem") {
			node := parseTreeViewItem(child)
			tv.AddRoot(node)
		}
	}

	return tv, nil
}

// parseTreeViewItem рекурсивно строит TreeNode из <TreeViewItem>.
func parseTreeViewItem(el xElement) *TreeNode {
	header := el.attr("Header", "Text", "Content")
	if header == "" {
		header = el.Text
	}
	node := NewTreeNode(header)

	// IsExpanded
	if strings.EqualFold(el.attr("IsExpanded"), "true") {
		node.Expanded = true
	}

	// Вложенные TreeViewItem
	for _, child := range el.Children {
		if strings.EqualFold(child.Tag, "TreeViewItem") {
			node.AddChild(parseTreeViewItem(child))
		}
	}

	return node
}

// ─── buildXAMLListViewFromDataGrid ─────────────────────────────────────────

// buildXAMLListViewFromDataGrid аппроксимирует WPF <DataGrid> как ListView.
//
// DataGrid — сложный табличный виджет. Наш движок не имеет полноценной таблицы,
// поэтому мы строим ListView, заголовки колонок формируем из дочерних
// <DataGridTextColumn Header="..."/>.
func buildXAMLListViewFromDataGrid(el xElement) Widget {
	var columns []string
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		// DataGrid.Columns property element
		if childTag == "datagrid.columns" {
			for _, col := range child.Children {
				header := col.attr("Header", "Text")
				if header != "" {
					columns = append(columns, header)
				}
			}
			continue
		}
		// Прямые колонки (DataGridTextColumn и др.)
		if strings.HasPrefix(childTag, "datagridtext") ||
			strings.HasPrefix(childTag, "datagridtemplate") ||
			strings.HasPrefix(childTag, "datagridcheck") ||
			strings.HasPrefix(childTag, "datagridcombo") {
			header := child.attr("Header", "Text")
			if header != "" {
				columns = append(columns, header)
			}
		}
	}

	// Формируем строку-заголовок из названий колонок
	var items []string
	if len(columns) > 0 {
		items = append(items, strings.Join(columns, "  |  "))
	}

	lv := NewListView(items...)
	lv.ItemHeight = 26

	// Background / Foreground
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			lv.Background = c
		}
	}

	return lv
}

// ─── Парсинг цветов ──────────────────────────────────────────────────────────

// parseXAMLColor разбирает строку цвета: "#RRGGBB", "#RRGGBBAA" или именованный цвет.
// Повторно использует parseColor из loader.go для hex-значений.
func parseXAMLColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "transparent":
		return color.RGBA{}, nil
	case "white":
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}, nil
	case "black":
		return color.RGBA{A: 255}, nil
	case "red":
		return color.RGBA{R: 255, A: 255}, nil
	case "green":
		return color.RGBA{G: 128, A: 255}, nil
	case "blue":
		return color.RGBA{B: 255, A: 255}, nil
	case "gray", "grey":
		return color.RGBA{R: 128, G: 128, B: 128, A: 255}, nil
	case "lightgray", "lightgrey":
		return color.RGBA{R: 211, G: 211, B: 211, A: 255}, nil
	case "darkgray", "darkgrey":
		return color.RGBA{R: 43, G: 43, B: 43, A: 220}, nil
	case "silver":
		return color.RGBA{R: 192, G: 192, B: 192, A: 255}, nil
	case "dodgerblue", "accent":
		return color.RGBA{R: 0, G: 120, B: 215, A: 255}, nil
	case "yellow":
		return color.RGBA{R: 255, G: 255, A: 255}, nil
	case "orange":
		return color.RGBA{R: 255, G: 165, A: 255}, nil
	case "cyan", "aqua":
		return color.RGBA{G: 255, B: 255, A: 255}, nil
	case "magenta", "fuchsia":
		return color.RGBA{R: 255, B: 255, A: 255}, nil
	case "navy":
		return color.RGBA{B: 128, A: 255}, nil
	case "teal":
		return color.RGBA{G: 128, B: 128, A: 255}, nil
	case "maroon":
		return color.RGBA{R: 128, A: 255}, nil
	case "olive":
		return color.RGBA{R: 128, G: 128, A: 255}, nil
	case "purple":
		return color.RGBA{R: 128, B: 128, A: 255}, nil
	case "lime":
		return color.RGBA{G: 255, A: 255}, nil
	case "cornflowerblue":
		return color.RGBA{R: 100, G: 149, B: 237, A: 255}, nil
	case "wheat":
		return color.RGBA{R: 245, G: 222, B: 179, A: 255}, nil
	default:
		return parseColor(s) // из loader.go: "#RRGGBB" / "#RRGGBBAA"
	}
}
