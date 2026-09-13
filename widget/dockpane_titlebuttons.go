// dockpane_titlebuttons.go — кнопки приложения в заголовке DockPane (GG-77).
//
// Штатные кнопки заголовка (закрепить, открепить, закрыть) движок рисует сам,
// а свою приложению поставить было негде. Кнопку «обновить с GitHub» или «≡» с
// меню вида панели приходилось держать строкой над содержимым — это строка,
// отнятая у самого содержимого, и раскладка, непривычная по SmartGit и IDE.
package widget

import (
	"image"
	"image/color"
)

// dockPaneAppGap — зазор между кнопками приложения и штатными.
const dockPaneAppGap = 6

// DockPaneButton — кнопка приложения в заголовке DockPane.
type DockPaneButton struct {
	// Icon — значок. Перекрашивается в цвет текста заголовка с сохранением
	// прозрачности, как глифы штатных кнопок: один значок годится и для
	// тёмной темы, и для светлой, и для подсвеченного заголовка активной
	// панели. У кнопки без значка рисуется знак «≡».
	Icon image.Image
	// KeepIconColors — рисовать значок своими цветами, без перекраски.
	KeepIconColors bool
	// Tooltip — подсказка при наведении.
	Tooltip string
	// OnClick — действие по щелчку. У кнопки с меню не зовётся.
	OnClick func()
	// Menu — пункты меню, открываемого под кнопкой. Непустое — кнопка
	// открывает меню.
	Menu []MenuItem
	// MenuFunc собирает пункты перед каждым открытием (отметки сортировки,
	// недоступные пункты). Если задан, важнее Menu.
	MenuFunc func() []MenuItem
	// Hidden — кнопка скрыта и места не занимает. Переключается без
	// перестройки списка: SetTitleButtonHidden.
	Hidden bool
	// Disabled — кнопка видна приглушённой и не нажимается.
	Disabled bool
}

// dockTintedIcon — значок кнопки, перекрашенный в цвет заголовка.
type dockTintedIcon struct {
	col color.RGBA
	img *image.RGBA
}

// SetTitleButtons задаёт кнопки приложения в заголовке. Они стоят слева от
// штатных, в порядке списка; последняя — ближе всех к штатным. nil убирает
// кнопки.
func (p *DockPane) SetTitleButtons(btns []DockPaneButton) {
	p.menu.dismiss()
	p.titleBtns = append([]DockPaneButton(nil), btns...)
	p.titleTint = nil
	if p.hoverBtn >= dockBtnCustom {
		p.hoverBtn = dockBtnNone
	}
	if p.armedBtn >= dockBtnCustom {
		p.armedBtn = dockBtnNone
	}
	p.Invalidate()
}

// TitleButtons возвращает копию кнопок приложения.
func (p *DockPane) TitleButtons() []DockPaneButton {
	return append([]DockPaneButton(nil), p.titleBtns...)
}

// SetTitleButtonHidden скрывает или показывает i-ю кнопку: значок GitHub
// нужен только у репозитория на github.com.
func (p *DockPane) SetTitleButtonHidden(i int, hidden bool) {
	if i < 0 || i >= len(p.titleBtns) || p.titleBtns[i].Hidden == hidden {
		return
	}
	p.titleBtns[i].Hidden = hidden
	if hidden && p.menu.open() && p.menuFor == i {
		p.menu.dismiss()
	}
	p.Invalidate()
}

// SetTitleButtonDisabled выключает или включает i-ю кнопку.
func (p *DockPane) SetTitleButtonDisabled(i int, disabled bool) {
	if i < 0 || i >= len(p.titleBtns) || p.titleBtns[i].Disabled == disabled {
		return
	}
	p.titleBtns[i].Disabled = disabled
	if disabled && p.menu.open() && p.menuFor == i {
		p.menu.dismiss()
	}
	p.Invalidate()
}

// titleButtonRects — прямоугольники кнопок приложения по индексам списка.
// Скрытая и не поместившаяся в узкий заголовок — пустой прямоугольник.
func (p *DockPane) titleButtonRects() []image.Rectangle {
	if len(p.titleBtns) == 0 {
		return nil
	}
	rects := make([]image.Rectangle, len(p.titleBtns))
	_, _, pinR := p.buttonRects()
	if pinR.Empty() {
		return rects
	}
	tb := p.titleBarRect()
	s := pinR.Dx()
	x := pinR.Min.X - dockPaneAppGap - s
	for i := len(p.titleBtns) - 1; i >= 0; i-- {
		if p.titleBtns[i].Hidden {
			continue
		}
		if x < tb.Min.X+4 {
			break // заголовок узок: кнопка не помещается
		}
		rects[i] = image.Rect(x, pinR.Min.Y, x+s, pinR.Max.Y)
		x -= s + dockPaneBtnGap
	}
	return rects
}

