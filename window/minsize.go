// minsize.go — минимальный размер окна из приложения.
//
// Бэкенды ограничение соблюдают: Win32 через WM_GETMINMAXINFO, X11 через
// WM_NORMAL_HINTS, Wayland через xdg_toplevel.set_min_size. А задать его из
// приложения было нечем: минимум брался ТОЛЬКО из полей MinWidth/MinHeight
// корневого widget.Window и применялся ровно один раз, внутри Run. Значит его
// нельзя было ни поменять во время работы (окно с двумя режимами — компактным
// и полным — требует разных минимумов), ни задать приложению, у которого
// корень дерева не widget.Window.
package window

// SetMinSize задаёт минимальный размер окна в ЛОГИЧЕСКИХ пикселях.
//
// Логических, а не физических: приложение думает в тех же единицах, что и
// виджеты, а пересчёт по HiDPI-масштабу — забота окна. Оно же пересчитывает
// минимум заново, когда окно переезжает на монитор с другим DPI: физический
// минимум, посчитанный однажды, на другом мониторе означал бы другой размер
// на глаз.
//
// Ноль по оси снимает ограничение по этой оси — то же соглашение, что у
// бэкендов (Win32 minW/minH, X11 PMinSize, xdg_toplevel.set_min_size).
//
// Вызывать можно и до Run() — минимум применится при создании окна, — и в
// любой момент работы. Заданный явно, он ОТМЕНЯЕТ значения MinWidth/MinHeight
// корневого widget.Window: явный вызов конкретнее разметки.
func (win *Window) SetMinSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	win.minW, win.minH, win.minWant = width, height, true
	win.applyMinSize()
}

// MinSize возвращает заданный минимум в логических пикселях (0, 0 — не задан).
func (win *Window) MinSize() (int, int) {
	if !win.minWant {
		return 0, 0
	}
	return win.minW, win.minH
}

// applyMinSize отправляет минимум бэкенду с учётом текущего масштаба.
//
// До создания окна бэкенда ещё нет — вызов просто откладывается: Run применит
// его сам. Так же ведут себя SetTrayIcon и SetIcon.
func (win *Window) applyMinSize() {
	if !win.minWant || win.native == nil {
		return
	}
	k := win.scale
	if k <= 0 {
		k = 1
	}
	win.native.SetMinSize(scaleMin(win.minW, k), scaleMin(win.minH, k))
}

// scaleMin переводит логический размер в физический, сохраняя ноль нулём:
// ноль означает «ограничения нет», и округление не должно превращать его в
// единицу.
func scaleMin(v int, k float64) int {
	if v <= 0 {
		return 0
	}
	return int(float64(v)*k + 0.5)
}

// pickupWidgetMinSize берёт минимум из корневого widget.Window, если явного
// вызова SetMinSize не было.
//
// Прежнее поведение остаётся умолчанием: разметка, задавшая MinWidth, работает
// как работала. Явный вызов её перекрывает — он конкретнее.
func (win *Window) pickupWidgetMinSize(mw, mh int) {
	if win.minWant || (mw <= 0 && mh <= 0) {
		return
	}
	win.minW, win.minH, win.minWant = mw, mh, true
}
