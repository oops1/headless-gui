// tray.go — публичный, платформенно-независимый API иконки в трее,
// системных balloon-уведомлений и восстановления окна из трея.
//
// Реально поддерживается только на Windows (Shell_NotifyIcon). Бэкенд
// реализует приватный интерфейс trayHost; на прочих платформах (X11/Wayland/
// macOS/headless) окно его не реализует — методы становятся вежливыми no-op'ами
// и возвращают ошибку/ничего. Компилируется на всех платформах.
package window

import (
	"errors"
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// errTrayUnsupported возвращается методами трея на платформах без поддержки.
var errTrayUnsupported = errors.New("window: иконка в трее поддерживается только на Windows")

// trayHost — приватная возможность бэкенда: иконка в области уведомлений,
// balloon-уведомления и сворачивание/восстановление окна. Реализуется
// Win32Window (tray_windows.go). Все методы вызываются на UI-потоке окна.
type trayHost interface {
	// setTrayIcon добавляет/обновляет иконку в трее (NIM_ADD/NIM_MODIFY).
	setTrayIcon(icon image.Image, tooltip string) error
	// removeTrayIcon убирает иконку (NIM_DELETE).
	removeTrayIcon()
	// setTrayClickHandler регистрирует диспетчер кликов по иконке
	// (button: 0=left, 1=right, 2=middle; doubleClick — двойной клик).
	setTrayClickHandler(fn func(button int, doubleClick bool))
	// showBalloon показывает системное всплывающее уведомление (NIF_INFO).
	// infoFlag: 0=none, 1=info, 2=warning, 3=error (NIIF_*).
	showBalloon(title, text string, infoFlag uint32) error
	// setBalloonClickHandler регистрирует колбэк клика по balloon
	// (NIN_BALLOONUSERCLICK).
	setBalloonClickHandler(fn func())
	// hideToTray прячет окно (SW_HIDE — исчезает из панели задач).
	hideToTray()
	// restoreFromTray показывает окно и передаёт ему фокус ОС.
	restoreFromTray()
	// cursorScreenPos возвращает позицию курсора в экранных координатах.
	cursorScreenPos() (int, int)
	// SetForeground выводит окно-носитель на передний план (класс. трей-трюк).
	SetForeground()
}

// severityInfoFlag переводит widget.DialogSeverity в dwInfoFlags balloon'а
// (NIIF_*): Info/Question → NIIF_INFO, Warning → NIIF_WARNING,
// Error → NIIF_ERROR, None → NIIF_NONE.
func severityInfoFlag(s widget.DialogSeverity) uint32 {
	switch s {
	case widget.SeverityInfo, widget.SeverityQuestion:
		return 1 // NIIF_INFO
	case widget.SeverityWarning:
		return 2 // NIIF_WARNING
	case widget.SeverityError:
		return 3 // NIIF_ERROR
	}
	return 0 // NIIF_NONE
}

// trayButton переводит код кнопки бэкенда в widget.MouseButton.
func trayButton(button int) widget.MouseButton {
	switch button {
	case 1:
		return widget.MouseRight
	case 2:
		return widget.MouseMiddle
	default:
		return widget.MouseLeft
	}
}

// ─── Чистые помощники (тестируются без ОС) ───────────────────────────────────

// Коды мышиных сообщений Win32, продублированы здесь для платформенно-
// независимого маппинга событий трея (uCallbackMessage несёт код события в
// младшем слове lParam). См. trayEventToClick / handleTrayCallback.
const (
	trayEvtLButtonUp        = 0x0202 // WM_LBUTTONUP
	trayEvtLButtonDblClk    = 0x0203 // WM_LBUTTONDBLCLK
	trayEvtRButtonUp        = 0x0205 // WM_RBUTTONUP
	trayEvtMButtonUp        = 0x0208 // WM_MBUTTONUP
	trayEvtBalloonUserClick = 0x0405 // NIN_BALLOONUSERCLICK (WM_USER+5)
)

// trayEventToClick разбирает событие иконки трея (младшее слово lParam
// callback-сообщения) в кнопку/двойной клик/клик-по-balloon. ok=false —
// событие нас не интересует (move, down и т.п.).
func trayEventToClick(ev uint32) (button int, doubleClick, balloon, ok bool) {
	switch ev & 0xFFFF {
	case trayEvtLButtonUp:
		return 0, false, false, true
	case trayEvtLButtonDblClk:
		return 0, true, false, true
	case trayEvtRButtonUp:
		return 1, false, false, true
	case trayEvtMButtonUp:
		return 2, false, false, true
	case trayEvtBalloonUserClick:
		return 0, false, true, true
	}
	return 0, false, false, false
}

// iconColorBuffer конвертирует премультиплицированный RGBA в top-down 32bpp
// BGRA-буфер цветного DIB иконки (длина = w*h*4). Порядок R,G,B,A → B,G,R,A.
func iconColorBuffer(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := (y*w + x) * 4
			out[di+0] = img.Pix[si+2] // B
			out[di+1] = img.Pix[si+1] // G
			out[di+2] = img.Pix[si+0] // R
			out[di+3] = img.Pix[si+3] // A
		}
	}
	return out
}

