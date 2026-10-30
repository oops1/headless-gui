//go:build windows

package win32

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/pointer"
)

// fakeHwnd — номер окна, под которым тест регистрирует Win32Window без
// настоящего HWND: wndProc находит окно по таблице и разбирает сообщения так
// же, как для живого.
const fakeHwnd = 0x7FFF0001

func fakeWin32(t *testing.T) *Win32Window {
	t.Helper()
	w := &Win32Window{}
	win32Mu.Lock()
	win32Windows[fakeHwnd] = w
	win32Mu.Unlock()
	t.Cleanup(func() {
		win32Mu.Lock()
		delete(win32Windows, fakeHwnd)
		win32Mu.Unlock()
	})
	return w
}

func mkLParam(x, y int) uintptr {
	return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16
}

type mouseCall struct {
	x, y, button int
	pressed      bool
}

// Боковые кнопки: WM_XBUTTON* доходят до приложения кодами X1/X2, окно
// возвращает TRUE, а прежние кнопки не задеты.
func TestWin32_XButtons(t *testing.T) {
	w := fakeWin32(t)
	var got []mouseCall
	w.onMouseButton = func(x, y, b int, p bool) { got = append(got, mouseCall{x, y, b, p}) }

	cases := []struct {
		msg    uint32
		wparam uintptr
		want   mouseCall
	}{
		{wmXbuttondown, 1 << 16, mouseCall{30, 40, 5, true}},
		{wmXbuttonup, 1 << 16, mouseCall{30, 40, 5, false}},
		{wmXbuttondown, 2<<16 | 0x0040, mouseCall{30, 40, 6, true}}, // MK_XBUTTON2 в младшем слове
		{wmXbuttonup, 2 << 16, mouseCall{30, 40, 6, false}},
		// Второй щелчок приходит как DBLCLK вместо DOWN — тот же нажатый.
		{wmXbuttondblclk, 1 << 16, mouseCall{30, 40, 5, true}},
	}
	for _, c := range cases {
		got = nil
		ret := wndProc(fakeHwnd, c.msg, c.wparam, mkLParam(30, 40))
		if ret != 1 {
			t.Errorf("msg 0x%x: wndProc вернул %d, по документации ждали TRUE", c.msg, ret)
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("msg 0x%x wparam 0x%x: до приложения дошло %v, ждал %v", c.msg, c.wparam, got, c.want)
		}
	}
}

// Отрицательные координаты (окно на втором мониторе слева) знаковые.
func TestWin32_XButtonNegativeCoords(t *testing.T) {
	w := fakeWin32(t)
	var got []mouseCall
	w.onMouseButton = func(x, y, b int, p bool) { got = append(got, mouseCall{x, y, b, p}) }
	wndProc(fakeHwnd, wmXbuttondown, 1<<16, mkLParam(-5, -7))
	if len(got) != 1 || got[0].x != -5 || got[0].y != -7 {
		t.Errorf("координаты %v, ждал (-5,-7)", got)
	}
}

// TrackMouseEvent(TME_LEAVE) взводится на первом движении после входа и не
// чаще: следующее движение запрос не повторяет; после WM_MOUSELEAVE — снова.
func TestWin32_MouseMoveArmsLeaveOnce(t *testing.T) {
	w := fakeWin32(t)
	var flags []uint32
	w.trackMouse = func(hwnd uintptr, f uint32) bool {
		if hwnd != fakeHwnd {
			t.Errorf("TrackMouseEvent для чужого окна %x", hwnd)
		}
		flags = append(flags, f)
		return true
	}
	var moves [][2]int
	w.onMouseMove = func(x, y int) { moves = append(moves, [2]int{x, y}) }

	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(10, 10))
	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(11, 10))
	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(12, 10))
	if len(flags) != 1 || flags[0] != tmeLeave {
		t.Fatalf("запросов TrackMouseEvent: %v, ждал один TME_LEAVE", flags)
	}
	if len(moves) != 3 {
		t.Fatalf("движений доставлено %d, ждал 3", len(moves))
	}

	// Система прислала WM_MOUSELEAVE: приложению — движение за окно.
	moves = nil
	if ret := wndProc(fakeHwnd, wmMouseleave, 0, 0); ret != 0 {
		t.Errorf("WM_MOUSELEAVE вернул %d", ret)
	}
	if len(moves) != 1 || moves[0] != [2]int{pointer.Outside, pointer.Outside} {
		t.Fatalf("после WM_MOUSELEAVE движения: %v, ждал (−1,−1)", moves)
	}

	// Вернулись в окно — запрос взводится заново.
	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(20, 20))
	if len(flags) != 2 {
		t.Fatalf("после возврата запросов %d, ждал 2", len(flags))
	}
}

