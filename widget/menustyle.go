package widget

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
)

// menustyle.go — вид контекстного меню по профилю темы.
//
// Меню само читает стиль компонента "menu" (части "", "item", "separator",
// "shortcut") и метрики theme.KeyMenu*, а не получает готовые числа от
// потребителя: профиль, в котором нет этих токенов, оставляет меню прежним,
// а объявивший их меняет его вид везде, где меню создаётся — и в оболочке
// рабочего стола, и в контекстных меню текстовых полей.

// MenuStyle — то, что профиль темы объявил для меню. Нулевое значение поля —
// «не объявлено»: меню остаётся прежним.
type MenuStyle struct {
	// Цвета. Нулевая альфа — не объявлен.
	Background, Text, Border, Shadow color.RGBA
	HoverBG, HoverText, Disabled     color.RGBA
	Shortcut, Separator              color.RGBA

	// Форма.
	Corner     int     // скругление меню
	ItemCorner int     // скругление плашки наведения
	Elevation  float64 // высота над подложкой: мягкая тень вместо прямоугольной

	// Метрики, логические пиксели.
	ItemHeight, SeparatorHeight   int
	PadX, PadLeft, PadRight, PadY int
	IconSize, IconGap             int
	WidthMin, SubMenuWidthMin     int
	ItemInset, SeparatorInset     int // ItemInset объявленный: не меньше 1
	ChevronRight, ChevronSize     int
	SubMenuDelay                  int // мс

	// TintIcons — значки рисуются цветом текста пункта.
	TintIcons bool
}

// isZero — профиль ничего не объявил.
func (ms MenuStyle) isZero() bool { return ms == MenuStyle{} }

// MenuStyleFromTheme собирает то, что разрешённая тема объявила для меню.
// nil даёт пустое значение.
//
// Цвета берутся так же, как их брал рабочий стол: заливка, текст и рамка —
// из "menu"; плашка наведения и её текст — из "menu.item" при наведении,
// если они отличаются от обычных (иначе состояние не объявлено и тема
// отдала обычное); недоступный пункт — из "menu.item" при Disabled.
func MenuStyleFromTheme(rt *theme.Theme) MenuStyle {
	return menuStyleOf(rt, true)
}

// menuStyleOf — разбор темы. direct=false даёт только то, чего нет в плоской
// теме (Materialize): заливку, плашку и недоступный цвет она уже несёт, а
// цвета, равные умолчанию стиля, профилем не объявлены.
func menuStyleOf(rt *theme.Theme, direct bool) MenuStyle {
	var ms MenuStyle
	if rt == nil {
		return ms
	}
	base := rt.Style("menu", "", theme.StateNormal)
	// Умолчание стиля компонента, о котором тема не знает ничего: по нему
	// отличаем «объявлено» от «унаследовано от общих токенов».
	def := rt.Style("\x00", "", theme.StateNormal)
	differs := func(v, d color.RGBA) color.RGBA {
		if direct || v != d {
			return v
		}
		return color.RGBA{}
	}

	ms.Text = differs(base.Text, def.Text)
	ms.Border = differs(base.Border, def.Border)
	ms.Shadow = base.Shadow
	ms.Elevation = base.Elevation

	if direct {
		ms.Background = base.Fill
		hover := rt.Style("menu", "item", theme.StateHover)
		if hover.Fill.A != 0 && hover.Fill != base.Fill {
			ms.HoverBG = hover.Fill
			if hover.Text.A != 0 {
				ms.HoverText = hover.Text
			} else {
				ms.HoverText = base.Text
			}
		}
		if dis := rt.Style("menu", "item", theme.StateDisabled); dis.Text.A != 0 && dis.Text != base.Text {
			ms.Disabled = dis.Text
		}
	}

	// Скругление: профиль, собранный из плоской темы, его не объявлял —
	// стиль получил радиус контролов по умолчанию.
	if _, flat := rt.Color(keyStyleName); !flat {
		ms.Corner = int(base.Corner)
		ms.ItemCorner = int(rt.Style("menu", "item", theme.StateHover).Corner)
	}
	if v, ok := rt.Metric(theme.KeyMenuCorner); ok && v >= 0 {
		ms.Corner = int(v + 0.5)
	}
	if v, ok := rt.Metric(theme.KeyMenuItemCorner); ok && v >= 0 {
		ms.ItemCorner = int(v + 0.5)
	}

	if sep := rt.Style("menu", "separator", theme.StateNormal); sep != base && sep.Fill.A != 0 {
		ms.Separator = sep.Fill
	}
	if sc := rt.Style("menu", "shortcut", theme.StateNormal); sc != base && sc.Text.A != 0 {
		ms.Shortcut = sc.Text
	}

	px := func(k theme.Key, into *int) {
		if v, ok := rt.Metric(k); ok && v > 0 {
			*into = int(v + 0.5)
		}
	}
	px(theme.KeyMenuItemHeight, &ms.ItemHeight)
	px(theme.KeyMenuSeparatorHeight, &ms.SeparatorHeight)
	px(theme.KeyMenuPadX, &ms.PadX)
	px(theme.KeyMenuPadLeft, &ms.PadLeft)
	px(theme.KeyMenuPadRight, &ms.PadRight)
	px(theme.KeyMenuPadY, &ms.PadY)
	px(theme.KeyMenuIconSize, &ms.IconSize)
	px(theme.KeyMenuIconGap, &ms.IconGap)
	px(theme.KeyMenuWidthMin, &ms.WidthMin)
	px(theme.KeyMenuSubWidthMin, &ms.SubMenuWidthMin)
	px(theme.KeyMenuSeparatorInset, &ms.SeparatorInset)
	px(theme.KeyMenuChevronRight, &ms.ChevronRight)
	px(theme.KeyMenuChevronSize, &ms.ChevronSize)
	px(theme.KeyMenuSubmenuDelay, &ms.SubMenuDelay)
	if v, ok := rt.Metric(theme.KeyMenuItemInset); ok && v >= 0 {
		ms.ItemInset = int(v + 0.5)
		if ms.ItemInset < 1 {
			ms.ItemInset = 1 // вплотную к рамке, но не поверх неё
		}
	}
	ms.TintIcons = rt.FlagOr(theme.FlagMenuIconTint, false)
	return ms
}

