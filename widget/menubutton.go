// menubutton.go — кнопка с выпадающим меню (GG-78).
//
// Панель инструментов клиента git наполовину состоит из таких кнопок: Pull▾,
// Push▾, Save Stash▾ — щелчок по основной части выполняет действие по
// умолчанию, по стрелке открывает варианты; у Git-Flow основной части нет
// вовсе, вся кнопка — меню. Раньше приложение изображало это обычной кнопкой,
// открывающей PopupMenu под собой, и разделённого вида получить не могло.
package widget

import (
	"image"
	"image/color"
	"strings"
)

// menuButtonArrowW — ширина зоны стрелки.
const menuButtonArrowW = 16

// MenuButton — кнопка с выпадающим меню.
//
// Два вида. Разделённая (Split, NewSplitButton): основная часть выполняет
// OnClick, стрелка справа открывает меню. Простая (NewMenuButton): меню
// открывает вся кнопка, OnClick не зовётся.
//
// Внешний вид, тема, значок, подпись и подсказка — от встроенной Button.
type MenuButton struct {
	*Button

	// Split — разделённая кнопка: основная часть — действие, стрелка — меню.
	Split bool
	// Items — пункты меню. Меню показывает копию: пункты можно менять между
	// открытиями, в том числе из OnOpening.
	Items []MenuItem
	// OnOpening зовётся перед каждым открытием меню — пересобрать Items под
	// нынешнее состояние (доступность пунктов, список stash).
	OnOpening func()

	actionDisabled bool // основная часть разделённой кнопки недоступна
	menuDisabled   bool // стрелка (у простой кнопки — вся кнопка) недоступна
	mainPressed    bool // основная часть нажата и ждёт отпускания

	menu rowMenuHost
}

// NewMenuButton создаёт кнопку, целиком открывающую меню.
func NewMenuButton(text string) *MenuButton {
	return &MenuButton{Button: NewButton(text)}
}

// NewSplitButton создаёт разделённую кнопку: onClick — действие основной
// части, меню открывает стрелка.
func NewSplitButton(text string, onClick func()) *MenuButton {
	b := NewButton(text)
	b.OnClick = onClick
	return &MenuButton{Button: b, Split: true}
}

// SetActionEnabled включает или выключает основную часть разделённой кнопки,
// не трогая стрелку: у Apply Stash действие недоступно без stash, а меню
// остаётся. Выключить кнопку целиком — SetEnabled.
func (mb *MenuButton) SetActionEnabled(v bool) {
	if mb.actionDisabled == !v {
		return
	}
	mb.actionDisabled = !v
	mb.mainPressed = false
	mb.Invalidate()
}

// ActionEnabled сообщает, доступна ли основная часть.
func (mb *MenuButton) ActionEnabled() bool { return !mb.actionDisabled }

// SetMenuEnabled включает или выключает меню: стрелку разделённой кнопки или
// всю простую кнопку.
func (mb *MenuButton) SetMenuEnabled(v bool) {
	if mb.menuDisabled == !v {
		return
	}
	mb.menuDisabled = !v
	if !v {
		mb.menu.dismiss()
	}
	mb.Invalidate()
}

// MenuEnabled сообщает, доступно ли меню.
func (mb *MenuButton) MenuEnabled() bool { return !mb.menuDisabled }

// OpenMenu открывает меню под кнопкой — как щелчок по стрелке. false — меню
// недоступно или пусто.
func (mb *MenuButton) OpenMenu() bool { return mb.openMenu(image.Point{}, false) }

// CloseMenu закрывает меню.
func (mb *MenuButton) CloseMenu() {
	if mb.menu.open() {
		mb.menu.dismiss()
		mb.Invalidate()
	}
}

// IsMenuOpen сообщает, открыто ли меню.
func (mb *MenuButton) IsMenuOpen() bool { return mb.menu.open() }

// openMenu собирает и показывает меню. at — точка нажатия: отпускание в ней же
// меню не гасит (см. rowMenuHost.routeMouse).
func (mb *MenuButton) openMenu(at image.Point, byMouse bool) bool {
	if !mb.IsEnabled() || mb.menuDisabled {
		return false
	}
	if mb.OnOpening != nil {
		mb.OnOpening()
	}
	m := mb.menu.build(at.X, at.Y, append([]MenuItem(nil), mb.Items...))
	if m == nil {
		return false
	}
	b := mb.bounds
	m.Show(b.Min.X, b.Max.Y)
	if !byMouse {
		// Отпускания, которое надо пропустить, не будет; с клавиатуры сразу
		// подсвечен первый доступный пункт — Enter выбирает его.
		mb.menu.armed = false
		m.setHoverIdx(m.nextActiveItem(-1))
	}
	mb.Invalidate()
	return true
}

