// xaml.go — главный диспетчер XAML-виджетов и построители простых элементов.
//
// Публичный API: LoadUIFromXAMLFile, LoadUIFromXAML, LoadUIFromXAMLWithBase.
// Внутренний диспетчер: buildXAMLWidget — маршрутизирует по тегу XAML-элемента.
// Построители простых виджетов: Label, Button, TextInput, Dropdown, ProgressBar,
// Separator, CheckBox, RadioButton, Slider, ToggleSwitch, ScrollView, ListView, Image.
//
// Контейнеры (Grid, Window, Canvas, DockPanel и др.) — в xaml_containers.go.
// Парсинг XML, цветов и Margin — в xaml_parse.go.
// Применение attached-свойств — в xaml_props.go.
package widget

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	w, reg, _, err := loadUIFromXAML(data, "", nil)
	return w, reg, err
}

// LoadUIFromXAMLWithBase разбирает XAML и строит дерево виджетов.
// baseDir используется для загрузки ресурсов (BackgroundImage и пр.).
func LoadUIFromXAMLWithBase(data []byte, baseDir string) (Widget, map[string]Widget, error) {
	w, reg, _, err := loadUIFromXAML(data, baseDir, nil)
	return w, reg, err
}

// LoadUIFromXAMLWithContext разбирает XAML и привязывает {Binding Path} к
// dataContext. Привязки живые: при INotifyPropertyChanged у dataContext UI
// обновляется автоматически, TwoWay-поля пишут обратно в модель.
// Возвращаемый BindingScope удерживается подписками, поэтому привязки работают
// даже если scope не сохранён вызывающим кодом.
func LoadUIFromXAMLWithContext(data []byte, dataContext interface{}) (Widget, map[string]Widget, error) {
	w, reg, _, err := loadUIFromXAML(data, "", dataContext)
	return w, reg, err
}

// LoadUIFromXAMLBindings — как LoadUIFromXAMLWithContext, но возвращает BindingScope
// для ручного управления (SetDataContext / Refresh).
func LoadUIFromXAMLBindings(data []byte, dataContext interface{}) (Widget, map[string]Widget, *BindingScope, error) {
	return loadUIFromXAML(data, "", dataContext)
}

// LoadUIFromXAMLFileWithContext — как LoadUIFromXAMLFile, но с DataContext для {Binding}.
func LoadUIFromXAMLFileWithContext(path string, dataContext interface{}) (Widget, map[string]Widget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	w, reg, _, err := loadUIFromXAML(data, filepath.Dir(path), dataContext)
	return w, reg, err
}

// LoadUIFromXAMLFileBindings — как LoadUIFromXAMLFileWithContext, но с BindingScope.
func LoadUIFromXAMLFileBindings(path string, dataContext interface{}) (Widget, map[string]Widget, *BindingScope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	return loadUIFromXAML(data, filepath.Dir(path), dataContext)
}

// loadUIFromXAML — общая реализация: парсинг → пред-обработка (Resources/Style/
// markup/{Binding}) → построение дерева → активация живых привязок.
func loadUIFromXAML(data []byte, baseDir string, dataContext interface{}) (Widget, map[string]Widget, *BindingScope, error) {
	root, err := parseXAML(data)
	if err != nil {
		return nil, nil, nil, err
	}
	env := preprocessXAML(root, dataContext)
	registry := make(map[string]Widget)
	w, err := buildXAMLWidget(*root, registry, image.Point{}, baseDir)
	if err != nil {
		return nil, nil, nil, err
	}
	scope := newBindingScope(dataContext, env, registry, baseDir)
	scope.activate()
	resolveWindowInputCommands(w, dataContext)
	return w, registry, scope, nil
}

