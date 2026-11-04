//go:build windows

// popupulw_windows.go — окно-попап с альфой точек (UpdateLayeredWindow).
//
// Раньше окно-попап было слоистым только ради прозрачности целиком
// (SetLayeredWindowAttributes), а кадр клал StretchDIBits. Альфу точек такое
// окно не смешивает: всё прозрачное в кадре — точки за скруглёнными углами
// меню, его мягкая тень — выходило ЧЁРНЫМ. Поэтому тень в окно не попадала
// вовсе (кадр резался по телу меню), а у скругления торчали чёрные уголки.
//
// Теперь кадр кладёт UpdateLayeredWindow с ULW_ALPHA: окно ОС смешивает
// каждую точку с тем, что под ним, — тень и сглаженные углы рисует движок,
// как в холсте, и они выглядят так же. Прозрачность окна целиком на
// переходах — SourceConstantAlpha того же вызова. Точки с альфой 0 мышь
// пропускают насквозь — прозрачный запас вокруг меню кликов не ловит.
//
// Окно, выведенное UpdateLayeredWindow, не получает WM_PAINT, а
// SetLayeredWindowAttributes для него запрещён: оба пути взаимоисключающие.
//
// Поток. Кадр, положение и прозрачность только запоминаются, а
// UpdateLayeredWindow зовёт поток окна (сообщение wmApp): вызов, меняющий
// размер или место окна, шлёт ему WM_WINDOWPOS*, и с чужого потока ждал бы
// его — а тот может ждать замка, который держит вызывающий. Заодно положение
// и картинка ложатся ОДНИМ вызовом: окно не показывает новый кадр на старом
// месте (или старый на новом), когда раскрытое подменю растит попап.
//
// Если UpdateLayeredWindow отказал (нет памяти под DIB и т. п.), окно
// возвращается к прежнему пути: StretchDIBits и выкройка по сплошным точкам —
// без тени, зато без черноты.

package win32

import (
	"image"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/platform/native"
)

var (
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
)

const (
	ulwAlpha   = 0x2 // ULW_ALPHA
	acSrcOver  = 0x0 // AC_SRC_OVER
	acSrcAlpha = 0x1 // AC_SRC_ALPHA: точки premultiplied, со своей альфой
)

// blendFunction — BLENDFUNCTION.
type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

// winPoint и winSize — POINT и SIZE.
type winPoint struct{ X, Y int32 }
type winSize struct{ CX, CY int32 }

// popupULW — состояние вывода окна-попапа через UpdateLayeredWindow.
type popupULW struct {
	// on — окно выводится UpdateLayeredWindow (ставит CreatePopup до
	// создания окна). failed — вызов отказал, окно на прежнем пути. Оба
	// читаются с любого потока.
	on     atomic.Bool
	failed atomic.Bool

	// Заказанное состояние — под mu: пишут хост (горутина движка или поток
	// окна), читает поток окна.
	mu       sync.Mutex
	img      *image.RGBA // последний кадр (premultiplied RGBA); хост его не переписывает
	imgDirty bool        // кадр ещё не переложен в DIB
	x, y     int         // экранное положение окна
	posSet   bool
	alpha    byte // прозрачность окна целиком (SourceConstantAlpha)
	shown    bool // UpdateLayeredWindow хоть раз положил кадр

	// pending — задание вывода уже стоит в очереди окна.
	pending atomic.Bool

	// GDI — только поток окна: DIB-секция под кадр и DC, в котором она
	// выбрана. Пересоздаются при смене размера кадра.
	dc, bmp, oldBmp uintptr
	bits            unsafe.Pointer
	bw, bh          int
}

// ulwActive — окно выводится UpdateLayeredWindow.
func (w *Win32Window) ulwActive() bool {
	return w.ulw.on.Load() && !w.ulw.failed.Load()
}