// writeMenuStyle объявляет в профиле то, что несёт ThemeStyle.Menu, — чтобы
// путь «плоская тема → профиль → плоская тема» не терял вид меню.
func writeMenuStyle(p *theme.Profile, styles map[theme.StyleKey]theme.StyleDelta, ms MenuStyle) {
	if ms.isZero() {
		return
	}
	put := func(part string, st theme.State, f func(*theme.StyleDelta)) {
		k := theme.StyleKey{Component: "menu", Part: part, State: st}
		d := styles[k]
		f(&d)
		styles[k] = d
	}
	col := func(c color.RGBA) *color.RGBA {
		if c.A == 0 {
			return nil
		}
		return theme.C(c)
	}
	put("", theme.StateNormal, func(d *theme.StyleDelta) {
		if c := col(ms.Text); c != nil {
			d.Text = c
		}
		if c := col(ms.Border); c != nil {
			d.Border = c
		}
		if c := col(ms.Shadow); c != nil {
			d.Shadow = c
		}
		if ms.Elevation > 0 {
			d.Elevation = theme.N(ms.Elevation)
		}
	})
	if c := col(ms.Separator); c != nil {
		put("separator", theme.StateNormal, func(d *theme.StyleDelta) { d.Fill = c })
	}
	if c := col(ms.Shortcut); c != nil {
		put("shortcut", theme.StateNormal, func(d *theme.StyleDelta) { d.Text = c })
	}
	nums := []struct {
		k theme.Key
		v int
	}{
		{theme.KeyMenuCorner, ms.Corner}, {theme.KeyMenuItemCorner, ms.ItemCorner},
		{theme.KeyMenuItemHeight, ms.ItemHeight}, {theme.KeyMenuSeparatorHeight, ms.SeparatorHeight},
		{theme.KeyMenuPadX, ms.PadX}, {theme.KeyMenuPadLeft, ms.PadLeft},
		{theme.KeyMenuPadRight, ms.PadRight}, {theme.KeyMenuPadY, ms.PadY},
		{theme.KeyMenuIconSize, ms.IconSize}, {theme.KeyMenuIconGap, ms.IconGap},
		{theme.KeyMenuWidthMin, ms.WidthMin}, {theme.KeyMenuSubWidthMin, ms.SubMenuWidthMin},
		{theme.KeyMenuItemInset, ms.ItemInset}, {theme.KeyMenuSeparatorInset, ms.SeparatorInset},
		{theme.KeyMenuChevronRight, ms.ChevronRight}, {theme.KeyMenuChevronSize, ms.ChevronSize},
		{theme.KeyMenuSubmenuDelay, ms.SubMenuDelay},
	}
	for _, n := range nums {
		// Скругление 0 тоже значимо, остальное — только положительное.
		if n.v > 0 || ((n.k == theme.KeyMenuCorner || n.k == theme.KeyMenuItemCorner) && n.v >= 0) {
			p.SetMetric(n.k, float64(n.v))
		}
	}
	if ms.TintIcons {
		p.SetFlag(theme.FlagMenuIconTint, true)
	}
}

