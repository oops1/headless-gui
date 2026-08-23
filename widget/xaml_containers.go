// xaml_containers.go — XAML-построители контейнерных виджетов.
//
// Grid, Window, Canvas, Panel, DockPanel, Border, StackPanel,
// ToolBar, StatusBar, TabControl, MenuBar, PopupMenu, TreeView.
package widget

import (
	"image"
	"image/color"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
	svgPkg "github.com/oops1/headless-gui/v3/widget/svg"
	tvPkg "github.com/oops1/headless-gui/v3/widget/treeview"
)

// applyXAMLSizeAttr сохраняет явные XAML Width/Height (для alignment).
// Спец-билдеры контейнеров не проходят общий путь buildXAMLWidgetAt,
// где это делается автоматически.
func applyXAMLSizeAttr(w Widget, el xElement) {
	type xamlSizeSetter interface {
		SetXAMLSize(w, h int)
	}
	if xss, ok := w.(xamlSizeSetter); ok {
		if xw, xh := xatoi(el.attr("Width")), xatoi(el.attr("Height")); xw > 0 || xh > 0 {
			xss.SetXAMLSize(xw, xh)
		}
	}
}

// ─── buildXAMLGrid ─────────────────────────────────────────────────────────

// buildXAMLGrid создаёт Grid из XAML-элемента, парсит RowDefinitions/ColumnDefinitions,
// создаёт потомков и вызывает layout.
func buildXAMLGrid(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(g, el)
	applyXAMLSizeAttr(g, el)

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
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			g.AddChild(cw)
		}
	}

	// Перезапускаем layout с добавленными потомками.
	g.layout()

	// Передаём GridSplitter'ам ссылку на родительский Grid (для drag-resize).
	for _, child := range g.children {
		if sp, ok := child.(*GridSplitter); ok {
			sp.SetGrid(g)
		}
	}

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
func buildXAMLWindow(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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

	// MinWidth / MinHeight — минимальный размер окна (логические пиксели).
	if mw := xatoi(el.attr("MinWidth")); mw > 0 {
		win.MinWidth = mw
	}
	if mh := xatoi(el.attr("MinHeight")); mh > 0 {
		win.MinHeight = mh
	}

	// MainWindow: True (default) | False. У главного окна рисуется XOR-рамка;
	// вложенные окна (MainWindow="False") её не получают.
	if mw := el.attr("MainWindow"); mw != "" {
		win.MainWindow = strings.EqualFold(mw, "true") || mw == "1"
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

	// ── Трей: иконка + подсказка (декларация; window.Window подхватит в Run) ──
	// TrayIcon — путь относительно baseDir (.png/.jpg декодируется, .svg
	// растеризуется 32×32). Ошибка загрузки — log.Printf и пропуск.
	if ti := el.attr("TrayIcon"); ti != "" {
		if img := loadTrayIcon(ti, baseDir); img != nil {
			win.TrayIconImage = img
		}
	}
	// TrayTooltip — по умолчанию = Title.
	if tt := el.attr("TrayTooltip"); tt != "" {
		win.TrayTooltip = tt
	} else {
		win.TrayTooltip = win.Title
	}

	// Bounds (с учётом parentOff — обычно 0,0 для корня)
	absBounds := b.Add(parentOff)
	win.SetBounds(absBounds)
	applyIsEnabled(win, el)
	applyToolTip(win, el)
	applyVisibility(win, el)
	applyLocaleIndicator(win, el)
	win.InputBindings = parseInputBindings(el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = win
	}

	// Дочерние виджеты размещаются относительно ContentBounds.
	contentOff := win.ContentBounds().Min
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// ── <TrayMenu> — контекстное меню трея (единственное) ────────────────
		// Строим существующим механизмом popup-пунктов (buildXAMLPopupMenu) и
		// кладём в ПОЛЕ Window.TrayMenu, НЕ добавляя ребёнком дерева: PopupMenu
		// прямым ребёнком Window опасен (см. Window.SetBounds skip *PopupMenu);
		// window.attachTrayMenu добавит его в дерево правильно уже в Run().
		if childTag == "traymenu" {
			cw, err := buildXAMLPopupMenu(child, reg, contentOff)
			if err != nil {
				return nil, err
			}
			if pm, ok := cw.(*PopupMenu); ok {
				win.TrayMenu = pm
			}
			continue
		}

		// Пропускаем property elements
		if strings.Contains(childTag, ".") {
			// Ресурсы и InputBindings уже собраны отдельно — не строим как виджеты.
			if strings.HasSuffix(childTag, ".resources") || strings.HasSuffix(childTag, ".inputbindings") {
				continue
			}
			// Но обрабатываем потомков property element (например Window.Content)
			for _, inner := range child.Children {
				cw, err := buildXAMLWidgetAt(inner, reg, contentOff, baseDir, depth+1)
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

		cw, err := buildXAMLWidgetAt(child, reg, contentOff, baseDir, depth+1)
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

// parseInputBindings разбирает <Window.InputBindings><KeyBinding .../>.
// Command может быть {Binding Path} (резолвится после загрузки) или пусто.
func parseInputBindings(el xElement) []InputBinding {
	var out []InputBinding
	for i := range el.Children {
		c := &el.Children[i]
		if !strings.HasSuffix(strings.ToLower(c.Tag), ".inputbindings") {
			continue
		}
		for j := range c.Children {
			kb := &c.Children[j]
			if !strings.EqualFold(kb.Tag, "KeyBinding") {
				continue
			}
			ib := InputBinding{
				Key:  parseKeyName(kb.attr("Key", "Gesture")),
				Mods: parseModifiers(kb.attr("Modifiers")),
			}
			// Gesture="Ctrl+S" — комбинированная запись.
			if g := kb.attr("Gesture"); g != "" && strings.Contains(g, "+") {
				parts := strings.Split(g, "+")
				ib.Key = parseKeyName(parts[len(parts)-1])
				ib.Mods = parseModifiers(g)
			}
			cmd := kb.attr("Command")
			if strings.HasPrefix(strings.TrimSpace(cmd), "{Binding") {
				ib.CommandPath = parseBindingPath(cmd)
			}
			if ib.Key != KeyUnknown {
				out = append(out, ib)
			}
		}
	}
	return out
}

// loadTrayIcon загружает иконку трея из файла src (относительно baseDir):
//   - .png/.jpg/.jpeg → декодируется как есть (loadImageFile);
//   - .svg            → растеризуется в 32×32; свои цвета документа сохраняются,
//     currentColor подставляется цветом текста темы (win10.LabelText), tint=false
//     (для трея не темизируем монохромно — берём оригинальные цвета SVG).
//
// Ошибка загрузки/разбора — log.Printf и nil (иконка просто не ставится).
func loadTrayIcon(src, baseDir string) image.Image {
	// Путь удерживается внутри каталога XAML-файла (SEC-8).
	path, rerr := resolveXAMLResource(baseDir, src)
	if rerr != nil {
		log.Printf("xaml: TrayIcon Source=%q: %v", src, rerr)
		return nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		doc, err := svgPkg.ParseFile(path)
		if err != nil {
			log.Printf("xaml: TrayIcon Source=%q: %v", src, err)
			return nil
		}
		img := doc.Rasterize(32, 32, win10.LabelText, false)
		if img == nil {
			log.Printf("xaml: TrayIcon Source=%q: пустая растеризация", src)
			return nil
		}
		return img
	default:
		img, err := loadImageFile(path)
		if err != nil {
			log.Printf("xaml: TrayIcon Source=%q: %v", src, err)
			return nil
		}
		return img
	}
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

	// ── Градиентный фон (LinearGradientBrush через property-element) ─────────
	if g := el.attr("__gradient"); g != "" {
		if grad := parseGradient(g); grad != nil {
			p.Gradient = grad
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
	// Путь удерживается внутри baseDir (SEC-8); при загрузке из строки
	// (baseDir == "") фон, как и раньше, не подхватывается.
	if bgImg := el.attr("BackgroundImage"); bgImg != "" && baseDir != "" {
		if imgPath, err := resolveXAMLResource(baseDir, bgImg); err == nil {
			if img, err := loadImageFile(imgPath); err == nil {
				p.BackgroundImage = img
			}
		}
	}

	return p
}

// ─── Canvas builder ─────────────────────────────────────────────────────────

// buildXAMLCanvas строит Canvas виджет из XAML-элемента.
// Canvas размещает дочерние виджеты по абсолютным координатам (Canvas.Left, Canvas.Top, и т.д.).
// Это полноценный аналог WPF Canvas, в отличие от Panel — Canvas сам управляет layout.
func buildXAMLCanvas(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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

	// Attached properties: Grid.Row/Column, DockPanel.Dock, Margin, ToolTip, …
	applyCommonProps(cv, el)
	applyXAMLSizeAttr(cv, el)
	cv.InputBindings = parseInputBindings(el)

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
				if err := addCanvasChild(cv, inner, reg, zeroOff, baseDir, depth); err != nil {
					return nil, err
				}
			}
			continue
		}

		if err := addCanvasChild(cv, child, reg, zeroOff, baseDir, depth); err != nil {
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
func addCanvasChild(cv *Canvas, child xElement, reg map[string]Widget, canvasOff image.Point, baseDir string, depth int) error {
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
	cw, err := buildXAMLWidgetAt(child, reg, canvasOff, baseDir, depth+1)
	if err != nil {
		return err
	}
	if cw == nil {
		return nil
	}

	// Если Width/Height не были заданы явно в XAML, попробуем взять
	// из bounds, которые buildXAMLWidget мог установить.
	// При якоре Right/Bottom bounds не годятся: el.bounds() читает
	// Right/Bottom как координаты прямоугольника и даёт мусорный размер;
	// оставляем 0 → layoutChild применит дефолт или вычислит по якорям.
	if desiredW <= 0 && props.Right < 0 {
		desiredW = cw.Bounds().Dx()
	}
	if desiredH <= 0 && props.Bottom < 0 {
		desiredH = cw.Bounds().Dy()
	}

	// Не сбрасываем bounds — Canvas.layoutChild пересчитает позицию
	// и сдвинет потомков на правильную дельту через shiftDescendants.
	// Если сбросить bounds в (0,0), дельта будет неверной для контейнеров.

	cv.AddChildAt(cw, props, desiredW, desiredH)
	return nil
}

// ─── buildXAMLTabControl ────────────────────────────────────────────────────

func buildXAMLTabControl(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	tc := NewTabControl()
	absBounds := el.bounds().Add(parentOff)
	tc.SetBounds(absBounds)
	// Общие свойства (включая Grid.Row/Column — без этого TabControl как прямой
	// потомок Grid игнорировал ячейку и рисовался от верха окна; см. BUG-1).
	applyCommonProps(tc, el)
	applyXAMLSizeAttr(tc, el)

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
				cw, err := buildXAMLWidgetAt(inner, reg, contentOff, baseDir, depth+1)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					content = cw
					break
				}
			}
			// Заголовок может быть {Loc Ключ}: переводим и запоминаем, как
			// обновить его при смене языка (индекс вкладки уже известен).
			text, key := locItemText(header)
			idx := tc.TabCount()
			tc.AddTab(text, content)
			registerLocItem(key, func(s string) { tc.SetTabHeader(idx, s) })
			if tip := child.attr("ToolTip", "Tooltip"); tip != "" {
				tipText, tipKey := locItemText(tip)
				tc.SetTabToolTip(idx, tipText)
				registerLocItem(tipKey, func(s string) { tc.SetTabToolTip(idx, s) })
			}
			if sep := child.attr("SeparatorBefore"); strings.EqualFold(sep, "true") || sep == "1" {
				tc.SetTabSeparator(idx, true)
			}
		} else if !strings.Contains(childTag, ".") {
			// Обычные дочерние виджеты (не TabItem)
			cw, err := buildXAMLWidgetAt(child, reg, contentOff, baseDir, depth+1)
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
	applyCommonProps(mb, el)

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

		// Рекурсивно собираем подпункты. Их надписи тоже могут быть {Loc …},
		// поэтому parseMenuItems возвращает ещё и ключи — по индексам.
		subItems, subKeys := parseMenuItems(child)
		text, key := locItemText(header)
		top := mb.MenuCount()
		mb.AddMenu(text, subItems...)
		registerLocItem(key, func(s string) { mb.SetMenuText(top, s) })
		for i, k := range subKeys {
			i := i
			registerLocItem(k, func(s string) { mb.SetSubItemText(top, i, s) })
		}
	}

	return mb, nil
}

// parseMenuItems рекурсивно собирает MenuItem из дочерних <MenuItem>.
// Второй результат — ключи перевода надписей (по индексам items): пункты меню
// не являются виджетами, поэтому {Loc …} для них разворачивает вызывающий код.
func parseMenuItems(parent xElement) ([]MenuItem, []string) {
	var items []MenuItem
	var keys []string // ключ перевода для каждого пункта («» — не локализуемый)
	for _, sub := range parent.Children {
		subTag := strings.ToLower(sub.Tag)
		if subTag != "menuitem" && subTag != "item" {
			continue
		}

		sep := strings.EqualFold(sub.attr("Separator"), "True")
		if sep {
			items = append(items, MenuItem{Separator: true})
			keys = append(keys, "")
			continue
		}

		raw := sub.attr("Header", "Text", "Content")
		if raw == "" {
			raw = sub.Text
		}
		text, key := locItemText(raw)

		disabled := strings.EqualFold(sub.attr("IsEnabled"), "False") ||
			strings.EqualFold(sub.attr("Disabled"), "True")

		item := MenuItem{
			Text:     text,
			Disabled: disabled,
		}

		// Рекурсивные подменю (3+ уровень). Их надписи локализуются
		// одноразово: путь до подпункта третьего уровня MenuBar менять не
		// умеет, а в разметке проекта такие меню пока не встречаются.
		if len(sub.Children) > 0 {
			item.SubItems, _ = parseMenuItems(sub)
		}

		items = append(items, item)
		keys = append(keys, key)
	}
	return items, keys
}

// ─── PopupMenu ──────────────────────────────────────────────────────────────

func buildXAMLPopupMenu(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	pm := NewPopupMenu()
	absBounds := el.bounds().Add(parentOff)
	pm.SetBounds(absBounds)
	applyCommonProps(pm, el)

	if id := el.name(); id != "" {
		reg[id] = pm
	}

	// Парсим дочерние <MenuItem> элементы.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		// Отдельный тег <Separator/> — горизонтальный разделитель (в дополнение к
		// <MenuItem Separator="True"/>). Удобно для трей-меню и контекстных меню.
		if childTag == "separator" {
			pm.AddSeparator()
			continue
		}
		if childTag != "menuitem" && childTag != "item" {
			continue
		}

		sep := strings.EqualFold(child.attr("Separator"), "True")
		if sep {
			pm.AddSeparator()
			continue
		}

		raw := child.attr("Header", "Text", "Content")
		if raw == "" {
			raw = child.Text
		}
		// Пункт контекстного меню — не виджет, поэтому {Loc …} разворачиваем
		// здесь и запоминаем, как обновить надпись при смене языка.
		text, locKey := locItemText(raw)

		disabled := strings.EqualFold(child.attr("IsEnabled"), "False") ||
			strings.EqualFold(child.attr("Disabled"), "True")

		item := MenuItem{
			Text:     text,
			Disabled: disabled,
		}
		pm.mu.Lock()
		idx := len(pm.items)
		pm.items = append(pm.items, item)
		pm.mu.Unlock()
		registerLocItem(locKey, func(s string) { pm.SetItemText(idx, s) })
	}

	return pm, nil
}

// ─── buildXAMLDockPanel ────────────────────────────────────────────────────

// buildXAMLDockPanel строит DockPanel из XAML-элемента <DockPanel>.
// Последний дочерний элемент заполняет оставшееся пространство (LastChildFill).
func buildXAMLDockPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(dp, el)

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
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
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
func buildXAMLBorder(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(dp, el)

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
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			dp.AddChild(cw)
		}
	}

	return dp, nil
}

