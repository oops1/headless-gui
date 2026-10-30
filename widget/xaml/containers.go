// containers.go — XAML-построители контейнерных виджетов.
//
// Grid, Window, Canvas, Panel, DockPanel, Border, StackPanel,
// ToolBar, StatusBar, TabControl, MenuBar, PopupMenu, TreeView.
package xaml

import (
	"image"
	"image/color"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
	dgridPkg "github.com/oops1/headless-gui/v4/widget/datagrid"
	svgPkg "github.com/oops1/headless-gui/v4/widget/svg"
	tvPkg "github.com/oops1/headless-gui/v4/widget/treeview"
)

// applyXAMLSizeAttr сохраняет явные XAML Width/Height (для alignment).
// Спец-билдеры контейнеров не проходят общий путь buildXAMLWidgetAt,
// где это делается автоматически.
func applyXAMLSizeAttr(w base.Widget, el Element) {
	type xamlSizeSetter interface {
		SetXAMLSize(w, h int)
	}
	if xss, ok := w.(xamlSizeSetter); ok {
		if xw, xh := ParseInt(el.Attr("Width")), ParseInt(el.Attr("Height")); xw > 0 || xh > 0 {
			xss.SetXAMLSize(xw, xh)
		}
	}
}

// ─── buildXAMLGrid ─────────────────────────────────────────────────────────

// buildXAMLGrid создаёт Grid из XAML-элемента, парсит RowDefinitions/ColumnDefinitions,
// создаёт потомков и вызывает layout.
func buildXAMLGrid(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	g := base.NewGrid()

	// Фон
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if strings.EqualFold(bgStr, "transparent") {
			g.UseAlpha = true
		} else if c, err := ParseColor(bgStr); err == nil {
			g.Background = c
			g.UseAlpha = c.A < 255
		}
	} else {
		g.UseAlpha = true
	}

	// ShowGridLines
	if strings.EqualFold(el.Attr("ShowGridLines"), "true") {
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
	absBounds := el.Bounds().Add(parentOff)
	g.SetBounds(absBounds) // вызовет layout() — но дети ещё не добавлены

	// Attached properties — важно для вложенных Grid'ов внутри родительского Grid.
	ApplyCommonProps(g, el)
	applyXAMLSizeAttr(g, el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
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

	// Перезапускаем layout с добавленными потомками (границы те же —
	// SetBounds только раскладывает).
	g.SetBounds(g.Bounds())

	// Передаём GridSplitter'ам ссылку на родительский Grid (для drag-resize).
	for _, child := range g.Children() {
		if sp, ok := child.(*base.GridSplitter); ok {
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
func parseGridDef(el Element, sizeAttr string) base.GridDefinition {
	raw := strings.TrimSpace(el.Attr(sizeAttr))
	d := base.GridDefinition{Mode: base.GridSizeStar, Value: 1} // default = 1*

	if raw == "" || raw == "*" {
		// default star
	} else if strings.EqualFold(raw, "auto") {
		d.Mode = base.GridSizeAuto
		d.Value = 0
	} else if strings.HasSuffix(raw, "*") {
		d.Mode = base.GridSizeStar
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
			d.Mode = base.GridSizePixel
			d.Value = v
		}
	}

	// Min/Max
	d.Min = ParseInt(el.Attr("MinHeight", "MinWidth"))
	d.Max = ParseInt(el.Attr("MaxHeight", "MaxWidth"))

	return d
}

// ─── buildXAMLWindow ───────────────────────────────────────────────────────

// ParseInputBindings разбирает <Window.InputBindings><KeyBinding .../>.
// Command может быть {Binding Path} (резолвится после загрузки) или пусто.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ParseInputBindings(el Element) []base.InputBinding {
	var out []base.InputBinding
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
			ib := base.InputBinding{
				Key:  base.ParseKeyName(kb.Attr("Key", "Gesture")),
				Mods: base.ParseModifiers(kb.Attr("Modifiers")),
			}
			// Gesture="Ctrl+S" — комбинированная запись.
			if g := kb.Attr("Gesture"); g != "" && strings.Contains(g, "+") {
				parts := strings.Split(g, "+")
				ib.Key = base.ParseKeyName(parts[len(parts)-1])
				ib.Mods = base.ParseModifiers(g)
			}
			cmd := kb.Attr("Command")
			if strings.HasPrefix(strings.TrimSpace(cmd), "{Binding") {
				ib.CommandPath = parseBindingPath(cmd)
			}
			if ib.Key != base.KeyUnknown {
				out = append(out, ib)
			}
		}
	}
	return out
}

