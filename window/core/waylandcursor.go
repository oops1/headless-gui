package core

// waylandcursor.go — картинки курсоров для Wayland.
//
// Курсор под Wayland рисует клиент: компоновщик только показывает поверхность,
// которую ему дали (wl_pointer.set_cursor). Готовых курсоров в протоколе нет —
// в системе они лежат темой XCursor, но её разбор тянет за собой поиск темы,
// размеров и анимаций. Поэтому формы рисуются здесь, семью маленькими
// картинками: этого хватает на всё, что просит движок (стрелка, текст, рука,
// четыре стрелки размера).
//
// Компоновщик, объявивший wp_cursor_shape_manager_v1, обслуживается дешевле —
// у него курсор просят по имени, и он рисует курсор системной темы (см.
// wlCursorShape). Эти картинки — запасной путь для тех, кто расширение не
// поддерживает.
//
// Файл без платформенного суффикса: рисование — чистая функция, её тесты идут
// в общем прогоне.

import "image"

// Размер картинки курсора. 24×24 хватает всем формам и совпадает с обычным
// размером курсора темы при масштабе 1. Не больше 32×32: пользовательский
// указатель крупнее не пройдёт через RDP (WinLine).
const wlCursorSize = 24

// Формы курсора движка (widget.Cursor): значения приходят в SetCursor числом.
const (
	wlCursorArrow = iota
	wlCursorIBeam
	wlCursorHand
	wlCursorSizeWE
	wlCursorSizeNS
	wlCursorSizeNWSE
	wlCursorSizeNESW
	wlCursorNone // курсор спрятан: картинки нет, поверхность курсора не показывается
	wlCursorGrab
	wlCursorGrabbing
	wlCursorSizeAll
	wlCursorCrosshair
	wlCursorWait
	wlCursorCount
)

// wlCursorShape — номер формы в wp_cursor_shape_device_v1.shape.
// 0 — формы нет, курсор придётся рисовать самому.
func wlCursorShape(c int) uint32 {
	switch c {
	case wlCursorArrow:
		return 1 // default
	case wlCursorIBeam:
		return 9 // text
	case wlCursorHand:
		return 4 // pointer
	case wlCursorSizeWE:
		return 26 // ew-resize
	case wlCursorSizeNS:
		return 27 // ns-resize
	case wlCursorSizeNWSE:
		return 29 // nwse-resize
	case wlCursorSizeNESW:
		return 28 // nesw-resize
	case wlCursorGrab:
		return 16 // grab
	case wlCursorGrabbing:
		return 17 // grabbing
	case wlCursorSizeAll:
		return 33 // all-scroll
	case wlCursorCrosshair:
		return 8 // crosshair
	case wlCursorWait:
		return 6 // wait
	}
	// wlCursorNone тоже без имени: спрятать курсор можно только пустой
	// поверхностью (wl_pointer.set_cursor), у cursor-shape-v1 такой формы нет.
	return 0
}

// wlCursorPixels рисует курсор в ARGB8888 с домноженной альфой (формат wl_shm
// 0, байты B,G,R,A) и возвращает картинку и горячую точку.
//
// Неизвестная форма — стрелка: курсор обязан быть виден всегда, «нет картинки»
// оставило бы под окном курсор соседнего приложения.
func wlCursorPixels(c int) (pix []byte, w, h, hotX, hotY int) {
	shape := wlCursorShapes[wlCursorArrow]
	if c >= 0 && c < len(wlCursorShapes) && wlCursorShapes[c].rows != nil {
		shape = wlCursorShapes[c]
	}
	pix = make([]byte, wlCursorSize*wlCursorSize*4)
	for y, row := range shape.rows {
		if y >= wlCursorSize {
			break
		}
		for x := 0; x < len(row) && x < wlCursorSize; x++ {
			var r, g, b, a byte
			switch row[x] {
			case 'X': // контур
				a = 255
			case 'o': // заливка
				r, g, b, a = 255, 255, 255, 255
			default:
				continue
			}
			i := (y*wlCursorSize + x) * 4
			pix[i+0], pix[i+1], pix[i+2], pix[i+3] = b, g, r, a
		}
	}
	return pix, wlCursorSize, wlCursorSize, shape.hotX, shape.hotY
}

// wlCursorImage — то же изображение курсора, но как *image.RGBA (домноженная
// альфа, байты R,G,B,A) и с горячей точкой. Для бэкендов, которым нужна не
// память wl_shm, а картинка: Win32 строит из неё HCURSOR, X11 — две 1-битные
// карты (контур и маска).
func wlCursorImage(c int) (img *image.RGBA, hotX, hotY int) {
	pix, w, h, hx, hy := wlCursorPixels(c)
	img = image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i+3 < len(pix); i += 4 {
		img.Pix[i+0], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i+0], pix[i+3]
	}
	return img, hx, hy
}

// wlCursorShape описывает форму: строки картинки ('X' — контур, 'o' —
// заливка, прочее — прозрачно) и горячая точка.
type wlCursorArt struct {
	rows []string
	hotX int
	hotY int
}