// ─── buildXAMLDockManager ──────────────────────────────────────────────────
//
// <DockManager>
//   <DockPane Id="tools" Title="Инструменты" Side="Left" Size="220" State="Docked">
//     …контент (один child)…
//   </DockPane>
//   <DockPane Id="props" Title="Свойства" Side="Right" Size="200"/>
//   <DockContent>…центр (документная область, один child)…</DockContent>
// </DockManager>
//
// DockManager раскладывает докинг-панели (widget/dockpane.go) вокруг
// документной области. <DockPane> пришвартовывается к своей стороне (Side,
// дефолт Left) через AddPane; Size задаёт SetSideSize для этой стороны
// (несколько панелей одной стороны — Size последней выигрывает); State
// переводит панель в AutoHidden/Floating/Closed сразу после добавления
// (Docked — состояние по умолчанию, вызовов не требует). <DockContent> —
// не виджет, а маркер: его единственный ребёнок становится SetCenter.

var xamlDockPaneAutoSeq int

// xamlDockPaneID возвращает Id панели: явный Id/Name, иначе слаг от Title,
// иначе "paneN" (автогенерация — на случай, если XAML не задал ни того ни
// другого).
func xamlDockPaneID(el xElement) string {
	if id := el.attr("Id", "Name", "x:Name"); id != "" {
		return id
	}
	if title := el.attr("Title"); title != "" {
		return xamlSlugify(title)
	}
	xamlDockPaneAutoSeq++
	return "pane" + strconv.Itoa(xamlDockPaneAutoSeq)
}