// resolveWindowInputCommands связывает CommandPath из Window/Canvas.InputBindings
// с объектами-командами из DataContext.
func resolveWindowInputCommands(root Widget, ctx interface{}) {
	if ctx == nil {
		return
	}
	var bindings []InputBinding
	switch t := root.(type) {
	case *Window:
		bindings = t.InputBindings
	case *Canvas:
		bindings = t.InputBindings
	default:
		return
	}
	for i := range bindings {
		p := bindings[i].CommandPath
		if p == "" {
			continue
		}
		if v, ok := dgridGetProperty(ctx, p); ok {
			if cmd, ok := v.(ICommand); ok {
				bindings[i].Command = cmd
			}
		}
	}
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

	// Ресурсы/стили обрабатываются в preprocessXAML — как виджеты не строим.
	switch tag {
	case "style", "setter", "resourcedictionary", "solidcolorbrush",
		"lineargradientbrush", "radialgradientbrush", "gradientstop", "color",
		"keybinding", "mousebinding", "controltemplate", "contentpresenter":
		return nil, nil
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

	// ── ItemsControl — список объектов по DataTemplate ──────────────────────
	// preprocessXAML разворачивает его в StackPanel (по элементам ItemsSource);
	// неразвёрнутый (нет шаблона/контекста) строится как пустой StackPanel.
	case "itemscontrol":
		return buildXAMLStackPanel(el, reg, parentOff, baseDir)

	// ── WrapPanel — контейнер с переносом ───────────────────────────────────
	case "wrappanel":
		return buildXAMLWrapPanel(el, reg, parentOff, baseDir)

	// ── UniformGrid — равномерная сетка ─────────────────────────────────────
	case "uniformgrid":
		return buildXAMLUniformGrid(el, reg, parentOff, baseDir)

	// ── GroupBox — рамка с заголовком ───────────────────────────────────────
	case "groupbox":
		return buildXAMLGroupBox(el, reg, parentOff, baseDir)

	// ── Expander — раскрывающаяся панель ────────────────────────────────────
	case "expander":
		return buildXAMLExpander(el, reg, parentOff, baseDir)

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

	// ── DockManager — зона докинга (инструментальные панели VS-стиля) ──────
	case "dockmanager":
		return buildXAMLDockManager(el, reg, parentOff, baseDir)

	// ── DockPane вне DockManager — игнорируем (валиден только как child) ───
	case "dockpane":
		return nil, nil

	// ── DockContent вне DockManager — маркер, не виджет ─────────────────────
	case "dockcontent":
		return nil, nil

	// ── GridSplitter — перетаскиваемый разделитель ячеек Grid ───────────────
	case "gridsplitter":
		s := NewGridSplitter()
		applyColor(&s.Background, el, "Background")
		w = s

	// ── SplitPanel — две панели с перетаскиваемым разделителем ──────────────
	case "splitpanel":
		return buildXAMLSplitPanel(el, reg, parentOff, baseDir)

	// ── ToolBarTray / ToolBar → горизонтальный StackPanel (WPF ToolBar) ────
	case "toolbartray":
		return buildXAMLToolBarTray(el, reg, parentOff, baseDir)
	case "toolbar":
		return buildXAMLToolBar(el, reg, parentOff, baseDir)

	// ── StatusBar → StackPanel (горизонтальный) ────────────────────────────
	case "statusbar":
		return buildXAMLStatusBar(el, reg, parentOff, baseDir)

	// ── DataGrid — полноценный табличный виджет ─────────────────────────────
	case "datagrid":
		w = buildXAMLDataGrid(el)

	// ── Border — контейнер с фоном и одним потомком ─────────────────────────
	case "border":
		return buildXAMLBorder(el, reg, parentOff, baseDir)

	// ── Canvas — контейнер с абсолютным позиционированием (WPF Canvas) ──────
	case "canvas":
		return buildXAMLCanvas(el, reg, parentOff, baseDir)

	// ── Контейнеры ──────────────────────────────────────────────────────────
	case "usercontrol",
		"panel", "viewbox", "contentcontrol", "headeredcontentcontrol":
		w = buildXAMLPanel(el, baseDir)

	// ── Текст ────────────────────────────────────────────────────────────────
	case "label", "textblock", "text", "run":
		w = buildXAMLLabel(el)

	// ── Кнопки ───────────────────────────────────────────────────────────────
	case "button", "togglebutton", "repeatbutton":
		w = buildXAMLButton(el, baseDir)

	// ── Ввод текста ──────────────────────────────────────────────────────────
	case "textbox", "textinput", "input", "richtextbox":
		// Многострочный (AcceptsReturn / TextWrapping="Wrap") → TextBox-редактор.
		if xamlWantsMultiline(el) {
			w = buildXAMLTextBox(el)
		} else {
			w = buildXAMLTextInput(el, false)
		}

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

	// ── NumericUpDown (Extended Toolkit IntegerUpDown / DoubleUpDown) ─────────
	case "numericupdown", "integerupdown", "doubleupdown":
		w = buildXAMLNumericUpDown(el)

	// ── VirtualizingItemsControl (UI-виртуализация длинных списков) ───────────
	case "virtualizingitemscontrol":
		w = buildXAMLVirtualizingItemsControl(el)

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

	// ── SVG-иконка (темизируемая векторная) ─────────────────────────────────
	case "svgicon":
		w = buildXAMLSVGIcon(el, baseDir)

	// ── Разделитель ──────────────────────────────────────────────────────────
	case "separator":
		w = buildXAMLSeparator(el)

	// ── Векторные фигуры (WPF Shapes) ────────────────────────────────────────
	case "rectangle":
		w = buildXAMLRectangleShape(el)
	case "ellipse":
		w = buildXAMLEllipse(el)
	case "line":
		return buildXAMLLine(el, reg, parentOff), nil
	case "polygon":
		return buildXAMLPolygon(el, reg, parentOff), nil
	case "polyline":
		return buildXAMLPolyline(el, reg, parentOff), nil

	// ── PopupMenu ────────────────────────────────────────────────────────────
	case "popupmenu", "contextmenu":
		return buildXAMLPopupMenu(el, reg, parentOff)

	// ── MenuBar (горизонтальное меню) ────────────────────────────────────────
	case "menu", "menubar", "mainmenu":
		return buildXAMLMenuBar(el, reg, parentOff)

	default:
		// Проверяем реестр пользовательских виджетов.
		if builder, ok := lookupCustomXAML(tag); ok {
			cw, err := builder(newXAMLAttrs(&el))
			if err != nil {
				return nil, fmt.Errorf("xaml: custom <%s>: %w", el.Tag, err)
			}
			if cw != nil {
				w = cw
				break
			}
		}
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

	// Сохраняем явные XAML Width/Height для alignment (даже если bounds пуст).
	type xamlSizeSetter interface {
		SetXAMLSize(w, h int)
	}
	if xss, ok := w.(xamlSizeSetter); ok {
		xw := xatoi(el.attr("Width"))
		xh := xatoi(el.attr("Height"))
		if xw > 0 || xh > 0 {
			xss.SetXAMLSize(xw, xh)
		}
	}

	// Attached properties: Grid.*, DockPanel.Dock, Margin, Alignment, IsEnabled,
	// ToolTip, Visibility, ShowLocaleIndicator.
	applyCommonProps(w, el)

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
		// Attached ContextMenu: <X.ContextMenu><ContextMenu>…</ContextMenu></X.ContextMenu>
		if strings.HasSuffix(childTag, ".contextmenu") {
			for _, inner := range child.Children {
				cw, err := buildXAMLWidget(inner, reg, childOff, baseDir)
				if err != nil {
					return nil, err
				}
				if pm, ok := cw.(*PopupMenu); ok {
					if h, ok := w.(interface{ SetContextMenu(*PopupMenu) }); ok {
						h.SetContextMenu(pm)
					}
					w.AddChild(pm) // в дереве — чтобы движок рисовал overlay и ловил клики
				}
			}
			continue
		}
		if strings.Contains(childTag, ".") {
			// Ресурсы уже собраны в preprocessXAML — не строим как виджеты.
			if strings.HasSuffix(childTag, ".resources") {
				continue
			}
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

// ─── Построители простых виджетов ──────────────────────────────────────────

func buildXAMLImage(el xElement) Widget {
	iw := NewImageWidget()
	if src := el.attr("Source"); src != "" {
		if err := iw.SetSource(src); err != nil {
			log.Printf("xaml: Image Source=%q: %v", src, err)
		}
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

	// TextWrapping="Wrap" или TextWrapping="WrapWithOverflow"
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

	// FontWeight / FontStyle / TextDecorations (P1)
	if fw := strings.ToLower(el.attr("FontWeight")); fw == "bold" || fw == "semibold" || fw == "black" || fw == "heavy" {
		lbl.Bold = true
	}
	if fs := strings.ToLower(el.attr("FontStyle")); fs == "italic" || fs == "oblique" {
		lbl.Italic = true
	}
	if td := strings.ToLower(el.attr("TextDecorations")); strings.Contains(td, "underline") {
		lbl.Underline = true
	}

	// Padding
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
			lbl.PaddingX = xatoi(strings.TrimSpace(parts[0]))
			lbl.PaddingY = xatoi(strings.TrimSpace(parts[1]))
		}
	}

	return lbl
}

func buildXAMLButton(el xElement, baseDir string) Widget {
	text := el.attr("Content", "Text")
	if text == "" {
		text = el.Text
	}

	// ── WPF Content Model: парсим вложенные элементы ───────────────────────
	// <Button> может содержать <StackPanel> с <Image> + <TextBlock>,
	// или напрямую <Image> / <TextBlock> как дочерние элементы.
	bc := extractButtonContent(el)
	if text == "" {
		text = bc.Text
	}
	// Иконка из атрибута имеет приоритет
	iconSrc := el.attr("Icon", "IconSource")
	if iconSrc == "" {
		iconSrc = bc.IconSrc
	}
	style := strings.ToLower(el.attr("Tag", "Style"))
	var btn *Button
	if style == "accent" || style == "primary" {
		btn = NewWin10AccentButton(text)
	} else {
		btn = NewButton(text)
	}

	applyColor(&btn.Background, el, "Background")
	applyColor(&btn.TextColor, el, "Foreground")
	applyColor(&btn.HoverBG, el, "HoverBG", "HoverBackground")
	applyColor(&btn.PressedBG, el, "PressedBG", "PressedBackground")
	applyColor(&btn.BorderColor, el, "BorderBrush")

	// ── ToolTip ────────────────────────────────────────────────────────────
	if tip := el.attr("ToolTip"); tip != "" {
		btn.ToolTip = tip
	}

	// ── Padding ────────────────────────────────────────────────────────────
	if pad := el.attr("Padding"); pad != "" {
		btn.Padding = parseMargin(pad)
	}

	// ── CornerRadius ───────────────────────────────────────────────────────
	if cr := xatoi(el.attr("CornerRadius")); cr > 0 {
		btn.CornerRadius = cr
	}

	// ── Иконка ─────────────────────────────────────────────────────────────
	if iconSrc != "" {
		path := iconSrc
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, iconSrc)
		}
		if img, err := loadImageFile(path); err == nil {
			btn.Icon = img
			btn.IconPath = iconSrc
		}
	}
	// Размер иконки из вложенного <Image Width="..." Height="...">
	if btn.IconSize <= 0 && bc.IconW > 0 {
		btn.IconSize = bc.IconW
	}
	if btn.IconSize <= 0 && bc.IconH > 0 {
		btn.IconSize = bc.IconH
	}
	// Foreground из вложенного TextBlock (если не задан на самой кнопке)
	if bc.Foreground != "" && el.attr("Foreground") == "" {
		if c, err := parseXAMLColor(bc.Foreground); err == nil {
			btn.TextColor = c
		}
	}

	switch strings.ToLower(el.attr("IconPosition", "IconPos")) {
	case "top":
		btn.IconPos = IconTop
	case "only", "icononly":
		btn.IconPos = IconOnly
	}
	if sz := xatoi(el.attr("IconSize")); sz > 0 {
		btn.IconSize = sz
	}

	return btn
}

