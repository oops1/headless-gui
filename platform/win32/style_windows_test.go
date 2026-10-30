package win32

// Живые окна Win32: стиль (native.Styler), владелец (native.OwnerSetter),
// Z-порядок (native.Stacker) и положение (native.Positioner) — style_windows.go.

import (
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// stExStyle — расширенный стиль окна.
func stExStyle(w *Win32Window) uintptr {
	ex, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), gwlExstyleAOT)
	return ex
}

// stNCRendering — рисует ли DWM неклиентскую часть окна
// (DWMWA_NCRENDERING_ENABLED): без неё нет ни тени, ни скругления.
func stNCRendering(t *testing.T, w *Win32Window) bool {
	t.Helper()
	const dwmwaNcrenderingEnabled = 1
	var on int32
	r, _, _ := procDwmGetWindowAttribute.Call(uintptr(w.hwnd), dwmwaNcrenderingEnabled,
		uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
	if r != 0 {
		t.Skipf("DwmGetWindowAttribute: %#x (DWM выключен?)", r)
	}
	return on != 0
}

// stHitLeftEdge — что окно отвечает системе на точку у левого края.
func stHitLeftEdge(w *Win32Window) uintptr {
	r := w.windowRect()
	x, y := r.Min.X+2, r.Min.Y+r.Dy()/2
	lp := uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16
	return wndProc(uintptr(w.hwnd), wmNchittest, 0, lp)
}

// stAbove — upper выше lower в Z-порядке.
func stAbove(upper, lower *Win32Window) bool {
	for h := uintptr(lower.hwnd); ; {
		if h, _, _ = procGetWindow.Call(h, gwHwndprev); h == 0 {
			return false
		}
		if h == uintptr(upper.hwnd) {
			return true
		}
	}
}

// stNewWindow — главное окно, ещё не созданное: тест сам решает, что задать
// до Create.
func stNewWindow(t *testing.T) *Win32Window {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	w := &Win32Window{}
	w.noQuit = true
	return w
}

// stCreate создаёт окно stNewWindow и уничтожает его по окончании теста.
func stCreate(t *testing.T, w *Win32Window, title string) {
	t.Helper()
	if err := w.Create(title, 400, 300); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	t.Cleanup(func() {
		w.noQuit = true
		procDestroyWindow.Call(uintptr(w.hwnd))
	})
}

func TestWin32Style_ToolBeforeCreate(t *testing.T) {
	w := stNewWindow(t)
	if got := w.ApplyStyle(native.Style{Tool: true}); got != (native.Style{Tool: true}) {
		t.Fatalf("ApplyStyle до Create = %+v", got)
	}
	stCreate(t, w, "style-tool")
	if ex := stExStyle(w); ex&wsExToolwindow == 0 || ex&wsExAppwindow != 0 {
		t.Fatalf("окно создано со стилем %#x, ждал WS_EX_TOOLWINDOW без WS_EX_APPWINDOW", ex)
	}
	if got := w.liveStyle(); !got.Tool {
		t.Error("liveStyle не видит Tool")
	}
}

// Смена Tool у видимого окна: стиль меняется, окно остаётся видимым.
func TestWin32Style_ToolLiveToggle(t *testing.T) {
	w := newFsWindow(t)
	if ex := stExStyle(w); ex&wsExAppwindow == 0 || ex&wsExToolwindow != 0 {
		t.Fatalf("обычное окно со стилем %#x", ex)
	}
	if got := w.ApplyStyle(native.Style{Tool: true}); got != (native.Style{Tool: true}) {
		t.Fatalf("ApplyStyle(Tool) = %+v", got)
	}
	if ex := stExStyle(w); ex&wsExToolwindow == 0 || ex&wsExAppwindow != 0 {
		t.Fatalf("после ApplyStyle(Tool) стиль %#x", ex)
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v == 0 {
		t.Fatal("окно осталось скрытым после смены стиля")
	}
	if got := w.ApplyStyle(native.Style{}); got != (native.Style{}) {
		t.Fatalf("ApplyStyle({}) = %+v", got)
	}
	if ex := stExStyle(w); ex&wsExAppwindow == 0 || ex&wsExToolwindow != 0 {
		t.Fatalf("после ApplyStyle({}) стиль %#x", ex)
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v == 0 {
		t.Fatal("окно осталось скрытым после возврата стиля")
	}
}

// Скрытое окно (в трее) меняет стиль, не показываясь.
func TestWin32Style_ToolHiddenStaysHidden(t *testing.T) {
	w := newFsWindow(t)
	procShowWindow.Call(uintptr(w.hwnd), swHide)
	w.ApplyStyle(native.Style{Tool: true})
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v != 0 {
		t.Fatal("скрытое окно показалось при смене стиля")
	}
	if stExStyle(w)&wsExToolwindow == 0 {
		t.Fatal("стиль скрытого окна не сменился")
	}
}

// Borderless: зон ресайза нет, DWM неклиентскую часть не рисует (ни тени, ни
// скругления); обратная смена всё возвращает.
func TestWin32Style_BorderlessLive(t *testing.T) {
	w := newFsWindow(t)
	w.SetResizable(true)
	if hit := stHitLeftEdge(w); hit != htLeft {
		t.Fatalf("обычное окно у левого края отвечает %d, ждал HTLEFT", hit)
	}
	ncBefore := stNCRendering(t, w)
	w.SetCornerRadius(8)
	rounded := w.RoundsCorners()

	if got := w.ApplyStyle(native.Style{Borderless: true}); got != (native.Style{Borderless: true}) {
		t.Fatalf("ApplyStyle(Borderless) = %+v", got)
	}
	if hit := stHitLeftEdge(w); hit == htLeft {
		t.Error("у Borderless-окна осталась зона ресайза у края")
	}
	if stNCRendering(t, w) {
		t.Error("DWM рисует неклиентскую часть Borderless-окна (тень)")
	}
	if w.RoundsCorners() {
		t.Error("RoundsCorners у Borderless-окна")
	}

	if got := w.ApplyStyle(native.Style{}); got != (native.Style{}) {
		t.Fatalf("ApplyStyle({}) = %+v", got)
	}
	if hit := stHitLeftEdge(w); hit != htLeft {
		t.Errorf("зона ресайза не вернулась: %d", hit)
	}
	if got := stNCRendering(t, w); got != ncBefore {
		t.Errorf("неклиентская отрисовка DWM %v, до Borderless было %v", got, ncBefore)
	}
	if got := w.RoundsCorners(); got != rounded {
		t.Errorf("RoundsCorners %v, до Borderless было %v", got, rounded)
	}
}

// Borderless до Create: окно рождается без тени, без перехода после показа.
func TestWin32Style_BorderlessBeforeCreate(t *testing.T) {
	w := stNewWindow(t)
	if got := w.ApplyStyle(native.Style{Borderless: true}); got != (native.Style{Borderless: true}) {
		t.Fatalf("ApplyStyle до Create = %+v", got)
	}
	if w.RoundsCorners() {
		t.Error("RoundsCorners у Borderless-окна до создания")
	}
	stCreate(t, w, "style-borderless")
	if stNCRendering(t, w) {
		t.Error("Borderless-окно создано с неклиентской отрисовкой DWM")
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(w.hwnd)); v == 0 {
		t.Error("Borderless-окно создано скрытым и не показано")
	}
	if stExStyle(w)&wsExAppwindow == 0 {
		t.Error("Borderless без Tool потерял WS_EX_APPWINDOW")
	}
}

// ApplyStyle с чужой горутины выполняется на потоке окна и отвечает итогом.
func TestWin32Style_CrossThread(t *testing.T) {
	w := newFsWindow(t)
	done := make(chan native.Style, 1)
	go func() { done <- w.ApplyStyle(native.Style{Borderless: true, Tool: true}) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		fsPumpMessages()
		select {
		case got := <-done:
			if got != (native.Style{Borderless: true, Tool: true}) {
				t.Fatalf("ApplyStyle с чужой горутины = %+v", got)
			}
			if stExStyle(w)&wsExToolwindow == 0 {
				t.Fatal("Tool не применён")
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("ApplyStyle с чужой горутины завис")
		}
		time.Sleep(time.Millisecond)
	}
}

// Поток окна не крутит цикл сообщений: ApplyStyle по сроку отвечает тем, что
// действует, и не виснет.
func TestWin32Style_StuckUIThread(t *testing.T) {
	old := styleUITimeout
	styleUITimeout = 30 * time.Millisecond
	t.Cleanup(func() { styleUITimeout = old })
	w := newFsWindow(t)
	done := make(chan native.Style, 1)
	go func() { done <- w.ApplyStyle(native.Style{Tool: true}) }()
	select {
	case got := <-done:
		if got.Tool {
			t.Errorf("по сроку ApplyStyle = %+v, ждал стиль, который действует", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ApplyStyle завис вместе с потоком окна")
	}
	fsPumpMessages() // вызов, отменённый по сроку, не выполняется и после
	if stExStyle(w)&wsExToolwindow != 0 {
		t.Error("ApplyStyle, отменённый по сроку, всё же применил Tool")
	}
}

// Окно уничтожено, пока вызов ждал очереди (WM_DESTROY выбрасывает вызовы
// окна): ApplyStyle отвечает сразу, а не по сроку.
func TestWin32Style_DestroyedWhileWaiting(t *testing.T) {
	w := newFsWindow(t)
	w.Close() // WM_CLOSE встаёт в очередь раньше вызова стиля
	done := make(chan native.Style, 1)
	start := time.Now()
	go func() { done <- w.ApplyStyle(native.Style{Tool: true}) }()
	// Ждём, пока вызов встанет в очередь (WM_APP за WM_CLOSE), ничего не
	// разбирая: иначе окно могло умереть раньше, и ApplyStyle ответил бы
	// сразу, минуя ожидание, которое здесь и проверяется.
	for {
		var m msg
		const pmNoremove = 0
		if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), uintptr(w.hwnd), wmApp, wmApp, pmNoremove); r != 0 {
			break
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("вызов стиля не встал в очередь")
		}
		time.Sleep(time.Millisecond)
	}
	for {
		fsPumpMessages()
		select {
		case got := <-done:
			if got.Tool {
				t.Errorf("ApplyStyle у уничтоженного окна = %+v", got)
			}
			if d := time.Since(start); d >= styleUITimeout/2 {
				t.Errorf("ответ через %v: ожидание не заметило гибели окна", d)
			}
			return
		default:
		}
		if time.Since(start) > 2*styleUITimeout {
			t.Fatal("ApplyStyle не ответил")
		}
		time.Sleep(time.Millisecond)
	}
}

// Смена Tool у видимого неактивного окна прячет и показывает его, но место в
// Z-порядке оставляет прежним. Подъём при показе Windows делает не всегда
// (окну фонового процесса — не всегда), поэтому без возврата места тест
// падает не в любом окружении; со возвратом — проходит в любом.
func TestWin32Style_ToolKeepsZOrder(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	if !b.LowerWindow() || !stAbove(a, b) {
		t.Fatal("предусловие: b не опустилось под a")
	}
	if fg, _, _ := procGetForegroundWindow.Call(); fg == uintptr(b.hwnd) {
		t.Skip("b активно: его показ с активацией и должен поднять")
	}
	for _, st := range []native.Style{{Tool: true}, {}} {
		b.ApplyStyle(st)
		if !stAbove(a, b) {
			t.Fatalf("ApplyStyle(%+v) подняло неактивное окно над a", st)
		}
	}
}

func TestWin32Owner_Live(t *testing.T) {
	a := newFsWindow(t)
	b := stNewWindow(t)
	b.noQuit = false
	stCreate(t, b, "owned")
	a.RaiseWindow() // b — ниже владельца, пока его нет
	if !stAbove(a, b) {
		t.Fatal("предусловие: b не ниже a")
	}

	if !b.SetOwnerWindow(a) {
		t.Fatal("SetOwnerWindow(a) = false")
	}
	if p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent); p != uintptr(a.hwnd) {
		t.Fatalf("GWLP_HWNDPARENT = %#x, ждал %#x", p, uintptr(a.hwnd))
	}
	if b.noQuit {
		t.Error("SetOwnerWindow пометил окно вторичным (это делает только SetOwner модалки)")
	}
	if !stAbove(b, a) {
		t.Error("окно с владельцем осталось ниже владельца")
	}
	if !b.SetOwnerWindow(nil) {
		t.Fatal("SetOwnerWindow(nil) = false")
	}
	if p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent); p != 0 {
		t.Fatalf("владелец не снят: %#x", p)
	}
}

func TestWin32Owner_Refusals(t *testing.T) {
	a := newFsWindow(t)
	if a.SetOwnerWindow(a) {
		t.Error("окно приняло себя владельцем")
	}
	if a.SetOwnerWindow(&Win32Window{}) {
		t.Error("принят владелец, у которого ещё нет окна")
	}
	if a.SetOwnerWindow(stForeign{}) {
		t.Error("принят владелец чужого бэкенда")
	}
}

// stForeign — окно не Win32 (владельцем быть не может).
type stForeign struct{ native.NativeWindow }

// Закрытый владелец не принимается ни до, ни после Create; заказанный до
// Create и закрытый к нему — не передаётся в CreateWindowExW (иначе окно не
// создалось бы вовсе).
func TestWin32Owner_DeadOwner(t *testing.T) {
	a := newFsWindow(t)
	procDestroyWindow.Call(uintptr(a.hwnd))
	fsPumpMessages()
	b := stNewWindow(t)
	if b.SetOwnerWindow(a) {
		t.Error("до Create принят закрытый владелец")
	}
	b.style.owner = a // заказан, пока был жив
	if err := b.Create("owner-died", 400, 300); err != nil {
		t.Fatalf("Create с закрытым владельцем, заказанным раньше: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(uintptr(b.hwnd)) })
	if p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent); p != 0 {
		t.Errorf("окно создано с закрытым владельцем %#x", p)
	}
	if b.SetOwnerWindow(a) {
		t.Error("после Create принят закрытый владелец")
	}
}

// Владелец «поверх всех»: окно встаёт над ним и само становится «поверх
// всех» (так Windows поступает с окнами такого владельца); не встало —
// владелец не меняется и ответ false.
func TestWin32Owner_TopmostOwner(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	if !a.SetAlwaysOnTop(true) {
		t.Skip("Windows не ставит окно теста поверх всех (фоновый процесс)")
	}
	ok := b.SetOwnerWindow(a)
	p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent)
	if !ok {
		if p != 0 {
			t.Errorf("отказ, но владелец остался: %#x", p)
		}
		t.Skip("Windows не пустила окно над владельцем «поверх всех»")
	}
	if p != uintptr(a.hwnd) || !stAbove(b, a) {
		t.Errorf("SetOwnerWindow = true: владелец %#x, над ним %v", p, stAbove(b, a))
	}
	if !b.IsAlwaysOnTop() {
		t.Error("окно владельца «поверх всех» не стало «поверх всех»")
	}
}

// Владелец, заказанный до Create, закладывается в создание окна.
func TestWin32Owner_BeforeCreate(t *testing.T) {
	a := newFsWindow(t)
	b := stNewWindow(t)
	if !b.SetOwnerWindow(a) {
		t.Fatal("SetOwnerWindow до Create = false")
	}
	stCreate(t, b, "owned-before")
	if p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent); p != uintptr(a.hwnd) {
		t.Fatalf("GWLP_HWNDPARENT = %#x, ждал %#x", p, uintptr(a.hwnd))
	}
	// Владелец, заданный при создании, тоже снимается его WM_CLOSE: окно
	// не гибнет вместе с ним.
	a.Close()
	fsPumpMessages()
	if hwndAlive(uintptr(a.hwnd)) {
		t.Fatal("владелец не закрылся")
	}
	if !hwndAlive(uintptr(b.hwnd)) {
		t.Fatal("окно с владельцем, заданным при создании, погибло вместе с ним")
	}
}

// Raise и Lower меняют порядок без активации. Над активным окном другого
// процесса окно теста (фоновый процесс) Windows не пускает, поэтому ответ
// Raise сверяется с тем, где окно оказалось, а не с true.
func TestWin32Stack_RaiseLower(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	fg, _, _ := procGetForegroundWindow.Call()
	if ok := a.RaiseWindow(); !stAbove(a, b) || ok != topOfLayer(uintptr(a.hwnd)) {
		t.Fatalf("RaiseWindow(a) = %v: a выше b %v, верхнее в слое %v", ok, stAbove(a, b), topOfLayer(uintptr(a.hwnd)))
	}
	if topOfLayer(uintptr(b.hwnd)) {
		t.Fatal("topOfLayer(b) = true, хотя над b видимое окно a того же слоя")
	}
	if ok := b.RaiseWindow(); !stAbove(b, a) || ok != topOfLayer(uintptr(b.hwnd)) {
		t.Fatalf("RaiseWindow(b) = %v: b выше a %v", ok, stAbove(b, a))
	}
	if !b.LowerWindow() || !stAbove(a, b) {
		t.Fatal("LowerWindow(b): b не ниже a")
	}
	if ok := b.RaiseWindow(); !stAbove(b, a) || ok != topOfLayer(uintptr(b.hwnd)) {
		t.Fatalf("RaiseWindow(b) снизу = %v: b выше a %v", ok, stAbove(b, a))
	}
	// Активное окно могло смениться и без нас (параллельные тесты других
	// пакетов): ошибка — только если активным стало окно этого процесса.
	if now, _, _ := procGetForegroundWindow.Call(); now != fg && stOwnProcess(now) {
		t.Errorf("Raise и Lower сменили активное окно: %#x → %#x", fg, now)
	}
	if (&Win32Window{}).RaiseWindow() || (&Win32Window{}).LowerWindow() {
		t.Error("Raise или Lower без окна = true")
	}
}

// Окно «поверх всех» не опускается: HWND_BOTTOM молча снял бы WS_EX_TOPMOST.
func TestWin32Stack_LowerKeepsTopmost(t *testing.T) {
	w := newFsWindow(t)
	if !w.SetAlwaysOnTop(true) {
		t.Skip("Windows не ставит окно теста поверх всех (фоновый процесс)")
	}
	if w.LowerWindow() {
		t.Error("окно поверх всех опустилось")
	}
	if !w.IsAlwaysOnTop() {
		t.Error("LowerWindow снял «поверх всех»")
	}
}

// Ответ SetAlwaysOnTop совпадает с тем, что действует. Окно, опущенное под
// активное окно другого процесса, Windows молча не ставит поверх всех, хотя
// SetWindowPos отвечает успехом: прежде SetAlwaysOnTop отвечал тогда true.
// Отказ зависит от того, что активно на рабочем столе, — прежний ответ тест
// ловит не в любом окружении.
func TestWin32AlwaysOnTop_AnswerMatchesState(t *testing.T) {
	newFsWindow(t) // с одним окном теста Windows так себя не ведёт
	b := newFsWindow(t)
	b.LowerWindow()
	if ok := b.SetAlwaysOnTop(true); ok != b.IsAlwaysOnTop() {
		t.Fatalf("SetAlwaysOnTop(true) = %v, а WS_EX_TOPMOST %v", ok, b.IsAlwaysOnTop())
	}
	if ok := b.SetAlwaysOnTop(false); !ok || b.IsAlwaysOnTop() {
		t.Fatalf("SetAlwaysOnTop(false) = %v, WS_EX_TOPMOST %v", ok, b.IsAlwaysOnTop())
	}
}

func TestWin32Position_MoveTo(t *testing.T) {
	w := newFsWindow(t)
	if !w.MoveTo(150, 120) {
		t.Fatal("MoveTo = false")
	}
	if x, y, ok := w.ScreenPosition(); !ok || x != 150 || y != 120 {
		t.Fatalf("ScreenPosition = %d, %d, %v", x, y, ok)
	}
	procShowWindow.Call(uintptr(w.hwnd), swMinimize)
	if w.MoveTo(10, 10) {
		t.Error("MoveTo у свёрнутого окна = true")
	}
	if _, _, ok := w.ScreenPosition(); ok {
		t.Error("ScreenPosition у свёрнутого окна ok")
	}
	procShowWindow.Call(uintptr(w.hwnd), swRestore)
	procShowWindow.Call(uintptr(w.hwnd), swMaximize)
	if w.MoveTo(10, 10) {
		t.Error("MoveTo у развёрнутого окна = true")
	}
	var empty Win32Window
	if empty.MoveTo(1, 1) {
		t.Error("MoveTo без окна = true")
	}
	if _, _, ok := empty.ScreenPosition(); ok {
		t.Error("ScreenPosition без окна ok")
	}
}

// stOwnProcess — окно h принадлежит процессу теста.
func stOwnProcess(h uintptr) bool {
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return int(pid) == os.Getpid()
}

// Кто мешает окну быть верхним (blocksTop, на нём стоят topOfLayer и ответ
// RaiseWindow): видимое окно того же слоя — да; своё (с ним во владельцах),
// скрытое и окно другого слоя — нет. Проверяется правило на каждом виде
// окна, а не итог RaiseWindow: тот зависит от того, активен ли процесс теста.
func TestWin32BlocksTop(t *testing.T) {
	a := newFsWindow(t)
	plain := newFsWindow(t)
	if !blocksTop(uintptr(a.hwnd), uintptr(plain.hwnd)) {
		t.Error("видимое обычное окно не мешает обычному")
	}
	owned := newFsWindow(t)
	procSetWindowLongPtrW.Call(uintptr(owned.hwnd), gwlpHwndParent, uintptr(a.hwnd))
	if !ownedBy(uintptr(owned.hwnd), uintptr(a.hwnd)) {
		t.Fatal("окно не получило владельца a")
	}
	if blocksTop(uintptr(a.hwnd), uintptr(owned.hwnd)) {
		t.Error("своё окно (с владельцем a) мешает a")
	}
	hidden := newFsWindow(t)
	procShowWindow.Call(uintptr(hidden.hwnd), swHide)
	if blocksTop(uintptr(a.hwnd), uintptr(hidden.hwnd)) {
		t.Error("скрытое окно мешает a")
	}
	top := newFsWindow(t)
	if top.SetAlwaysOnTop(true) {
		if blocksTop(uintptr(a.hwnd), uintptr(top.hwnd)) {
			t.Error("окно «поверх всех» (другой слой) мешает обычному")
		}
		if blocksTop(uintptr(top.hwnd), uintptr(a.hwnd)) {
			t.Error("обычное окно мешает окну «поверх всех»")
		}
	}
}

// stOwnedChild — окно с владельцем owner, поставленное над ним.
func stOwnedChild(t *testing.T, owner *Win32Window) *Win32Window {
	t.Helper()
	c := newFsWindow(t)
	procSetWindowLongPtrW.Call(uintptr(c.hwnd), gwlpHwndParent, uintptr(owner.hwnd))
	procSetWindowPos.Call(uintptr(c.hwnd), hwndTop, 0, 0, 0, 0, swpNomove|swpNosize|swpNoactivate)
	if !stAbove(c, owner) {
		t.Fatal("окно с владельцем не встало над ним")
	}
	return c
}

// Смена Tool оставляет окна, принадлежащие окну (его диалоги, панели), над
// ним и на прежнем месте относительно чужих окон.
func TestWin32Style_ToolKeepsOwnedAbove(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	d := stOwnedChild(t, b)
	// Порядок [a, d, b]: a над группой b.
	procSetWindowPos.Call(uintptr(b.hwnd), hwndBottom, 0, 0, 0, 0, swpNomove|swpNosize|swpNoactivate)
	if !stAbove(a, d) || !stAbove(d, b) {
		t.Fatal("предусловие: порядок не [a, d, b]")
	}
	if fg, _, _ := procGetForegroundWindow.Call(); fg == uintptr(b.hwnd) || fg == uintptr(d.hwnd) {
		t.Skip("группа b активна")
	}
	for _, st := range []native.Style{{Tool: true}, {}} {
		b.ApplyStyle(st)
		if !stAbove(d, b) {
			t.Fatalf("ApplyStyle(%+v): окно b встало над своим окном d", st)
		}
		if !stAbove(a, d) {
			t.Fatalf("ApplyStyle(%+v): группа b поднялась над a", st)
		}
	}
}

// Подъём над новым владельцем берёт с собой окна, принадлежащие окну.
func TestWin32Owner_PlaceKeepsOwnedAbove(t *testing.T) {
	m := newFsWindow(t)
	b := newFsWindow(t)
	c := stOwnedChild(t, b)
	m.RaiseWindow() // [m, c, b] или выше: m над группой b
	if !stAbove(m, c) {
		t.Fatal("предусловие: m не над группой b")
	}
	if !b.SetOwnerWindow(m) {
		t.Fatal("SetOwnerWindow(m) = false")
	}
	if !stAbove(b, m) {
		t.Error("b не встало над владельцем")
	}
	if !stAbove(c, b) {
		t.Error("окно c, принадлежащее b, осталось под ним")
	}
}

// Окно, которое спрятал свёрнутый владелец, после снятия владельца (его
// закрывают) показывается само: иначе показать его было бы некому.
func TestWin32Owner_ReleaseShowsHiddenByOwner(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	// c — окно самого b (его диалог). Свёрнутый a его не прячет; после
	// снятия владельца оно видно.
	c := newFsWindow(t)
	if !b.SetOwnerWindow(a) || !c.SetOwnerWindow(b) {
		t.Fatal("SetOwnerWindow = false")
	}
	procShowWindow.Call(uintptr(a.hwnd), swMinimize)
	fsPumpMessages()
	if v, _, _ := procIsWindowVisible.Call(uintptr(b.hwnd)); v != 0 {
		t.Skip("Windows не спрятала окно вместе со свёрнутым владельцем")
	}
	if !b.style.hiddenByOwner.Load() {
		t.Fatal("окно, спрятанное владельцем, не отмечено")
	}
	if !b.SetOwnerWindow(nil) {
		t.Fatal("SetOwnerWindow(nil) = false")
	}
	fsPumpMessages()
	if v, _, _ := procIsWindowVisible.Call(uintptr(b.hwnd)); v == 0 {
		t.Error("окно, спрятанное свёрнутым владельцем, после снятия владельца осталось скрытым")
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(c.hwnd)); v == 0 {
		t.Error("окно, принадлежащее освобождённому, осталось скрытым")
	}
	if fg, _, _ := procGetForegroundWindow.Call(); fg == uintptr(b.hwnd) {
		t.Error("показ активировал окно")
	}
}

// Владелец закрывается: окна, которым его поставил SetOwnerWindow, Windows не
// уничтожает вместе с ним (WM_CLOSE снимает владельца), а окна модальных
// диалогов (SetOwner) гибнут с ним, как прежде.
func TestWin32Owner_CloseReleasesOwned(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	m := newFsWindow(t)
	if !b.SetOwnerWindow(a) {
		t.Fatal("SetOwnerWindow(a) = false")
	}
	m.SetOwner(a) // модальное окно
	a.Close()
	fsPumpMessages()
	if hwndAlive(uintptr(a.hwnd)) {
		t.Fatal("владелец не закрылся")
	}
	if !hwndAlive(uintptr(b.hwnd)) {
		t.Fatal("окно SetOwnerWindow погибло вместе с владельцем")
	}
	if o, _, _ := procGetWindow.Call(uintptr(b.hwnd), gwOwner); o != 0 || b.style.ownerSet.Load() {
		t.Errorf("у освобождённого окна владелец %#x, отметка %v", o, b.style.ownerSet.Load())
	}
	if hwndAlive(uintptr(m.hwnd)) {
		t.Error("модальное окно пережило владельца")
	}
}

// SetOwnerWindow с чужой горутины ставит владельца на его потоке и отвечает
// итогом.
func TestWin32Owner_CrossThread(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	done := make(chan bool, 1)
	go func() { done <- b.SetOwnerWindow(a) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		fsPumpMessages()
		select {
		case ok := <-done:
			if p, _, _ := procGetWindowLongPtrW.Call(uintptr(b.hwnd), gwlpHwndParent); !ok || p != uintptr(a.hwnd) {
				t.Fatalf("SetOwnerWindow с чужой горутины = %v, владелец %#x", ok, p)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("SetOwnerWindow с чужой горутины завис")
		}
		time.Sleep(time.Millisecond)
	}
}

// Окно, свёрнутое само до сворачивания владельца, после снятия владельца
// показывается свёрнутым, а его диалог, который спрятало его сворачивание,
// остаётся скрытым: его вернёт восстановление окна.
func TestWin32Owner_ReleaseKeepsMinimizedState(t *testing.T) {
	a := newFsWindow(t)
	b := newFsWindow(t)
	c := newFsWindow(t)
	if !b.SetOwnerWindow(a) || !c.SetOwnerWindow(b) {
		t.Fatal("SetOwnerWindow = false")
	}
	procShowWindow.Call(uintptr(b.hwnd), swMinimize)
	fsPumpMessages()
	procShowWindow.Call(uintptr(a.hwnd), swMinimize)
	fsPumpMessages()
	if v, _, _ := procIsWindowVisible.Call(uintptr(b.hwnd)); v != 0 {
		t.Skip("Windows не спрятала свёрнутое окно вместе со свёрнутым владельцем")
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(c.hwnd)); v != 0 {
		t.Skip("Windows не спрятала диалог свёрнутого окна")
	}
	b.SetOwnerWindow(nil)
	fsPumpMessages()
	if v, _, _ := procIsWindowVisible.Call(uintptr(b.hwnd)); v == 0 {
		t.Error("окно осталось скрытым после снятия владельца")
	}
	if r, _, _ := procIsIconic.Call(uintptr(b.hwnd)); r == 0 {
		t.Error("свёрнутое окно показалось развёрнутым")
	}
	if v, _, _ := procIsWindowVisible.Call(uintptr(c.hwnd)); v != 0 {
		t.Error("диалог свёрнутого окна показался рядом со свёрнутым окном")
	}
}
