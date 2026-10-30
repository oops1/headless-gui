//go:build windows

package tray

import (
	"errors"
	"image"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/internal/testhook"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/win32"
)

var (
	testUser32            = windows.NewLazySystemDLL("user32.dll")
	procTestSendMessageW  = testUser32.NewProc("SendMessageW")
	procTestDestroyWindow = testUser32.NewProc("DestroyWindow")
)

// Сквозная проверка на живом окне Win32 через трей, который зарегистрировал
// init пакета: Shell_NotifyIcon получает NIM_ADD, затем NIM_MODIFY; события
// иконки (wmTrayCallback) доходят до колбэков через обработчик бэкенда;
// WM_DESTROY снимает иконку (NIM_DELETE) и пропускает сообщение дальше — окно
// бэкенд убирает (handled=false); после этого трей отвечает отказом.
func TestTray_LiveWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, ok := native.TrayService()
	if !ok {
		t.Fatal("init пакета не зарегистрировал трей")
	}
	if f(nil) != nil {
		t.Error("трей выдан окну, которое не Win32")
	}
	nw, err := testhook.Win32NewWindow("tray-test", 200, 100)
	if err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	w := nw.(*win32.Win32Window)
	hwnd := w.HWND()
	destroyed := false
	defer func() {
		if !destroyed {
			procTestDestroyWindow.Call(hwnd)
		}
	}()

	var msgs []uint32
	old := shellNotifyIcon
	shellNotifyIcon = func(msg uint32, nid *notifyIconDataW) bool {
		if uintptr(nid.HWnd) == hwnd {
			msgs = append(msgs, msg)
		}
		return true
	}
	defer func() { shellNotifyIcon = old }()

	tr := f(w)
	if tr == nil {
		t.Fatal("трей окну Win32 не выдан")
	}
	type click struct {
		button int
		dbl    bool
	}
	var clicks []click
	balloons := 0
	tr.SetTrayClickHandler(func(b int, dbl bool) { clicks = append(clicks, click{b, dbl}) })
	tr.SetBalloonClickHandler(func() { balloons++ })
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if err := tr.SetTrayIcon(icon, "подсказка"); err != nil {
		t.Fatal(err)
	}
	if err := tr.SetTrayIcon(icon, "другая"); err != nil {
		t.Fatal(err)
	}
	if err := tr.ShowBalloon("заголовок", "текст", 1); err != nil {
		t.Fatal(err)
	}
	if tr.SetTrayMenuNative(nil) {
		t.Error("меню трея Windows объявлено системным")
	}

	procTestSendMessageW.Call(hwnd, wmTrayCallback, 0, trayEvtLButtonDblClk)
	procTestSendMessageW.Call(hwnd, wmTrayCallback, 0, trayEvtRButtonUp)
	procTestSendMessageW.Call(hwnd, wmTrayCallback, 0, trayEvtBalloonUserClick)
	if len(clicks) != 2 || clicks[0] != (click{0, true}) || clicks[1] != (click{1, false}) || balloons != 1 {
		t.Fatalf("события иконки через окно: клики %v, уведомление %d", clicks, balloons)
	}

	procTestDestroyWindow.Call(hwnd)
	destroyed = true
	want := []uint32{nimAdd, nimModify, nimModify, nimDelete}
	if len(msgs) != len(want) {
		t.Fatalf("Shell_NotifyIcon: %v, ждал %v", msgs, want)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Fatalf("Shell_NotifyIcon: %v, ждал %v", msgs, want)
		}
	}
	if win32.WindowOf(hwnd) == w {
		t.Error("окно осталось у бэкенда: обработчик WM_DESTROY забрал сообщение")
	}
	traysMu.Lock()
	_, kept := trays[w]
	traysMu.Unlock()
	if kept {
		t.Error("трей уничтоженного окна остался в карте")
	}
	if err := tr.SetTrayIcon(icon, "после"); !errors.Is(err, native.ErrTrayUnsupported) {
		t.Errorf("SetTrayIcon после гибели окна: %v", err)
	}
	if err := tr.ShowBalloon("t", "x", 0); !errors.Is(err, native.ErrTrayUnsupported) {
		t.Errorf("ShowBalloon после гибели окна: %v", err)
	}
	tr.Close()
	if len(msgs) != len(want) {
		t.Errorf("повторный Close снова снимал иконку: %v", msgs)
	}
}