// LoadTrayIcon загружает иконку трея из файла src (относительно baseDir или
// из fs.FS — LoadUIFromXAMLFS, GG-7):
//   - .png/.jpg/.jpeg → декодируется как есть (decodeXAMLImage);
//   - .svg            → растеризуется в 32×32; свои цвета документа сохраняются,
//     currentColor подставляется цветом текста темы (win10.LabelText), tint=false
//     (для трея не темизируем монохромно — берём оригинальные цвета SVG).
//
// Расширение берём из src (атрибута разметки), а не из резолвленного пути —
// у fs.FS нет «пути на диске», а filepath.Ext(src) от источника не зависит.
//
// Ошибка загрузки/разбора — log.Printf и nil (иконка просто не ставится).
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func LoadTrayIcon(src, baseDir string) image.Image {
	switch strings.ToLower(filepath.Ext(src)) {
	case ".svg":
		// Путь/граница удерживаются внутри baseDir или fs.FS (SEC-8, GG-7).
		data, err := readXAMLResourceBytes(baseDir, src, svgPkg.MaxFileBytes)
		if err != nil {
			log.Printf("xaml: TrayIcon Source=%q: %v", src, err)
			return nil
		}
		doc, err := svgPkg.Parse(data)
		if err != nil {
			log.Printf("xaml: TrayIcon Source=%q: %v", src, err)
			return nil
		}
		img := doc.Rasterize(32, 32, base.CurrentTheme().LabelText, false)
		if img == nil {
			log.Printf("xaml: TrayIcon Source=%q: пустая растеризация", src)
			return nil
		}
		return img
	default:
		img, err := decodeXAMLImage(baseDir, src)
		if err != nil {
			log.Printf("xaml: TrayIcon Source=%q: %v", src, err)
			return nil
		}
		return img
	}
}

// ─── buildXAMLPanel ────────────────────────────────────────────────────────

func buildXAMLPanel(el Element, baseDir string) base.Widget {
	style := strings.ToLower(el.Attr("Tag", "Style"))
	bgStr := el.Attr("Background", "Fill", "Color")
	cr := ParseInt(el.Attr("CornerRadius"))

	var p *base.Panel

	switch style {
	case "win10":
		p = base.NewWin10Panel()
		if bgStr != "" {
			if c, err := ParseColor(bgStr); err == nil && c.A > 0 {
				p.Background = c
			}
		}
		p.CornerRadius = cr

	default:
		if bgStr == "" || strings.EqualFold(bgStr, "transparent") {
			p = base.NewPanel(color.RGBA{})
			p.UseAlpha = true
			p.CornerRadius = cr
		} else if c, err := ParseColor(bgStr); err == nil {
			p = base.NewPanel(c)
			p.UseAlpha = c.A < 255
			p.CornerRadius = cr
			if bc := el.Attr("BorderBrush"); bc != "" {
				if bc2, err := ParseColor(bc); err == nil {
					p.BorderColor = bc2
					p.ShowBorder = true
				}
			}
			if el.Attr("BorderThickness") != "" {
				p.ShowBorder = true
			}
		} else {
			p = base.NewPanel(color.RGBA{})
			p.UseAlpha = true
			p.CornerRadius = cr
		}
	}

	// ── Градиентный фон (LinearGradientBrush через property-element) ─────────
	if g := el.Attr("__gradient"); g != "" {
		if grad := parseGradient(g); grad != nil {
			p.Gradient = grad
		}
	}

	// ── Заголовок окна ──────────────────────────────────────────────────────
	if caption := el.Attr("Caption", "Title"); caption != "" {
		p.Caption = caption
	}
	// ShowHeader: по умолчанию true (задано в конструкторе).
	// XAML может явно выключить: ShowHeader="False".
	if sh := el.Attr("ShowHeader"); sh != "" {
		p.ShowHeader = strings.EqualFold(sh, "true") || sh == "1"
	}
	// TitleStyle: Auto (default) | Win | Mac
	// Также поддерживается legacy-атрибут MacStyle="True"
	switch strings.ToLower(el.Attr("TitleStyle")) {
	case "win", "windows":
		p.TitleStyle = base.WindowTitleWin
	case "mac", "macos":
		p.TitleStyle = base.WindowTitleMac
	default:
		// Fallback: legacy-атрибут MacStyle
		if ms := el.Attr("MacStyle"); ms != "" {
			p.MacStyle = strings.EqualFold(ms, "true") || ms == "1"
		}
	}
	// HeaderHeight
	if hh := ParseInt(el.Attr("HeaderHeight")); hh > 0 {
		p.HeaderHeight = hh
	}

	// BackgroundImage — фоновая картинка из файла (относительно XAML-файла)
	// или из fs.FS (LoadUIFromXAMLFS, GG-7). Путь удерживается внутри
	// источника (SEC-8); при загрузке из строки (baseDir == "") фон, как и
	// раньше, не подхватывается.
	if bgImg := el.Attr("BackgroundImage"); bgImg != "" && baseDir != "" {
		if img, err := decodeXAMLImageRGBA(baseDir, bgImg); err == nil {
			p.BackgroundImage = img
		}
	}

	return p
}

