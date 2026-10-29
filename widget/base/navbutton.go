package base

// navbutton.go — кнопка сворачивания боковой области в полосе заголовка и
// размеры полосы: общие для окна (widget/chrome) и диалога.

import (
	"image"
	"image/color"
)

// Служебные: для подпакетов widget, в фасад не входят.
//
//facadegen:skip
const (
	NavBtnSize   = 24 // сторона кнопки сворачивания в полосе заголовка
	TitleBarGap  = 8  // зазор между элементами полосы
	TitleBarPadV = 4  // отступ начинки от верхнего и нижнего края полосы
	TitleBarMinW = 48 // уже этого начинку не показываем — рисовать нечего
	// TitleBarHandleW — свободное место между начинкой приложения и кнопками
	// окна. Полоса заголовка — это ещё и место, за которое окно тащат: поле
	// поиска во всю ширину не оставляло бы за него ни одной точки.
	TitleBarHandleW = 72
	// LocaleBadgeReserve — место, отводимое плашке локали в полосе заголовка.
	// Её ширина зависит от подписи локали и известна только на отрисовке, а
	// раскладка начинки идёт в SetBounds — берём заведомо достаточное.
	LocaleBadgeReserve = 44
	dlgVPad            = 12 // вертикальный отступ содержимого диалога
)

// TitleNavButton — кнопка в левой части полосы заголовка, сворачивающая боковую
// область приложения.
//
// Само сворачивание делает приложение: движок не знает, ЧТО у него слева —
// панель навигации, дерево или разделённый SplitPanel. Здесь только кнопка,
// её состояние и уведомление; иначе пришлось бы выдумывать за приложение,
// какой виджет считать «боковой областью» и что делать с остальными.
//
// Служебный: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
type TitleNavButton struct {
	Base
	Icon      image.Image // иконка в развёрнутом состоянии (nil — встроенная «≡»)
	IconAlt   image.Image // иконка в свёрнутом состоянии (nil — та же)
	Collapsed bool
	hover     bool
	// armed — кнопка «взведена» нажатием: срабатывание на отпускании и только
	// если курсор всё ещё над ней (семантика кнопок заголовка, как у ✕).
	armed  bool
	FG     color.RGBA // цвет встроенной иконки; хост ставит его перед отрисовкой
	capMgr CaptureManager
	Toggle func()
}

func (nb *TitleNavButton) currentIcon() image.Image {
	if nb.Collapsed && nb.IconAlt != nil {
		return nb.IconAlt
	}
	return nb.Icon
}

func (nb *TitleNavButton) Draw(ctx DrawContext) {
	b := nb.bounds
	if b.Empty() {
		return
	}
	if nb.hover {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.BtnHoverBG)
	}
	if ic := nb.currentIcon(); ic != nil {
		sz := b.Dx() - 8
		if h := b.Dy() - 8; h < sz {
			sz = h
		}
		if sz < 1 {
			return
		}
		ctx.DrawImageScaled(ic, b.Min.X+(b.Dx()-sz)/2, b.Min.Y+(b.Dy()-sz)/2, sz, sz)
		return
	}
	// Встроенная «≡»: три полосы. Рисуется цветом подписи заголовка — иконка
	// живёт в полосе и обязана следовать её теме, а не палитре кнопок.
	const lineW, lineH, gap = 14, 2, 4
	x := b.Min.X + (b.Dx()-lineW)/2
	y := b.Min.Y + (b.Dy()-(lineH*3+gap*2))/2
	for i := 0; i < 3; i++ {
		ctx.FillRect(x, y+i*(lineH+gap), lineW, lineH, nb.FG)
	}
}

func (nb *TitleNavButton) OnMouseMove(x, y int) {
	h := image.Pt(x, y).In(nb.bounds)
	if h != nb.hover {
		nb.hover = h
		nb.Invalidate()
	}
}

// SetCaptureManager инжектится движком: захват нужен, чтобы отпускание пришло
// кнопке, даже если курсор ушёл с неё.
func (nb *TitleNavButton) SetCaptureManager(cm CaptureManager) { nb.capMgr = cm }

func (nb *TitleNavButton) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(nb.bounds)
}

func (nb *TitleNavButton) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(nb.bounds)
	if e.Pressed {
		if !over {
			return false
		}
		nb.armed = true
		return true
	}
	if !nb.armed {
		return false
	}
	nb.armed = false
	if nb.capMgr != nil {
		nb.capMgr.ReleaseCapture()
	}
	if over && nb.Toggle != nil {
		nb.Toggle()
	}
	return true
}
