package native

// fullscreen_rect.go — арифметика полного экрана Win32: что запомнить, на какой
// прямоугольник и со стилем какого вида встать, как вернуться.
//
// Файл без платформенного суффикса намеренно: вызовы Win32 здесь не нужны, а
// ошибки в этой арифметике (окно вернулось за край экрана, потеряло рамку
// ресайза, осталось маленьким после смены монитора) проверяются тестами на
// любой машине (fullscreen_rect_test.go).

import "image"

// FullscreenThickFrame — WS_THICKFRAME. Рамка изменения размера на время полного
// экрана снимается: с ней система ограничивает окно максимальным размером
// отслеживания (по умолчанию — размер основного экрана плюс рамка), и окно не
// покрыло бы монитор больше основного.
//
//facadegen:skip
const FullscreenThickFrame = 0x00040000

// FullscreenSnapshot — состояние окна, которое надо вернуть при выходе из полного
// экрана.
//
//facadegen:skip
type FullscreenSnapshot struct {
	// Rect — внешний прямоугольник окна в экранных координатах в НОРМАЛЬНОМ
	// (не развёрнутом) состоянии.
	Rect image.Rectangle
	// Style — стиль окна (GWL_STYLE) до входа.
	Style uint32
	// Maximized — окно перед входом было развёрнуто. На время полного экрана
	// его сворачивают в нормальное состояние (иначе система держит окно в
	// zoomed и таскбар не скрывается), а при выходе разворачивают снова.
	Maximized bool
}

// FullscreenEnterPlan возвращает стиль и прямоугольник полноэкранного окна для
// монитора monitor (rcMonitor, не rcWork: полный экран закрывает и таскбар).
//
//facadegen:skip
func FullscreenEnterPlan(style uint32, monitor image.Rectangle) (uint32, image.Rectangle) {
	return style &^ FullscreenThickFrame, monitor
}

// FullscreenLeavePlan возвращает стиль и прямоугольник окна после выхода из полного
// экрана. cur — стиль окна сейчас; monitors — прямоугольники мониторов СЕЙЧАС:
// пока окно было на весь экран, монитор мог исчезнуть или смениться
// разрешение, и сохранённый прямоугольник оказался бы за краем (fsFitToMonitors).
//
// Рамка ресайза возвращается из снимка, остальные биты стиля берутся из cur:
// за время полного экрана приложение могло их менять (WS_MINIMIZEBOX и т. п.).
//
//facadegen:skip
func FullscreenLeavePlan(cur uint32, snap FullscreenSnapshot, monitors []image.Rectangle) (uint32, image.Rectangle) {
	style := (cur &^ FullscreenThickFrame) | (snap.Style & FullscreenThickFrame)
	return style, fsFitToMonitors(snap.Rect, monitors)
}

// FullscreenRefit решает, надо ли переставить полноэкранное окно, лежащее в cur, чтобы
// оно закрывало монитор monitor. ok=false — окно уже ровно на мониторе.
//
// Нужен для WM_DISPLAYCHANGE (сменилось разрешение), переноса окна на другой
// монитор (Win+Shift+стрелка) и смены DPI.
//
//facadegen:skip
func FullscreenRefit(cur, monitor image.Rectangle) (image.Rectangle, bool) {
	if monitor.Empty() || cur == monitor {
		return cur, false
	}
	return monitor, true
}

// fsFitToMonitors возвращает прямоугольник r, годный для обычного окна: если
// достаточная часть окна лежит на каком-то из мониторов, он остаётся как есть;
// иначе окно переносится на ближайший монитор (размер сжимается, если окно не
// помещается).
//
// «Достаточная» — не меньше четверти площади: окно, торчащее за край на
// несколько пикселей, пользователь двигает сам, а окно, ушедшее целиком за
// отключённый монитор, достать нечем.
func fsFitToMonitors(r image.Rectangle, monitors []image.Rectangle) image.Rectangle {
	if r.Empty() || len(monitors) == 0 {
		return r
	}
	area := r.Dx() * r.Dy()
	for _, m := range monitors {
		in := r.Intersect(m)
		if !in.Empty() && in.Dx()*in.Dy()*4 >= area {
			return r
		}
	}
	// Ближайший монитор — по расстоянию между центрами.
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	best, bestD := monitors[0], -1
	for _, m := range monitors {
		mx, my := (m.Min.X+m.Max.X)/2, (m.Min.Y+m.Max.Y)/2
		d := (cx-mx)*(cx-mx) + (cy-my)*(cy-my)
		if bestD < 0 || d < bestD {
			best, bestD = m, d
		}
	}
	w, h := min(r.Dx(), best.Dx()), min(r.Dy(), best.Dy())
	x := min(max(r.Min.X, best.Min.X), best.Max.X-w)
	y := min(max(r.Min.Y, best.Min.Y), best.Max.Y-h)
	return image.Rect(x, y, x+w, y+h)
}
