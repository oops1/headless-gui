// xaml_containers.go — XAML-построители контейнерных виджетов.
//
// Grid, Window, Canvas, Panel, DockPanel, Border, StackPanel,
// ToolBar, StatusBar, TabControl, MenuBar, PopupMenu, TreeView.
package widget

import (
	"image"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
)

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

// ─── buildXAMLWindow ───────────────────────────────────────────────────────

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
//	TitleStyle       — Auto | Win | Mac  (расширение; WPF не имеет; Auto = по ОС)
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

	// TitleStyle: Auto (default, по ОС) | Win | Mac
	switch strings.ToLower(el.attr("TitleStyle")) {
	case "win", "windows":
		win.TitleStyle = WindowTitleWin
	case "mac", "macos":
		win.TitleStyle = WindowTitleMac
	default:
		win.TitleStyle = WindowTitleAuto // авто-определение по ОС
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

// ─── buildXAMLPanel ────────────────────────────────────────────────────────

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
	// TitleStyle: Auto (default) | Win | Mac
	// Также поддерживается legacy-атрибут MacStyle="True"
	switch strings.ToLower(el.attr("TitleStyle")) {
	case "win", "windows":
		p.TitleStyle = WindowTitleWin
	case "mac", "macos":
		p.TitleStyle = WindowTitleMac
	default:
		// Fallback: legacy-атрибут MacStyle
		if ms := el.attr("MacStyle"); ms != "" {
			p.MacStyle = strings.EqualFold(ms, "true") || ms == "1"
		}
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

// ─── buildXAMLTabControl ────────────────────────────────────────────────────

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
			// Кнопки в ToolBar получают скруглённые углы по умолчанию
			if btn, ok := cw.(*Button); ok && btn.CornerRadius == 0 {
				btn.CornerRadius = 4
			}
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