// xamlSlugify огрубляет произвольную строку до идентификатора: буквы/цифры
// латиницы и кириллицы в нижнем регистре, остальное → "-".
func xamlSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r >= 'а' && r <= 'я', r == 'ё':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		xamlDockPaneAutoSeq++
		return "pane" + strconv.Itoa(xamlDockPaneAutoSeq)
	}
	return out
}

// xamlDockSide парсит атрибут Side="Left|Top|Bottom|Right" (дефолт Left).
func xamlDockSide(s string) DockSide {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "top":
		return DockTop
	case "bottom":
		return DockBottom
	case "right":
		return DockRight
	default:
		return DockLeft
	}
}

// buildXAMLDockPane строит DockPane из <DockPane> внутри <DockManager>.
// Возвращает панель и её запрошенные Side/Size/State — сам DockManager
// решает, что с ними делать (AddPane/SetSideSize/Unpin·Float·Close).
func buildXAMLDockPane(el xElement, reg map[string]Widget, baseDir string, depth int) (pane *DockPane, side DockSide, size int, state string, err error) {
	id := xamlDockPaneID(el)
	title := el.attr("Title")
	if title == "" {
		title = id
	}

	// Содержимое — первый дочерний виджет (кроме WPF property-тегов "X.Y").
	var content Widget
	for _, child := range el.Children {
		if strings.Contains(child.Tag, ".") {
			continue
		}
		cw, cerr := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if cerr != nil {
			return nil, DockLeft, 0, "", cerr
		}
		if cw != nil {
			content = cw
			break
		}
	}

	pane = NewDockPane(id, title, content)
	side = xamlDockSide(el.attr("Side"))
	size = xatoi(el.attr("Size"))
	state = strings.ToLower(strings.TrimSpace(el.attr("State")))

	if name := el.attr("Name", "x:Name"); name != "" {
		reg[name] = pane
	} else {
		reg[id] = pane
	}

	return pane, side, size, state, nil
}

