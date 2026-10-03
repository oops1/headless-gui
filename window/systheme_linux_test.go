//go:build linux && !android

package window

import (
	"sync/atomic"
	"testing"
	"time"
)

// GTK_THEME важнее портала и не требует шины вовсе: ответ приходит до того,
// как мы к ней потянемся — иначе на машине без D-Bus эта проверка зависла бы.
func TestDetectSystemTheme_GTKThemeEnv(t *testing.T) {
	t.Setenv("GTK_THEME", "Adwaita:dark")
	if got := DetectSystemTheme(); got != SystemThemeDark {
		t.Errorf("GTK_THEME=Adwaita:dark → %v, ждал dark", got)
	}
	t.Setenv("GTK_THEME", "Adwaita:light")
	if got := DetectSystemTheme(); got != SystemThemeLight {
		t.Errorf("GTK_THEME=Adwaita:light → %v, ждал light", got)
	}
}

// startFakePortal делает тест порталом настроек на настоящей шине. Если
// портал в системе уже есть (нормальный десктоп), имя занято — тест
// пропускается: подменять живой портал нельзя.
//
// mode: 0 — портал старый, знает только Read (на ReadOne — UnknownMethod);
// иначе — знает ReadOne. scheme — отдаваемый color-scheme.
func startFakePortal(t *testing.T, mode, scheme *atomic.Int32) *dbusConn {
	t.Helper()
	daemon := dialTestBus(t)
	daemon.setCallHandler(func(msg *dbusMessage) *dbusReply {
		if msg.Path != portalObjPath || msg.Interface != portalSettingsIface {
			return nil
		}
		if len(msg.Body) != 2 || msg.Body[0] != portalAppearanceNS || msg.Body[1] != portalColorSchemeKey {
			return &dbusReply{ErrName: "org.freedesktop.portal.Error.NotFound", ErrMsg: "нет такого ключа"}
		}
		val := dbusVariant{Sig: "u", Val: uint32(scheme.Load())}
		switch msg.Member {
		case "ReadOne":
			if mode.Load() == 0 {
				return &dbusReply{ErrName: "org.freedesktop.DBus.Error.UnknownMethod", ErrMsg: "нет ReadOne"}
			}
			return &dbusReply{Sig: "v", Body: []any{val}}
		case "Read":
			// Старый метод: variant в variant.
			return &dbusReply{Sig: "v", Body: []any{dbusVariant{Sig: "v", Val: val}}}
		}
		return nil
	})
	if err := daemon.requestName(portalBusName, 0x4 /*DO_NOT_QUEUE*/); err != nil {
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
	if got := readPortalColorScheme(client); got != SystemThemeDark {
		t.Errorf("старый портал (только Read), color-scheme=1 → %v, ждал dark", got)
	}

	mode.Store(1)
	scheme.Store(2)
	if got := readPortalColorScheme(client); got != SystemThemeLight {
		t.Errorf("портал с ReadOne, color-scheme=2 → %v, ждал light", got)
	}

	scheme.Store(0)
	if got := readPortalColorScheme(client); got != SystemThemeUnknown {
		t.Errorf("color-scheme=0 (нет предпочтения) → %v, ждал unknown", got)
	}
}

// TestReadPortalColorScheme_NoPortal — на шине портала нет: ответ Unknown, а
// не ошибка и не зависание.
func TestReadPortalColorScheme_NoPortal(t *testing.T) {
	client := dialTestBus(t)
	if client.nameHasOwner(portalBusName) {
		t.Skip("портал в системе есть, проверять его отсутствие нечем")
	}
	start := time.Now()
	if got := readPortalColorScheme(client); got != SystemThemeUnknown {
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

	got := make(chan SystemTheme, 4)
	stop := watchSystemTheme(nil, func(th SystemTheme) { got <- th })
	if stop == nil {
		t.Skip("сессионной шины нет")
	}

	emit := func(key string, v uint32) {
		t.Helper()
		err := daemon.emit(portalObjPath, portalSettingsIface, "SettingChanged", "ssv",
			[]any{portalAppearanceNS, key, dbusVariant{Sig: "u", Val: v}})
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
		if th != SystemThemeDark {
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
