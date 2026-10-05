// window_icon.go — значок окна в заголовке и системное меню по нему.
//
// Заголовок окна рисует движок: подпись, кнопки управления, вкладки. Значка
// приложения в нём не было — SetNavIcons рисует иконку КНОПКИ сворачивания
// боковой панели (12 px, с наведением), а не окна. Потребителю, которому нужен
// значок слева от подписи (классическое окно Windows 2000 — это 16 px и
// системное меню по щелчку), приходилось класть свой виджет поверх заголовка
// и сдвигать подпись пробелами в названии.
//
// Значок — такой же ребёнок полосы заголовка, как кнопка сворачивания: его
// геометрию считает окно, подпись сдвигается за него сама, а нажатие на него
// не тащит окно (см. titleBarChildHit).
package widget

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget/svg"
)

// Размеры значка окна в заголовке, px.
const (
	// windowIconSize — сторона значка, если тема её не задала
	// (theme.KeyWindowCaptionIconSize).
	windowIconSize = 16
	// windowIconInsetClassic и windowIconInset — отступ значка от левого края
	// полосы: у классики 2 px, как в Windows 2000, у прочих тем — как у
	// кнопки сворачивания.
	windowIconInsetClassic = 2
	windowIconInset        = 8
	// windowIconGapClassic и windowIconGap — воздух между значком и первой
	// буквой подписи.
	windowIconGapClassic = 3
	windowIconGap        = 8
)

func init() {
	RegisterStrings("EN", map[string]string{
		"win.sys.restore":  "Restore",
		"win.sys.move":     "Move",
		"win.sys.size":     "Size",
		"win.sys.minimize": "Minimize",
		"win.sys.maximize": "Maximize",
		"win.sys.close":    "Close",
	})
	RegisterStrings("RU", map[string]string{
		"win.sys.restore":  "Восстановить",
		"win.sys.move":     "Переместить",
		"win.sys.size":     "Размер",
		"win.sys.minimize": "Свернуть",
		"win.sys.maximize": "Развернуть",
		"win.sys.close":    "Закрыть",
	})
}

// windowIcon — виджет значка окна в полосе заголовка.
type windowIcon struct {
	Base
	owner *Window
	img   image.Image
	doc   *svg.Document
}

// SetIcon задаёт значок окна — картинку слева от подписи в заголовке.
// nil убирает значок вместе с системным меню.
//
// Размер значка — метрика темы (theme.KeyWindowCaptionIconSize; Windows 2000 —
// 16, без метрики 16): картинка другого размера масштабируется. Подпись
// заголовка сдвигается за значок сама — пробелы в названии не нужны.
//
// Щелчок по значку открывает системное меню (SystemMenuItems), двойной —
// закрывает окно (OnClose), как в Windows; нажатие на значок не тащит окно.
// Значок есть только в заголовке Windows-раскладки: в mac-раскладке слева
// стоят кнопки окна, и значку там не место.
func (w *Window) SetIcon(img image.Image) {
	if img == nil {
		w.removeIcon()
		return
	}
	ic := w.ensureIcon()
	ic.img, ic.doc = img, nil
	w.layoutTitleBar()
	w.Invalidate()
}

// SetIconSVG задаёт значок окна векторной картинкой: она растеризуется в
// физическом размере значка, поэтому на HiDPI остаётся чёткой. currentColor в
// SVG — цвет подписи заголовка. При ошибке разбора прежний значок остаётся.
func (w *Window) SetIconSVG(data []byte) error {
	doc, err := svg.Parse(data)
	if err != nil {
		return err
	}
	ic := w.ensureIcon()
	ic.img, ic.doc = nil, doc
	w.layoutTitleBar()
	w.Invalidate()
	return nil
}

// HasIcon сообщает, есть ли у окна значок в заголовке.
func (w *Window) HasIcon() bool { return w.icon != nil }

// IconBounds — прямоугольник значка в абсолютных координатах. Пустой, если
// значка нет или он не показан (mac-раскладка, окно без заголовка).
func (w *Window) IconBounds() image.Rectangle {
	if w.icon == nil || !IsWidgetVisible(w.icon) {
		return image.Rectangle{}
	}
	return w.icon.Bounds()
}

// ensureIcon создаёт значок и кладёт его в дерево окна.
func (w *Window) ensureIcon() *windowIcon {
	if w.icon == nil {
		w.icon = &windowIcon{owner: w}
		w.AddChild(w.icon)
	}
	return w.icon
}

// removeIcon убирает значок; открытое меню закрывается. Само меню остаётся
// ребёнком окна — пустое и закрытое, оно ничего не рисует и не ловит ввод.
func (w *Window) removeIcon() {
	ic := w.icon
	if ic == nil {
		return
	}
	if w.sysMenu != nil {
		w.sysMenu.Close()
	}
	w.RemoveChild(ic)
	w.icon = nil
	w.layoutTitleBar()
	w.Invalidate()
}