// ─── Canvas builder ─────────────────────────────────────────────────────────

// buildXAMLCanvas строит Canvas виджет из XAML-элемента.
// Canvas размещает дочерние виджеты по абсолютным координатам (Canvas.Left, Canvas.Top, и т.д.).
// Это полноценный аналог WPF Canvas, в отличие от Panel — Canvas сам управляет layout.
func buildXAMLCanvas(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	cv := base.NewCanvas()

	// Background
	if bgStr := el.Attr("Background", "Fill", "Color"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			cv.Background = c
			cv.UseAlpha = c.A < 255
		}
	}

	// ClipToBounds (WPF default = false)
	if clip := el.Attr("ClipToBounds"); clip != "" {
		cv.ClipToBounds = strings.EqualFold(clip, "true") || clip == "1"
	}

	// Bounds Canvas — абсолютные координаты
	absBounds := el.Bounds().Add(parentOff)
	cv.SetBounds(absBounds)

	// Attached properties: Grid.Row/Column, DockPanel.Dock, Margin, ToolTip, …
	ApplyCommonProps(cv, el)
	applyXAMLSizeAttr(cv, el)
	cv.InputBindings = ParseInputBindings(el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
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
func addCanvasChild(cv *base.Canvas, child Element, reg map[string]base.Widget, canvasOff image.Point, baseDir string, depth int) error {
	// ── Извлекаем Canvas attached properties ────────────────────────────────
	props := base.CanvasAttached{
		Left:   xatoiOrNeg1(child.Attr("Canvas.Left")),
		Top:    xatoiOrNeg1(child.Attr("Canvas.Top")),
		Right:  xatoiOrNeg1(child.Attr("Canvas.Right")),
		Bottom: xatoiOrNeg1(child.Attr("Canvas.Bottom")),
	}

	// Если Canvas.Left/Top не указаны, пробуем Left/Top/X/Y (упрощённый синтаксис)
	if props.Left < 0 {
		if v := child.Attr("Left", "X"); v != "" {
			props.Left = ParseInt(v)
		}
	}
	if props.Top < 0 {
		if v := child.Attr("Top", "Y"); v != "" {
			props.Top = ParseInt(v)
		}
	}
	if props.Right < 0 {
		if v := child.Attr("Right"); v != "" {
			props.Right = ParseInt(v)
		}
	}
	if props.Bottom < 0 {
		if v := child.Attr("Bottom"); v != "" {
			props.Bottom = ParseInt(v)
		}
	}

	// ── Желаемый размер из XAML атрибутов ───────────────────────────────────
	desiredW := ParseInt(child.Attr("Width"))
	desiredH := ParseInt(child.Attr("Height"))

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
	// Размера нет ни в разметке, ни в границах — спрашиваем сам виджет.
	// Без этого Canvas подставлял дефолтные 80×30, и подпись без Width
	// обрезалась по чужой коробке.
	if desiredW <= 0 || desiredH <= 0 {
		dw, dh := base.DesiredOf(cw)
		if desiredW <= 0 && props.Right < 0 {
			desiredW = dw
		}
		if desiredH <= 0 && props.Bottom < 0 {
			desiredH = dh
		}
	}

	// Не сбрасываем bounds — Canvas.layoutChild пересчитает позицию
	// и сдвинет потомков на правильную дельту через ShiftDescendants.
	// Если сбросить bounds в (0,0), дельта будет неверной для контейнеров.

	cv.AddChildAt(cw, props, desiredW, desiredH)
	return nil
}

// ─── buildXAMLTabControl ────────────────────────────────────────────────────

func buildXAMLTabControl(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	tc := base.NewTabControl()
	absBounds := el.Bounds().Add(parentOff)
	tc.SetBounds(absBounds)
	// Общие свойства (включая Grid.Row/Column — без этого TabControl как прямой
	// потомок Grid игнорировал ячейку и рисовался от верха окна; см. BUG-1).
	ApplyCommonProps(tc, el)
	applyXAMLSizeAttr(tc, el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
		reg[id] = tc
	}

	// contentOff — смещение для содержимого вкладок (ниже полосы табов).
	contentOff := image.Pt(absBounds.Min.X, absBounds.Min.Y+tc.TabHeight)

	// Обрабатываем TabItem дочерние элементы
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag == "tabitem" {
			header := child.Attr("Header", "Content", "Text")
			if header == "" {
				header = child.Text
			}

			// Содержимое вкладки — первый дочерний элемент TabItem.
			// Координаты дочерних виджетов относительно области контента (ниже табов).
			var content base.Widget
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
			text, key := LocItemText(header)
			idx := tc.TabCount()
			tc.AddTab(text, content)
			RegisterLocItem(key, func(s string) { tc.SetTabHeader(idx, s) })
			if tip := child.Attr("ToolTip", "Tooltip"); tip != "" {
				tipText, tipKey := LocItemText(tip)
				tc.SetTabToolTip(idx, tipText)
				RegisterLocItem(tipKey, func(s string) { tc.SetTabToolTip(idx, s) })
			}
			if sep := child.Attr("SeparatorBefore"); strings.EqualFold(sep, "true") || sep == "1" {
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

	if sel := el.Attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			tc.SetActive(idx)
		}
	}

	return tc, nil
}

// ─── MenuBar ────────────────────────────────────────────────────────────────

func buildXAMLMenuBar(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string) (base.Widget, error) {
	mb := base.NewMenuBar()
	absBounds := el.Bounds().Add(parentOff)
	mb.SetBounds(absBounds)

	if id := el.Name(); id != "" {
		reg[id] = mb
	}

	// Foreground
	if fgStr := el.Attr("Foreground"); fgStr != "" {
		if c, err := ParseColor(fgStr); err == nil {
			mb.TextColor = c
		}
	}

	// Background
	if bgStr := el.Attr("Background"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			mb.Background = c
		}
	}

	// Мнемоники — по требованию: в подписях бывают настоящие подчёркивания.
	if strings.EqualFold(el.Attr("UseMnemonics"), "True") {
		mb.UseMnemonics = true
	}

	// Attached properties
	ApplyCommonProps(mb, el)

	// Парсим верхнеуровневые <MenuItem Header="..."> с вложенными подпунктами.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if childTag != "menuitem" && childTag != "item" {
			continue
		}

		header := child.Attr("Header", "Text", "Content")
		if header == "" {
			header = child.Text
		}

		// Рекурсивно собираем подпункты. Их надписи тоже могут быть {Loc …},
		// поэтому parseMenuItems возвращает ещё и ключи — по индексам.
		subItems, subKeys := parseMenuItems(child, baseDir)
		text, key := LocItemText(header)
		top := mb.MenuCount()
		mb.AddMenu(text, subItems...)
		RegisterLocItem(key, func(s string) { mb.SetMenuText(top, s) })
		for i, k := range subKeys {
			i := i
			RegisterLocItem(k, func(s string) { mb.SetSubItemText(top, i, s) })
		}
	}

	return mb, nil
}

