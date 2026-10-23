//go:build windows

package window

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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

	want := SystemThemeDark
	if v == 1 {
		want = SystemThemeLight
	}
	if got := DetectSystemTheme(); got != want {
		t.Errorf("DetectSystemTheme() = %v при AppsUseLightTheme=%d, ждал %v", got, v, want)
	}
}

// wmSettingchangeArea отправляет handleSettingChange строку так, как её
// кладёт система: указатель на UTF-16 в lParam.
func wmSettingchangeArea(t *testing.T, w *Win32Window, area string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(area)
	if err != nil {
		t.Fatal(err)
	}
	w.handleSettingChange(uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

// Из WM_SETTINGCHANGE о смене темы говорит только строка ImmersiveColorSet;
// остальные области (раскладка, переменные среды) тему не трогают и реестр
// перечитывать не должны.
func TestWatchSystemTheme_OnlyImmersiveColorSet(t *testing.T) {
	w := &Win32Window{}
	var got []SystemTheme
	stop := watchSystemTheme(w, func(th SystemTheme) { got = append(got, th) })
	if stop == nil {
		t.Fatal("подписка на Win32Window не поднялась")
	}

	wmSettingchangeArea(t, w, "intl")
	wmSettingchangeArea(t, w, "Environment")
	if len(got) != 0 {
		t.Fatalf("посторонние области разбудили подписчика: %v", got)
	}

	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if len(got) != 1 {
		t.Fatalf("ImmersiveColorSet дал %d вызовов, ждал 1", len(got))
	}
	if got[0] != detectSystemTheme() {
		t.Errorf("подписчику ушла тема %v, а в реестре %v", got[0], detectSystemTheme())
	}

	// После снятия подписки сообщения идут мимо.
	stop()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if len(got) != 1 {
		t.Errorf("подписка снята, но вызов был: %v", got)
	}
}

// Без подписчика lParam не разыменовывается вовсе: у части системных
// параметров он не указатель на текст, и приложению, которому тема не нужна,
// это не должно стоить падения.
func TestHandleSettingChange_NoSubscriberDoesNotTouchLParam(t *testing.T) {
	w := &Win32Window{}
	w.handleSettingChange(1) // заведомо недоступный адрес
	w.handleSettingChange(0)

	// С подписчиком нулевой lParam — тоже не повод читать память.
	called := false
	fn := func(string) { called = true }
	w.onSettingChange.Store(&fn)
	w.handleSettingChange(0)
	if called {
		t.Error("подписчик вызван при lParam == 0")
	}
}

// Строка без нуля на конце не должна уводить чтение за пределы лимита:
// читаем не больше settingChangeAreaMax знаков.
func TestReadUTF16Bounded(t *testing.T) {
	buf := make([]uint16, settingChangeAreaMax*2) // ни одного нуля
	for i := range buf {
		buf[i] = 'x'
	}
	got := readUTF16Bounded(unsafe.Pointer(&buf[0]), settingChangeAreaMax)
	if len(got) != settingChangeAreaMax {
		t.Errorf("прочитано %d знаков, ждал ровно предел %d", len(got), settingChangeAreaMax)
	}

	p, _ := windows.UTF16PtrFromString("Intl")
	if s := readUTF16Bounded(unsafe.Pointer(p), settingChangeAreaMax); s != "Intl" {
		t.Errorf("прочитано %q, ждал Intl", s)
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

// Акцент приходит с тем же ImmersiveColorSet, что и тема, но подписки у них
// независимые: у каждой своя ячейка окна, и обе получают одно сообщение.
func TestWatchSystemAccent_IndependentFromTheme(t *testing.T) {
	w := &Win32Window{}
	var themes, accents int
	stopT := watchSystemTheme(w, func(SystemTheme) { themes++ })
	stopA := watchSystemAccent(w, func(color.RGBA, bool) { accents++ })
	if stopT == nil || stopA == nil {
		t.Fatal("подписки на Win32Window не подняты")
	}

	wmSettingchangeArea(t, w, "intl")
	if themes != 0 || accents != 0 {
		t.Fatalf("посторонняя область разбудила подписчиков: %d/%d", themes, accents)
	}
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if themes != 1 || accents != 1 {
		t.Fatalf("ImmersiveColorSet: тема %d, акцент %d вызовов; ждал по 1", themes, accents)
	}

	// Снятие одной подписки вторую не трогает.
	stopT()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if themes != 1 || accents != 2 {
		t.Errorf("после снятия темы: тема %d, акцент %d; ждал 1 и 2", themes, accents)
	}
	stopA()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if accents != 2 {
		t.Errorf("подписка на акцент снята, но вызов был: %d", accents)
	}
	if watchSystemAccent(nil, func(color.RGBA, bool) {}) != nil {
		t.Error("не Win32Window принят за источник сообщений")
	}
}

// Сквозная проверка: сообщение окна → реестр → общий слой окна → колбэк
// приложения. Запомненное «прежнее» оформление искажается, чтобы значение из
// реестра выглядело переменой.
func TestWindowAppearanceWiredToWin32(t *testing.T) {
	cur, hasAccent := detectSystemAccent()
	if !hasAccent {
		t.Skip("акцента в реестре нет")
	}
	w := &Win32Window{}
	win, _ := appearanceTestWin()
	win.native = w
	got := make(chan SystemAppearance, 4)
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { got <- a })
	if w.onSettingChange.Load() == nil || w.onAccentChange.Load() == nil {
		t.Fatal("подписки на тему и акцент не подняты")
	}

	win.sysTheme.mu.Lock()
	win.sysTheme.last.Accent = color.RGBA{R: cur.R ^ 0xFF, G: cur.G, B: cur.B, A: 255}
	win.sysTheme.mu.Unlock()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	select {
	case a := <-got:
		if !a.HasAccent || a.Accent != cur {
			t.Errorf("приложение получило %+v, ждал акцент %v", a, cur)
		}
	default:
		t.Fatal("колбэк оформления не вызван")
	}

	win.SetOnSystemAppearanceChanged(nil)
	if w.onSettingChange.Load() != nil || w.onAccentChange.Load() != nil {
		t.Error("колбэк снят, а подписки окна остались")
	}
}