// isIconWidget сообщает, что c — значок окна.
func (w *Window) isIconWidget(c Widget) bool {
	return w.icon != nil && Widget(w.icon) == c
}

// iconSize — сторона значка в полосе высотой barH.
func (w *Window) iconSize(barH int) int {
	s := w.style().CaptionIconSize
	if s <= 0 {
		s = windowIconSize
	}
	if m := barH - 2; s > m {
		s = m
	}
	if s < 1 {
		s = 1
	}
	return s
}

// layoutIcon ставит значок в полосу заголовка tb (из layoutTitleBar).
func (w *Window) layoutIcon(tb image.Rectangle) {
	ic := w.icon
	if ic == nil {
		return
	}
	if tb.Empty() || w.macTitleBar() {
		ic.SetVisible(false)
		ic.SetBounds(image.Rectangle{})
		return
	}
	s := w.iconSize(tb.Dy())
	inset := windowIconInset
	if w.style().Classic3D {
		inset = windowIconInsetClassic
	}
	x := tb.Min.X + inset
	y := tb.Min.Y + (tb.Dy()-s)/2
	ic.SetVisible(true)
	ic.SetBounds(image.Rect(x, y, x+s, y+s))
}

// titleTextLeft — левый край подписи заголовка: двенадцать точек от края
// полосы, а за значком окна и за кнопкой сворачивания — после них. Одна точка
// правды для отрисовки подписи и для раскладки начинки полосы.
func (w *Window) titleTextLeft(tb image.Rectangle) int {
	x := tb.Min.X + 12
	if r := w.IconBounds(); !r.Empty() {
		gap := windowIconGap
		if w.style().Classic3D {
			gap = windowIconGapClassic
		}
		x = r.Max.X + gap
	}
	if w.navBtn != nil && !w.navBtn.bounds.Empty() {
		x = w.navBtn.bounds.Max.X + titleBarGap
	}
	return x
}

// titleTabsLeft — левый край вкладок заголовка; def — прежний (без значка).
func (w *Window) titleTabsLeft(def int) int {
	if r := w.IconBounds(); !r.Empty() {
		if x := r.Max.X + 4; x > def {
			return x
		}
	}
	return def
}

// iconClaims сообщает, что точка над значком, у которого есть меню: такое
// нажатие принадлежит значку, а не перетаскиванию окна. Значок без меню —
// просто картинка, и за него окно тащат, как за подпись.
func (w *Window) iconClaims(pt image.Point) bool {
	if w.icon == nil || !w.hasSystemMenu() {
		return false
	}
	r := w.IconBounds()
	return !r.Empty() && pt.In(r)
}

// ─── Отрисовка и ввод значка ────────────────────────────────────────────────

func (ic *windowIcon) Draw(ctx DrawContext) {
	b := ic.Bounds()
	if b.Empty() {
		return
	}
	switch {
	case ic.doc != nil:
		_, _, tc := ic.owner.titleColors()
		DrawSVG(ctx, ic.doc, b, tc, false)
	case ic.img != nil:
		if ib := ic.img.Bounds(); ib.Dx() == b.Dx() && ib.Dy() == b.Dy() {
			ctx.DrawImage(ic.img, b.Min.X, b.Min.Y)
		} else {
			ctx.DrawImageScaled(ic.img, b.Min.X, b.Min.Y, b.Dx(), b.Dy())
		}
	}
}

// OnMouseButton: нажатие открывает системное меню, двойное — закрывает окно.
//
// Меню открывается на нажатии, а не на отпускании, как в Windows. Нажатие,
// которое только что погасило это же меню, его заново не открывает: движок
// гасит открытые меню до доставки события (PopupMenu.DismissedByPress).
func (ic *windowIcon) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !ic.owner.iconClaims(image.Pt(e.X, e.Y)) {
		return false
	}
	if e.Clicks >= 2 {
		ic.owner.closeFromIcon()
		return true
	}
	ic.owner.toggleSystemMenu()
	return true
}

// closeFromIcon — двойной щелчок по значку: то же, что кнопка ×.
func (w *Window) closeFromIcon() {
	if w.OnClose != nil {
		w.OnClose()
	}
}

// ─── Системное меню ─────────────────────────────────────────────────────────

// SetMaximized сообщает окну, что оно развёрнуто на весь экран: системное меню
// включает «Восстановить» и выключает «Развернуть», «Переместить», «Размер».
// Состояние ведёт хост (window.Window делает это сам); виджетное окно на
// канвасе его не меняет.
func (w *Window) SetMaximized(v bool) {
	if w.maximized == v {
		return
	}
	w.maximized = v
	w.Invalidate()
}

// IsMaximized возвращает последнее сообщённое состояние окна.
func (w *Window) IsMaximized() bool { return w.maximized }

