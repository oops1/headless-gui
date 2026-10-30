//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v4/platform/pointer"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Win32: курсор из картинки (Engine.SetCursorImage) строится настоящим
// CreateIconIndirect, лежит в кэше окна по ключу содержимого, вытесняется по
// LRU с освобождением системных объектов и действует над попапами.

var (
	procCIWGetIconInfo       = user32.NewProc("GetIconInfo")
	procCIWDrawIconEx        = user32.NewProc("DrawIconEx")
	procCIWCreateCompatDC    = gdi32.NewProc("CreateCompatibleDC")
	procCIWSelectObject      = gdi32.NewProc("SelectObject")
	procCIWDeleteDC          = gdi32.NewProc("DeleteDC")
	procCIWCreateDIBSection2 = gdi32.NewProc("CreateDIBSection")
)

// ciwIconInfo — ICONINFO.
type ciwIconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// ciwInfo возвращает ICONINFO курсора; ok=false — дескриптор недействителен.
func ciwInfo(h uintptr) (ii ciwIconInfo, ok bool) {
	r, _, _ := procCIWGetIconInfo.Call(h, uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return ii, false
	}
	procDeleteObject.Call(ii.HbmMask)
	procDeleteObject.Call(ii.HbmColor)
	return ii, true
}

func ciwImage(w, h int, c color.NRGBA) *base.CursorImage {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return base.NewCursorImage(img, 3, 5)
}

func TestWin32CursorImage_BuildsCursorWithHotspot(t *testing.T) {
	w := newFsWindow(t)
	ci := ciwImage(48, 40, color.NRGBA{R: 200, G: 30, B: 30, A: 255})
	w.SetCursorImage(ci)
	if w.cursorHandle == 0 || w.cursorHidden {
		t.Fatalf("курсор не выбран: handle=%#x hidden=%v", w.cursorHandle, w.cursorHidden)
	}
	ii, ok := ciwInfo(w.cursorHandle)
	if !ok {
		t.Fatal("GetIconInfo отверг дескриптор курсора")
	}
	if ii.FIcon != 0 {
		t.Error("построен значок, а не курсор")
	}
	if ii.XHotspot != 3 || ii.YHotspot != 5 {
		t.Errorf("горячая точка (%d,%d), ждал (3,5)", ii.XHotspot, ii.YHotspot)
	}
	if h, _ := w.setCursorMessage(htClient); h != w.cursorHandle {
		t.Errorf("WM_SETCURSOR отдал %#x, ждал курсор-картинку %#x", h, w.cursorHandle)
	}
}

// Та же картинка — тот же курсор, системный объект не создаётся заново; разные
// картинки — разные курсоры.
func TestWin32CursorImage_CachedByKey(t *testing.T) {
	w := newFsWindow(t)
	a := ciwImage(32, 32, color.NRGBA{R: 255, A: 255})
	a2 := ciwImage(32, 32, color.NRGBA{R: 255, A: 255}) // равное содержимое, другой объект
	b := ciwImage(32, 32, color.NRGBA{G: 255, A: 255})

	w.SetCursorImage(a)
	ha := w.cursorHandle
	w.SetCursorImage(a2)
	if w.imgCreated != 1 || w.cursorHandle != ha {
		t.Fatalf("равная картинка построена заново: создано %d, handle %#x → %#x", w.imgCreated, ha, w.cursorHandle)
	}
	w.SetCursorImage(b)
	if w.imgCreated != 2 || w.cursorHandle == ha {
		t.Fatalf("другая картинка не получила своего курсора: создано %d", w.imgCreated)
	}
	w.SetCursorImage(a)
	if w.imgCreated != 2 || w.cursorHandle != ha {
		t.Errorf("возврат к первой картинке пересобрал курсор: создано %d", w.imgCreated)
	}
}

