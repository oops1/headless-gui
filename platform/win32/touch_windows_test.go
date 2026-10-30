//go:build windows

package win32

import (
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// Раскладка структур указателей обязана совпадать с C: через неё система
// отдаёт координаты, и сдвиг поля дал бы мусорные точки без единой ошибки.
func TestPointerStructLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("размеры C проверяются для 64-разрядной сборки")
	}
	var pi pointerInfo
	if got := unsafe.Sizeof(pi); got != 96 {
		t.Errorf("sizeof(POINTER_INFO) = %d, ждал 96", got)
	}
	if got := unsafe.Offsetof(pi.PtPixelLocation); got != 32 {
		t.Errorf("offsetof(ptPixelLocation) = %d, ждал 32", got)
	}
	if got := unsafe.Offsetof(pi.DwTime); got != 64 {
		t.Errorf("offsetof(dwTime) = %d, ждал 64", got)
	}
	if got := unsafe.Offsetof(pi.PerformanceCount); got != 80 {
		t.Errorf("offsetof(PerformanceCount) = %d, ждал 80", got)
	}
	var ti pointerTouchInfo
	if got := unsafe.Sizeof(ti); got != 144 {
		t.Errorf("sizeof(POINTER_TOUCH_INFO) = %d, ждал 144", got)
	}
	if got := unsafe.Offsetof(ti.Pressure); got != 140 {
		t.Errorf("offsetof(pressure) = %d, ждал 140", got)
	}
}

func TestTouchPhaseOfPointer(t *testing.T) {
	cases := []struct {
		msg   uint32
		flags uint32
		want  base.TouchPhase
		ok    bool
	}{
		{wmPointerDown, 0, base.TouchBegan, true},
		{wmPointerUpdate, 0, base.TouchMoved, true},
		{wmPointerUp, 0, base.TouchEnded, true},
		{wmPointerUp, pointerFlagCanceled, base.TouchCancelled, true},
		{wmPointerCaptureChanged, 0, base.TouchCancelled, true},
		{wmMousemove, 0, 0, false},
	}
	for _, c := range cases {
		got, ok := touchPhaseOfPointer(c.msg, c.flags)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("msg %#x flags %#x: %v,%v, ждал %v,%v", c.msg, c.flags, got, ok, c.want, c.ok)
		}
	}
}

func TestTouchEventFromPointer(t *testing.T) {
	ev := touchEventFromPointer(7, base.TouchMoved, 1500, 30, 40)
	if ev.ID != 7 || ev.X != 30 || ev.Y != 40 || ev.Phase != base.TouchMoved || ev.Time != 1500*time.Millisecond {
		t.Errorf("%+v", ev)
	}
}

func TestPointerIDFromWParam(t *testing.T) {
	// HIWORD — флаги, LOWORD — номер.
	if id := pointerIDFromWParam(0x00060000 | 0x0123); id != 0x0123 {
		t.Errorf("id %#x", id)
	}
}

// Без включённых касаний окно не трогает WM_POINTER*: всё уходит системе.
func TestHandlePointerMsg_DisabledPassesThrough(t *testing.T) {
	w := &Win32Window{}
	if w.handlePointerMsg(0, wmPointerDown, 1) {
		t.Error("выключенные касания обработали сообщение")
	}
	w.touch.on.Store(true)
	// Включены, но приёмника нет — тоже системе.
	if w.handlePointerMsg(0, wmPointerDown, 1) {
		t.Error("касания без приёмника обработали сообщение")
	}
}

// ─── Живая проверка: InjectTouchInput в настоящее окно ──────────────────────

var (
	// procWindowFromPoint — общий, из mouse_windows.go.
	procInitializeTouchInjection = user32.NewProc("InitializeTouchInjection")
	procInjectTouchInput         = user32.NewProc("InjectTouchInput")
	procSetWindowPosTop          = user32.NewProc("SetWindowPos")
)

// injectTouch шлёт одно касание в экранной точке. phase: 0 — down, 1 — update,
// 2 — up.
func injectTouch(id uint32, x, y int, phase int) bool {
	var c pointerTouchInfo
	c.PointerInfo.PointerType = ptTouch
	c.PointerInfo.PointerID = id
	c.PointerInfo.PtPixelLocation = point{X: int32(x), Y: int32(y)}
	const (
		inRange   = 0x2
		inContact = 0x4
		down      = 0x10000
		update    = 0x20000
		up        = 0x40000
	)
	switch phase {
	case 0:
		c.PointerInfo.PointerFlags = down | inRange | inContact
	case 1:
		c.PointerInfo.PointerFlags = update | inRange | inContact
	default:
		c.PointerInfo.PointerFlags = up
	}
	c.TouchMask = 0x1 | 0x2 | 0x4 // contact area, orientation, pressure
	c.RcContact = rect{Left: int32(x - 2), Top: int32(y - 2), Right: int32(x + 2), Bottom: int32(y + 2)}
	c.Orientation = 90
	c.Pressure = 32
	r, _, _ := procInjectTouchInput.Call(1, uintptr(unsafe.Pointer(&c)))
	return r != 0
}

