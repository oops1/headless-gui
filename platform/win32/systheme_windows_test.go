//go:build windows

package win32

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v3/platform/native"
)

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
	var got []native.SystemTheme
	stop := WatchSystemTheme(w, func(th native.SystemTheme) { got = append(got, th) })
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
	if got[0] != ReadSystemTheme() {
		t.Errorf("подписчику ушла тема %v, а в реестре %v", got[0], ReadSystemTheme())
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

// Акцент приходит с тем же ImmersiveColorSet, что и тема, но подписки у них
// независимые: у каждой своя ячейка окна, и обе получают одно сообщение.
func TestWatchSystemAccent_IndependentFromTheme(t *testing.T) {
	w := &Win32Window{}
	var themes, accents int
	stopT := WatchSystemTheme(w, func(native.SystemTheme) { themes++ })
	stopA := WatchSystemAccent(w, func(color.RGBA, bool) { accents++ })
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
	if WatchSystemAccent(nil, func(color.RGBA, bool) {}) != nil {
		t.Error("не Win32Window принят за источник сообщений")
	}
}
