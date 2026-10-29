package native

// initial.go — начальное положение окна и подсказки размеров под ОС.

// InitialPlacement — то, что бэкенд обязан знать до Create, чтобы окно сразу
// появилось где надо. Размер бэкенд получает аргументами Create; здесь только
// то, чего у Create нет.
//
//facadegen:skip
type InitialPlacement struct {
	// PosSet — окно создаётся в точке (X, Y), экранные координаты. Без него
	// бэкенд выбирает место сам (по центру экрана).
	PosSet bool
	X, Y   int
	// SizeSet — размер в Create задан приложением, а не выбран движком: X11 в
	// этом случае объявляет его менеджеру (USSize).
	SizeSet bool
	// Maximized — показать развёрнутым.
	Maximized bool
}

// Константы Win32, нужные чистой арифметике ниже (сами константы бэкенда живут
// в platform/win32/native_windows.go под платформенным тегом).
const (
	initWsVisible = 0x10000000 // WS_VISIBLE
	initSwShowMax = 3          // SW_SHOWMAXIMIZED
)

// Win32InitialCreate решает, с каким стилем и командой показа создавать
// главное окно Win32, и где.
//
//   - Развёрнутое окно создаётся БЕЗ WS_VISIBLE: иначе CreateWindowEx показал
//     бы его обычным (на своём прямоугольнике), и разворачивание шло бы на
//     глазах. Первый показ — ShowWindow(SW_SHOWMAXIMIZED) на скрытом окне:
//     система берёт прямоугольник создания как нормальный, и «восстановить»
//     вернёт его.
//   - Положение: заданное приложением или (cx, cy) — по центру.
//
// show — команда показа, заданная вызывающим (SW_SHOW для обычного окна).
//
//facadegen:skip
func Win32InitialCreate(p InitialPlacement, style uint32, show uintptr, cx, cy int) (x, y int, st uint32, cmd uintptr) {
	x, y, st, cmd = cx, cy, style, show
	if p.PosSet {
		x, y = p.X, p.Y
	}
	if p.Maximized {
		st &^= initWsVisible
		cmd = initSwShowMax
	}
	return
}

// WlNormalSize решает, какой размер принять из xdg_toplevel.configure окну,
// начавшему жизнь развёрнутым. nw×nh — размер из события (0×0 — «решай
// сам»), normW×normH — размер, с которым окно создавалось (0 — окно не
// начинало развёрнутым или размер уже использован).
//
// Компоновщик не видел обычного окна, а потому на «восстановить» присылает
// 0×0, и окно осталось бы размером с экран. Когда окно не развёрнуто и не
// на весь экран, а компоновщик размера не назвал, берём запомненный. used —
// запомненное значение пошло в дело (или стало ненужным): очистить.
//
//facadegen:skip
func WlNormalSize(nw, nh int, maximized, fullscreen bool, normW, normH int) (w, h int, used bool) {
	if normW <= 0 || normH <= 0 || maximized || fullscreen {
		return nw, nh, false
	}
	if nw == 0 && nh == 0 {
		return normW, normH, true
	}
	return nw, nh, true // компоновщик назвал размер сам — запомненный не нужен
}

// Флаги ICCCM для WM_NORMAL_HINTS (XSizeHints.flags).
const (
	//facadegen:skip
	X11USPosition = 1 << 0 // положение назначил пользователь/приложение
	//facadegen:skip
	X11USSize = 1 << 1 // размер назначил пользователь/приложение
	//facadegen:skip
	X11PPosition = 1 << 2 // положение назначила программа
	//facadegen:skip
	X11PSize = 1 << 3 // размер назначила программа
	//facadegen:skip
	X11PMinSize = 1 << 4
)

// X11SizeHints собирает тело WM_NORMAL_HINTS (WM_SIZE_HINTS, 18×CARD32, раскладка
// — в X11NormalHintsMinSize) с минимальным размером и начальным положением.
//
// Положение и размер объявляются менеджеру флагами USPosition|PPosition и
// USSize|PSize: без них ICCCM разрешает менеджеру разместить окно по-своему
// (каскад, центр), и заданное в CreateWindow положение пропадает. В полях
// x, y, width, height (смещения 4..19, устаревшие, но менеджеры их читают)
// повторяются значения из CreateWindow. Чистая функция: раскладка проверяется
// тестом на любой платформе.
//
//facadegen:skip
func X11SizeHints(minW, minH int, p InitialPlacement, x, y, width, height int) []byte {
	buf := x11NormalHintsMinSizeBytes(minW, minH)
	flags := uint32(0)
	if minW > 0 || minH > 0 {
		flags |= X11PMinSize
	}
	if p.PosSet {
		flags |= X11USPosition | X11PPosition
		putLE32(buf[4:8], uint32(int32(x)))
		putLE32(buf[8:12], uint32(int32(y)))
	}
	if p.SizeSet {
		flags |= X11USSize | X11PSize
		putLE32(buf[12:16], uint32(int32(width)))
		putLE32(buf[16:20], uint32(int32(height)))
	}
	putLE32(buf[0:4], flags)
	return buf
}

// x11NormalHintsMinSizeBytes — тело свойства с одним минимальным размером
// (флаги и поля положения нулевые): основа X11SizeHints. Живёт здесь, а не в
// minsize_linux.go, чтобы чистая арифметика проверялась и вне Linux.
func x11NormalHintsMinSizeBytes(minW, minH int) []byte {
	buf := make([]byte, 18*4)
	if minW < 0 {
		minW = 0
	}
	if minH < 0 {
		minH = 0
	}
	if minW > 0 || minH > 0 {
		putLE32(buf[20:24], uint32(minW))
		putLE32(buf[24:28], uint32(minH))
	}
	return buf
}

func putLE32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}
