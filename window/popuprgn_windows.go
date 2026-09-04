//go:build windows

// popuprgn_windows.go — выкройка окна-попапа по закрашенной части картинки.
//
// Вынесенный оверлей занимает ПРЯМОУГОЛЬНИК — объединение всего, что он
// рисует. У каскадного меню это полоса, раскрытое подменю и его дочернее
// подменю, стоящее правее и ниже; между ними остаётся площадь, которую никто
// не закрашивает. В холсте сквозь неё виден рабочий стол, а в отдельном окне
// ОС видна ЧЕРНОТА: окно непрозрачно, и «ничего» показывается чёрным
// прямоугольником — ровно то, что прислал заказчик Go.Git.
//
// Движок сообщает, что он закрасил (engine.OpaqueBands). Окно кроится по этим
// полосам, и за их пределами окна просто нет — чернеть нечему.
package window

import "image"

// applyOpaqueRegion выкраивает окно по полосам bands.
//
// Пустой список снимает выкройку: окно снова обычный прямоугольник. Так же
// поступаем, когда полос ровно одна и она покрывает весь кадр, — региона там
// не нужно, а лишний регион стоит окну перерисовки при каждом движении.
func (w *Win32Window) applyOpaqueRegion(bands []image.Rectangle, full image.Rectangle) {
	if w.hwnd == 0 {
		return
	}
	if len(bands) == 0 || (len(bands) == 1 && bands[0] == full) {
		procSetWindowRgn.Call(uintptr(w.hwnd), 0, 1)
		return
	}

	var total uintptr
	for _, b := range bands {
		if b.Empty() {
			continue
		}
		rgn, _, _ := procCreateRectRgn.Call(
			uintptr(b.Min.X), uintptr(b.Min.Y), uintptr(b.Max.X), uintptr(b.Max.Y))
		if rgn == 0 {
			continue
		}
		if total == 0 {
			total = rgn
			continue
		}
		// RGN_OR = 2: складываем полосы в одну область. Результат кладём в
		// total, а вторую область удаляем — иначе утечёт объект GDI, а их у
		// процесса конечное число.
		if _, _, _ = procCombineRgn.Call(total, total, rgn, 2); true {
			procDeleteObject.Call(rgn)
		}
	}
	if total == 0 {
		procSetWindowRgn.Call(uintptr(w.hwnd), 0, 1)
		return
	}
	// SetWindowRgn забирает владение регионом — удалять его после этого
	// нельзя, иначе система работает с уничтоженным объектом.
	procSetWindowRgn.Call(uintptr(w.hwnd), total, 1)
}

// blitPopupRegion — вспомогательный вызов для хоста попапов: положить кадр и
// сразу выкроить по нему окно.
func (w *Win32Window) blitPopupRegion(img *image.RGBA, bands []image.Rectangle) {
	if img == nil {
		return
	}
	w.BlitRGBA(img)
	w.applyOpaqueRegion(bands, img.Bounds())
}