// parts делит кнопку на основную часть и зону стрелки.
func (mb *MenuButton) parts() (main, arrow image.Rectangle) {
	b := mb.bounds
	w := menuButtonArrowW
	if w > b.Dx() {
		w = b.Dx()
	}
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X-w, b.Max.Y),
		image.Rect(b.Max.X-w, b.Min.Y, b.Max.X, b.Max.Y)
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// Draw рисует корпус, содержимое в основной части и стрелку. Открытое меню
// показывается нажатой стрелкой (у простой кнопки — нажатой кнопкой).
func (mb *MenuButton) Draw(ctx DrawContext) {
	b := mb.bounds
	if b.Empty() {
		return
	}
	open := mb.menu.open()
	main, arrow := mb.parts()

	if !mb.Split {
		pressed := open
		bg, txt, border := mb.stateColors(pressed)
		if mb.menuDisabled && mb.IsEnabled() {
			bg, txt, border = disabledLook(bg, txt, border)
		}
		mb.drawChrome(ctx, b, bg, border, pressed)
		mb.drawContent(ctx, main, txt)
		mb.drawArrow(ctx, arrow, txt)
		return
	}

	bg, txt, border := mb.stateColors(false)
	mb.drawChrome(ctx, b, bg, border, false)
	if mb.mainPressed {
		mb.drawPressedPart(ctx, main)
	}
	if open {
		mb.drawPressedPart(ctx, arrow)
	}
	mainTxt, arrowTxt := txt, txt
	if mb.IsEnabled() {
		if mb.actionDisabled {
			_, mainTxt, _ = disabledLook(bg, txt, border)
		}
		if mb.menuDisabled {
			_, arrowTxt, _ = disabledLook(bg, txt, border)
		}
	}
	mb.drawContent(ctx, main, mainTxt)

	div := border
	if st := currentStyle(); st.Classic3D {
		div = st.BevelDark
	}
	if div.A == 0 {
		div = mixRGBA(txt, bg, 0.6)
	}
	ctx.DrawVLine(arrow.Min.X, b.Min.Y+4, b.Dy()-8, div)
	mb.drawArrow(ctx, arrow, arrowTxt)
}

