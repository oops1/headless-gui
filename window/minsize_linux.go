package window

import "encoding/binary"

// Файл БЕЗ платформенного build-тега: содержит только чистую логику GG-18
// (сборку тела свойства WM_NORMAL_HINTS для X11 и перевод физических пикселей
// в surface-local координаты xdg_toplevel.set_min_size для Wayland), без
// обращения к сокету X-сервера/композитора. Платформенные native_linux.go и
// native_wayland.go (оба под "linux && !android") зовут эти функции и сами
// отправляют готовый результат в протокол.
//
// ВАЖНО: имя файла оканчивается на "_linux", и Go по правилам именования
// (https://pkg.go.dev/go/build#hdr-Build_Constraints) добавляет отсюда
// НЕЯВНЫЙ build-тег "linux" — точно так же, как для native_linux.go, даже
// без единого явного //go:build. Убрать его нельзя: имена файлов зафиксированы
// заданием. Поэтому `go test ./window/...` на Windows-хосте этот файл и его
// тест молча пропускает — компилируется и реально исполняется он только при
// GOOS=linux (кросс-компиляция подтверждает лишь типы, не поведение; фактически
// тесты были прогнаны через WSL, где GOOS=linux нативен — см. отчёт).

// ─── X11: WM_NORMAL_HINTS ────────────────────────────────────────────────────

// x11NormalHintsMinSize собирает тело свойства WM_NORMAL_HINTS (тип
// WM_SIZE_HINTS = атом 41, format 32) с заполненными только min_width/
// min_height — остальные поля структуры (max_size, resize_inc, aspect,
// base_size, win_gravity) X11-бэкенд не использует и оставляет нулевыми
// без соответствующих битов флагов.
//
// Раскладка — ICCCM §4.1.2.3, 18×CARD32 = 72 байта (тот же формат, что отдаёт
// Xlib-структура XSizeHints через XSetWMNormalHints):
//
//	offset  0: flags                    (CARD32)
//	offset  4: pad: старые x/y/w/h      (4×CARD32, до-ICCCM совместимость)
//	offset 20: min_width, min_height    (INT32×2)   ← пишем при PMinSize
//	offset 28: max_width, max_height    (INT32×2)
//	offset 36: width_inc, height_inc    (INT32×2)
//	offset 44: min_aspect (num, den)    (INT32×2)
//	offset 52: max_aspect (num, den)    (INT32×2)
//	offset 60: base_width, base_height  (INT32×2)
//	offset 68: win_gravity              (INT32)
//
// Единицы — те же физические пиксели клиентской области, что принимает
// SetMinSize (см. комментарий в native.go): у X11 нет отдельного понятия
// "логических" координат поверхности, WM_NORMAL_HINTS всегда в пикселях
// окна, поэтому пересчёта по масштабу здесь не требуется (в отличие от
// Wayland, см. wlMinSizeArgs).
//
// width или height ≤ 0 — «минимума нет» (тот же контракт, что у
// Win32.SetMinSize(0,0) и у обеих осей независимо, как в Win32Window, где
// minW/minH проверяются раздельно): бит PMinSize(0x10) выставляется, только
// если задана хотя бы одна ось, а нулевая ось пишется как 0 — большинство
// WM трактуют 0 в качестве отсутствия ограничения по этой оси (то же самое
// соглашение принято ниже и в xdg_toplevel.set_min_size на Wayland).
func x11NormalHintsMinSize(width, height int) []byte {
	const sizeHintsLen = 18 * 4 // 18 CARD32-полей, см. раскладку выше
	buf := make([]byte, sizeHintsLen)

	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if width == 0 && height == 0 {
		return buf // flags=0 — свойство без PMinSize, минимум не ограничен
	}

	const pMinSize = 0x10 // ICCCM/Xutil.h: PMinSize = 1<<4
	binary.LittleEndian.PutUint32(buf[0:4], pMinSize)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(width))
	binary.LittleEndian.PutUint32(buf[24:28], uint32(height))
	return buf
}

// ─── Wayland: xdg_toplevel.set_min_size ─────────────────────────────────────

// wlMinSizeArgs переводит SetMinSize(width, height) — контракт NativeWindow
// требует ФИЗИЧЕСКИЕ пиксели клиентской области (см. native.go) — в аргументы
// запроса xdg_toplevel.set_min_size (опкод 8 в этом файле, см. константу
// xdgToplevelSetMinSize в native_wayland.go — она же используется в Create
// при фиксации начального размера, так что нумерация гарантированно совпадает
// с тем, что уже отправляет остальной код этого бэкенда).
//
// По спецификации xdg-shell аргументы set_min_size — surface-local (то есть
// ЛОГИЧЕСКИЕ, уже поделённые на буферный масштаб) координаты, а не физические
// пиксели: на HiDPI (buffer_scale>1) это другие единицы, и отправка
// физических пикселей "как есть" завысила бы минимум окна в scale раз.
//
// Этот Wayland-бэкенд, однако, НЕ отслеживает буферный масштаб — ни
// wl_surface.set_buffer_scale, ни wp_fractional_scale здесь не реализованы
// (см. шапку native_wayland.go: весь путь блита и, в частности, уже
// существующий вызов set_min_size/set_max_size в Create отправляют размер
// без пересчёта). Поэтому параметр scale передаётся с расчётом на будущее:
// когда/если бэкенд научится определять реальный масштаб поверхности, вызывающая
// сторона должна будет передать его сюда вместо 1 — пересчёт уже готов и
// протестирован. Пока же вызывающий код (SetMinSize ниже) передаёт scale=1,
// что даёт physical == logical, как и для остальных вызовов этого бэкенда.
//
// width или height ≤ 0 по каждой оси независимо превращается в 0 — это прямое
// значение по спецификации xdg_toplevel.set_min_size ("a value of zero means
// that the client has no size limit in that dimension"), то есть ровно та же
// семантика "минимума нет", что и на Windows при SetMinSize(0,0).
func wlMinSizeArgs(width, height int, scale float64) (int32, int32) {
	return wlMinSizeDim(width, scale), wlMinSizeDim(height, scale)
}

// wlMinSizeDim переводит один физический размер в surface-local координату
// с округлением к ближайшему целому (симметрично физический→логический
// пересчёт минимума на Win32 делает вызывающая сторона: win.scale домножает
// логическое на физическое в window.go, здесь — обратная операция).
func wlMinSizeDim(px int, scale float64) int32 {
	if px <= 0 {
		return 0
	}
	if scale <= 0 {
		scale = 1 // нет данных о масштабе — считаем как есть (см. wlMinSizeArgs)
	}
	v := int32(float64(px)/scale + 0.5)
	if v < 0 { // защита от переполнения при экзотическом scale — семантика "нет минимума"
		return 0
	}
	return v
}
