package base

// dialog_titlebar.go — полоса заголовка диалога: начинка, кнопка
// сворачивания, отступы содержимого (методы окна — widget/chrome).

import "image"

// SetContentPadding задаёт отступ содержимого от краёв диалога.
//
// До сих пор это была константа: 14 точек по горизонтали и 12 по вертикали, и
// боковая панель, доходящая до края окна, была недостижима — вокруг неё
// оставалась полоса фона диалога, закрасить которую нечем (фон один на всё
// окно).
//
// Отрицательное значение возвращает умолчание: у обычного диалога прежние
// 14/12 (кнопки и подписи MessageBox рассчитаны на них), у диалога без
// штатной полосы (SetChromeless) — ноль, потому что там приложение рисует всё
// само и отступ движка ему только мешает.
func (d *Dialog) SetContentPadding(px int) {
	if px < 0 {
		if !d.padSet {
			return
		}
		d.padSet, d.pad = false, 0
	} else {
		if d.padSet && d.pad == px {
			return
		}
		d.padSet, d.pad = true, px
	}
	// Область содержимого изменилась — перекладываем.
	d.SetBounds(d.bounds)
	d.Invalidate()
}

// ContentPadding возвращает действующий отступ содержимого по горизонтали и
// вертикали.
func (d *Dialog) ContentPadding() (h, v int) { return d.contentPadding() }

func (d *Dialog) contentPadding() (h, v int) {
	if d.padSet {
		return d.pad, d.pad
	}
	if d.chromeless {
		return 0, 0
	}
	return dlgPad, dlgVPad
}

// SetTitleBarContent помещает виджет приложения в свободную часть полосы
// заголовка — между подписью и кнопкой ✕.
//
//	dlg.SetTitleBarContent(searchBox) // поиск в полосе заголовка
//	dlg.SetTitleBarContent(nil)       // убрать
//
// Раскладывает его движок: полоса принадлежит ему, её высота зависит от темы,
// а справа в ней стоят его же органы управления. Приложение, которому нужна
// полоса целиком, берёт SetChromeless и рисует её само.
//
// Нажатие на этот виджет не тащит окно: он ребёнок диалога, а дети в штатной
// полосе забирают клик себе (см. titleDragHit).
func (d *Dialog) SetTitleBarContent(w Widget) {
	if d.titleBarContent == w {
		return
	}
	if d.titleBarContent != nil {
		d.RemoveChild(d.titleBarContent)
	}
	d.titleBarContent = w
	if w != nil {
		d.AddChild(w)
	}
	d.layoutTitleBar()
	d.Invalidate()
}

// TitleBarContent возвращает виджет, стоящий в полосе заголовка (nil — нет).
func (d *Dialog) TitleBarContent() Widget { return d.titleBarContent }

// TitleBarContentBounds — свободная часть полосы заголовка: справа от подписи
// (и кнопки сворачивания, если она есть) и слева от ✕.
//
// Пустой прямоугольник означает, что места нет: полосы нет вовсе
// (SetChromeless) или она занята подписью и кнопками целиком.
func (d *Dialog) TitleBarContentBounds() image.Rectangle {
	th := d.titleH()
	if th == 0 {
		return image.Rectangle{}
	}
	b := d.bounds
	left := d.titleTextX() + d.titleTextW()
	if d.Title != "" {
		left += TitleBarGap
	}
	// Колонка боковой панели поднята под верх окна — начинка полосы начинается
	// ЗА ней, а не поверх: иначе поле поиска заезжало бы на панель, у которой
	// свой цвет и свои пункты.
	if nb := d.NavPanelBounds(); !nb.Empty() && nb.Max.X+TitleBarGap > left {
		left = nb.Max.X + TitleBarGap
	}
	// Правее начинки остаётся свободная полоска — за неё окно тащат.
	right := d.sysButtonsLeft() - TitleBarHandleW
	if d.ShowLocaleIndicator {
		right -= LocaleBadgeReserve
	}
	r := image.Rect(left, b.Min.Y+TitleBarPadV, right, b.Min.Y+th-TitleBarPadV)
	if r.Dx() < TitleBarMinW || r.Dy() < 1 {
		return image.Rectangle{}
	}
	return r
}

// titleTextX — левый край подписи заголовка в абсолютных координатах.
// Одна точка правды для отрисовки и для раскладки начинки: кнопка
// сворачивания сдвигает подпись вправо, и посчитать это в двух местах
// по-разному значило бы наложить поиск на заголовок.
func (d *Dialog) titleTextX() int {
	if d.navBtn != nil && !d.navBtn.bounds.Empty() {
		return d.navBtn.bounds.Max.X + TitleBarGap
	}
	if currentStyle().Classic3D {
		return d.bounds.Min.X + 10
	}
	return d.bounds.Min.X + dlgPad
}