// buildXAMLDockManager строит DockManager из <DockManager> (см. схему выше).
func buildXAMLDockManager(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	dm := NewDockManager()

	if bgStr := el.attr("Background"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			dm.Background = c
		}
	}

	// NativeFloating="True" — декларация нативного отрыва панелей (window.Window
	// подхватит поле в Run → EnableDockFloating). В headless — без эффекта.
	if strings.EqualFold(el.attr("NativeFloating"), "true") {
		dm.NativeFloating = true
	}

	absBounds := el.bounds().Add(parentOff)
	dm.SetBounds(absBounds)
	applyCommonProps(dm, el)

	if id := el.name(); id != "" {
		reg[id] = dm
	}

	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		switch childTag {
		case "dockpane":
			p, side, size, state, err := buildXAMLDockPane(child, reg, baseDir, depth)
			if err != nil {
				return nil, err
			}
			if p == nil {
				continue
			}
			dm.AddPane(p, side)
			if size > 0 {
				dm.SetSideSize(side, size)
			}
			switch state {
			case "autohidden":
				p.Unpin()
			case "floating":
				p.Float()
			case "closed":
				p.Close()
			}

		case "dockcontent":
			var content Widget
			for _, inner := range child.Children {
				if strings.Contains(inner.Tag, ".") {
					continue
				}
				cw, err := buildXAMLWidgetAt(inner, reg, image.Point{}, baseDir, depth+1)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					content = cw
					break
				}
			}
			dm.SetCenter(content)

		default:
			if strings.Contains(childTag, ".") {
				continue
			}
			// Запасной путь: виджет без обёртки DockContent/DockPane, ещё не
			// заданный центр — трактуем как центр (удобно для беглых правок XAML).
			if dm.Center() == nil {
				cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					dm.SetCenter(cw)
				}
			}
		}
	}

	return dm, nil
}