// SetSystemMenu задаёт пункты системного меню, открывающегося по значку окна.
//
//	win.SetSystemMenu(append(win.SystemMenuItems(), widget.MenuItem{Separator: true},
//	    widget.MenuItem{Text: "Новая вкладка", OnClick: newTab}))
//
// nil возвращает пункты по умолчанию (SystemMenuItems), пустой, но не nil
// срез убирает меню совсем: значок остаётся картинкой, и за него окно тащат.
// Можно вызывать до SetIcon.
// Заданный список приложение ведёт само — доступность пунктов не
// пересчитывается.
func (w *Window) SetSystemMenu(items []MenuItem) {
	w.sysCustom = items != nil
	w.sysItems = items
	if len(items) == 0 && w.sysMenu != nil {
		w.sysMenu.Close()
	}
}

// SystemMenuItems возвращает пункты системного меню по умолчанию:
// «Восстановить», «Переместить», «Размер», «Свернуть», «Развернуть» и
// «Закрыть» — подписи через Tr (ключи win.sys.*), действия на колбэки окна.
//
//   - Восстановить и Развернуть — OnMaximize (хост переключает состояние), доступны
//     по IsMaximized и по тому, что у окна вообще есть такая кнопка;
//   - Переместить — OnNativeMove, Размер — OnNativeResize: их задаёт хост, умеющий
//     двигать окно средствами ОС; без него пункты недоступны, а окно двигают
//     за заголовок;
//   - Свернуть — OnMinimize; Закрыть — OnClose.
//
// Пункт без обработчика отображается недоступным, как в Windows: пункты не
// пропадают, чтобы меню не прыгало от окна к окну.
func (w *Window) SystemMenuItems() []MenuItem {
	maxed := w.IsMaximized()
	canMin := w.btnCount() >= 2 && w.OnMinimize != nil
	canMax := w.btnCount() >= 3 && w.OnMaximize != nil
	canMove := !maxed && w.OnNativeMove != nil
	canSize := !maxed && w.Resize == ResizeModeCanResize && w.Style != WindowStyleNone &&
		w.OnNativeResize != nil

	return []MenuItem{
		{Text: Tr("win.sys.restore"), Disabled: !(maxed && canMax), OnClick: w.toggleMaximize},
		{Text: Tr("win.sys.move"), Disabled: !canMove, OnClick: func() {
			if w.OnNativeMove != nil {
				w.OnNativeMove()
			}
		}},
		{Text: Tr("win.sys.size"), Disabled: !canSize, OnClick: func() {
			if w.OnNativeResize != nil {
				w.OnNativeResize(NativeEdgeBottom | NativeEdgeRight)
			}
		}},
		{Text: Tr("win.sys.minimize"), Disabled: !canMin, OnClick: func() {
			if w.OnMinimize != nil {
				w.OnMinimize()
			}
		}},
		{Text: Tr("win.sys.maximize"), Disabled: maxed || !canMax, OnClick: w.toggleMaximize},
		{Separator: true},
		{Text: Tr("win.sys.close"), Shortcut: "Alt+F4", Disabled: w.OnClose == nil, OnClick: w.closeFromIcon},
	}
}

// toggleMaximize — «Восстановить» и «Развернуть»: хост переключает состояние
// одним колбэком, как кнопка □.
func (w *Window) toggleMaximize() {
	if w.OnMaximize != nil {
		w.OnMaximize()
	}
}

// hasSystemMenu сообщает, что у значка есть меню: по умолчанию — есть;
// пустой список, заданный приложением, его убирает.
func (w *Window) hasSystemMenu() bool {
	return w.icon != nil && (!w.sysCustom || len(w.sysItems) > 0)
}

// SystemMenu возвращает popup-меню значка (nil — значка нет или меню убрано).
// Пункты в нём собираются при каждом открытии.
func (w *Window) SystemMenu() *PopupMenu {
	if !w.hasSystemMenu() {
		return nil
	}
	if w.sysMenu == nil {
		w.sysMenu = NewPopupMenu()
		w.AddChild(w.sysMenu)
	}
	return w.sysMenu
}

// OpenSystemMenu открывает системное меню под значком (true — открыто).
// Закрытое нажатием меню повторно не открывается (PopupMenu.Toggle).
func (w *Window) OpenSystemMenu() bool {
	return w.toggleSystemMenu()
}

// toggleSystemMenu открывает меню под значком или закрывает, если оно открыто.
func (w *Window) toggleSystemMenu() bool {
	m := w.SystemMenu()
	r := w.IconBounds()
	if m == nil || r.Empty() {
		return false
	}
	if !m.IsOpen() && !m.DismissedByPress() {
		// Пункты — на момент открытия: язык, состояние окна и набор колбэков
		// меняются между открытиями.
		items := w.sysItems
		if !w.sysCustom {
			items = w.SystemMenuItems()
		}
		m.SetItems(items)
	}
	return m.Toggle(r.Min.X, r.Max.Y)
}
