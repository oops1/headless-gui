package dock

// xaml.go — теги дока в разметке: <DockPanel>, <Border> (контейнер с фоном и
// одним потомком — через DockPanel) и <DockManager> с <DockPane>. Строители —
// контейнеры (xaml.Registry.RegisterComplete): сами строят детей и
// регистрируют имена.

import (
	"image"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

// buildXAMLDockPanel строит DockPanel из XAML-элемента <DockPanel>.
// Последний дочерний элемент заполняет оставшееся пространство (LastChildFill).
func buildXAMLDockPanel(el xaml.Element, l *xaml.Loader) (base.Widget, error) {
	parentOff := l.Offset()
	dp := NewDockPanel()

	// Background: цвет, "transparent" или роль темы "{Theme WindowBG}".
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if role, ok := base.ParseBackgroundRole(bgStr); ok {
			dp.SetBackgroundRole(role)
		} else if strings.EqualFold(bgStr, "transparent") {
			dp.UseAlpha = true
		} else if c, err := xaml.ParseColor(bgStr); err == nil {
			dp.Background = c
			dp.UseAlpha = c.A < 255
		}
	}

	// LastChildFill — WPF-атрибут: выключает растяжение последнего ребёнка.
	// Без него строку, где ВСЕ дети докованы, собрать нельзя: последний
	// забирает остаток, и его собственный Dock не действует.
	if v := el.Attr("LastChildFill"); v != "" {
		dp.LastChildFill = !strings.EqualFold(v, "false") && v != "0"
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	dp.SetBounds(absBounds)

	// Attached properties
	xaml.ApplyCommonProps(dp, el)

	// Регистрация
	if id := el.Name(); id != "" {
		l.SetName(id, dp)
	}

	// Дочерние виджеты (parentOff=0 — DockPanel.layout() сам расставит)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := l.BuildChild(&child, image.Point{})
		if err != nil {
			return nil, err
		}
		if cw != nil {
			dp.AddChild(cw) // AddChild → layout()
		}
	}

	return dp, nil
}

// buildXAMLBorder строит Border — контейнер с фоном/рамкой и одним потомком.
// В WPF Border.Child заполняет всю область Border.
// Реализуем через DockPanel (последний ребёнок заполняет оставшееся пространство).
func buildXAMLBorder(el xaml.Element, l *xaml.Loader) (base.Widget, error) {
	parentOff := l.Offset()
	dp := NewDockPanel()

	// Background: цвет, "transparent" или роль темы "{Theme WindowBG}".
	if bgStr := el.Attr("Background", "Fill"); bgStr != "" {
		if role, ok := base.ParseBackgroundRole(bgStr); ok {
			dp.SetBackgroundRole(role)
		} else if strings.EqualFold(bgStr, "transparent") {
			dp.UseAlpha = true
		} else if c, err := xaml.ParseColor(bgStr); err == nil {
			dp.Background = c
			dp.UseAlpha = c.A < 255
		}
	} else {
		dp.UseAlpha = true
	}

	// Bounds
	absBounds := el.Bounds().Add(parentOff)
	dp.SetBounds(absBounds)

	// Attached properties
	xaml.ApplyCommonProps(dp, el)

	// Регистрация
	if id := el.Name(); id != "" {
		l.SetName(id, dp)
	}

	// Дочерние виджеты — DockPanel.layout() заполнит последнего ребёнка.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := l.BuildChild(&child, image.Point{})
		if err != nil {
			return nil, err
		}
		if cw != nil {
			dp.AddChild(cw)
		}
	}

	return dp, nil
}

// xamlDockSide парсит атрибут Side="Left|Top|Bottom|Right" (дефолт Left).
func xamlDockSide(s string) base.DockSide {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "top":
		return base.DockTop
	case "bottom":
		return base.DockBottom
	case "right":
		return base.DockRight
	default:
		return base.DockLeft
	}
}