// Система отказала во взводе запроса — на следующем движении пробуем снова,
// иначе указатель навсегда остался бы без отслеживания.
func TestWin32_TrackRefusalRetried(t *testing.T) {
	w := fakeWin32(t)
	calls := 0
	w.trackMouse = func(uintptr, uint32) bool { calls++; return false }
	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(1, 1))
	wndProc(fakeHwnd, wmMousemove, 0, mkLParam(2, 1))
	if calls != 2 {
		t.Errorf("попыток взвести запрос: %d, ждал 2", calls)
	}
}

// Переход с клиентской области на кнопку заголовка, нарисованную приложением,
// ложного ухода не даёт, настоящий уход — даёт.
func TestWin32_MouseLeftOntoTitleButton(t *testing.T) {
	w := &Win32Window{}
	var moves [][2]int
	w.onMouseMove = func(x, y int) { moves = append(moves, [2]int{x, y}) }
	// Окно на экране (100,100)–(300,200); кнопка «закрыть» — в правом верхнем
	// углу, полоса заголовка выше 30 px.
	w.SetHitTest(func(x, y int) native.HitArea {
		switch {
		case y < 30 && x >= 160:
			return native.HitCloseButton
		case y < 30:
			return native.HitCaption
		}
		return native.HitClient
	})
	wr := rect{Left: 100, Top: 100, Right: 300, Bottom: 200}

	cases := []struct {
		name      string
		sx, sy    int
		havePos   bool
		wantLeave bool
	}{
		{"на кнопку «закрыть»", 290, 110, true, false},
		{"на заголовок", 150, 110, true, true},
		{"в клиентскую область (запоздавшее сообщение)", 150, 150, true, true},
		{"за окно", 50, 50, true, true},
		{"положение неизвестно", 290, 110, false, true},
	}
	for _, c := range cases {
		moves = nil
		w.mouseLeft(c.sx, c.sy, c.havePos, wr)
		got := len(moves) == 1 && moves[0] == [2]int{pointer.Outside, pointer.Outside}
		if got != c.wantLeave {
			t.Errorf("%s: уход сообщён=%v, ждал %v (движения %v)", c.name, got, c.wantLeave, moves)
		}
		if !got && len(moves) != 0 {
			t.Errorf("%s: приложению ушло лишнее %v", c.name, moves)
		}
	}
}

// Кнопка заголовка: WM_NCMOUSEMOVE ведёт подсветку и взводит отслеживание
// нерабочей области; WM_NCMOUSELEAVE уводит мышь.
func TestWin32_NcButtonTracking(t *testing.T) {
	w := fakeWin32(t)
	var flags []uint32
	w.trackMouse = func(_ uintptr, f uint32) bool { flags = append(flags, f); return true }
	var moves [][2]int
	w.onMouseMove = func(x, y int) { moves = append(moves, [2]int{x, y}) }

	wndProc(fakeHwnd, wmNcmousemove, htMaxButton, mkLParam(5, 6))
	wndProc(fakeHwnd, wmNcmousemove, htMaxButton, mkLParam(6, 6))
	if len(flags) != 1 || flags[0] != tmeLeave|tmeNonclient {
		t.Fatalf("запросы %v, ждал один TME_LEAVE|TME_NONCLIENT", flags)
	}
	if len(moves) != 2 {
		t.Fatalf("движений %d, ждал 2", len(moves))
	}
	moves = nil
	wndProc(fakeHwnd, wmNcmouseleave, 0, 0)
	if len(moves) != 1 || moves[0] != [2]int{pointer.Outside, pointer.Outside} {
		t.Fatalf("после WM_NCMOUSELEAVE: %v", moves)
	}
	wndProc(fakeHwnd, wmNcmousemove, htMaxButton, mkLParam(5, 6))
	if len(flags) != 2 {
		t.Errorf("после ухода с кнопки запрос не взведён заново: %v", flags)
	}
}