// parseMenuItems рекурсивно собирает MenuItem из дочерних <MenuItem>.
// Второй результат — ключи перевода надписей (по индексам items): пункты меню
// не являются виджетами, поэтому {Loc …} для них разворачивает вызывающий код.
func parseMenuItems(parent Element, baseDir string) ([]base.MenuItem, []string) {
	var items []base.MenuItem
	var keys []string // ключ перевода для каждого пункта («» — не локализуемый)
	for _, sub := range parent.Children {
		subTag := strings.ToLower(sub.Tag)
		if subTag != "menuitem" && subTag != "item" {
			continue
		}

		sep := strings.EqualFold(sub.Attr("Separator"), "True")
		if sep {
			items = append(items, base.MenuItem{Separator: true})
			keys = append(keys, "")
			continue
		}

		raw := sub.Attr("Header", "Text", "Content")
		if raw == "" {
			raw = sub.Text
		}
		text, key := LocItemText(raw)

		disabled := strings.EqualFold(sub.Attr("IsEnabled"), "False") ||
			strings.EqualFold(sub.Attr("Disabled"), "True")

		item := base.MenuItem{
			Text:      text,
			Disabled:  disabled,
			Checkable: menuItemCheckable(sub),
			Checked:   strings.EqualFold(sub.Attr("IsChecked"), "True"),
			// InputGestureText — имя этого атрибута в WPF; Shortcut принят
			// как более понятный синоним.
			Shortcut: sub.Attr("InputGestureText", "Shortcut"),
			// GroupName — имя WPF: пункты одной группы в одном подменю ведут
			// себя как переключатель.
			RadioGroup: sub.Attr("GroupName"),
		}
		applyMenuItemIcon(&item, sub, baseDir)

		// Рекурсивные подменю (3+ уровень). Их надписи локализуются
		// одноразово: путь до подпункта третьего уровня MenuBar менять не
		// умеет, а в разметке проекта такие меню пока не встречаются.
		if len(sub.Children) > 0 {
			item.SubItems, _ = parseMenuItems(sub, baseDir)
		}

		items = append(items, item)
		keys = append(keys, key)
	}
	return items, keys
}

// ─── PopupMenu ──────────────────────────────────────────────────────────────

