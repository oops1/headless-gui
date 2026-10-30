//go:build linux && !android

package shell

import (
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/platform/native"
)

// GTK_THEME важнее портала и не требует шины вовсе: ответ приходит до того,
// как мы к ней потянемся — иначе на машине без D-Bus эта проверка зависла бы.
func TestDetectSystemTheme_GTKThemeEnv(t *testing.T) {
	t.Setenv("GTK_THEME", "Adwaita:dark")
	if got := detectSystemTheme(); got != native.SystemThemeDark {
		t.Errorf("GTK_THEME=Adwaita:dark → %v, ждал dark", got)
	}
	t.Setenv("GTK_THEME", "Adwaita:light")
	if got := detectSystemTheme(); got != native.SystemThemeLight {
		t.Errorf("GTK_THEME=Adwaita:light → %v, ждал light", got)
	}
}

// startFakePortal делает тест порталом настроек на настоящей шине. Если
// портал в системе уже есть (нормальный десктоп), имя занято — тест
// пропускается: подменять живой портал нельзя.
//
// mode: 0 — портал старый, знает только Read (на ReadOne — UnknownMethod);
// иначе — знает ReadOne. scheme — отдаваемый color-scheme.
func startFakePortal(t *testing.T, mode, scheme *atomic.Int32) *dbus.Conn {
	t.Helper()
	daemon := dialTestBus(t)
	daemon.SetCallHandler(func(msg *dbus.Message) *dbus.Reply {
		if msg.Path != portalObjPath || msg.Interface != portalSettingsIface {
			return nil
		}
		if len(msg.Body) != 2 || msg.Body[0] != portalAppearanceNS || msg.Body[1] != portalColorSchemeKey {
			return &dbus.Reply{ErrName: "org.freedesktop.portal.Error.NotFound", ErrMsg: "нет такого ключа"}
		}
		val := dbus.Variant{Sig: "u", Val: uint32(scheme.Load())}
		switch msg.Member {
		case "ReadOne":
			if mode.Load() == 0 {
				return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.UnknownMethod", ErrMsg: "нет ReadOne"}
			}
			return &dbus.Reply{Sig: "v", Body: []any{val}}
		case "Read":
			// Старый метод: variant в variant.
			return &dbus.Reply{Sig: "v", Body: []any{dbus.Variant{Sig: "v", Val: val}}}
		}
		return nil
	})
	if err := daemon.RequestName(portalBusName, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		t.Skipf("портал настроек уже запущен, подменять не будем: %v", err)
	}
	return daemon
}

// TestReadPortalColorScheme_FakePortal — сквозная проверка чтения: оба метода
// портала, новый и устаревший, и переход с первого на второй.
func TestReadPortalColorScheme_FakePortal(t *testing.T) {
	var mode, scheme atomic.Int32
	startFakePortal(t, &mode, &scheme)
	client := dialTestBus(t)

	scheme.Store(1)
	if got := readPortalColorScheme(client); got != native.SystemThemeDark {
		t.Errorf("старый портал (только Read), color-scheme=1 → %v, ждал dark", got)
	}

	mode.Store(1)
	scheme.Store(2)
	if got := readPortalColorScheme(client); got != native.SystemThemeLight {
		t.Errorf("портал с ReadOne, color-scheme=2 → %v, ждал light", got)
	}

	scheme.Store(0)
	if got := readPortalColorScheme(client); got != native.SystemThemeUnknown {
		t.Errorf("color-scheme=0 (нет предпочтения) → %v, ждал unknown", got)
	}
}

// TestReadPortalColorScheme_NoPortal — на шине портала нет: ответ Unknown, а
// не ошибка и не зависание.
func TestReadPortalColorScheme_NoPortal(t *testing.T) {
	client := dialTestBus(t)
	if client.NameHasOwner(portalBusName) {
		t.Skip("портал в системе есть, проверять его отсутствие нечем")
	}
	start := time.Now()
	if got := readPortalColorScheme(client); got != native.SystemThemeUnknown {
		t.Errorf("без портала → %v, ждал unknown", got)
	}
	if d := time.Since(start); d > portalCallTimeout {
		t.Errorf("ответ без портала занял %v: ServiceUnknown должен приходить сразу", d)
	}
}