// ApplyMenuStyle применяет к меню то, что объявил профиль.
//
// Цвета, которых профиль не объявил, остаются как были. Размеры и поля —
// наоборот: сначала возвращаются к умолчанию (22/7 для классики, иначе 30/9),
// затем применяется объявленное. Меню создаётся один раз, а показывается при
// любом числе смен темы, и значение прежней темы не должно пережить её уход.
func (m *PopupMenu) ApplyMenuStyle(ms MenuStyle) {
	m.applyMenuStyle(ms, currentStyle().Classic3D)
}

// applyMenuStyle — ApplyMenuStyle для темы, чей признак «классика» известен
// вызывающему (ApplyTheme получает его от применяемой темы, а не от общей).
func (m *PopupMenu) applyMenuStyle(ms MenuStyle, classic bool) {
	if ms.Background.A != 0 {
		m.Background = ms.Background
	}
	if ms.Text.A != 0 {
		m.TextColor = ms.Text
	}
	if ms.Border.A != 0 {
		m.BorderColor, m.SeparatorColor = ms.Border, ms.Border
	}
	if ms.Separator.A != 0 {
		m.SeparatorColor = ms.Separator
	}
	if ms.Shadow.A != 0 {
		m.ShadowColor = ms.Shadow
	}
	if ms.HoverBG.A != 0 {
		m.HoverBG = ms.HoverBG
		if ms.HoverText.A != 0 {
			m.HoverTextColor = ms.HoverText
		}
	}
	if ms.Disabled.A != 0 {
		m.DisabledColor = ms.Disabled
	}
	m.ShortcutColor = ms.Shortcut

	// Размеры: умолчание, затем объявленное.
	m.ItemHeight, m.SeparatorH = 30, 9
	if classic {
		m.ItemHeight, m.SeparatorH = 22, 7
	}
	set := func(dst *int, v int) {
		if v > 0 {
			*dst = v
		}
	}
	set(&m.ItemHeight, ms.ItemHeight)
	set(&m.SeparatorH, ms.SeparatorHeight)

	// Поля и ширина меняют уже заданные потребителем значения, поэтому
	// сбрасываются только если их прежде ставил профиль.
	if prev := m.applied; prev.PadX > 0 && ms.PadX == 0 {
		m.PaddingX = 16
	}
	if prev := m.applied; prev.WidthMin > 0 && ms.WidthMin == 0 {
		m.MinWidth = 160
	}
	set(&m.PaddingX, ms.PadX)
	set(&m.MinWidth, ms.WidthMin)

	m.PadLeft, m.PadRight, m.PaddingY = ms.PadLeft, ms.PadRight, ms.PadY
	m.IconSize, m.IconGap = ms.IconSize, ms.IconGap
	m.ItemInset, m.SeparatorInset = ms.ItemInset, ms.SeparatorInset
	m.ChevronRight, m.ChevronSize = ms.ChevronRight, ms.ChevronSize
	m.SubMenuDelay = ms.SubMenuDelay
	m.CornerRadius, m.ItemCorner, m.Elevation = ms.Corner, ms.ItemCorner, ms.Elevation
	m.TintIcons = ms.TintIcons

	// Подменю по ширине содержимого: минимум нужен корневому меню.
	switch {
	case ms.SubMenuWidthMin > 0:
		m.SubMenuMinWidth = ms.SubMenuWidthMin
	case ms.WidthMin > 0:
		m.SubMenuMinWidth = -1
	default:
		m.SubMenuMinWidth = 0
	}
	m.applied = ms
}
