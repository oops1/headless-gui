//go:build windows

// shadowwnd_windows.go — тень окна, которую рисует движок, когда её не даёт
// DWM, и выбор источника тени (SetShadowSource).
//
// Тень окну Windows даёт DWM: на Windows 11 — вместе со скруглением, на
// Windows 10 — от рамки DWM в 1 px (applyCorners). Но не всегда: тени окон
// выключают в «Параметрах быстродействия» и в сеансах удалённого рабочего
// стола с облегчённым оформлением (SPI_GETDROPSHADOW = FALSE), а без
// композиции DWM теней нет вовсе. Тогда окно сливалось с тем, что позади.
//
// Своя тень — отдельное окно ОС, которое стоит сразу ПОД окном и следует за
// ним: слоистое (UpdateLayeredWindow с альфой точек), прозрачное для мыши
// (WS_EX_TRANSPARENT), не активируется и не видно на панели задач и в
// Alt+Tab. Само окно и его вывод не меняются. Свёрнутое, развёрнутое, на весь
// экран, скрытое и окно без рамки (Style.Borderless) тени не имеют.
//
// Картинка тени — по токенам темы (SetEngineShadow), растр core/raster. Окно
// большое, а тянут его за край на каждый пиксель, поэтому тень строится из
// образца: размывается квадрат чуть больше двух радиусов размытия, а
// картинка окна собирается из его углов и середин краёв (вдоль края тень
// одинакова). Середина картинки пуста — её закрывает само окно.

package win32

import (
	"image"
	"image/color"
	"math"
	"sync"
	"unsafe"

	"github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/platform/native"
	"golang.org/x/sys/windows"
)

// Источник тени — значения window/core.ShadowSource.
const (
	shadowAuto   = 0 // DWM, где он её даёт; иначе своя
	shadowSystem = 1 // только DWM
	shadowEngine = 2 // всегда своя: тень DWM снимается
)

const (
	wsExTransparent      = 0x00000020 // WS_EX_TRANSPARENT: мышь проходит насквозь
	spiGetDropShadow     = 0x1024     // SPI_GETDROPSHADOW
	spiSetDropShadow     = 0x1025     // SPI_SETDROPSHADOW (wParam WM_SETTINGCHANGE)
	wmDwmCompositionChgd = 0x031E     // WM_DWMCOMPOSITIONCHANGED
	swpShowwindow        = 0x0040
	swpHidewindow        = 0x0080
)

var (
	procSystemParametersInfoW   = user32.NewProc("SystemParametersInfoW")
	procDwmIsCompositionEnabled = dwmapi.NewProc("DwmIsCompositionEnabled")
)

// dwmShadowsOn — даёт ли DWM тени окнам: композиция включена и тени окон не
// выключены в системе. Переменная — тесты проходят путь «теней DWM нет».
var dwmShadowsOn = func() bool {
	var comp int32
	if r, _, _ := procDwmIsCompositionEnabled.Call(uintptr(unsafe.Pointer(&comp))); r != 0 || comp == 0 {
		return false
	}
	var on int32
	if r, _, _ := procSystemParametersInfoW.Call(spiGetDropShadow, 0, uintptr(unsafe.Pointer(&on)), 0); r == 0 {
		return true // не узнали — считаем, что тени есть (прежнее поведение)
	}
	return on != 0
}

// engineShadowSpec — вид своей тени окна в физических точках.
type engineShadowSpec struct {
	blur, offX, offY float64
	col              color.RGBA
}

// defaultEngineShadow — пока SetEngineShadow не звали (бэкенд без ядра окна).
var defaultEngineShadow = engineShadowSpec{blur: 16, offY: 6, col: color.RGBA{A: 80}}

// SetShadowSource реализует выбор источника тени окна (window/core): src —
// ShadowSource. Можно звать до Create.
func (w *Win32Window) SetShadowSource(src int) {
	w.shadowSrc.Store(int32(src))
	if w.hwnd != 0 {
		w.postUICall(w.applyCorners)
	}
}

// SetEngineShadow задаёт вид своей тени окна: размытие и смещение в
// физических точках, цвет premultiplied. Можно звать до Create.
func (w *Win32Window) SetEngineShadow(blur, offsetX, offsetY float64, col color.RGBA) {
	w.shadowMu.Lock()
	w.shadowSpec, w.shadowSpecSet = engineShadowSpec{blur, offsetX, offsetY, col}, true
	w.shadowMu.Unlock()
	if w.hwnd != 0 {
		w.postUICall(w.updateShadowWnd)
	}
}

// engineShadowOnly — тень окну рисует движок, DWM её не даёт (ShadowEngine).
func (w *Win32Window) engineShadowOnly() bool {
	return w.shadowSrc.Load() == shadowEngine && !w.ulw.on.Load() && w.popupSysCorner == 0
}

