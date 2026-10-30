//go:build windows

package shell

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/win32"
)

// wmSettingchangeArea отдаёт подпискам окна строку так, как её кладёт
// система: указатель на UTF-16 в lParam.
func wmSettingchangeArea(t *testing.T, w *win32.Win32Window, area string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(area)
	if err != nil {
		t.Fatal(err)
	}
	settingMu.Lock()
	s := settingWins[w]
	settingMu.Unlock()
	if s != nil {
		s.handle(uintptr(unsafe.Pointer(p)))
	}
	runtime.KeepAlive(p)
}

// Из WM_SETTINGCHANGE о смене темы говорит только строка ImmersiveColorSet;
// остальные области (раскладка, переменные среды) тему не трогают и реестр
// перечитывать не должны.
func TestWatchSystemTheme_OnlyImmersiveColorSet(t *testing.T) {
	w := &win32.Win32Window{}
	var got []native.SystemTheme
	stop := watchSystemTheme(w, func(th native.SystemTheme) { got = append(got, th) })
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

	// После снятия подписки сообщения идут мимо, а окно забыто.
	stop()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if len(got) != 1 {
		t.Errorf("подписка снята, но вызов был: %v", got)
	}
	settingMu.Lock()
	_, kept := settingWins[w]
	settingMu.Unlock()
	if kept {
		t.Error("окно без подписок осталось в карте подписок")
	}
}

// Без подписчика lParam не разыменовывается вовсе: у части системных
// параметров он не указатель на текст, и приложению, которому тема не нужна,
// это не должно стоить падения.
func TestHandleSettingChange_NoSubscriberDoesNotTouchLParam(t *testing.T) {
	s := &settingSubs{theme: map[uint64]func(string){}, accent: map[uint64]func(string){}}
	s.handle(1) // заведомо недоступный адрес
	s.handle(0)

	// С подписчиком нулевой lParam — тоже не повод читать память.
	called := false
	s.theme[1] = func(string) { called = true }
	s.handle(0)
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
// независимые: обе получают одно сообщение, и снятие одной другую не трогает.
// Подписчику уходит акцент из реестра.
func TestWatchSystemAccent_IndependentFromTheme(t *testing.T) {
	w := &win32.Win32Window{}
	var (
		themes, accents int
		lastA           color.RGBA
		lastOK          bool
	)
	stopT := watchSystemTheme(w, func(native.SystemTheme) { themes++ })
	stopA := watchSystemAccent(w, func(c color.RGBA, ok bool) { accents++; lastA, lastOK = c, ok })
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
	if wc, wok := detectSystemAccent(); lastA != wc || lastOK != wok {
		t.Errorf("подписчику ушёл акцент %v, %v, а в реестре %v, %v", lastA, lastOK, wc, wok)
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

// Две подписки одного окна (ядро может поднять её дважды и снять лишнюю):
// stop первой не трогает вторую, сообщение до неё доходит, а окно забывается
// только со снятием последней. Старый stop, позванный ещё раз после того, как
// окно забыто и подписано заново, новой подписки не трогает.
func TestWatchSetting_StopRemovesOnlyItself(t *testing.T) {
	w := &win32.Win32Window{}
	var first, second, accents int
	stop1 := watchSystemTheme(w, func(native.SystemTheme) { first++ })
	stop2 := watchSystemTheme(w, func(native.SystemTheme) { second++ })
	stopA1 := watchSystemAccent(w, func(color.RGBA, bool) { accents++ })
	stopA2 := watchSystemAccent(w, func(color.RGBA, bool) { accents += 10 })
	stop1()
	stopA2()
	stop1() // повторный stop — без последствий
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if first != 0 || second != 1 || accents != 1 {
		t.Fatalf("после снятия первых: первая тема %d, вторая %d, акцент %d; ждал 0, 1, 1", first, second, accents)
	}
	stop2()
	settingMu.Lock()
	_, kept := settingWins[w]
	settingMu.Unlock()
	if !kept {
		t.Fatal("окно забыто, хотя подписка на акцент жива")
	}
	stopA1()
	settingMu.Lock()
	_, kept = settingWins[w]
	settingMu.Unlock()
	if kept {
		t.Error("окно без подписок осталось в карте подписок")
	}

	var third int
	stop3 := watchSystemTheme(w, func(native.SystemTheme) { third++ })
	stopA1()
	stop2()
	wmSettingchangeArea(t, w, "ImmersiveColorSet")
	if third != 1 {
		t.Errorf("старый stop снял новую подписку: вызовов %d, ждал 1", third)
	}
	stop3()
	settingMu.Lock()
	_, kept = settingWins[w]
	settingMu.Unlock()
	if kept {
		t.Error("окно без подписок осталось в карте подписок")
	}
}