func buildXAMLPopupMenu(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string) (base.Widget, error) {
	pm := base.NewPopupMenu()
	absBounds := el.Bounds().Add(parentOff)
	pm.SetBounds(absBounds)
	ApplyCommonProps(pm, el)

	if strings.EqualFold(el.Attr("UseMnemonics"), "True") {
		pm.UseMnemonics = true
	}

	if id := el.Name(); id != "" {
		reg[id] = pm
	}

	// Парсим дочерние <MenuItem> элементы. Пункты собираются целиком и
	// ставятся одним SetItems, а слушатели смены языка — после: до этого
	// SetItemText по их индексам некому звать, и перевод не затрётся копией.
	items := pm.Items()
	var locs []func()
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		// Отдельный тег <Separator/> — горизонтальный разделитель (в дополнение к
		// <MenuItem Separator="True"/>). Удобно для трей-меню и контекстных меню.
		if childTag == "separator" {
			items = append(items, base.MenuItem{Separator: true})
			continue
		}
		if childTag != "menuitem" && childTag != "item" {
			continue
		}

		sep := strings.EqualFold(child.Attr("Separator"), "True")
		if sep {
			items = append(items, base.MenuItem{Separator: true})
			continue
		}

		raw := child.Attr("Header", "Text", "Content")
		if raw == "" {
			raw = child.Text
		}
		// Пункт контекстного меню — не виджет, поэтому {Loc …} разворачиваем
		// здесь и запоминаем, как обновить надпись при смене языка.
		text, locKey := LocItemText(raw)

		disabled := strings.EqualFold(child.Attr("IsEnabled"), "False") ||
			strings.EqualFold(child.Attr("Disabled"), "True")

		item := base.MenuItem{
			Text:       text,
			Disabled:   disabled,
			Checkable:  menuItemCheckable(child),
			Checked:    strings.EqualFold(child.Attr("IsChecked"), "True"),
			RadioGroup: child.Attr("GroupName"),
			Shortcut:   child.Attr("InputGestureText", "Shortcut"),
		}
		applyMenuItemIcon(&item, child, baseDir)
		idx := len(items)
		items = append(items, item)
		locs = append(locs, func() { RegisterLocItem(locKey, func(s string) { pm.SetItemText(idx, s) }) })
	}
	pm.SetItems(items)
	for _, register := range locs {
		register()
	}

	return pm, nil
}

// ─── buildXAMLDockPanel ────────────────────────────────────────────────────

// ─── buildXAMLBorder ───────────────────────────────────────────────────────

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

// ─── buildXAMLStatusBar ────────────────────────────────────────────────────