// ─── buildXAMLStatusBar ────────────────────────────────────────────────────

// buildXAMLStatusBar строит StatusBar как горизонтальный StackPanel.
// WPF StatusBar — набор StatusBarItem. Мы упрощаем: строим StackPanel Horizontal.
func buildXAMLStatusBar(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(sp, el)

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
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
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
func buildXAMLToolBarTray(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(sp, el)

	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние элементы (ToolBar-ы и другие)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
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
func buildXAMLToolBar(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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

	applyCommonProps(sp, el)

	if id := el.name(); id != "" {
		reg[id] = sp
	}

	// Дочерние виджеты: кнопки, разделители, и т.д.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
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
func buildXAMLStackPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
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
	applyCommonProps(sp, el)

	// Дочерние виджеты. StackPanel сам расставляет детей через layout(),
	// поэтому передаём parentOff = image.Point{} (аналогично Grid).
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// Пропускаем property elements
		if strings.Contains(childTag, ".") {
			continue
		}

		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw) // AddChild вызывает layout()
		}
	}

	return sp, nil
}

// ─── buildXAMLGroupBox ──────────────────────────────────────────────────────

// buildXAMLGroupBox строит GroupBox (Header + один контент).
func buildXAMLGroupBox(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	header := el.attr("Header", "Title")
	gb := NewGroupBox(header)
	if bgStr := el.attr("Background"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := parseXAMLColor(bgStr); err == nil {
			gb.Background = c
		}
	}
	applyColor(&gb.BorderColor, el, "BorderBrush")
	applyColor(&gb.HeaderColor, el, "Foreground")
	gb.SetBounds(el.bounds().Add(parentOff))
	applyCommonProps(gb, el)
	if id := el.name(); id != "" {
		reg[id] = gb
	}
	contentOff := gb.ContentBounds().Min
	for _, child := range el.Children {
		ct := strings.ToLower(child.Tag)
		if strings.Contains(ct, ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, contentOff, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			gb.AddChild(cw)
		}
	}
	cb := gb.ContentBounds()
	for _, c := range gb.Children() {
		if c.Bounds().Empty() {
			c.SetBounds(cb)
		}
	}
	return gb, nil
}

// ─── buildXAMLExpander ──────────────────────────────────────────────────────

// buildXAMLExpander строит Expander (Header, IsExpanded + контент).
func buildXAMLExpander(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	header := el.attr("Header", "Title")
	ex := NewExpander(header)
	if strings.EqualFold(el.attr("IsExpanded"), "true") {
		ex.IsExpanded = true
	}
	applyColor(&ex.HeaderBG, el, "Background")
	applyColor(&ex.TextColor, el, "Foreground")
	ex.SetBounds(el.bounds().Add(parentOff))
	applyCommonProps(ex, el)
	if id := el.name(); id != "" {
		reg[id] = ex
	}
	contentOff := image.Pt(0, 0)
	if cb := ex.ContentBounds(); !cb.Empty() {
		contentOff = cb.Min
	}
	for _, child := range el.Children {
		ct := strings.ToLower(child.Tag)
		if strings.Contains(ct, ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, contentOff, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			ex.AddChild(cw)
		}
	}
	ex.SetBounds(ex.Bounds()) // разложить контент по ContentBounds
	return ex, nil
}