// TestWatchSystemTheme_FakePortal — сигнал SettingChanged доходит до
// подписчика, чужие ключи и снятая подписка — нет.
func TestWatchSystemTheme_FakePortal(t *testing.T) {
	var mode, scheme atomic.Int32
	daemon := startFakePortal(t, &mode, &scheme)

	got := make(chan native.SystemTheme, 4)
	stop := watchSystemTheme(nil, func(th native.SystemTheme) { got <- th })
	if stop == nil {
		t.Skip("сессионной шины нет")
	}

	emit := func(key string, v uint32) {
		t.Helper()
		err := daemon.Emit(portalObjPath, portalSettingsIface, "SettingChanged", "ssv",
			[]any{portalAppearanceNS, key, dbus.Variant{Sig: "u", Val: v}})
		if err != nil {
			t.Fatalf("emit: %v", err)
		}
	}

	// Акцентный цвет подписчика будить не должен. Шлём его первым: сигналы
	// приходят по порядку, так что если он просочился бы, мы увидели бы его
	// раньше темы.
	emit("accent-color", 7)
	emit(portalColorSchemeKey, 1)
	select {
	case th := <-got:
		if th != native.SystemThemeDark {
			t.Errorf("подписчику пришло %v, ждал dark", th)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("сигнал SettingChanged не доехал до подписчика")
	}

	stop()
	emit(portalColorSchemeKey, 2)
	// Негативная проверка: ждём ровно столько, сколько хватило бы доставке.
	select {
	case th := <-got:
		t.Errorf("подписка снята, но пришло %v", th)
	case <-time.After(300 * time.Millisecond):
	}
}

// startFakeAccentPortal делает тест порталом, отдающим accent-color; как и
// startFakePortal, пропускается, если настоящий портал уже занял имя.
func startFakeAccentPortal(t *testing.T, accent *atomic.Value) *dbus.Conn {
	t.Helper()
	daemon := dialTestBus(t)
	daemon.SetCallHandler(func(msg *dbus.Message) *dbus.Reply {
		if msg.Path != portalObjPath || msg.Interface != portalSettingsIface {
			return nil
		}
		if len(msg.Body) != 2 || msg.Body[0] != portalAppearanceNS || msg.Body[1] != portalAccentKey {
			return &dbus.Reply{ErrName: "org.freedesktop.portal.Error.NotFound", ErrMsg: "нет такого ключа"}
		}
		v := accent.Load().(dbus.Variant)
		switch msg.Member {
		case "ReadOne":
			return &dbus.Reply{Sig: "v", Body: []any{v}}
		case "Read":
			return &dbus.Reply{Sig: "v", Body: []any{dbus.Variant{Sig: "v", Val: v}}}
		}
		return nil
	})
	if err := daemon.RequestName(portalBusName, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		t.Skipf("портал настроек уже запущен, подменять не будем: %v", err)
	}
	return daemon
}

// Чтение accent-color через настоящую шину: структура из трёх double
// превращается в цвет, «нет акцента» — в ok=false.
func TestDetectSystemAccent_FakePortal(t *testing.T) {
	var accent atomic.Value
	accent.Store(accentTestVal(0, 0.5, 1))
	startFakeAccentPortal(t, &accent)
	client := dialTestBus(t)

	v, ok := readPortalAppearance(client, portalAccentKey)
	if !ok {
		t.Fatal("accent-color не прочитан")
	}
	if c, ok := accentFromPortalValue(v); !ok || c != (color.RGBA{R: 0, G: 128, B: 255, A: 255}) {
		t.Errorf("accent-color → %v, %v", c, ok)
	}
	accent.Store(accentTestVal(-1, -1, -1))
	v, _ = readPortalAppearance(client, portalAccentKey)
	if _, ok := accentFromPortalValue(v); ok {
		t.Error("сброшенный акцент (-1,-1,-1) принят за цвет")
	}
}

// Сигнал SettingChanged по accent-color доходит до подписчика на акцент, не
// будит подписчика на тему, а color-scheme не будит подписчика на акцент.
func TestWatchSystemAccent_FakePortal(t *testing.T) {
	var accent atomic.Value
	accent.Store(accentTestVal(0, 0, 0))
	daemon := startFakeAccentPortal(t, &accent)

	type acc struct {
		c  color.RGBA
		ok bool
	}
	gotA := make(chan acc, 4)
	gotT := make(chan native.SystemTheme, 4)
	stopA := watchSystemAccent(nil, func(c color.RGBA, ok bool) { gotA <- acc{c, ok} })
	if stopA == nil {
		t.Skip("сессионной шины нет")
	}
	stopT := watchSystemTheme(nil, func(th native.SystemTheme) { gotT <- th })
	defer stopT()

	emit := func(key string, v any) {
		t.Helper()
		if err := daemon.Emit(portalObjPath, portalSettingsIface, "SettingChanged", "ssv",
			[]any{portalAppearanceNS, key, v}); err != nil {
			t.Fatalf("emit: %v", err)
		}
	}
	// Тему шлём первой: сигналы идут по порядку, и если бы она будила подписчика
	// на акцент, мы увидели бы это раньше цвета.
	emit(portalColorSchemeKey, dbus.Variant{Sig: "u", Val: uint32(1)})
	emit(portalAccentKey, accentTestVal(1, 0, 0))

	select {
	case a := <-gotA:
		if !a.ok || a.c != (color.RGBA{R: 255, A: 255}) {
			t.Errorf("подписчик на акцент получил %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("сигнал accent-color не доехал до подписчика")
	}
	select {
	case th := <-gotT:
		if th != native.SystemThemeDark {
			t.Errorf("подписчик на тему получил %v, ждал dark", th)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("сигнал color-scheme не доехал до подписчика на тему")
	}
	select {
	case a := <-gotA:
		t.Errorf("смена темы разбудила подписчика на акцент: %+v", a)
	case th := <-gotT:
		t.Errorf("смена акцента разбудила подписчика на тему: %v", th)
	case <-time.After(200 * time.Millisecond):
	}

	stopA()
	emit(portalAccentKey, accentTestVal(0, 1, 0))
	select {
	case a := <-gotA:
		t.Errorf("подписка снята, но пришло %+v", a)
	case <-time.After(300 * time.Millisecond):
	}
}
