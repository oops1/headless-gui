//go:build windows

package window

import (
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