// buttonContent содержит информацию, извлечённую из дочерних элементов Button.
type buttonContent struct {
	Text       string     // текст из TextBlock/Label
	IconSrc    string     // Source из Image
	IconW      int        // Width из Image
	IconH      int        // Height из Image
	Foreground string     // Foreground из TextBlock (для цвета текста кнопки)
}

// extractButtonContent рекурсивно обходит дочерние элементы Button,
// извлекая текст из TextBlock и путь к иконке из Image.
// Поддерживает WPF Content Model: <Button> → <StackPanel> → <Image> + <TextBlock>.
func extractButtonContent(el xElement) buttonContent {
	var bc buttonContent
	extractButtonContentRec(el, &bc)
	return bc
}

func extractButtonContentRec(el xElement, bc *buttonContent) {
	for _, child := range el.Children {
		tag := strings.ToLower(child.Tag)
		if strings.Contains(tag, ".") {
			continue
		}
		switch tag {
		case "textblock", "label", "run":
			if bc.Text == "" {
				if t := child.attr("Text", "Content"); t != "" {
					bc.Text = t
				} else if child.Text != "" {
					bc.Text = child.Text
				}
			}
			if bc.Foreground == "" {
				bc.Foreground = child.attr("Foreground")
			}
		case "image":
			if bc.IconSrc == "" {
				if src := child.attr("Source", "Src"); src != "" {
					bc.IconSrc = src
				}
			}
			if bc.IconW == 0 {
				bc.IconW = xatoi(child.attr("Width"))
			}
			if bc.IconH == 0 {
				bc.IconH = xatoi(child.attr("Height"))
			}
		case "stackpanel", "wrappanel", "dockpanel", "grid":
			extractButtonContentRec(child, bc)
		}
	}
}