// iconMaskBuffer строит 1-битную AND-маску иконки из альфы: бит=1 → пиксель
// прозрачный (a < threshold). Сканлайны выровнены на границу WORD (16 бит),
// как требует GDI CreateBitmap. Возвращает буфер и stride в байтах.
func iconMaskBuffer(img *image.RGBA, threshold uint8) ([]byte, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := ((w + 15) / 16) * 2
	out := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := img.Pix[img.PixOffset(b.Min.X+x, b.Min.Y+y)+3]
			if a < threshold {
				out[y*stride+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	return out, stride
}

// ─── Публичный API (window.Window) ───────────────────────────────────────────

// SetTrayIcon помещает иконку в область уведомлений (системный трей) с
// всплывающей подсказкой tooltip. Повторный вызов обновляет иконку/подсказку.
// icon масштабируется до системного размера маленькой иконки (SM_CXSMICON);
// прозрачность берётся из альфа-канала.
//
// Разумный дефолт: если задан SetTrayIcon и приложение НЕ переопределило
// SetOnTrayClick, двойной левый клик по иконке восстанавливает окно из трея.
//
// Можно вызывать до Run() (состояние применится при создании окна) или из
// обработчиков UI во время работы. На не-Windows платформах — no-op (до Run
// возвращает nil, во время работы — errTrayUnsupported).
func (win *Window) SetTrayIcon(icon image.Image, tooltip string) error {
	win.trayIcon = icon
	win.trayTooltip = tooltip
	win.trayIconWant = true
	if win.native == nil {
		return nil // применится в Run() → applyPendingTray
	}
	th, ok := win.native.(trayHost)
	if !ok {
		return errTrayUnsupported
	}
	win.ensureTrayDispatcher(th)
	return th.setTrayIcon(icon, tooltip)
}

// RemoveTrayIcon убирает иконку из трея.
func (win *Window) RemoveTrayIcon() {
	win.trayIconWant = false
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		th.removeTrayIcon()
	}
}

// SetOnTrayClick регистрирует колбэк кликов по иконке трея. button — какая
// кнопка нажата, doubleClick — был ли это двойной клик. Переопределяет
// дефолтное поведение (двойной левый → восстановить окно).
func (win *Window) SetOnTrayClick(fn func(button widget.MouseButton, doubleClick bool)) {
	win.onTrayClick = fn
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		win.ensureTrayDispatcher(th)
	}
}

// SetTrayMenu задаёт НАШЕ контекстное меню (widget.PopupMenu), показываемое по
// правому клику на иконке трея. Меню рендерится в собственном окне-попапе у
// курсора (благодаря хосту popup-оверлеев) — даже за пределами главного окна и
// при скрытом окне. Передайте nil, чтобы убрать меню.
func (win *Window) SetTrayMenu(menu *widget.PopupMenu) {
	win.trayMenu = menu
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		win.ensureTrayDispatcher(th)
		win.attachTrayMenu()
	}
}