// titleButtonAt — индекс кнопки приложения под точкой или -1.
func (p *DockPane) titleButtonAt(pt image.Point) int {
	for i, r := range p.titleButtonRects() {
		if !r.Empty() && pt.In(r) {
			return i
		}
	}
	return -1
}

// titleTextLimit — правая граница, до которой идёт заголовок: самая левая из
// кнопок, штатных или приложения.
func (p *DockPane) titleTextLimit() int {
	_, _, pinR := p.buttonRects()
	limit := pinR.Min.X
	for _, r := range p.titleButtonRects() {
		if !r.Empty() && r.Min.X < limit {
			limit = r.Min.X
		}
	}
	return limit
}

// drawTitleButtons рисует кнопки приложения.
func (p *DockPane) drawTitleButtons(ctx DrawContext) {
	rects := p.titleButtonRects()
	if len(rects) == 0 {
		return
	}
	col, bg := p.TitleText, p.TitleBG
	if p.active {
		bg = p.TitleActiveBG
		if p.TitleTextActive.A > 0 {
			col = p.TitleTextActive
		}
	}
	if len(p.titleTint) != len(p.titleBtns) {
		p.titleTint = make([]dockTintedIcon, len(p.titleBtns))
	}
	open := p.menu.open()
	for i, r := range rects {
		if r.Empty() {
			continue
		}
		b := p.titleBtns[i]
		kind := dockBtnCustom + dockPaneBtn(i)
		if !b.Disabled && (p.hoverBtn == kind || p.armedBtn == kind || (open && p.menuFor == i)) {
			ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{R: 255, G: 255, B: 255, A: 40})
		}
		c := col
		if b.Disabled {
			c = mixRGBA(col, bg, 0.55)
		}
		if b.Icon == nil {
			cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
			for _, dy := range []int{-3, 0, 3} {
				ctx.DrawHLine(cx-4, cy+dy, 9, c)
			}
			continue
		}
		sz := r.Dy() - 4
		if sz > 16 {
			sz = 16
		}
		var img image.Image = b.Icon
		if !b.KeepIconColors {
			img = p.tintedTitleIcon(i, c)
		}
		ctx.DrawImageScaled(img, r.Min.X+(r.Dx()-sz)/2, r.Min.Y+(r.Dy()-sz)/2, sz, sz)
	}
}

// tintedTitleIcon — значок i-й кнопки в цвете col (с кэшем до смены цвета или
// списка кнопок).
func (p *DockPane) tintedTitleIcon(i int, col color.RGBA) image.Image {
	t := &p.titleTint[i]
	if t.img == nil || t.col != col {
		t.img, t.col = tintImage(p.titleBtns[i].Icon, col), col
	}
	return t.img
}

// tintImage перекрашивает картинку в цвет col, сохраняя прозрачность.
func tintImage(src image.Image, col color.RGBA) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			aa := uint32(col.A) * (a >> 8) / 255
			i := out.PixOffset(x-b.Min.X, y-b.Min.Y)
			// image.RGBA хранит цвет, умноженный на альфу.
			out.Pix[i] = uint8(uint32(col.R) * aa / 255)
			out.Pix[i+1] = uint8(uint32(col.G) * aa / 255)
			out.Pix[i+2] = uint8(uint32(col.B) * aa / 255)
			out.Pix[i+3] = uint8(aa)
		}
	}
	return out
}

// activateTitleButton — отпускание над i-й кнопкой: меню под ней или действие.
func (p *DockPane) activateTitleButton(i int, at image.Point) {
	if i < 0 || i >= len(p.titleBtns) {
		return
	}
	b := p.titleBtns[i]
	if b.Hidden || b.Disabled {
		return
	}
	items := b.Menu
	if b.MenuFunc != nil {
		items = b.MenuFunc()
	}
	if len(items) == 0 {
		if b.OnClick != nil {
			b.OnClick()
		}
		return
	}
	r := p.titleButtonRects()[i]
	m := p.menu.build(at.X, at.Y, append([]MenuItem(nil), items...))
	if m == nil {
		return
	}
	// Меню открывается по отпусканию — пропускать больше нечего.
	p.menu.armed = false
	p.menuFor = i
	m.Show(r.Min.X, r.Max.Y)
	p.Invalidate()
}

// HasOverlay реализует OverlayDrawer: открыто меню кнопки заголовка.
func (p *DockPane) HasOverlay() bool { return p.menu.open() }

// DrawOverlay рисует меню кнопки заголовка поверх UI.
func (p *DockPane) DrawOverlay(ctx DrawContext) { p.menu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (p *DockPane) OverlayBounds() image.Rectangle { return p.menu.overlayBounds() }

// ToolTipAt — подсказка кнопки приложения под точкой.
func (p *DockPane) ToolTipAt(x, y int) string {
	if i := p.titleButtonAt(image.Pt(x, y)); i >= 0 {
		return p.titleBtns[i].Tooltip
	}
	return ""
}
