// navpanel.go — боковая навигация с пунктами-иконками и свёрнутым режимом.
//
// Окно настроек, устроенное как вертикальные вкладки браузера, приложение
// собирало из панели и кнопок вручную, и при сворачивании от него оставалась
// пустота: пункты либо есть целиком, либо их нет. Настоящая боковая навигация
// сворачивается в ПОЛОСКУ с иконками — ширина уходит, а сами пункты остаются
// доступными, только без подписей.
//
// Отсюда и устройство виджета: пункт — это иконка И подпись сразу, а
// свёрнутый режим прячет подписи, а не пункты.
package widget

import (
	"image"
	"image/color"
	"strings"
)

// Геометрия по умолчанию.
const (
	navExpandedW  = 240 // ширина развёрнутой панели
	navCollapsedW = 48  // ширина полоски со «свёрнутыми» пунктами
	navItemH      = 36  // высота пункта
	navIconSize   = 18  // сторона иконки
	navItemPad    = 6   // отступ пунктов от краёв панели
	navAccentW    = 3   // ширина полоски у выбранного пункта
)

// NavPanelItem — пункт боковой навигации.
//
// Иконка нужна не для красоты: в свёрнутом режиме от пункта остаётся только
// она. Пункт без иконки рисуется первой буквой подписи — панель не должна
// превращаться в полоску пустых строк у того, кто иконки не завёл.
type NavPanelItem struct {
	Icon image.Image
	Text string
	Tag  any // данные приложения (движок их не трогает)
}

// NavPanel — боковая панель навигации: список пунктов с иконками, выделение
// текущего и свёрнутый режим (полоска с одними иконками).
//
//	nav := widget.NewNavPanel()
//	nav.AddItem(iconGeneral, "Общие")
//	nav.AddItem(iconGit, "Git")
//	nav.OnSelect = func(i int) { pages.Show(i) }
//	dlg.SetNavPanel(nav) // панель во всю высоту окна, включая полосу заголовка
type NavPanel struct {
	Base

	items     []NavPanelItem
	selected  int
	hovered   int
	collapsed bool

	// ExpandedWidth/CollapsedWidth — ширина панели в двух состояниях. Ширину
	// спрашивает хост (DesiredSize), поэтому менять её достаточно здесь.
	ExpandedWidth  int
	CollapsedWidth int
	ItemHeight     int
	IconSize       int
	FontSize       float64

	Background   color.RGBA
	HoverBG      color.RGBA
	SelectedBG   color.RGBA
	TextColor    color.RGBA
	SelectedText color.RGBA
	AccentColor  color.RGBA

	// TopInset — сколько места сверху занято чужим: подписью заголовка окна,
	// когда панель поднята под самый верх (Dialog.SetNavPanel). Пункты
	// начинаются ниже него.
	TopInset int

	// OnSelect вызывается при выборе пункта мышью (не при SetSelected).
	OnSelect func(index int)
	// OnCollapsedChanged вызывается при смене режима — и мышью, и SetCollapsed.
	OnCollapsedChanged func(collapsed bool)

	// onLayout — хост пересчитывает раскладку, когда меняется ширина панели.
	// Не публичное: приложение о раскладке хоста не знает, а хост о своём
	// содержимом — знает.
	onLayout func()
}

// NewNavPanel создаёт пустую боковую навигацию с цветами активной темы.
func NewNavPanel() *NavPanel {
	n := &NavPanel{
		selected:       -1,
		hovered:        -1,
		ExpandedWidth:  navExpandedW,
		CollapsedWidth: navCollapsedW,
		ItemHeight:     navItemH,
		IconSize:       navIconSize,
	}
	n.ApplyTheme(CurrentTheme())
	return n
}

// AddItem добавляет пункт и возвращает его индекс.
//
// Иконка может быть nil — тогда в свёрнутом режиме рисуется первая буква
// подписи.
func (n *NavPanel) AddItem(icon image.Image, text string) int {
	n.items = append(n.items, NavPanelItem{Icon: icon, Text: text})
	if n.selected < 0 {
		n.selected = 0
	}
	n.Invalidate()
	return len(n.items) - 1
}

// SetItems заменяет список пунктов целиком.
func (n *NavPanel) SetItems(items []NavPanelItem) {
	n.items = append(n.items[:0:0], items...)
	if n.selected >= len(n.items) {
		n.selected = len(n.items) - 1
	}
	if n.selected < 0 && len(n.items) > 0 {
		n.selected = 0
	}
	n.Invalidate()
}

// Items возвращает пункты панели.
func (n *NavPanel) Items() []NavPanelItem { return n.items }

// ItemCount возвращает число пунктов.
func (n *NavPanel) ItemCount() int { return len(n.items) }

// Selected возвращает индекс выбранного пункта (-1 — ничего не выбрано).
func (n *NavPanel) Selected() int { return n.selected }

// SetSelected выбирает пункт БЕЗ вызова OnSelect: обработчик сообщает о выборе
// пользователя, а не о состоянии, выставленном программой.
func (n *NavPanel) SetSelected(i int) {
	if i < -1 || i >= len(n.items) || i == n.selected {
		return
	}
	n.selected = i
	n.Invalidate()
}

// SetCollapsed сворачивает панель в полоску с иконками (или разворачивает).
//
// Ширина меняется, поэтому хост пересчитывает раскладку: содержимое справа
// занимает освободившееся место.
func (n *NavPanel) SetCollapsed(v bool) {
	if n.collapsed == v {
		return
	}
	n.collapsed = v
	if n.onLayout != nil {
		n.onLayout()
	}
	n.Invalidate()
	if n.OnCollapsedChanged != nil {
		n.OnCollapsedChanged(v)
	}
}