// Кэш ограничен: сверх pointer.CursorCacheSize давние курсоры уничтожаются, а
// показываемый сейчас остаётся.
func TestWin32CursorImage_LRUEvictsAndDestroys(t *testing.T) {
	w := newFsWindow(t)
	var handles []uintptr
	for i := 0; i < pointer.CursorCacheSize+4; i++ {
		w.SetCursorImage(ciwImage(24, 24, color.NRGBA{R: uint8(i), G: uint8(i * 7), B: 1, A: 255}))
		handles = append(handles, w.cursorHandle)
	}
	if n := w.imgCursors.Len(); n != pointer.CursorCacheSize {
		t.Fatalf("в кэше %d курсоров, ждал %d", n, pointer.CursorCacheSize)
	}
	for i, h := range handles {
		_, alive := ciwInfo(h)
		wantAlive := i >= 4 // четыре самых давних вытеснены
		if alive != wantAlive {
			t.Errorf("курсор #%d: жив=%v, ждал %v", i, alive, wantAlive)
		}
	}
}

// Снятие картинки возвращает форму: SetCursor после SetCursorImage заменяет
// курсор, а кэш картинок остаётся.
func TestWin32CursorImage_ShapeReplacesImage(t *testing.T) {
	w := newFsWindow(t)
	w.SetCursorImage(ciwImage(32, 32, color.NRGBA{B: 255, A: 255}))
	img := w.cursorHandle
	w.SetCursor(pointer.CursorIBeam)
	if w.cursorHandle == img || w.cursorHandle == 0 {
		t.Fatalf("форма не заменила картинку: %#x", w.cursorHandle)
	}
	w.SetCursorImage(ciwImage(32, 32, color.NRGBA{B: 255, A: 255}))
	if w.cursorHandle != img || w.imgCreated != 1 {
		t.Errorf("картинка после формы не из кэша: handle %#x → %#x, создано %d", img, w.cursorHandle, w.imgCreated)
	}
}

// Окно разбирается — курсоры-картинки освобождены.
func TestWin32CursorImage_DestroyedWithWindow(t *testing.T) {
	w := newFsWindow(t)
	w.SetCursorImage(ciwImage(32, 32, color.NRGBA{R: 9, A: 255}))
	h := w.cursorHandle
	w.destroyImageCursors()
	if _, alive := ciwInfo(h); alive {
		t.Error("курсор жив после destroyImageCursors")
	}
	if w.imgCursors.Len() != 0 {
		t.Error("кэш не пуст")
	}
}

// Полупрозрачный пиксель смешивается с фоном по прямой альфе: красный 255 при
// альфе 128 на чёрном даёт ~128. Домноженные на альфу байты (128) дали бы ~64 —
// проверено на живой системе: DIB значка читается как прямая альфа. Рисуется
// настоящим DrawIconEx в память.
func TestWin32CursorImage_AlphaBlendsAsStraight(t *testing.T) {
	w := newFsWindow(t)
	ci := ciwImage(16, 16, color.NRGBA{R: 255, A: 128})
	w.SetCursorImage(ci)

	hdc, _, _ := procCIWCreateCompatDC.Call(0)
	if hdc == 0 {
		t.Skip("нет контекста устройства")
	}
	defer procCIWDeleteDC.Call(hdc)
	bi := bitmapInfo{BmiHeader: bitmapInfoHeader{
		BiSize: uint32(unsafe.Sizeof(bitmapInfoHeader{})), BiWidth: 16, BiHeight: -16,
		BiPlanes: 1, BiBitCount: 32, BiCompression: biRgb,
	}}
	var bits unsafe.Pointer
	hbm, _, _ := procCIWCreateDIBSection2.Call(hdc, uintptr(unsafe.Pointer(&bi)), uintptr(dibRgbColors),
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == nil {
		t.Skip("DIB не создан")
	}
	defer procDeleteObject.Call(hbm)
	old, _, _ := procCIWSelectObject.Call(hdc, hbm)
	defer procCIWSelectObject.Call(hdc, old)
	pix := unsafe.Slice((*byte)(bits), 16*16*4) // фон — чёрный (DIB обнулён)

	const diNormal = 3
	if r, _, _ := procCIWDrawIconEx.Call(hdc, 0, 0, w.cursorHandle, 16, 16, 0, 0, diNormal); r == 0 {
		t.Fatal("DrawIconEx не нарисовал курсор")
	}
	red := int(pix[(8*16+8)*4+2]) // BGRA: красный — третий байт
	if red < 118 || red > 138 {
		t.Errorf("красный на чёрном после смешивания = %d, ждал ~128 (прямая альфа)", red)
	}
}