// wantsOwnShadow — окну нужна своя тень сейчас: источник её допускает, DWM
// тени не даёт (или выбран движок), а окно видимо в обычном состоянии.
func (w *Win32Window) wantsOwnShadow() bool {
	if w.hwnd == 0 || w.isPopup || w.style.borderless.Load() || w.maximized || w.fs.on.Load() {
		return false
	}
	switch w.shadowSrc.Load() {
	case shadowEngine:
	case shadowSystem:
		return false
	default:
		if dwmShadowsOn() {
			return false
		}
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v == 0 {
		return false
	}
	if ic, _, _ := procIsIconic.Call(uintptr(w.hwnd)); ic != 0 {
		return false
	}
	if z, _, _ := procIsZoomed.Call(uintptr(w.hwnd)); z != 0 {
		return false
	}
	return true
}

// shadowWindow — окно-тень и его картинка. Только поток окна.
type shadowWindow struct {
	hwnd    uintptr
	dc, bmp uintptr
	oldBmp  uintptr
	bits    unsafe.Pointer
	bw, bh  int
	// drawn — для какого размера окна и вида тени картинка уже собрана.
	drawnSize image.Point
	drawnSpec engineShadowSpec
	drawnR    int
	topmost   bool
	visible   bool
}

var (
	shadowClassOnce sync.Once
	shadowClassName *uint16
)

// registerShadowClass — класс окна-тени: своя оконная процедура не нужна,
// сообщения уходят в DefWindowProc.
func registerShadowClass() *uint16 {
	shadowClassOnce.Do(func() {
		shadowClassName, _ = windows.UTF16PtrFromString("HeadlessGUI_Shadow")
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			LpfnWndProc:   procDefWindowProcW.Addr(),
			LpszClassName: shadowClassName,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	return shadowClassName
}

// shadowMargin — на сколько физических точек тень выходит за окно.
func shadowMargin(sp engineShadowSpec) int {
	off := math.Max(math.Abs(sp.offX), math.Abs(sp.offY))
	return int(math.Ceil(sp.blur*2+off)) + 1
}

// updateShadowWnd приводит окно-тень к окну: показывает, двигает, ставит
// сразу под окном, перерисовывает при смене размера; прячет, когда тень не
// нужна. Только поток окна.
func (w *Win32Window) updateShadowWnd() {
	if !w.wantsOwnShadow() {
		w.hideShadowWnd()
		return
	}
	w.shadowMu.Lock()
	sp := defaultEngineShadow
	if w.shadowSpecSet {
		sp = w.shadowSpec
	}
	w.shadowMu.Unlock()
	if sp.blur <= 0 || sp.col.A == 0 {
		w.hideShadowWnd()
		return
	}
	s := w.shadow
	if s == nil {
		s = &shadowWindow{}
		w.shadow = s
	}
	if s.hwnd == 0 {
		h, _, _ := procCreateWindowExW.Call(
			uintptr(wsExLayered|wsExTransparent|wsExToolwindow|wsExNoactivate),
			uintptr(unsafe.Pointer(registerShadowClass())), 0, uintptr(wsPopup),
			0, 0, 1, 1, 0, 0, 0, 0)
		if h == 0 {
			return
		}
		s.hwnd = h
	}
	wr := w.windowRect()
	if wr.Empty() {
		w.hideShadowWnd()
		return
	}
	m := shadowMargin(sp)
	radius := 0
	if w.RoundsCorners() {
		radius = w.cornerRadius
	}
	size := wr.Size()
	if s.bits == nil || s.drawnSize != size || s.drawnSpec != sp || s.drawnR != radius {
		if !s.draw(size, m, radius, sp) {
			return
		}
	}
	pt := winPoint{int32(wr.Min.X - m), int32(wr.Min.Y - m)}
	sz := winSize{int32(s.bw), int32(s.bh)}
	src := winPoint{}
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	procUpdateLayeredWindow.Call(s.hwnd, 0, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)),
		s.dc, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)

	// Поверх всех — вместе с окном: иначе тень окна «поверх всех» осталась
	// бы под чужими окнами.
	top := w.IsAlwaysOnTop()
	if top != s.topmost {
		after := hwndNotopmost
		if top {
			after = hwndTopmost
		}
		procSetWindowPos.Call(s.hwnd, after, 0, 0, 0, 0, swpNomove|swpNosize|swpNoactivate)
		s.topmost = top
	}
	// Сразу под окном. SWP_NOOWNERZORDER: окна, принадлежащие окну-тени,
	// (их нет) и владельца не трогаем.
	procSetWindowPos.Call(s.hwnd, uintptr(w.hwnd), 0, 0, 0, 0,
		swpNomove|swpNosize|swpNoactivate|swpNoownerzorder|swpShowwindow)
	s.visible = true
}

// hideShadowWnd прячет окно-тень (если есть). Только поток окна.
func (w *Win32Window) hideShadowWnd() {
	if s := w.shadow; s != nil && s.hwnd != 0 && s.visible {
		procShowWindow.Call(s.hwnd, 0) // SW_HIDE
		s.visible = false
	}
}

// destroyShadowWnd уничтожает окно-тень и его картинку. Только поток окна.
func (w *Win32Window) destroyShadowWnd() {
	s := w.shadow
	if s == nil {
		return
	}
	s.release()
	if s.hwnd != 0 {
		procDestroyWindow.Call(s.hwnd)
	}
	w.shadow = nil
}

// ownShadowShown — окно-тень показано (для тестов и проверок).
func (w *Win32Window) ownShadowShown() (image.Rectangle, bool) {
	s := w.shadow
	if s == nil || s.hwnd == 0 || !s.visible {
		return image.Rectangle{}, false
	}
	var r rect
	procGetWindowRect.Call(s.hwnd, uintptr(unsafe.Pointer(&r)))
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom)), true
}