// TestWin32TouchLive проверяет оба режима на настоящем окне, подмешивая в
// систему синтетическое касание (InjectTouchInput):
//
//   - касания включены: приёмник получает Began, Moved…, Ended, а мыши из них нет;
//   - выключены: касаний нет, зато система сама порождает мышь — так же, как
//     получали её приложения до появления этого кода.
//
// Окно выскакивает на экран и касание «нажимает» на рабочем столе, поэтому тест
// включается переменной HEADLESS_GUI_TOUCH_E2E=1 (нужна интерактивная сессия).
func TestWin32TouchLive(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_TOUCH_E2E") != "1" {
		t.Skip("живой тест касаний: HEADLESS_GUI_TOUCH_E2E=1")
	}
	if procInjectTouchInput.Find() != nil {
		t.Skip("InjectTouchInput недоступен")
	}
	if r, _, _ := procInitializeTouchInjection.Call(2, 3 /*TOUCH_FEEDBACK_NONE*/); r == 0 {
		t.Skip("InitializeTouchInjection отказал (нет интерактивной сессии?)")
	}

	for _, mode := range []struct {
		name    string
		enabled bool
	}{{"включены", true}, {"выключены", false}} {
		t.Run(mode.name, func(t *testing.T) {
			var (
				mu      sync.Mutex
				touches []base.TouchEvent
				mouse   int
			)
			ready := make(chan *Win32Window, 1)
			done := make(chan struct{})
			go func() {
				runtime.LockOSThread()
				defer close(done)
				w := &Win32Window{}
				if err := w.Create("touch e2e", 300, 200); err != nil {
					t.Errorf("Create: %v", err)
					ready <- nil
					return
				}
				w.SetTouchEnabled(mode.enabled)
				w.SetOnTouch(func(ev base.TouchEvent) {
					mu.Lock()
					touches = append(touches, ev)
					mu.Unlock()
				})
				w.SetOnMouseMove(func(x, y int) { mu.Lock(); mouse++; mu.Unlock() })
				w.SetOnMouseButton(func(x, y, b int, p bool) { mu.Lock(); mouse++; mu.Unlock() })
				ready <- w
				w.RunEventLoop()
			}()
			w := <-ready
			if w == nil {
				return
			}
			defer func() {
				w.Close()
				<-done
			}()

			// На передний план: касание достанется тому, что под точкой.
			procSetWindowPosTop.Call(uintptr(w.hwnd), ^uintptr(0) /*HWND_TOPMOST*/, 0, 0, 0, 0, 0x0001|0x0002|0x0010)
			procSetForegroundWindow.Call(uintptr(w.hwnd))
			time.Sleep(500 * time.Millisecond)
			px, py := w.GetPosition()
			sw, sh := w.GetSize()
			cx, cy := px+sw/2, py+sh/2
			// Не тыкаем в чужие окна: касание уходит только если под точкой наше.
			if under, _, _ := procWindowFromPoint.Call(uintptr(uint32(int32(cx))) | uintptr(uint32(int32(cy)))<<32); under != uintptr(w.hwnd) {
				t.Skipf("под точкой (%d,%d) не наше окно (%#x), касание не посылаем", cx, cy, under)
			}
			if !injectTouch(0, cx, cy, 0) {
				t.Skip("InjectTouchInput отказал")
			}
			time.Sleep(60 * time.Millisecond)
			injectTouch(0, cx+15, cy+5, 1)
			time.Sleep(60 * time.Millisecond)
			injectTouch(0, cx+30, cy+10, 1)
			time.Sleep(60 * time.Millisecond)
			injectTouch(0, cx+30, cy+10, 2)
			time.Sleep(300 * time.Millisecond)

			mu.Lock()
			defer mu.Unlock()
			t.Logf("касаний: %d, событий мыши: %d", len(touches), mouse)
			if mode.enabled {
				if len(touches) < 3 {
					t.Fatalf("получено касаний %d: %+v", len(touches), touches)
				}
				if touches[0].Phase != base.TouchBegan || touches[len(touches)-1].Phase != base.TouchEnded {
					t.Errorf("фазы: %+v", touches)
				}
				if touches[0].X != sw/2 || touches[0].Y != sh/2 {
					// Клиентские координаты: центр окна.
					t.Logf("Began в %d,%d (центр %d,%d)", touches[0].X, touches[0].Y, sw/2, sh/2)
				}
				if mouse != 0 {
					t.Errorf("мышь из касаний не подавлена: %d событий", mouse)
				}
				// Нажим и контакт, посланные инжектором (pressure 32 из 1024,
				// контакт 4x4 под углом 90), доходят до приёмника.
				for i, ev := range touches {
					t.Logf("событие %d: %v pressure=%.4f major=%.1f minor=%.1f orientation=%.0f",
						i, ev.Phase, ev.Pressure, ev.Major, ev.Minor, ev.Orientation)
				}
				if p := touches[0].Pressure; p < 0.02 || p > 0.04 {
					t.Errorf("нажим первого касания %v, ждал около 32/1024", p)
				}
				if touches[0].Major <= 0 || touches[0].Minor <= 0 || touches[0].Minor > touches[0].Major {
					t.Errorf("контакт первого касания %v x %v", touches[0].Major, touches[0].Minor)
				}
			} else {
				if len(touches) != 0 {
					t.Errorf("выключенные касания дошли до приёмника: %+v", touches)
				}
				if mouse == 0 {
					t.Error("система не породила мышь из касания (регрессия: приложения без касаний теряют мышь)")
				}
			}
		})
	}
}