// ulwSetImage запоминает кадр и ставит вывод.
func (w *Win32Window) ulwSetImage(img *image.RGBA) {
	w.ulw.mu.Lock()
	w.ulw.img, w.ulw.imgDirty = img, true
	w.ulw.mu.Unlock()
	w.ulwSchedule()
}

// ulwSetPosition запоминает экранное положение окна и ставит вывод.
func (w *Win32Window) ulwSetPosition(x, y int) {
	w.ulw.mu.Lock()
	w.ulw.x, w.ulw.y, w.ulw.posSet = x, y, true
	w.ulw.mu.Unlock()
	w.ulwSchedule()
}

// ulwPosition — заказанное положение окна (ok=false — не заказано).
func (w *Win32Window) ulwPosition() (x, y int, ok bool) {
	w.ulw.mu.Lock()
	defer w.ulw.mu.Unlock()
	return w.ulw.x, w.ulw.y, w.ulw.posSet
}

// ulwSetAlpha запоминает прозрачность окна целиком и ставит вывод.
func (w *Win32Window) ulwSetAlpha(a byte) {
	w.ulw.mu.Lock()
	changed := w.ulw.alpha != a
	w.ulw.alpha = a
	w.ulw.mu.Unlock()
	if changed {
		w.ulwSchedule()
	}
}

// ulwSchedule ставит одно задание вывода в очередь окна: сколько бы раз
// состояние ни сменилось до его выполнения, вызов будет один и положит
// последнее.
func (w *Win32Window) ulwSchedule() {
	if w.hwnd == 0 || !w.ulw.pending.CompareAndSwap(false, true) {
		return
	}
	if !w.postUICall(w.ulwFlush) {
		w.ulw.pending.Store(false) // окна уже нет
	}
}

// ulwFlush кладёт заказанное состояние в окно. Только поток окна.
func (w *Win32Window) ulwFlush() {
	// Снимаем метку ДО чтения состояния: всё, что заказано после этой
	// точки, поставит новое задание, и оно не потеряется.
	w.ulw.pending.Store(false)
	if w.hwnd == 0 || !w.ulwActive() {
		return
	}
	w.ulw.mu.Lock()
	img, dirty := w.ulw.img, w.ulw.imgDirty
	w.ulw.imgDirty = false
	x, y, posSet := w.ulw.x, w.ulw.y, w.ulw.posSet
	alpha := w.ulw.alpha
	w.ulw.mu.Unlock()
	if img == nil || img.Rect.Empty() {
		return // кадра ещё нет: окно невидимо, положение ляжет с ним
	}
	b := img.Rect
	if dirty || w.ulw.bits == nil {
		if !w.ulwCopy(img) {
			w.ulwFallback()
			return
		}
	}
	size := winSize{int32(b.Dx()), int32(b.Dy())}
	src := winPoint{}
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: alpha, AlphaFormat: acSrcAlpha}
	var dst uintptr
	pt := winPoint{int32(x), int32(y)}
	if posSet {
		dst = uintptr(unsafe.Pointer(&pt))
	}
	ret, _, _ := procUpdateLayeredWindow.Call(uintptr(w.hwnd), 0, dst,
		uintptr(unsafe.Pointer(&size)), w.ulw.dc, uintptr(unsafe.Pointer(&src)),
		0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if ret == 0 {
		w.ulwFallback()
		return
	}
	w.width, w.height = b.Dx(), b.Dy()
	w.ulw.mu.Lock()
	w.ulw.shown = true
	w.ulw.mu.Unlock()
}

