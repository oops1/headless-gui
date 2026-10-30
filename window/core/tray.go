// tray.go — публичный, платформенно-независимый API иконки в трее,
// системных balloon-уведомлений и восстановления окна из трея.
//
// Иконка, её меню и клики, уведомления — возможность platform/tray (ADR-0013
// §3): Windows — Shell_NotifyIcon, Linux (X11 и Wayland одинаково) —
// StatusNotifierItem и org.freedesktop.Notifications по D-Bus. Ядро создаёт
// трей окну после создания окна ОС (native.TrayService) и снимает при
// закрытии; без возможности (window/minimal, macOS) методы иконки и
// уведомлений — вежливые no-op'ы и ErrTrayUnsupported. Спрятать окно и вернуть
// его (HideToTray, RestoreFromTray) — операции самого окна у бэкенда, они
// работают и без трея.
//
// Разница платформ, о которой знает этот файл: контекстное меню трея на
// Windows рисуем МЫ (widget.PopupMenu у курсора), а на Linux его показывает
// сама панель по протоколу com.canonical.dbusmenu (Tray.SetTrayMenuNative).
package core

import (
	"image"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// trayWindow — операции окна, которые нужны трею: спрятать окно (исчезает из
// панели задач) и показать его снова с передачей фокуса ОС. Реализуют Win32,
// X11 и Wayland (там — no-op: xdg_toplevel спрятать нельзя). Зовутся на потоке
// окна (onNative).
type trayWindow interface {
	HideToTray()
	RestoreFromTray()
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
// обработчиков UI во время работы. Без трея (macOS, window/minimal) до Run
// возвращает nil, во время работы — native.ErrTrayUnsupported.
func (win *Window) SetTrayIcon(icon image.Image, tooltip string) error {
	win.trayIcon = icon
	win.trayTooltip = tooltip
	win.trayIconWant = true
	if win.native == nil {
		return nil // применится в Run() → applyPendingTray
	}
	t := win.trayOf()
	if t == nil {
		return native.ErrTrayUnsupported
	}
	win.ensureTrayDispatcher(t)
	return t.SetTrayIcon(icon, tooltip)
}

// RemoveTrayIcon убирает иконку из трея.
func (win *Window) RemoveTrayIcon() {
	win.trayIconWant = false
	if t := win.trayOf(); t != nil {
		t.RemoveTrayIcon()
	}
}

// SetOnTrayClick регистрирует колбэк кликов по иконке трея. button — какая
// кнопка нажата, doubleClick — был ли это двойной клик. Переопределяет
// дефолтное поведение (двойной левый → восстановить окно).
func (win *Window) SetOnTrayClick(fn func(button widget.MouseButton, doubleClick bool)) {
	win.onTrayClick = fn
	if t := win.trayOf(); t != nil {
		win.ensureTrayDispatcher(t)
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
	if t := win.trayOf(); t != nil {
		win.ensureTrayDispatcher(t)
		win.pushTrayMenuNative(t)
		win.attachTrayMenu()
	}
}

// ShowBalloon показывает системное balloon-уведомление с заголовком title,
// текстом text и значком по severity (Info/Warning/Error).
//
// Windows: требует ранее установленной иконки трея (SetTrayIcon) — уведомление
// принадлежит ей. Linux: иконка не нужна, уведомление показывает демон рабочего
// стола (org.freedesktop.Notifications); без демона возвращается ошибка.
// До Run, на macOS и в window/minimal — native.ErrTrayUnsupported.
func (win *Window) ShowBalloon(title, text string, severity widget.DialogSeverity) error {
	t := win.trayOf()
	if t == nil {
		return native.ErrTrayUnsupported
	}
	return t.ShowBalloon(title, text, severityInfoFlag(severity))
}

// SetOnBalloonClick регистрирует колбэк клика пользователя по уведомлению
// (Windows — NIN_BALLOONUSERCLICK, Linux — действие "default" в ActionInvoked).
// Колбэк выполняется на горутине движка, как и обработчики виджетов.
func (win *Window) SetOnBalloonClick(fn func()) {
	win.onBalloonClick = fn
	if t := win.trayOf(); t != nil {
		t.SetBalloonClickHandler(win.onEngineFunc(fn))
	}
}

// HideToTray прячет окно (исчезает из панели задач). Иконка трея остаётся.
// Это операция окна: работает и без трея (window/minimal).
func (win *Window) HideToTray() {
	if win.native == nil {
		return
	}
	// Зовут из обработчиков, то есть с горутины движка, а окно ОС прячется на
	// потоке своего цикла сообщений.
	if tw, ok := win.native.(trayWindow); ok {
		win.onNative(tw.HideToTray)
	}
}

// RestoreFromTray показывает окно и выводит его на передний план.
func (win *Window) RestoreFromTray() {
	if win.native == nil {
		return
	}
	if tw, ok := win.native.(trayWindow); ok {
		win.onNative(tw.RestoreFromTray)
	}
}

// ─── Внутреннее ──────────────────────────────────────────────────────────────

// pickupDeclarativeTray подхватывает декларации трея из XAML-корня widget.Window
// (<Window TrayIcon=… TrayTooltip=…> и <TrayMenu>): заполняет буферизованные поля
// трея, если приложение НЕ задало их явно до Run() (SetTrayIcon/SetTrayMenu).
// Приоритет у приложения: заданные им иконка/меню не перетираются. Вызывается в
// Run() ДО applyPendingTray, которая уже отправит итоговое состояние трею.
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

// trayOf — трей окна; nil — трея нет (до Run, без возможности, у бэкенда без
// трея). После закрытия окна поле остаётся (как native), а сам трей отвечает
// отказом.
func (win *Window) trayOf() native.Tray {
	if p := win.tray.Load(); p != nil {
		return *p
	}
	return nil
}

// startTray создаёт трей окну ОС: bringUp, после Create — до applyPendingTray.
// Трей новый, поэтому диспетчер и меню системы ему отдаются заново.
func (win *Window) startTray() {
	win.trayDispatcherSet = false
	win.trayMenuNative = false
	win.tray.Store(nil)
	f, ok := native.TrayService()
	if !ok {
		return
	}
	if t := f(win.native); t != nil {
		win.tray.Store(&t)
	}
}

// stopTray снимает трей при закрытии окна (tearDown): иконка и подписки
// уведомлений не переживают окно.
func (win *Window) stopTray() {
	if t := win.trayOf(); t != nil {
		t.Close()
	}
}

// applyPendingTray применяет отложенное состояние трея после создания окна.
// Вызывается из Run(). Без трея — no-op.
func (win *Window) applyPendingTray() {
	t := win.trayOf()
	if t == nil {
		return
	}
	// Уведомления доступны шире иконки (Linux — без неё), поэтому колбэк
	// клика по уведомлению — первым, независимо от иконки.
	if win.onBalloonClick != nil {
		t.SetBalloonClickHandler(win.onEngineFunc(win.onBalloonClick))
	}
	if win.trayIconWant || win.onTrayClick != nil || win.trayMenu != nil {
		win.ensureTrayDispatcher(t)
	}
	win.pushTrayMenuNative(t)
	win.attachTrayMenu()
	if win.trayIconWant && win.trayIcon != nil {
		_ = t.SetTrayIcon(win.trayIcon, win.trayTooltip)
	}
}

// ensureTrayDispatcher единожды регистрирует у трея диспетчер кликов и
// очередь для пунктов системного меню.
func (win *Window) ensureTrayDispatcher(t native.Tray) {
	if win.trayDispatcherSet {
		return
	}
	win.trayDispatcherSet = true
	t.SetTrayClickHandler(win.dispatchTrayClick)
	// Меню, которое показывает сама система (Linux), сообщает выбор пункта со
	// своего потока — пункт выполняется на горутине движка.
	t.SetTrayMenuRunner(win.onEngine)
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
	// система (Windows). На Linux панель уже нарисовала своё — см.
	// Tray.SetTrayMenuNative.
	if button == 1 && !doubleClick && win.trayMenu != nil && !win.trayMenuNative {
		win.showTrayMenu()
	}
}

// pushTrayMenuNative отдаёт меню трею: если его показывает сама система
// (Linux), запоминаем это — пока trayMenuNative взведён, наш PopupMenu в
// дерево виджетов не добавляется и по правому клику не открывается.
func (win *Window) pushTrayMenuNative(t native.Tray) {
	win.trayMenuNative = t.SetTrayMenuNative(win.trayMenu)
}

// showTrayMenu показывает трей-меню у курсора. Экранные координаты курсора
// переводятся в логические координаты носителя ((screen − окно)/scale), затем
// menu.Show открывает оверлей — хост popup'ов вынесет его в окно у курсора.
// Перед показом носитель выводится на передний план (классический трей-трюк:
// иначе меню не закроется по клику мимо него).
func (win *Window) showTrayMenu() {
	t := win.trayOf()
	if t == nil || win.trayMenu == nil {
		return
	}
	sx, sy := t.CursorScreenPos()
	if fg, ok := win.native.(foregrounder); ok {
		fg.SetForeground()
	}
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
