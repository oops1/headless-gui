// toolbar.go — панель инструментов: разделители, режим «только иконки»,
// переполнение.
//
// Теги <ToolBarTray> и <ToolBar> разметка понимала и раньше, но собирала их
// горизонтальным StackPanel: раскладка была, поведения не было. Разделитель
// приходилось изображать узкой панелью, режим «только иконки» — переставлять
// IconPos у каждой кнопки вручную, а панель, не поместившаяся в окно, просто
// обрезалась: часть кнопок становилась недостижимой.
package widget

import (
	"image"
	"image/color"
)

// toolBarChevronW — ширина зоны кнопки переполнения (шеврон).
const toolBarChevronW = 18

// ToolBar — горизонтальная панель инструментов.
type ToolBar struct {
	Base

	Background color.RGBA
	UseAlpha   bool

	// Spacing — расстояние между элементами, Padding — внутренний отступ.
	Spacing int
	Padding int

	// SeparatorColor — цвет вертикальной черты разделителя.
	SeparatorColor color.RGBA

	// IconsOnly — кнопки с иконкой показывают только иконку.
	//
	// Переключается на лету: прежнее положение подписи запоминается и
	// возвращается при выключении. Иначе панель, разок побывавшая в режиме
	// иконок, теряла бы подписи навсегда.
	IconsOnly bool

	// Overflow — прятать не поместившиеся элементы в меню под шевроном.
	//
	// По умолчанию включено: обрезанная кнопка недостижима, и это хуже, чем
	// кнопка, до которой нужно два щелчка.
	Overflow bool

	// iconPosSaved — исходное положение подписи кнопок (для IconsOnly).
	iconPosSaved map[*Button]IconPosition

	// overflowed — элементы, не поместившиеся в последней раскладке.
	overflowed []Widget
	chevron    image.Rectangle
	menu       rowMenuHost
}

// NewToolBar создаёт пустую панель инструментов.
func NewToolBar() *ToolBar {
	return &ToolBar{
		Spacing:        2,
		Padding:        4,
		SeparatorColor: win10.Border,
		Overflow:       true,
		UseAlpha:       true,
		iconPosSaved:   map[*Button]IconPosition{},
	}
}

// toolBarSeparator — вертикальная черта между группами кнопок.
//
// Отдельный тип, а не «панель шириной в пиксель»: разделитель не участвует в
// переполнении как кнопка (в меню он становится чертой), и панель обязана
// уметь его отличить.
type toolBarSeparator struct {
	Base
	Color color.RGBA
}

// Draw рисует черту по центру отведённой ширины.
func (s *toolBarSeparator) Draw(ctx DrawContext) {
	b := s.bounds
	if b.Empty() {
		return
	}
	x := b.Min.X + b.Dx()/2
	ctx.DrawVLine(x, b.Min.Y+2, b.Dy()-4, s.Color)
}

// AddSeparator добавляет разделитель в конец панели.
func (tb *ToolBar) AddSeparator() {
	tb.AddChild(&toolBarSeparator{Color: tb.SeparatorColor})
}

// AddChild добавляет элемент и пересчитывает раскладку.
func (tb *ToolBar) AddChild(w Widget) {
	tb.Base.AddChild(w)
	tb.applyIconsOnly()
	tb.layout()
}

// SetBounds задаёт границы и пересчитывает раскладку.
func (tb *ToolBar) SetBounds(r image.Rectangle) {
	tb.Base.SetBounds(r)
	tb.layout()
}

// SetIconsOnly включает или выключает режим «только иконки».
func (tb *ToolBar) SetIconsOnly(v bool) {
	if tb.IconsOnly == v {
		return
	}
	tb.IconsOnly = v
	tb.applyIconsOnly()
	tb.layout()
	tb.Invalidate()
}

// applyIconsOnly переставляет подписи кнопок и запоминает исходное положение.
func (tb *ToolBar) applyIconsOnly() {
	if tb.iconPosSaved == nil {
		tb.iconPosSaved = map[*Button]IconPosition{}
	}
	for _, child := range tb.children {
		btn, ok := child.(*Button)
		if !ok || btn.Icon == nil {
			continue // кнопке без иконки прятать подпись не во что
		}
		if tb.IconsOnly {
			if _, seen := tb.iconPosSaved[btn]; !seen {
				tb.iconPosSaved[btn] = btn.IconPos
			}
			btn.IconPos = IconOnly
			continue
		}
		if prev, seen := tb.iconPosSaved[btn]; seen {
			btn.IconPos = prev
			delete(tb.iconPosSaved, btn)
		}
	}
}