// buildXAMLDockPane строит DockPane из <DockPane> внутри <DockManager>.
// Возвращает панель и её запрошенные Side/Size/State — сам DockManager
// решает, что с ними делать (AddPane/SetSideSize/Unpin·Float·Close).
func buildXAMLDockPane(el xaml.Element, l *xaml.Loader) (pane *DockPane, side base.DockSide, size int, state string, err error) {
	id := xamlDockPaneID(el)
	title := el.Attr("Title")
	if title == "" {
		title = id
	}

	// Содержимое — первый дочерний виджет (кроме WPF property-тегов "X.Y").
	var content base.Widget
	for _, child := range el.Children {
		if strings.Contains(child.Tag, ".") {
			continue
		}
		cw, cerr := l.BuildChild(&child, image.Point{})
		if cerr != nil {
			return nil, base.DockLeft, 0, "", cerr
		}
		if cw != nil {
			content = cw
			break
		}
	}

	pane = NewDockPane(id, title, content)
	if v := xaml.ParseInt(el.Attr("MinSize")); v > 0 {
		pane.MinSize = v // минимум стороны, пока панель на ней (GG-79)
	}
	side = xamlDockSide(el.Attr("Side"))
	size = xaml.ParseInt(el.Attr("Size"))
	state = strings.ToLower(strings.TrimSpace(el.Attr("State")))

	if name := el.Attr("Name", "x:Name"); name != "" {
		l.SetName(name, pane)
	} else {
		l.SetName(id, pane)
	}

	return pane, side, size, state, nil
}

// buildXAMLDockManager строит DockManager из <DockManager> (см. схему выше).
func buildXAMLDockManager(el xaml.Element, l *xaml.Loader) (base.Widget, error) {
	parentOff := l.Offset()
	dm := NewDockManager()

	if bgStr := el.Attr("Background"); bgStr != "" {
		if c, err := xaml.ParseColor(bgStr); err == nil {
			dm.Background = c
		}
	}

	// NativeFloating="True" — декларация нативного отрыва панелей (window.Window
	// подхватит поле в Run → EnableDockFloating). В headless — без эффекта.
	if strings.EqualFold(el.Attr("NativeFloating"), "true") {
		dm.NativeFloating = true
	}

	absBounds := el.Bounds().Add(parentOff)
	dm.SetBounds(absBounds)
	xaml.ApplyCommonProps(dm, el)

	if id := el.Name(); id != "" {
		l.SetName(id, dm)
	}

	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		switch childTag {
		case "dockpane":
			p, side, size, state, err := buildXAMLDockPane(child, l)
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
			var content base.Widget
			for _, inner := range child.Children {
				if strings.Contains(inner.Tag, ".") {
					continue
				}
				cw, err := l.BuildChild(&inner, image.Point{})
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
				cw, err := l.BuildChild(&child, image.Point{})
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

// xamlDockPaneID возвращает Id панели: явный Id/Name, иначе слаг от Title,
// иначе "paneN" (автогенерация — на случай, если XAML не задал ни того ни
// другого).
func xamlDockPaneID(el xaml.Element) string {
	if id := el.Attr("Id", "Name", "x:Name"); id != "" {
		return id
	}
	if title := el.Attr("Title"); title != "" {
		return xamlSlugify(title)
	}
	xamlDockPaneAutoSeq++
	return "pane" + strconv.Itoa(xamlDockPaneAutoSeq)
}

// RegisterXAML регистрирует теги дока в реестре XAML r. Фасад widget зовёт её
// при первой загрузке разметки; программа на widget/base и widget/xaml зовёт
// сама: dock.RegisterXAML(xaml.Default()). В фасад не входит.
//
//facadegen:skip
func RegisterXAML(r *xaml.Registry) {
	r.RegisterComplete(func(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
		return buildXAMLDockPanel(*el, l)
	}, "dockpanel")
	r.RegisterComplete(func(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
		return buildXAMLBorder(*el, l)
	}, "border")
	r.RegisterComplete(func(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
		return buildXAMLDockManager(*el, l)
	}, "dockmanager")
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

var xamlDockPaneAutoSeq int
