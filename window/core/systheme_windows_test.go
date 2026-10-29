//go:build windows

package core

import (
	"image/color"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/win32"
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
	if got := DetectSystemTheme(); got != want {
		t.Errorf("DetectSystemTheme() = %v при AppsUseLightTheme=%d, ждал %v", got, v, want)
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
	got, ok := DetectSystemAccent()
	if !ok || got != want {
		t.Errorf("DetectSystemAccent() = %v, %v при AccentColor=%#x, ждал %v", got, ok, v, want)
	}
}

// Сквозная проверка: сообщение окна → реестр → общий слой окна → колбэк
// приложения. Запомненное «прежнее» оформление искажается, чтобы значение из
// реестра выглядело переменой.
func TestWindowAppearanceWiredToWin32(t *testing.T) {
	cur, hasAccent := win32.ReadSystemAccent()
	if !hasAccent {
		t.Skip("акцента в реестре нет")
	}
	w := &Win32Window{}
	win, _ := appearanceTestWin()
	win.native = w
	got := make(chan SystemAppearance, 4)
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { got <- a })
	if theme, accent := testhook.Win32SettingWatched(w); !theme || !accent {
		t.Fatal("подписки на тему и акцент не подняты")
	}

	win.sysTheme.mu.Lock()
	win.sysTheme.last.Accent = color.RGBA{R: cur.R ^ 0xFF, G: cur.G, B: cur.B, A: 255}
	win.sysTheme.mu.Unlock()
	testhook.Win32SettingChange(w, "ImmersiveColorSet")
	select {
	case a := <-got:
		if !a.HasAccent || a.Accent != cur {
			t.Errorf("приложение получило %+v, ждал акцент %v", a, cur)
		}
	default:
		t.Fatal("колбэк оформления не вызван")
	}

	win.SetOnSystemAppearanceChanged(nil)
	if theme, accent := testhook.Win32SettingWatched(w); theme || accent {
		t.Error("колбэк снят, а подписки окна остались")
	}
}