// itemWidth возвращает желаемую ширину элемента панели.
//
// Кнопку меряем сами: общий desiredWidth отдаёт для неё круглые 80 пикселей, и
// панель из шести кнопок разъезжалась бы вдвое шире нужного, а в режиме иконок
// — во столько же раз уже.
func (tb *ToolBar) itemWidth(w Widget, h int) int {
	if _, ok := w.(*toolBarSeparator); ok {
		return 9
	}
	btn, ok := w.(*Button)
	if !ok {
		return desiredWidth(w)
	}
	if xw, _ := xamlSizeOf(btn); xw > 0 {
		return xw
	}
	icon := 0
	if btn.Icon != nil {
		icon = btn.IconSize
		if icon <= 0 {
			icon = h - 8
		}
		if icon < 12 {
			icon = 12
		}
	}
	if btn.IconPos == IconOnly || btn.Text == "" {
		if icon == 0 {
			return 32
		}
		return icon + 10
	}
	gap := 0
	if icon > 0 {
		gap = 4
	}
	return icon + gap + MeasureUIText(btn.Text, DefaultFontSizePt) + 14
}

// xamlSizeOf возвращает размер, заданный в разметке (0, если не задан).
func xamlSizeOf(w Widget) (int, int) {
	if g, ok := w.(interface{ GetXAMLSize() (int, int) }); ok {
		return g.GetXAMLSize()
	}
	return 0, 0
}

// layout расставляет элементы слева направо и убирает не поместившиеся.
func (tb *ToolBar) layout() {
	b := tb.bounds
	tb.overflowed = nil
	tb.chevron = image.Rectangle{}
	if b.Empty() {
		return
	}

	innerY := b.Min.Y + tb.Padding
	innerH := b.Dy() - tb.Padding*2
	if innerH < 1 {
		innerH = b.Dy()
		innerY = b.Min.Y
	}

	// Место под шеврон резервируется ЗАРАНЕЕ, если элементы вообще могут не
	// поместиться: иначе последняя кнопка встала бы ровно под шеврон и
	// оказалась бы наполовину им закрыта.
	limit := b.Max.X - tb.Padding
	if tb.Overflow && tb.totalWidth(innerH) > b.Dx()-tb.Padding*2 {
		limit -= toolBarChevronW
	}

	x := b.Min.X + tb.Padding
	first := true
	for _, child := range tb.children {
		w := tb.itemWidth(child, innerH)
		// Первый элемент не прячем никогда: панель из одной слишком широкой
		// кнопки превратилась бы в один шеврон, за которым не видно, что это
		// вообще за панель.
		if tb.Overflow && !first && x+w > limit {
			tb.overflowed = append(tb.overflowed, child)
			setToolBarItemVisible(child, false)
			continue
		}
		setToolBarItemVisible(child, true)
		child.SetBounds(image.Rect(x, innerY, x+w, innerY+innerH))
		x += w + tb.Spacing
		first = false
	}

	if len(tb.overflowed) > 0 {
		tb.chevron = image.Rect(b.Max.X-tb.Padding-toolBarChevronW, innerY,
			b.Max.X-tb.Padding, innerY+innerH)
	}
}

// totalWidth — сколько места хотят все элементы вместе.
func (tb *ToolBar) totalWidth(h int) int {
	total := 0
	for i, child := range tb.children {
		if i > 0 {
			total += tb.Spacing
		}
		total += tb.itemWidth(child, h)
	}
	return total
}

// setToolBarItemVisible прячет и показывает элемент панели.
func setToolBarItemVisible(w Widget, v bool) {
	if s, ok := w.(interface{ SetVisible(bool) }); ok {
		s.SetVisible(v)
	}
}

// OverflowCount сообщает, сколько элементов не поместилось.
//
// Нужен приложению, которое хочет узнать, что панель тесна, — и тестам,
// которым иначе пришлось бы искать шеврон по пикселям.
func (tb *ToolBar) OverflowCount() int { return len(tb.overflowed) }