// drawPressedPart рисует нажатой одну часть разделённой кнопки.
func (mb *MenuButton) drawPressedPart(ctx DrawContext, r image.Rectangle) {
	st := currentStyle()
	if st.Classic3D {
		drawBevelSunken(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
		return
	}
	in := r.Inset(1)
	if in.Empty() {
		return
	}
	cr := mb.CornerRadius
	if cr == 0 {
		cr = st.ControlCorner
	}
	if cr > 1 {
		ctx.FillRoundRect(in.Min.X, in.Min.Y, in.Dx(), in.Dy(), cr-1, mb.PressedBG)
		return
	}
	ctx.FillRect(in.Min.X, in.Min.Y, in.Dx(), in.Dy(), mb.PressedBG)
}

// drawArrow рисует стрелку «▾». У кнопки со значком сверху стрелка стоит на
// строке подписи, как у панелей инструментов, иначе — по центру.
func (mb *MenuButton) drawArrow(ctx DrawContext, r image.Rectangle, col color.RGBA) {
	if r.Empty() {
		return
	}
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	if b := mb.bounds; mb.Icon != nil && mb.Text != "" && mb.IconPos == IconTop {
		// Та же раскладка, что у Button.drawContent для IconTop.
		iconSz := mb.IconSize
		if iconSz <= 0 {
			iconSz = b.Dy() - 8
			if iconSz < 12 {
				iconSz = 12
			}
		}
		const gap, textH = 4, 13
		startY := b.Min.Y + (b.Dy()-(iconSz+gap+textH))/2
		if startY < b.Min.Y+2 {
			startY = b.Min.Y + 2
		}
		cy = startY + iconSz + gap + textH/2
	}
	for i := 0; i < 4; i++ {
		ctx.DrawHLine(cx-3+i, cy-1+i, 7-2*i, col)
	}
}

// ─── Ввод ───────────────────────────────────────────────────────────────────

// OnMouseButton: основная часть — действие на отпускании, как у Button;
// стрелка (у простой кнопки — вся кнопка) открывает меню на нажатии. Нажатие
// по стрелке при открытом меню его закрывает.
func (mb *MenuButton) OnMouseButton(e MouseEvent) bool {
	if !mb.IsEnabled() {
		return false
	}
	wasOpen := mb.menu.open()
	if mb.menu.routeMouse(e) {
		return true
	}
	if e.Button != MouseLeft {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	main, arrow := mb.parts()

	if e.Pressed {
		if !pt.In(mb.bounds) {
			return false
		}
		if !mb.Split || pt.In(arrow) {
			if !wasOpen {
				mb.openMenu(pt, true)
			}
			mb.Invalidate()
			return true
		}
		if mb.actionDisabled {
			return true
		}
		mb.mainPressed = true
		mb.Invalidate()
		return true
	}

	if mb.mainPressed {
		mb.mainPressed = false
		mb.Invalidate()
		if pt.In(main) {
			mb.fireClick()
		}
		return true
	}
	return false
}

// OnMouseMove ведёт подсветку пунктов открытого меню и наведение на кнопку.
func (mb *MenuButton) OnMouseMove(x, y int) {
	if mb.menu.routeMove(x, y) {
		return
	}
	mb.Button.OnMouseMove(x, y)
}

// OnKeyEvent: открытому меню — навигация по пунктам; ↓ открывает меню; Enter
// и Space выполняют действие разделённой кнопки или открывают меню простой.
func (mb *MenuButton) OnKeyEvent(e KeyEvent) {
	if mb.menu.routeKey(e) {
		mb.Invalidate()
		return
	}
	if !mb.IsEnabled() || !e.Pressed {
		return
	}
	switch e.Code {
	case KeyDown:
		mb.openMenu(image.Point{}, false)
	case KeyEnter, KeySpace:
		if !mb.Split {
			mb.openMenu(image.Point{}, false)
		} else if !mb.actionDisabled {
			mb.fireClick()
		}
	}
}

// ─── Overlay ────────────────────────────────────────────────────────────────

// HasOverlay реализует OverlayDrawer.
func (mb *MenuButton) HasOverlay() bool { return mb.menu.open() }

// DrawOverlay рисует открытое меню поверх UI.
func (mb *MenuButton) DrawOverlay(ctx DrawContext) { mb.menu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (mb *MenuButton) OverlayBounds() image.Rectangle { return mb.menu.overlayBounds() }

// Dismiss закрывает меню. Реализует Dismissable.
func (mb *MenuButton) Dismiss() {
	if mb.menu.open() {
		mb.menu.dismiss()
		mb.Invalidate()
	}
}

// overflowItem — пункт меню переполнения панели инструментов: подменю с
// пунктами кнопки, у разделённой — с действием первым пунктом.
func (mb *MenuButton) overflowItem() (MenuItem, bool) {
	text := mb.Text
	if text == "" {
		text = mb.ToolTip
	}
	if text == "" {
		return MenuItem{}, false
	}
	var sub []MenuItem
	if !mb.menuDisabled {
		if mb.OnOpening != nil {
			mb.OnOpening()
		}
		sub = append(sub, mb.Items...)
	}
	if mb.Split {
		self := mb
		head := []MenuItem{{Text: text, Disabled: mb.actionDisabled, OnClick: func() { self.fireClick() }}}
		if len(sub) > 0 {
			head = append(head, MenuItem{Separator: true})
		}
		sub = append(head, sub...)
	}
	if len(sub) == 0 {
		return MenuItem{Text: text, Disabled: true}, true
	}
	return MenuItem{Text: text, Disabled: !mb.IsEnabled(), SubItems: sub}, true
}

// buildXAMLMenuButton строит кнопку с меню из <MenuButton> или <SplitButton>:
//
//	<SplitButton Name="pull" Content="Pull" Icon="pull.png" IconPosition="Top">
//	  <MenuItem Header="{Loc Fetch}"/>
//	  <MenuItem Header="Pull (rebase)"/>
//	</SplitButton>
//
// Атрибуты — как у Button; Split="True" делает разделённой и <MenuButton>,
// IsActionEnabled/IsMenuEnabled="False" выключают части.
func buildXAMLMenuButton(el xElement, tag, baseDir string) Widget {
	btn, _ := buildXAMLButton(el, baseDir).(*Button)
	if btn == nil {
		return nil
	}
	mb := &MenuButton{Button: btn}
	mb.Split = tag == "splitbutton" || strings.EqualFold(el.attr("Split"), "true")
	mb.actionDisabled = strings.EqualFold(el.attr("IsActionEnabled"), "false")
	mb.menuDisabled = strings.EqualFold(el.attr("IsMenuEnabled"), "false")
	items, keys := parseMenuItems(el, baseDir)
	mb.Items = items
	registerLocItemList(keys, func(i int, s string) {
		if i < len(mb.Items) {
			mb.Items[i].Text = s
		}
	})
	return mb
}