// titleTextW — ширина подписи заголовка тем шрифтом, которым её рисуют.
func (d *Dialog) titleTextW() int {
	if d.Title == "" {
		return 0
	}
	if currentStyle().Classic3D {
		return MeasureTextFontFor(d, d.Title, DefaultFontSize(), BuiltinFontBold)
	}
	return MeasureTextFontFor(d, d.Title, 11, BuiltinFontBold)
}

// SetNavButton показывает или убирает кнопку сворачивания в левой части
// полосы заголовка.
//
// Само сворачивание — за приложением: движку неизвестно, что у него слева.
// Кнопка лишь переключает состояние и зовёт OnNavToggle:
//
//	dlg.SetNavButton(true)
//	dlg.SetNavIcons(iconMenu, nil) // своя иконка (nil — встроенная «≡»)
//	dlg.OnNavToggle = func(collapsed bool) {
//	    nav.SetVisible(!collapsed)
//	    layout()
//	}
//
// У диалога без штатной полосы (SetChromeless) кнопки нет: её негде рисовать,
// и шапку там приложение рисует само — вместе со своей кнопкой.
func (d *Dialog) SetNavButton(v bool) {
	if v == (d.navBtn != nil) {
		return
	}
	if !v {
		d.RemoveChild(d.navBtn)
		d.navBtn = nil
	} else {
		nb := &TitleNavButton{FG: d.TitleColor}
		nb.Toggle = func() { d.SetNavCollapsed(!nb.Collapsed) }
		d.navBtn = nb
		d.AddChild(nb)
	}
	d.layoutTitleBar()
	d.Invalidate()
}

// HasNavButton сообщает, показана ли кнопка сворачивания.
func (d *Dialog) HasNavButton() bool { return d.navBtn != nil }

// SetNavIcons задаёт иконки кнопки сворачивания для развёрнутого и свёрнутого
// состояния. nil во втором аргументе — рисовать ту же иконку в обоих.
func (d *Dialog) SetNavIcons(expanded, collapsed image.Image) {
	if d.navBtn == nil {
		d.SetNavButton(true)
	}
	d.navBtn.Icon, d.navBtn.IconAlt = expanded, collapsed
	d.navBtn.Invalidate()
}

// SetNavCollapsed задаёт состояние кнопки сворачивания и уведомляет
// приложение (OnNavToggle) — как если бы по кнопке щёлкнули.
func (d *Dialog) SetNavCollapsed(v bool) {
	if d.navBtn == nil || d.navBtn.Collapsed == v {
		return
	}
	d.navBtn.Collapsed = v
	d.navBtn.Invalidate()
	// Кнопка в полосе — орган управления именно боковой панелью, когда она
	// задана: не сворачивать её здесь значило бы просить приложение повторить
	// то же самое в OnNavToggle.
	if c, ok := d.navPanel.(interface{ SetCollapsed(bool) }); ok {
		c.SetCollapsed(v)
	}
	if d.OnNavToggle != nil {
		d.OnNavToggle(v)
	}
}

// IsNavCollapsed сообщает состояние кнопки сворачивания.
func (d *Dialog) IsNavCollapsed() bool { return d.navBtn != nil && d.navBtn.Collapsed }

// layoutTitleBar расставляет кнопку сворачивания и начинку полосы.
// Зовётся из SetBounds: ширина полосы меняется вместе с диалогом.
func (d *Dialog) layoutTitleBar() {
	b := d.bounds
	th := d.titleH()
	if d.navBtn != nil {
		if th == 0 {
			d.navBtn.SetBounds(image.Rectangle{})
			d.navBtn.SetVisible(false)
		} else {
			sz := NavBtnSize
			if sz > th-4 {
				sz = th - 4
			}
			top := b.Min.Y + (th-sz)/2
			d.navBtn.SetVisible(true)
			d.navBtn.SetBounds(image.Rect(b.Min.X+6, top, b.Min.X+6+sz, top+sz))
		}
	}
	if d.titleBarContent != nil {
		r := d.TitleBarContentBounds()
		SetWidgetVisible(d.titleBarContent, !r.Empty())
		d.titleBarContent.SetBounds(r)
	}
}

// SetWidgetVisible прячет виджет, если он это умеет.
//
// Начинку полосы даёт приложение, и это произвольный Widget: интерфейс не
// обязывает уметь SetVisible. Пустых границ мало — виджет, рисующий себя без
// оглядки на них, оставил бы след поверх заголовка.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func SetWidgetVisible(w Widget, v bool) {
	if s, ok := w.(interface{ SetVisible(bool) }); ok {
		s.SetVisible(v)
	}
}