// ─── buildXAMLWrapPanel ─────────────────────────────────────────────────────

// buildXAMLWrapPanel строит WrapPanel из XAML (Orientation, Background, Spacing).
func buildXAMLWrapPanel(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	orient := OrientationHorizontal
	if strings.EqualFold(el.attr("Orientation"), "vertical") {
		orient = OrientationVertical
	}
	wp := NewWrapPanel(orient)
	if bgStr := el.attr("Background", "Fill"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := parseXAMLColor(bgStr); err == nil {
			wp.Background = c
			wp.UseAlpha = c.A < 255
		}
	}
	if s := xatoi(el.attr("Spacing", "ItemSpacing")); s > 0 {
		wp.Spacing = s
		wp.LineSpacing = s
	}
	if s := xatoi(el.attr("LineSpacing")); s > 0 {
		wp.LineSpacing = s
	}
	if p := xatoi(el.attr("Padding")); p > 0 {
		wp.Padding = p
	}
	wp.SetBounds(el.bounds().Add(parentOff))
	applyCommonProps(wp, el)
	if id := el.name(); id != "" {
		reg[id] = wp
	}
	for _, child := range el.Children {
		if strings.Contains(strings.ToLower(child.Tag), ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			wp.AddChild(cw)
		}
	}
	return wp, nil
}

// ─── buildXAMLUniformGrid ───────────────────────────────────────────────────

// buildXAMLUniformGrid строит UniformGrid из XAML (Rows, Columns, Background).
func buildXAMLUniformGrid(el xElement, reg map[string]Widget, parentOff image.Point, baseDir string, depth int) (Widget, error) {
	ug := NewUniformGrid()
	ug.Rows = xatoi(el.attr("Rows"))
	ug.Columns = xatoi(el.attr("Columns"))
	if bgStr := el.attr("Background", "Fill"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := parseXAMLColor(bgStr); err == nil {
			ug.Background = c
			ug.UseAlpha = c.A < 255
		}
	}
	if s := xatoi(el.attr("Spacing")); s > 0 {
		ug.Spacing = s
	}
	ug.SetBounds(el.bounds().Add(parentOff))
	applyCommonProps(ug, el)
	if id := el.name(); id != "" {
		reg[id] = ug
	}
	for _, child := range el.Children {
		if strings.Contains(strings.ToLower(child.Tag), ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			ug.AddChild(cw)
		}
	}
	return ug, nil
}

// ─── buildXAMLTreeView ─────────────────────────────────────────────────────

// buildXAMLTreeView строит TreeViewWidget из XAML-элемента <TreeView>.
//
// Поддерживаемые WPF-совместимые атрибуты:
//
//	Background           — цвет фона (#RRGGBB / имя)
//	Foreground           — цвет текста
//	ItemHeight           — высота строки (px)
//	IndentSize           — отступ уровня вложенности (px)
//	IsReadOnly           — только чтение (True/False)
//	ShowIndentGuides     — показывать линии иерархии (True/False)
//
// Вложенные элементы:
//
//	<TreeViewItem>       — статические узлы дерева
//	<TreeView.ItemTemplate> — HierarchicalDataTemplate для data binding
func buildXAMLTreeView(el xElement, reg map[string]Widget, parentOff image.Point) (Widget, error) {
	tw := NewTreeViewWidget()

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			tw.Tree.Theme.Background = c
		}
	}

	// Foreground
	if fgStr := el.attr("Foreground"); fgStr != "" {
		if c, err := parseXAMLColor(fgStr); err == nil {
			tw.Tree.Theme.Foreground = c
		}
	}

	// ItemHeight
	if ih := xatoi(el.attr("ItemHeight")); ih > 0 {
		tw.Tree.ItemHeight = ih
	}

	// IndentSize
	if is := xatoi(el.attr("IndentSize")); is > 0 {
		tw.Tree.IndentSize = is
	}

	// IsReadOnly
	if strings.EqualFold(el.attr("IsReadOnly"), "true") {
		tw.Tree.IsReadOnly = true
	}

	// ShowIndentGuides
	if strings.EqualFold(el.attr("ShowIndentGuides"), "true") {
		tw.Tree.ShowIndentGuides = true
	}

	// Bounds
	absBounds := el.bounds().Add(parentOff)
	tw.SetBounds(absBounds)

	// Attached properties
	applyCommonProps(tw, el)

	// Регистрация по имени
	if id := el.name(); id != "" {
		reg[id] = tw
	}

	// Рекурсивный парсинг дочерних элементов
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		switch {
		case childTag == "treeviewitem":
			item := parseTreeViewItemNew(child)
			tw.Tree.AddRoot(item)

		case childTag == "treeview.itemtemplate":
			// <TreeView.ItemTemplate> → HierarchicalDataTemplate
			for _, tmplEl := range child.Children {
				if strings.EqualFold(tmplEl.Tag, "HierarchicalDataTemplate") {
					tmpl := parseHierarchicalDataTemplate(tmplEl)
					tw.Tree.SetItemTemplate(tmpl)
				}
			}
		}
	}

	return tw, nil
}