func buildXAMLTextInput(el xElement, isPassword bool) Widget {
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
	if strings.EqualFold(el.attr("Focused", "IsFocused", "Focus"), "true") {
		ti.SetFocused(true)
	}
	applyColor(&ti.TextColor, el, "Foreground")
	applyColor(&ti.Background, el, "Background")

	if ff := el.attr("FontFamily"); ff != "" {
		ti.FontName = ff
	}
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			ti.FontSize = v
		}
	}
	if strings.EqualFold(el.attr("AcceptsReturn"), "true") {
		ti.AcceptsReturn = true
	}
	if wrap := strings.ToLower(el.attr("TextWrapping")); wrap == "wrap" || wrap == "wrapwithoverflow" {
		ti.AcceptsReturn = true
	}
	if ml := el.attr("MaxLength"); ml != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(ml)); err == nil && v > 0 {
			ti.MaxLength = v
		}
	}

	return ti
}

// xamlWantsMultiline — тег текстового поля описывает многострочный редактор.
func xamlWantsMultiline(el xElement) bool {
	if strings.EqualFold(el.attr("AcceptsReturn"), "true") {
		return true
	}
	wrap := strings.ToLower(el.attr("TextWrapping"))
	return wrap == "wrap" || wrap == "wrapwithoverflow"
}

