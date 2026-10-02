//go:build windows

package window

// hittest_windows.go — ответы системе о зонах окна (Aero Snap и Snap Layouts).
//
// Окно borderless: заголовок и кнопки рисует приложение, система о них не
// знает. Пока она не знала, окно двигалось выставлением позиции из дельт
// мыши — без прилипания к краям и раскладок, — а наведение на кнопку
// «развернуть» не показывало макеты привязки Windows 11: их показывают
// только над зоной, объявленной HTMAXBUTTON.
//
// Теперь приложение может сказать, что под курсором, и система сама делает
// привычное: тащит окно за заголовок, разворачивает его двойным щелчком и
// предлагает макеты. Без колбэка всё как прежде.

// SetHitTest подключает колбэк зон окна. Реализует hitTester.
func (w *Win32Window) SetHitTest(fn func(x, y int) HitArea) {
	w.hitTestMu.Lock()
	w.hitTest = fn
	w.hitTestMu.Unlock()
}

// hitAreaAt спрашивает, что лежит в точке окна (клиентские координаты в
// ФИЗИЧЕСКИХ пикселях), и переводит ответ в код Win32. Перевод в логические
// делает window.Window — масштаб знает он, а не бэкенд.
//
// ok=false — колбэка нет или там обычное содержимое: тогда решает прежний
// код, то есть края и DefWindowProc.
func (w *Win32Window) hitAreaAt(x, y int) (uintptr, bool) {
	w.hitTestMu.Lock()
	fn := w.hitTest
	w.hitTestMu.Unlock()
	if fn == nil {
		return 0, false
	}
	switch fn(x, y) {
	case HitCaption:
		return htCaption, true
	case HitMinButton:
		return htMinButton, true
	case HitMaxButton:
		return htMaxButton, true
	case HitCloseButton:
		return htClose, true
	}
	return 0, false
}

// ncButtonArea — что под курсором в событиях нерабочей области (кнопки
// заголовка): система шлёт их отдельными сообщениями WM_NC*.
func ncButtonArea(wparam uintptr) (HitArea, bool) {
	switch wparam {
	case htMinButton:
		return HitMinButton, true
	case htMaxButton:
		return HitMaxButton, true
	case htClose:
		return HitCloseButton, true
	}
	return HitClient, false
}