// parseTreeViewItemNew рекурсивно строит TreeViewItem из <TreeViewItem>.
func parseTreeViewItemNew(el xElement) *tvPkg.TreeViewItem {
	header := el.attr("Header", "Text", "Content")
	if header == "" {
		header = el.Text
	}
	item := tvPkg.NewItem(header)

	// IsExpanded
	if strings.EqualFold(el.attr("IsExpanded"), "true") {
		item.Expanded = true
	}

	// IsEnabled
	if strings.EqualFold(el.attr("IsEnabled"), "false") {
		item.IsEnabled = false
	}

	// Вложенные TreeViewItem
	for _, child := range el.Children {
		if strings.EqualFold(child.Tag, "TreeViewItem") {
			item.AddChild(parseTreeViewItemNew(child))
		}
	}

	return item
}

// parseHierarchicalDataTemplate парсит <HierarchicalDataTemplate> из XAML.
func parseHierarchicalDataTemplate(el xElement) *tvPkg.HierarchicalDataTemplate {
	tmpl := &tvPkg.HierarchicalDataTemplate{}

	// ItemsSource="{Binding Children}"
	if is := el.attr("ItemsSource"); is != "" {
		tmpl.ItemsSourcePath = parseBindingPath(is)
	}

	// Ищем вложенные элементы для определения HeaderPath и IconPath
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		switch {
		case childTag == "stackpanel":
			// <StackPanel Orientation="Horizontal">
			//   <Image Source="{Binding Icon}"/>
			//   <TextBlock Text="{Binding Name}"/>
			for _, inner := range child.Children {
				innerTag := strings.ToLower(inner.Tag)
				switch innerTag {
				case "image":
					if src := inner.attr("Source"); src != "" {
						tmpl.IconPath = parseBindingPath(src)
					}
				case "textblock":
					if txt := inner.attr("Text"); txt != "" {
						tmpl.HeaderPath = parseBindingPath(txt)
					}
				}
			}

		case childTag == "textblock":
			// Прямой TextBlock как содержимое шаблона
			if txt := child.attr("Text"); txt != "" {
				tmpl.HeaderPath = parseBindingPath(txt)
			}
		}
	}

	return tmpl
}

// ─── buildXAMLDataGrid ─────────────────────────────────────────────────────

// buildXAMLDataGrid строит полноценный DataGrid из XAML-элемента <DataGrid>.
//
// Поддерживаемые WPF-совместимые атрибуты:
//
//	AutoGenerateColumns — автогенерация колонок из модели (True/False)
//	IsReadOnly          — только чтение (True/False)
//	CanUserSortColumns  — сортировка по клику на заголовок (True/False)
//	CanUserResizeColumns — изменение ширины колонок мышью (True/False)
//	SelectionMode       — Single | Extended
//	RowHeight           — высота строки (пиксели)
//	HeaderHeight        — высота заголовка (пиксели)
//	Background          — цвет фона
//
// Колонки объявляются внутри <DataGrid.Columns>:
//
//	<DataGridTextColumn Header="Name" Binding="{Binding Name}" Width="*" />
//	<DataGridCheckBoxColumn Header="Active" Binding="{Binding IsActive}" Width="60" />
//	<DataGridTemplateColumn Header="Actions" Width="100" />
func buildXAMLDataGrid(el xElement) Widget {
	dg := NewDataGridWidget()

	// ── Свойства ────────────────────────────────────────────────────────

	// AutoGenerateColumns
	if strings.EqualFold(el.attr("AutoGenerateColumns"), "true") {
		dg.Grid.AutoGenerateColumns = true
	}

	// IsReadOnly
	if strings.EqualFold(el.attr("IsReadOnly"), "true") {
		dg.Grid.IsReadOnly = true
	}

	// CanUserSortColumns (по умолчанию true)
	if strings.EqualFold(el.attr("CanUserSortColumns"), "false") {
		dg.Grid.CanUserSortColumns = false
	}

	// CanUserResizeColumns (по умолчанию true)
	if strings.EqualFold(el.attr("CanUserResizeColumns"), "false") {
		dg.Grid.CanUserResizeColumns = false
	}

	// SelectionMode
	if strings.EqualFold(el.attr("SelectionMode"), "extended") {
		dg.Grid.SelectionMode = dgridPkg.SelectionExtended
	}

	// RowHeight
	if rh := xatoi(el.attr("RowHeight")); rh > 0 {
		dg.Grid.RowHeight = rh
	}

	// HeaderHeight
	if hh := xatoi(el.attr("HeaderHeight")); hh > 0 {
		dg.Grid.HeaderHeight = hh
	}

	// Background
	if bgStr := el.attr("Background", "Fill"); bgStr != "" {
		if c, err := parseXAMLColor(bgStr); err == nil {
			dg.Grid.Background = c
		}
	}

	// ── Колонки ─────────────────────────────────────────────────────────

	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// <DataGrid.Columns> property element
		if childTag == "datagrid.columns" {
			for _, colEl := range child.Children {
				col := parseDataGridColumn(colEl)
				if col != nil {
					dg.Grid.AddColumn(col)
				}
			}
			continue
		}

		// Прямые колонки (DataGridTextColumn и др.) — альтернативный синтаксис
		col := parseDataGridColumn(child)
		if col != nil {
			dg.Grid.AddColumn(col)
		}
	}

	return dg
}