// buildXAMLStatusBar строит StatusBar как горизонтальный StackPanel.
// WPF StatusBar — набор StatusBarItem. Мы упрощаем: строим StackPanel Horizontal.
func buildXAMLStatusBar(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	sp := base.NewStackPanel(base.OrientationHorizontal)
	sp.Spacing = 10
	sp.Padding = 6

	// Background
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	sp.SetBounds(absBounds)

	// Attached properties
	ApplyCommonProps(sp, el)

	// Регистрация
	if id := el.Name(); id != "" {
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
func buildXAMLToolBarTray(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	sp := base.NewStackPanel(base.OrientationHorizontal)
	sp.Spacing = 0
	sp.Padding = 0

	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	sp.SetBounds(absBounds)

	// Attached properties
	ApplyCommonProps(sp, el)

	if id := el.Name(); id != "" {
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
func buildXAMLToolBar(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	tb := base.NewToolBar()

	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			tb.Background = c
			tb.UseAlpha = c.A < 255
		}
	}
	// Иначе фон остаётся прозрачным: ToolBar наследует его от ToolBarTray.

	if v := el.Attr("IconsOnly"); v != "" {
		tb.IconsOnly = strings.EqualFold(v, "true") || v == "1"
	}
	if v := el.Attr("Overflow"); v != "" {
		tb.Overflow = !strings.EqualFold(v, "false") && v != "0"
	}

	ApplyCommonProps(tb, el)

	if id := el.Name(); id != "" {
		reg[id] = tb
	}

	// Дочерние виджеты: кнопки, разделители, и т.д.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		if childTag == "separator" {
			tb.AddSeparator()
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			// Кнопки в ToolBar получают скруглённые углы по умолчанию
			if btn := toolBarButtonOf(cw); btn != nil && btn.CornerRadius == 0 {
				btn.CornerRadius = 4
			}
			tb.AddChild(cw)
		}
	}

	// Границы — ПОСЛЕ детей: раскладка панели считает переполнение по ним, и
	// на пустой панели считать было бы нечего.
	tb.SetBounds(el.Bounds().Add(parentOff))

	return tb, nil
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
func buildXAMLStackPanel(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	orient := base.OrientationVertical
	if strings.EqualFold(el.Attr("Orientation"), "horizontal") {
		orient = base.OrientationHorizontal
	}

	sp := base.NewStackPanel(orient)

	// Background: цвет, "transparent" или роль темы "{Theme PanelBG}".
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if role, ok := base.ParseBackgroundRole(bgStr); ok {
			sp.SetBackgroundRole(role)
		} else if strings.EqualFold(bgStr, "transparent") {
			sp.UseAlpha = true
		} else if c, err := ParseColor(bgStr); err == nil {
			sp.Background = c
			sp.UseAlpha = c.A < 255
		}
	} else {
		sp.UseAlpha = true
	}

	// Spacing
	if s := ParseInt(el.Attr("Spacing")); s > 0 {
		sp.Spacing = s
	}

	// Padding
	if p := ParseInt(el.Attr("Padding")); p > 0 {
		sp.Padding = p
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	sp.SetBounds(absBounds) // вызовет layout(), но дети ещё не добавлены

	// Регистрация по имени
	if id := el.Name(); id != "" {
		reg[id] = sp
	}

	// Attached properties
	ApplyCommonProps(sp, el)

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
func buildXAMLGroupBox(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	header := el.Attr("Header", "Title")
	gb := base.NewGroupBox(header)
	if bgStr := el.Attr("Background"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := ParseColor(bgStr); err == nil {
			gb.Background = c
		}
	}
	ApplyColor(&gb.BorderColor, el, "BorderBrush")
	ApplyColor(&gb.HeaderColor, el, "Foreground")
	gb.SetBounds(el.Bounds().Add(parentOff))
	ApplyCommonProps(gb, el)
	if id := el.Name(); id != "" {
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
func buildXAMLExpander(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	header := el.Attr("Header", "Title")
	ex := base.NewExpander(header)
	if strings.EqualFold(el.Attr("IsExpanded"), "true") {
		ex.IsExpanded = true
	}
	ApplyColor(&ex.HeaderBG, el, "Background")
	ApplyColor(&ex.TextColor, el, "Foreground")
	ex.SetBounds(el.Bounds().Add(parentOff))
	ApplyCommonProps(ex, el)
	if id := el.Name(); id != "" {
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
func buildXAMLWrapPanel(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	orient := base.OrientationHorizontal
	if strings.EqualFold(el.Attr("Orientation"), "vertical") {
		orient = base.OrientationVertical
	}
	wp := base.NewWrapPanel(orient)
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := ParseColor(bgStr); err == nil {
			wp.Background = c
			wp.UseAlpha = c.A < 255
		}
	}
	if s := ParseInt(el.Attr("Spacing", "ItemSpacing")); s > 0 {
		wp.Spacing = s
		wp.LineSpacing = s
	}
	if s := ParseInt(el.Attr("LineSpacing")); s > 0 {
		wp.LineSpacing = s
	}
	if p := ParseInt(el.Attr("Padding")); p > 0 {
		wp.Padding = p
	}
	wp.SetBounds(el.Bounds().Add(parentOff))
	ApplyCommonProps(wp, el)
	if id := el.Name(); id != "" {
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
func buildXAMLUniformGrid(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	ug := base.NewUniformGrid()
	ug.Rows = ParseInt(el.Attr("Rows"))
	ug.Columns = ParseInt(el.Attr("Columns"))
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" && !strings.EqualFold(bgStr, "transparent") {
		if c, err := ParseColor(bgStr); err == nil {
			ug.Background = c
			ug.UseAlpha = c.A < 255
		}
	}
	if s := ParseInt(el.Attr("Spacing")); s > 0 {
		ug.Spacing = s
	}
	ug.SetBounds(el.Bounds().Add(parentOff))
	ApplyCommonProps(ug, el)
	if id := el.Name(); id != "" {
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
func buildXAMLTreeView(el Element, reg map[string]base.Widget, parentOff image.Point) (base.Widget, error) {
	tw := base.NewTreeViewWidget()

	// Background
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			tw.Tree.Theme.Background = c
		}
	}

	// Foreground
	if fgStr := el.Attr("Foreground"); fgStr != "" {
		if c, err := ParseColor(fgStr); err == nil {
			tw.Tree.Theme.Foreground = c
		}
	}

	// ItemHeight
	if ih := ParseInt(el.Attr("ItemHeight")); ih > 0 {
		tw.Tree.ItemHeight = ih
	}

	// IndentSize
	if is := ParseInt(el.Attr("IndentSize")); is > 0 {
		tw.Tree.IndentSize = is
	}

	// SelectionMode="Extended" — выбор набора узлов (GG-86).
	if sm := el.Attr("SelectionMode"); sm != "" {
		if strings.EqualFold(sm, "extended") || strings.EqualFold(sm, "multiple") {
			tw.Tree.SelectionMode = tvPkg.SelectionExtended
		}
	}

	// IsReadOnly
	if strings.EqualFold(el.Attr("IsReadOnly"), "true") {
		tw.Tree.IsReadOnly = true
	}

	// ShowIndentGuides
	if strings.EqualFold(el.Attr("ShowIndentGuides"), "true") {
		tw.Tree.ShowIndentGuides = true
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	tw.SetBounds(absBounds)

	// Attached properties
	ApplyCommonProps(tw, el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
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
func parseTreeViewItemNew(el Element) *tvPkg.TreeViewItem {
	header := el.Attr("Header", "Text", "Content")
	if header == "" {
		header = el.Text
	}
	item := tvPkg.NewItem(header)

	// IsExpanded
	if strings.EqualFold(el.Attr("IsExpanded"), "true") {
		item.Expanded = true
	}

	// IsEnabled
	if strings.EqualFold(el.Attr("IsEnabled"), "false") {
		item.IsEnabled = false
	}

	// Detail — приписка за текстом узла своим цветом (счётчик, размер).
	if d := el.Attr("Detail"); d != "" {
		item.Detail = d
	}
	if c := el.Attr("DetailColor"); c != "" {
		if col, err := ParseColor(c); err == nil {
			item.DetailColor = col
		}
	}
	if c := el.Attr("Foreground"); c != "" {
		if col, err := ParseColor(c); err == nil {
			item.Foreground = col
		}
	}
	if strings.EqualFold(el.Attr("FontWeight"), "Bold") {
		item.Bold = true
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
func parseHierarchicalDataTemplate(el Element) *tvPkg.HierarchicalDataTemplate {
	tmpl := &tvPkg.HierarchicalDataTemplate{}

	// ItemsSource="{Binding Children}"
	if is := el.Attr("ItemsSource"); is != "" {
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
					if src := inner.Attr("Source"); src != "" {
						tmpl.IconPath = parseBindingPath(src)
					}
				case "textblock":
					if txt := inner.Attr("Text"); txt != "" {
						tmpl.HeaderPath = parseBindingPath(txt)
					}
				}
			}

		case childTag == "textblock":
			// Прямой TextBlock как содержимое шаблона
			if txt := child.Attr("Text"); txt != "" {
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
func buildXAMLDataGrid(el Element) base.Widget {
	dg := base.NewDataGridWidget()

	// ── Свойства ────────────────────────────────────────────────────────

	// AutoGenerateColumns
	if strings.EqualFold(el.Attr("AutoGenerateColumns"), "true") {
		dg.Grid.AutoGenerateColumns = true
	}

	// IsReadOnly
	if strings.EqualFold(el.Attr("IsReadOnly"), "true") {
		dg.Grid.IsReadOnly = true
	}

	// CanUserSortColumns (по умолчанию true)
	if strings.EqualFold(el.Attr("CanUserSortColumns"), "false") {
		dg.Grid.CanUserSortColumns = false
	}

	// CanUserResizeColumns (по умолчанию true)
	if strings.EqualFold(el.Attr("CanUserResizeColumns"), "false") {
		dg.Grid.CanUserResizeColumns = false
	}

	// SelectionMode
	if strings.EqualFold(el.Attr("SelectionMode"), "extended") {
		dg.Grid.SelectionMode = dgridPkg.SelectionExtended
	}

	// RowHeight
	if rh := ParseInt(el.Attr("RowHeight")); rh > 0 {
		dg.Grid.RowHeight = rh
	}

	// HeaderHeight
	if hh := ParseInt(el.Attr("HeaderHeight")); hh > 0 {
		dg.Grid.HeaderHeight = hh
	}

	// Background
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := ParseColor(bgStr); err == nil {
			dg.Grid.Background = c
		}
	}

	// ── Колонки ─────────────────────────────────────────────────────────

	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// <DataGrid.Columns> property element
		if childTag == "datagrid.columns" {
			for _, colEl := range child.Children {
				col, locKey := parseDataGridColumn(colEl)
				if col != nil {
					dg.Grid.AddColumn(col)
					registerColumnHeaderLoc(col, locKey)
				}
			}
			continue
		}

		// Прямые колонки (DataGridTextColumn и др.) — альтернативный синтаксис
		col, locKey := parseDataGridColumn(child)
		if col != nil {
			dg.Grid.AddColumn(col)
			registerColumnHeaderLoc(col, locKey)
		}
	}

	return dg
}

// registerColumnHeaderLoc подписывает заголовок колонки на смену языка.
//
// Колонка — не виджет: сборщик складывает её в таблицу, и свойства, которое
// можно переустановить при смене языка, у неё снаружи нет. Поэтому заголовок
// регистрируется как «свёрнутая» строка (loc_items.go) — так же, как
// подписи вкладок и пунктов меню. Без этого {Loc …} в Header разворачивался
// один раз при загрузке и оставался на языке загрузки навсегда.
func registerColumnHeaderLoc(col dgridPkg.Column, key string) {
	if key == "" {
		return
	}
	RegisterLocItem(key, func(s string) { col.SetHeader(s) })
}

// parseDataGridColumn парсит один элемент-колонку из XAML.
//
// Второе значение — ключ {Loc …} заголовка (пустой, если заголовок обычный):
// подписаться на смену языка должен вызывающий, у которого есть таблица.
func parseDataGridColumn(el Element) (dgridPkg.Column, string) {
	tag := strings.ToLower(el.Tag)
	header, headerLocKey := LocItemText(el.Attr("Header", "Text"))

	// Binding path: разбираем "{Binding PropertyName}"
	bindingPath := parseBindingPath(el.Attr("Binding"))
	if bindingPath == "" {
		bindingPath = el.Attr("SortMemberPath")
	}

	// Width: "Auto", "*", "2*", "150"
	width := parseColumnWidth(el.Attr("Width"))

	// IsReadOnly: tri-state.
	//   - атрибут отсутствует → колонка НАСЛЕДУЕТ значение DataGrid.IsReadOnly
	//   - IsReadOnly="True"  → жёстко RO (перекрывает grid.IsReadOnly=false)
	//   - IsReadOnly="False" → жёстко editable (перекрывает grid.IsReadOnly=true)
	//
	// applyReadOnly выставляет SetReadOnly(...) только если атрибут
	// действительно присутствует в XAML, и пропускает в противном случае.
	roAttr := el.Attr("IsReadOnly")
	applyReadOnly := func(setROFn func(bool)) {
		if roAttr == "" {
			return
		}
		setROFn(strings.EqualFold(roAttr, "true"))
	}

	// SortMemberPath
	sortPath := el.Attr("SortMemberPath")
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
		return col, headerLocKey

	case strings.HasPrefix(tag, "datagridcheckboxcolumn"),
		strings.HasPrefix(tag, "datagridcheckbox"):
		col := dgridPkg.NewCheckBoxColumn(header, bindingPath)
		col.SetWidth(width)
		applyReadOnly(col.SetReadOnly)
		return col, headerLocKey

	case strings.HasPrefix(tag, "datagridtemplatecolumn"),
		strings.HasPrefix(tag, "datagridtemplate"):
		col := dgridPkg.NewTemplateColumn(header, nil)
		col.SetWidth(width)
		applyReadOnly(col.SetReadOnly)
		return col, headerLocKey
	}

	return nil, ""
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
		if n := ParseInt(numStr); n > 0 {
			return dgridPkg.StarWidth(float64(n))
		}
		return dgridPkg.StarWidth(1)
	}
	if n := ParseInt(s); n > 0 {
		return dgridPkg.PixelWidth(float64(n))
	}
	return dgridPkg.StarWidth(1)
}

// menuItemCheckable — объявлен ли пункт меню переключателем.
//
// IsCheckable="True" — прямое объявление, как в WPF. Но IsChecked="True" тоже
// делает пункт переключателем: разметка, которая ставит отметку, но забыла
// объявить пункт отмечаемым, иначе показывала бы отметку в никуда. Имя группы
// значит то же самое — переключатель без отметки всё равно переключатель.
func menuItemCheckable(el Element) bool {
	return strings.EqualFold(el.Attr("IsCheckable"), "True") ||
		strings.EqualFold(el.Attr("IsChecked"), "True") ||
		el.Attr("GroupName") != ""
}

// applyMenuItemIcon читает значок пункта меню из разметки: Icon="icons/copy.png".
//
// Путь ограничен каталогом XAML-файла или fs.FS (SEC-8, GG-7) — тем же
// механизмом, что у Button.Icon. Не прочитался — пункт остаётся без значка:
// меню без картинки понятнее, чем меню, не открывшееся из-за неё.
func applyMenuItemIcon(item *base.MenuItem, el Element, baseDir string) {
	src := el.Attr("Icon", "IconSource")
	if src == "" {
		return
	}
	if img, err := decodeXAMLImage(baseDir, src); err == nil {
		item.Icon = img
		item.IconPath = src
	}
	if sz := ParseInt(el.Attr("IconSize")); sz > 0 {
		item.IconSize = sz
	}
}

// toolBarButtonOf — кнопка элемента панели инструментов: сама кнопка или
// кнопка с меню (MenuButton); nil — не кнопка.
func toolBarButtonOf(w base.Widget) *base.Button {
	switch v := w.(type) {
	case *base.Button:
		return v
	case *base.MenuButton:
		return v.Button
	}
	return nil
}
