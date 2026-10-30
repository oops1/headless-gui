// tray.go — публичный, платформенно-независимый API иконки в трее,
// системных balloon-уведомлений и восстановления окна из трея.
//
// Поддержаны Windows (Shell_NotifyIcon, platform/win32/tray_windows.go) и Linux — X11 и
// Wayland (StatusNotifierItem по D-Bus, platform/tray/sni_linux.go). Бэкенд реализует
// приватный интерфейс trayHost; на прочих платформах (macOS/headless) окно его
// не реализует — методы становятся вежливыми no-op'ами и возвращают
// ошибку/ничего. Компилируется на всех платформах.
//
// Разница платформ, о которой знает этот файл: контекстное меню трея на
// Windows рисуем МЫ (widget.PopupMenu у курсора), а на Linux его показывает
// сама панель по протоколу com.canonical.dbusmenu. Бэкенд заявляет об этом
// опциональным интерфейсом nativeTrayMenu.
package core

import (
	"image"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// trayHost — приватная возможность бэкенда: иконка в области уведомлений,
// balloon-уведомления и сворачивание/восстановление окна. Реализуется
// Win32Window (platform/win32/tray_windows.go). Все методы вызываются на UI-потоке окна.
type trayHost interface {
	// SetTrayIcon добавляет/обновляет иконку в трее (NIM_ADD/NIM_MODIFY).
	SetTrayIcon(icon image.Image, tooltip string) error
	// RemoveTrayIcon убирает иконку (NIM_DELETE).
	RemoveTrayIcon()
	// SetTrayClickHandler регистрирует диспетчер кликов по иконке
	// (button: 0=left, 1=right, 2=middle; doubleClick — двойной клик).
	SetTrayClickHandler(fn func(button int, doubleClick bool))
	// ShowBalloon показывает системное всплывающее уведомление (NIF_INFO).
	// infoFlag: 0=none, 1=info, 2=warning, 3=error (NIIF_*).
	ShowBalloon(title, text string, infoFlag uint32) error
	// SetBalloonClickHandler регистрирует колбэк клика по balloon
	// (NIN_BALLOONUSERCLICK).
	SetBalloonClickHandler(fn func())
	// HideToTray прячет окно (SW_HIDE — исчезает из панели задач).
	HideToTray()
	// RestoreFromTray показывает окно и передаёт ему фокус ОС.
	RestoreFromTray()
	// CursorScreenPos возвращает позицию курсора в экранных координатах.
	CursorScreenPos() (int, int)
	// SetForeground выводит окно-носитель на передний план (класс. трей-трюк).
	SetForeground()
}

// nativeTrayMenu — опциональная возможность бэкенда: контекстное меню трея
// показывает САМА система, а не мы. Так устроен Linux (StatusNotifierItem
// отдаёт дерево меню панели по com.canonical.dbusmenu, и позиции иконки мы
// даже не знаем). Реализуется tray.SNI (platform/tray).
//
// SetTrayMenuNative передаёт бэкенду актуальное меню (nil — убрать) и
// возвращает true, если он взял показ на себя: тогда Window не добавляет
// PopupMenu в дерево виджетов и не открывает его по правому клику.
// Win32Window интерфейс НЕ реализует — путь Windows не меняется.
type nativeTrayMenu interface {
	SetTrayMenuNative(menu *widget.PopupMenu) bool
}

// balloonHost — УЗКАЯ часть возможностей трея: только системные уведомления.
// Выделена отдельно, потому что на Linux уведомления живут без иконки в трее
// (демон org.freedesktop.Notifications показывает их сам), а полноценного трея
// у X11/Wayland-бэкендов пока нет. Win32Window реализует и trayHost, и этот
// интерфейс; X11Window/WaylandWindow — только его (см. platform/tray/notify_linux.go).
type balloonHost interface {
	// ShowBalloon показывает системное уведомление.
	// infoFlag: 0=none, 1=info, 2=warning, 3=error.
	ShowBalloon(title, text string, infoFlag uint32) error
	// SetBalloonClickHandler регистрирует колбэк клика по уведомлению.
	SetBalloonClickHandler(fn func())
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
// возвращает nil, во время работы — native.ErrTrayUnsupported).
func (win *Window) SetTrayIcon(icon image.Image, tooltip string) error {
	win.trayIcon = icon
	win.trayTooltip = tooltip
	win.trayIconWant = true
	if win.native == nil {
		return nil // применится в Run() → applyPendingTray
	}
	th, ok := win.native.(trayHost)
	if !ok {
		return native.ErrTrayUnsupported
	}
	win.ensureTrayDispatcher(th)
	return th.SetTrayIcon(icon, tooltip)
}

// RemoveTrayIcon убирает иконку из трея.
func (win *Window) RemoveTrayIcon() {
	win.trayIconWant = false
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		th.RemoveTrayIcon()
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

// SetTrayMenu задаёт контекстное меню (widget.PopupMenu), показываемое по
// правому клику на иконке трея. Передайте nil, чтобы убрать меню.
//
// Windows: меню рендерим МЫ в собственном окне-попапе у курсора (благодаря
// хосту popup-оверлеев) — даже за пределами главного окна и при скрытом окне.
// Linux: то же дерево пунктов отдаётся панели по com.canonical.dbusmenu, и
// рисует его среда рабочего стола; наш попап при этом не показывается.
func (win *Window) SetTrayMenu(menu *widget.PopupMenu) {
	win.trayMenu = menu
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		win.ensureTrayDispatcher(th)
		win.pushTrayMenuNative()
		win.attachTrayMenu()
	}
}

// ShowBalloon показывает системное balloon-уведомление с заголовком title,
// текстом text и значком по severity (Info/Warning/Error).
//
// Windows: требует ранее установленной иконки трея (SetTrayIcon) — уведомление
// принадлежит ей. Linux: иконка не нужна, уведомление показывает демон рабочего
// стола (org.freedesktop.Notifications); без демона возвращается ошибка.
// macOS/headless — native.ErrTrayUnsupported.
func (win *Window) ShowBalloon(title, text string, severity widget.DialogSeverity) error {
	if win.native == nil {
		return native.ErrTrayUnsupported
	}
	bh, ok := win.native.(balloonHost)
	if !ok {
		return native.ErrTrayUnsupported
	}
	return bh.ShowBalloon(title, text, severityInfoFlag(severity))
}

// SetOnBalloonClick регистрирует колбэк клика пользователя по уведомлению
// (Windows — NIN_BALLOONUSERCLICK, Linux — действие "default" в ActionInvoked).
// Колбэк выполняется на горутине движка, как и обработчики виджетов.
func (win *Window) SetOnBalloonClick(fn func()) {
	win.onBalloonClick = fn
	if win.native == nil {
		return
	}
	if bh, ok := win.native.(balloonHost); ok {
		bh.SetBalloonClickHandler(win.onEngineFunc(fn))
	}
}

// HideToTray прячет окно (исчезает из панели задач). Иконка трея остаётся.
func (win *Window) HideToTray() {
	if win.native == nil {
		return
	}
	// Зовут из обработчиков, то есть с горутины движка, а окно ОС прячется на
	// потоке своего цикла сообщений.
	if th, ok := win.native.(trayHost); ok {
		win.onNative(th.HideToTray)
	}
}

// RestoreFromTray показывает окно и выводит его на передний план.
func (win *Window) RestoreFromTray() {
	if win.native == nil {
		return
	}
	if th, ok := win.native.(trayHost); ok {
		win.onNative(th.RestoreFromTray)
	}
}

// ─── Внутреннее ──────────────────────────────────────────────────────────────

// pickupDeclarativeTray подхватывает декларации трея из XAML-корня widget.Window
// (<Window TrayIcon=… TrayTooltip=…> и <TrayMenu>): заполняет буферизованные поля
// трея, если приложение НЕ задало их явно до Run() (SetTrayIcon/SetTrayMenu).
// Приоритет у приложения: заданные им иконка/меню не перетираются. Вызывается в
// Run() ДО applyPendingTray, которая уже отправит итоговое состояние бэкенду.
func (win *Window) pickupDeclarativeTray() {
	root := win.eng.Root()
	if root == nil {
		return
	}
	ww, ok := root.(*widget.Window)
	if !ok {
		return
	}
	if !win.trayIconWant && ww.TrayIconImage != nil {
		win.trayIcon = ww.TrayIconImage
		win.trayTooltip = ww.TrayTooltip
		win.trayIconWant = true
	}
	if win.trayMenu == nil && ww.TrayMenu != nil {
		win.trayMenu = ww.TrayMenu
	}
}

// applyPendingTray применяет отложенное состояние трея после создания окна.
// Вызывается из Run(). На платформах без trayHost — no-op.
func (win *Window) applyPendingTray() {
	// Уведомления доступны шире трея (Linux — без иконки), поэтому колбэк
	// клика применяем через balloonHost, до проверки полного trayHost.
	if bh, ok := win.native.(balloonHost); ok && win.onBalloonClick != nil {
		bh.SetBalloonClickHandler(win.onEngineFunc(win.onBalloonClick))
	}
	th, ok := win.native.(trayHost)
	if !ok {
		return
	}
	if win.trayIconWant || win.onTrayClick != nil || win.trayMenu != nil {
		win.ensureTrayDispatcher(th)
	}
	win.pushTrayMenuNative()
	win.attachTrayMenu()
	if win.trayIconWant && win.trayIcon != nil {
		_ = th.SetTrayIcon(win.trayIcon, win.trayTooltip)
	}
}

// ensureTrayDispatcher единожды регистрирует диспетчер кликов трея у бэкенда.
func (win *Window) ensureTrayDispatcher(th trayHost) {
	if win.trayDispatcherSet {
		return
	}
	win.trayDispatcherSet = true
	th.SetTrayClickHandler(win.dispatchTrayClick)
	// Меню, которое показывает сама система (Linux), сообщает выбор пункта со
	// своего потока — пункт выполняется на горутине движка.
	if mr, ok := th.(interface{ SetTrayMenuRunner(run func(fn func())) }); ok {
		mr.SetTrayMenuRunner(win.onEngine)
	}
}

// dispatchTrayClick — единый диспетчер кликов трея: вызывает пользовательский
// колбэк (или дефолт: двойной левый клик восстанавливает окно) и показывает
// трей-меню по правому клику.
//
// Бэкенд зовёт диспетчер со своего потока (Win32 — цикл сообщений, Linux —
// D-Bus), а колбэк приложения и меню — код виджетов: они выполняются на
// горутине движка (GG-68).
func (win *Window) dispatchTrayClick(button int, doubleClick bool) {
	if fn := win.onTrayClick; fn != nil {
		b := trayButton(button)
		win.onEngine(func() { fn(b, doubleClick) })
	} else if doubleClick && button == 0 && win.trayIconWant {
		win.RestoreFromTray()
	}
	// Правый клик открывает НАШЕ меню только там, где его не показывает
	// система (Windows). На Linux панель уже нарисовала своё — см. nativeTrayMenu.
	if button == 1 && !doubleClick && win.trayMenu != nil && !win.trayMenuNative {
		win.showTrayMenu()
	}
}

// pushTrayMenuNative отдаёт трей-меню бэкенду, если тот показывает меню сам
// (Linux). Запоминает результат: пока trayMenuNative взведён, наш PopupMenu в
// дерево виджетов не добавляется и по правому клику не открывается.
func (win *Window) pushTrayMenuNative() {
	nm, ok := win.native.(nativeTrayMenu)
	if !ok {
		return
	}
	win.trayMenuNative = nm.SetTrayMenuNative(win.trayMenu)
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
	sx, sy := th.CursorScreenPos()
	th.SetForeground()
	cx, cy := win.native.GetPosition()
	scale := win.scale
	if scale <= 0 {
		scale = 1
	}
	lx := int(float64(sx-cx)/scale + 0.5)
	ly := int(float64(sy-cy)/scale + 0.5)
	// Положение курсора снято сейчас, а меню открывается на горутине движка.
	menu := win.trayMenu
	win.onEngine(func() { menu.Show(lx, ly) })
}

// attachTrayMenu добавляет трей-меню в дерево корневого виджета носителя, чтобы
// движок собирал его оверлей в sink (иначе popup-хост его не увидит).
// Когда меню рисует система (Linux), добавлять его в дерево незачем.
func (win *Window) attachTrayMenu() {
	if win.trayMenu == nil || win.eng == nil || win.trayMenuNative {
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