// parseDataGridColumn парсит один элемент-колонку из XAML.
func parseDataGridColumn(el xElement) dgridPkg.Column {
	tag := strings.ToLower(el.Tag)
	header := el.attr("Header", "Text")

	// Binding path: разбираем "{Binding PropertyName}"
	bindingPath := parseBindingPath(el.attr("Binding"))
	if bindingPath == "" {
		bindingPath = el.attr("SortMemberPath")
	}

	// Width: "Auto", "*", "2*", "150"
	width := parseColumnWidth(el.attr("Width"))

	// IsReadOnly: tri-state.
	//   - атрибут отсутствует → колонка НАСЛЕДУЕТ значение DataGrid.IsReadOnly
	//   - IsReadOnly="True"  → жёстко RO (перекрывает grid.IsReadOnly=false)
	//   - IsReadOnly="False" → жёстко editable (перекрывает grid.IsReadOnly=true)
	//
	// applyReadOnly выставляет SetReadOnly(...) только если атрибут
	// действительно присутствует в XAML, и пропускает в противном случае.
	roAttr := el.attr("IsReadOnly")
	applyReadOnly := func(setROFn func(bool)) {
		if roAttr == "" {
			return
		}
		setROFn(strings.EqualFold(roAttr, "true"))
	}

	// SortMemberPath
	sortPath := el.attr("SortMemberPath")
	if sortPath == "" {
		sortPath = bindingPath
	}

	switch {
	case strings.HasPrefix(tag, "datagridtextcolumn"),
		strings.HasPrefix(tag, "datagridtext"):
		col := dgridPkg.NewTextColumn(header, bindingPath)
		col.SetWidth(width)
		applyReadOnly(col.SetReadOnly)
		if sortPath != "" {
			col.SetSortPath(sortPath)
		}
		return col

	case strings.HasPrefix(tag, "datagridcheckboxcolumn"),
		strings.HasPrefix(tag, "datagridcheckbox"):
		col := dgridPkg.NewCheckBoxColumn(header, bindingPath)
		col.SetWidth(width)
		applyReadOnly(col.SetReadOnly)
		return col

	case strings.HasPrefix(tag, "datagridtemplatecolumn"),
		strings.HasPrefix(tag, "datagridtemplate"):
		col := dgridPkg.NewTemplateColumn(header, nil)
		col.SetWidth(width)
		applyReadOnly(col.SetReadOnly)
		return col
	}

	return nil
}

// parseBindingPath извлекает путь из WPF binding-синтаксиса.
// "{Binding Name}" → "Name"
// "{Binding Path=User.Name}" → "User.Name"
// "Name" → "Name" (прямое указание без скобок)
func parseBindingPath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Удаляем { } если есть
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = strings.TrimPrefix(s, "{")
		s = strings.TrimSuffix(s, "}")
		s = strings.TrimSpace(s)
	}

	// Удаляем "Binding " префикс
	if strings.HasPrefix(s, "Binding ") {
		s = strings.TrimPrefix(s, "Binding ")
		s = strings.TrimSpace(s)
	} else if s == "Binding" {
		return ""
	}

	// Проверяем Path=
	if strings.HasPrefix(s, "Path=") {
		s = strings.TrimPrefix(s, "Path=")
		// Может содержать запятую (другие параметры binding)
		if idx := strings.Index(s, ","); idx >= 0 {
			s = s[:idx]
		}
		return strings.TrimSpace(s)
	}

	// Может содержать запятую (Mode=TwoWay и т.д.)
	if idx := strings.Index(s, ","); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}

// parseColumnWidth парсит ширину колонки: "Auto", "*", "2*", "150".
func parseColumnWidth(s string) dgridPkg.ColumnWidth {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "auto") {
		return dgridPkg.AutoWidth()
	}
	if s == "*" {
		return dgridPkg.StarWidth(1)
	}
	if strings.HasSuffix(s, "*") {
		numStr := strings.TrimSuffix(s, "*")
		if n := xatoi(numStr); n > 0 {
			return dgridPkg.StarWidth(float64(n))
		}
		return dgridPkg.StarWidth(1)
	}
	if n := xatoi(s); n > 0 {
		return dgridPkg.PixelWidth(float64(n))
	}
	return dgridPkg.StarWidth(1)
}