// buildXAMLTextBox создаёт многострочный TextBox из XAML-элемента.
func buildXAMLTextBox(el xElement) Widget {
	tb := NewTextBox(el.attr("Tag", "Placeholder", "PlaceholderText", "Hint"))

	wrap := strings.ToLower(el.attr("TextWrapping"))
	tb.Wrap = wrap == "" || wrap == "wrap" || wrap == "wrapwithoverflow"
	if strings.EqualFold(el.attr("IsReadOnly", "ReadOnly"), "true") {
		tb.ReadOnly = true
	}
	if text := el.attr("Text"); text != "" {
		tb.SetText(text)
	}
	if strings.EqualFold(el.attr("Focused", "IsFocused", "Focus"), "true") {
		tb.SetFocused(true)
	}
	applyColor(&tb.TextColor, el, "Foreground")
	applyColor(&tb.Background, el, "Background")
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			tb.FontSize = v
		}
	}
	return tb
}

func buildXAMLDropdown(el xElement) Widget {
	var items []string
	var keys []string // ключи {Loc …} по индексам items (пустая строка — нет)
	if raw := el.attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				text, key := locItemText(s)
				items = append(items, text)
				keys = append(keys, key)
			}
		}
	}
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		if t == "item" || t == "comboboxitem" || t == "listboxitem" {
			v := child.attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				text, key := locItemText(v)
				items = append(items, text)
				keys = append(keys, key)
			}
		}
	}

	dd := NewDropdown(items...)
	// Элементы списка — не виджеты, поэтому обновляем их сами: при смене
	// языка перечитываем переводы и переустанавливаем весь список.
	registerLocItemList(keys, func(i int, s string) {
		cur := dd.Items()
		if i < len(cur) {
			cur[i] = s
			dd.SetItems(cur)
		}
	})
	if sel := el.attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			dd.SetSelected(idx)
		}
	}
	return dd
}

func buildXAMLProgressBar(el xElement) Widget {
	pb := NewProgressBar()
	applyColor(&pb.FillColor, el, "Foreground", "Fill")
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

	w := xatoi(el.attr("Width"))
	h := xatoi(el.attr("Height"))
	if w <= 0 && h <= 0 {
		p.SetBounds(image.Rect(0, 0, 1, 24))
	} else if w > 0 && h <= 0 {
		p.SetBounds(image.Rect(0, 0, w, 1))
	} else if h > 0 && w <= 0 {
		p.SetBounds(image.Rect(0, 0, 1, h))
	} else if w > 0 && h > 0 {
		p.SetBounds(image.Rect(0, 0, w, h))
	}
	return p
}

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