// IsCollapsed сообщает, свёрнута ли панель.
func (n *NavPanel) IsCollapsed() bool { return n.collapsed }

// ToggleCollapsed переключает режим панели.
func (n *NavPanel) ToggleCollapsed() { n.SetCollapsed(!n.collapsed) }

// Width возвращает ширину панели в текущем режиме.
func (n *NavPanel) Width() int {
	if n.collapsed {
		if n.CollapsedWidth > 0 {
			return n.CollapsedWidth
		}
		return navCollapsedW
	}
	if n.ExpandedWidth > 0 {
		return n.ExpandedWidth
	}
	return navExpandedW
}

// DesiredSize сообщает хосту ширину панели; высоту задаёт хост — панель
// занимает всю доступную.
func (n *NavPanel) DesiredSize() (int, int) {
	h := n.ItemHeight * len(n.items)
	return n.Width(), h + n.TopInset + navItemPad*2
}

// SetOnLayout — внутренний хук хоста (Dialog/Window): пересчитать раскладку
// после смены ширины.
func (n *NavPanel) setOnLayout(f func()) { n.onLayout = f }

// ApplyTheme обновляет цвета панели.
func (n *NavPanel) ApplyTheme(t *Theme) {
	n.Background = t.PanelBG
	n.HoverBG = t.ListItemHover
	n.SelectedBG = t.ListItemSelect
	n.TextColor = t.LabelText
	n.SelectedText = t.LabelText
	n.AccentColor = t.ProgressFill
}

// itemH возвращает высоту пункта.
func (n *NavPanel) itemH() int {
	if n.ItemHeight > 0 {
		return n.ItemHeight
	}
	return navItemH
}

// ItemRect возвращает прямоугольник пункта в абсолютных координатах.
func (n *NavPanel) ItemRect(i int) image.Rectangle {
	if i < 0 || i >= len(n.items) {
		return image.Rectangle{}
	}
	b := n.bounds
	top := b.Min.Y + n.TopInset + navItemPad + i*n.itemH()
	return image.Rect(b.Min.X+navItemPad, top, b.Max.X-navItemPad, top+n.itemH())
}

// itemAt возвращает индекс пункта под точкой (-1 — мимо).
func (n *NavPanel) itemAt(pt image.Point) int {
	for i := range n.items {
		if pt.In(n.ItemRect(i)) {
			return i
		}
	}
	return -1
}

func (n *NavPanel) Draw(ctx DrawContext) {
	b := n.bounds
	if b.Empty() {
		return
	}
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), n.Background)

	size := n.FontSize
	if size <= 0 {
		size = DefaultFontSizePt
	}
	iconSz := n.IconSize
	if iconSz <= 0 {
		iconSz = navIconSize
	}

	for i, it := range n.items {
		r := n.ItemRect(i)
		if r.Empty() {
			continue
		}
		fg := n.TextColor
		switch {
		case i == n.selected:
			ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, n.SelectedBG)
			// Полоска у левого края: в свёрнутом режиме подписи нет, и
			// выделение фоном одной иконки читается хуже, чем у строки.
			ctx.FillRoundRect(r.Min.X, r.Min.Y+4, navAccentW, r.Dy()-8, 1, n.AccentColor)
			fg = n.SelectedText
		case i == n.hovered:
			ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, n.HoverBG)
		}

		iconX := r.Min.X + 10
		if n.collapsed {
			// Полоска: иконка по центру, подписи нет.
			iconX = r.Min.X + (r.Dx()-iconSz)/2
		}
		iconY := r.Min.Y + (r.Dy()-iconSz)/2
		if it.Icon != nil {
			ctx.DrawImageScaled(it.Icon, iconX, iconY, iconSz, iconSz)
		} else if s := firstRune(it.Text); s != "" {
			// Пункт без иконки: первая буква подписи. Иначе свёрнутая панель
			// превратилась бы в полоску пустых строк.
			tw := ctx.MeasureText(s, size)
			ctx.DrawTextFont(s, iconX+(iconSz-tw)/2, iconY+(iconSz-13)/2, size, BuiltinFontBold, fg)
		}

		if n.collapsed {
			continue
		}
		textX := iconX + iconSz + 10
		text := ellipsizeText(ctx, it.Text, r.Max.X-8-textX, size)
		ctx.DrawTextSize(text, textX, r.Min.Y+(r.Dy()-14)/2, size, fg)
	}
}

// firstRune возвращает первую букву строки в верхнем регистре.
func firstRune(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}

func (n *NavPanel) OnMouseMove(x, y int) {
	h := n.itemAt(image.Pt(x, y))
	if h != n.hovered {
		n.hovered = h
		n.Invalidate()
	}
}

func (n *NavPanel) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	i := n.itemAt(image.Pt(e.X, e.Y))
	if i < 0 {
		// Клик по пустому месту панели гасим: под ней содержимое диалога, и
		// «провалившийся» клик выбрал бы что-то за её спиной.
		return image.Pt(e.X, e.Y).In(n.bounds)
	}
	if i != n.selected {
		n.selected = i
		n.Invalidate()
	}
	if n.OnSelect != nil {
		n.OnSelect(i)
	}
	return true
}

// SetTopInset задаёт, сколько места сверху занято чужим — подписью заголовка
// окна, когда панель поднята под самый верх. Зовётся хостом при раскладке.
func (n *NavPanel) SetTopInset(px int) {
	if n.TopInset == px {
		return
	}
	n.TopInset = px
	n.Invalidate()
}
