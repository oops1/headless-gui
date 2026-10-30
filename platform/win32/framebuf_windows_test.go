package win32

import (
	"image"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Буфер кадра для GDI лежит в одной резервации VirtualAlloc. Срез Go больше
// 4 МБ на windows/amd64 почти всегда пересекал границу двух арен кучи, и на
// Windows 10 StretchDIBits не копировал из него ни строки — окно оставалось
// пустым (отчёт RDP-клиента на v3.38.0). Сам сбой воспроизводится только на
// Windows 10; здесь проверяется его причина — где лежит буфер.
func reservationOf(t *testing.T, p uintptr) uintptr {
	t.Helper()
	var mbi windows.MemoryBasicInformation
	if err := windows.VirtualQuery(p, &mbi, unsafe.Sizeof(mbi)); err != nil {
		t.Fatalf("VirtualQuery(%#x): %v", p, err)
	}
	return mbi.AllocationBase
}

func TestWin32FrameBuf_OneReservation(t *testing.T) {
	w := &Win32Window{}
	n := 1451 * 920 * 4 // размер из отчёта: 5 339 680 байт
	w.mu.Lock()
	w.allocFrameBuf(n)
	buf := w.frameBuf
	va := w.frameVA
	w.mu.Unlock()
	if !va || len(buf) != n {
		t.Fatalf("буфер не из VirtualAlloc: va=%v len=%d", va, len(buf))
	}
	first := uintptr(unsafe.Pointer(&buf[0]))
	last := uintptr(unsafe.Pointer(&buf[n-1]))
	if a, b := reservationOf(t, first), reservationOf(t, last); a != b {
		t.Fatalf("буфер в двух резервациях: %#x и %#x", a, b)
	}
	w.mu.Lock()
	w.freeFrameBuf()
	gone := w.frameBuf == nil && !w.frameVA
	w.mu.Unlock()
	if !gone {
		t.Fatal("freeFrameBuf не освободил буфер")
	}
}

func newBlitWindow(t *testing.T, wd, ht int) *Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	w := &Win32Window{}
	w.noQuit = true
	if err := w.Create("blit-test", wd, ht); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	t.Cleanup(func() {
		if w.hwnd != 0 {
			procDestroyWindow.Call(uintptr(w.hwnd))
		}
	})
	return w
}

// Большой кадр доходит до окна: StretchDIBits возвращает строки, а не 0.
// На Windows 11 прошло бы и со срезом Go; на Windows 10 это и есть
// регрессионная проверка.
func TestWin32Blit_LargeFrameReachesWindow(t *testing.T) {
	w := newBlitWindow(t, 1500, 950)
	img := image.NewRGBA(image.Rect(0, 0, 1500, 950))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	w.BlitRGBA(img)
	if n := w.blitFails.Load(); n != 0 {
		t.Fatalf("GDI не вывел кадр %d раз подряд", n)
	}
	w.mu.Lock()
	va := w.frameVA
	w.mu.Unlock()
	if !va {
		t.Fatal("буфер кадра окна — срез Go, а не VirtualAlloc")
	}
}

// После уничтожения окна буфер освобождён, и запоздавший кадр ничего не
// делает (а не пишет в освобождённую память).
func TestWin32Blit_AfterDestroyIsNoop(t *testing.T) {
	w := newBlitWindow(t, 400, 300)
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	w.BlitRGBA(img)
	procDestroyWindow.Call(uintptr(w.hwnd))
	w.mu.Lock()
	freed := w.frameBuf == nil && w.frameDead
	w.mu.Unlock()
	if !freed {
		t.Fatal("WM_DESTROY не освободил буфер кадра")
	}
	w.BlitRGBA(img) // не должен паниковать и заводить буфер заново
	w.mu.Lock()
	again := w.frameBuf != nil
	w.mu.Unlock()
	if again {
		t.Fatal("после уничтожения окна блит завёл буфер заново")
	}
	w.hwnd = 0
}

var procGetUpdateRectT = user32.NewProc("GetUpdateRect")

// Первый WM_PAINT приходит внутри Create, когда перерисовка ещё не
// подключена; BeginPaint его валидирует. Подключение перерисовки
// (SetOnExpose) снова просит у системы перерисовку — иначе окно, первый кадр
// которого не дошёл, так и осталось бы пустым.
func TestWin32Paint_EarlyPaintIsNotLost(t *testing.T) {
	w := newBlitWindow(t, 400, 300)
	if !w.paintMiss.Load() {
		t.Skip("WM_PAINT при создании не пришёл синхронно — проверять нечего")
	}
	w.SetOnExpose(func(image.Rectangle) {})
	if w.paintMiss.Load() {
		t.Fatal("SetOnExpose не снял отметку пропущенной перерисовки")
	}
	if r, _, _ := procGetUpdateRectT.Call(uintptr(w.hwnd), 0, 0); r == 0 {
		t.Fatal("после SetOnExpose окну не заказана перерисовка")
	}
}