func TestWin32CursorSpec(t *testing.T) {
	cases := []struct {
		c    int
		want win32CursorSpec
	}{
		{pointer.CursorArrow, win32CursorSpec{idc: idcArrow}},
		{pointer.CursorIBeam, win32CursorSpec{idc: idcIBeam}},
		{pointer.CursorHand, win32CursorSpec{idc: idcHand}},
		{pointer.CursorSizeWE, win32CursorSpec{idc: idcSizeWE}},
		{pointer.CursorSizeNS, win32CursorSpec{idc: idcSizeNS}},
		{pointer.CursorSizeNWSE, win32CursorSpec{idc: idcSizeNWSE}},
		{pointer.CursorSizeNESW, win32CursorSpec{idc: idcSizeNESW}},
		{pointer.CursorNone, win32CursorSpec{hidden: true}},
		{pointer.CursorGrab, win32CursorSpec{art: true}},
		{pointer.CursorGrabbing, win32CursorSpec{art: true}},
		{pointer.CursorSizeAll, win32CursorSpec{idc: 32646}},   // IDC_SIZEALL
		{pointer.CursorCrosshair, win32CursorSpec{idc: 32515}}, // IDC_CROSS
		{pointer.CursorWait, win32CursorSpec{idc: 32514}},      // IDC_WAIT
		{999, win32CursorSpec{idc: idcArrow}},                  // незнакомая — стрелка
	}
	for _, c := range cases {
		if got := win32CursorSpecFor(c.c); got != c.want {
			t.Errorf("форма %d → %+v, ждал %+v", c.c, got, c.want)
		}
	}
}

// CursorNone: WM_SETCURSOR над клиентской областью ставит SetCursor(NULL), а
// не уходит в DefWindowProc (тот вернул бы класс-курсор «стрелка»).
func TestWin32_HiddenCursorMessage(t *testing.T) {
	w := &Win32Window{}
	w.SetCursor(pointer.CursorNone) // hwnd == 0: системный SetCursor не зовётся
	if !w.cursorHidden {
		t.Fatal("SetCursor(None) не спрятал курсор")
	}
	h, handled := w.setCursorMessage(htClient)
	if !handled || h != 0 {
		t.Fatalf("WM_SETCURSOR(HTCLIENT) при скрытом курсоре: h=%d handled=%v, ждал 0,true", h, handled)
	}
	// Над рамкой и кнопками курсор выбирает система.
	if _, handled := w.setCursorMessage(htClose); handled {
		t.Error("над нерабочей областью курсор должен выбирать система")
	}

	// Вернули форму — курсор снова виден.
	w.SetCursor(pointer.CursorCrosshair)
	if w.cursorHidden {
		t.Fatal("курсор остался спрятанным после смены формы")
	}
	h, handled = w.setCursorMessage(htClient)
	if !handled || h == 0 {
		t.Fatalf("после смены формы: h=%d handled=%v", h, handled)
	}
}

// Окно, которому курсор ещё не задавали, оставляет его системе.
func TestWin32_CursorUnsetLeavesToSystem(t *testing.T) {
	w := &Win32Window{}
	if _, handled := w.setCursorMessage(htClient); handled {
		t.Error("без заданной формы WM_SETCURSOR должен идти в DefWindowProc")
	}
}

// Рисованные руки создаются настоящими HCURSOR, кэшируются и освобождаются.
func TestWin32_ArtCursors(t *testing.T) {
	w := &Win32Window{}
	grab, hidden := w.cursorFor(pointer.CursorGrab)
	grabbing, _ := w.cursorFor(pointer.CursorGrabbing)
	if hidden || grab == 0 || grabbing == 0 {
		t.Fatalf("курсоры не созданы: grab=%x grabbing=%x hidden=%v", grab, grabbing, hidden)
	}
	if grab == grabbing {
		t.Error("раскрытая и сжатая рука — один и тот же HCURSOR")
	}
	if again, _ := w.cursorFor(pointer.CursorGrab); again != grab {
		t.Error("курсор формы не кэшируется")
	}
	w.destroyArtCursors()
	if len(w.cursorCache) != 0 {
		t.Errorf("после destroyArtCursors в кэше осталось %v", w.cursorCache)
	}
	// Остальные новые формы — системные курсоры.
	for _, c := range []int{pointer.CursorSizeAll, pointer.CursorCrosshair, pointer.CursorWait} {
		if h, _ := w.cursorFor(c); h == 0 {
			t.Errorf("форма %d: системный курсор не загружен", c)
		}
	}
}
