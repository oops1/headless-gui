package chrome

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/xaml"
)

// RegisterXAML регистрирует тег <Window> в реестре XAML r — строителем-
// контейнером (xaml.Registry.RegisterComplete): он сам строит детей, меню,
// вкладки заголовка и регистрирует имя. Фасад widget зовёт её при первой
// загрузке разметки; программа на widget/base и widget/xaml зовёт сама:
// chrome.RegisterXAML(xaml.Default()). В фасад не входит.
//
//facadegen:skip
func RegisterXAML(r *xaml.Registry) {
	r.RegisterComplete(func(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
		return buildXAMLWindow(*el, l)
	}, "window")
}

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
func buildXAMLWindow(el xaml.Element, l *xaml.Loader) (base.Widget, error) {
	baseDir, parentOff := l.BaseDir(), l.Offset()
	b := el.Bounds()
	if b.Empty() {
		b = image.Rect(0, 0, 800, 600) // default
	}
	win := NewWindow(el.Attr("Title", "Caption"), b.Dx(), b.Dy())

	// WindowStyle: SingleBorderWindow (default) | None | ToolWindow
	switch strings.ToLower(el.Attr("WindowStyle")) {
	case "none":
		win.Style = WindowStyleNone
	case "toolwindow":
		win.Style = WindowStyleToolWindow
	default:
		win.Style = WindowStyleSingleBorder
	}

	// TitleStyle: Auto (default, по ОС) | Win | Mac
	switch strings.ToLower(el.Attr("TitleStyle")) {
	case "win", "windows":
		win.TitleStyle = base.WindowTitleWin
	case "mac", "macos":
		win.TitleStyle = base.WindowTitleMac
	default:
		win.TitleStyle = base.WindowTitleAuto // авто-определение по ОС
	}

	// ResizeMode: CanResize (default) | NoResize | CanMinimize
	switch strings.ToLower(el.Attr("ResizeMode")) {
	case "noresize":
		win.Resize = ResizeModeNoResize
	case "canminimize":
		win.Resize = ResizeModeCanMinimize
	default:
		win.Resize = ResizeModeCanResize
	}

	// MinWidth / MinHeight — минимальный размер окна (логические пиксели).
	if mw := xaml.ParseInt(el.Attr("MinWidth")); mw > 0 {
		win.MinWidth = mw
	}
	if mh := xaml.ParseInt(el.Attr("MinHeight")); mh > 0 {
		win.MinHeight = mh
	}

	// MainWindow: True (default) | False. У главного окна рисуется XOR-рамка;
	// вложенные окна (MainWindow="False") её не получают.
	if mw := el.Attr("MainWindow"); mw != "" {
		win.MainWindow = strings.EqualFold(mw, "true") || mw == "1"
	}

	// Background
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if c, err := xaml.ParseColor(bgStr); err == nil {
			win.Background = c
		}
	}

	// BorderBrush
	if bc := el.Attr("BorderBrush"); bc != "" {
		if c, err := xaml.ParseColor(bc); err == nil {
			win.BorderColor = c
		}
	}

	// CornerRadius
	if cr := xaml.ParseInt(el.Attr("CornerRadius")); cr > 0 {
		win.CornerRadius = cr
	}

	// TitleBarHeight
	if h := xaml.ParseInt(el.Attr("TitleBarHeight")); h > 0 {
		win.TitleBarHeight = h
	}

	// TitleBackground / TitleForeground
	if tbg := el.Attr("TitleBackground"); tbg != "" {
		if c, err := xaml.ParseColor(tbg); err == nil {
			win.TitleBG = c
		}
	}
	if tfc := el.Attr("TitleForeground"); tfc != "" {
		if c, err := xaml.ParseColor(tfc); err == nil {
			win.TitleColor = c
		}
	}

	// OutlineDrag="True" — таскать за заголовок контур, а не само окно
	// (аналог «не показывать содержимое окна при перетаскивании»).
	if od := el.Attr("OutlineDrag"); strings.EqualFold(od, "true") || od == "1" {
		win.OutlineDrag = true
	}
	if ofc := el.Attr("OutlineDragFill"); ofc != "" {
		if c, err := xaml.ParseColor(ofc); err == nil {
			win.OutlineDragFill = c
		}
	}
	// OutlineDragStyle: Border (по умолчанию) | Filled.
	if strings.EqualFold(el.Attr("OutlineDragStyle"), "filled") {
		win.OutlineDragStyle = OutlineDragFilled
	}

	// TitleTabs="True" — вкладки в заголовке окна (стиль Windows 11 Terminal).
	// Вкладки задаются дочерними <TabItem>, меню шеврона — <TitleTabsMenu>.
	if ttv := el.Attr("TitleTabs"); strings.EqualFold(ttv, "true") || ttv == "1" {
		win.EnableTitleTabs()
	}

	// ── Трей: иконка + подсказка (декларация; window.Window подхватит в Run) ──
	// TrayIcon — путь относительно baseDir (.png/.jpg декодируется, .svg
	// растеризуется 32×32). Ошибка загрузки — log.Printf и пропуск.
	if ti := el.Attr("TrayIcon"); ti != "" {
		if img := xaml.LoadTrayIcon(ti, baseDir); img != nil {
			win.TrayIconImage = img
		}
	}
	// TrayTooltip — по умолчанию = Title.
	if tt := el.Attr("TrayTooltip"); tt != "" {
		win.TrayTooltip = tt
	} else {
		win.TrayTooltip = win.Title
	}

	// Bounds (с учётом parentOff — обычно 0,0 для корня)
	absBounds := b.Add(parentOff)
	win.SetBounds(absBounds)
	xaml.ApplyIsEnabled(win, el)
	xaml.ApplyToolTip(win, el)
	xaml.ApplyVisibility(win, el)
	if v := el.Attr("ShowLocaleIndicator", "ShowLocale"); v != "" {
		win.ShowLocaleIndicator = strings.EqualFold(v, "true") || v == "1"
	}
	win.InputBindings = xaml.ParseInputBindings(el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
		l.SetName(id, win)
	}

	// Дочерние виджеты размещаются относительно ContentBounds.
	contentOff := win.ContentBounds().Min
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)

		// ── <TrayMenu> — контекстное меню трея (единственное) ────────────────
		// Строим существующим механизмом popup-пунктов (Loader.BuildPopupMenu) и
		// кладём в ПОЛЕ Window.TrayMenu, НЕ добавляя ребёнком дерева: PopupMenu
		// прямым ребёнком Window опасен (см. Window.SetBounds skip *PopupMenu);
		// window.attachTrayMenu добавит его в дерево правильно уже в Run().
		if childTag == "traymenu" {
			cw, err := l.BuildPopupMenu(&child, contentOff)
			if err != nil {
				return nil, err
			}
			if pm, ok := cw.(*base.PopupMenu); ok {
				win.TrayMenu = pm
			}
			continue
		}

		// ── <TabItem> — вкладка заголовка (режим TitleTabs) ──────────────────
		if childTag == "tabitem" && win.TitleTabsEnabled() {
			header := child.Attr("Header", "Text")
			if header == "" {
				header = child.Text
			}
			var content base.Widget
			for _, inner := range child.Children {
				if strings.Contains(strings.ToLower(inner.Tag), ".") {
					continue
				}
				cw, err := l.BuildChild(&inner, contentOff)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					content = cw
					break
				}
			}
			text, key := xaml.LocItemText(header)
			idx := win.AddTitleTab(text, content)
			xaml.RegisterLocItem(key, func(s string) { win.SetTitleTabHeader(idx, s) })
			if ic := child.Attr("Icon"); ic != "" && baseDir != "" {
				if img := xaml.LoadTrayIcon(ic, baseDir); img != nil {
					win.SetTitleTabIcon(idx, img)
				}
			}
			if tip := child.Attr("ToolTip", "Tooltip"); tip != "" {
				tipText, tipKey := xaml.LocItemText(tip)
				win.SetTitleTabToolTip(idx, tipText)
				xaml.RegisterLocItem(tipKey, func(s string) { win.SetTitleTabToolTip(idx, s) })
			}
			continue
		}

		// ── <TitleTabsMenu> — меню кнопки «v» полосы вкладок ─────────────────
		if childTag == "titletabsmenu" {
			cw, err := l.BuildPopupMenu(&child, contentOff)
			if err != nil {
				return nil, err
			}
			if pm, ok := cw.(*base.PopupMenu); ok {
				win.SetTitleTabsMenu(pm)
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
				cw, err := l.BuildChild(&inner, contentOff)
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

		cw, err := l.BuildChild(&child, contentOff)
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
		// PopupMenu (TitleTabsMenu/контекстные) позиционируется собственным
		// Show — растягивать нельзя (см. Window.SetBounds).
		if _, ok := child.(*base.PopupMenu); ok {
			continue
		}
		childB := child.Bounds()
		if childB.Dx() <= 0 || childB.Dy() <= 0 || childB.Empty() {
			child.SetBounds(cb)
		}
	}

	return win, nil
}
