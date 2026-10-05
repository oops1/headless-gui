// appgroup.go — кнопки приложений: группа окон одного приложения и контекстное
// меню кнопки.
//
// Два вопроса, которые область приложений не решает сама:
//
//   - что делать со списком окон, когда у приложения их несколько. Кнопка
//     одна (taskbutton.group), а окна показывает предпросмотр: он слушает
//     область через GroupHoverArea и GroupClickArea, а область о нём ничего
//     не знает;
//   - какие команды лежат в меню кнопки. Состав и подписи — дело потребителя
//     (AppCommands): «Закрепить» для одной оболочки значит одно, для другой —
//     другое. Свои подписи по умолчанию у движка есть (DefaultAppCommands),
//     но это именно умолчание, а не навязанный набор.
package desktop

import (
	"image"
	"image/color"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// AppButton — что оболочке известно о кнопке приложения. Отдаётся
// потребителю, когда тот собирает команды контекстного меню.
type AppButton struct {
	// App — приложение кнопки; пусто у окна, не привязанного к каталогу.
	App AppID
	// Title — название приложения (заголовок окна, если каталог о нём не знает).
	Title string
	// Pinned — приложение закреплено на панели.
	Pinned bool
	// Windows — открытые окна приложения; пусто — оно не запущено.
	Windows []WindowInfo
}

// AppCommand — одна команда контекстного меню кнопки приложения.
type AppCommand struct {
	// Title — подпись; её даёт потребитель (или DefaultAppCommands).
	Title string
	// Icon — необязательный значок команды.
	Icon image.Image
	// Disabled — команда показана, но недоступна.
	Disabled bool
	// Separator — разделитель вместо команды (остальные поля игнорируются).
	Separator bool
	// Run выполняется после закрытия меню.
	Run func()
}

// AppCommands — источник команд меню кнопки приложения. Реализует потребитель.
//
// Зовётся из горутины кадра в момент правого щелчка; вызов должен быть быстрым.
// Пустой ответ — меню не открывается.
type AppCommands interface {
	Commands(b AppButton) []AppCommand
}

// AppCommandsFunc — AppCommands одной функцией.
type AppCommandsFunc func(b AppButton) []AppCommand

// Commands реализует AppCommands.
func (f AppCommandsFunc) Commands(b AppButton) []AppCommand { return f(b) }

// DefaultAppCommands — набор команд кнопки по умолчанию: запуск (ещё одно
// окно), закрепить или открепить, закрыть окно (все окна, если их несколько).
// Подписи — строки движка desktop.app.* (widget.Tr), потребитель переопределяет
// их через widget.RegisterStrings.
//
// Потребителю, которому нужно добавить пункт, достаточно вызвать эту функцию
// из своего AppCommands и дописать результат.
func DefaultAppCommands(cat AppCatalog, wm WindowModel, b AppButton) []AppCommand {
	var cmds []AppCommand
	if cat != nil && b.App != "" {
		app := b.App
		cmds = append(cmds, AppCommand{Title: b.Title, Run: func() { _ = cat.Launch(app) }})
		cmds = append(cmds, AppCommand{Separator: true})
		if b.Pinned {
			cmds = append(cmds, AppCommand{Title: tr(StrAppUnpin), Run: func() { cat.Unpin(app) }})
		} else {
			cmds = append(cmds, AppCommand{Title: tr(StrAppPin), Run: func() { cat.Pin(app) }})
		}
	}
	if wm != nil && len(b.Windows) > 0 {
		if len(cmds) > 0 {
			cmds = append(cmds, AppCommand{Separator: true})
		}
		ids := make([]WindowID, 0, len(b.Windows))
		for _, w := range b.Windows {
			ids = append(ids, w.ID)
		}
		title := tr(StrAppCloseWindow)
		if len(ids) > 1 {
			title = tr(StrAppCloseAll)
		}
		cmds = append(cmds, AppCommand{Title: title, Run: func() {
			for _, id := range ids {
				wm.Close(id)
			}
		}})
	}
	return cmds
}

// menuItems переводит команды в пункты меню, убирая разделители по краям и
// двойные: набор от потребителя собирается условно, и «пустая» часть оставила
// бы две черты подряд.
func menuItems(cmds []AppCommand) []widget.MenuItem {
	var items []widget.MenuItem
	for _, c := range cmds {
		if c.Separator {
			if len(items) == 0 || items[len(items)-1].Separator {
				continue
			}
			items = append(items, widget.MenuItem{Separator: true})
			continue
		}
		items = append(items, widget.MenuItem{
			Text: c.Title, Icon: c.Icon, Disabled: c.Disabled || c.Run == nil, OnClick: c.Run,
		})
	}
	if n := len(items); n > 0 && items[n-1].Separator {
		items = items[:n-1]
	}
	return items
}

// GroupHoverArea — область кнопок, у которых бывает несколько окон. Необязателен
// и дополняет HoverArea: предпросмотр проверяет его приведением типа, так что
// область потребителя, о группах не знающая, остаётся рабочей.
type GroupHoverArea interface {
	// WindowsAt — все окна кнопки i (больше одного — «стопка»).
	WindowsAt(i int) []WindowInfo
}

// GroupClickArea — область, которая отдаёт щелчок по стопке окон слушателю
// вместо того, чтобы переключать окно самой. Предпросмотр так показывает
// список окон по щелчку.
type GroupClickArea interface {
	// SetGroupListener сообщает, кому отдавать щелчок по кнопке со многими
	// окнами. nil — область справляется сама (переключает окна по кругу).
	SetGroupListener(fn func(idx int))
}

// appMenuHost — контекстное меню области, которое рисует и разбирает сама
// область.
//
// Меню, ни за кем не закреплённое, не рисуется вовсе: оверлеи собираются
// обходом дерева. В дерево же его класть нельзя — PopupMenu Focusable, и
// невидимое меню стало бы остановкой в обходе Tab по панели. Поэтому область
// реализует контракт оверлея сама и передаёт работу этому помощнику.
type appMenuHost struct {
	mu   sync.Mutex
	menu *widget.PopupMenu
	// tm — тема, которой красится меню: пока оно открыто, светлый режим панели
	// (taskbar.light) могут переключить, и перекрашивать приходится на лету, а
	// не пересоздавать меню.
	tm *theme.Manager
}

func (h *appMenuHost) get() *widget.PopupMenu {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.menu
}

func (h *appMenuHost) open() bool {
	m := h.get()
	return m != nil && m.IsOpen()
}

// show открывает меню у прямоугольника anchor, на стороне, обращённой к
// рабочему столу: над кнопкой у нижней панели (EdgeBottom), под ней у верхней
// (EdgeTop), справа от неё у левой (EdgeLeft) и слева у правой (EdgeRight).
func (h *appMenuHost) show(tm *theme.Manager, items []widget.MenuItem, anchor image.Rectangle, edge Edge) {
	if len(items) == 0 {
		return
	}
	m := widget.NewPopupMenu()
	m.SetItems(items)
	themeMenu(m, tm)
	h.mu.Lock()
	old := h.menu
	h.menu, h.tm = m, tm
	h.mu.Unlock()
	if old != nil {
		old.Close()
	}

	// Меню ставят у края кнопки, обращённого к рабочему столу: PopupMenu
	// умеет только «от точки», поэтому высоту узнаём пробным показом.
	switch edge {
	case EdgeTop:
		m.Show(anchor.Min.X, anchor.Max.Y)
	case EdgeLeft:
		m.Show(anchor.Max.X, anchor.Min.Y)
	case EdgeRight:
		m.Show(anchor.Min.X, anchor.Min.Y)
		if r := m.OverlayBounds(); !r.Empty() {
			m.Show(anchor.Min.X-r.Dx(), anchor.Min.Y)
		}
	default:
		m.Show(anchor.Min.X, anchor.Max.Y)
		if r := m.OverlayBounds(); !r.Empty() {
			m.Show(anchor.Min.X, anchor.Min.Y-r.Dy())
		}
	}
}

func (h *appMenuHost) drawOverlay(ctx widget.DrawContext) {
	if m := h.get(); m != nil && m.IsOpen() {
		h.mu.Lock()
		tm := h.tm
		h.mu.Unlock()
		themeMenu(m, tm) // режим панели мог смениться, пока меню открыто
		m.DrawOverlay(ctx)
	}
}

func (h *appMenuHost) overlayBounds() image.Rectangle {
	if m := h.get(); m != nil && m.IsOpen() {
		return m.OverlayBounds()
	}
	return image.Rectangle{}
}

func (h *appMenuHost) dismiss() {
	if m := h.get(); m != nil && m.IsOpen() {
		m.Close()
	}
}

// routeMouse отдаёт нажатие открытому меню; true — событие разобрано.
// Щелчок мимо гасит меню и не поглощается: он должен отработать как обычный.
func (h *appMenuHost) routeMouse(e widget.MouseEvent) bool {
	m := h.get()
	if m == nil || !m.IsOpen() {
		return false
	}
	if image.Pt(e.X, e.Y).In(m.Bounds()) {
		return m.OnMouseButton(e)
	}
	m.Close()
	return false
}

// routeMove ведёт подсветку пунктов; true — курсор над меню.
func (h *appMenuHost) routeMove(x, y int) bool {
	m := h.get()
	if m == nil || !m.IsOpen() {
		return false
	}
	m.OnMouseMove(x, y)
	return image.Pt(x, y).In(m.Bounds())
}

// routeKey отдаёт клавишу открытому меню (стрелки, Enter, Esc).
func (h *appMenuHost) routeKey(e widget.KeyEvent) bool {
	m := h.get()
	if m == nil || !m.IsOpen() {
		return false
	}
	m.OnKeyEvent(e)
	return true
}

// ─── Метки состояния кнопки ──────────────────────────────────────────────────

// taskButtonMuted — приглушать ли свёрнутое окно и незапущенное закреплённое
// приложение состоянием Disabled. По умолчанию да; Windows 10 отключает это,
// чтобы наведение подсвечивало их как обычные кнопки.
func taskButtonMuted(tm *theme.Manager) bool {
	return tm == nil || tm.GetFlag(KeyTaskButtonMuted, true)
}

// tmMetric читает метрику темы (0 — темы нет или метрика не объявлена).
func tmMetric(tm *theme.Manager, k theme.Key) float64 {
	if tm == nil {
		return 0
	}
	return tm.GetMetric(k)
}

// drawTaskMark рисует метку открытого окна под кнопкой r.
//
// Активное окно отмечено полосой метрики KeyTaskButtonUnderlineLen (во всю
// ширину в Windows 10). Запущенное, но не активное — полосой своей длины
// KeyTaskButtonUnderlineIdleLen, если тема её задала; иначе правило прежнее
// (DrawUnderline): полосу «во всю ширину» неактивному не рисуют вовсе, а
// короткую рисуют вдвое короче.
func drawTaskMark(ctx widget.DrawContext, tm *theme.Manager, r image.Rectangle, active bool, s *theme.Style) {
	thick := int(tmMetric(tm, KeyTaskButtonUnderline))
	if idle := tmMetric(tm, KeyTaskButtonUnderlineIdleLen); !active && idle > 0 {
		if t := int(tmMetric(tm, KeyTaskButtonUnderlineIdle)); t > 0 {
			thick = t
		}
		drawMarkBar(ctx, r, thick, idle, textOnly(s))
		return
	}
	DrawUnderline(ctx, r, thick, tmMetric(tm, KeyTaskButtonUnderlineLen), active, s)
}

// drawStack рисует торцы «стопки» правее значка: несколько тонких чёрточек,
// каждая следующая короче и бледнее. Так кнопка сообщает, что за ней не одно
// окно. Число чёрточек и их размеры — метрики темы; нуль чёрточек — стопки
// на кнопке не видно (другие темы).
func drawStack(ctx widget.DrawContext, tm *theme.Manager, icon image.Rectangle, s *theme.Style) {
	n := int(tmMetric(tm, KeyTaskButtonStack))
	if n <= 0 || s == nil || icon.Empty() {
		return
	}
	w := int(tmMetric(tm, KeyTaskButtonStackWidth))
	if w < 1 {
		w = 1
	}
	gap := int(tmMetric(tm, KeyTaskButtonStackGap))
	x := icon.Max.X + int(tmMetric(tm, KeyTaskButtonStackOffset))
	for i := 0; i < n; i++ {
		// Каждая следующая чёрточка короче предыдущей на gap с каждого конца,
		// а первая — на gap*2: край «листа» стопки уходит вглубь.
		h := icon.Dy() - 2*gap*(i+1)
		if h <= 0 {
			break
		}
		y := icon.Min.Y + (icon.Dy()-h)/2
		ctx.FillRect(x, y, w, h, fadeColor(s.Text, 1/float64(i+1)))
		x += w + gap
	}
}

// fadeColor ослабляет цвет в k раз (0..1). Цвет хранится предумноженным по
// альфе, поэтому масштабируются все четыре канала.
func fadeColor(c color.RGBA, k float64) color.RGBA {
	if k >= 1 {
		return c
	}
	if k < 0 {
		k = 0
	}
	return color.RGBA{
		R: uint8(float64(c.R) * k), G: uint8(float64(c.G) * k),
		B: uint8(float64(c.B) * k), A: uint8(float64(c.A) * k),
	}
}

// textOnly — тот же стиль без цвета рамки: полоса (drawMarkBar) тогда берёт
// цвет текста. Запущенное неактивное окно отмечено цветом текста панели —
// светлым на тёмной и тёмным на светлой, — а не цветом рамки, который у
// большинства тем нейтральный серый и на светлой панели не виден.
func textOnly(s *theme.Style) *theme.Style {
	c := *s
	c.Border = color.RGBA{}
	return &c
}

// themeMenu красит меню стилями компонента «menu» темы рабочего стола. Без
// этого оно берёт цвета общей палитры виджетов и не следует за темой оболочки.
// Чего тема не объявила (нулевая альфа), то остаётся по умолчанию.
func themeMenu(m *widget.PopupMenu, tm *theme.Manager) {
	if tm == nil {
		return
	}
	base := tm.GetStyle("menu", "", theme.StateNormal)
	if base.Fill.A != 0 {
		m.Background = base.Fill
	}
	if base.Text.A != 0 {
		m.TextColor = base.Text
	}
	if base.Border.A != 0 {
		m.BorderColor, m.SeparatorColor = base.Border, base.Border
	}
	if hover := tm.GetStyle("menu", "item", theme.StateHover); hover.Fill.A != 0 && hover.Fill != base.Fill {
		m.HoverBG = hover.Fill
		if hover.Text.A != 0 {
			m.HoverTextColor = hover.Text
		} else if base.Text.A != 0 {
			m.HoverTextColor = base.Text
		}
	}
	// Недоступный пункт: тема без своего правила отдаёт цвет обычного текста
	// (состояние не объявлено), и тогда остаётся приглушённый по умолчанию.
	if dis := tm.GetStyle("menu", "item", theme.StateDisabled); dis.Text.A != 0 && dis.Text != base.Text {
		m.DisabledColor = dis.Text
	}
}