// release освобождает картинку окна-тени.
func (s *shadowWindow) release() {
	if s.dc != 0 {
		if s.oldBmp != 0 {
			procSelectObject.Call(s.dc, s.oldBmp)
		}
		procDeleteDC.Call(s.dc)
	}
	if s.bmp != 0 {
		procDeleteObject.Call(s.bmp)
	}
	s.dc, s.bmp, s.oldBmp, s.bits, s.bw, s.bh = 0, 0, 0, nil, 0, 0
}

// draw собирает картинку тени окна размера size с запасом m по краям.
func (s *shadowWindow) draw(size image.Point, m, radius int, sp engineShadowSpec) bool {
	img := shadowImage(size, m, radius, sp)
	bw, bh := img.Rect.Dx(), img.Rect.Dy()
	if s.bits == nil || s.bw != bw || s.bh != bh {
		s.release()
		dc, _, _ := procCreateCompatibleDC.Call(0)
		if dc == 0 {
			return false
		}
		bi := bitmapInfo{BmiHeader: bitmapInfoHeader{
			BiSize:  uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth: int32(bw), BiHeight: -int32(bh), BiPlanes: 1, BiBitCount: 32,
			BiCompression: biRgb,
		}}
		var bits unsafe.Pointer
		bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)),
			uintptr(dibRgbColors), uintptr(unsafe.Pointer(&bits)), 0, 0)
		if bmp == 0 || bits == nil {
			procDeleteDC.Call(dc)
			return false
		}
		old, _, _ := procSelectObject.Call(dc, bmp)
		s.dc, s.bmp, s.oldBmp, s.bits, s.bw, s.bh = dc, bmp, old, bits, bw, bh
	}
	dst := unsafe.Slice((*byte)(s.bits), bw*bh*4)
	row := bw * 4
	for y := 0; y < bh; y++ {
		native.SwapRBRow(dst[y*row:(y+1)*row], img.Pix[y*img.Stride:][:row])
	}
	s.drawnSize, s.drawnSpec, s.drawnR = size, sp, radius
	return true
}

// shadowImage — картинка тени окна размера size (physical) с запасом m:
// размер (size + 2m), окно — в (m, m). Большое окно собирается из образца
// (shadowSample), маленькое размывается целиком.
func shadowImage(size image.Point, m, radius int, sp engineShadowSpec) *image.RGBA {
	W, H := size.X+2*m, size.Y+2*m
	// Образец: тело со стороной, при которой середина края уже не чувствует
	// углов, — дальше вдоль края тень одинакова.
	t := 2*(m+radius) + 4
	if size.X <= t && size.Y <= t {
		return shadowSample(size, m, radius, sp)
	}
	tw, th := min(size.X, t), min(size.Y, t)
	smp := shadowSample(image.Pt(tw, th), m, radius, sp)
	SW, SH := tw+2*m, th+2*m
	out := image.NewRGBA(image.Rect(0, 0, W, H))
	// Точка картинки → точка образца: левая (верхняя) половина — как есть,
	// правая (нижняя) — от своего края, середина — середина образца.
	mapX := func(x int) int {
		switch {
		case x < SW/2:
			return x
		case x >= W-(SW-SW/2):
			return x - (W - SW)
		default:
			return SW / 2
		}
	}
	mapY := func(y int) int {
		switch {
		case y < SH/2:
			return y
		case y >= H-(SH-SH/2):
			return y - (H - SH)
		default:
			return SH / 2
		}
	}
	// Под окном (кроме полос у края шириной band) картинка пуста: её
	// закрывает само окно, а пустая середина — это дешёвый ресайз.
	band := m + radius + 2
	for y := 0; y < H; y++ {
		sy := mapY(y)
		inY := y >= band && y < H-band
		srow := smp.Pix[sy*smp.Stride:]
		drow := out.Pix[y*out.Stride:]
		for x := 0; x < W; x++ {
			if inY && x >= band && x < W-band {
				x = W - band - 1
				continue
			}
			sx := mapX(x)
			copy(drow[x*4:x*4+4], srow[sx*4:sx*4+4])
		}
	}
	return out
}

// shadowSample размывает тень тела size (physical) в картинку с запасом m.
func shadowSample(size image.Point, m, radius int, sp engineShadowSpec) *image.RGBA {
	c := raster.NewCanvas(size.X+2*m, size.Y+2*m, nil)
	c.DrawShadow(image.Rect(m, m, m+size.X, m+size.Y), radius, sp.blur, sp.offX, sp.offY, sp.col)
	return c.Back()
}
