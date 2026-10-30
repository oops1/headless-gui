//go:build windows

package shell

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/oops1/headless-gui/v4/internal/testhook"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/win32"
)

// TestDetectSystemTheme_ReadsRegistry сверяет ответ с реестром, прочитанным
// независимо: ловит неверный путь раздела или имя значения (AppsUseLightTheme
// и SystemUsesLightTheme легко перепутать).
func TestDetectSystemTheme_ReadsRegistry(t *testing.T) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		t.Skipf("раздела темы нет (Windows до 10 1607?): %v", err)
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		t.Skipf("значения AppsUseLightTheme нет: %v", err)
	}

	want := native.SystemThemeDark
	if v == 1 {
		want = native.SystemThemeLight
	}
	if got := detectSystemTheme(); got != want {
		t.Errorf("detectSystemTheme() = %v при AppsUseLightTheme=%d, ждал %v", got, v, want)
	}
}

// TestDetectSystemAccent_ReadsRegistry сверяет акцент с реестром, прочитанным
// независимо: ловит перепутанный порядок байт (AccentColor — ABGR).
func TestDetectSystemAccent_ReadsRegistry(t *testing.T) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\DWM`, registry.QUERY_VALUE)
	if err != nil {
		t.Skipf("раздела DWM нет: %v", err)
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AccentColor")
	if err != nil {
		t.Skipf("значения AccentColor нет: %v", err)
	}
	want := color.RGBA{R: uint8(v & 0xFF), G: uint8(v >> 8 & 0xFF), B: uint8(v >> 16 & 0xFF), A: 255}
	got, ok := detectSystemAccent()
	if !ok || got != want {
		t.Errorf("detectSystemAccent() = %v, %v при AccentColor=%#x, ждал %v", got, ok, v, want)
	}
}

var (
	testUser32            = windows.NewLazySystemDLL("user32.dll")
	procTestSendMessageW  = testUser32.NewProc("SendMessageW")
	procTestDestroyWindow = testUser32.NewProc("DestroyWindow")
)

// Сквозная проверка на живом окне Win32: WM_SETTINGCHANGE окна доходит до
// подписок, зарегистрированных пакетом (WatchTheme, WatchAccent), через
// обработчик бэкенда (win32.AddMessageHook, его ставит init
// пакета), посторонняя область — нет, и сообщение идёт дальше обычным путём
// (обработчик отвечает handled=false): SPI_SETWORKAREA того же сообщения
// доходит до мониторов бэкенда.
func TestWatchSystemTheme_LiveWindowMessage(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	nw, err := testhook.Win32NewWindow("shell-test", 200, 100)
	if err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	w := nw.(*win32.Win32Window)
	defer procTestDestroyWindow.Call(w.HWND())

	var (
		themes, accents, mons int
		lastT                 native.SystemTheme
		lastA                 color.RGBA
		lastOK                bool
	)
	s := registered(t)
	stopT := s.WatchTheme(w, func(th native.SystemTheme) { themes++; lastT = th })
	stopA := s.WatchAccent(w, func(c color.RGBA, ok bool) { accents++; lastA, lastOK = c, ok })
	stopM := w.WatchMonitors(func() { mons++ })
	defer stopT()
	defer stopA()
	defer stopM()
	send := func(wparam uintptr, area string) {
		p, _ := windows.UTF16PtrFromString(area)
		procTestSendMessageW.Call(w.HWND(), wmSettingChange, wparam, uintptr(unsafe.Pointer(p)))
		runtime.KeepAlive(p)
	}
	send(0, "intl")
	if themes != 0 || accents != 0 || mons != 0 {
		t.Fatalf("посторонняя область дошла до подписок: тема %d, акцент %d, мониторы %d", themes, accents, mons)
	}
	send(0, "ImmersiveColorSet")
	if themes != 1 || accents != 1 {
		t.Fatalf("ImmersiveColorSet через окно: тема %d, акцент %d; ждал по 1", themes, accents)
	}
	if lastT != detectSystemTheme() {
		t.Errorf("подписчику ушла тема %v, а в реестре %v", lastT, detectSystemTheme())
	}
	if wc, wok := detectSystemAccent(); lastA != wc || lastOK != wok {
		t.Errorf("подписчику ушёл акцент %v, %v, а в реестре %v, %v", lastA, lastOK, wc, wok)
	}
	// SPI_SETWORKAREA: одно сообщение и подписчикам оболочки, и мониторам
	// бэкенда — обработчик не забирает его себе.
	const spiSetWorkArea = 0x002F
	send(spiSetWorkArea, "ImmersiveColorSet")
	if mons != 1 || themes != 2 || accents != 2 {
		t.Fatalf("SPI_SETWORKAREA: мониторы %d, тема %d, акцент %d; ждал 1, 2, 2", mons, themes, accents)
	}
}