// wlCursorShapes — формы в порядке констант widget.Cursor.
var wlCursorShapes = [wlCursorCount]wlCursorArt{
	wlCursorArrow: {hotX: 0, hotY: 0, rows: []string{
		"X",
		"XX",
		"XoX",
		"XooX",
		"XoooX",
		"XooooX",
		"XoooooX",
		"XooooooX",
		"XoooooooX",
		"XooooooooX",
		"XoooooXXXXX",
		"XooXooX",
		"XoX.XooX",
		"XX..XooX",
		"X....XooX",
		".....XooX",
		"......XooX",
		"......XooX",
		".......XX",
	}},
	wlCursorIBeam: {hotX: 5, hotY: 10, rows: []string{
		"",
		"",
		"XXXXXXXXXXX",
		"XooXoXooX",
		"XXXXoXXXX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"XXXXoXXXX",
		"XooXoXooX",
		"XXXXXXXXXXX",
	}},
	wlCursorHand: {hotX: 5, hotY: 0, rows: []string{
		"....XX",
		"...XooX",
		"...XooX",
		"...XooX",
		"...XooX",
		"...XooXXX",
		"...XooXooXX",
		"...XooXooXooX",
		"XX.XooXooXooX",
		"XooXooooooooX",
		"XoooooooooooX",
		".XooooooooooX",
		".XooooooooooX",
		"..XoooooooooX",
		"..XoooooooooX",
		"...XooooooooX",
		"...XXXXXXXXXX",
	}},
	wlCursorSizeWE: {hotX: 11, hotY: 5, rows: []string{
		"",
		"",
		"...X.................X",
		"..XoX...............XoX",
		".XooX...............XooX",
		"XoooXXXXXXXXXXXXXXXXXoooX",
		"XoooooooooooooooooooooooX",
		"XoooXXXXXXXXXXXXXXXXXoooX",
		".XooX...............XooX",
		"..XoX...............XoX",
		"...X.................X",
	}},
	wlCursorSizeNS: {hotX: 5, hotY: 11, rows: []string{
		".....X",
		"....XoX",
		"...XoooX",
		"..XXXoXXX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"....XoX",
		"..XXXoXXX",
		"...XoooX",
		"....XoX",
		".....X",
	}},
	wlCursorSizeNWSE: {hotX: 8, hotY: 8, rows: []string{
		"XXXXXXXX",
		"XoooooX",
		"XooooX",
		"XoooooX",
		"XooXoooX",
		"XoX.XoooX",
		"XX...XoooX",
		"X.....XoooX",
		".......XoooX.....X",
		"........XoooX...XX",
		".........XoooX.XoX",
		"..........XoooXooX",
		"...........XoooooX",
		"............XooooX",
		"...........XoooooX",
		"..........XXXXXXXX",
	}},
	wlCursorSizeNESW: {hotX: 8, hotY: 8, rows: []string{
		"........XXXXXXXX",
		".........XoooooX",
		"..........XooooX",
		".........XoooooX",
		".........XoooXoX",
		"........XoooX.XX",
		".......XoooX...X",
		"......XoooX",
		".....XoooX.......X",
		"XX..XoooX.......XX",
		"XoX.XooX.......XoX",
		"XooXooX.......XooX",
		"XoooooX......XoooX",
		"XooooX......XXXXXX",
		"XoooooX",
		"XXXXXXXX",
	}},
	wlCursorGrab: {hotX: 12, hotY: 12, rows: []string{
		"",
		"............XXXXX",
		"........XXXXXoooX",
		"........XoooXoooX",
		"........XoooXoooXXXXX",
		"....XXXXXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		".XXXXoooXoooXoooXoooX",
		".XooooooXoooXoooXoooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XXXXoooooooooooooooX",
		"....XoooooooooooooooX",
		"....XXoooooooooooooXX",
		".....XXXXXXXXXXXXXXX",
	}},
	wlCursorGrabbing: {hotX: 12, hotY: 12, rows: []string{
		"",
		"",
		"",
		"",
		"",
		"",
		"........XXXXXXXXX",
		"....XXXXXoooXoooXXXXX",
		"....XoooXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		"....XoooXoooXoooXoooX",
		".XXXXoooXoooXoooXoooX",
		".XooooooXoooXoooXoooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XooooooooooooooooooX",
		".XXXXoooooooooooooooX",
		"....XoooooooooooooooX",
		"....XXoooooooooooooXX",
		".....XXXXXXXXXXXXXXX",
	}},
	wlCursorSizeAll: {hotX: 11, hotY: 11, rows: []string{
		"..........XXX",
		".........XXoXX",
		"........XXoooXX",
		".......XXoooooXX",
		".......XoooooooX",
		".......XXXXoXXXX",
		"..........XoX",
		"...XXX....XoX....XXX",
		"..XXoX....XoX....XoXX",
		".XXooX....XoX....XooXX",
		"XXoooXXXXXXoXXXXXXoooXX",
		"XoooooooooooooooooooooX",
		"XXoooXXXXXXoXXXXXXoooXX",
		".XXooX....XoX....XooXX",
		"..XXoX....XoX....XoXX",
		"...XXX....XoX....XXX",
		"..........XoX",
		".......XXXXoXXXX",
		".......XoooooooX",
		".......XXoooooXX",
		"........XXoooXX",
		".........XXoXX",
		"..........XXX",
	}},
	wlCursorCrosshair: {hotX: 11, hotY: 11, rows: []string{
		"..........XXX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XXX",
		"XXXXXXXXXX...XXXXXXXXXX",
		"XooooooooX...XooooooooX",
		"XXXXXXXXXX...XXXXXXXXXX",
		"..........XXX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XoX",
		"..........XXX",
	}},
	wlCursorWait: {hotX: 11, hotY: 11, rows: []string{
		"",
		".....XXXXXXXXXXXXX",
		".....XoooooooooooX",
		".....XoooooooooooX",
		".....XXoooooooooXX",
		"......XoooooooooX",
		"......XXoooooooXX",
		".......XXoooooXX",
		"........XoooooX",
		"........XXoooXX",
		".........XXoXX",
		"..........XoX",
		".........XXoXX",
		".........XoooX",
		"........XXoooXX",
		"........XoooooX",
		".......XXoooooXX",
		"......XXoooooooXX",
		".....XXoooooooooXX",
		".....XoooooooooooX",
		".....XoooooooooooX",
		".....XXXXXXXXXXXXX",
	}},
}