// ulwCopy перекладывает кадр в DIB-секцию (RGBA → BGRA, альфа уже
// premultiplied — как ждёт AC_SRC_ALPHA). Секция заводится под размер кадра.
// Только поток окна.
func (w *Win32Window) ulwCopy(img *image.RGBA) bool {
	b := img.Rect
	bw, bh := b.Dx(), b.Dy()
	if w.ulw.bits == nil || w.ulw.bw != bw || w.ulw.bh != bh {
		w.ulwRelease()
		dc, _, _ := procCreateCompatibleDC.Call(0)
		if dc == 0 {
			return false
		}
		bi := bitmapInfo{
			BmiHeader: bitmapInfoHeader{
				BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
				BiWidth:       int32(bw),
				BiHeight:      -int32(bh), // top-down: строка 0 — верхняя
				BiPlanes:      1,
				BiBitCount:    32,
				BiCompression: biRgb,
			},
		}
		var bits unsafe.Pointer
		bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)),
			uintptr(dibRgbColors), uintptr(unsafe.Pointer(&bits)), 0, 0)
		if bmp == 0 || bits == nil {
			procDeleteDC.Call(dc)
			return false
		}
		old, _, _ := procSelectObject.Call(dc, bmp)
		w.ulw.dc, w.ulw.bmp, w.ulw.oldBmp, w.ulw.bits = dc, bmp, old, bits
		w.ulw.bw, w.ulw.bh = bw, bh
	}
	dst := unsafe.Slice((*byte)(w.ulw.bits), bw*bh*4)
	row := bw * 4
	for y := 0; y < bh; y++ {
		s := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):][:row]
		native.SwapRBRow(dst[y*row:(y+1)*row], s)
	}
	return true
}

// ulwRelease освобождает DIB-секцию и DC. Только поток окна.
func (w *Win32Window) ulwRelease() {
	if w.ulw.dc != 0 {
		if w.ulw.oldBmp != 0 {
			procSelectObject.Call(w.ulw.dc, w.ulw.oldBmp)
		}
		procDeleteDC.Call(w.ulw.dc)
	}
	if w.ulw.bmp != 0 {
		procDeleteObject.Call(w.ulw.bmp)
	}
	w.ulw.dc, w.ulw.bmp, w.ulw.oldBmp, w.ulw.bits = 0, 0, 0, nil
	w.ulw.bw, w.ulw.bh = 0, 0
}

// ulwFallback переводит окно на прежний путь: прозрачность целиком,
// StretchDIBits и выкройка по сплошным точкам. Только поток окна.
func (w *Win32Window) ulwFallback() {
	if w.ulw.failed.Swap(true) {
		return
	}
	w.ulwRelease()
	w.ulw.mu.Lock()
	img := w.ulw.img
	x, y, posSet := w.ulw.x, w.ulw.y, w.ulw.posSet
	w.ulw.mu.Unlock()
	w.applyLayeredAlpha()
	if img == nil {
		return
	}
	b := img.Rect
	flags := uintptr(0x0004 | 0x0010) // SWP_NOZORDER | SWP_NOACTIVATE
	if !posSet {
		flags |= 0x0002 // SWP_NOMOVE
	}
	procSetWindowPos.Call(uintptr(w.hwnd), 0, uintptr(x), uintptr(y),
		uintptr(b.Dx()), uintptr(b.Dy()), flags)
	w.blitSolidRegion(img)
}

// blitSolidRegion — прежний путь вывода попапа: окно выкраивается по
// сплошным точкам кадра (тень и сглаженный край отрезаются — смешать их
// некому, и они вышли бы чёрными) и кадр кладётся StretchDIBits.
func (w *Win32Window) blitSolidRegion(img *image.RGBA) {
	bands := raster.SolidBands(img)
	if o := img.Rect.Min; o != (image.Point{}) {
		for i := range bands {
			bands[i] = bands[i].Sub(o)
		}
	}
	w.applyOpaqueRegion(bands, img.Rect.Sub(img.Rect.Min))
	w.BlitRGBADirty(img, img.Bounds())
}

// popupAlphaByte — прозрачность окна-попапа целиком, как она заказана
// (0…255); ok=false — окно не попап с UpdateLayeredWindow. Для тестов.
func (w *Win32Window) popupAlphaByte() (byte, bool) {
	if !w.ulwActive() {
		return 0, false
	}
	w.ulw.mu.Lock()
	defer w.ulw.mu.Unlock()
	return w.ulw.alpha, w.ulw.shown
}