// ShowBalloon показывает системное balloon-уведомление с заголовком title,
// текстом text и значком по severity (Info/Warning/Error). Требует ранее
// установленной иконки трея (SetTrayIcon) — иначе возвращает ошибку.
func (win *Window) ShowBalloon(title, text string, severity widget.DialogSeverity) error {
	if win.native == nil {
		return errTrayUnsupported
	}
	th, ok := win.native.(trayHost)
	if !ok {
		return errTrayUnsupported
	}
	return th.showBalloon(title, text, severityInfoFlag(severity))
}

// SetOnBalloonClick регистрирует колбэк клика пользователя по balloon'у
// (NIN_BALLOONUSERCLICK).
func (win *Window) SetOnBalloonClick(fn func()) {
	win.onBalloonClick = fn
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		th.setBalloonClickHandler(fn)
	}
}

// HideToTray прячет окно (исчезает из панели задач). Иконка трея остаётся.
func (win *Window) HideToTray() {
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		th.hideToTray()
	}
}

// RestoreFromTray показывает окно и выводит его на передний план.
func (win *Window) RestoreFromTray() {
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		th.restoreFromTray()
	}
}

// ─── Внутреннее ──────────────────────────────────────────────────────────────

// applyPendingTray применяет отложенное состояние трея после создания окна.
// Вызывается из Run(). На платформах без trayHost — no-op.
func (win *Window) applyPendingTray() {
	th, ok := win.native.(trayHost)
	if !ok {
		return
	}
	if win.trayIconWant || win.onTrayClick != nil || win.trayMenu != nil {
		win.ensureTrayDispatcher(th)
	}
	if win.onBalloonClick != nil {
		th.setBalloonClickHandler(win.onBalloonClick)
	}
	win.attachTrayMenu()
	if win.trayIconWant && win.trayIcon != nil {
		_ = th.setTrayIcon(win.trayIcon, win.trayTooltip)
	}
}

// ensureTrayDispatcher единожды регистрирует диспетчер кликов трея у бэкенда.
func (win *Window) ensureTrayDispatcher(th trayHost) {
	if win.trayDispatcherSet {
		return
	}
	win.trayDispatcherSet = true
	th.setTrayClickHandler(win.dispatchTrayClick)
}

// dispatchTrayClick — единый диспетчер кликов трея: вызывает пользовательский
// колбэк (или дефолт: двойной левый клик восстанавливает окно) и показывает
// трей-меню по правому клику.
func (win *Window) dispatchTrayClick(button int, doubleClick bool) {
	if win.onTrayClick != nil {
		win.onTrayClick(trayButton(button), doubleClick)
	} else if doubleClick && button == 0 && win.trayIconWant {
		win.RestoreFromTray()
	}
	if button == 1 && !doubleClick && win.trayMenu != nil {
		win.showTrayMenu()
	}
}

// showTrayMenu показывает трей-меню у курсора. Экранные координаты курсора
// переводятся в логические координаты носителя ((screen − окно)/scale), затем
// menu.Show открывает оверлей — хост popup'ов вынесет его в окно у курсора.
// Перед показом носитель выводится на передний план (классический трей-трюк:
// иначе меню не закроется по клику мимо него).
func (win *Window) showTrayMenu() {
	th, ok := win.native.(trayHost)
	if !ok || win.trayMenu == nil {
		return
	}
	sx, sy := th.cursorScreenPos()
	th.SetForeground()
	cx, cy := win.native.GetPosition()
	scale := win.scale
	if scale <= 0 {
		scale = 1
	}
	lx := int(float64(sx-cx)/scale + 0.5)
	ly := int(float64(sy-cy)/scale + 0.5)
	win.trayMenu.Show(lx, ly)
}

// attachTrayMenu добавляет трей-меню в дерево корневого виджета носителя, чтобы
// движок собирал его оверлей в sink (иначе popup-хост его не увидит).
func (win *Window) attachTrayMenu() {
	if win.trayMenu == nil || win.eng == nil {
		return
	}
	root := win.eng.Root()
	if root == nil {
		return
	}
	for _, c := range root.Children() {
		if c == widget.Widget(win.trayMenu) {
			return
		}
	}
	root.AddChild(win.trayMenu)
}