func buildXAMLVirtualizingItemsControl(el xElement) Widget {
	v := NewVirtualizingItemsControl()
	if ih := el.attr("ItemHeight"); ih != "" {
		if n := xatoi(ih); n > 0 {
			v.ItemHeight = n
		}
	}
	if bf := el.attr("Buffer"); bf != "" {
		if n := xatoi(bf); n >= 0 {
			v.Buffer = n
		}
	}
	if strings.EqualFold(el.attr("ShowBorder"), "false") {
		v.ShowBorder = false
	}
	applyColor(&v.Background, el, "Background")
	return v
}

func buildXAMLNumericUpDown(el xElement) Widget {
	n := NewNumericUpDown()
	if min := el.attr("Minimum", "Min"); min != "" {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			n.Min = v
		}
	}
	if max := el.attr("Maximum", "Max"); max != "" {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			n.Max = v
		}
	}
	if inc := el.attr("Increment", "Step"); inc != "" {
		if v, err := strconv.ParseFloat(inc, 64); err == nil {
			n.Step = v
		}
	}
	// Decimals напрямую, либо вывод из FormatString вида "F2".
	if d := el.attr("Decimals", "DecimalPlaces"); d != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(d)); err == nil && v >= 0 {
			n.Decimals = v
		}
	} else if fs := el.attr("FormatString", "StringFormat"); len(fs) >= 2 {
		if c := fs[0]; c == 'F' || c == 'f' || c == 'N' || c == 'n' {
			if v, err := strconv.Atoi(fs[1:]); err == nil && v >= 0 {
				n.Decimals = v
			}
		}
	}
	if val := el.attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			n.SetValue(v)
		}
	}
	return n
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
	applyColor(&sv.Background, el, "Background")
	return sv
}

func buildXAMLListView(el xElement) Widget {
	var items []string
	if raw := el.attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				items = append(items, s)
			}
		}
	}
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		switch {
		case t == "item" || t == "listviewitem" || t == "listboxitem":
			v := child.attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				items = append(items, v)
			}
		case t == "textblock" || t == "label" || t == "run":
			// WPF: <ListView><TextBlock>text</TextBlock></ListView>
			v := child.attr("Text")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				items = append(items, v)
			}
		}
	}
	lv := NewListView(items...)

	applyColor(&lv.Background, el, "Background")

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

// buildXAMLSplitPanel строит SplitPanel: две панели с перетаскиваемым
// разделителем. Первый дочерний элемент — First, второй — Second.
func buildXAMLSplitPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string) (Widget, error) {
	orient := OrientationHorizontal
	if strings.EqualFold(el.attr("Orientation"), "vertical") {
		orient = OrientationVertical
	}
	sp := NewSplitPanel(orient)

	if fs := el.attr("Position"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v >= 0 && v <= 1 {
			sp.Position = v
		}
	}
	if v := xatoi(el.attr("SplitterSize")); v > 0 {
		sp.SplitterSize = v
	}
	if v := xatoi(el.attr("MinFirst")); v > 0 {
		sp.MinFirst = v
	}
	if v := xatoi(el.attr("MinSecond")); v > 0 {
		sp.MinSecond = v
	}
	applyColor(&sp.Background, el, "Background")
	applyColor(&sp.HoverColor, el, "HoverColor")

	absBounds := el.bounds().Add(parentOff)
	sp.SetBounds(absBounds)
	applyCommonProps(sp, el)
	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние виджеты: layout SplitPanel сам расставит первых двух.
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

// buildXAMLSVGIcon строит темизируемую векторную иконку из .svg-файла.
// Без явного Color иконка перекрашивается под цвет текста темы.
func buildXAMLSVGIcon(el xElement, baseDir string) Widget {
	ic := NewSVGIcon()
	if src := el.attr("Source"); src != "" {
		p := src
		if baseDir != "" && !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		if err := ic.SetSVGFile(p); err != nil {
			log.Printf("xaml: SVGIcon Source=%q: %v", src, err)
		}
	}
	var c color.RGBA
	applyColor(&c, el, "Color", "Foreground", "Fill")
	if c.A > 0 {
		ic.SetColor(c)
	}
	if strings.EqualFold(el.attr("Tint"), "true") {
		ic.SetTint(true)
	}
	return ic
}
