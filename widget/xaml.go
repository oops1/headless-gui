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
	"os"
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
	return LoadUIFromXAML(data)
}

// LoadUIFromXAML разбирает XAML и строит дерево виджетов.
func LoadUIFromXAML(data []byte) (Widget, map[string]Widget, error) {
	root, err := parseXAML(data)
	if err != nil {
		return nil, nil, err
	}
	registry := make(map[string]Widget)
	w, err := buildXAMLWidget(*root, registry, image.Point{})
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
func buildXAMLWidget(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	tag := strings.ToLower(el.Tag)

	// Игнорируем теги-свойства WPF (Panel.Children, Grid.RowDefinitions, …)
	if strings.Contains(tag, ".") {
		return nil, nil // пропускаем как дочерний виджет
	}

	var w Widget

	switch tag {
	// ── Контейнеры ──────────────────────────────────────────────────────────
	// window / usercontrol — корневые элементы WPF/Blend; трактуем как Canvas.
	case "window", "usercontrol",
		"panel", "canvas", "grid", "stackpanel", "border", "dockpanel", "viewbox":
		w = buildXAMLPanel(el)

	// ── Текст ────────────────────────────────────────────────────────────────
	case "label", "textblock", "text", "run":
		w = buildXAMLLabel(el)

	// ── Кнопки ───────────────────────────────────────────────────────────────
	case "button", "togglebutton", "repeatbutton":
		w = buildXAMLButton(el)

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
		return buildXAMLTabControl(el, reg, parentOff)

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

	default:
		return nil, fmt.Errorf("xaml: неизвестный элемент <%s>", el.Tag)
	}

	// Координаты в XAML относительны родительского Canvas/Panel (стандарт WPF).
	// Прибавляем parentOff чтобы получить абсолютные экранные координаты.
	absBounds := el.bounds().Add(parentOff)
	w.SetBounds(absBounds)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = w
	}

	// Смещение для дочерних виджетов — Min текущего элемента.
	childOff := absBounds.Min

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
				cw, err := buildXAMLWidget(inner, reg, childOff)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					w.AddChild(cw)
				}
			}
			continue
		}
		cw, err := buildXAMLWidget(child, reg, childOff)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			w.AddChild(cw)
		}
	}

	return w, nil
}

// ─── Построители конкретных виджетов ────────────────────────────────────────

func buildXAMLPanel(el xElement) Widget {
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

	return p
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

	return lbl
}

func buildXAMLButton(el xElement) Widget {
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

func buildXAMLTabControl(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	tc := NewTabControl()
	absBounds := el.bounds().Add(parentOff)
	tc.SetBounds(absBounds)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = tc
	}

	childOff := absBounds.Min

	// Обрабатываем TabItem дочерние элементы
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag == "tabitem" {
			header := child.attr("Header", "Content", "Text")
			if header == "" {
				header = child.Text
			}

			// Содержимое вкладки — первый дочерний элемент TabItem
			var content Widget
			for _, inner := range child.Children {
				innerTag := strings.ToLower(inner.Tag)
				if strings.Contains(innerTag, ".") {
					continue
				}
				cw, err := buildXAMLWidget(inner, reg, childOff)
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
			cw, err := buildXAMLWidget(child, reg, childOff)
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
	case "darkgray", "darkgrey":
		return color.RGBA{R: 43, G: 43, B: 43, A: 220}, nil
	case "dodgerblue", "accent":
		return color.RGBA{R: 0, G: 120, B: 215, A: 255}, nil
	default:
		return parseColor(s) // из loader.go: "#RRGGBB" / "#RRGGBBAA"
	}
}