// Draw рисует фон панели, элементы и шеврон переполнения.
func (tb *ToolBar) Draw(ctx DrawContext) {
	b := tb.bounds
	if b.Empty() {
		return
	}
	if !tb.UseAlpha || tb.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), tb.Background)
	}
	tb.drawChildren(ctx)

	if !tb.chevron.Empty() {
		c := tb.chevron
		cx, cy := c.Min.X+c.Dx()/2, c.Min.Y+c.Dy()/2
		col := win10.BtnText
		// Двойная стрелка вправо — тот же знак, которым переполнение
		// показывает WPF.
		for _, dx := range []int{-3, 1} {
			for i := 0; i < 4; i++ {
				ctx.SetPixel(cx+dx-2+i, cy-3+i, col)
				ctx.SetPixel(cx+dx-2+i, cy+3-i, col)
			}
		}
	}
}

// ─── Overlay (меню переполнения) ───────────────────────────────────────────

// HasOverlay реализует OverlayDrawer.
func (tb *ToolBar) HasOverlay() bool { return tb.menu.open() }

// DrawOverlay рисует меню переполнения поверх всего UI.
func (tb *ToolBar) DrawOverlay(ctx DrawContext) { tb.menu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (tb *ToolBar) OverlayBounds() image.Rectangle { return tb.menu.overlayBounds() }

// Dismiss закрывает меню переполнения. Реализует Dismissable.
func (tb *ToolBar) Dismiss() { tb.menu.dismiss() }

// OnMouseButton открывает меню переполнения по щелчку на шевроне.
func (tb *ToolBar) OnMouseButton(e MouseEvent) bool {
	if !tb.IsEnabled() {
		return false
	}
	if tb.menu.routeMouse(e) {
		return true
	}
	if e.Button != MouseLeft || !e.Pressed || tb.chevron.Empty() {
		return false
	}
	if !image.Pt(e.X, e.Y).In(tb.chevron) {
		return false
	}
	if m := tb.menu.build(e.X, e.Y, tb.overflowItems()); m != nil {
		m.Show(tb.chevron.Min.X, tb.chevron.Max.Y)
	}
	return true
}

// OnMouseMove ведёт подсветку пунктов открытого меню.
func (tb *ToolBar) OnMouseMove(x, y int) { tb.menu.routeMove(x, y) }

// overflowItems превращает спрятанные элементы в пункты меню.
//
// В меню попадают кнопки и разделители — то, у чего есть текстовое
// представление. Произвольный виджет (поле поиска, индикатор) пунктом меню
// стать не может: его нельзя нарисовать в списке строк, и притворяться, что
// можно, значит показать пустую строку без действия. Такой элемент просто не
// показывается — поэтому нестандартные виджеты в панели стоит ставить ПЕРВЫМИ,
// они переполняются последними.
func (tb *ToolBar) overflowItems() []MenuItem {
	items := make([]MenuItem, 0, len(tb.overflowed))
	for _, w := range tb.overflowed {
		if _, ok := w.(*toolBarSeparator); ok {
			// Разделитель в начале списка не нужен: черта под ничем.
			if len(items) > 0 {
				items = append(items, MenuItem{Separator: true})
			}
			continue
		}
		btn, ok := w.(*Button)
		if !ok {
			continue
		}
		text := btn.Text
		if text == "" {
			text = btn.ToolTip
		}
		if text == "" {
			continue // нечего показать в списке строк
		}
		b := btn
		items = append(items, MenuItem{
			Text:     text,
			Disabled: !btn.IsEnabled(),
			OnClick:  func() { b.fireClick() },
		})
	}
	// Хвостовой разделитель — та же черта под ничем, только снизу.
	for len(items) > 0 && items[len(items)-1].Separator {
		items = items[:len(items)-1]
	}
	return items
}

// ApplyTheme обновляет цвета панели.
func (tb *ToolBar) ApplyTheme(t *Theme) {
	tb.SeparatorColor = t.Border
	for _, child := range tb.children {
		if s, ok := child.(*toolBarSeparator); ok {
			s.Color = t.Border
		}
	}
}
