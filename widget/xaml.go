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

// ─── Построители простых виджетов ──────────────────────────────────────────

func buildXAMLImage(el xElement) Widget {
	iw := NewImageWidget()
	if src := el.attr("Source"); src != "" {
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

	return ti
}

func buildXAMLDropdown(el xElement) Widget {
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