// Нажим и контакт читаются только по TouchMask: заглушка драйвера в полях без
// бита не должна превращаться в «давление».
func TestTouchDetailsFromInfo(t *testing.T) {
	var info pointerTouchInfo
	info.Pressure = 512
	info.Orientation = 90
	info.RcContact = rect{Left: 10, Top: 20, Right: 22, Bottom: 50}
	if p, a, b, o := touchDetailsFromInfo(&info); p != 0 || a != 0 || b != 0 || o != 0 {
		t.Errorf("без маски: %v %v %v %v, ждал нули", p, a, b, o)
	}

	info.TouchMask = touchMaskPressure
	if p, a, _, _ := touchDetailsFromInfo(&info); p != 0.5 || a != 0 {
		t.Errorf("только нажим: %v %v", p, a)
	}

	info.TouchMask = touchMaskContactArea
	p, major, minor, o := touchDetailsFromInfo(&info)
	if p != 0 || major != 30 || minor != 12 || o != 0 {
		t.Errorf("только контакт: p=%v major=%v minor=%v o=%v, ждал 0/30/12/0", p, major, minor, o)
	}

	info.TouchMask = touchMaskContactArea | touchMaskOrientation | touchMaskPressure
	info.Pressure = 2000 // драйвер вышел за диапазон
	info.Orientation = 450
	p, major, minor, o = touchDetailsFromInfo(&info)
	if p != 1 || major != 30 || minor != 12 || o != 90 {
		t.Errorf("всё сразу: p=%v major=%v minor=%v o=%v, ждал 1/30/12/90", p, major, minor, o)
	}

	// Широкий контакт: длинная сторона — Major.
	info.RcContact = rect{Left: 0, Top: 0, Right: 40, Bottom: 8}
	if _, major, minor, _ := touchDetailsFromInfo(&info); major != 40 || minor != 8 {
		t.Errorf("широкий контакт: %v x %v", major, minor)
	}
}

// Событие из POINTER_TOUCH_INFO несёт и точку, и время, и нажим с контактом.
func TestTouchEventFromInfo(t *testing.T) {
	var info pointerTouchInfo
	info.PointerInfo.DwTime = 2500
	info.TouchMask = touchMaskPressure | touchMaskContactArea | touchMaskOrientation
	info.Pressure = 256
	info.Orientation = 45
	info.RcContact = rect{Left: 0, Top: 0, Right: 6, Bottom: 10}
	ev := touchEventFromInfo(9, base.TouchMoved, &info, 31, 41)
	if ev.ID != 9 || ev.X != 31 || ev.Y != 41 || ev.Phase != base.TouchMoved || ev.Time != 2500*time.Millisecond {
		t.Errorf("основа события: %+v", ev)
	}
	if ev.Pressure != 0.25 || ev.Major != 10 || ev.Minor != 6 || ev.Orientation != 45 {
		t.Errorf("нажим/контакт: %+v", ev)
	}
}
